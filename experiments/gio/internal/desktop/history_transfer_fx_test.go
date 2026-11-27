package desktop

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestTransferQuoteFillsDraftAndActualAmountsMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, setup := openBankFixture(t, root)
	from, _, to, _ := seedMovementAccounts(t, seed, true)
	old := appendTransferReference(t, seed)
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	ref, _ := openBankFixture(t, copyBankProfile(t, root))
	service, storage := openBankFixture(t, root)
	fault := &cashMovementFault{Service: service}
	c, page, h, finish := cashMovementHarness(t, fault)
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	require := func(label string) { t.Helper(); assertPageLabel(t, &h.router, label) }
	click("Record change")
	click("Record transfer")
	pickAccountReference(h, "From account", "Harbor", "Harbor Checking · USD")
	pickAccountReference(h, "To account", "Euro", "Euro Cash · EUR")
	finish()
	d := c.draft
	referenceLabel := "Latest reference · 1 EUR = 1.2 USD"
	require(referenceLabel)
	wantQuote, err := quoteapi.NewService(ref).CurrentFXQuote(ctx, "USD", "EUR")
	if err != nil || wantQuote == nil || d.fx.value == nil || wire.FromFXQuote(*d.fx.value) != *wantQuote || d.fx.value.ID != old.ID {
		t.Fatalf("Gio current reference differs from Wails: %+v / %+v, %v", d.fx.value, wantQuote, err)
	}
	// The selected day can be earlier than the latest saved reference.
	h.click("Effective date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	finish()
	require(referenceLabel)
	if d.day != "2026-11-21" || d.fx.value.ID != old.ID {
		t.Fatalf("date change replaced the current quote: %s / %+v", d.day, d.fx.value)
	}

	click("Fill from reference rate")
	require("Enter one amount first.")
	setBankField(h, "Sent amount · USD", &d.amount, "0.0001")
	click("Fill from reference rate")
	require("The estimate is below one minor unit or exceeds the allowed amount.")
	if d.received.Text() != "" {
		t.Fatal("a rounded zero was inserted into the draft")
	}
	setBankField(h, "Sent amount · USD", &d.amount, "125.625")
	click("Fill from reference rate")
	if d.received.Text() != "104.69" || d.amount.Text() != "125.625" {
		t.Fatalf("reverse direction did not divide and round: %q / %q", d.amount.Text(), d.received.Text())
	}
	click("Fill from reference rate")
	require("Clear one amount to calculate it.")
	setBankField(h, "Sent amount · USD", &d.amount, "")
	click("Fill from reference rate")
	if d.amount.Text() != "125.63" || d.received.Text() != "104.69" {
		t.Fatalf("forward direction did not multiply and round: %q / %q", d.amount.Text(), d.received.Text())
	}
	if len(fault.attempts) != 0 || d.review != nil {
		t.Fatal("calculating armed or issued a write")
	}
	setBankField(h, "Sent amount · USD", &d.amount, "125.625")
	click("Preview transfer")
	require("Received principal · 104.69 EUR")
	click("Back to details")
	setBankField(h, "Received amount · EUR", &d.received, "100.5")
	setBankField(h, "Fee in source currency (optional) · USD", &d.fee, "1.2505")
	if d.review != nil {
		t.Fatal("a changed actual amount retained its review")
	}
	click("Preview transfer")
	require("Sent principal · 125.625 USD")
	require("Received principal · 100.5 EUR")
	require("Executed rate · 1 USD = 0.8 EUR")
	gotPreview := wire.FromReviewedChangePreview(d.review.Preview)
	req := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: from.String(), ToAccountID: to.String(),
		Sent: "125.625", SentCurrency: "USD", Received: "100.5", ReceivedCurrency: "EUR", Fee: "1.2505", FeeCurrency: "USD",
		EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "01:00", MutationID: domain.NewMutationID().String()}
	adapter := historyapi.NewService(ref)
	wantPreview, err := adapter.PreviewChange(ctx, req)
	if err != nil || !reflect.DeepEqual(gotPreview.Review, wantPreview.Review) || !reflect.DeepEqual(gotPreview.DerivedRate, wantPreview.DerivedRate) {
		t.Fatalf("actual transfer preview differs from Wails: %+v / %+v, %v", gotPreview, wantPreview, err)
	}
	// Queue two confirmations before allowing completion to update the model.
	bounds := semanticBounds(t, &h.router, "Confirm transfer", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	h.clickAt(point)
	finish()
	require("Transfer posted.")
	if len(fault.attempts) != 1 {
		t.Fatalf("double confirmation invoked %d writes", len(fault.attempts))
	}
	written, err := adapter.RecordChange(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	wantActivity, err := adapter.Activity(ctx, written.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := page.reads.detail.id
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	got, err := reopened.Activity(ctx, id)
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
	if !reflect.DeepEqual(normalize(wire.FromActivity(got)), normalize(wantActivity)) || got.TransactionFXRate == nil ||
		got.TransactionFXRate.Canonical() != "0.8" || got.ID == "" {
		t.Fatalf("reopened execution differs from the Wails actual amounts: %+v / %+v", got, wantActivity)
	}
	for id, want := range map[domain.AccountID]string{from: "1607.2545", to: "142.5"} {
		if value := readBankRecord(t, reopened, id).LatestValue.Amount.CanonicalAmount(); value != want {
			t.Fatalf("native balance %s, want %s", value, want)
		}
	}
	quotes, err := reopened.FXQuoteHistory(ctx)
	if err != nil || len(quotes) != 1 || wire.FromFXQuote(quotes[0]) != *wantQuote {
		t.Fatalf("draft calculation changed market data: %+v / %v", quotes, err)
	}
}

func TestTransferQuoteShowsCivilReferenceDateWithoutShiftingIt(t *testing.T) {
	view := ui.View{Language: i18n.English, DateFormat: settings.DateFormatISO,
		TimeLocation: time.FixedZone("Synthetic local zone", -8*60*60)}
	quote := domain.FXQuote{
		SourceKind: domain.QuoteSourceProvider, SourceKey: "synthetic-market", TimestampBasis: "date_label",
		EffectiveDate: "2026-11-21", QuotedAt: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
	}
	if got := transferQuoteEvidence(view, quote); got != "Provider quote · synthetic-market · 2026-11-21" {
		t.Fatalf("date-labelled evidence was not kept as a civil date: %q", got)
	}
}

type transferQuoteReadFault struct {
	*cashMovementFault
	first, release chan struct{}
	attempted      atomic.Int32
}

func (s *transferQuoteReadFault) CurrentFXQuote(ctx context.Context, from, to domain.CurrencyCode) (*domain.FXQuote, error) {
	switch s.attempted.Add(1) {
	case 1:
		close(s.first)
		<-s.release
		// Simulate a late response that did not observe cancellation.
		rate, _ := domain.ParseFxRate("9")
		return &domain.FXQuote{BaseCurrency: "EUR", QuoteCurrency: "USD", Rate: rate}, nil
	case 2:
		return nil, errors.New("test: quote read unavailable")
	default:
		return s.Service.CurrentFXQuote(ctx, from, to)
	}
}

func TestTransferQuoteIgnoresAbandonedReadAndRetriesOnlyRead(t *testing.T) {
	app, _ := openBankFixture(t, createFixtureRoot(t))
	seedMovementAccounts(t, app, true)
	appendTransferReference(t, app)
	fault := &transferQuoteReadFault{cashMovementFault: &cashMovementFault{Service: app}, first: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	c, _, h, finish := cashMovementHarness(t, fault)
	t.Cleanup(release)
	h.click("Record change", false)
	h.click("Record transfer", false)
	finish()
	pickAccountReference(h, "From account", "Harbor", "Harbor Checking · USD")
	pickAccountReference(h, "To account", "Euro", "Euro Cash · EUR")
	select {
	case <-fault.first:
	case <-time.After(3 * time.Second):
		t.Fatal("cross-currency quote was not requested")
	}
	pickAccountReference(h, "To account", "Reserve", "Cash Reserve · USD")
	if findPageLabel(&h.router, "Fill from reference rate") != nil {
		t.Fatal("same-currency transfer exposed the calculator")
	}
	pickAccountReference(h, "To account", "Euro", "Euro Cash · EUR")
	release()
	finish()
	assertPageLabel(t, &h.router, "Could not read the saved FX quote.")
	if c.draft.fx.value != nil || findPageLabel(&h.router, "Fill from reference rate") != nil {
		t.Fatal("a late quote replaced the new pair state")
	}
	d := c.draft
	setBankField(h, "Sent amount · USD", &d.amount, "12")
	h.click("Retry quote read", false)
	finish()
	assertPageLabel(t, &h.router, "Latest reference · 1 EUR = 1.2 USD")
	if fault.attempted.Load() != 3 || len(fault.attempts) != 0 || d.amount.Text() != "12" || d.received.Text() != "" {
		t.Fatal("read retry touched the amounts or called the writer")
	}
	h.click("Fill from reference rate", false)
	finish()
	if d.received.Text() != "10" || len(fault.attempts) != 0 {
		t.Fatal("the replacement quote failed to fill the draft")
	}
}

func appendTransferReference(t *testing.T, service *application.Service) domain.FXQuote {
	t.Helper()
	ctx := t.Context()
	if _, err := service.SetFXPreference(ctx, "USD", "EUR", "manual"); err != nil {
		t.Fatal(err)
	}
	q, err := service.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", "2026-11-21T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return q
}
