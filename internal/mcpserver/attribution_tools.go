package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type ListContributionsInput struct {
	Query      PeriodAnalysisQuery `json:"query" jsonschema:"Inclusive closed-day YYYY-MM-DD range in the History Origin timezone. Scope and filters intersect. For realized, currency matches the sell event's native denomination and assetClass matches current Instrument.Type; member filtering is unsupported."`
	ReturnType string              `json:"returnType" jsonschema:"One of total_return, realized, unrealized, dividend_interest; unrealized currently reports unavailable"`
	GroupBy    string              `json:"groupBy" jsonschema:"One of instrument, account, currency, asset_class"`
	Ordering   string              `json:"ordering,omitempty" jsonschema:"One of amount_desc, amount_asc, rate_desc, rate_asc, name_asc; defaults to amount_desc"`
	Limit      int                 `json:"limit,omitempty" jsonschema:"Initial row count, 1 to 100; defaults to 25. Continue rows with get_read_page."`
}

type GetContributionItemInput struct {
	CaptureID string `json:"captureId" jsonschema:"Exact captureId returned by list_contributions"`
	GroupKey  string `json:"groupKey" jsonschema:"Exact row key from that frozen capture"`
}

type GetReturnDayInput struct {
	Limit int                 `json:"limit,omitempty" jsonschema:"Initial rows per section, default 25, maximum 100"`
	Query PeriodAnalysisQuery `json:"query" jsonschema:"Inclusive closed-day range in the History Origin timezone"`
	Date  string              `json:"date" jsonschema:"YYYY-MM-DD date within query.from and query.to"`
}

type GetAssetDriverDetailInput struct {
	Limit     int                 `json:"limit,omitempty" jsonschema:"Initial rows per section, default 25, maximum 100"`
	Query     PeriodAnalysisQuery `json:"query" jsonschema:"Use the same query as analyze_period"`
	DriverKey string              `json:"driverKey" jsonschema:"One assetChange row bucket: external_flow, income, spending, dividend_interest, price_change, fx_impact, fx_conversion_spread, fee, liability_impact, adjustment, residual"`
}

// The regular readTool infers descriptions but has no enum metadata. These
// tools publish the application adapters' accepted values as schema enums.
func readAttributionTool[I any](server *toolSet, name, description string, enums map[string][]any, fn func(context.Context, I) (any, error)) {
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
	registerTool(server, &agentcapability.Definition{Name: name, Description: description + " May materialize derived snapshots and invalidate ledger previews; no financial facts or provider data are written. Available in existing read_only mode.", InputSchema: schema, Effect: agentcapability.Analysis, Annotations: &agentcapability.Annotations{ReadOnlyHint: false, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, in I) (Response, error) {
		value, err := fn(ctx, in)
		if err != nil {
			return Response{}, safeError(err)
		}
		return Response{Data: value}, nil
	})
}

func (s *Service) attributionTools(server *toolSet, adapter *analysis.Service) {
	generation := s.contextGeneration
	readAttributionTool(server, "list_contributions", "Page investment return contributions by instrument, account, currency or asset class. Realized contributions can be filtered by scope and filters, which intersect: currency selects the native sell-event denomination, asset class selects the instrument's current Type, and account/instrument constraints also intersect. Member filtering remains unsupported. Realized rows group by instrument or account only; currency and asset-class filters do not add grouping modes. The returned rows keep nullable amounts/rates, available, status, missingReason and ratedDays/totalDays. Returns frozen mcp-read/1 rows; continue with get_read_page and use captureId plus row key with get_contribution_item. Default limit 25, maximum 100. All rows and detail share one capture; expiry or revocation requires restarting it. Dates are inclusive closed days in the History Origin timezone.", map[string][]any{
		"returnType": {"total_return", "realized", "unrealized", "dividend_interest"},
		"groupBy":    {"instrument", "account", "currency", "asset_class"},
		"ordering":   {"amount_desc", "amount_asc", "rate_desc", "rate_asc", "name_asc"},
	}, func(ctx context.Context, in ListContributionsInput) (any, error) {
		ordering := strings.TrimSpace(in.Ordering)
		if ordering == "" {
			ordering = "amount_desc"
		}
		in.Ordering = ordering
		return s.buildRead(ctx, generation, in.Limit, func(ctx context.Context) (readCapture, error) {
			var value wire.ContributionDTO
			var items []wire.ContributionItemDTO
			err := s.app.WithWrite(ctx, func(ctx context.Context) error {
				var err error
				value, items, err = analysis.CaptureContributions(ctx, s.app, in.Query.request(), in.ReturnType, in.GroupBy, ordering)
				return err
			})
			if err != nil {
				return readCapture{}, err
			}
			capture, err := sectionedReadCapture("contributions", in, value, "rows")
			if err != nil {
				return readCapture{}, err
			}
			capture.InitialSections = []string{"rows"}
			capture.Details = map[string]any{}
			capture.DetailSections = map[string][]string{}
			for _, item := range items {
				detail, err := sectionedReadCapture("", nil, item, "components", "byAccount")
				if err != nil {
					return readCapture{}, err
				}
				key := "rows/" + item.Key
				capture.Details[key] = detail.Summary
				for _, field := range []string{"components", "byAccount"} {
					section := key + "/" + field
					capture.Sections[section] = detail.Sections[field]
					capture.DetailSections[key] = append(capture.DetailSections[key], section)
				}
			}
			return capture, nil
		})
	})
	readTool(server, "get_contribution_item", "Read one contribution from its exact frozen list_contributions captureId and groupKey. No recalculation, database reads or TTL renewal. Components and byAccount are independent get_read_page sections. Related-history hints may cover broader records under currency/asset-class filters; they are not a second amount calculation.", func(ctx context.Context, in GetContributionItemInput) (any, error) {
		if strings.TrimSpace(in.GroupKey) == "" {
			return nil, fail("validation", "groupKey must be a non-empty list_contributions row key")
		}
		return s.readItem(generation, ReadItemInput{CaptureID: in.CaptureID, Section: "rows", ID: in.GroupKey}, "contributions")
	})

	readAttributionTool(server, "get_return_day", "Read frozen mcp-read/1 sections with get_read_page continuation for one day's return amount, rate, composition, contributors, coverage and valuation issues. The date must be within the query's inclusive closed-day range. Preserve nullable values and amountStatus; partial amounts are not complete totals.", nil, func(ctx context.Context, in GetReturnDayInput) (any, error) {
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
		if err != nil {
			return nil, err
		}
		return s.buildRead(ctx, generation, in.Limit, func(context.Context) (readCapture, error) {
			return sectionedReadCapture("get_return_day", in, value, "composition", "contributors", "issues")
		})
	})
	readAttributionTool(server, "get_asset_driver_detail", "Read frozen mcp-read/1 sections with get_read_page continuation for instrument and account amounts plus residual source records for one asset-change driver bucket. Use a bucket key from analyze_period.assetChange. Dates are inclusive closed days in the History Origin timezone; preserve available, status, missingReason and nullable amounts.", map[string][]any{
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
		if err != nil {
			return nil, err
		}
		return s.buildRead(ctx, generation, in.Limit, func(context.Context) (readCapture, error) {
			return sectionedReadCapture("get_asset_driver_detail", in, value, "byInstrument", "byAccount", "residualDetails")
		})
	})
}
