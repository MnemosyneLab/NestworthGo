package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func addDebtScopeAccount(t *testing.T, app *application.Service, name, kind, role, mode, currency, amount string) domain.AccountID {
	t.Helper()
	boot, err := app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(t.Context(), application.AccountInput{
		Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: currency,
		InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return account.Account.ID
}

func TestCashAccountDebtPaymentUsesHoldingsCash(t *testing.T) {
	ctx := t.Context()
	app, _, _, bank := seedBankBalanceFixture(t, false)
	ref, _, _, refBank := seedBankBalanceFixture(t, false)
	var cash []domain.AccountID
	for _, svc := range []*application.Service{app, ref} {
		setBalanceTargetClock(-48*time.Hour, svc)
		id := addDebtScopeAccount(t, svc, "Synthetic Brokerage", "brokerage", "asset", "holdings", "USD", "")
		cash = append(cash, id)
		for _, code := range []string{"USD", "EUR"} {
			if _, err := svc.AppendAccountCashValue(ctx, id, "25.125", code, ""); err != nil {
				t.Fatal(err)
			}
		}
		setBalanceTargetClock(-24*time.Hour, svc)
		if _, err := svc.CaptureSyntheticOrigin(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
		setBalanceTargetClock(0, svc)
	}
	p, h, finish := liabilityPostingHarness(t, app, "Synthetic Brokerage", image.Pt(660, 740), func() time.Time { return accountTestClock })
	click := func(text string) { h.click(text, false); finish() }
	scrollToAccountDetailLabel(h, "Record debt activity")
	click("Record debt activity")
	click("Debt payment")
	d := p.liabilityPosting.draft
	h.click("Debt account", true)
	finish()
	click("Synthetic Card · USD")
	setBankField(h, "Debt principal · USD", &d.amount, "10.125")
	setBankField(h, "Interest or fee (optional) · USD", &d.fee, "0.0001")
	click("Preview change")
	assertPageLabel(t, &h.router, "Synthetic Brokerage · 25.125 USD → 14.9999 USD")
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeDebtPayment, DebtAccountID: syntheticCardID(t, ref).String(), CashAccountID: cash[1].String(),
		Principal: "10.125", PrincipalCurrency: "USD", InterestOrFee: "0.0001", InterestOrFeeCurrency: "USD",
		EffectiveAt: accountTestClock.UTC().Format(time.RFC3339Nano),
	}
	adapter := historyapi.NewService(ref)
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
	for i, svc := range []*application.Service{app, ref} {
		cashValues, err := svc.ListAccountCashValues(ctx, cash[i])
		if err != nil {
			t.Fatal(err)
		}
		balances := map[domain.CurrencyCode]string{}
		for _, row := range cashValues {
			if code := row.Amount.Currency(); balances[code] == "" {
				balances[code] = row.Amount.CanonicalAmount()
			}
		}
		if balances["USD"] != "14.9999" || balances["EUR"] != "25.125" {
			t.Fatalf("unexpected holdings balances: %+v", balances)
		}
	}
	if readBankRecord(t, app, bank).LatestValue.Amount.CanonicalAmount() != "100" ||
		readBankRecord(t, ref, refBank).LatestValue.Amount.CanonicalAmount() != "100" {
		t.Fatal("settling from Holdings touched the separate balance account")
	}
}

func TestLiabilityReasonsAndCorrectionMatchWails(t *testing.T) {
	ctx := t.Context()
	app, session, _, _ := seedBankBalanceFixture(t, false)
	ref, _, _, _ := seedBankBalanceFixture(t, false)
	var loans []domain.AccountID
	for _, svc := range []*application.Service{app, ref} {
		setBalanceTargetClock(-48*time.Hour, svc)
		loans = append(loans, addDebtScopeAccount(t, svc, "Synthetic Loan", "loan", "liability", "balance", "USD", "50"))
		setBalanceTargetClock(-24*time.Hour, svc)
		if _, err := svc.CaptureSyntheticOrigin(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	now := accountTestClock
	p, h, finish := liabilityPostingHarness(t, app, "Synthetic Loan", image.Pt(780, 830), func() time.Time { return now })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	adapter := historyapi.NewService(ref)
	for i, step := range []struct {
		label, amount, choice, after string
		kind                         historyapi.ChangeCommandKind
		reason                       domain.ActivityReason
	}{
		{"Increase debt", "1.2345", "Interest", "51.2345", historyapi.ChangeMoneyAdded, domain.ReasonInterest},
		{"Increase debt", "2.0001", "Fee", "53.2346", historyapi.ChangeMoneyAdded, domain.ReasonFee},
		{"Reduce debt", "0.2346", "Reconciliation", "53", historyapi.ChangeMoneyRemoved, domain.ReasonReconciliation},
		{"Outstanding balance correction", "0", "Reconciliation", "0", historyapi.ChangeValueUpdate, domain.ReasonReconciliation},
	} {
		now = accountTestClock.Add(time.Duration(i) * time.Minute)
		setBalanceTargetClock(time.Duration(i)*time.Minute, app, ref)
		click("Record debt activity")
		click(step.label)
		d := p.liabilityPosting.draft
		field := "Amount"
		if step.kind == historyapi.ChangeValueUpdate {
			field = "Target amount owed · USD"
		} else {
			h.click("Reason", true)
			finish()
			if step.kind == historyapi.ChangeMoneyRemoved && findPageLabel(&h.router, "Fee") != nil {
				t.Fatal("fee appeared in Wails decrease reason choices")
			}
			click(step.choice)
		}
		setBankField(h, field, &d.amount, step.amount)
		click("Preview change")
		label("Reason · " + step.choice)
		request := historyapi.ChangeCommandRequest{
			Kind: step.kind, AccountID: loans[1].String(), Reason: string(step.reason), EffectiveAt: now.UTC().Format(time.RFC3339Nano),
		}
		if step.kind == historyapi.ChangeValueUpdate {
			request.NewValue, request.NewValueCurrency = step.amount, "USD"
		} else {
			request.Amount, request.Currency = step.amount, "USD"
		}
		compareLiabilityReview(t, d.review.Preview, adapter, request)
		click("Confirm posting")
		label("Reason · " + step.choice)
		label("Amount owed after · " + step.after + " USD")
		assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
		click("Back to account")
	}
	assertBankObservations(t, session, loans[0], 1, 4)
}
