package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type TradeLedger interface {
	CashMovementLedger
	WithRead(context.Context, func(context.Context) error) error
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	AccountValuations(context.Context, domain.AccountFilter) ([]domain.AccountValuation, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
	PreviewExistingTradeCost(context.Context, domain.ChangePreview) (application.ExistingTradeCostPreview, error)
}

type TradeSources struct {
	Origin      domain.HistoryOrigin
	Location    *time.Location
	Account     domain.Account
	Instruments []domain.Instrument
	Holdings    map[domain.InstrumentID]domain.Holding
}

func TradeAccount(account domain.Account) bool {
	return acceptsHoldingsCash(account) && account.AccountType != domain.TypeCashOnHand
}

func LoadTradeSources(ctx context.Context, ledger TradeLedger, account domain.AccountID, side domain.TradeSide) (sources TradeSources, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		var readErr error
		sources, readErr = tradeSources(ctx, ledger, account, side)
		return readErr
	})
	return sources, err
}

func tradeSources(ctx context.Context, ledger TradeLedger, id domain.AccountID, side domain.TradeSide) (TradeSources, error) {
	if side != domain.TradeBuy && side != domain.TradeSell {
		return TradeSources{}, &domain.Error{Code: domain.ErrInvalidTrade, Field: "side"}
	}
	origin, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return TradeSources{}, err
	}
	account, err := cashMovementAccount(ctx, ledger, id, "settlementAccountId")
	if err != nil {
		return TradeSources{}, err
	}
	if !TradeAccount(account) {
		return TradeSources{}, &domain.Error{Code: domain.ErrValidation, Field: "settlementAccountId"}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{id})
	if err != nil {
		return TradeSources{}, err
	}
	if side == domain.TradeBuy {
		values, err := ledger.AccountValuations(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
		if err != nil {
			return TradeSources{}, err
		}
		// The pilot uses the account's default-currency sleeve. Historical
		// spendable cash at the chosen time is checked by the trade command.
		if len(values) != 1 || !slices.ContainsFunc(values[0].Components, func(row domain.ValuationComponent) bool {
			return row.InstrumentID == nil && row.NativeCurrency == account.DefaultCurrency && row.NativeAmount != "" && row.NativeAmount != "0"
		}) {
			return TradeSources{}, &domain.Error{Code: domain.ErrInsufficientBalance, Field: "settlementCash"}
		}
	}
	instruments, err := ledger.ListInstruments(ctx, false)
	if err != nil {
		return TradeSources{}, err
	}
	managed, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return TradeSources{}, err
	}
	active := make(map[domain.InstrumentID]domain.Holding)
	for _, holding := range holdings[id] {
		if holding.ArchivedAt == nil && (side == domain.TradeBuy || !holding.Quantity.IsZero()) {
			active[holding.InstrumentID] = holding
		}
	}
	existingOnly := side == domain.TradeSell || len(holdings[id]) != 0
	instruments = slices.DeleteFunc(instruments, func(i domain.Instrument) bool {
		_, owned := managed[i.ID.String()]
		_, held := active[i.ID]
		return owned || i.ArchivedAt != nil || i.QuoteCurrency != account.DefaultCurrency || (existingOnly && !held)
	})
	return TradeSources{Origin: origin, Location: location, Account: account, Instruments: instruments, Holdings: active}, nil
}

type TradeInput struct {
	Side       domain.TradeSide
	Account    domain.AccountID
	Instrument domain.InstrumentID
	Holding    domain.HoldingID
	Quantity   string
	Gross      string
	Fee        string
	Date       string
	Time       string
}

type ReviewedTrade struct {
	Preview               domain.ChangePreview
	UnitCost              domain.UnitPrice
	ExistingCost          *application.ExistingTradeCostPreview
	Timezone              *time.Location
	command               any
	mutation, hash, token string
}

func ReviewTrade(ctx context.Context, ledger TradeLedger, input TradeInput) (ReviewedTrade, error) {
	var review ReviewedTrade
	var existing bool
	err := ledger.WithRead(ctx, func(ctx context.Context) error {
		preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
			sources, err := tradeSources(ctx, ledger, input.Account, input.Side)
			if err != nil {
				return nil, err
			}
			if !slices.ContainsFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == input.Instrument }) {
				return nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId"}
			}
			holding, held := sources.Holdings[input.Instrument]
			if holding.ID != input.Holding {
				return nil, &domain.Error{Code: domain.ErrConflict, Field: "holdingId"}
			}
			existing = held
			request, err := tradeRequest(input, sources.Account.DefaultCurrency, holding.ID)
			if err != nil {
				return nil, err
			}
			request.MutationID = domain.NewMutationID().String()
			review.command, err = request.ToCommand(sources.Origin.HouseholdID, sources.Origin.Timezone)
			if err != nil {
				return nil, err
			}
			review.mutation, review.hash, err = request.MutationEnvelope()
			review.Timezone = sources.Location
			return review.command, err
		})
		if err != nil {
			return err
		}
		review.Preview, review.token = preview, token
		if existing {
			cost, err := ledger.PreviewExistingTradeCost(ctx, preview)
			if err != nil {
				return err
			}
			review.ExistingCost = &cost
		}
		return nil
	})
	if err != nil {
		return ReviewedTrade{}, err
	}
	if input.Side == domain.TradeSell {
		return review, nil
	}
	trade := review.Preview.Activity.TradeDetail
	if trade == nil {
		return ReviewedTrade{}, &domain.Error{Code: domain.ErrIntegrity, Field: "trade"}
	}
	basis, err := domain.ReplayCostBasis(nil, []domain.CostBasisEvent{{
		Kind: domain.CostBasisBuy, Quantity: trade.Quantity, UnitPrice: &trade.UnitPrice,
		Gross: &trade.Gross, Fee: trade.Fee, Currency: trade.Gross.Currency(),
	}})
	if err != nil {
		return ReviewedTrade{}, err
	}
	review.UnitCost = basis.Current.AverageUnitCost
	return review, nil
}

func tradeRequest(input TradeInput, currency domain.CurrencyCode, holding domain.HoldingID) (historyapi.ChangeCommandRequest, error) {
	var request historyapi.ChangeCommandRequest
	quantity, err := domain.ParseQuantity(strings.TrimSpace(input.Quantity))
	if err != nil || quantity.IsZero() {
		return request, &domain.Error{Code: domain.ErrValidation, Field: "quantity"}
	}
	gross, err := domain.ParseMoney(strings.TrimSpace(input.Gross), currency)
	if err != nil || gross.IsZero() {
		return request, &domain.Error{Code: domain.ErrValidation, Field: "gross"}
	}
	if strings.TrimSpace(input.Date) == "" || strings.TrimSpace(input.Time) == "" {
		return request, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
	}
	request = historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: string(input.Side), SettlementAccountID: input.Account.String(), HoldingID: holding.String(),
		InstrumentID: input.Instrument.String(), Quantity: quantity.Canonical(), Gross: gross.CanonicalAmount(), GrossCurrency: currency.String(),
		EffectiveLocalDate: input.Date, EffectiveLocalTime: input.Time,
	}
	if value := strings.TrimSpace(input.Fee); value != "" {
		fee, err := domain.ParseMoney(value, currency)
		if err != nil {
			return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "fee"}
		}
		request.Fee, request.FeeCurrency = fee.CanonicalAmount(), currency.String()
	}
	return request, nil
}

func (r ReviewedTrade) Commit(ctx context.Context, ledger TradeLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(ctx, r.command, r.mutation, r.hash, r.token)
}
