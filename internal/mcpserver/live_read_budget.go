package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
)

// Re-query the same live page with a smaller row bound so the authoritative
// reader constructs the cursor after exactly the delivered rows. Never trim a
// returned page and keep its old cursor. One application fence keeps candidate
// reads on the same source revision. No writes/provider calls occur here.
func fitLiveReadPage[T any](ctx context.Context, app *application.Service, limit int, read func(context.Context, int) (T, error)) (T, error) {
	var result T
	if limit < 1 || limit > 100 {
		return result, fail("validation", "page size must be 1 to 100")
	}
	err := app.WithWrite(ctx, func(ctx context.Context) error {
		for {
			value, err := read(ctx, limit)
			if err != nil {
				return err
			}
			fits, err := readFits(value)
			if err != nil {
				return err
			}
			if fits {
				result = value
				return nil
			}
			if limit == 1 {
				return fail("too_large", "single_item or required page header exceeds 64 KiB complete wire budget; smaller pages cannot recover this item")
			}
			limit = (limit + 1) / 2
		}
	})
	return result, err
}

func isReviewReadTool(name string) bool {
	switch name {
	case "list_investment_review_rules", "preview_investment_review_rule", "list_investment_review_alerts", "get_investment_review_alert", "list_investment_review_daily_evidence", "list_investment_review_episode_history", "list_investment_review_episode_events", "get_investment_review_coverage", "list_investment_review_suppressions":
		return true
	}
	return false
}
