package history

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
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
	for _, reason := range []domain.ActivityReason{domain.ReasonFee, domain.ReasonTax} {
		activity.Kind, activity.Reason = domain.ActivityCashOut, reason
		activity.Effects[0].Direction, activity.Effects[0].Classification = domain.EffectRemoved, domain.ClassificationFee
		if _, ok := InvestmentAmendment(activity); ok {
			t.Fatalf("a holding-associated %s was accepted as a dividend", reason)
		}
	}
}

func amendmentMoney(t *testing.T, amount string) domain.Money {
	t.Helper()
	money, err := domain.ParseMoney(amount, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return money
}
