package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// ListActivitiesInput uses the same filters and three-part cursor as the
// History Wails adapter. All dates refer to the immutable History Origin zone.
type ListActivitiesInput struct {
	AccountID        *string  `json:"accountId,omitempty"`
	InstrumentID     *string  `json:"instrumentId,omitempty"`
	Kinds            []string `json:"kinds,omitempty" jsonschema:"Persisted activity kinds such as cash_in, cash_out, buy or sell; not change command kinds"`
	FromLocalDate    string   `json:"fromLocalDate,omitempty"`
	ToLocalDate      string   `json:"toLocalDate,omitempty"`
	AfterEffectiveAt string   `json:"afterEffectiveAt,omitempty"`
	AfterCreatedAt   string   `json:"afterCreatedAt,omitempty"`
	AfterID          string   `json:"afterId,omitempty"`
	Limit            int      `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; defaults to 100"`
}

func (in ListActivitiesInput) request() history.ActivityQueryRequest {
	return history.ActivityQueryRequest{
		AccountID: in.AccountID, InstrumentID: in.InstrumentID, Kinds: in.Kinds,
		FromLocalDate: in.FromLocalDate, ToLocalDate: in.ToLocalDate,
		AfterEffectiveAt: in.AfterEffectiveAt, AfterCreatedAt: in.AfterCreatedAt,
		AfterID: in.AfterID, Limit: in.Limit,
	}
}

type AnalyzePeriodInput struct {
	Query PeriodAnalysisQuery `json:"query" jsonschema:"Inclusive from/to YYYY-MM-DD local dates and optional scope, valuation, and filters"`
}

// The Wails adapter defaults omitted scope, valuation and basis. This MCP
// input keeps those fields optional in the published JSON schema as well.
type PeriodAnalysisQuery struct {
	From         string  `json:"from"`
	To           string  `json:"to"`
	ScopeKind    string  `json:"scopeKind,omitempty"`
	ScopeID      string  `json:"scopeId,omitempty"`
	Valuation    string  `json:"valuation,omitempty"`
	Basis        string  `json:"basis,omitempty"`
	IncludeCash  bool    `json:"includeCash,omitempty"`
	AccountID    *string `json:"accountId,omitempty"`
	Currency     *string `json:"currency,omitempty"`
	AssetClass   string  `json:"assetClass,omitempty"`
	InstrumentID *string `json:"instrumentId,omitempty"`
	MemberID     *string `json:"memberId,omitempty"`
}

func (q PeriodAnalysisQuery) request() analysis.AnalysisQueryRequest {
	return analysis.AnalysisQueryRequest{
		From: q.From, To: q.To, ScopeKind: q.ScopeKind, ScopeID: q.ScopeID,
		Valuation: q.Valuation, Basis: q.Basis, IncludeCash: q.IncludeCash,
		AccountID: q.AccountID, Currency: q.Currency, AssetClass: q.AssetClass,
		InstrumentID: q.InstrumentID, MemberID: q.MemberID,
	}
}

type PeriodAnalysisDTO struct {
	Income            wire.CategoriesDTO  `json:"income"`
	Expenses          wire.CategoriesDTO  `json:"expenses"`
	AssetChange       wire.AssetChangeDTO `json:"assetChange"`
	InvestmentReturns wire.ReturnTrendDTO `json:"investmentReturns"`
}

func (s *Service) historyTools(server *mcp.Server) {
	historyService := history.NewService(s.app)
	analysisService := analysis.NewService(s.app)
	readTool(server, "list_activities", "Page recorded activities, including corrections, with optional account, instrument, persisted activity kind and inclusive local-date filters. Dates use the immutable History Origin timezone. Pass all three next cursor fields as afterEffectiveAt, afterCreatedAt and afterId; an empty next means the end.", func(ctx context.Context, in ListActivitiesInput) (any, error) {
		if in.Limit < 0 || in.Limit > 100 {
			return nil, fail("validation", "limit must be between 1 and 100, or omitted")
		}
		cursorFields := 0
		for _, value := range []string{in.AfterEffectiveAt, in.AfterCreatedAt, in.AfterID} {
			if value != "" {
				cursorFields++
			}
		}
		if cursorFields != 0 && cursorFields != 3 {
			return nil, fail("validation", "provide all three next cursor fields together")
		}
		return historyService.ListActivityPage(ctx, in.request())
	})
	readTool(server, "get_activity", "Read one recorded activity by UUID, including its effects, trade or dividend details, and correction links. IDs come from list_activities.", func(ctx context.Context, in IDInput) (any, error) {
		return historyService.Activity(ctx, in.ID)
	})
	analysisTool(server, "analyze_period", "Analyze income, expenses, asset changes and investment returns for an inclusive YYYY-MM-DD range in the immutable History Origin timezone. The range must end before the current local day; missing prices/FX can leave closed-day results partial or unavailable. Preserve each DTO's available, status, missingReason, nullable amounts/rates and ratedDays/totalDays; missing values are not zero. This reads local records and does not place brokerage orders.", func(ctx context.Context, in AnalyzePeriodInput) (any, error) {
		var result PeriodAnalysisDTO
		query := in.Query.request()
		// Keep all four projections on one application revision. Nested snapshot
		// maintenance in the adapters remains reentrant on this context.
		err := s.app.WithWrite(ctx, func(ctx context.Context) error {
			var err error
			result.Income, err = analysisService.Categories(ctx, query, "income")
			if err != nil {
				return err
			}
			result.Expenses, err = analysisService.Categories(ctx, query, "spending")
			if err != nil {
				return err
			}
			result.AssetChange, err = analysisService.AssetChange(ctx, query)
			if err != nil {
				return err
			}
			result.InvestmentReturns, err = analysisService.ReturnTrend(ctx, query, "period_return_amount")
			return err
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
	s.attributionTools(server, analysisService)
}
