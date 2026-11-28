package history

import (
	"context"
	"slices"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// AmendableInvestment holds the native amounts and immutable recorded endpoints.
// A historical amendment uses the existing holding even if its quantity is now zero.
type AmendableInvestment struct {
	Kind       domain.ActivityKind
	Account    domain.AccountID
	Instrument domain.InstrumentID
	Currency   domain.CurrencyCode
	Quantity   string
	Amount     string
	Fee        string
	holding    domain.HoldingID
}

func InvestmentAmendment(activity domain.Activity) (AmendableInvestment, bool) {
	if activity.ReversesActivityID != nil || activity.ProductContext != nil {
		return AmendableInvestment{}, false
	}
	switch activity.Kind {
	case domain.ActivityBuy, domain.ActivitySell:
		return recordedTradeAmendment(activity)
	case domain.ActivityCashDividend:
		return recordedDividendAmendment(activity)
	default:
		return AmendableInvestment{}, false
	}
}

func recordedTradeAmendment(activity domain.Activity) (AmendableInvestment, bool) {
	trade, effects := activity.TradeDetail, activity.Effects
	if trade == nil || activity.DividendDetail != nil || activity.Reason != domain.ReasonPrincipal ||
		trade.HoldingID == "" || trade.InstrumentID == "" || len(effects) < 2 || len(effects) > 3 {
		return AmendableInvestment{}, false
	}
	side, quantityDirection, cashDirection := domain.TradeBuy, domain.EffectAdded, domain.EffectRemoved
	if activity.Kind == domain.ActivitySell {
		side, quantityDirection, cashDirection = domain.TradeSell, domain.EffectRemoved, domain.EffectAdded
	}
	quantity, principal := effects[0], effects[1]
	if trade.Side != side || quantity.Target != domain.EffectTargetHoldingQuantity || quantity.Role != domain.EffectRoleQuantity ||
		quantity.Direction != quantityDirection || quantity.Classification != domain.ClassificationTradePrincipal ||
		quantity.HoldingID == nil || *quantity.HoldingID != trade.HoldingID ||
		quantity.InstrumentID == nil || *quantity.InstrumentID != trade.InstrumentID ||
		quantity.Quantity == nil || quantity.Quantity.IsZero() || quantity.Quantity.Canonical() != trade.Quantity.Canonical() ||
		quantity.AccountID != nil || quantity.Money != nil || quantity.CostUnitPrice != nil ||
		!matchesInvestmentCash(principal, domain.EffectRolePrincipal, cashDirection, domain.ClassificationTradePrincipal, trade.Gross) {
		return AmendableInvestment{}, false
	}
	fee := ""
	if trade.Fee != nil {
		if trade.Fee.Currency() != trade.Gross.Currency() {
			return AmendableInvestment{}, false
		}
		fee = trade.Fee.CanonicalAmount()
	}
	if fee != "" && fee != "0" {
		if len(effects) != 3 || !matchesInvestmentCash(effects[2], domain.EffectRoleFee, domain.EffectRemoved, domain.ClassificationFee, *trade.Fee) ||
			*effects[2].AccountID != *principal.AccountID {
			return AmendableInvestment{}, false
		}
	} else if len(effects) != 2 {
		return AmendableInvestment{}, false
	}
	return AmendableInvestment{
		Kind: activity.Kind, Account: *principal.AccountID, Instrument: trade.InstrumentID, holding: trade.HoldingID,
		Currency: trade.Gross.Currency(), Quantity: trade.Quantity.Canonical(), Amount: trade.Gross.CanonicalAmount(), Fee: fee,
	}, true
}

func recordedDividendAmendment(activity domain.Activity) (AmendableInvestment, bool) {
	dividend, effects := activity.DividendDetail, activity.Effects
	if dividend == nil || activity.TradeDetail != nil || activity.Reason != domain.ReasonIncome ||
		dividend.HoldingID == "" || dividend.InstrumentID == "" || len(effects) != 1 ||
		!matchesInvestmentCash(effects[0], domain.EffectRoleAmount, domain.EffectAdded, domain.ClassificationIncome, dividend.Amount) {
		return AmendableInvestment{}, false
	}
	return AmendableInvestment{
		Kind: activity.Kind, Account: *effects[0].AccountID, Instrument: dividend.InstrumentID, holding: dividend.HoldingID,
		Currency: dividend.Amount.Currency(), Amount: dividend.Amount.CanonicalAmount(),
	}, true
}

func matchesInvestmentCash(effect domain.ActivityEffect, role domain.EffectRole, direction domain.EffectDirection, class domain.ActivityClassification, amount domain.Money) bool {
	return matchesCashEffect(effect, role, direction, class) && effect.Target == domain.EffectTargetAccountCash &&
		*effect.AccountID != "" && effect.CostUnitPrice == nil && effect.Quantity == nil &&
		effect.Money.Currency() == amount.Currency() && effect.Money.CanonicalAmount() == amount.CanonicalAmount()
}

// position verifies the exact holding and reconciliation linkage without using
// today's quantity as an eligibility gate for historical replay.
func (m AmendableInvestment) position(ctx context.Context, ledger DetailReader, id domain.ActivityID, rows []domain.AccountRecord, instruments []domain.Instrument) (PositionHolding, error) {
	index := slices.IndexFunc(rows, func(row domain.AccountRecord) bool {
		return row.Account.ID == m.Account && supportsCashMovement(row) &&
			row.Account.TrackingMode == domain.TrackingHoldings && row.Account.AccountType != domain.TypeCashOnHand
	})
	instrument := slices.IndexFunc(instruments, func(instrument domain.Instrument) bool { return instrument.ID == m.Instrument })
	if index < 0 || instrument < 0 {
		return PositionHolding{}, nil
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{m.Account})
	if err != nil {
		return PositionHolding{}, err
	}
	inventory := holdings[m.Account]
	holding := slices.IndexFunc(inventory, func(holding domain.Holding) bool {
		return holding.ID == m.holding && holding.AccountID == m.Account && holding.InstrumentID == m.Instrument
	})
	if holding < 0 {
		return PositionHolding{}, nil
	}
	linked, err := ledger.HasReconciliationActivityLink(ctx, m.Account, id)
	if err != nil || linked {
		return PositionHolding{}, err
	}
	return PositionHolding{Holding: inventory[holding], Account: rows[index].Account, Instrument: instruments[instrument]}, nil
}

// InvestmentReplacementInitial seeds the historical amendment from the original
// facts. Review supplies the original effective instant and its own mutation ID.
func InvestmentReplacementInitial(activity domain.Activity) (historyapi.ChangeCommandRequest, bool) {
	investment, ok := InvestmentAmendment(activity)
	if !ok {
		return historyapi.ChangeCommandRequest{}, false
	}
	input := AmendmentInput{Amount: investment.Amount, Quantity: investment.Quantity, Fee: investment.Fee}
	if activity.Note != nil {
		input.Note = *activity.Note
	}
	return investment.replacement(input), true
}

func (m AmendableInvestment) replacement(input AmendmentInput) historyapi.ChangeCommandRequest {
	note := strings.TrimSpace(input.Note)
	request := historyapi.ChangeCommandRequest{HoldingID: m.holding.String()}
	if note != "" {
		request.Note = &note
	}
	if m.Kind == domain.ActivityCashDividend {
		request.Kind = historyapi.ChangeCashDividend
		request.Amount, request.Currency = strings.TrimSpace(input.Amount), m.Currency.String()
		return request
	}
	request.Kind, request.Side = historyapi.ChangeTrade, string(domain.TradeBuy)
	if m.Kind == domain.ActivitySell {
		request.Side = string(domain.TradeSell)
	}
	request.SettlementAccountID, request.InstrumentID = m.Account.String(), m.Instrument.String()
	request.Quantity, request.Gross, request.GrossCurrency = strings.TrimSpace(input.Quantity), strings.TrimSpace(input.Amount), m.Currency.String()
	request.Fee, request.FeeCurrency = strings.TrimSpace(input.Fee), m.Currency.String()
	return request
}

func (m AmendableInvestment) unchanged(command any, originalNote *string) bool {
	sameNote := func(note *string) bool {
		text := func(value *string) string {
			if value == nil {
				return ""
			}
			return strings.TrimSpace(*value)
		}
		return text(originalNote) == text(note)
	}
	switch input := command.(type) {
	case domain.TradeInput:
		fee := ""
		if input.Fee != nil && input.Fee.Currency() != m.Currency {
			return false
		}
		if input.Fee != nil && !input.Fee.IsZero() {
			fee = input.Fee.CanonicalAmount()
		}
		return m.sameTradePosition(input) && m.Quantity == input.Quantity.Canonical() && m.Amount == input.Gross.CanonicalAmount() &&
			(m.Fee == fee || (m.Fee == "0" && fee == "")) && sameNote(input.Note)
	case domain.CashDividendInput:
		return m.Kind == domain.ActivityCashDividend && m.holding == input.HoldingID && m.Currency == input.Amount.Currency() &&
			m.Amount == input.Amount.CanonicalAmount() && sameNote(input.Note)
	default:
		return false
	}
}

func (m AmendableInvestment) sameTradePosition(input domain.TradeInput) bool {
	sideMatches := (m.Kind == domain.ActivityBuy && input.Side == domain.TradeBuy) ||
		(m.Kind == domain.ActivitySell && input.Side == domain.TradeSell)
	return sideMatches && m.Account == input.SettlementAccountID && m.holding == input.HoldingID &&
		m.Instrument == input.InstrumentID && m.Currency == input.Gross.Currency()
}

func (m AmendableInvestment) restrictedTrade(position PositionHolding) bool {
	return m.Kind != domain.ActivityCashDividend && (position.Account.ArchivedAt != nil ||
		position.Holding.ArchivedAt != nil || position.Instrument.ArchivedAt != nil)
}

func (m AmendableInvestment) validateArchivedTrade(position PositionHolding, command any) error {
	if !m.restrictedTrade(position) {
		return nil
	}
	trade, ok := command.(domain.TradeInput)
	if !ok || !m.sameTradePosition(trade) || m.Quantity != trade.Quantity.Canonical() {
		return &domain.Error{Code: domain.ErrConflict, Field: "holdingId"}
	}
	return nil
}
