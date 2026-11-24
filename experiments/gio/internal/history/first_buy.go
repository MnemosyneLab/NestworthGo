package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type FirstBuyLedger interface {
	CashMovementLedger
	WithRead(context.Context, func(context.Context) error) error
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	AccountValuations(context.Context, domain.AccountFilter) ([]domain.AccountValuation, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
}

type FirstBuySources struct {
	Origin      domain.HistoryOrigin
	Location    *time.Location
	Account     domain.Account
	Instruments []domain.Instrument
}

func FirstBuyAccount(account domain.Account) bool {
	return acceptsHoldingsCash(account) && account.AccountType != domain.TypeCashOnHand
}

func LoadFirstBuySources(ctx context.Context, ledger FirstBuyLedger, account domain.AccountID) (sources FirstBuySources, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		var readErr error
		sources, readErr = firstBuySources(ctx, ledger, account)
		return readErr
	})
	return sources, err
}

func firstBuySources(ctx context.Context, ledger FirstBuyLedger, id domain.AccountID) (FirstBuySources, error) {
	origin, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return FirstBuySources{}, err
	}
	account, err := cashMovementAccount(ctx, ledger, id, "settlementAccountId")
	if err != nil {
		return FirstBuySources{}, err
	}
	if !FirstBuyAccount(account) {
		return FirstBuySources{}, &domain.Error{Code: domain.ErrValidation, Field: "settlementAccountId"}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{id})
	if err != nil {
		return FirstBuySources{}, err
	}
	if len(holdings[id]) != 0 {
		return FirstBuySources{}, &domain.Error{Code: domain.ErrValidation, Field: "holdings"}
	}
	values, err := ledger.AccountValuations(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
	if err != nil {
		return FirstBuySources{}, err
	}
	// The pilot uses the account's default-currency sleeve. Historical
	// spendable cash at the chosen time is checked by the trade command.
	if len(values) != 1 || !slices.ContainsFunc(values[0].Components, func(row domain.ValuationComponent) bool {
		return row.InstrumentID == nil && row.NativeCurrency == account.DefaultCurrency && row.NativeAmount != "" && row.NativeAmount != "0"
	}) {
		return FirstBuySources{}, &domain.Error{Code: domain.ErrInsufficientBalance, Field: "settlementCash"}
	}
	instruments, err := ledger.ListInstruments(ctx, false)
	if err != nil {
		return FirstBuySources{}, err
	}
	managed, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return FirstBuySources{}, err
	}
	instruments = slices.DeleteFunc(instruments, func(i domain.Instrument) bool {
		_, owned := managed[i.ID.String()]
		return owned || i.ArchivedAt != nil || i.QuoteCurrency != account.DefaultCurrency
	})
	return FirstBuySources{Origin: origin, Location: location, Account: account, Instruments: instruments}, nil
}

type FirstBuyInput struct {
	Account    domain.AccountID
	Instrument domain.InstrumentID
	Quantity   string
	Gross      string
	Fee        string
	Date       string
	Time       string
}

type ReviewedFirstBuy struct {
	Preview               domain.ChangePreview
	UnitCost              domain.UnitPrice
	Timezone              *time.Location
	command               any
	mutation, hash, token string
}

func ReviewFirstBuy(ctx context.Context, ledger FirstBuyLedger, input FirstBuyInput) (ReviewedFirstBuy, error) {
	var review ReviewedFirstBuy
	preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		sources, err := firstBuySources(ctx, ledger, input.Account)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == input.Instrument }) {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId"}
		}
		request, err := firstBuyRequest(input, sources.Account.DefaultCurrency)
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
		return ReviewedFirstBuy{}, err
	}
	trade := preview.Activity.TradeDetail
	if trade == nil {
		return ReviewedFirstBuy{}, &domain.Error{Code: domain.ErrIntegrity, Field: "trade"}
	}
	basis, err := domain.ReplayCostBasis(nil, []domain.CostBasisEvent{{
		Kind: domain.CostBasisBuy, Quantity: trade.Quantity, UnitPrice: &trade.UnitPrice,
		Gross: &trade.Gross, Fee: trade.Fee, Currency: trade.Gross.Currency(),
	}})
	if err != nil {
		return ReviewedFirstBuy{}, err
	}
	review.Preview, review.UnitCost, review.token = preview, basis.Current.AverageUnitCost, token
	return review, nil
}

func firstBuyRequest(input FirstBuyInput, currency domain.CurrencyCode) (historyapi.ChangeCommandRequest, error) {
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
		Kind: historyapi.ChangeTrade, Side: string(domain.TradeBuy), SettlementAccountID: input.Account.String(),
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

func (r ReviewedFirstBuy) Commit(ctx context.Context, ledger FirstBuyLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(ctx, r.command, r.mutation, r.hash, r.token)
}
