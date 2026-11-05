package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

type HistoricalOverviewInput struct {
	HistoricalReadOptions
	Date      string `json:"date,omitempty" jsonschema:"Closed YYYY-MM-DD date in the household history timezone; empty selects the GUI default month end"`
	CompareTo string `json:"compareTo,omitempty" jsonschema:"Optional second closed YYYY-MM-DD date or current; empty returns one state"`
}

type HistoricalOverviewSeriesInput struct {
	HistoricalReadOptions
	From string `json:"from" jsonschema:"First closed YYYY-MM-DD date in the household history timezone"`
	To   string `json:"to" jsonschema:"Last closed YYYY-MM-DD date in the household history timezone; range is 1 to 31 dates"`
}

// ledgerWriteTool follows the durable outer MCP receipt convention for local
// preference changes too. Unknown outcomes remain conservative because these
// application configuration writes do not share a transaction with the MCP
// receipt file.
func ledgerWriteTool[I any](s *Service, server *toolSet, name, description string, fn func(context.Context, I) (any, error)) {
	closed := false
	tool := &agentcapability.Definition{Name: name, Description: description, Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed}}
	registerTool(server, tool, func(ctx context.Context, in Mutation[I]) (Response, error) {
		value, err := s.execute(ctx, in.OperationID, name, in.Input, func(ctx context.Context) (any, error) { return fn(ctx, in.Input) })
		if err != nil {
			return Response{}, safeError(err)
		}
		return Response{Data: value}, nil
	})
}

func (s *Service) featureTools(server *toolSet) {
	generation := s.contextGeneration
	readTool(server, "get_upcoming_actions", "The GUI's upcoming product, reservation, receipt, eligibility and data-check actions for the next 30 household-local days (upcoming-actions/1), with exact reasons, source evidence, timezone, capture time and completeness. items and dataChecks page independently; follow cursors. Do not rebuild it from product lists or treat estimates as posted facts.", func(ctx context.Context, in UpcomingReadInput) (any, error) {
		return s.pagedUpcoming(ctx, generation, in)
	})
	readTool(server, "get_historical_overview", "Historical net-worth endpoint view for one closed date, optionally compared with another closed date or current, in the household history timezone. Compact leaf rows by default; view=evidence expands them. Filter by account, instrument, kind, inclusion, quality, reason and side: household totals keep full scope while matchedLeft/matchedRight count only filtered leaves. Pending means one included leaf with a carried price or FX, never a rollup or informational notice. Page with get_read_page; get_read_item opens rows/<id> (adopted price and FX evidence), overview/endpoints (categories, contributions) and states/left or states/right (allocations). Endpoint movement is not a return; use analyze_period or compare_financial_attribution for returns.", func(ctx context.Context, in HistoricalOverviewInput) (any, error) {
		return s.pagedHistoricalOverview(ctx, generation, in)
	})
	readTool(server, "get_historical_overview_series", "Daily trend over 1-31 forward closed YYYY-MM-DD dates: compact household points and filtered endpoint leaves, without a nested full overview. Row filters never change the household trend, totals or reconciliation. Page with get_read_page; get_read_item opens points/<date> (pageable missing evidence), rows/<id>, overview/endpoints and reconciliationDetails/<id>. Keep quality, nulls, source dates and gap/scope/basis flags. Endpoint movement includes flows and is not a confirmed return.", func(ctx context.Context, in HistoricalOverviewSeriesInput) (any, error) {
		return s.pagedHistoricalSeries(ctx, generation, in)
	})
}
