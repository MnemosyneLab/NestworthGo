package application

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func parseOptionalMoney(field string, value *string, currency domain.CurrencyCode) (*domain.Money, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	money, err := domain.ParseMoney(strings.TrimSpace(*value), currency)
	if err != nil {
		return nil, err
	}
	if money.Amount().IsNegative() {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: field, Message: "must not be negative"}
	}
	return &money, nil
}

func parseRequiredMoney(field, value string, currency domain.CurrencyCode) (domain.Money, error) {
	if strings.TrimSpace(value) == "" {
		return domain.Money{}, &domain.Error{Code: domain.ErrValidation, Field: field, Message: "is required"}
	}
	money, err := domain.ParseMoney(strings.TrimSpace(value), currency)
	if err != nil {
		return domain.Money{}, err
	}
	return money, nil
}

func parseEffectiveAt(value string, origin *domain.HistoryOrigin, now time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return now.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, strings.TrimSpace(value))
		if err != nil {
			return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "must be an RFC3339 timestamp"}
		}
	}
	parsed = parsed.UTC()
	if origin != nil && parsed.Before(origin.StartedAt.UTC()) {
		return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "change time cannot precede the Starting point"}
	}
	if parsed.After(now.UTC()) {
		return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "change time cannot be in the future"}
	}
	return parsed, nil
}

func parseAnnualRate(input ProductTermsInput) (*domain.AnnualRate, error) {
	if input.AnnualRatePercent != nil && strings.TrimSpace(*input.AnnualRatePercent) != "" {
		rate, err := domain.ParseAnnualRatePercent(*input.AnnualRatePercent)
		if err != nil {
			return nil, err
		}
		return &rate, nil
	}
	if input.AnnualRate != nil && strings.TrimSpace(*input.AnnualRate) != "" {
		rate, err := domain.ParseAnnualRateRatio(*input.AnnualRate)
		if err != nil {
			return nil, err
		}
		return &rate, nil
	}
	return nil, nil
}

func resolveProductTerms(input ProductTermsInput, currency domain.CurrencyCode, principal domain.Money) (domain.ProductKind, domain.InterestMode, *domain.AnnualRate, *domain.Money, *string, error) {
	kind, err := domain.ParseProductKind(input.Kind)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	mode, err := domain.ParseInterestMode(input.InterestMode)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	rate, err := parseAnnualRate(input)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	maturityInterest, err := parseOptionalMoney("maturityInterest", input.MaturityInterest, currency)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	_ = principal
	return kind, mode, rate, maturityInterest, input.InterestPaidThroughOn, nil
}

func buildManagedInstrument(household domain.HouseholdID, name string, currency domain.CurrencyCode, when time.Time) (domain.Instrument, error) {
	return domain.NewInstrument(domain.InstrumentInput{
		HouseholdID:   household,
		Name:          name,
		Type:          domain.InstrumentBankInvestmentProduct,
		QuoteCurrency: currency,
		QuoteSource:   domain.QuoteSourceManual,
	}, when)
}

func buildManagedQuote(instrument domain.Instrument, amount domain.Money, when time.Time) (domain.InstrumentQuote, error) {
	price, err := domain.ParseUnitPrice(amount.CanonicalAmount())
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	return domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{
		UnitPrice:  price,
		Currency:   amount.Currency(),
		SourceKind: domain.QuoteSourceManual,
		QuotedAt:   when,
	}, when)
}

func zeroQuantity() domain.Quantity {
	quantity, err := domain.ParseQuantity("0")
	if err != nil {
		panic(err)
	}
	return quantity
}

func oneQuantity() domain.Quantity {
	quantity, err := domain.ParseQuantity(domain.ManagedHoldingOpenQuantity)
	if err != nil {
		panic(err)
	}
	return quantity
}

func addInstrumentToState(state domain.ChangeState, instrument domain.Instrument, holding domain.Holding) domain.ChangeState {
	state = cloneWorkingState(state)
	state.Instruments[instrument.ID] = instrument
	state.Holdings[holding.ID] = domain.ChangeHoldingState{
		ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID,
		InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: holding.Quantity,
	}
	return state
}

func cloneWorkingState(state domain.ChangeState) domain.ChangeState {
	_, _, _ = domain.ApplyEffects(state, nil)
	cloned, _, err := domain.ApplyEffects(state, []domain.ActivityEffect{})
	if err != nil {
		return state
	}
	return cloned
}

func applyPreview(state domain.ChangeState, preview domain.ChangePreview) (domain.ChangeState, error) {
	next, _, err := domain.ApplyEffects(state, preview.Effects)
	return next, err
}

func optionalZeroFee(fee *domain.Money) *domain.Money {
	if fee == nil || fee.IsZero() {
		return nil
	}
	return fee
}

func unitCostFromTotal(total domain.Money) (*domain.UnitPrice, error) {
	price, err := domain.ParseUnitPrice(total.CanonicalAmount())
	if err != nil {
		return nil, err
	}
	return &price, nil
}

func accountFromState(snapshot domain.PortfolioSnapshot, accountID domain.AccountID) (domain.AccountRecord, bool) {
	for _, record := range snapshot.Accounts {
		if record.Account.ID == accountID {
			return record, true
		}
	}
	return domain.AccountRecord{}, false
}

func cashAfter(state domain.ChangeState, accountID domain.AccountID, currency domain.CurrencyCode) (domain.Money, error) {
	return currentCashAmount(state, accountID, currency)
}

func policyFromInput(household domain.HouseholdID, source domain.LiquiditySourceRef, input ProductPolicyInput, currency domain.CurrencyCode, managed bool, now time.Time) (domain.LiquidityPolicy, error) {
	access, err := domain.ParseAccessKind(input.AccessKind)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	early, err := domain.ParseEarlyKind(input.EarlyKind)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	var dayBasis *domain.DayBasis
	if input.DayBasis != nil && strings.TrimSpace(*input.DayBasis) != "" {
		parsed, err := domain.ParseDayBasis(*input.DayBasis)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		dayBasis = &parsed
	}
	var earlyBasis *domain.DayBasis
	if input.EarlyDayBasis != nil && strings.TrimSpace(*input.EarlyDayBasis) != "" {
		parsed, err := domain.ParseDayBasis(*input.EarlyDayBasis)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		earlyBasis = &parsed
	}
	var earlyMode *domain.EarlyAmountMode
	if input.EarlyAmountMode != nil && strings.TrimSpace(*input.EarlyAmountMode) != "" {
		parsed, err := domain.ParseEarlyAmountMode(*input.EarlyAmountMode)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		earlyMode = &parsed
	}
	fee, err := parseOptionalMoney("normalExitFee", input.NormalExitFee, currency)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	earlyFee, err := parseOptionalMoney("earlyFee", input.EarlyFee, currency)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	earlyGross, err := parseOptionalMoney("earlyGrossAmount", input.EarlyGrossAmount, currency)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	cap, err := parseOptionalMoney("accessibleAmountCap", input.AccessibleAmountCap, currency)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	policy := domain.LiquidityPolicy{
		AccessibleAmountCap: cap,
		ID:                  domain.NewLiquidityPolicyID(), HouseholdID: household, Source: source,
		AccessKind: access, UnlockOn: input.UnlockOn, SettlementDays: input.SettlementDays, DayBasis: dayBasis,
		ReceiptOnOverride: input.ReceiptOnOverride, NormalExitFee: fee, EarlyKind: early,
		EarlySettlementDays: input.EarlySettlementDays, EarlyDayBasis: earlyBasis, EarlyFee: earlyFee,
		EarlyAmountMode: earlyMode, EarlyGrossAmount: earlyGross, Note: input.Note, Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := policy.Validate(managed); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	return policy, nil
}

func contractPolicyForOpen(household domain.HouseholdID, accountID domain.AccountID, holdingID domain.HoldingID, kind domain.ProductKind, maturity *string, input ProductPolicyInput, currency domain.CurrencyCode, now time.Time) (domain.LiquidityPolicy, error) {
	source := domain.HoldingSourceRef(accountID, holdingID)
	if input.AccessKind == "" {
		input.AccessKind = string(domain.AccessOnDate)
	}
	if input.EarlyKind == "" {
		input.EarlyKind = string(domain.EarlyNotAllowed)
	}
	if kind == domain.ProductTermDeposit && input.UnlockOn == nil && maturity != nil {
		copy := *maturity
		input.UnlockOn = &copy
	}
	policy, err := policyFromInput(household, source, input, currency, true, now)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if err := policy.ValidateProductDates(kind, maturity); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	return policy, nil
}

func moneyPtr(value domain.Money) *domain.Money { return &value }

func maxMoney(left, right domain.Money) domain.Money {
	if left.Amount().GreaterThan(right.Amount()) {
		return left
	}
	return right
}

func decimalZero() decimal.Decimal { return decimal.Zero }

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func resolveProductEffectiveTime(timestamp, date, clock string, origin *domain.HistoryOrigin, now time.Time) (string, error) {
	if date == "" && clock == "" {
		return timestamp, nil
	}
	if timestamp != "" {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "use either local time or timestamp"}
	}
	when, err := domain.ResolveLocalDateTime(date, clock, origin.Timezone)
	if err != nil {
		return "", err
	}
	value := when.UTC().Format(time.RFC3339Nano)
	if _, err = parseEffectiveAt(value, origin, now); err != nil {
		return "", err
	}
	return value, nil
}
