package mcpserver

import (
	"context"

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
	Limit            int      `json:"limit,omitempty" jsonschema:"Page size, 1 to 100; defaults to 25"`
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
	Limit int                 `json:"limit,omitempty" jsonschema:"Initial rows per section; omitted fills small results within a byte budget (at least 5 when they fit, at most 25), maximum 100"`
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
	Currency     *string `json:"currency,omitempty" jsonschema:"Currency filter. For realized contributions this matches the sell event's native denomination, not account default or output currency."`
	AssetClass   string  `json:"assetClass,omitempty" jsonschema:"Asset-class filter. For realized contributions this matches the instrument's current Type metadata."`
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

func (s *Service) historyTools(server *toolSet) {
	generation := s.contextGeneration
	historyService := history.NewService(s.app)
	analysisService := analysis.NewService(s.app)
	readTool(server, "list_activities", "Page live recorded activities with account/instrument/kind/date filters in the history timezone. Default 25/max 100; actual pages may be smaller to fit 64 KiB complete wire. Continue page.nextCursor with get_read_page. Short pages do not prove completion.", func(ctx context.Context, in ListActivitiesInput) (any, error) {
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
		size := in.Limit
		if size == 0 {
			size = 25
		}
		return fitLiveReadPage(ctx, s.app, size, func(ctx context.Context, size int) (wire.ActivityPageDTO, error) {
			query := in.request()
			query.Limit = size
			return historyService.ListActivityPage(ctx, query)
		})
	})
	readTool(server, "get_activity", "Read one recorded activity by UUID, including its effects, trade or dividend details, and correction links. IDs come from list_activities.", func(ctx context.Context, in IDInput) (any, error) {
		return historyService.Activity(ctx, in.ID)
	})
	analysisTool(server, "analyze_period", "Income, expenses, asset changes and investment returns for an inclusive YYYY-MM-DD range in the household history timezone; the range must end before the current local day. Missing prices or FX can leave results partial or unavailable: keep available, status, missingReason and nullable amounts and rates, and never read null as zero. Own-account transfers and purchase principal are not income or expenses. Returns a summary plus sections (income, expenses, assetChange, investmentReturns); continue with get_read_page. Open one frozen return day with get_read_item section=returnDays and id=YYYY-MM-DD, or one asset-change bucket with section=assetDrivers and the bucket key. Those details stay with this capture and are omitted from its stored body. May refresh derived snapshots.", func(ctx context.Context, in AnalyzePeriodInput) (any, error) {
		result, drilldown, err := loadPeriodAnalysis(ctx, s.app, analysisService, in.Query)
		if err != nil {
			return nil, err
		}
		return s.buildRead(ctx, generation, autoInlineWhenUnset(in.Limit), func(context.Context) (readCapture, error) {
			return periodReadCapture(in, result, drilldown)
		})
	})
	s.attributionTools(server)
}
