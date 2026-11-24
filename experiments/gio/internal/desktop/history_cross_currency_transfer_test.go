package desktop

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestCrossCurrencyTransferWithoutQuoteMatchesWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, seedSession := openBankFixture(t, root)
	from, _, to, _ := seedMovementAccounts(t, seed, true)
	if err := seedSession.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	for _, app := range []*application.Service{service, reference} {
		if quote, err := app.CurrentFXQuote(ctx, "USD", "EUR"); err != nil || quote != nil {
			t.Fatalf("the fixture already has a valuation quote: %+v, %v", quote, err)
		}
	}
	fault := &cashMovementFault{Service: service, loseAttempt: 1, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	c, timeline, h, finish := cashMovementHarness(t, fault)
	t.Cleanup(func() { release(); c.workers.Wait() })
	label := func(text string) { assertPageLabel(t, &h.router, text) }

	compareOverview := func(current *application.Service, total string) {
		t.Helper()
		got, err := current.Overview(ctx, domain.AccountFilter{})
		ref, refErr := portfolio.NewService(reference).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || refErr != nil || !reflect.DeepEqual(wire.FromMissingInputs(got.MissingInputs), ref.MissingInputs) ||
			!reflect.DeepEqual(wire.FromHouseholdDisplay(got.DisplayValuation), ref.DisplayValuation) {
			t.Fatalf("Overview differs from Wails: %+v / %+v, %v / %v", got.DisplayValuation, ref.DisplayValuation, err, refErr)
		}
		if total == "" {
			if got.Complete || ref.Complete || len(got.MissingInputs) != 1 || got.MissingInputs[0].Kind != domain.MissingFXRate ||
				got.MissingInputs[0].AccountID != to || got.MissingInputs[0].BaseCurrency != "USD" || got.MissingInputs[0].QuoteCurrency != "EUR" {
				t.Fatalf("missing market FX did not leave valuation incomplete: %+v", got.MissingInputs)
			}
		} else if !got.Complete || !ref.Complete || got.NetWorth.String() != total || ref.NetWorth != total {
			t.Fatalf("Gio net worth %s, Wails %s, want %s", got.NetWorth, ref.NetWorth, total)
		}
	}
	compareOverview(service, "")
	h.click("Record transfer", false)
	finish()
	d := c.draft
	pickAccountReference(h, "From account", "Harbor", "Harbor Checking · USD")
	pickAccountReference(h, "To account", "Euro", "Euro Cash · EUR")
	h.click("Effective date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	if d.day != "2026-11-21" || d.time != "01:00" {
		t.Fatalf("unexpected History effective time %s %s", d.day, d.time)
	}
	setBankField(h, "Sent amount · USD", &d.amount, "125.625")
	setBankField(h, "Fee in source currency (optional) · USD", &d.fee, "1.2505")
	h.click("Preview transfer", false)
	finish()
	const invalidAmount = "Enter the received amount as a positive value with up to four decimal places."
	label(invalidAmount)
	setBankField(h, "Received amount · EUR", &d.received, "0")
	h.click("Preview transfer", false)
	finish()
	label(invalidAmount)
	if len(fault.attempts) != 0 || d.review != nil {
		t.Fatal("a blank or zero native receipt reached confirmation")
	}

	setBankField(h, "Received amount · EUR", &d.received, "120")
	pickAccountReference(h, "To account", "Reserve", "Cash Reserve · USD")
	label("Transfer amount")
	if d.received.Text() != "" || d.amount.Text() != "125.625" || d.fee.Text() != "1.2505" {
		t.Fatal("changing the destination currency reinterpreted or discarded another native amount")
	}
	pickAccountReference(h, "To account", "Euro", "Euro Cash · EUR")
	if d.received.Text() != "" {
		t.Fatal("changing back restored a discarded receipt amount")
	}
	setBankField(h, "Received amount · EUR", &d.received, "99.25")
	h.click("Preview transfer", false)
	finish()
	label("Received principal · 99.25 EUR")
	h.click("Back to details", false)
	finish()
	setBankField(h, "Received amount · EUR", &d.received, "100.5")
	h.click("Preview transfer", false)
	finish()
	for _, text := range []string{"Sent principal · 125.625 USD", "Received principal · 100.5 EUR", "Executed rate · 1 USD = 0.8 EUR",
		"Fee · 1.2505 USD", "Balance before · 1734.13 USD", "Balance before · 42 EUR",
		"Balance change · -126.8755 USD", "Balance change · 100.5 EUR",
		"Resulting balance · 1607.2545 USD", "Resulting balance · 142.5 EUR"} {
		label(text)
	}
	gioPreview := wire.FromReviewedChangePreview(d.review.Preview)
	bounds := semanticBounds(t, &h.router, "Confirm transfer", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not start")
	}
	h.clickAt(point)
	release()
	finish()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	h.click("Retry same submission", false)
	finish()
	label("Transfer posted.")
	label("Could not load this activity.")
	id := fault.firstActivity
	if len(fault.attempts) != 2 || timeline.reads.detail.id != id || !reflect.DeepEqual(fault.attempts[0], fault.attempts[1]) {
		t.Fatal("double confirmation or response recovery changed the posted cross-currency intent")
	}
	h.click("Retry", false)
	finish()
	label("Executed rate · 1 USD = 0.8 EUR")
	label("Sent from Harbor Checking · 125.625 USD")
	label("Received by Euro Cash · 100.5 EUR")
	label("Fee paid by Harbor Checking · 1.2505 USD")
	if len(fault.attempts) != 2 {
		t.Fatal("the detail retry called the writer")
	}
	h.click("Back to ledger activity", false)
	finish()
	if timeline.model.Rows()[0].ID != id {
		t.Fatal("the cross-currency transfer was absent from refreshed History")
	}
	h.click("Overview", false)
	finish()
	label("Missing exchange rate · USD/EUR · Euro Cash")

	attempt := fault.attempts[0]
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashTransfer, FromAccountID: from.String(), ToAccountID: to.String(),
		Sent: "125.625", SentCurrency: "USD", Received: "100.5", ReceivedCurrency: "EUR", Fee: "1.2505", FeeCurrency: "USD",
		EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "01:00", MutationID: attempt.mutation,
	}
	_, hash, err := request.MutationEnvelope()
	if err != nil || hash != attempt.hash {
		t.Fatalf("Wails and Gio mutation envelopes differ: %v", err)
	}
	adapter := historyapi.NewService(reference)
	wantPreview, err := adapter.PreviewChange(ctx, request)
	if err != nil || !reflect.DeepEqual(gioPreview.Review, wantPreview.Review) || !reflect.DeepEqual(gioPreview.Resulting, wantPreview.Resulting) ||
		wantPreview.DerivedRate == nil || *wantPreview.DerivedRate != "0.8" || !reflect.DeepEqual(gioPreview.DerivedRate, wantPreview.DerivedRate) {
		t.Fatalf("Gio and Wails preview differ without market FX: %+v / %+v, %v", gioPreview, wantPreview, err)
	}
	written, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	wailsDetail, err := adapter.Activity(ctx, written.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Activity(ctx, id)
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
	if !reflect.DeepEqual(normalize(wire.FromActivity(got)), normalize(wailsDetail)) || len(got.Effects) != 3 ||
		got.EffectiveAt.Format(time.RFC3339) != "2026-11-20T17:00:00Z" || got.TransactionFXRate == nil || got.TransactionFXRate.Canonical() != "0.8" {
		t.Fatalf("unexpected saved activity: %+v versus Wails %+v", got, wailsDetail)
	}
	for i, want := range []struct {
		role      domain.EffectRole
		direction domain.EffectDirection
		class     domain.ActivityClassification
		account   domain.AccountID
		amount    string
		currency  domain.CurrencyCode
	}{
		{domain.EffectRoleTransferFrom, domain.EffectRemoved, domain.ClassificationInternalTransfer, from, "125.625", "USD"},
		{domain.EffectRoleTransferTo, domain.EffectAdded, domain.ClassificationInternalTransfer, to, "100.5", "EUR"},
		{domain.EffectRoleFee, domain.EffectRemoved, domain.ClassificationFee, from, "1.2505", "USD"},
	} {
		effect := got.Effects[i]
		if effect.Sequence != i+1 || effect.Role != want.role || effect.Direction != want.direction || effect.Classification != want.class ||
			effect.Target != domain.EffectTargetAccountValue || effect.AccountID == nil || *effect.AccountID != want.account ||
			effect.Money == nil || effect.Money.CanonicalAmount() != want.amount || effect.Money.Currency() != want.currency {
			t.Fatalf("effect %d: %+v, want %+v", i+1, effect, want)
		}
	}
	compareOverview(service, "")

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	replayed, err := reopened.RecordChangeWithMutation(ctx, attempt.command, attempt.mutation, attempt.hash)
	if err != nil || replayed.Activity.ID != id {
		t.Fatalf("durable replay returned %s, %v", replayed.Activity.ID, err)
	}
	request.Received = "125.625"
	changed, err := request.ToCommand(got.HouseholdID, "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err = request.MutationEnvelope()
	if err != nil {
		t.Fatal(err)
	}
	var issue *domain.Error
	if _, err := reopened.RecordChangeWithMutation(ctx, changed, attempt.mutation, hash); !errors.As(err, &issue) || issue.Code != domain.ErrConflict || issue.Field != "mutationId" {
		t.Fatalf("same ID with a changed receipt amount returned %v", err)
	}
	if len(openingActivity(t, reopened, from)) != 7 || len(openingActivity(t, reopened, to)) != 2 {
		t.Fatal("the two accounts did not share exactly one new business activity")
	}
	wailsRows, err := account.NewService(reference).ListAccounts(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[domain.AccountID]wire.MoneyView{
		from: {Amount: "1607.2545", Currency: "USD"},
		to:   {Amount: "142.5", Currency: "EUR"},
	} {
		var wailsBalance *wire.AccountValueDTO
		for _, row := range wailsRows {
			if row.Account.ID == id.String() {
				wailsBalance = row.LatestValue
				break
			}
		}
		actual := readBankRecord(t, reopened, id)
		if actual.LatestValue == nil || wire.FromMoney(actual.LatestValue.Amount) != want || wailsBalance == nil || wailsBalance.Amount != want {
			t.Fatalf("reopened native balance for %s differs from Wails or %+v", id, want)
		}
	}

	// A market quote changes valuation only. The execution and its receipt
	// remain immutably bound to the amounts typed by the user.
	for _, app := range []*application.Service{reopened, reference} {
		if _, err := app.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
			t.Fatal(err)
		}
		if _, err := app.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", "2026-11-20T17:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	compareOverview(reopened, "1978.2545")
	final, err := reopened.Activity(ctx, id)
	if err != nil || !reflect.DeepEqual(wire.FromActivity(got), wire.FromActivity(final)) {
		t.Fatalf("a later quote changed recorded execution: %+v, %v", final, err)
	}
}
