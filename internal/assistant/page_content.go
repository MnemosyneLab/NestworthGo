package assistant

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// PageNode is a presentation fact, never HTML, a route or UI execution authority.
// Module/control IDs are stable publisher paths; anonymous text IDs are snapshot scoped.
type PageNode struct {
	Effect    string   `json:"effect,omitempty"`
	ID        string   `json:"id"`
	TargetRef string   `json:"targetRef,omitempty"`
	Parent    string   `json:"parent,omitempty"`
	Module    string   `json:"module"`
	Role      string   `json:"role"`
	Name      string   `json:"name,omitempty"`
	Value     string   `json:"value,omitempty"`
	State     []string `json:"state,omitempty"`
	Actions   []string `json:"actions,omitempty"`
}
type PageContent struct {
	ID          string     `json:"id"`
	Page        string     `json:"page"`
	CapturedAt  time.Time  `json:"capturedAt"`
	Nodes       []PageNode `json:"nodes"`
	Limitations []string   `json:"limitations"`
}
type PageReadRequest struct {
	ID string `json:"id"`
}
type pendingPageRead struct {
	revision uint64
	response chan PageContent
}
type PageContentReader func(context.Context, json.RawMessage) (json.RawMessage, error)
type pageContentKey struct{}

func CurrentPageReader(ctx context.Context) PageContentReader {
	r, _ := ctx.Value(pageContentKey{}).(PageContentReader)
	return r
}

var businessPageName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func pageText(s string) bool { return len(s) <= 2048 && !strings.ContainsAny(s, "\x00") }
func (p PageContent) valid(now time.Time) bool {
	if !canonicalID(p.ID) || p.CapturedAt.After(now.Add(time.Second)) || now.Sub(p.CapturedAt) >= 10*time.Second || len(p.Nodes) > 8192 || len(p.Limitations) > 16 {
		return false
	}
	switch p.Page {
	case "unsupported":
		if len(p.Nodes) != 0 || len(p.Limitations) == 0 {
			return false
		}
	default:
		if !businessPageName.MatchString(p.Page) || len(p.Nodes) == 0 {
			return false
		}
	}
	ids := map[string]bool{}
	targets := map[string]bool{}
	roles := "|region|text|heading|button|link|textbox|combobox|checkbox|radio|switch|tab|tablist|option|table|row|cell|columnheader|status|alert|group|img|disclosure|slider|spinbutton|article|form|navigation|list|listitem|term|definition|tabpanel|radiogroup|dialog|alertdialog|menu|menuitem|menuitemcheckbox|menuitemradio|tooltip|progressbar|separator|tree|treeitem|grid|rowheader|listbox|"
	for _, n := range p.Nodes {
		if (n.Effect != "" && n.Effect != "presentation") || (n.TargetRef != "" && (!canonicalID(n.TargetRef) || targets[n.TargetRef])) || n.ID == "" || len(n.ID) > 240 || len(n.Module) > 120 || n.Module == "" || ids[n.ID] || (n.Parent != "" && !ids[n.Parent]) || !strings.Contains(roles, "|"+n.Role+"|") || !pageText(n.Name) || !pageText(n.Value) || len(n.State) > 12 || len(n.Actions) > 8 {
			return false
		}
		ids[n.ID] = true
		if n.TargetRef != "" {
			targets[n.TargetRef] = true
		}
		for _, v := range append(append([]string{}, n.State...), n.Actions...) {
			if !pageText(v) {
				return false
			}
		}
	}
	for _, v := range p.Limitations {
		if !pageText(v) {
			return false
		}
	}
	raw, err := jsoncodec.Marshal(p)
	return err == nil && len(raw) <= 512<<10
}

// PendingPageReads is desktop IPC only. A tool creates a request; the mounted
// frontend captures its current supported page on demand, never by user refresh.
func (r *Runtime) PendingPageReads(ctx context.Context, epoch string) ([]PageReadRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return nil, err
	}
	out := []PageReadRequest{}
	for id := range r.pageReads {
		out = append(out, PageReadRequest{ID: id})
	}
	return out, nil
}
func (r *Runtime) DeliverPageContent(ctx context.Context, epoch, requestID string, content PageContent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.pageReads[requestID]
	if r.valid(ctx, epoch) != nil || !ok || pending.revision != r.state.Revision || !r.state.Config.Enabled || !content.valid(r.navigationNow()) {
		return ErrStale
	}
	raw, err := jsoncodec.Marshal(content)
	if err != nil {
		return ErrUnavailable
	}
	if r.options.Credentials.With(ctx, func(key []byte) error {
		if len(key) > 0 && (strings.Contains(string(raw), string(key)) || contentContains(content, string(key))) {
			return ErrUnavailable
		}
		return nil
	}) != nil {
		return ErrUnavailable
	}
	var copy PageContent
	if jsoncodec.Unmarshal(raw, &copy) != nil {
		return ErrUnavailable
	}
	delete(r.pageReads, requestID)
	pending.response <- copy
	return nil
}
func (r *Runtime) capturePage(ctx context.Context, revision uint64) (PageContent, error) {
	r.mu.Lock()
	if r.closed || r.state.Revision != revision || !r.state.Config.Enabled || len(r.pageReads) >= 2 {
		r.mu.Unlock()
		return PageContent{}, ErrUnavailable
	}
	if r.pageReads == nil {
		r.pageReads = map[string]pendingPageRead{}
	}
	id := uuid.NewString()
	response := make(chan PageContent, 1)
	r.pageReads[id] = pendingPageRead{revision, response}
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pageReads, id); r.mu.Unlock() }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case p := <-response:
		r.mu.Lock()
		valid := r.valid(ctx, r.epoch) == nil && ctx.Err() == nil && r.state.Revision == revision && r.state.Config.Enabled
		r.mu.Unlock()
		if !valid {
			return PageContent{}, ErrStale
		}
		return p, nil
	case <-ctx.Done():
		return PageContent{}, ErrStale
	case <-timer.C:
		return PageContent{}, ErrUnavailable
	}
}
func (r *Runtime) pageReader(runCtx context.Context, revision uint64) PageContentReader {
	// One reader per run, so a model cannot import another run's snapshot/cursor.
	var frozen *PageContent
	var readerMu sync.Mutex
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		readerMu.Lock()
		defer readerMu.Unlock()
		if runCtx.Err() != nil {
			return nil, ErrStale
		}
		var q struct {
			SnapshotID   string `json:"snapshotId"`
			ModuleCursor int    `json:"moduleCursor"`
			Module       string `json:"module"`
			Cursor       int    `json:"cursor"`
			Limit        int    `json:"limit"`
		}
		var fields map[string]json.RawMessage
		if len(args) > 2048 || jsoncodec.Unmarshal(args, &fields) != nil || fields == nil {
			return nil, ErrUnavailable
		}
		for k := range fields {
			if k != "snapshotId" && k != "module" && k != "cursor" && k != "limit" && k != "moduleCursor" {
				return nil, ErrUnavailable
			}
		}
		if jsoncodec.Unmarshal(args, &q) != nil || q.Cursor < 0 || q.ModuleCursor < 0 || q.Limit < 0 || q.Limit > 64 || ((q.Cursor > 0 || q.ModuleCursor > 0) && q.SnapshotID == "") {
			return nil, ErrUnavailable
		}
		current, err := r.capturePage(ctx, revision)
		if err != nil {
			return nil, err
		}
		if runCtx.Err() != nil {
			return nil, ErrStale
		}
		if q.SnapshotID != "" {
			if frozen == nil || frozen.ID != q.SnapshotID || current.ID != q.SnapshotID {
				return nil, ErrStale
			}
		} else {
			frozen = &current
		}
		p := frozen
		nodes := []PageNode{}
		modules := []string{}
		seen := map[string]bool{}
		selected := map[string]bool{}
		for _, n := range p.Nodes {
			if !seen[n.Module] {
				modules = append(modules, n.Module)
				seen[n.Module] = true
			}
			if q.Module == "" || q.Module == n.Module || selected[n.Parent] {
				selected[n.ID] = true
				nodes = append(nodes, n)
			}
		}
		if q.Module != "" && !seen[q.Module] || q.Cursor > len(nodes) {
			return nil, ErrUnavailable
		}
		if q.ModuleCursor > len(modules) {
			return nil, ErrUnavailable
		}
		moduleEnd := min(q.ModuleCursor+64, len(modules))
		truncated := false
		for _, n := range nodes {
			for _, state := range n.State {
				if state == "value_truncated=true" {
					truncated = true
				}
			}
		}
		limit := q.Limit
		if limit == 0 {
			limit = 32
		}
		end := min(q.Cursor+limit, len(nodes))
		for {
			result := map[string]any{"snapshotId": p.ID, "page": p.Page, "capturedAt": p.CapturedAt, "modules": modules[q.ModuleCursor:moduleEnd], "moduleCount": len(modules), "moduleOffset": q.ModuleCursor, "hasMoreModules": moduleEnd < len(modules), "modulesComplete": q.ModuleCursor == 0 && moduleEnd == len(modules), "module": q.Module, "nodes": nodes[q.Cursor:end], "totalNodes": len(nodes), "hasMore": end < len(nodes), "complete": q.Cursor == 0 && end == len(nodes) && len(p.Limitations) == 0 && !truncated, "offset": q.Cursor, "limitations": p.Limitations, "interactionPolicy": "targets_are_not_authority; presentation_allowed_by_default; other_controls_require_Full; live_claim_required; dispatch_is_not_financial_success"}
			if moduleEnd < len(modules) {
				result["nextModuleCursor"] = moduleEnd
			}
			if end < len(nodes) {
				result["nextCursor"] = end
			}
			raw, e := jsoncodec.Marshal(result)
			if e != nil {
				return nil, ErrUnavailable
			}
			if len(raw) <= 24<<10 {
				return raw, nil
			}
			end--
			if end <= q.Cursor {
				return nil, BudgetFailure("tool_result_bytes")
			}
		}
	}
}

func contentContains(p PageContent, secret string) bool {
	for _, n := range p.Nodes {
		for _, v := range append(append([]string{n.ID, n.Parent, n.Module, n.Role, n.Name, n.Value}, n.State...), n.Actions...) {
			if strings.Contains(v, secret) {
				return true
			}
		}
	}
	for _, v := range p.Limitations {
		if strings.Contains(v, secret) {
			return true
		}
	}
	return false
}
