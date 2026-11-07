// Package einoopenai provides a bounded, completion-gated Eino Responses model.
// It is an integration component, not an enabled assistant or write approval.
package einoopenai

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	ao "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go/v3/responses"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Limits apply to one whole provider round, including trailing framing/EOF.
// Zero selects the default. Positive overrides may only reduce the defaults.
type Limits struct {
	Bytes   int
	Events  int
	Items   int
	Timeout time.Duration
}

type Config struct {
	ReasoningEffort string
	BaseURL         string // HTTPS; defaults to https://api.openai.com/v1.
	APIKey          string // Host-owned secret; never place it in messages or checkpoints.
	Model           string
	Limits          Limits
	Transport       http.RoundTripper // nil uses http.DefaultTransport (including its proxy).
}

// Model implements Eino's AgenticModel. Stream buffers the provider stream and
// yields one complete verified message. No provisional tool arguments escape.
type Model struct {
	inner  model.AgenticModel
	limits Limits
}

var _ model.AgenticModel = (*Model)(nil)
var errProtocol = errors.New("provider response rejected: invalid or unsuccessful completion")
var errLimit = errors.New("provider response rejected: resource limit")

const nativeKey = "nestworth.responses.output.v1"

type roundKey struct{}
type displayKey struct{}
type nativeState struct {
	Output        []json.RawMessage `json:"output"`
	ContentSHA256 string            `json:"content_sha256"`
}

type round struct {
	display     func(string)
	stream      bool
	replay      map[string][]json.RawMessage
	replayOrder []string
	output      []json.RawMessage
	failure     error
}

// Preserve each complete native output group at its original history position.
// The private markers exist only between the upstream request builder and our
// transport. They are replaced before any request reaches the network.
func (m *Model) prepareInput(in []*schema.AgenticMessage, state *round) ([]*schema.AgenticMessage, error) {
	prepared := make([]*schema.AgenticMessage, len(in))
	nativeBytes, nativeItems := 0, 0
	ids := map[string]bool{}
	for i, msg := range in {
		if msg == nil {
			return nil, errProtocol
		}
		prepared[i] = msg
		raw, exists := msg.Extra[nativeKey]
		if !exists {
			// Generated native messages may not silently fall back to the lossy converter.
			if msg.ResponseMeta != nil && msg.ResponseMeta.OpenAIExtension != nil {
				return nil, errProtocol
			}
			if msg.Role == schema.AgenticRoleTypeAssistant {
				if len(msg.ContentBlocks) != 1 {
					return nil, errProtocol
				}
				for _, block := range msg.ContentBlocks {
					if block == nil || block.Type != schema.ContentBlockTypeAssistantGenText || block.AssistantGenText == nil || block.AssistantGenText.OpenAIExtension != nil {
						return nil, errProtocol
					}
				}
			}
			continue
		}
		s, ok := raw.(string)
		if ok && len(s) > m.limits.Bytes {
			return nil, errLimit
		}
		if !ok || strictJSON([]byte(s)) != nil || msg.Role != schema.AgenticRoleTypeAssistant {
			return nil, errProtocol
		}
		nativeBytes += len(s)
		if nativeBytes > m.limits.Bytes {
			return nil, errLimit
		}
		var snapshot nativeState
		if jsoncodec.Unmarshal([]byte(s), &snapshot) != nil || snapshot.Output == nil {
			return nil, errProtocol
		}
		seal, err := nativeSeal(msg, snapshot.Output)
		if err != nil || seal != snapshot.ContentSHA256 {
			return nil, errProtocol
		}
		nativeItems += len(snapshot.Output)
		if nativeItems > m.limits.Items {
			return nil, errLimit
		}
		for _, item := range snapshot.Output {
			obj, err := validateItem(item)
			if err != nil {
				return nil, err
			}
			id := str(obj, "id")
			if ids[id] {
				return nil, errProtocol
			}
			ids[id] = true
		}
		marker := "nestworth_native_" + rand.Text()
		state.replay[marker] = snapshot.Output
		state.replayOrder = append(state.replayOrder, marker)
		prepared[i] = &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: marker})}}
	}
	return prepared, nil
}

// A consistency check for accidental edits/reordering, never approval authority.
func nativeSeal(msg *schema.AgenticMessage, output []json.RawMessage) (string, error) {
	raw, err := jsoncodec.Marshal(struct {
		Blocks []*schema.ContentBlock `json:"blocks"`
		Output []json.RawMessage      `json:"output"`
	}{msg.ContentBlocks, output})
	if err != nil {
		return "", errProtocol
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func New(ctx context.Context, cfg Config) (*Model, error) { return newModel(ctx, cfg, false) }

func newModel(ctx context.Context, cfg Config, testHTTP bool) (*Model, error) {
	if !assistant.ValidReasoning(cfg.Model, cfg.ReasoningEffort) {
		return nil, assistant.ErrUnavailable
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && !(testHTTP && u.Scheme == "http" && u.Hostname() == "127.0.0.1")) {
		return nil, errors.New("provider requires an HTTPS base URL without credentials, query or fragment")
	}
	if cfg.APIKey == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("provider requires a key and model")
	}
	limits := cfg.Limits
	if limits.Bytes == 0 {
		limits.Bytes = 4 << 20
	}
	if limits.Events == 0 {
		limits.Events = 4096
	}
	if limits.Items == 0 {
		limits.Items = 128
	}
	if limits.Timeout == 0 {
		limits.Timeout = 60 * time.Second
	}
	if limits.Bytes < 1 || limits.Bytes > 4<<20 || limits.Events < 1 || limits.Events > 4096 || limits.Items < 1 || limits.Items > 128 || limits.Timeout < 1 || limits.Timeout > 60*time.Second {
		return nil, errLimit
	}
	target := *u
	target.Path = strings.TrimRight(target.Path, "/") + "/responses"
	inner := cfg.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}
	client := &http.Client{Transport: &transport{inner: inner, target: target.String(), limits: limits}, Timeout: limits.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	extra := map[string]any{}
	if cfg.ReasoningEffort != "" {
		extra["reasoning"] = map[string]string{"effort": cfg.ReasoningEffort}
	}
	store, retries := false, 0
	upstream, err := ao.NewResponsesModel(ctx, &ao.ResponsesConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL,
		Model: cfg.Model, Store: &store, EnableAutoCache: false, Include: []responses.ResponseIncludable{"reasoning.encrypted_content"},
		ExtraFields: extra, MaxRetries: &retries, HTTPClient: client})
	if err != nil {
		return nil, errors.New("provider configuration rejected")
	}
	return &Model{inner: upstream, limits: limits}, nil
}

func (m *Model) Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	_, live := ctx.Value(displayKey{}).(func(string))
	return m.generate(ctx, in, live, opts...)
}
func (m *Model) Stream(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg, err := m.generate(ctx, in, true, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.AgenticMessage{msg}), nil
}
func (m *Model) generate(ctx context.Context, in []*schema.AgenticMessage, stream bool, opts ...model.Option) (*schema.AgenticMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, m.limits.Timeout)
	defer cancel()
	display, _ := ctx.Value(displayKey{}).(func(string))
	state := &round{display: display, stream: stream, replay: map[string][]json.RawMessage{}}
	prepared, err := m.prepareInput(in, state)
	if errors.Is(err, errLimit) {
		return nil, assistant.BudgetFailure("provider_payload")
	}
	if err != nil {
		return nil, assistant.ErrModelHistory
	}
	ctx = context.WithValue(ctx, roundKey{}, state)
	// The upstream nonstream converter preserves complete native reasoning. Its
	// unsafe streaming converter is never invoked. The transport changes the wire
	// mode and returns the verified terminal response as JSON to the same SDK.
	msg, err := m.inner.Generate(ctx, prepared, opts...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if state.failure != nil {
			return nil, state.failure
		}
		return nil, assistant.ProviderResponseFailure("sdk_conversion")
	} // SDK errors may contain provider bodies.
	if msg == nil {
		return nil, assistant.ProviderResponseFailure("sdk_conversion")
	}
	seal, err := nativeSeal(msg, state.output)
	if err != nil {
		return nil, assistant.ProviderResponseFailure("sdk_conversion")
	}
	raw, err := jsoncodec.Marshal(nativeState{Output: state.output, ContentSHA256: seal})
	if err != nil {
		return nil, assistant.ProviderResponseFailure("sdk_conversion")
	}
	if msg.Extra == nil {
		msg.Extra = map[string]any{}
	}
	msg.Extra[nativeKey] = string(raw)

	return msg, nil
}

type transport struct {
	inner  http.RoundTripper
	target string
	limits Limits
}

func (t *transport) RoundTrip(req *http.Request) (responseOut *http.Response, failure error) {
	phase := "request_contract"
	// The SDK may wrap/replace transport errors. Keep only our fixed classification
	// in the request-local round, never arbitrary SDK or provider text.
	defer func() {
		if failure != nil {
			if state, ok := req.Context().Value(roundKey{}).(*round); ok {
				switch {
				case errors.Is(failure, errLimit):
					state.failure = assistant.BudgetFailure("provider_payload")
				case errors.Is(failure, errProtocol):
					state.failure = assistant.ProviderResponseFailure(phase)
				default:
					state.failure = failure
				}
			}
		}
	}()
	state, ok := req.Context().Value(roundKey{}).(*round)
	if !ok || req.URL.String() != t.target || req.Method != http.MethodPost || req.Body == nil {
		return nil, errProtocol
	}
	body, err := boundedRead(req.Body, t.limits.Bytes)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	fields, err := object(body)
	if err != nil {
		return nil, errProtocol
	}
	// Enforce these at the wire boundary, including per-call upstream options.
	for _, name := range []string{"previous_response_id", "conversation", "background", "prompt_cache_key", "prompt_cache_retention"} {
		if _, ok := fields[name]; ok {
			return nil, errProtocol
		}
	}
	var input []json.RawMessage
	if jsoncodec.Unmarshal(fields["input"], &input) != nil {
		return nil, errProtocol
	}
	seen := map[string]bool{}
	var restored []json.RawMessage
	for _, raw := range input {
		item, err := object(raw)
		if err != nil {
			return nil, errProtocol
		}
		marker := nativeMarker(item)
		if original, ok := state.replay[marker]; ok {
			if seen[marker] || len(seen) >= len(state.replayOrder) || state.replayOrder[len(seen)] != marker {
				return nil, errProtocol
			}
			seen[marker] = true
			restored = append(restored, original...)
		} else {
			if str(item, "type") == "reasoning" {
				return nil, errProtocol
			}
			restored = append(restored, raw)
		}
	}
	if len(seen) != len(state.replay) {
		return nil, errProtocol
	}
	if restored == nil {
		restored = []json.RawMessage{}
	}
	fields["input"], _ = jsoncodec.Marshal(restored)
	fields["stream"], _ = jsoncodec.Marshal(state.stream)
	fields["store"] = json.RawMessage("false")
	fields["include"] = json.RawMessage(`["reasoning.encrypted_content"]`)
	fields["truncation"] = json.RawMessage(`"disabled"`)
	// Only host function tools are supported; no remote/provider-executed tools.
	if raw, ok := fields["tools"]; ok {
		var tools []map[string]json.RawMessage
		if jsoncodec.Unmarshal(raw, &tools) != nil {
			return nil, errProtocol
		}
		for _, tool := range tools {
			if str(tool, "type") != "function" {
				return nil, errProtocol
			}
			// Shared capability schemas intentionally have optional fields. Explicitly
			// retain omission semantics rather than Responses' implicit strict conversion.
			tool["strict"] = json.RawMessage("false")
		}
		fields["tools"], _ = jsoncodec.Marshal(tools)
	}
	body, err = jsoncodec.Marshal(fields)
	if err != nil || len(body) > t.limits.Bytes {
		return nil, errLimit
	}
	wire := req.Clone(req.Context())
	wire.Body = io.NopCloser(bytes.NewReader(body))
	wire.ContentLength = int64(len(body))
	wire.GetBody = nil
	usage := assistant.BeginUsage(req.Context(), str(fields, "model"))
	if req.URL.Scheme == "https" && req.URL.Host == "api.openai.com" && req.URL.Path == "/v1/responses" {
		usage.PricingScope = "openai-public"
	}
	defer func() { assistant.FinishUsage(req.Context(), usage, failure == nil) }()
	response, err := t.inner.RoundTrip(wire)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, assistant.ErrProviderConnection
	}
	phase = "response_headers"
	if response == nil || response.Body == nil {
		return nil, errProtocol
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case 401, 403:
			return nil, assistant.ErrProviderAuthentication
		case 429:
			return nil, assistant.ErrProviderRateLimited
		default:
			if response.StatusCode >= 500 {
				return nil, assistant.ErrProviderUnavailable
			}
			return nil, assistant.ErrProviderRequest
		}
	}
	phase = "response_headers"
	typ, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (state.stream && typ != "text/event-stream") || (!state.stream && typ != "application/json") {
		return nil, errProtocol
	}
	phase = "response_body"
	var raw []byte
	var output []json.RawMessage
	if state.stream {
		raw, output, err = validateStreamReader(response.Body, t.limits, state.display, func(raw []byte) { assistant.CaptureUsage(&usage, raw) })
	} else {
		raw, err = boundedRead(response.Body, t.limits.Bytes)
		if err == nil {
			assistant.CaptureUsage(&usage, raw)
			output, err = validateResponse(raw, t.limits)
		}
	}
	if err != nil {
		return nil, err
	}
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}
	state.output = output
	copy := *response
	copy.Header = response.Header.Clone()
	copy.Header.Set("Content-Type", "application/json")
	copy.Header.Del("Content-Encoding")
	copy.Header.Del("Content-Length")
	copy.ContentLength = int64(len(raw))
	copy.Body = io.NopCloser(bytes.NewReader(raw))
	return &copy, nil
}
func boundedRead(r io.Reader, limit int) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if len(raw) > limit {
		return nil, errLimit
	}
	if err != nil {
		return nil, errProtocol
	}
	return raw, nil
}

func nativeMarker(item map[string]json.RawMessage) string {
	if str(item, "role") != "assistant" || str(item, "type") != "message" {
		return ""
	}
	var content []map[string]json.RawMessage
	if jsoncodec.Unmarshal(item["content"], &content) != nil || len(content) != 1 || str(content[0], "type") != "output_text" {
		return ""
	}
	return str(content[0], "text")
}
