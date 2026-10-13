package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type HistoricalOverviewInput struct {
	Date      string `json:"date,omitempty" jsonschema:"Closed YYYY-MM-DD date in the household history timezone; empty selects the GUI default month end"`
	CompareTo string `json:"compareTo,omitempty" jsonschema:"Optional second closed YYYY-MM-DD date or current; empty returns one state"`
}

type HistoricalOverviewSeriesInput struct {
	From string `json:"from" jsonschema:"First closed YYYY-MM-DD date in the household history timezone"`
	To   string `json:"to" jsonschema:"Last closed YYYY-MM-DD date in the household history timezone; range is 1 to 31 dates"`
}

type SaveInvestmentReviewPolicyInput struct {
	Policy           domain.InvestmentReviewPolicy `json:"policy"`
	ExpectedRevision int                           `json:"expectedRevision" jsonschema:"Revision returned by get_investment_reviews"`
}

type AcknowledgeInvestmentReviewInput struct {
	EpisodeID string `json:"episodeId" jsonschema:"Current episodeId returned by get_investment_reviews"`
}

type SetInvestmentReviewEnrollmentInput struct {
	InstrumentID     string `json:"instrumentId"`
	Enabled          bool   `json:"enabled" jsonschema:"true enables monitoring; false disables it"`
	ExpectedRevision int    `json:"expectedRevision" jsonschema:"Revision returned by get_investment_reviews"`
}

type SetInvestmentReviewSnoozeInput struct {
	InstrumentID     string `json:"instrumentId"`
	Days             int    `json:"days" jsonschema:"7 or 30 snoozes; 0 resumes immediately"`
	ExpectedRevision int    `json:"expectedRevision" jsonschema:"Revision returned by get_investment_reviews"`
}

// ledgerWriteTool follows the durable outer MCP receipt convention for local
// preference changes too. Unknown outcomes remain conservative because these
// application configuration writes do not share a transaction with the MCP
// receipt file.
func ledgerWriteTool[I any](s *Service, server *mcp.Server, name, description string, fn func(context.Context, I) (any, error)) {
	closed := false
	tool := &mcp.Tool{Name: name, Description: description + " Requires ledger_write. Read the current report first. Supply a new operationId for each intended change and reuse the exact same ID and input when retrying; an unknown outcome is never replayed automatically. Read get_operation and current review state before another intentional change.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}
	if schema := reviewBatchSchema[I](name); schema != nil {
		tool.InputSchema = schema
	}
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[I]) (*mcp.CallToolResult, Response, error) {
		value, err := s.execute(ctx, in.OperationID, name, in.Input, func(ctx context.Context) (any, error) { return fn(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func (s *Service) featureTools(server *mcp.Server, mode string) {
	generation := s.contextGeneration
	readTool(server, "get_investment_reviews", "Read opt-in investment review episodes and policy. responseMode=paged keeps report header/policy/revision and pages filtered globally sorted items; follow every cursor. Uses the same exact unit-NAV history, eligibility, freshness, household civil-date and stale-source safeguards as the App; this is not a sell recommendation and it never writes ledger facts.", func(ctx context.Context, in ReviewReadInput) (any, error) {
		if err := validateResponseMode(in.ResponseMode, in.Filter != nil || in.Sort != nil || in.Limit != 0); err != nil {
			return nil, err
		}
		if in.ResponseMode == "paged" {
			return s.pagedReviews(ctx, generation, in)
		}
		return s.app.InvestmentReviews(ctx)
	})
	readTool(server, "get_upcoming_actions", "Read the GUI's aggregated upcoming product, reservation, receipt, eligibility and data-check actions for the next 30 household-local civil dates. responseMode=paged independently pages filtered globally sorted items and dataChecks while retaining household header/counts. Follow every cursor. Returns the shared upcoming-actions/1 projection, exact reasons, source evidence, timezone, capture time and completeness; do not reconstruct it from product lists or treat estimates as posted facts.", func(ctx context.Context, in UpcomingReadInput) (any, error) {
		if err := validateResponseMode(in.ResponseMode, in.Filter != nil || in.Sort != nil || in.Limit != 0); err != nil {
			return nil, err
		}
		if in.ResponseMode == "paged" {
			return s.pagedUpcoming(ctx, generation, in)
		}
		return liquidity.NewService(s.app).UpcomingActions(ctx)
	})
	readTool(server, "get_historical_overview", "Read the same immutable historical overview as the App, including exact endpoint_bridge/1 reconciliation when comparing two endpoints. Additive displayValuation exposes full provisional estimates or labeled known subtotals independently of strict fields; estimatedChange includes flows and is not confirmed return. Closed civil dates use the household history timezone; preserve completeness, capturedAt, source timestamp basis, later-endpoint FX convention, reasons and nil unknowns. No snapshots are written, providers refreshed or ledger facts changed. This explains endpoint movement; it is not return attribution.", func(ctx context.Context, in HistoricalOverviewInput) (any, error) {
		return s.app.HistoricalOverview(ctx, in.Date, in.CompareTo)
	})
	readTool(server, "get_historical_overview_series", "Read the GUI's bounded historical overview and daily series from one reconciliation capture. Supply 1 to 31 forward closed YYYY-MM-DD civil dates in the household history timezone. The overview includes exact endpoint_bridge/1 reconciliation; points retain strict completeness and additive displayValuation quality, gaps, scope/basis changes and connect flags. Effective NAV dates differ from retrieval times; provisional estimates never imply confirmed returns. No snapshots are written, providers refreshed or ledger facts changed. This is not return attribution.", func(ctx context.Context, in HistoricalOverviewSeriesInput) (any, error) {
		return s.app.HistoricalOverviewSeries(ctx, in.From, in.To)
	})
	if mode != LedgerWrite {
		return
	}
	ledgerWriteTool(s, server, "save_investment_review_policy", "Save the full household investment-review policy with revision compare-and-swap. Policy thresholds are percentages; preserve the 7/30-day window and existing eligibility safeguards.", func(ctx context.Context, in SaveInvestmentReviewPolicyInput) (any, error) {
		return s.app.SaveInvestmentReviewPolicy(ctx, in.Policy, in.ExpectedRevision)
	})
	ledgerWriteTool(s, server, "acknowledge_investment_review", "Acknowledge one currently active episode using its current episodeId. Stale or no-longer-eligible episodes are rejected by the application service.", func(ctx context.Context, in AcknowledgeInvestmentReviewInput) (any, error) {
		return s.app.AcknowledgeInvestmentReview(ctx, in.EpisodeID)
	})
	ledgerWriteTool(s, server, "set_investment_review_enrollment", "Enable or disable per-instrument investment review enrollment. Requires the current report revision; disabling does not delete holdings or modify the policy.", func(ctx context.Context, in SetInvestmentReviewEnrollmentInput) (any, error) {
		return s.app.SetInvestmentReviewEnrollment(ctx, in.InstrumentID, in.Enabled, in.ExpectedRevision)
	})
	ledgerWriteTool(s, server, "set_investment_review_snooze", "Snooze one eligible instrument for 7 or 30 household civil days, or resume now with days=0. Requires the current report revision; this does not acknowledge an episode or alter enrollment.", func(ctx context.Context, in SetInvestmentReviewSnoozeInput) (any, error) {
		return s.app.SetInvestmentReviewSnooze(ctx, in.InstrumentID, in.Days, in.ExpectedRevision)
	})
}
