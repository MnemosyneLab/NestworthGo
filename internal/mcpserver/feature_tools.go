package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

// HistoricalOverviewInput is one tool with two mutually exclusive shapes.
// {date, compareTo} is one closed endpoint, optionally compared with another
// closed date or current. {from, to} is a 1-31 day trend. The result stays an
// mcp-read/2 envelope; series captures add household points and omit a nested
// full overview. Endpoint movement is not a return.
type HistoricalOverviewInput struct {
	HistoricalReadOptions
	Date      string `json:"date,omitempty" jsonschema:"Endpoint shape. Closed YYYY-MM-DD in the household history timezone; empty selects the GUI default month end. Forbidden with from/to."`
	CompareTo string `json:"compareTo,omitempty" jsonschema:"Endpoint shape. Optional second closed YYYY-MM-DD or current; empty returns one state. Forbidden with from/to."`
	From      string `json:"from,omitempty" jsonschema:"Series shape. First closed YYYY-MM-DD. Requires to and forbids date/compareTo."`
	To        string `json:"to,omitempty" jsonschema:"Series shape. Last closed YYYY-MM-DD. The range is 1 to 31 forward dates. Requires from."`
}

type HistoricalOverviewSeriesInput struct {
	HistoricalReadOptions
	From string
	To   string
}

// ledgerWriteTool follows the durable outer MCP receipt convention for local
// preference changes too. Unknown outcomes remain conservative because these
// application configuration writes do not share a transaction with the MCP
// receipt file.
func ledgerWriteTool[I any](s *Service, server *toolSet, name, description string, fn func(context.Context, I) (any, error)) {
	closed := false
	tool := &agentcapability.Definition{Name: name, Description: description, Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed}}
	tool.InputSchema = workflowSchema[Mutation[I]](name)
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
	readTool(server, "get_historical_overview", "Historical net-worth endpoints in the household history timezone. Input is one of two shapes. {date, compareTo}: one closed YYYY-MM-DD (empty date selects the GUI default month end) optionally compared with another closed date or current. {from, to}: a forward daily trend of 1-31 closed dates; do not combine the shapes. Compact leaf rows by default; view=evidence expands them. Filters select endpoint rows only. Household totals, the series trend and reconciliation stay full scope; matchedLeft/matchedRight count filtered leaves. A series returns compact household points and filtered leaves, without a nested full overview. Pending means one included leaf with a carried price or FX, never a rollup or notice. Page with get_read_page. get_read_item opens rows/<id> (adopted price and FX evidence), points/<date> (pageable missing evidence), overview/endpoints and states/left or states/right. Endpoint movement includes flows and is not a return; use analyze_period or compare_financial_attribution for returns.", func(ctx context.Context, in HistoricalOverviewInput) (any, error) {
		return s.pagedHistoricalRead(ctx, generation, in)
	})
}
