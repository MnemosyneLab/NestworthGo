// Package portfolio adapts internal/application.Service's read-only
// Overview/Portfolio/NetWorthTrend surface for the Wails IPC boundary. It
// has no mutation methods.
package portfolio

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// OverviewDTO mirrors domain.OverviewResult.
type OverviewDTO struct {
	Currency          string                       `json:"currency"`
	AccountCount      int                          `json:"accountCount"`
	Complete          bool                         `json:"complete"`
	MissingInputs     []wire.MissingInputDTO       `json:"missingInputs"`
	Assets            string                       `json:"assets"`
	Liabilities       string                       `json:"liabilities"`
	NetWorth          string                       `json:"netWorth"`
	AssetsByType      []wire.BreakdownDTO          `json:"assetsByType"`
	LiabilitiesByType []wire.BreakdownDTO          `json:"liabilitiesByType"`
	ByMember          []wire.BreakdownDTO          `json:"byMember"`
	ByInstitution     []wire.BreakdownDTO          `json:"byInstitution"`
	ByGroup           []wire.BreakdownDTO          `json:"byGroup"`
	ByAccountType     []wire.BreakdownDTO          `json:"byAccountType"`
	HistoryStarted    bool                         `json:"historyStarted"`
	RecentActivities  []wire.ActivityDTO           `json:"recentActivities"`
	AccountLabels     []OverviewNamedDTO           `json:"accountLabels"`
	InstrumentLabels  []OverviewInstrumentLabelDTO `json:"instrumentLabels"`
	HoldingLabels     []OverviewHoldingLabelDTO    `json:"holdingLabels"`
}

type OverviewNamedDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OverviewInstrumentLabelDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	QuoteSource string `json:"quoteSource,omitempty"`
}

type OverviewHoldingLabelDTO struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	InstrumentID string `json:"instrumentId"`
	Name         string `json:"name"`
}

func fromOverview(value domain.OverviewResult) OverviewDTO {
	assets, liabilities, netWorth := value.Assets.String(), value.Liabilities.String(), value.NetWorth.String()
	if value.Assets.IsZero() {
		assets = "0"
	}
	if value.Liabilities.IsZero() {
		liabilities = "0"
	}
	if value.NetWorth.IsZero() {
		netWorth = "0"
	}
	accountLabels := make([]OverviewNamedDTO, 0, len(value.AccountLabels))
	for _, label := range value.AccountLabels {
		accountLabels = append(accountLabels, OverviewNamedDTO{ID: label.ID, Name: label.Name})
	}
	instrumentLabels := make([]OverviewInstrumentLabelDTO, 0, len(value.InstrumentLabels))
	for _, label := range value.InstrumentLabels {
		instrumentLabels = append(instrumentLabels, OverviewInstrumentLabelDTO{ID: label.ID, Name: label.Name, QuoteSource: string(label.QuoteSource)})
	}
	holdingLabels := make([]OverviewHoldingLabelDTO, 0, len(value.HoldingLabels))
	for _, label := range value.HoldingLabels {
		holdingLabels = append(holdingLabels, OverviewHoldingLabelDTO{ID: label.ID, AccountID: label.AccountID, InstrumentID: label.InstrumentID, Name: label.Name})
	}
	return OverviewDTO{
		Currency: value.Currency.String(), AccountCount: value.AccountCount, Complete: value.Complete,
		MissingInputs: wire.FromMissingInputs(value.MissingInputs),
		Assets:        assets, Liabilities: liabilities, NetWorth: netWorth,
		AssetsByType: wire.FromBreakdowns(value.AssetsByType), LiabilitiesByType: wire.FromBreakdowns(value.LiabilitiesByType), ByMember: wire.FromBreakdowns(value.ByMember),
		ByInstitution: wire.FromBreakdowns(value.ByInstitution), ByGroup: wire.FromBreakdowns(value.ByGroup),
		ByAccountType:  wire.FromBreakdowns(value.ByAccountType),
		HistoryStarted: value.HistoryStarted, RecentActivities: wire.FromActivities(value.RecentActivities),
		AccountLabels: accountLabels, InstrumentLabels: instrumentLabels, HoldingLabels: holdingLabels,
	}
}

func (s *Service) Overview(ctx context.Context, request account.AccountFilterRequest) (OverviewDTO, error) {
	filter, err := request.ToDomain()
	if err != nil {
		return OverviewDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.Overview(ctx, filter)
	if err != nil {
		return OverviewDTO{}, apierror.Wrap(err)
	}
	return fromOverview(result), nil
}

// PortfolioDTO mirrors domain.PortfolioValuation.
type PortfolioDTO struct {
	Currency         string                     `json:"currency"`
	ValuedSubtotal   *wire.MoneyView            `json:"valuedSubtotal,omitempty"`
	Complete         bool                       `json:"complete"`
	Accounts         []wire.AccountValuationDTO `json:"accounts"`
	MissingInputs    []wire.MissingInputDTO     `json:"missingInputs"`
	ByCurrency       []wire.AllocationDTO       `json:"byCurrency"`
	ByCountry        []wire.AllocationDTO       `json:"byCountry"`
	ByInstrumentType []wire.AllocationDTO       `json:"byInstrumentType"`
}

func fromPortfolio(value domain.PortfolioValuation) PortfolioDTO {
	return PortfolioDTO{
		Currency: value.Currency.String(), ValuedSubtotal: wire.FromMoneyView(value.ValuedSubtotal), Complete: value.Complete,
		Accounts: wire.FromAccountValuations(value.Accounts), MissingInputs: wire.FromMissingInputs(value.MissingInputs),
		ByCurrency: wire.FromAllocations(value.ByCurrency), ByCountry: wire.FromAllocations(value.ByCountry),
		ByInstrumentType: wire.FromAllocations(value.ByInstrumentType),
	}
}

func (s *Service) Portfolio(ctx context.Context, request account.AccountFilterRequest) (PortfolioDTO, error) {
	filter, err := request.ToDomain()
	if err != nil {
		return PortfolioDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.Portfolio(ctx, filter)
	if err != nil {
		return PortfolioDTO{}, apierror.Wrap(err)
	}
	return fromPortfolio(result), nil
}

// NetWorthTrendPointDTO mirrors domain.NetWorthTrendPoint.
type NetWorthTrendPointDTO struct {
	LocalDate    string                `json:"localDate"`
	NetWorth     *wire.SignedMoneyView `json:"netWorth,omitempty"`
	Assets       *wire.MoneyView       `json:"assets,omitempty"`
	Liabilities  *wire.MoneyView       `json:"liabilities,omitempty"`
	Complete     bool                  `json:"complete"`
	MissingCount int                   `json:"missingCount"`
}

// NetWorthTrendDTO mirrors domain.NetWorthTrend.
type NetWorthTrendDTO struct {
	Range    string                  `json:"range"`
	Currency string                  `json:"currency"`
	Points   []NetWorthTrendPointDTO `json:"points"`
	Start    *wire.SignedMoneyView   `json:"start,omitempty"`
	End      *wire.SignedMoneyView   `json:"end,omitempty"`
	Change   *wire.SignedMoneyView   `json:"change,omitempty"`
}

func fromNetWorthTrend(result domain.NetWorthTrend) NetWorthTrendDTO {
	points := make([]NetWorthTrendPointDTO, 0, len(result.Points))
	for _, point := range result.Points {
		points = append(points, NetWorthTrendPointDTO{
			LocalDate:    point.LocalDate,
			NetWorth:     wire.FromSignedMoneyPtr(point.NetWorth),
			Assets:       wire.FromMoneyPtr(point.Assets),
			Liabilities:  wire.FromMoneyPtr(point.Liabilities),
			Complete:     point.Complete,
			MissingCount: point.MissingCount,
		})
	}
	var change *wire.SignedMoneyView
	if result.Change != nil {
		view := wire.FromSignedMoney(*result.Change)
		change = &view
	}
	return NetWorthTrendDTO{
		Range: string(result.Range), Currency: result.Currency.String(), Points: points,
		Start: wire.FromSignedMoneyPtr(result.Start), End: wire.FromSignedMoneyPtr(result.End), Change: change,
	}
}

func (s *Service) NetWorthTrend(ctx context.Context, trendRange string) (NetWorthTrendDTO, error) {
	parsed, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return NetWorthTrendDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.NetWorthTrend(ctx, parsed)
	if err != nil {
		return NetWorthTrendDTO{}, apierror.Wrap(err)
	}
	return fromNetWorthTrend(result), nil
}

// PortfolioTrendPointDTO mirrors domain.PortfolioTrendPoint.
type PortfolioTrendPointDTO struct {
	LocalDate      string          `json:"localDate"`
	ValuedSubtotal *wire.MoneyView `json:"valuedSubtotal,omitempty"`
	Complete       bool            `json:"complete"`
	MissingCount   int             `json:"missingCount"`
}

// PortfolioTrendDTO mirrors domain.PortfolioTrend.
type PortfolioTrendDTO struct {
	Range    string                   `json:"range"`
	Currency string                   `json:"currency"`
	Points   []PortfolioTrendPointDTO `json:"points"`
}

func (s *Service) PortfolioTrend(ctx context.Context, trendRange string) (PortfolioTrendDTO, error) {
	parsed, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return PortfolioTrendDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.PortfolioTrend(ctx, parsed)
	if err != nil {
		return PortfolioTrendDTO{}, apierror.Wrap(err)
	}
	points := make([]PortfolioTrendPointDTO, 0, len(result.Points))
	for _, point := range result.Points {
		points = append(points, PortfolioTrendPointDTO{
			LocalDate:      point.LocalDate,
			ValuedSubtotal: wire.FromMoneyPtr(point.ValuedSubtotal),
			Complete:       point.Complete,
			MissingCount:   point.MissingCount,
		})
	}
	return PortfolioTrendDTO{Range: string(result.Range), Currency: result.Currency.String(), Points: points}, nil
}
