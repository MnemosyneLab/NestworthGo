package desktop

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const cardInvalidTarget = "Enter a nonnegative amount owed with up to four decimal places. Overpaid balances are unsupported."

func TestCreditCardBalanceBeforeHistoryMatchesWailsWithoutReplay(t *testing.T) {
	ctx := t.Context()
	app, session, paths, _ := seedBankBalanceFixture(t, false)
	ref, refSession, _, _ := seedBankBalanceFixture(t, false)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	fault := &balanceTargetTestService{Service: app, lostAppend: true}
	p, h, finish := balanceTargetHarness(t, fault, "Synthetic Card", func() time.Time { return accountTestClock })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }

	click("Update card balance")
	d := p.balanceTarget.draft
	setBankField(h, "Target amount owed · USD", &d.amount, "-10")
	click("Review card balance")
	label(cardInvalidTarget)
	adapter := account.NewService(ref)
	if _, err := adapter.AppendAccountValue(ctx, refCard.String(), "-10", ""); err == nil {
		t.Fatal("Wails append accepted an overpaid card balance")
	}
	assertBankObservations(t, session, card, 1, 0)
	assertBankObservations(t, refSession, refCard, 1, 0)

	setBankField(h, "Target amount owed · USD", &d.amount, "0")
	click("Review card balance")
	label("Amount owed after · 0 USD")
	label("Balance before · -30 USD")
	label("Resulting balance · 0 USD")
	label("Effective at confirmation")
	click("Confirm card balance")
	label("Could not confirm the save. Check the recorded balance before making another change.")
	if findPageLabel(&h.router, "Retry same submission") != nil || findPageLabel(&h.router, "Card balance saved.") != nil {
		t.Fatal("uncertain observation advertised an acknowledgement or safe replay")
	}
	click("Reload")
	if fault.appends != 1 || len(fault.records) != 0 {
		t.Fatalf("read recovery attempted a write: %d appends, %d adjustments", fault.appends, len(fault.records))
	}
	want, err := adapter.AppendAccountValue(ctx, refCard.String(), "0", "")
	got := wire.FromAccountValue(*readBankRecord(t, app, card).LatestValue)
	got.ID, got.AccountID, want.ID, want.AccountID = "", "", "", ""
	if err != nil || !reflect.DeepEqual(got, want) || got.Amount.Amount != "0" {
		t.Fatalf("zero debt observation differs from Wails: %+v / %+v, %v", got, want, err)
	}
	assertBankObservations(t, session, card, 2, 0)
	assertBankObservations(t, refSession, refCard, 2, 0)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, after := reopenOnboardingProfile(t, paths)
	assertBankObservations(t, after, card, 2, 0)
	assertBalanceTargetNetWorth(t, reopened, ref, "100", "0")
}

func TestCreditCardReconciliationSignedReviewAndReceiptMatchWails(t *testing.T) {
	ctx := t.Context()
	app, session, paths, _ := seedBankBalanceFixture(t, true)
	ref, refSession, _, _ := seedBankBalanceFixture(t, true)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	fault := &balanceTargetTestService{Service: app, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	uiNow := accountTestClock
	p, h, finish := balanceTargetHarness(t, fault, "Synthetic Card", func() time.Time { return uiNow })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	assertBalanceTargetNetWorth(t, app, ref, "70", "30")

	click("Update card balance")
	d := p.balanceTarget.draft
	setBankField(h, "Target amount owed · USD", &d.amount, "-10")
	click("Review card balance")
	label(cardInvalidTarget)
	adapter := historyapi.NewService(ref)
	overpaid := wailsBalanceAdjustment(refCard, "-10", uiNow)
	if _, err := adapter.PreviewChange(ctx, overpaid); err == nil {
		t.Fatal("Wails preview accepted an overpaid card balance")
	}
	if _, err := adapter.RecordChange(ctx, overpaid); err == nil {
		t.Fatal("Wails commit accepted an overpaid card balance")
	}
	assertBankObservations(t, session, card, 1, 0)
	assertBankObservations(t, refSession, refCard, 1, 0)

	setBankField(h, "Target amount owed · USD", &d.amount, "30")
	click("Review card balance")
	label("The target matches the current amount owed.")
	setBankField(h, "Target amount owed · USD", &d.amount, "50.125")
	click("Review card balance")
	label("Amount owed after · 50.125 USD")
	label("Balance before · -30 USD")
	label("Resulting balance · -50.125 USD")
	label("Adjustment · -20.125 USD")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{app, card}, {ref, refCard}} {
		if _, err := historyapi.NewService(side.app).RecordChange(ctx, wailsBalanceAdjustment(side.id, "35", uiNow)); err != nil {
			t.Fatal(err)
		}
	}
	uiNow = accountTestClock.Add(time.Minute)
	setBalanceTargetClock(time.Minute, app, ref)
	click("Confirm card balance")
	label("The ledger changed. Preview this action again.")
	click("Review card balance")
	label("Balance before · -35 USD")
	label("Adjustment · -15.125 USD")
	label("Effective: 2026-11-22 01:01:00.000 (Asia/Singapore)")
	plan := *d.review
	button := semanticBounds(t, &h.router, "Confirm card balance", false)
	point := button.Min.Add(button.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("card confirmation did not start")
	}
	h.clickAt(point)
	release()
	finish()
	label("Saving could not be confirmed. Retry the same submission.")
	if len(fault.records) != 2 {
		t.Fatal("duplicate confirmation produced another write attempt")
	}
	click("Retry same submission")
	label("Card balance saved.")
	label("Update failed. Showing the previous result.")
	posted := p.balanceTarget.saved.activity
	click("Retry")
	if len(fault.records) != 3 || fault.records[1] != plan.mutation || fault.records[2] != plan.mutation || fault.appends != 0 {
		t.Fatal("keyed recovery changed identity or read recovery reissued the write")
	}
	assertWailsBalanceAdjustment(t, app, ref, refCard, plan, posted)
	assertBalanceTargetNetWorth(t, app, ref, "49.875", "50.125")
	click("View reconciliation")
	label("Value update")
	label("Synthetic Card · Debt increased by 15.125 USD")
	label("Amount owed after · 50.125 USD")
	label("Remeasurement")
	click("Back")

	uiNow = accountTestClock.Add(2 * time.Minute)
	setBalanceTargetClock(2*time.Minute, app, ref)
	click("Update card balance")
	d = p.balanceTarget.draft
	setBankField(h, "Target amount owed · USD", &d.amount, "0")
	click("Review card balance")
	label("Amount owed after · 0 USD")
	label("Resulting balance · 0 USD")
	label("Adjustment · +50.125 USD")
	zero := *d.review
	click("Confirm card balance")
	assertWailsBalanceAdjustment(t, app, ref, refCard, zero, p.balanceTarget.saved.activity)
	assertBalanceTargetNetWorth(t, app, ref, "100", "0")
	click("View reconciliation")
	label("Synthetic Card · Debt decreased by 50.125 USD")
	label("Amount owed after · 0 USD")
	label("Remeasurement")
	assertBankObservations(t, session, card, 1, 3)
	assertBankObservations(t, refSession, refCard, 1, 3)

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, after := reopenOnboardingProfile(t, paths)
	setBalanceTargetClock(72*time.Hour, reopened, ref)
	again, err := plan.commit(ctx, reopened)
	if err != nil || again.activity != posted {
		t.Fatalf("original receipt replay after reopen: %+v, %v", again, err)
	}
	assertBankObservations(t, after, card, 1, 3)
	assertBalanceTargetNetWorth(t, reopened, ref, "100", "0")
	assertCardReconciliationClassification(t, reopened)
	assertCardAssetChanges(t, reopened, ref)
}

func syntheticCardID(t *testing.T, ledger *application.Service) domain.AccountID {
	t.Helper()
	rows, err := ledger.ListAccounts(t.Context(), domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Account.AccountType == domain.TypeCreditCard {
			return row.Account.ID
		}
	}
	t.Fatal("marked fixture has no credit card")
	return ""
}

func assertCardReconciliationClassification(t *testing.T, ledger *application.Service) {
	t.Helper()
	rows, err := ledger.ListActivities(t.Context(), 50)
	if err != nil || len(rows) != 3 {
		t.Fatalf("expected only the three explicit card adjustments: %+v, %v", rows, err)
	}
	wantAmounts := []string{"50.125", "15.125", "5"}
	wantDirections := []domain.EffectDirection{domain.EffectRemoved, domain.EffectAdded, domain.EffectAdded}
	for i, activity := range rows {
		if activity.Kind != domain.ActivityValueUpdate || activity.Reason != domain.ReasonReconciliation || len(activity.Effects) != 1 {
			t.Fatalf("unexpected card history: %+v", activity)
		}
		effect := activity.Effects[0]
		if effect.Classification != domain.ClassificationRemeasurement || effect.Target != domain.EffectTargetAccountValue ||
			effect.Direction != wantDirections[i] || effect.Money == nil || effect.Money.Currency() != "USD" || effect.Money.CanonicalAmount() != wantAmounts[i] {
			t.Fatalf("unexpected card remeasurement: %+v", effect)
		}
	}
}

func assertCardAssetChanges(t *testing.T, gio, ref *application.Service) {
	t.Helper()
	ctx := t.Context()
	request := analysis.AnalysisQueryRequest{ScopeKind: string(domain.ScopeHousehold), From: "2026-11-21", To: "2026-11-22",
		Valuation: string(domain.ValuationBase), Basis: string(domain.ReturnBasisInvestment), IncludeCash: true}
	query, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	got, err := gio.AssetChange(ctx, query)
	want, refErr := analysis.NewService(ref).AssetChange(ctx, request)
	if err != nil || refErr != nil || got.Status != domain.CompletenessOK || want.Status != string(domain.CompletenessOK) {
		t.Fatalf("asset changes incomplete: %s / %s, %v / %v", got.Status, want.Status, err, refErr)
	}
	local, reference := map[string]string{}, map[string]string{}
	for _, row := range got.Waterfall {
		if row.Amount != nil {
			local[string(row.Bucket)] = row.Amount.CanonicalAmount()
		}
	}
	for _, row := range want.Waterfall {
		if row.Amount != nil {
			reference[row.Bucket] = row.Amount.Amount
		}
	}
	external, adjustment := string(domain.BucketExternalFlow), string(domain.BucketAdjustment)
	if !reflect.DeepEqual(local, reference) || (local[external] != "" && local[external] != "0") || local[adjustment] != "30" {
		t.Fatalf("want USD 30 adjustment and zero external flow on both sides: Gio %v, Wails %v", local, reference)
	}
}
