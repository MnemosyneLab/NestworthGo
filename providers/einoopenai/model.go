// Package einoopenai provides a bounded, completion-gated Eino Responses model.
// It is an integration component, not an enabled assistant or write approval.
package einoopenai

import (
	"bytes"
	"context"
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
	BaseURL   string // HTTPS; defaults to https://api.openai.com/v1.
	APIKey    string // Host-owned secret; never place it in messages or checkpoints.
	Model     string
	Limits    Limits
	Transport http.RoundTripper // nil uses http.DefaultTransport (including its proxy).
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

const nativeKey = "nestworth.responses.reasoning.v1"

type roundKey struct{}
type round struct {
	stream      bool
	native      map[string]json.RawMessage
	nativeOrder []string
	output      []json.RawMessage
}

func New(ctx context.Context, cfg Config) (*Model, error) { return newModel(ctx, cfg, false) }

func newModel(ctx context.Context, cfg Config, testHTTP bool) (*Model, error) {
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
	store, retries := false, 0
	upstream, err := ao.NewResponsesModel(ctx, &ao.ResponsesConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL,
		Model: cfg.Model, Store: &store, EnableAutoCache: false, Include: []responses.ResponseIncludable{"reasoning.encrypted_content"},
		MaxRetries: &retries, HTTPClient: client})
	if err != nil {
		return nil, errors.New("provider configuration rejected")
	}
	return &Model{inner: upstream, limits: limits}, nil
}

func (m *Model) Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	return m.generate(ctx, in, false, opts...)
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
	state := &round{stream: stream, native: map[string]json.RawMessage{}}
	nativeBytes := 0
	for _, msg := range in {
		if msg == nil {
			return nil, errProtocol
		}
		raw, exists := msg.Extra[nativeKey]
		if !exists {
			continue
		}
		s, ok := raw.(string)
		if !ok || len(s) > m.limits.Bytes || strictJSON([]byte(s)) != nil {
			return nil, errProtocol
		}
		nativeBytes += len(s)
		if nativeBytes > m.limits.Bytes {
			return nil, errLimit
		}
		var items []json.RawMessage
		if jsoncodec.Unmarshal([]byte(s), &items) != nil || len(items) > m.limits.Items {
			return nil, errProtocol
		}
		for _, item := range items {
			obj, err := validateItem(item)
			if err != nil || str(obj, "type") != "reasoning" || str(obj, "id") == "" {
				return nil, errProtocol
			}
			id := str(obj, "id")
			if _, exists := state.native[id]; exists {
				return nil, errProtocol
			}
			if len(state.native) >= m.limits.Items {
				return nil, errLimit
			}
			state.native[id] = item
			state.nativeOrder = append(state.nativeOrder, id)
		}
	}
	ctx = context.WithValue(ctx, roundKey{}, state)
	// The upstream nonstream converter preserves complete native reasoning. Its
	// unsafe streaming converter is never invoked. The transport changes the wire
	// mode and returns the verified terminal response as JSON to the same SDK.
	msg, err := m.inner.Generate(ctx, in, opts...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, errProtocol
	} // SDK errors may contain provider bodies.
	if msg == nil {
		return nil, errProtocol
	}
	var native []json.RawMessage
	for _, raw := range state.output {
		obj, _ := object(raw)
		if str(obj, "type") == "reasoning" {
			native = append(native, raw)
		}
	}
	if len(native) > 0 {
		raw, err := jsoncodec.Marshal(native)
		if err != nil {
			return nil, errProtocol
		}
		if msg.Extra == nil {
			msg.Extra = map[string]any{}
		}
		msg.Extra[nativeKey] = string(raw)
	}
	return msg, nil
}

type transport struct {
	inner  http.RoundTripper
	target string
	limits Limits
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
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
	for i, raw := range input {
		item, err := object(raw)
		if err != nil {
			return nil, errProtocol
		}
		if str(item, "type") == "reasoning" {
			id := str(item, "id")
			original, ok := state.native[id]
			if !ok || seen[id] || len(seen) >= len(state.nativeOrder) || state.nativeOrder[len(seen)] != id {
				return nil, errProtocol
			}
			input[i], seen[id] = original, true
		}
	}
	if len(seen) != len(state.native) {
		return nil, errProtocol
	}
	fields["input"], _ = jsoncodec.Marshal(input)
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
		}
	}
	body, err = jsoncodec.Marshal(fields)
	if err != nil || len(body) > t.limits.Bytes {
		return nil, errLimit
	}
	wire := req.Clone(req.Context())
	wire.Body = io.NopCloser(bytes.NewReader(body))
	wire.ContentLength = int64(len(body))
	wire.GetBody = nil
	response, err := t.inner.RoundTrip(wire)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, errProtocol
	}
	if response == nil || response.Body == nil {
		return nil, errProtocol
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errProtocol
	}
	typ, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (state.stream && typ != "text/event-stream") || (!state.stream && typ != "application/json") {
		return nil, errProtocol
	}
	raw, err := boundedRead(response.Body, t.limits.Bytes)
	if err != nil {
		return nil, err
	}
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}
	var output []json.RawMessage
	if state.stream {
		raw, output, err = validateStream(raw, t.limits)
	} else {
		output, err = validateResponse(raw, t.limits)
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
