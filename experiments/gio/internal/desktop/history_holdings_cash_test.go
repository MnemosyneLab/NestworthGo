package desktop

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHoldingsCashTransferAndConversionMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, seedSession := openBankFixture(t, root)
	boot, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := seed.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 || rows[0].Account.Name != "Harbor Checking" {
		t.Fatalf("unexpected synthetic source accounts: %v, %v", rows, err)
	}
	bank := rows[0].Account.ID
	seed.SetClock(func() time.Time { return accountTestClock.Add(-96 * time.Hour) })
	if _, err := seed.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	studioRecord, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Cash Studio", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	studio := studioRecord.Account.ID
	opening := holding.NewService(seed)
	plan, err := opening.PreviewCashBalance(ctx, studio.String(), "100", "USD", nil)
	if err != nil || plan.Current != "0" || plan.Delta != "100" {
		t.Fatalf("Wails opening cash plan: %+v, %v", plan, err)
	}
	initial, err := opening.ConfirmCashBalance(ctx, plan.PlanID)
	if err != nil || initial.ActivityReason == nil || *initial.ActivityReason != string(domain.ReasonReconciliation) {
		t.Fatalf("expected the post-History reconciliation used by Wails: %+v, %v", initial, err)
	}
	if err := seedSession.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	fault := &cashMovementFault{Service: service, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	c, timeline, h, finish := cashMovementHarness(t, fault)
	t.Cleanup(func() { release(); c.workers.Wait() })
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	pickCurrency := func(field, code string) {
		t.Helper()
		h.click(field, true)
		h.router.Queue(key.EditEvent{Text: code})
		h.frame()
		label(code)
		h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
		h.frame()
		h.press(key.NameReturn)
	}
	previousDay := func(days int) {
		t.Helper()
		h.click("Effective date", true)
		for range days {
			h.press(key.NameLeftArrow)
		}
		h.press(key.NameReturn)
	}
	const day = "2026-11-21"
	refHistory := historyapi.NewService(reference)
	compare := func(req historyapi.ChangeCommandRequest, preview wire.ChangePreviewDTO, id domain.ActivityID) domain.Activity {
		t.Helper()
		wantPreview, err := refHistory.PreviewChange(ctx, req)
		if err != nil || !reflect.DeepEqual(preview.Review, wantPreview.Review) || !reflect.DeepEqual(preview.Resulting, wantPreview.Resulting) ||
			!reflect.DeepEqual(preview.DerivedRate, wantPreview.DerivedRate) {
			t.Fatalf("preview differs from Wails: %+v / %+v, %v", preview, wantPreview, err)
		}
		committed, err := refHistory.RecordChange(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		want, err := refHistory.Activity(ctx, committed.Activity.ID)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := service.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		normalize := func(dto wire.ActivityDTO) wire.ActivityDTO {
			dto.ID = ""
			for i := range dto.Effects {
				dto.Effects[i].ID, dto.Effects[i].ActivityID = "", ""
			}
			return dto
		}
		if !reflect.DeepEqual(normalize(wire.FromActivity(actual)), normalize(want)) {
			t.Fatalf("saved activity differs from Wails: %+v / %+v", actual, want)
		}
		return actual
	}
	expectInvalid := func(input history.CashMovementInput, field string) {
		t.Helper()
		_, err := history.ReviewCashMovement(ctx, service, input)
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Field != field {
			t.Fatalf("wrong validation for %+v: %v, want %s", input, err, field)
		}
	}

	h.click("Record transfer", false)
	finish()
	d := c.draft
	pickAccountReference(h, "From account", "Harbor", "Harbor Checking · USD")
	pickAccountReference(h, "To account", "Studio", "Cash Studio")
	label("Transfer amount")
	pickCurrency("Received currency", "EUR")
	previousDay(2)
	setBankField(h, "Sent amount · USD", &d.amount, "125.625")
	setBankField(h, "Received amount · EUR", &d.received, "100.5")
	h.click("Preview transfer", false)
	finish()
	label("Balance before · 0 EUR")
	label("Resulting balance · 100.5 EUR")
	transferPreview := wire.FromReviewedChangePreview(d.review.Preview)
	h.click("Confirm transfer", false)
	finish()
	label("Transfer posted.")
	transferID := c.detail.id
	transfer := compare(historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashTransfer, FromAccountID: bank.String(), ToAccountID: studio.String(),
		Sent: "125.625", SentCurrency: "USD", Received: "100.5", ReceivedCurrency: "EUR", FeeCurrency: "USD",
		EffectiveLocalDate: "2026-11-20", EffectiveLocalTime: "01:00", MutationID: fault.attempts[0].mutation,
	}, transferPreview, transferID)
	if len(transfer.Effects) != 2 || transfer.Effects[0].Target != domain.EffectTargetAccountValue ||
		transfer.Effects[1].Target != domain.EffectTargetAccountCash || transfer.Effects[0].Classification != domain.ClassificationInternalTransfer ||
		transfer.Effects[1].Classification != domain.ClassificationInternalTransfer {
		t.Fatalf("unexpected transfer effects: %+v", transfer.Effects)
	}
	h.click("Back to ledger activity", false)
	finish()

	expectInvalid(history.CashMovementInput{Movement: history.CashTransfer, Account: studio, Currency: "USD", Destination: studio, ToCurrency: "EUR", Amount: "1", Received: "1", Date: day, Time: "01:01"}, "toAccountId")
	expectInvalid(history.CashMovementInput{Movement: history.CashConversion, Account: bank, Currency: "USD", ToCurrency: "EUR", Amount: "1", Received: "1", Date: day, Time: "01:01"}, "accountId")
	expectInvalid(history.CashMovementInput{Movement: history.CashConversion, Account: studio, Currency: "USD", ToCurrency: "USD", Amount: "1", Received: "1", Date: day, Time: "01:01"}, "toCurrency")
	expectInvalid(history.CashMovementInput{Movement: history.CashConversion, Account: studio, Currency: "USD", ToCurrency: "XXX", Amount: "1", Received: "1", Date: day, Time: "01:01"}, "toCurrency")

	h.click("Record conversion", false)
	finish()
	d = c.draft
	if d.account != studio || d.currency != "USD" || d.toCurrency != "" {
		t.Fatalf("unexpected default FX endpoints: %+v", d)
	}
	h.click("Holdings account", true)
	label("Cash Studio")
	if findPageLabel(&h.router, "Harbor Checking · USD") != nil {
		t.Fatal("a total-balance account appeared in the Holdings-only picker")
	}
	h.press(key.NameEscape)
	pickCurrency("Bought currency", "EUR")
	pickCurrency("Sold currency", "EUR")
	if d.toCurrency != "" {
		t.Fatal("changing sold currency left a same-currency conversion")
	}
	pickCurrency("Sold currency", "USD")
	pickCurrency("Bought currency", "EUR")
	previousDay(1)
	h.click("Effective time", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.press(key.NameEscape)
	if d.time != "02:00" {
		t.Fatalf("unexpected fixed conversion time: %s", d.time)
	}
	setBankField(h, "Sold amount · USD", &d.amount, "100")
	setBankField(h, "Bought amount · EUR", &d.received, "32")
	setBankField(h, "Fee in source currency (optional) · USD", &d.fee, "1.25")
	h.click("Preview conversion", false)
	finish()
	label("The account balance at the effective time cannot cover this amount.")
	setBankField(h, "Sold amount · USD", &d.amount, "40")
	h.click("Preview conversion", false)
	finish()
	label("Review conversion")
	label("Resulting balance · 58.75 USD")
	label("Resulting balance · 132.5 EUR")
	label("Executed rate · 1 USD = 0.8 EUR")
	label("Fee · 1.25 USD")
	fxPreview := wire.FromReviewedChangePreview(d.review.Preview)
	bounds := semanticBounds(t, &h.router, "Confirm conversion", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("conversion confirmation did not start")
	}
	h.clickAt(point)
	release()
	finish()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	h.click("Retry same submission", false)
	finish()
	label("Conversion posted.")
	label("Could not load this activity.")
	id := fault.firstActivity
	if len(fault.attempts) != 3 || c.detail.id != id || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) {
		t.Fatal("the same conversion intent was not recovered")
	}
	h.click("Retry", false)
	finish()
	label("Sent from Cash Studio · 40 USD")
	label("Received by Cash Studio · 32 EUR")
	label("Fee paid by Cash Studio · 1.25 USD")
	if len(fault.attempts) != 3 {
		t.Fatal("post-confirmation read retried the conversion writer")
	}
	req := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeFXConversion, AccountID: studio.String(), Sold: "40", SoldCurrency: "USD",
		Bought: "32", BoughtCurrency: "EUR", Fee: "1.25", FeeCurrency: "USD",
		EffectiveLocalDate: day, EffectiveLocalTime: "02:00", MutationID: fault.attempts[1].mutation,
	}
	_, hash, err := req.MutationEnvelope()
	if err != nil || hash != fault.attempts[1].hash {
		t.Fatalf("mutation envelope differs from Wails: %v", err)
	}
	fx := compare(req, fxPreview, id)
	if fx.Kind != domain.ActivityFXConversion || len(fx.Effects) != 3 {
		t.Fatalf("unexpected conversion: %+v", fx)
	}
	for i, effect := range fx.Effects {
		classes := []domain.ActivityClassification{domain.ClassificationInternalTransfer, domain.ClassificationInternalTransfer, domain.ClassificationFee}
		if effect.Target != domain.EffectTargetAccountCash || effect.AccountID == nil || *effect.AccountID != studio ||
			effect.Classification != classes[i] || effect.Sequence != i+1 {
			t.Fatalf("unexpected effect %d: %+v", i+1, effect)
		}
	}
	h.click("Back to ledger activity", false)
	finish()
	if timeline.model.Rows()[0].ID != id {
		t.Fatal("History did not refresh to the conversion")
	}
	h.click("Overview", false)
	finish()
	label("Missing exchange rate · USD/EUR · Cash Studio")

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	attempt := fault.attempts[1]
	replayed, err := reopened.RecordChangeWithMutation(ctx, attempt.command, attempt.mutation, attempt.hash)
	if err != nil || replayed.Activity.ID != id {
		t.Fatalf("durable replay returned %s, %v", replayed.Activity.ID, err)
	}
	if moves := openingActivity(t, reopened, studio); len(moves) != 3 || moves[0].ID != id || moves[1].ID != transferID {
		t.Fatalf("unexpected persisted Holdings history: %+v", moves)
	}
	for _, s := range []*application.Service{reopened, reference} {
		cash, err := holding.NewService(s).ListAccountCashValues(ctx, studio.String())
		if err != nil {
			t.Fatal(err)
		}
		latest := map[string]string{}
		for _, v := range cash {
			if _, exists := latest[v.Amount.Currency]; !exists {
				latest[v.Amount.Currency] = v.Amount.Amount
			}
		}
		if !reflect.DeepEqual(latest, map[string]string{"USD": "58.75", "EUR": "132.5"}) {
			t.Fatalf("unexpected persisted account cash: %+v", latest)
		}
	}

	// A market rate is unnecessary to commit both exact native legs. Only the
	// analytical comparison against independently stored FX can give a spread.
	query := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: day, To: day,
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	}
	spread := func(app *application.Service, status domain.Completeness, want string) {
		t.Helper()
		result, err := app.AssetChange(ctx, query)
		if err != nil || result.Status != status {
			t.Fatalf("unexpected analysis completeness: %+v, %d residuals, %v", result.AnalysisAvailability, result.ResidualIssueCount, err)
		}
		var fee string
		for _, entry := range result.Waterfall {
			if entry.Bucket == domain.BucketExternalFlow && entry.Amount != nil && !entry.Amount.IsZero() {
				t.Fatalf("internal cash movements became an external flow: %+v", entry)
			}
			if entry.Bucket == domain.BucketFXConversionSpread && entry.Amount != nil {
				if want == "" && entry.Amount.IsZero() {
					continue
				}
				if entry.Amount.CanonicalAmount() != want {
					t.Fatalf("conversion spread: %s, want %s", entry.Amount.CanonicalAmount(), want)
				}
				want = ""
			}
			if entry.Bucket == domain.BucketFee && entry.Amount != nil {
				fee = entry.Amount.CanonicalAmount()
			}
		}
		if want != "" {
			t.Fatalf("expected an execution spread of %s", want)
		}
		if status == domain.CompletenessOK && fee != "-1.25" {
			t.Fatalf("source fee was not a separate USD 1.25 expense: %q", fee)
		}
	}
	for _, s := range []*application.Service{reopened, reference} {
		spread(s, domain.CompletenessPartial, "")
		if _, err := s.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", "2026-11-18T16:00:00Z"); err != nil {
			t.Fatal(err)
		}
		spread(s, domain.CompletenessOK, "-1.6")
	}
}

func TestHoldingsCashFlowsKeepWailsOpeningBaseline(t *testing.T) {
	ctx := t.Context()
	prepareBaseline := func() (*application.Service, domain.AccountID, wire.AccountCashValueDTO) {
		t.Helper()
		paths := prepareOnboardingProfile(t)
		seed, _ := reopenOnboardingProfile(t, paths)
		seed.SetClock(func() time.Time { return accountTestClock.Add(-48 * time.Hour) })
		if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{
			HouseholdName: "Synthetic cash baseline", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
		}); err != nil {
			t.Fatal(err)
		}
		boot, err := seed.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		row, err := seed.CreateAccount(ctx, application.AccountInput{
			Name: "Opening cash", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		id := row.Account.ID
		opening := holding.NewService(seed)
		plan, err := opening.PreviewCashBalance(ctx, id.String(), "100", "USD", nil)
		if err != nil || plan.Current != "0" || plan.Target != "100" || plan.Timezone != "UTC" {
			t.Fatalf("unexpected Wails opening plan %+v, %v", plan, err)
		}
		initial, err := opening.ConfirmCashBalance(ctx, plan.PlanID)
		if err != nil || initial.ObservationKind != "baseline" || initial.ActivityID != nil {
			t.Fatalf("opening cash should be a baseline: %+v, %v", initial, err)
		}
		baselineReplay, err := opening.ConfirmCashBalance(ctx, plan.PlanID)
		if err != nil || baselineReplay.ID != initial.ID {
			t.Fatalf("opening replay changed the baseline: %+v, %v", baselineReplay, err)
		}
		in := history.CashMovementInput{Movement: history.CashContribution, Account: id, Currency: "SGD", Amount: "20", Date: "2026-11-21", Time: "01:00"}
		var issue *domain.Error
		if _, err := history.ReviewCashMovement(ctx, seed, in); !errors.As(err, &issue) || issue.Code != domain.ErrHistoryNotStarted {
			t.Fatalf("History movement tried to replace the opening path: %v", err)
		}
		seed.SetClock(func() time.Time { return accountTestClock.Add(-36 * time.Hour) })
		if _, err := seed.StartHistory(ctx, "Asia/Singapore"); err != nil {
			t.Fatal(err)
		}
		seed.SetClock(func() time.Time { return accountTestClock })
		return seed, id, initial
	}
	gio, id, initial := prepareBaseline()
	reference, refID, referenceInitial := prepareBaseline()
	in := history.CashMovementInput{Account: id, Currency: "SGD", Date: "2026-11-21"}
	adapter := historyapi.NewService(reference)
	for _, step := range []struct {
		move   history.CashMovement
		kind   historyapi.ChangeCommandKind
		reason domain.ActivityReason
		class  domain.ActivityClassification
		amount string
		minute string
		before string
		after  string
	}{
		{history.CashContribution, historyapi.ChangeMoneyAdded, domain.ReasonContribution, domain.ClassificationExternalInflow, "20", "01:00", "0", "20"},
		{history.CashWithdrawal, historyapi.ChangeMoneyRemoved, domain.ReasonOther, domain.ClassificationExternalOutflow, "8.125", "01:01", "20", "11.875"},
	} {
		in.Movement, in.Amount, in.Time = step.move, step.amount, step.minute
		wantReq := historyapi.ChangeCommandRequest{Kind: step.kind, AccountID: refID.String(), Amount: step.amount, Currency: "SGD",
			Reason: string(step.reason), EffectiveLocalDate: in.Date, EffectiveLocalTime: in.Time, MutationID: domain.NewMutationID().String()}
		preview, err := history.ReviewCashMovement(ctx, gio, in)
		if err != nil {
			t.Fatal(err)
		}
		want, err := adapter.PreviewChange(ctx, wantReq)
		if err != nil || !reflect.DeepEqual(wire.FromReviewedChangePreview(preview.Preview).Review, want.Review) ||
			len(want.Review) != 1 || want.Review[0].Before != step.before || want.Review[0].After != step.after {
			t.Fatalf("Gio cash flow differs from Wails: %+v / %+v, %v", preview.Preview.Review, want.Review, err)
		}
		posted, err := preview.Commit(ctx, gio)
		if err != nil || len(posted.Effects) != 1 || posted.Effects[0].Target != domain.EffectTargetAccountCash || posted.Effects[0].Classification != step.class {
			t.Fatalf("unexpected Holdings cash flow %+v, %v", posted.Effects, err)
		}
		if _, err := adapter.RecordChange(ctx, wantReq); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct {
		app     *application.Service
		id      domain.AccountID
		opening wire.AccountCashValueDTO
	}{{gio, id, initial}, {reference, refID, referenceInitial}} {
		cash, err := holding.NewService(fixture.app).ListAccountCashValues(ctx, fixture.id.String())
		if err != nil || len(cash) != 3 || cash[0].Amount != (wire.MoneyView{Amount: "11.875", Currency: "SGD"}) || cash[2].ID != fixture.opening.ID ||
			cash[2].Amount != fixture.opening.Amount || cash[2].ActivityID != nil {
			t.Fatalf("History flows changed the original USD baseline or the exact SGD balance: %+v, %v", cash, err)
		}
	}
}
