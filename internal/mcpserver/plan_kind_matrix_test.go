package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Every commit tool verifies the stored plan's kind. A plan committed through
// any other commit tool is rejected before execution and stays usable by its
// own commit tool; nothing is written by the rejected attempts.
var commitTools = []string{
	"commit_change", "commit_batch", "commit_reconciliation", "commit_correction",
	"commit_product_operation", "commit_product_terms", "commit_product_valuation",
	"commit_reconciliation_session", "commit_save_liquidity_policy", "commit_reset_liquidity_policy",
}

func TestCommitToolsRejectPlansOfAnotherKind(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	values := map[string]string{"accountId": fx.checking, "effectiveAt": "2026-09-29T12:00:00Z"}
	income := skillExample(t, "cash-and-debt", "income", values)
	plans := map[string]string{
		"commit_change":         ledgerPreview(t, c, income)["planId"].(string),
		"commit_batch":          batchPreview(t, c, income, income)["planId"].(string),
		"commit_reconciliation": call(t, c, "preview_reconciliation", skillExample(t, "reconciliation-and-corrections", "balance-target", values), false)["data"].(map[string]any)["planId"].(string),
	}
	before := ledgerActivityCount(t, fx.app)
	for owner, planID := range plans {
		for _, tool := range commitTools {
			if tool == owner {
				continue
			}
			message := commitError(t, c, tool, planID)
			if !strings.Contains(message, "matching this preview plan") {
				t.Errorf("%s accepted or mis-rejected a %s plan: %s", tool, owner, message)
			}
		}
	}
	if got := ledgerActivityCount(t, fx.app); got != before {
		t.Fatalf("rejected commits wrote activities: %d -> %d", before, got)
	}
	// Rejected attempts leave every stored plan intact for its own kind. (Plans
	// are version-bound, so committing one makes the others stale; the files are
	// checked directly and one matching commit is exercised.)
	kinds := map[string]string{"commit_change": "change", "commit_batch": "batch", "commit_reconciliation": "reconciliation"}
	for owner, planID := range plans {
		if _, err := fx.service.loadPreviewPlan(planID, kinds[owner]); err != nil {
			t.Fatalf("%s plan was damaged by rejected commits: %v", owner, err)
		}
	}
	call(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": plans["commit_change"]}}, false)
	if ledgerActivityCount(t, fx.app) == before {
		t.Fatal("matching commit recorded nothing")
	}
}

func commitError(t *testing.T, c *mcp.ClientSession, tool, planID string) string {
	t.Helper()
	res, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}})
	if err != nil {
		t.Fatal(tool, err)
	}
	if !res.IsError {
		t.Fatalf("%s accepted a plan of another kind", tool)
	}
	raw, _ := jsoncodec.Marshal(res.Content)
	return string(raw)
}
