package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/version"
)

const ExportFormat = "com.nestworth.export"
const ExportFormatVersion = 2

type ExportDocument struct {
	Format        string             `json:"format"`
	FormatVersion int                `json:"formatVersion"`
	ExportedAt    string             `json:"exportedAt"`
	AppVersion    string             `json:"appVersion"`
	AppBuild      string             `json:"appBuild"`
	Timezone      *string            `json:"timezone"`
	Facts         domain.ExportFacts `json:"facts"`
	CurrentState  ExportCurrentState `json:"currentState"`
}

type ExportCurrentState struct {
	Derived      bool                 `json:"derived"`
	AsOf         string               `json:"asOf"`
	BaseCurrency string               `json:"baseCurrency"`
	FXProvider   string               `json:"fxProvider"`
	Accounts     []ExportAccountState `json:"accounts"`
	Holdings     []ExportHoldingState `json:"holdings"`
}

type ExportAccountState struct {
	AccountID string        `json:"accountId"`
	Archived  bool          `json:"archived"`
	Balance   *ExportMoney  `json:"balance"`
	Cash      []ExportMoney `json:"cash"`
	// A partial valued subtotal must not be mistaken for the full account value.
	ValuedSubtotal *ExportMoney         `json:"valuedSubtotal"`
	Complete       bool                 `json:"complete"`
	MissingInputs  []ExportMissingInput `json:"missingInputs"`
}

type ExportMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type ExportMissingInput struct {
	Kind          string  `json:"kind"`
	InstrumentID  *string `json:"instrumentId"`
	BaseCurrency  string  `json:"baseCurrency"`
	QuoteCurrency string  `json:"quoteCurrency"`
}

type ExportHoldingState struct {
	HoldingID         string               `json:"holdingId"`
	AccountID         string               `json:"accountId"`
	InstrumentID      string               `json:"instrumentId"`
	Archived          bool                 `json:"archived"`
	Quantity          string               `json:"quantity"`
	AverageUnitCost   *ExportMoney         `json:"averageUnitCost"`
	CostBasis         *ExportMoney         `json:"costBasis"`
	CostStatus        string               `json:"costStatus"`
	NativeValue       *ExportMoney         `json:"nativeValue"`
	BaseValue         *ExportMoney         `json:"baseValue"`
	ValuationComplete bool                 `json:"valuationComplete"`
	MissingInputs     []ExportMissingInput `json:"missingInputs"`
}

// ExportJSONBytes does no writes, refreshes, or history rebuilds. The export
// never marshals domain decimal wrappers or runtime configuration directly.
func (s *Service) ExportJSONBytes(ctx context.Context) ([]byte, error) {
	snapshot, err := s.repository.ReadExportSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot.Portfolio.Household == nil {
		return nil, onboardingRequired()
	}
	now := s.clock()
	doc := ExportDocument{Format: ExportFormat, FormatVersion: ExportFormatVersion,
		ExportedAt: now.UTC().Format(time.RFC3339Nano), AppVersion: version.Version, AppBuild: version.Build, Facts: snapshot.Facts}
	if origin := snapshot.Portfolio.Origin; origin != nil {
		timezone := origin.Timezone
		doc.Timezone = &timezone
	}
	doc.CurrentState, err = s.exportCurrentState(ctx, snapshot, now)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (s *Service) exportCurrentState(ctx context.Context, snapshot domain.ExportSnapshot, now time.Time) (ExportCurrentState, error) {
	portfolio := snapshot.Portfolio
	provider := s.FXProviderKey()
	ttl := s.QuoteCacheTTL()
	valuation := NewValuationService(nil, func() time.Time { return now })
	valuation.SetFXProviderKey(func() string { return provider })
	valuation.SetQuoteCacheTTL(func() time.Duration { return ttl })
	result := ExportCurrentState{Derived: true, AsOf: now.UTC().Format(time.RFC3339Nano),
		BaseCurrency: portfolio.Household.BaseCurrency.String(), FXProvider: provider,
		Accounts: []ExportAccountState{}, Holdings: []ExportHoldingState{}}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, item := range portfolio.Instruments {
		instruments[item.ID] = item
	}
	// Preloaded maps make cost replay (including transfer sources) independent
	// of any subsequent live database changes.
	replay := newCostBasisReplayContext(nil, portfolio.Holdings)
	replay.events, replay.starting = snapshot.CostEvents, snapshot.StartingCosts
	for _, record := range portfolio.Accounts {
		holdings := []domain.Holding{}
		cash := []domain.AccountCashValue{}
		for _, item := range portfolio.Holdings {
			if item.AccountID == record.Account.ID {
				holdings = append(holdings, item)
			}
		}
		for _, item := range portfolio.CashValues {
			if item.AccountID == record.Account.ID {
				cash = append(cash, item)
			}
		}
		valued, err := valuation.valueAccount(portfolio, record, instruments, holdings, cash)
		if err != nil {
			return result, err
		}
		account := ExportAccountState{AccountID: record.Account.ID.String(), Archived: record.Account.ArchivedAt != nil,
			Cash: []ExportMoney{}, ValuedSubtotal: exportMoneyView(valued.model.BaseValue), Complete: valued.model.Complete,
			MissingInputs: exportMissing(valued.model.MissingInputs)}
		if record.LatestValue != nil {
			account.Balance = &ExportMoney{Amount: record.LatestValue.Amount.CanonicalAmount(), Currency: record.LatestValue.Amount.Currency().String()}
		}
		for _, value := range latestCashValues(cash) {
			account.Cash = append(account.Cash, ExportMoney{Amount: value.Amount.CanonicalAmount(), Currency: value.Amount.Currency().String()})
		}
		result.Accounts = append(result.Accounts, account)
	}
	for _, holding := range portfolio.Holdings {
		instrument, ok := instruments[holding.InstrumentID]
		if !ok {
			return result, &domain.Error{Code: domain.ErrIntegrity, Message: "export holding instrument is missing"}
		}
		component, missing, err := valuation.valueHolding(portfolio, holding.AccountID, holding, instrument)
		if err != nil {
			return result, err
		}
		state := ExportHoldingState{HoldingID: holding.ID.String(), AccountID: holding.AccountID.String(), InstrumentID: holding.InstrumentID.String(),
			Archived: holding.ArchivedAt != nil, Quantity: holding.Quantity.Canonical(), CostStatus: "unavailable",
			BaseValue: exportMoneyView(component.BaseAmount), ValuationComplete: component.Available, MissingInputs: exportMissing(missing)}
		if component.NativeAmount != "" {
			state.NativeValue = &ExportMoney{Amount: component.NativeAmount, Currency: component.NativeCurrency.String()}
		}
		costs, costErr := replay.replay(ctx, holding.ID, nil)
		if costErr == nil && costs.Current.Quantity.Canonical() == holding.Quantity.Canonical() {
			state.CostStatus = "complete"
			state.AverageUnitCost = &ExportMoney{Amount: costs.Current.AverageUnitCost.Canonical(), Currency: instrument.QuoteCurrency.String()}
			state.CostBasis = &ExportMoney{Amount: costs.Current.AverageUnitCost.Decimal().Mul(holding.Quantity.Decimal()).String(), Currency: instrument.QuoteCurrency.String()}
		} else if costErr != nil {
			var domainErr *domain.Error
			if !errors.As(costErr, &domainErr) || (domainErr.Code != domain.ErrCostBasisRequired && domainErr.Code != domain.ErrUnavailable) {
				return result, costErr
			}
		}
		result.Holdings = append(result.Holdings, state)
	}
	sort.Slice(result.Accounts, func(i, j int) bool { return result.Accounts[i].AccountID < result.Accounts[j].AccountID })
	sort.Slice(result.Holdings, func(i, j int) bool { return result.Holdings[i].HoldingID < result.Holdings[j].HoldingID })
	return result, nil
}

func exportMoneyView(value *domain.MoneyView) *ExportMoney {
	if value == nil {
		return nil
	}
	return &ExportMoney{Amount: value.Amount, Currency: value.Currency.String()}
}

func exportMissing(values []domain.MissingInputView) []ExportMissingInput {
	result := make([]ExportMissingInput, 0, len(values))
	for _, value := range values {
		item := ExportMissingInput{Kind: string(value.Kind), BaseCurrency: value.BaseCurrency.String(), QuoteCurrency: value.QuoteCurrency.String()}
		if value.InstrumentID != nil {
			id := value.InstrumentID.String()
			item.InstrumentID = &id
		}
		result = append(result, item)
	}
	return result
}
