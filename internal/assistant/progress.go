package assistant

import (
	"context"
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"strings"
	"unicode/utf8"
)

// ProgressEvent is transient display telemetry, never a checkpoint or authority.
// Text is a cumulative snapshot for the current model round; tools carry only
// an ordinal, a code-owned source name and bounded credential-filtered details.
type ProgressEvent struct {
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	Tool   int    `json:"tool,omitempty"`
	Source string `json:"source,omitempty"`
	State  string `json:"state,omitempty"`
}
type ToolProgress struct {
	Input           string `json:"input"`
	Output          string `json:"output"`
	InputTruncated  bool   `json:"inputTruncated"`
	OutputTruncated bool   `json:"outputTruncated"`
	Source          string `json:"source"`
	State           string `json:"state"`
}
type Progress struct {
	RunID    string         `json:"runId"`
	Revision uint64         `json:"revision"`
	Text     string         `json:"text"`
	Tools    []ToolProgress `json:"tools"`
}
type progressKey struct{}

func WithProgress(ctx context.Context, observe func(ProgressEvent)) context.Context {
	return context.WithValue(ctx, progressKey{}, observe)
}
func HasProgress(ctx context.Context) bool {
	_, ok := ctx.Value(progressKey{}).(func(ProgressEvent))
	return ok
}
func EmitProgress(ctx context.Context, event ProgressEvent) {
	if observe, ok := ctx.Value(progressKey{}).(func(ProgressEvent)); ok && ctx.Err() == nil {
		observe(event)
	}
}

// A snapshot is recoverable while this process is alive; no display event replay.
func (r *Runtime) progressView(v *Conversation) {
	if p := r.progress[v.ID]; p != nil {
		copy := *p
		copy.Tools = append([]ToolProgress(nil), p.Tools...)
		v.Progress = &copy
	}
}
func (r *Runtime) observe(ctx context.Context, id, runID string, key []byte, event ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil || r.valid(ctx, r.epoch) != nil || !r.state.Config.Enabled {
		return
	}
	c := r.find(id)
	if c == nil || len(c.Runs) == 0 {
		return
	}
	run := c.Runs[len(c.Runs)-1].Run
	if run.ID != runID || terminal(run.State) {
		return
	}
	if r.progress == nil {
		r.progress = map[string]*Progress{}
	}
	p := r.progress[id]
	if p == nil || p.RunID != runID {
		p = &Progress{RunID: runID, Tools: []ToolProgress{}}
		r.progress[id] = p
	}
	switch event.Kind {
	case "text":
		if len(event.Text) > MaxTextBytes || !utf8.ValidString(event.Text) || strings.Contains(event.Text, string(key)) {
			return
		}
		p.Text = event.Text
	case "tool":
		if event.Tool < 1 || event.Tool > 16 || !progressSource(event.Source) {
			return
		}
		if event.State == "running" && event.Tool == len(p.Tools)+1 {
			detail, truncated := toolDisplay(event.Input, key)
			p.Tools = append(p.Tools, ToolProgress{Source: event.Source, State: event.State, Input: detail, InputTruncated: truncated})
		} else if event.Tool <= len(p.Tools) && p.Tools[event.Tool-1].Source == event.Source && p.Tools[event.Tool-1].State == "running" && (event.State == "succeeded" || event.State == "failed" || event.State == "refused") {
			p.Tools[event.Tool-1].State = event.State
			p.Tools[event.Tool-1].Output, p.Tools[event.Tool-1].OutputTruncated = toolDisplay(event.Output, key)
		} else {
			return
		}
	default:
		return
	}
	// Bound all tool details in one attempt, including repeated pages.
	total := 0
	for i := range p.Tools {
		for _, field := range []struct {
			value     *string
			truncated *bool
		}{{&p.Tools[i].Input, &p.Tools[i].InputTruncated}, {&p.Tools[i].Output, &p.Tools[i].OutputTruncated}} {
			remaining := (64 << 10) - total
			if len(*field.value) > remaining {
				for remaining > 0 && !utf8.RuneStart((*field.value)[remaining]) {
					remaining--
				}
				*field.value = (*field.value)[:remaining]
				*field.truncated = true
			}
			total += len(*field.value)
		}
	}
	p.Revision++
}
func progressSource(s string) bool {
	switch s {
	case "get_financial_context_page", "get_financial_comparison_page":
		return false
	// Retired proposal names are accepted only as saved transcript evidence.
	case "propose_cash_change":
		return true
	case "operate_page_control", "execute_ledger_change", "read_page_content", "get_current_page", "get_navigation_targets", "open_page", "open_return_trend", "compare_financial_context", "compare_financial_attribution", "get_financial_context", "get_financial_context_item", "get_overview", "list_accounts", "get_account_snapshot", "get_read_page", "get_read_item":
		return true
	}
	// Provider progress sources are names from the controlled discovered registry.
	// Keep persistence bounded and reject arbitrary display text.
	if len(s) == 0 || len(s) > 96 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && c != '_' && !(c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Preserve financial values exactly; remove credential fields only. Details are
// bounded and never copied into the recoverable provider history.
func toolDisplay(raw string, key []byte) (string, bool) {
	if raw == "" {
		return "", false
	}
	if len(raw) > 64<<10 || !utf8.ValidString(raw) {
		return "[unavailable]", false
	}
	return clipTool(raw, key)
}
func clipTool(raw string, key []byte) (string, bool) {
	var clean func([]byte) []byte
	clean = func(value []byte) []byte {
		value = []byte(strings.TrimSpace(string(value)))
		var text string
		if jsoncodec.Unmarshal(value, &text) == nil && len(key) > 0 && strings.Contains(text, string(key)) {
			return []byte(`"[credential removed]"`)
		}
		var object map[string]json.RawMessage
		if len(value) > 0 && value[0] == '{' && jsoncodec.Unmarshal(value, &object) == nil {
			for name, field := range object {
				normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(name))
				switch normalized {
				case "apikey", "accesstoken", "refreshtoken", "token", "password", "secret", "authorization", "credential", "credentials":
					object[name] = json.RawMessage(`"[credential removed]"`)
				default:
					object[name] = clean(field)
				}
			}
			encoded, err := jsoncodec.Marshal(object)
			if err == nil {
				return encoded
			}
		}
		var rows []json.RawMessage
		if len(value) > 0 && value[0] == '[' && jsoncodec.Unmarshal(value, &rows) == nil {
			for i := range rows {
				rows[i] = clean(rows[i])
			}
			encoded, err := jsoncodec.Marshal(rows)
			if err == nil {
				return encoded
			}
		}
		return value
	}
	// RawMessage preserves exact numeric lexemes; never round financial values.
	raw = string(clean([]byte(strings.TrimSpace(raw))))
	if len(key) > 0 && strings.Contains(raw, string(key)) {
		return "[credential removed]", false
	}
	if len(raw) <= 8192 {
		return raw, false
	}
	end := 8192
	for !utf8.RuneStart(raw[end]) {
		end--
	}
	return raw[:end], true
}

func validTools(tools []ToolProgress) bool {
	if len(tools) > 16 {
		return false
	}
	total := 0
	for _, tool := range tools {
		if !progressSource(tool.Source) || (tool.State != "succeeded" && tool.State != "refused" && tool.State != "failed" && tool.State != "interrupted") || len(tool.Input) > 8192 || len(tool.Output) > 8192 || !utf8.ValidString(tool.Input) || !utf8.ValidString(tool.Output) {
			return false
		}
		total += len(tool.Input) + len(tool.Output)
	}
	return total <= 64<<10
}
