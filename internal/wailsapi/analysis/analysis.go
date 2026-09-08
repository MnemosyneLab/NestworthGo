// Package analysis exposes the Phase 2a Asset Changes projections. It is
// intentionally separate from the legacy analytics service, which remains the
// read surface for cost and gain screens.
package analysis

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// AnalysisQueryRequest is the wire form of domain.AnalysisQuery. IDs remain
// strings at the IPC boundary; conversion validates and canonicalizes them
// before the application service sees the query.
type AnalysisQueryRequest struct {
	ScopeKind    string  `json:"scopeKind"`
	ScopeID      string  `json:"scopeId,omitempty"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Valuation    string  `json:"valuation"`
	Basis        string  `json:"basis"`
	IncludeCash  bool    `json:"includeCash"`
	AccountID    *string `json:"accountId,omitempty"`
	Currency     *string `json:"currency,omitempty"`
	AssetClass   string  `json:"assetClass,omitempty"`
	InstrumentID *string `json:"instrumentId,omitempty"`
	MemberID     *string `json:"memberId,omitempty"`
}

func (r AnalysisQueryRequest) ToDomain() (domain.AnalysisQuery, error) {
	scopeKind := strings.TrimSpace(r.ScopeKind)
	if scopeKind == "" {
		scopeKind = string(domain.ScopeHousehold)
	}
	kind, err := domain.ParseScopeKind(scopeKind)
	if err != nil {
		return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "scopeKind", Message: "scope kind is invalid"}
	}
	scopeID := strings.TrimSpace(r.ScopeID)
	switch kind {
	case domain.ScopeAccount:
		id, parseErr := domain.ParseAccountID(scopeID)
		if parseErr != nil {
			return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "scopeId", Message: "scope ID is invalid"}
		}
		scopeID = id.String()
	case domain.ScopeInstrument:
		id, parseErr := domain.ParseInstrumentID(scopeID)
		if parseErr != nil {
			return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "scopeId", Message: "scope ID is invalid"}
		}
		scopeID = id.String()
	case domain.ScopeCurrency:
		id, parseErr := domain.ParseCurrency(scopeID)
		if parseErr != nil {
			return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "scopeId", Message: "scope ID is invalid"}
		}
		scopeID = id.String()
	case domain.ScopeAssetClass:
		if scopeID == "" {
			return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "scopeId", Message: "scope ID is required"}
		}
		scopeID = strings.ToLower(scopeID)
	case domain.ScopeHousehold:
		scopeID = ""
	}
	valuation := strings.TrimSpace(r.Valuation)
	if valuation == "" {
		valuation = string(domain.ValuationBase)
	}
	parsedValuation, err := domain.ParseValuation(valuation)
	if err != nil {
		return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "valuation", Message: "valuation is invalid"}
	}
	basis := strings.TrimSpace(r.Basis)
	if basis == "" {
		basis = string(domain.ReturnBasisInvestment)
	}
	parsedBasis, err := domain.ParseReturnBasis(basis)
	if err != nil {
		return domain.AnalysisQuery{}, &domain.Error{Code: domain.ErrValidation, Field: "basis", Message: "return basis is invalid"}
	}
	filters, err := parseFilters(r)
	if err != nil {
		return domain.AnalysisQuery{}, err
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: kind, ID: scopeID}, From: domain.LocalDate(r.From), To: domain.LocalDate(r.To), Valuation: parsedValuation, Basis: parsedBasis, IncludeCash: r.IncludeCash, Filters: filters}
	if err := query.Validate(); err != nil {
		return domain.AnalysisQuery{}, err
	}
	return query, nil
}

func parseFilters(r AnalysisQueryRequest) (domain.AnalysisFilters, error) {
	filters := domain.AnalysisFilters{AssetClass: strings.ToLower(strings.TrimSpace(r.AssetClass))}
	if r.AccountID != nil {
		id, err := domain.ParseAccountID(*r.AccountID)
		if err != nil {
			return domain.AnalysisFilters{}, &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "account ID is invalid"}
		}
		filters.AccountID = &id
	}
	if r.Currency != nil {
		id, err := domain.ParseCurrency(*r.Currency)
		if err != nil {
			return domain.AnalysisFilters{}, &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "currency is invalid"}
		}
		filters.Currency = &id
	}
	if r.InstrumentID != nil {
		id, err := domain.ParseInstrumentID(*r.InstrumentID)
		if err != nil {
			return domain.AnalysisFilters{}, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument ID is invalid"}
		}
		filters.InstrumentID = &id
	}
	if r.MemberID != nil {
		id, err := domain.ParseMemberID(*r.MemberID)
		if err != nil {
			return domain.AnalysisFilters{}, &domain.Error{Code: domain.ErrValidation, Field: "memberId", Message: "member ID is invalid"}
		}
		filters.MemberID = &id
	}
	return filters, nil
}

// Aliases keep the converter names local to this package while the actual
// wire contract remains in the shared wire package.
type AssetChangeDTO = wire.AssetChangeDTO
type AssetChangeSummaryDTO = wire.AssetChangeSummaryDTO
type AssetChangeRowDTO = wire.AssetChangeRowDTO
type AssetChangeGroupDTO = wire.AssetChangeGroupDTO
type AnalysisDimensionAmountDTO = wire.AnalysisDimensionAmountDTO
type AssetDriverDetailDTO = wire.AssetDriverDetailDTO
type AssetResidualDetailDTO = wire.AssetResidualDetailDTO
type AssetTrendPointDTO = wire.AssetTrendPointDTO
type AssetTrendDTO = wire.AssetTrendDTO
type CategoryRowDTO = wire.CategoryRowDTO
type CategoriesDTO = wire.CategoriesDTO
type CategoryChildDTO = wire.CategoryChildDTO
type CategoryActivityRefDTO = wire.CategoryActivityRefDTO
type CategoryDetailDTO = wire.CategoryDetailDTO

func (s *Service) AssetChange(ctx context.Context, request AnalysisQueryRequest) (AssetChangeDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return AssetChangeDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.AssetChange(ctx, query)
	if err != nil {
		return AssetChangeDTO{}, apierror.Wrap(err)
	}
	return fromAssetChange(result), nil
}

func (s *Service) AssetDriverDetail(ctx context.Context, request AnalysisQueryRequest, driverKey string) (AssetDriverDetailDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return AssetDriverDetailDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.AssetDriverDetail(ctx, query, driverKey)
	if err != nil {
		return AssetDriverDetailDTO{}, apierror.Wrap(err)
	}
	return fromAssetDriverDetail(result), nil
}

func (s *Service) AssetTrend(ctx context.Context, request AnalysisQueryRequest, granularity, metric string) (AssetTrendDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return AssetTrendDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.AssetTrend(ctx, query, application.AssetTrendGranularity(strings.TrimSpace(granularity)), application.AssetTrendMetric(strings.TrimSpace(metric)))
	if err != nil {
		return AssetTrendDTO{}, apierror.Wrap(err)
	}
	return fromAssetTrend(result), nil
}

func (s *Service) Categories(ctx context.Context, request AnalysisQueryRequest, categoryType string) (CategoriesDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return CategoriesDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.Categories(ctx, query, application.AnalysisCategoryType(strings.TrimSpace(categoryType)))
	if err != nil {
		return CategoriesDTO{}, apierror.Wrap(err)
	}
	return fromCategories(result), nil
}

func (s *Service) CategoryDetail(ctx context.Context, request AnalysisQueryRequest, categoryType, rowKey string) (CategoryDetailDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return CategoryDetailDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.CategoryDetail(ctx, query, application.AnalysisCategoryType(strings.TrimSpace(categoryType)), rowKey)
	if err != nil {
		return CategoryDetailDTO{}, apierror.Wrap(err)
	}
	return fromCategoryDetail(result), nil
}

func fromAssetChange(value application.AssetChangeResult) AssetChangeDTO {
	return AssetChangeDTO{
		Summary: AssetChangeSummaryDTO{
			BeginningValue: wire.FromSignedMoneyPtr(value.Summary.BeginningValue),
			EndingValue:    wire.FromSignedMoneyPtr(value.Summary.EndingValue),
			Change:         wire.FromSignedMoneyPtr(value.Summary.Change),
		},
		Available:       value.Available,
		Status:          string(value.Status),
		MissingReason:   value.MissingReason,
		ValuationForced: value.ValuationForced,
		Waterfall:       fromAssetChangeRows(value.Waterfall),
		Groups:          fromAssetChangeGroups(value.Groups),
	}
}

func fromAssetChangeRows(values []application.AssetChangeRow) []AssetChangeRowDTO {
	result := make([]AssetChangeRowDTO, 0, len(values))
	for _, value := range values {
		result = append(result, AssetChangeRowDTO{Key: value.Key, Label: value.Label, Bucket: string(value.Bucket), Amount: wire.FromSignedMoneyPtr(value.Amount)})
	}
	return result
}

func fromAssetChangeGroups(values []application.AssetChangeGroup) []AssetChangeGroupDTO {
	result := make([]AssetChangeGroupDTO, 0, len(values))
	for _, value := range values {
		result = append(result, AssetChangeGroupDTO{Key: value.Key, Label: value.Label, Amount: wire.FromSignedMoneyPtr(value.Amount), Rows: fromAssetChangeRows(value.Rows)})
	}
	return result
}

func fromAssetDriverDetail(value application.AssetDriverDetailResult) AssetDriverDetailDTO {
	result := AssetDriverDetailDTO{DriverKey: value.DriverKey, Available: value.Available, Status: string(value.Status), MissingReason: value.MissingReason, ValuationForced: value.ValuationForced, ByInstrument: fromDimensionAmounts(value.ByInstrument), ByAccount: fromDimensionAmounts(value.ByAccount), ResidualDetails: fromAssetResidualDetails(value.ResidualDetails)}
	return result
}

func fromAssetResidualDetails(values []application.AssetResidualDetail) []AssetResidualDetailDTO {
	result := make([]AssetResidualDetailDTO, 0, len(values))
	for _, value := range values {
		result = append(result, AssetResidualDetailDTO{Date: string(value.Date), ComponentKey: value.ComponentKey, AccountID: value.AccountID, HoldingID: value.HoldingID, InstrumentID: value.InstrumentID, Amount: wire.FromSignedMoneyPtr(value.Amount)})
	}
	return result
}

func fromDimensionAmounts(values []application.AnalysisDimensionAmount) []AnalysisDimensionAmountDTO {
	result := make([]AnalysisDimensionAmountDTO, 0, len(values))
	for _, value := range values {
		result = append(result, AnalysisDimensionAmountDTO{Key: value.Key, Label: value.Label, AccountID: value.AccountID, InstrumentID: value.InstrumentID, Amount: wire.FromSignedMoneyPtr(value.Amount)})
	}
	return result
}

func fromAssetTrend(value application.AssetTrendResult) AssetTrendDTO {
	result := AssetTrendDTO{Available: value.Available, Status: string(value.Status), MissingReason: value.MissingReason, ValuationForced: value.ValuationForced, Summary: wire.FromSignedMoneyPtr(value.Summary), Rate: decimalStringPtr(value.Rate), RatedDays: value.Coverage.RatedDays, TotalDays: value.Coverage.TotalDays, Points: make([]AssetTrendPointDTO, 0, len(value.Points))}
	for _, point := range value.Points {
		result.Points = append(result.Points, AssetTrendPointDTO{Period: point.Period, Value: wire.FromSignedMoneyPtr(point.Value), Rate: decimalStringPtr(point.Rate), RatedDays: point.Coverage.RatedDays, TotalDays: point.Coverage.TotalDays, Available: point.Available, Status: string(point.Status), MissingReason: point.MissingReason, ValuationForced: point.ValuationForced})
	}
	return result
}

func fromCategories(value application.CategoriesResult) CategoriesDTO {
	result := CategoriesDTO{Total: wire.FromSignedMoneyPtr(value.Total), Available: value.Available, Status: string(value.Status), MissingReason: value.MissingReason, ValuationForced: value.ValuationForced, Rows: make([]CategoryRowDTO, 0, len(value.Rows))}
	for _, row := range value.Rows {
		result.Rows = append(result.Rows, CategoryRowDTO{Key: row.Key, Label: row.Label, AccountID: row.AccountID, InstrumentID: row.InstrumentID, AssetClass: row.AssetClass, Amount: wire.FromSignedMoneyPtr(row.Amount)})
	}
	return result
}

func fromCategoryDetail(value application.CategoryDetailResult) CategoryDetailDTO {
	result := CategoryDetailDTO{Available: value.Available, Status: string(value.Status), MissingReason: value.MissingReason, ValuationForced: value.ValuationForced, Children: make([]CategoryChildDTO, 0, len(value.Children)), ActivityRefs: make([]CategoryActivityRefDTO, 0, len(value.ActivityRefs))}
	for _, child := range value.Children {
		result.Children = append(result.Children, CategoryChildDTO{Key: child.Key, Label: child.Label, Amount: wire.FromSignedMoneyPtr(child.Amount)})
	}
	for _, ref := range value.ActivityRefs {
		result.ActivityRefs = append(result.ActivityRefs, CategoryActivityRefDTO{Date: ref.Date, ActivityID: ref.ActivityID, AccountID: ref.AccountID, HoldingID: ref.HoldingID, InstrumentID: ref.InstrumentID, Amount: wire.FromSignedMoneyPtr(ref.Amount)})
	}
	return result
}
