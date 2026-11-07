package einoopenai

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Engine uses the completion-gated Eino model in a bounded serial tool loop.
// No auto-resume, summarizer, retry, approval or writer is installed.
// Each saved checkpoint is a complete turn, including untouched native items.
type Engine struct {
	model      model.AgenticModel
	tools      map[string]tool.InvokableTool
	infos      []*schema.ToolInfo
	catalog    func(bool) ([]agentcapability.Definition, error)
	config     assistant.ModelConfig
	secret     []byte
	close      func()
	evidence   func() []assistant.ReadEvidence
	navigation func() *assistant.ModelNavigationTarget
}
type checkpoint struct {
	CatalogFull bool                     `json:"catalog_full,omitempty"`
	Version     int                      `json:"version"`
	Provider    string                   `json:"provider"`
	Model       string                   `json:"model"`
	Messages    []*schema.AgenticMessage `json:"messages"`
}

var errEngine = errors.New("assistant model run refused")

func NewEngine(ctx context.Context, cfg assistant.ModelConfig, key []byte, session CapabilitySession, close func()) (*Engine, error) {
	if !cfg.Valid() {
		return nil, errEngine
	}
	m, err := New(ctx, Config{APIKey: string(key), Model: cfg.Model})
	if err != nil {
		return nil, errEngine
	}
	out, err := engineWithModel(ctx, cfg, m, session, close)
	if err != nil {
		return nil, err
	}
	out.secret = append([]byte(nil), key...)
	return out, nil
}
func engineWithModel(ctx context.Context, cfg assistant.ModelConfig, m model.AgenticModel, session CapabilitySession, close func()) (*Engine, error) {
	tools, err := CapabilityTools(session)
	if err != nil {
		return nil, errEngine
	}
	out := &Engine{model: m, config: cfg, tools: map[string]tool.InvokableTool{}, close: close}
	if catalog, ok := session.(interface {
		CatalogDefinitions(bool) ([]agentcapability.Definition, error)
	}); ok {
		out.catalog = catalog.CatalogDefinitions
	}
	if trace, ok := session.(interface {
		Evidence() []assistant.ReadEvidence
	}); ok {
		out.evidence = trace.Evidence
	}
	if navigation, ok := session.(interface {
		Navigation() *assistant.ModelNavigationTarget
	}); ok {
		out.navigation = navigation.Navigation
	}
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, errEngine
		}
		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			return nil, errEngine
		}
		out.infos = append(out.infos, info)
		out.tools[info.Name] = invokable
	}
	return out, nil
}
func (e *Engine) Close() {
	clear(e.secret)
	e.secret = nil
	if e.close != nil {
		e.close()
		e.close = nil
	}
	e.model = nil
}
func (e *Engine) Complete(ctx context.Context, history []byte, prompt string) (assistant.Completion, error) {
	empty := assistant.Completion{}
	if ctx.Err() != nil {
		return empty, ctx.Err()
	}
	if len(prompt) > assistant.MaxPromptBytes {
		return empty, assistant.BudgetFailure("prompt_bytes")
	}
	if len(history) > assistant.MaxHistoryBytes {
		return empty, assistant.BudgetFailure("history_bytes")
	}
	if e.model == nil || e.tainted([]byte(prompt)) || e.tainted(history) {
		return empty, errEngine
	}
	instructions := commonInstructions()
	saved := checkpoint{Version: 1, Provider: e.config.Provider, Model: e.config.Model, Messages: []*schema.AgenticMessage{schema.SystemAgenticMessage(instructions)}}
	if len(history) > 0 {
		if strictJSON(history) != nil || jsoncodec.Unmarshal(history, &saved) != nil || (saved.Version != 1 && saved.Version != 2) || saved.Provider != e.config.Provider || saved.Model != e.config.Model || len(saved.Messages) == 0 {
			return empty, assistant.ErrModelHistory
		}
	}
	// Refresh only host-owned policy, never assistant-native items or reasoning.
	// Legacy v1 conversations migrate once; thereafter this prefix is invariant.
	// Native items, reasoning and earlier per-turn mode messages remain untouched.
	if saved.Messages[0] == nil || saved.Messages[0].Role != schema.AgenticRoleTypeSystem {
		return empty, assistant.ErrModelHistory
	}
	saved.Messages[0] = schema.SystemAgenticMessage(instructions)
	saved.Version = 2
	saved.CatalogFull = saved.CatalogFull || e.tools["commit_plan"] != nil
	infos := e.infos
	if e.catalog != nil {
		definitions, err := e.catalog(saved.CatalogFull)
		if err != nil {
			return empty, errEngine
		}
		infos, err = catalogInfos(definitions)
		if err != nil {
			return empty, errEngine
		}
	}
	catalogNames := make(map[string]bool, len(infos))
	for _, info := range infos {
		catalogNames[info.Name] = true
	}
	allowed := make([]string, 0, len(e.tools))
	for name := range e.tools {
		if !catalogNames[name] {
			return empty, errEngine
		}
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)
	allowedTools := make([]*schema.AllowedTool, 0, len(allowed))
	for _, name := range allowed {
		allowedTools = append(allowedTools, &schema.AllowedTool{FunctionName: name})
	}
	choice := &schema.AgenticToolChoice{Type: schema.ToolChoiceAllowed, Allowed: &schema.AgenticAllowedToolChoice{Tools: allowedTools}}
	if len(allowedTools) == 0 {
		choice = &schema.AgenticToolChoice{Type: schema.ToolChoiceForbidden}
	}
	saved.Messages = appendModeSnapshot(saved.Messages, e.tools["commit_plan"] != nil)
	saved.Messages = append(saved.Messages, schema.UserAgenticMessage(prompt))
	seenCalls := map[string]bool{}
	calls := 0
	executed := 0
	for _, msg := range saved.Messages {
		if msg == nil {
			return empty, errEngine
		}
		for _, b := range msg.ContentBlocks {
			if b != nil && b.FunctionToolCall != nil {
				seenCalls[b.FunctionToolCall.CallID] = true
			}
		}
	}
	for round := range 8 {
		raw, err := jsoncodec.Marshal(saved)
		if err != nil || e.tainted(raw) {
			return empty, errEngine
		}
		if len(raw) > assistant.MaxHistoryBytes {
			return empty, assistant.BudgetFailure("history_bytes")
		}
		if len(saved.Messages) > 128 {
			return empty, assistant.BudgetFailure("history_messages")
		}
		roundCtx := ctx
		if assistant.HasProgress(ctx) {
			assistant.EmitProgress(ctx, assistant.ProgressEvent{Kind: "text"})
			last := time.Time{}
			roundCtx = context.WithValue(ctx, displayKey{}, func(text string) {
				if e.tainted([]byte(text)) {
					return
				}
				// Keep a secret-sized suffix private so a key split over deltas
				// cannot escape before the complete value can be detected.
				end := len(text) - len(e.secret)
				if end < 0 {
					return
				}
				for end > 0 && end < len(text) && !utf8.RuneStart(text[end]) {
					end--
				}
				if time.Since(last) < 50*time.Millisecond {
					return
				}
				last = time.Now()
				assistant.EmitProgress(ctx, assistant.ProgressEvent{Kind: "text", Text: text[:end]})
			})
		}

		// Keep the existing eight-response limit, reserving its last response
		// for a final answer. Hitting the tool cap also ends further reads.
		// Add policy to a detached request so it never becomes saved history.
		final := round == 7 || calls == 16
		input := saved.Messages
		options := []model.Option{model.WithTools(infos), model.WithAgenticToolChoice(choice)}
		if final {
			input = append(append([]*schema.AgenticMessage(nil), saved.Messages...), schema.SystemAgenticMessage("The controlled read budget for this request is exhausted. No more tools are available. Answer using only evidence already read for this request. Explicitly disclose incomplete pages, missing evidence and unresolved questions; ask for a narrower question where needed. Never present partial results as complete or invent financial facts."))
			options = append(options, model.WithAgenticToolChoice(&schema.AgenticToolChoice{Type: schema.ToolChoiceForbidden}))
		}
		if len(input) > 128 {
			return empty, assistant.BudgetFailure("history_messages")
		}
		msg, err := e.model.Generate(roundCtx, input, options...)
		if err != nil {
			return empty, err
		}
		if ctx.Err() != nil {
			return empty, ctx.Err()
		}
		if msg == nil {
			return empty, assistant.ProviderResponseFailure("sdk_conversion")
		}
		encoded, encodeErr := jsoncodec.Marshal(msg)
		if encodeErr != nil || e.tainted(encoded) {
			return empty, errEngine
		}
		// Validate the entire call inventory before execution. The actual model has
		// already checked terminal success AND clean wire EOF (#201).
		pending := []*schema.FunctionToolCall{}
		for _, b := range msg.ContentBlocks {
			if b == nil {
				return empty, errEngine
			}
			if b.Type == schema.ContentBlockTypeFunctionToolCall {
				call := b.FunctionToolCall
				if call == nil || call.CallID == "" || seenCalls[call.CallID] || e.tools[call.Name] == nil || len(call.Arguments) > 64<<10 {
					return empty, assistant.ProviderResponseFailure("engine_inventory")
				}
				seenCalls[call.CallID] = true
				pending = append(pending, call)
			}
		}
		if final && len(pending) > 0 {
			rule := "model_rounds"
			if calls == 16 {
				rule = "tool_calls"
			}
			return empty, assistant.BudgetFailure(rule)
		}
		calls += len(pending)
		if calls > 16 {
			return empty, assistant.BudgetFailure("tool_calls")
		}
		saved.Messages = append(saved.Messages, msg)
		if len(saved.Messages) > 128 {
			return empty, assistant.BudgetFailure("history_messages")
		}
		if len(pending) == 0 {
			var text strings.Builder
			for _, b := range msg.ContentBlocks {
				if b.Type == schema.ContentBlockTypeAssistantGenText && b.AssistantGenText != nil {
					text.WriteString(b.AssistantGenText.Text)
					// The SDK represents refusal text in an extension rather
					// than Text. A complete refusal is a displayable answer.
					if b.AssistantGenText.Text == "" && b.AssistantGenText.OpenAIExtension != nil && b.AssistantGenText.OpenAIExtension.Refusal != nil {
						text.WriteString(b.AssistantGenText.OpenAIExtension.Refusal.Reason)
					}
				}
			}
			raw, err := jsoncodec.Marshal(saved)
			if err != nil {
				return empty, errEngine
			}
			if len(raw) > assistant.MaxHistoryBytes {
				return empty, assistant.BudgetFailure("history_bytes")
			}
			if text.Len() > assistant.MaxTextBytes {
				return empty, assistant.BudgetFailure("text_bytes")
			}
			if text.Len() == 0 {
				return empty, assistant.ProviderResponseFailure("engine_empty_text")
			}
			result := assistant.Completion{History: raw, Text: text.String()}
			if e.evidence != nil {
				result.Evidence = e.evidence()
			}
			if e.navigation != nil {
				result.Navigation = e.navigation()
			}
			return result, nil
		}
		for _, call := range pending {
			if ctx.Err() != nil {
				return empty, ctx.Err()
			}
			executed++
			notify := func(state string) {
				assistant.EmitProgress(ctx, assistant.ProgressEvent{Kind: "tool", Tool: executed, Source: call.Name, State: state, Input: call.Arguments})
			}
			notify("running")
			result, err := e.tools[call.Name].InvokableRun(ctx, call.Arguments)
			status := "succeeded"
			if err != nil {
				status = "failed"
			}
			if errors.Is(err, errCapabilityRefused) {
				status = "refused"
			}
			if errors.Is(err, errCapabilityWriteRecoveryRequired) {
				status = "refused"
				result = `{"error":{"code":"write_blocked_pending_operation","recovery":"An earlier operation remains unresolved. Inspect its original receipt and current data. Do not retry it or create a replacement mutation; this new attempt was not dispatched."}}`
			}
			if errors.Is(err, errCapabilityWriteRejected) {
				status = "refused"
				result = writeNotSubmittedDiagnostic
			}
			if errors.Is(err, errCapabilityWriteRefused) {
				result = `{"error":{"code":"write_requires_inspection","recovery":"Inspect the original operation receipt. This failure is not evidence of rollback. Do not retry, create another operation identity or prepare a replacement mutation."}}`
				// Stop this turn: subsequent model calls must not reconstruct a mutation
				// after a missing response. The desktop retains its recovery reference.
			}
			if errors.Is(err, errPageOperationRefused) && ctx.Err() == nil {
				status = "refused"
				result = pageOperationRefusedDiagnostic
				err = nil
			}
			if errors.Is(err, errCapabilityRefused) && ctx.Err() == nil {
				result = `{"error":{"code":"controlled_read_refused","recovery":"No financial fact was returned. The capture may be expired/revoked, arguments invalid, or the read unavailable. Re-capture the initial read and restart relevant pages; never mix old and new capture IDs or treat refusal as zero. Do not widen permissions or disclosure."}}`
				err = nil
			}
			assistant.EmitProgress(ctx, assistant.ProgressEvent{Kind: "tool", Tool: executed, Source: call.Name, State: status, Output: result})
			if ctx.Err() != nil {
				return empty, ctx.Err()
			}
			if budget := assistant.NormalizeBudgetFailure(err); budget != nil {
				return empty, budget
			}
			if err != nil {
				return empty, assistant.ErrToolResult
			}
			saved.Messages = append(saved.Messages, &schema.AgenticMessage{Role: schema.AgenticRoleTypeUser, ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolResult{CallID: call.CallID, Name: call.Name, Content: []*schema.FunctionToolResultContentBlock{{Type: schema.FunctionToolResultContentBlockTypeText, Text: &schema.UserInputText{Text: result}}}}),
			}})
		}
	}
	return empty, assistant.BudgetFailure("model_rounds")
}

var _ assistant.Engine = (*Engine)(nil)

func (e *Engine) tainted(raw []byte) bool { return len(e.secret) > 0 && bytes.Contains(raw, e.secret) }

// A snapshot only saves prompt space; it is never execution authority. Compare
// the latest system message (not a checkpoint flag or any earlier matching
// message) with the exact current host policy. Legacy/changed/unknown policy
// gets a new snapshot. Preserve existing history, including old duplicates.
func appendModeSnapshot(messages []*schema.AgenticMessage, full bool) []*schema.AgenticMessage {
	current := schema.SystemAgenticMessage(modeInstructions(full))
	for i := len(messages) - 1; i > 0; i-- {
		if message := messages[i]; message != nil && message.Role == schema.AgenticRoleTypeSystem {
			if reflect.DeepEqual(message, current) {
				return messages
			}
			break
		}
	}
	return append(messages, current)
}
