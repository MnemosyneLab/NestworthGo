package history

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestInvestmentAmendmentKeepsExactTradeEndpointsAndAmounts(t *testing.T) {
	account, holding, instrument := domain.NewAccountID(), domain.NewHoldingID(), domain.NewInstrumentID()
	quantity, err := domain.ParseQuantity("1.12345678")
	if err != nil {
		t.Fatal(err)
	}
	gross, fee := amendmentMoney(t, "12.3456"), amendmentMoney(t, "0.1234")
	activity := domain.Activity{
		Kind: domain.ActivitySell, Reason: domain.ReasonPrincipal,
		TradeDetail: &domain.TradeDetail{Side: domain.TradeSell, InstrumentID: instrument, HoldingID: holding, Quantity: quantity, Gross: gross, Fee: &fee},
		Effects: []domain.ActivityEffect{
			{Role: domain.EffectRoleQuantity, Direction: domain.EffectRemoved, Target: domain.EffectTargetHoldingQuantity,
				Classification: domain.ClassificationTradePrincipal, HoldingID: &holding, InstrumentID: &instrument, Quantity: &quantity},
			{Role: domain.EffectRolePrincipal, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash,
				Classification: domain.ClassificationTradePrincipal, AccountID: &account, Money: &gross},
			{Role: domain.EffectRoleFee, Direction: domain.EffectRemoved, Target: domain.EffectTargetAccountCash,
				Classification: domain.ClassificationFee, AccountID: &account, Money: &fee},
		},
	}
	model, ok := InvestmentAmendment(activity)
	if !ok || model.Quantity != "1.12345678" || model.Amount != "12.3456" || model.Fee != "0.1234" {
		t.Fatalf("trade evidence lost precision: %+v", model)
	}
	request := model.replacement(AmendmentInput{Quantity: "1.12345678", Amount: "12.3456", Fee: "0.1234"})
	command, err := request.ToCommand(domain.NewHouseholdID())
	if err != nil {
		t.Fatal(err)
	}
	trade := command.(domain.TradeInput)
	if trade.HoldingID != holding || trade.InstrumentID != instrument || trade.SettlementAccountID != account ||
		trade.Side != domain.TradeSell || !model.unchanged(command, nil) {
		t.Fatalf("replacement lost its recorded scope: %+v", trade)
	}
	archivedAt := time.Unix(50, 0)
	archivedPosition := PositionHolding{Holding: domain.Holding{ArchivedAt: &archivedAt}}
	if err := model.validateArchivedTrade(archivedPosition, trade); err != nil {
		t.Fatal(err)
	}
	trade.Side = domain.TradeBuy
	if model.unchanged(trade, nil) || model.validateArchivedTrade(archivedPosition, trade) == nil {
		t.Fatal("changing side was treated as the original archived trade")
	}
	trade.Side, trade.SettlementAccountID = domain.TradeSell, domain.NewAccountID()
	if model.unchanged(trade, nil) || model.validateArchivedTrade(archivedPosition, trade) == nil {
		t.Fatal("changing settlement account was treated as the original archived trade")
	}
	request = model.replacement(AmendmentInput{Quantity: "1.12345678", Amount: "12.3456", Note: "fee removed"})
	command, err = request.ToCommand(domain.NewHouseholdID())
	if err != nil {
		t.Fatal(err)
	}
	trade = command.(domain.TradeInput)
	if trade.Fee != nil || trade.Note == nil || *trade.Note != "fee removed" || model.unchanged(command, nil) {
		t.Fatalf("clearing the optional trade fee lost a change: %+v", trade)
	}
	wrongAccount := domain.NewAccountID()
	activity.Effects[2].AccountID = &wrongAccount
	if _, ok := InvestmentAmendment(activity); ok {
		t.Fatal("a separate cash leg was accepted as the trade fee")
	}
}

func TestInvestmentAmendmentDistinguishesDividendsFromFeeAndTax(t *testing.T) {
	account, holding, instrument := domain.NewAccountID(), domain.NewHoldingID(), domain.NewInstrumentID()
	amount := amendmentMoney(t, "1.2345")
	activity := domain.Activity{
		Kind: domain.ActivityCashDividend, Reason: domain.ReasonIncome,
		DividendDetail: &domain.DividendDetail{HoldingID: holding, InstrumentID: instrument, Amount: amount},
		Effects: []domain.ActivityEffect{{Role: domain.EffectRoleAmount, Direction: domain.EffectAdded,
			Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome, AccountID: &account, Money: &amount}},
	}
	model, ok := InvestmentAmendment(activity)
	if !ok || model.Amount != "1.2345" || model.Currency != "USD" {
		t.Fatalf("dividend evidence lost its recorded amount: %+v", model)
	}
	otherCurrency, err := domain.ParseMoney("1.2345", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	if model.unchanged(domain.CashDividendInput{HoldingID: holding, Amount: otherCurrency}, nil) ||
		model.unchanged(domain.CashDividendInput{HoldingID: domain.NewHoldingID(), Amount: amount}, nil) {
		t.Fatal("changing the dividend holding or payment currency was treated as an unchanged amount")
	}
	for _, reason := range []domain.ActivityReason{domain.ReasonFee, domain.ReasonTax} {
		activity.Kind, activity.Reason = domain.ActivityCashOut, reason
		activity.Effects[0].Direction, activity.Effects[0].Classification = domain.EffectRemoved, domain.ClassificationFee
		if _, ok := InvestmentAmendment(activity); ok {
			t.Fatalf("a holding-associated %s was accepted as a dividend", reason)
		}
	}
}

func TestExplicitInvestmentReplacementOwnsItsMutationAndOriginalTime(t *testing.T) {
	ctx := t.Context()
	probe := newInvestmentReplacementProbe(t)
	note := "different kind and account"
	supplied := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: domain.NewAccountID().String(), Amount: "13.4567", Currency: "USD",
		Reason: string(domain.ReasonContribution), Note: &note, MutationID: domain.NewMutationID().String(),
		EffectiveAt: "bad caller instant", EffectiveLocalDate: "2025-01-01", EffectiveLocalTime: "12:00",
	}
	review, err := ReviewAmendment(ctx, probe, CorrectActivity, probe.activity.ID, AmendmentInput{Replacement: &supplied}, time.Unix(500, 0))
	if err != nil {
		t.Fatal(err)
	}
	command, ok := review.replacement.(domain.MoneyAddedInput)
	if !ok || command.AccountID.String() != supplied.AccountID || !command.EffectiveAt.Equal(probe.activity.EffectiveAt) ||
		review.mutation == supplied.MutationID || review.hash == "" {
		t.Fatalf("caller envelope changed correction binding: %+v, %+v", command, review)
	}
	withoutEnvelope := supplied
	withoutEnvelope.MutationID, withoutEnvelope.EffectiveAt, withoutEnvelope.EffectiveLocalDate, withoutEnvelope.EffectiveLocalTime = "", "", "", ""
	second, err := ReviewAmendment(ctx, probe, CorrectActivity, probe.activity.ID, AmendmentInput{Replacement: &withoutEnvelope}, time.Unix(501, 0))
	if err != nil || second.hash != review.hash {
		t.Fatalf("caller envelope changed payload identity: %v", err)
	}
	note = "edited after preview"
	if command.Note == nil || *command.Note != "different kind and account" {
		t.Fatal("caller changed the frozen replacement note after preview")
	}
	supplied.ReconciliationLink = &domain.ReconciliationLink{}
	var issue *domain.Error
	_, err = ReviewAmendment(ctx, probe, CorrectActivity, probe.activity.ID, AmendmentInput{Replacement: &supplied}, time.Time{})
	if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChange || issue.Field != "replacement" {
		t.Fatalf("explicit reconciliation link was accepted: %v", err)
	}
}

type investmentReplacementProbe struct {
	AmendmentLedger
	activity   domain.Activity
	account    domain.Account
	holding    domain.Holding
	instrument domain.Instrument
}

func newInvestmentReplacementProbe(t *testing.T) *investmentReplacementProbe {
	t.Helper()
	household, account, holding, instrument := domain.NewHouseholdID(), domain.NewAccountID(), domain.NewHoldingID(), domain.NewInstrumentID()
	amount := amendmentMoney(t, "1.2345")
	return &investmentReplacementProbe{
		account:    domain.Account{ID: account, AccountType: domain.TypeBrokerage, TrackingMode: domain.TrackingHoldings, BalanceSheetRole: domain.RoleAsset},
		holding:    domain.Holding{ID: holding, AccountID: account, InstrumentID: instrument},
		instrument: domain.Instrument{ID: instrument},
		activity: domain.Activity{ID: domain.NewActivityID(), HouseholdID: household, Kind: domain.ActivityCashDividend, Reason: domain.ReasonIncome,
			EffectiveAt:    time.Unix(55, 123_000_000),
			DividendDetail: &domain.DividendDetail{HoldingID: holding, InstrumentID: instrument, Amount: amount},
			Effects: []domain.ActivityEffect{{Role: domain.EffectRoleAmount, Direction: domain.EffectAdded,
				Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome, AccountID: &account, Money: &amount}}},
	}
}

func (p *investmentReplacementProbe) Activity(context.Context, domain.ActivityID) (domain.Activity, error) {
	return p.activity, nil
}

func (p *investmentReplacementProbe) ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error) {
	return []domain.AccountRecord{{Account: p.account}}, nil
}

func (p *investmentReplacementProbe) ListInstruments(context.Context, bool) ([]domain.Instrument, error) {
	return []domain.Instrument{p.instrument}, nil
}

func (p *investmentReplacementProbe) HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error) {
	return map[domain.AccountID][]domain.Holding{p.account.ID: {p.holding}}, nil
}

func (p *investmentReplacementProbe) HasReconciliationActivityLink(context.Context, domain.AccountID, domain.ActivityID) (bool, error) {
	return false, nil
}

func (p *investmentReplacementProbe) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	return &domain.HistoryOrigin{HouseholdID: p.activity.HouseholdID, Timezone: "UTC"}, nil
}

func (p *investmentReplacementProbe) PreviewFixChangeGuardedWith(ctx context.Context, id domain.ActivityID, build func(context.Context) (any, error)) (domain.ChangePreview, string, error) {
	_, err := build(ctx)
	return domain.ChangePreview{Activity: p.activity}, "test-token", err
}

func amendmentMoney(t *testing.T, amount string) domain.Money {
	t.Helper()
	money, err := domain.ParseMoney(amount, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return money
}
