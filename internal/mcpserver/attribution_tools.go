package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type ListContributionsInput struct {
	Query      PeriodAnalysisQuery `json:"query" jsonschema:"Inclusive closed-day YYYY-MM-DD range in the History Origin timezone, with optional scope and filters"`
	ReturnType string              `json:"returnType" jsonschema:"One of total_return, realized, unrealized, dividend_interest; unrealized currently reports unavailable"`
	GroupBy    string              `json:"groupBy" jsonschema:"One of instrument, account, currency, asset_class"`
	Ordering   string              `json:"ordering,omitempty" jsonschema:"One of amount_desc, amount_asc, rate_desc, rate_asc, name_asc; defaults to amount_desc"`
	Offset     int                 `json:"offset,omitempty" jsonschema:"Zero-based row offset; defaults to 0"`
	Limit      int                 `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; defaults to 100"`
}

type ContributionPageDTO struct {
	wire.ContributionDTO
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	TotalRows  int  `json:"totalRows"`
	HasMore    bool `json:"hasMore"`
	NextOffset *int `json:"nextOffset"`
}

type GetContributionItemInput struct {
	Query      PeriodAnalysisQuery `json:"query" jsonschema:"Use the same query as list_contributions"`
	ReturnType string              `json:"returnType" jsonschema:"Use the same returnType as list_contributions"`
	GroupBy    string              `json:"groupBy" jsonschema:"Use the same groupBy as list_contributions"`
	GroupKey   string              `json:"groupKey" jsonschema:"Exact key from a list_contributions row"`
}

type GetReturnDayInput struct {
	Query PeriodAnalysisQuery `json:"query" jsonschema:"Inclusive closed-day range in the History Origin timezone"`
	Date  string              `json:"date" jsonschema:"YYYY-MM-DD date within query.from and query.to"`
}

type GetAssetDriverDetailInput struct {
	Query     PeriodAnalysisQuery `json:"query" jsonschema:"Use the same query as analyze_period"`
	DriverKey string              `json:"driverKey" jsonschema:"One assetChange row bucket: external_flow, income, spending, dividend_interest, price_change, fx_impact, fx_conversion_spread, fee, liability_impact, adjustment, residual"`
}

// The regular readTool infers descriptions but has no enum metadata. These
// tools publish the application adapters' accepted values as schema enums.
func readAttributionTool[I any](server *mcp.Server, name, description string, enums map[string][]any, fn func(context.Context, I) (any, error)) {
	schema, err := jsonschema.For[I](nil)
	if err != nil {
		panic(fmt.Sprintf("%s input schema: %v", name, err))
	}
	for field, values := range enums {
		property := schema.Properties[field]
		if property == nil {
			panic(fmt.Sprintf("%s missing schema property %s", name, field))
		}
		property.Enum = values
	}
	if query := schema.Properties["query"]; query != nil {
		for field, values := range map[string][]any{
			"scopeKind": {"household", "account", "currency", "instrument", "asset_class"},
			"valuation": {"base", "native"},
			"basis":     {"investment"},
		} {
			if property := query.Properties[field]; property != nil {
				property.Enum = values
			}
		}
	}
	closed, destructive := false, false
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description + " May materialize derived snapshots and invalidate ledger previews; no financial facts or provider data are written. Available in existing read_only mode.", InputSchema: schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in I) (*mcp.CallToolResult, Response, error) {
		value, err := fn(ctx, in)
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func (s *Service) attributionTools(server *mcp.Server, adapter *analysis.Service) {
	readAttributionTool(server, "list_contributions", "Page investment return contributions by instrument, account, currency or asset class. The returned rows keep nullable amounts/rates, available, status, missingReason and ratedDays/totalDays. Use a row key with get_contribution_item; repeat the same query, returnType and groupBy. Dates are inclusive closed days in the History Origin timezone.", map[string][]any{
		"returnType": {"total_return", "realized", "unrealized", "dividend_interest"},
		"groupBy":    {"instrument", "account", "currency", "asset_class"},
		"ordering":   {"amount_desc", "amount_asc", "rate_desc", "rate_asc", "name_asc"},
	}, func(ctx context.Context, in ListContributionsInput) (any, error) {
		if in.Offset < 0 || in.Limit < 0 || in.Limit > 100 {
			return nil, fail("validation", "offset must be non-negative and limit must be between 1 and 100, or omitted")
		}
		limit := in.Limit
		if limit == 0 {
			limit = 100
		}
		ordering := strings.TrimSpace(in.Ordering)
		if ordering == "" {
			ordering = "amount_desc"
		}
		var value wire.ContributionDTO
		err := s.app.WithWrite(ctx, func(ctx context.Context) error {
			var err error
			value, err = adapter.Contribution(ctx, in.Query.request(), in.ReturnType, in.GroupBy, ordering)
			return err
		})
		if err != nil {
			return nil, err
		}
		page := ContributionPageDTO{ContributionDTO: value, Offset: in.Offset, Limit: limit, TotalRows: len(value.Rows)}
		start := min(in.Offset, len(value.Rows))
		end := start + min(limit, len(value.Rows)-start)
		page.Rows = append([]wire.ContributionRowDTO{}, value.Rows[start:end]...)
		page.HasMore = end < len(value.Rows)
		if page.HasMore {
			page.NextOffset = &end
		}
		return page, nil
	})
	readAttributionTool(server, "get_contribution_item", "Read the full components, account breakdown and History activity hint for one contribution row. Use an exact key from list_contributions with the same query, returnType and groupBy; the hint identifies related records and is not a second calculation of the amount.", map[string][]any{
		"returnType": {"total_return", "realized", "unrealized", "dividend_interest"},
		"groupBy":    {"instrument", "account", "currency", "asset_class"},
	}, func(ctx context.Context, in GetContributionItemInput) (any, error) {
		if strings.TrimSpace(in.GroupKey) == "" {
			return nil, fail("validation", "groupKey must be a non-empty list_contributions row key")
		}
		var value wire.ContributionItemDTO
		err := s.app.WithWrite(ctx, func(ctx context.Context) error {
			var err error
			value, err = adapter.ContributionItem(ctx, in.Query.request(), in.ReturnType, in.GroupBy, in.GroupKey)
			return err
		})
		return value, err
	})
	readAttributionTool(server, "get_return_day", "Read one day's return amount, rate, composition, contributors, coverage and valuation issues. The date must be within the query's inclusive closed-day range. Preserve nullable values and amountStatus; partial amounts are not complete totals.", nil, func(ctx context.Context, in GetReturnDayInput) (any, error) {
		query, err := in.Query.request().ToDomain()
		if err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02", in.Date)
		if err != nil || date.Format("2006-01-02") != in.Date || in.Date < string(query.From) || in.Date > string(query.To) {
			return nil, fail("validation", "date must be YYYY-MM-DD within query.from and query.to")
		}
		var value wire.ReturnDayDTO
		err = s.app.WithWrite(ctx, func(ctx context.Context) error {
			var err error
			value, err = adapter.ReturnDay(ctx, in.Query.request(), in.Date)
			return err
		})
		return value, err
	})
	readAttributionTool(server, "get_asset_driver_detail", "Read the instrument and account amounts plus residual source records for one asset-change driver bucket. Use a bucket key from analyze_period.assetChange. Dates are inclusive closed days in the History Origin timezone; preserve available, status, missingReason and nullable amounts.", map[string][]any{
		"driverKey": {"external_flow", "income", "spending", "dividend_interest", "price_change", "fx_impact", "fx_conversion_spread", "fee", "liability_impact", "adjustment", "residual"},
	}, func(ctx context.Context, in GetAssetDriverDetailInput) (any, error) {
		if strings.TrimSpace(in.DriverKey) == "" {
			return nil, fail("validation", "driverKey must be a non-empty asset-change bucket")
		}
		var value wire.AssetDriverDetailDTO
		err := s.app.WithWrite(ctx, func(ctx context.Context) error {
			var err error
			value, err = adapter.AssetDriverDetail(ctx, in.Query.request(), in.DriverKey)
			return err
		})
		return value, err
	})
}
