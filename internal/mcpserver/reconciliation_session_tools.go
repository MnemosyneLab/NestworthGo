package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReconciliationReadInput struct {
	SessionID string `json:"sessionId,omitempty"`
	Section   string `json:"section,omitempty" jsonschema:"accounts, items, links, or attempts; attempts requires accountId and itemId"`
	AccountID string `json:"accountId,omitempty"`
	ItemID    string `json:"itemId,omitempty"`
	Status    string `json:"status,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Sort      string `json:"sort,omitempty" jsonschema:"asc or desc; default desc for sessions, asc for detail"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Default 25; maximum 50"`
}
type reconciliationPage struct {
	Items      []any  `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
	Revision   int64  `json:"revision,omitempty"`
}
type reconciliationCursor struct {
	Hash   string `json:"hash"`
	Offset int    `json:"offset"`
}

func pageReconciliation(in ReconciliationReadInput, items []any, revision int64) (any, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 25
	}
	if limit < 1 || limit > 50 {
		return nil, fail("validation", "limit must be 1 to 50")
	}
	query := in
	query.Cursor = ""
	query.Limit = 0
	raw, _ := json.Marshal(struct {
		Query    ReconciliationReadInput
		Items    []any
		Revision int64
	}{query, items, revision})
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	offset := 0
	if len(in.Cursor) > 1024 {
		return nil, fail("validation", "invalid reconciliation cursor")
	}
	if in.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		if err != nil || len(raw) > 512 {
			return nil, fail("validation", "invalid reconciliation cursor")
		}
		var c reconciliationCursor
		if json.Unmarshal(raw, &c) != nil || c.Hash != hash {
			return nil, fail("stale_cursor", "reconciliation changed; restart paging")
		}
		offset = c.Offset
	}
	if offset < 0 || offset > len(items) {
		return nil, fail("validation", "invalid reconciliation cursor offset")
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	result := reconciliationPage{Items: items[offset:end], Total: len(items), Revision: revision}
	if end < len(items) {
		raw, _ := json.Marshal(reconciliationCursor{Hash: hash, Offset: end})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	raw, _ = json.Marshal(result)
	if len(raw) > 96*1024 {
		return nil, fail("response_too_large", "reduce limit; no evidence has been truncated")
	}
	return result, nil
}
func (s *Service) reconciliationSessionTools(server *mcp.Server, mode string) {
	readTool(server, "list_reconciliation_sessions", "Bounded reconciliation history and in-progress sessions. Filter status, accountId and started civil date from/to; sort asc/desc. Default 25, max 50, stable content-bound cursor with stale_cursor on change.", s.listReconciliationSessions)
	readTool(server, "get_reconciliation_session", "Read a reconciliation header and explicit outcome counts. No account/item histories are embedded. Use get_reconciliation_session_page and get_reconciliation_item for complete evidence; completed records remain immutable and recheck status is separate.", func(ctx context.Context, in IDInput) (any, error) {
		v, e := s.app.GetReconciliationSession(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		v.Accounts = nil
		return v, nil
	})
	readTool(server, "get_reconciliation_session_page", "Read bounded accounts, items, changes since prior successful check, links or attempts. Changes use recorded account revision order, include backdated entries, and show both recorded and effective times. First sessions have no prior watermark. Supply sessionId and section. Filter accountId and item outcome status. Attempts requires accountId/itemId. Item rows omit full detail and attempt arrays: get_reconciliation_item returns full single-point evidence and attempt count; page attempts separately. Links retain exact reviewed before and committed after evidence. Cursors are bound to current evidence and session revision.", s.reconciliationSessionPage)
	readTool(server, "get_reconciliation_item", "Read one complete saved/working check point including product terms, exact values, source precision and reviewed financial revision. Attempts are separate pageable evidence with attemptCount, never silently truncated.", func(ctx context.Context, in ReconciliationReadInput) (any, error) {
		v, e := s.app.GetReconciliationSession(ctx, in.SessionID)
		if e != nil {
			return nil, e
		}
		for _, a := range v.Accounts {
			if a.AccountID != in.AccountID {
				continue
			}
			for _, item := range a.Items {
				if item.ID == in.ItemID {
					count := len(item.Attempts)
					item.Attempts = nil
					return map[string]any{"item": item, "attemptCount": count, "sessionRevision": v.Revision}, nil
				}
			}
		}
		return nil, fail("not_found", "reconciliation item not found")
	})
	if mode != LedgerWrite {
		return
	}
	readTool(server, "preview_reconciliation_session", "Preview start, pause, resume, check or finish. Start selects active accountIds (omitted means all, max 100). All other actions require sessionId/expectedRevision. check requires accountId/itemId/current financialRevision and explicit outcome confirmed/skipped/unresolved/pending. Confirmed requires actual user-supplied source.reference and source.asOfDate; optional HH:mm uses the ledger timezone, date-only remains date-only. Never fabricate an attestation. Skip/unresolved require a note. Partial finish requires allowPartial and explanation. A correction never confirms an item. Inspect returned counts before commit.", func(ctx context.Context, in application.ReconciliationAction) (any, error) {
		p, e := s.app.PreviewReconciliationSession(ctx, in)
		if e != nil {
			return nil, e
		}
		if e = s.writePrivate(filepath.Join(s.dir, "plans", p.ID+".json"), struct {
			ID   string                                `json:"id"`
			Type string                                `json:"type"`
			Plan application.ReconciliationSessionPlan `json:"plan"`
		}{p.ID, "reconciliation_session", p}); e != nil {
			return nil, e
		}
		var checked *domain.ReconciliationItem
		if in.Action == "check" {
			for _, a := range p.Result.Accounts {
				if a.AccountID == in.AccountID {
					for _, item := range a.Items {
						if item.ID == in.ItemID {
							copy := item
							copy.Attempts = nil
							checked = &copy
						}
					}
				}
			}
		}
		result := p.Result
		for i := range result.Accounts {
			result.Accounts[i].Items = nil
		}
		return map[string]any{"planId": p.ID, "expiresAt": p.ExpiresAt, "result": result, "action": p.Action, "checkedItem": checked}, nil
	})
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_reconciliation_session", Description: "Commit exactly the saved session preview. Requires ledger_write. Retry same operationId and planId after an unknown outcome. Durable session receipt is checked before expiry, completed-state and revision checks. Returned header is the original committed outcome; reread current evidence separately.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		v, e := s.executeWithRecovery(ctx, in.OperationID, "commit_reconciliation_session", in.Input, true, func(ctx context.Context) (any, error) {
			raw, e := s.loadPreviewPlan(in.Input.PlanID, "reconciliation_session")
			if e != nil {
				return nil, e
			}
			var saved struct {
				Plan application.ReconciliationSessionPlan `json:"plan"`
			}
			if e = json.Unmarshal(raw, &saved); e != nil {
				return nil, e
			}
			result, e := s.app.CommitReconciliationSession(ctx, saved.Plan)
			result.Accounts = nil
			return result, e
		})
		if e != nil {
			return nil, Response{}, safeError(e)
		}
		return nil, Response{Data: v}, nil
	})
}
func (s *Service) listReconciliationSessions(ctx context.Context, in ReconciliationReadInput) (any, error) {
	for _, date := range []string{in.From, in.To} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return nil, fail("validation", "date filters must be YYYY-MM-DD")
			}
		}
	}
	if in.From != "" && in.To != "" && in.From > in.To {
		return nil, fail("validation", "date filters must be ordered")
	}
	if in.Sort != "" && in.Sort != "asc" && in.Sort != "desc" {
		return nil, fail("validation", "sort must be asc or desc")
	}
	if in.Status != "" && in.Status != "in_progress" && in.Status != "paused" && in.Status != "completed" {
		return nil, fail("validation", "invalid session status")
	}
	values, e := s.app.ListReconciliationSessions(ctx)
	if e != nil {
		return nil, e
	}
	rows := []any{}
	for _, v := range values {
		if in.Status != "" && v.Status != in.Status || in.From != "" && v.StartedAt[:10] < in.From || in.To != "" && v.StartedAt[:10] > in.To {
			continue
		}
		if in.AccountID != "" {
			full, e := s.app.GetReconciliationSession(ctx, v.ID)
			if e != nil {
				return nil, e
			}
			found := false
			for _, a := range full.Accounts {
				if a.AccountID == in.AccountID {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		rows = append(rows, v)
	}
	if in.Sort == "asc" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	return pageReconciliation(in, rows, 0)
}
func (s *Service) reconciliationSessionPage(ctx context.Context, in ReconciliationReadInput) (any, error) {
	if in.Sort != "" && in.Sort != "asc" && in.Sort != "desc" {
		return nil, fail("validation", "sort must be asc or desc")
	}
	v, e := s.app.GetReconciliationSession(ctx, in.SessionID)
	if e != nil {
		return nil, e
	}
	rows := []any{}
	switch in.Section {
	case "accounts":
		for _, a := range v.Accounts {
			if in.AccountID != "" && a.AccountID != in.AccountID {
				continue
			}
			a.Items = nil
			rows = append(rows, a)
		}
	case "items", "attempts":
		for _, a := range v.Accounts {
			if in.AccountID != "" && a.AccountID != in.AccountID {
				continue
			}
			for _, item := range a.Items {
				if in.ItemID != "" && item.ID != in.ItemID || in.Status != "" && item.Outcome != in.Status {
					continue
				}
				if in.Section == "attempts" {
					if in.AccountID == "" || in.ItemID == "" {
						return nil, fail("validation", "attempts requires accountId and itemId")
					}
					for _, attempt := range item.Attempts {
						rows = append(rows, attempt)
					}
				} else {
					item.Detail = nil
					count := len(item.Attempts)
					item.Attempts = nil
					rows = append(rows, map[string]any{"item": item, "attemptCount": count})
				}
			}
		}
	case "changes":
		changes, e := s.app.ReconciliationChanges(ctx, in.SessionID, in.AccountID)
		if e != nil {
			return nil, e
		}
		for _, change := range changes {
			rows = append(rows, change)
		}
	case "links":
		links, e := s.app.ReconciliationLinkedChanges(ctx, in.SessionID)
		if e != nil {
			return nil, e
		}
		for _, link := range links {
			if in.AccountID != "" && link.AccountID != in.AccountID || in.ItemID != "" && link.ItemID != in.ItemID {
				continue
			}
			rows = append(rows, link)
		}
	default:
		return nil, fail("validation", "section must be accounts, items, attempts, changes or links")
	}
	// Repository and account/item identifiers define a deterministic order.
	if in.Section == "accounts" {
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.Compare(rows[i].(domain.ReconciliationAccount).AccountID, rows[j].(domain.ReconciliationAccount).AccountID) < 0
		})
	}
	if in.Sort == "desc" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	return pageReconciliation(in, rows, v.Revision)
}
