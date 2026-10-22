package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
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
func ledgerWriteTool[I any](s *Service, server *mcp.Server, name, description string, fn func(context.Context, I) (any, error)) {
	closed := false
	tool := &mcp.Tool{Name: name, Description: description + " Requires ledger_write. Supply a new operationId for each intended change and reuse the exact same ID and input when retrying; an unknown outcome is never replayed automatically. Read get_operation and current data before another intended change.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[I]) (*mcp.CallToolResult, Response, error) {
		value, err := s.execute(ctx, in.OperationID, name, in.Input, func(ctx context.Context) (any, error) { return fn(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func (s *Service) featureTools(server *mcp.Server) {
	generation := s.contextGeneration
	readTool(server, "get_upcoming_actions", "Read the GUI's aggregated upcoming product, reservation, receipt, eligibility and data-check actions for the next 30 household-local civil dates. responseMode=paged independently pages filtered globally sorted items and dataChecks while retaining household header/counts. Follow every cursor. Returns the shared upcoming-actions/1 projection, exact reasons, source evidence, timezone, capture time and completeness; do not reconstruct it from product lists or treat estimates as posted facts.", func(ctx context.Context, in UpcomingReadInput) (any, error) {
		if err := validateResponseMode(in.ResponseMode, in.Filter != nil || in.Sort != nil || in.Limit != 0); err != nil {
			return nil, err
		}
		if in.ResponseMode == "paged" {
			return s.pagedUpcoming(ctx, generation, in)
		}
		return liquidity.NewService(s.app).UpcomingActions(ctx)
	})
	readTool(server, "get_historical_overview", "Capture historical-read/2 in frozen mcp-read/1 pages. Default compact leaves; view=evidence expands rows. Filter endpoint rows by account/instrument/kind/inclusion/effective quality/reason/side; household monetary totals remain full scope, matchedLeft/Right counts cover filtered leaves only. Pending is one included leaf with carried price OR FX, never account rollups or informational warnings. Dates are closed household civil dates; compareTo may be current. get_read_page continues sections; get_read_item rows/id returns full adopted price/FX evidence, overview/endpoints opens categories/contributions, states/left or right gives allocations. Initial default 5/section, continuation 25/max 100, byte-aware 64 KiB complete wire. No provider/snapshot/ledger writes. endpoint_bridge/1 explains movement, not return. ", func(ctx context.Context, in HistoricalOverviewInput) (any, error) {
		return s.pagedHistoricalOverview(ctx, generation, in)
	})
	readTool(server, "get_historical_overview_series", "Capture 1–31 forward closed YYYY-MM-DD household civil dates in historical-read/2 mcp-read/1 pages. Independent compact household points and filtered endpoint leaves, without a nested full Overview. Row filters do not change household trend/totals/reconciliation. Use get_read_page for rows/points; get_read_item points/date opens full pageable missing evidence, rows/id opens adopted quote evidence, overview/endpoints opens categories/contributions. Preserve quality, nulls, source dates, gap/scope/basis/connect flags. Default compact view, initial 5/section, continuation 25/max 100; byte-aware 64 KiB complete wire, separate unchanged computation limits. No provider/snapshot/ledger writes. Endpoint movement includes flows and is not confirmed return. ", func(ctx context.Context, in HistoricalOverviewSeriesInput) (any, error) {
		return s.pagedHistoricalSeries(ctx, generation, in)
	})
}
