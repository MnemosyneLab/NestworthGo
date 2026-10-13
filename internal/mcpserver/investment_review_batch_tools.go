package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
)

func reviewReceiptBudget() error {
	receipt := application.ReviewBatchReceipt{PreviousRevision: 999999999, Revision: 999999999, RequestedCount: 100, ChangedCount: 100, Items: make([]application.ReviewBatchReceiptItem, 100)}
	for i := range receipt.Items {
		receipt.Items[i] = application.ReviewBatchReceiptItem{InstrumentID: strings.Repeat("a", 36), Changed: true}
	}
	op := Operation{ID: strings.Repeat("a", 36), Tool: strings.Repeat("a", 48), Hash: strings.Repeat("a", 64), Status: "succeeded", CreatedAt: "2026-10-13T12:00:00.123456789Z"}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	op.Result = raw
	if ok, err := readFits(op); err != nil {
		return err
	} else if !ok {
		return fail("too_large", "batch receipt exceeds wire budget")
	}
	return nil
}
func (s *Service) reviewBatchTools(server *mcp.Server, mode string) {
	if mode != LedgerWrite {
		return
	}
	ledgerWriteTool(s, server, "set_investment_review_enrollment_batch", "Atomically enable/disable 1 to 100 verified instrument review enrollments at one expectedRevision. All targets must remain eligible; one changed batch advances revision once, no-op preserves it. Short receipt; policy, snooze and acknowledgements are unchanged.", func(ctx context.Context, in application.ReviewEnrollmentBatchInput) (any, error) {
		if err := reviewReceiptBudget(); err != nil {
			return nil, err
		}
		return s.app.SetInvestmentReviewEnrollmentBatch(ctx, in)
	})
	ledgerWriteTool(s, server, "set_investment_review_snooze_batch", "Atomically snooze/resume 1 to 100 reviews at one expectedRevision. days is 0/7/30, using one household civil date. One changed batch advances revision once. Enrollment, policy and acknowledgements are unchanged.", func(ctx context.Context, in application.ReviewSnoozeBatchInput) (any, error) {
		if err := reviewReceiptBudget(); err != nil {
			return nil, err
		}
		return s.app.SetInvestmentReviewSnoozeBatch(ctx, in)
	})
	ledgerWriteTool(s, server, "acknowledge_investment_reviews_batch", "Atomically acknowledge 1 to 100 current instrumentId/episodeId pairs at one expectedRevision. Stale, snoozed or ineligible episodes reject the whole batch. One changed batch advances revision once; no-op preserves it. Short receipt; read paged reviews for fresh state.", func(ctx context.Context, in application.ReviewAcknowledgeBatchInput) (any, error) {
		if err := reviewReceiptBudget(); err != nil {
			return nil, err
		}
		return s.app.AcknowledgeInvestmentReviewsBatch(ctx, in)
	})
}
