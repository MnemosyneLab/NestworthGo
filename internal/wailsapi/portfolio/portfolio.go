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
	Currency          string                 `json:"currency"`
	AccountCount      int                    `json:"accountCount"`
	Complete          bool                   `json:"complete"`
	MissingInputs     []wire.MissingInputDTO `json:"missingInputs"`
	Assets            string                 `json:"assets"`
	Liabilities       string                 `json:"liabilities"`
	NetWorth          string                 `json:"netWorth"`
	AssetsByType      []wire.BreakdownDTO    `json:"assetsByType"`
	LiabilitiesByType []wire.BreakdownDTO    `json:"liabilitiesByType"`
	ByMember          []wire.BreakdownDTO    `json:"byMember"`
	ByInstitution     []wire.BreakdownDTO    `json:"byInstitution"`
	ByGroup           []wire.BreakdownDTO    `json:"byGroup"`
	ByAccountType     []wire.BreakdownDTO    `json:"byAccountType"`
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
	return OverviewDTO{
		Currency: value.Currency.String(), AccountCount: value.AccountCount, Complete: value.Complete,
		MissingInputs: wire.FromMissingInputs(value.MissingInputs),
		Assets:        assets, Liabilities: liabilities, NetWorth: netWorth,
		AssetsByType: wire.FromBreakdowns(value.AssetsByType), LiabilitiesByType: wire.FromBreakdowns(value.LiabilitiesByType), ByMember: wire.FromBreakdowns(value.ByMember),
		ByInstitution: wire.FromBreakdowns(value.ByInstitution), ByGroup: wire.FromBreakdowns(value.ByGroup),
		ByAccountType: wire.FromBreakdowns(value.ByAccountType),
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
	LocalDate string          `json:"localDate"`
	Value     *wire.MoneyView `json:"value,omitempty"`
	Complete  bool            `json:"complete"`
}

// NetWorthTrendDTO mirrors domain.NetWorthTrend.
type NetWorthTrendDTO struct {
	Range    string                  `json:"range"`
	Currency string                  `json:"currency"`
	Points   []NetWorthTrendPointDTO `json:"points"`
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
	points := make([]NetWorthTrendPointDTO, 0, len(result.Points))
	for _, point := range result.Points {
		points = append(points, NetWorthTrendPointDTO{LocalDate: point.LocalDate, Value: wire.FromMoneyPtr(point.Value), Complete: point.Complete})
	}
	return NetWorthTrendDTO{Range: string(result.Range), Currency: result.Currency.String(), Points: points}, nil
}
