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

type InvestmentLedger interface {
	CashMovementLedger
	WithRead(context.Context, func(context.Context) error) error
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
	PreviewExistingTradeCost(context.Context, domain.ChangePreview) (application.ExistingTradeCostPreview, error)
	ValidateReconciliationCommandLink(context.Context, *domain.ReconciliationLink, any, domain.ChangePreview) error
}

type InvestmentSources struct {
	Origin      domain.HistoryOrigin
	Location    *time.Location
	Account     domain.Account
	Instruments []domain.Instrument
	Holdings    map[domain.InstrumentID]domain.Holding
}

func TradeAccount(account domain.Account) bool {
	return acceptsHoldingsCash(account) && account.AccountType != domain.TypeCashOnHand
}

func LoadInvestmentSources(ctx context.Context, ledger InvestmentLedger, account domain.AccountID, kind domain.ActivityKind) (sources InvestmentSources, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		var readErr error
		sources, readErr = investmentSources(ctx, ledger, account, kind)
		return readErr
	})
	return sources, err
}

func investmentSources(ctx context.Context, ledger InvestmentLedger, id domain.AccountID, kind domain.ActivityKind) (InvestmentSources, error) {
	if kind != domain.ActivityBuy && kind != domain.ActivitySell && kind != domain.ActivityCashDividend && kind != domain.ActivityPositionTransfer {
		return InvestmentSources{}, &domain.Error{Code: domain.ErrInvalidTrade, Field: "side"}
	}
	origin, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return InvestmentSources{}, err
	}
	account, err := cashMovementAccount(ctx, ledger, id, "settlementAccountId")
	if err != nil {
		return InvestmentSources{}, err
	}
	if !TradeAccount(account) {
		return InvestmentSources{}, &domain.Error{Code: domain.ErrValidation, Field: "settlementAccountId"}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{id})
	if err != nil {
		return InvestmentSources{}, err
	}
	instruments, err := ledger.ListInstruments(ctx, false)
	if err != nil {
		return InvestmentSources{}, err
	}
	managed, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return InvestmentSources{}, err
	}
	active := make(map[domain.InstrumentID]domain.Holding)
	for _, holding := range holdings[id] {
		if holding.ArchivedAt == nil && (kind != domain.ActivitySell || !holding.Quantity.IsZero()) {
			active[holding.InstrumentID] = holding
		}
	}
	instruments = slices.DeleteFunc(instruments, func(i domain.Instrument) bool {
		_, owned := managed[i.ID.String()]
		_, held := active[i.ID]
		if owned || i.ArchivedAt != nil {
			return true
		}
		switch kind {
		case domain.ActivityPositionTransfer:
			return held
		case domain.ActivitySell:
			return !held
		case domain.ActivityCashDividend:
			return !held
		}
		return false
	})
	return InvestmentSources{Origin: origin, Location: location, Account: account, Instruments: instruments, Holdings: active}, nil
}

type TradeInput struct {
	Side        domain.TradeSide
	Account     domain.AccountID
	Instrument  domain.InstrumentID
	Holding     domain.HoldingID
	Currency    domain.CurrencyCode
	Quantity    string
	Gross       string
	Fee         string
	Note        string
	Date        string
	Time        string
	EffectiveAt time.Time
	Link        *domain.ReconciliationLink
}

type ReviewedInvestment struct {
	Preview               domain.ChangePreview
	UnitCost              domain.UnitPrice
	ExistingCost          *application.ExistingTradeCostPreview
	Timezone              *time.Location
	command               any
	mutation, hash, token string
	link                  *domain.ReconciliationLink
}

func ReviewTrade(ctx context.Context, ledger InvestmentLedger, input TradeInput) (ReviewedInvestment, error) {
	var review ReviewedInvestment
	var kind domain.ActivityKind
	switch input.Side {
	case domain.TradeBuy:
		kind = domain.ActivityBuy
	case domain.TradeSell:
		kind = domain.ActivitySell
	default:
		return review, &domain.Error{Code: domain.ErrInvalidTrade, Field: "side"}
	}
	var existing bool
	err := ledger.WithRead(ctx, func(ctx context.Context) error {
		preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
			sources, err := investmentSources(ctx, ledger, input.Account, kind)
			if err != nil {
				return nil, err
			}
			at := slices.IndexFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == input.Instrument })
			if at < 0 {
				return nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId"}
			}
			currency := sources.Instruments[at].QuoteCurrency
			if currency != input.Currency {
				return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "grossCurrency"}
			}
			holding, held := sources.Holdings[input.Instrument]
			if holding.ID != input.Holding {
				return nil, &domain.Error{Code: domain.ErrConflict, Field: "holdingId"}
			}
			existing = held
			request, err := tradeRequest(input, currency, holding.ID)
			if err != nil {
				return nil, err
			}
			return review.bind(sources.Origin, sources.Location, request)
		})
		if err != nil {
			return err
		}
		review.Preview, review.token = preview, token
		if err := review.validateLink(ctx, ledger); err != nil {
			return err
		}
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
		return ReviewedInvestment{}, err
	}
	if input.Side == domain.TradeSell {
		return review, nil
	}
	trade := review.Preview.Activity.TradeDetail
	if trade == nil {
		return ReviewedInvestment{}, &domain.Error{Code: domain.ErrIntegrity, Field: "trade"}
	}
	basis, err := domain.ReplayCostBasis(nil, []domain.CostBasisEvent{{
		Kind: domain.CostBasisBuy, Quantity: trade.Quantity, UnitPrice: &trade.UnitPrice,
		Gross: &trade.Gross, Fee: trade.Fee, Currency: trade.Gross.Currency(),
	}})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	review.UnitCost = basis.Current.AverageUnitCost
	return review, nil
}

// bind freezes the same request identity used by Wails Go. The plan and its
// token stay together across retries, including a lost response after commit.
func (r *ReviewedInvestment) bind(origin domain.HistoryOrigin, location *time.Location, request historyapi.ChangeCommandRequest) (any, error) {
	request.MutationID = domain.NewMutationID().String()
	var err error
	r.command, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
	if err != nil {
		return nil, err
	}
	r.mutation, r.hash, err = request.MutationEnvelope()
	if request.ReconciliationLink != nil {
		link := *request.ReconciliationLink
		r.link = &link
	}
	r.Timezone = location
	return r.command, err
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
	if input.EffectiveAt.IsZero() && (strings.TrimSpace(input.Date) == "" || strings.TrimSpace(input.Time) == "") {
		return request, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
	}
	request = historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, ReconciliationLink: input.Link, Side: string(input.Side), SettlementAccountID: input.Account.String(), HoldingID: holding.String(),
		InstrumentID: input.Instrument.String(), Quantity: quantity.Canonical(), Gross: gross.CanonicalAmount(), GrossCurrency: currency.String(),
	}
	if note := strings.TrimSpace(input.Note); note != "" {
		request.Note = &note
	}
	if input.EffectiveAt.IsZero() {
		request.EffectiveLocalDate, request.EffectiveLocalTime = input.Date, input.Time
	} else {
		request.EffectiveAt = input.EffectiveAt.UTC().Format(time.RFC3339Nano)
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

func (r ReviewedInvestment) validateLink(ctx context.Context, ledger InvestmentLedger) error {
	if r.link == nil {
		return nil
	}
	return ledger.ValidateReconciliationCommandLink(ctx, r.link, r.command, r.Preview)
}

func (r ReviewedInvestment) Commit(ctx context.Context, ledger InvestmentLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(domain.WithReconciliationLink(ctx, r.link), r.command, r.mutation, r.hash, r.token)
}
