package desktop

import (
	"image"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

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

func TestCashAccountDebtPostingResumesAfterStartingPoint(t *testing.T) {
	ctx := t.Context()
	app, session, paths, bank := seedBankBalanceFixture(t, false)
	ref, _, _, refBank := seedBankBalanceFixture(t, false)
	for _, svc := range []*application.Service{app, ref} {
		setBalanceTargetClock(-48*time.Hour, svc)
	}
	loan := addDebtScopeAccount(t, app, "Synthetic Loan", "loan", "liability", "balance", "USD", "50")
	refLoan := addDebtScopeAccount(t, ref, "Synthetic Loan", "loan", "liability", "balance", "USD", "50")
	for _, svc := range []*application.Service{app, ref} {
		addDebtScopeAccount(t, svc, "Other Currency Loan", "loan", "liability", "balance", "EUR", "15")
		archived := addDebtScopeAccount(t, svc, "Archived Loan", "loan", "liability", "balance", "USD", "0")
		if err := svc.ArchiveAccount(ctx, archived, true); err != nil {
			t.Fatal(err)
		}
		setBalanceTargetClock(0, svc)
	}
	fault := &balanceTargetTestService{Service: app, keepReads: true, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	now := accountTestClock
	p, h, finish := liabilityPostingHarness(t, fault, "Harbor Checking", image.Pt(620, 780), func() time.Time { return now })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	adapter := historyapi.NewService(ref)
	click("Record debt activity")
	label("Harbor Checking · The Starting Point applies to the whole household.")
	click("Review Starting Point")
	click("Cancel")
	h.focused(&p.liabilityPosting.open)
	assertBankObservations(t, session, bank, 1, 0)

	h.press(key.NameReturn)
	finish()
	click("Review Starting Point")
	h.press(key.NameReturn)
	finish()
	label("Starting Point saved.")
	if _, err := ref.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	d := p.liabilityPosting.draft
	h.focused(&d.accountPicker.Trigger)
	if d.account.ID != bank || d.kind != historyapi.ChangeDebtDraw {
		t.Fatal("Starting Point changed the cash account or debt action")
	}
	h.press(key.NameReturn)
	finish()
	if findPageLabel(&h.router, "Archived Loan · USD") != nil {
		t.Fatal("archived debt account was offered in the picker")
	}
	click("Other Currency Loan · EUR")
	setBankField(h, "Debt principal · USD", &d.amount, "20.125")
	click("Preview change")
	label("The currency must match the account balances.")
	h.click("Debt account", true)
	finish()
	click("Synthetic Loan · USD")
	click("Preview change")
	label("Harbor Checking · 100 USD → 120.125 USD")
	label("Synthetic Loan · -50 USD → -70.125 USD")
	// An independent write invalidates the reviewed token.
	other := wailsBalanceAdjustment(bank, "110", now)
	if _, err := historyapi.NewService(app).RecordChange(ctx, other); err != nil {
		t.Fatal(err)
	}
	other.AccountID, other.MutationID = refBank.String(), domain.NewMutationID().String()
	if _, err := adapter.RecordChange(ctx, other); err != nil {
		t.Fatal(err)
	}
	click("Confirm posting")
	label("The ledger changed. Preview the change again.")
	if d.counterparty != loan || d.amount.Text() != "20.125" {
		t.Fatal("stale preview lost the selected debt or principal")
	}
	click("Preview change")
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeDebtDraw, DebtAccountID: refLoan.String(), CashAccountID: refBank.String(),
		Principal: "20.125", PrincipalCurrency: "USD", EffectiveAt: now.UTC().Format(time.RFC3339Nano),
	}
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	confirm := semanticBounds(t, &h.router, "Confirm posting", false)
	point := confirm.Min.Add(confirm.Max).Div(2)
	h.press(key.NameReturn)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("guarded draw did not reach the write")
	}
	h.clickAt(point)
	h.click("Back", false)
	if !p.liabilityPostingUnresolved() || h.shell.activeRoute() != accountsRoute {
		t.Fatal("pending write allowed navigation")
	}
	release()
	finish()
	label("Saving could not be confirmed. Retry the same submission.")
	h.focused(&d.retryConfirm)
	click("Retry same submission")
	assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
	if len(fault.records) != 3 || fault.records[1] != fault.records[2] {
		t.Fatal("retry submitted a second mutation")
	}
	click("Back to account")
	h.focused(&p.liabilityPosting.open)
	assertBankObservations(t, session, bank, 1, 2)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	if readBankRecord(t, reopened, bank).LatestValue.Amount.CanonicalAmount() != "130.125" ||
		readBankRecord(t, reopened, loan).LatestValue.Amount.CanonicalAmount() != "70.125" {
		t.Fatal("reopened exact balances differ from confirmation")
	}
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
		if _, err := svc.StartHistory(ctx, "UTC"); err != nil {
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
		if _, err := svc.StartHistory(ctx, "UTC"); err != nil {
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
