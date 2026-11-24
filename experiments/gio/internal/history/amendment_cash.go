package history

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// AmendableCash describes the recorded native endpoints and amounts. Both the
// detail and guarded preview use the same eligibility checks.
type AmendableCash struct {
	Movement  CashMovement
	Principal domain.ActivityEffect
	Received  domain.ActivityEffect
	Fee       domain.ActivityEffect
}

type amendmentAccounts struct {
	source, destination domain.Account
}

func (a amendmentAccounts) active() bool {
	return a.source.ArchivedAt == nil && a.destination.ArchivedAt == nil
}

func (m AmendableCash) Paired() bool { return m.Received.Money != nil }

func (m AmendableCash) CrossCurrency() bool {
	return m.Paired() && m.Principal.Money.Currency() != m.Received.Money.Currency()
}

func CashAmendment(activity domain.Activity) (AmendableCash, bool) {
	movement, supported := CashMovementOf(activity.Kind, activity.Reason)
	if !supported || activity.ReversesActivityID != nil || activity.ProductContext != nil || len(activity.Effects) == 0 {
		return AmendableCash{}, false
	}
	effects := activity.Effects
	m := AmendableCash{Movement: movement, Principal: effects[0]}
	switch movement {
	case CashContribution:
		return m, len(effects) == 1 && matchesCashEffect(m.Principal, domain.EffectRoleAmount, domain.EffectAdded, domain.ClassificationExternalInflow)
	case CashWithdrawal:
		return m, len(effects) == 1 && matchesCashEffect(m.Principal, domain.EffectRoleAmount, domain.EffectRemoved, domain.ClassificationExternalOutflow)
	}
	if len(effects) < 2 || len(effects) > 3 || activity.Reason != domain.ReasonOther {
		return AmendableCash{}, false
	}
	m.Received = effects[1]
	if !matchesCashEffect(m.Principal, domain.EffectRoleTransferFrom, domain.EffectRemoved, domain.ClassificationInternalTransfer) ||
		!matchesCashEffect(m.Received, domain.EffectRoleTransferTo, domain.EffectAdded, domain.ClassificationInternalTransfer) {
		return AmendableCash{}, false
	}
	sameAccount := *m.Principal.AccountID == *m.Received.AccountID
	if movement == CashConversion {
		if !sameAccount || !m.CrossCurrency() || m.Principal.Target != domain.EffectTargetAccountCash || m.Received.Target != domain.EffectTargetAccountCash {
			return AmendableCash{}, false
		}
	} else if sameAccount || (!m.CrossCurrency() && m.Principal.Money.CanonicalAmount() != m.Received.Money.CanonicalAmount()) {
		return AmendableCash{}, false
	}
	if len(effects) == 3 {
		m.Fee = effects[2]
		if !matchesCashEffect(m.Fee, domain.EffectRoleFee, domain.EffectRemoved, domain.ClassificationFee) ||
			*m.Fee.AccountID != *m.Principal.AccountID || m.Fee.Target != m.Principal.Target || m.Fee.Money.Currency() != m.Principal.Money.Currency() {
			return AmendableCash{}, false
		}
	}
	return m, true
}

func matchesCashEffect(effect domain.ActivityEffect, role domain.EffectRole, direction domain.EffectDirection, class domain.ActivityClassification) bool {
	return effect.AccountID != nil && effect.Money != nil && !effect.Money.IsZero() && effect.HoldingID == nil && effect.InstrumentID == nil &&
		(effect.Target == domain.EffectTargetAccountValue || effect.Target == domain.EffectTargetAccountCash) &&
		effect.Role == role && effect.Direction == direction && effect.Classification == class
}

func eligibleAmendmentAccount(rows []domain.AccountRecord, effect domain.ActivityEffect) (domain.Account, bool) {
	for _, row := range rows {
		account := row.Account
		if account.ID != *effect.AccountID || !supportsCashMovement(row) {
			continue
		}
		if account.TrackingMode == domain.TrackingHoldings && effect.Target == domain.EffectTargetAccountCash {
			_, err := movementCurrency(account, effect.Money.Currency(), "currency")
			return account, err == nil
		}
		if account.TrackingMode == domain.TrackingBalance && effect.Target == domain.EffectTargetAccountValue &&
			account.DefaultCurrency == effect.Money.Currency() {
			return account, true
		}
	}
	return domain.Account{}, false
}

func (m AmendableCash) accounts(rows []domain.AccountRecord) (amendmentAccounts, bool) {
	source, ok := eligibleAmendmentAccount(rows, m.Principal)
	if !ok {
		return amendmentAccounts{}, false
	}
	accounts := amendmentAccounts{source: source}
	if m.Paired() {
		accounts.destination, ok = eligibleAmendmentAccount(rows, m.Received)
	}
	return accounts, ok
}

func (m AmendableCash) replacement(amounts AmendmentAmounts, accounts amendmentAccounts) (historyapi.ChangeCommandRequest, error) {
	input := CashMovementInput{Movement: m.Movement, Currency: m.Principal.Money.Currency(), Amount: amounts.Amount}
	if m.Paired() {
		input.ToCurrency, input.Received, input.Fee = m.Received.Money.Currency(), amounts.Received, amounts.Fee
	}
	return cashMovementRequest(input, accounts.source, accounts.destination, m.Movement == CashTransfer)
}

func (m AmendableCash) unchanged(request historyapi.ChangeCommandRequest) bool {
	principal := m.Principal.Money.CanonicalAmount()
	if !m.Paired() {
		return request.Amount == principal
	}
	fee := ""
	if m.Fee.Money != nil {
		fee = m.Fee.Money.CanonicalAmount()
	}
	received := m.Received.Money.CanonicalAmount()
	if m.Movement == CashConversion {
		return request.Sold == principal && request.Bought == received && request.Fee == fee
	}
	// The shared transfer correction contract interprets a blank fee as
	// "keep the original" and explicit zero as "remove it".
	actualFee := request.Fee
	switch actualFee {
	case "":
		actualFee = fee
	case "0":
		actualFee = ""
	}
	return request.Sent == principal && request.Received == received && actualFee == fee
}
