package desktop

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

type ambiguousClosedDateWriter struct {
	marketClosedDateWriter
	operations []string
}

func (w *ambiguousClosedDateWriter) SetInstrumentClosedDate(ctx context.Context, operation, id, day string) (application.AgentMarketDataResult, error) {
	w.operations = append(w.operations, operation)
	result, err := w.marketClosedDateWriter.SetInstrumentClosedDate(ctx, operation, id, day)
	if err == nil && len(w.operations) == 1 {
		return application.AgentMarketDataResult{}, errors.New("synthetic lost acknowledgement")
	}
	return result, err
}

func TestMarketAgentEvidenceScopedClosureRetryAndReadback(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "agent-evidence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := application.NewService(sqlite.NewRepository(db))
	clock := func() time.Time { return time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC) }
	app.SetClock(clock)
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Agent evidence", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	asset, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic Agent NAV", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	source := "Synthetic NAV document"
	_, err = app.ImportAgentMarketData(ctx, domain.NewActivityID().String(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{
		{Kind: "nav", InstrumentID: asset.ID.String(), Currency: "USD", Value: "12.34567891", Date: "2026-11-20", SourceTitle: source, SourceURL: "https://example.invalid/synthetic-nav"},
		{Kind: "daily_reference", BaseCurrency: "SGD", QuoteCurrency: "USD", Value: "0.76", Date: "2026-11-20", SourceTitle: "Unrelated synthetic rate"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	finding := application.HealthIssue{
		ID: "agent-open-gap", InstrumentID: asset.ID.String(), Label: asset.DisplayLabel(), Kind: application.HealthKindMissingAgentPrice,
		Action: application.HealthActionNone, RangeStart: "2026-11-22", RangeEnd: "2026-11-22",
	}
	healthReader := &countedHealthReport{data: application.MarketDataHealthReport{Issues: []application.HealthIssue{finding}}}
	closureWriter := &ambiguousClosedDateWriter{marketClosedDateWriter: quoteapi.NewService(app)}
	market := &marketDataPage{ctx: ctx, reader: app, agentReader: app, closedDateWriter: closureWriter, quoteHistoryClock: clock, invalidate: func() {}}
	health := &dataHealthPage{ctx: ctx, reader: healthReader, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(dataHealthRoute)
	h.size = image.Pt(640, 480)
	focus := healthFindingFocus{id: finding.ID, instrument: finding.InstrumentID, from: finding.RangeStart, to: finding.RangeEnd}
	health.enter(health.list.Position, focus)
	health.workers.Wait()
	h.frame()
	wait := func() { market.workers.Wait(); h.frame() }
	h.click("Agent evidence", false)
	wait()
	d := market.agentEvidence
	if d == nil || d.result == nil || !d.scope || d.date != "2026-11-22" || len(d.rows) != 0 {
		t.Fatalf("Agent evidence did not open to the selected civil date: %+v", d)
	}
	h.click("Show all dates", false)
	h.frame()
	if len(d.rows) != 1 || d.rows[0].record.SourceTitle != source || d.rows[0].record.InstrumentQuote.UnitPrice.Canonical() != "12.34567891" {
		t.Fatalf("past evidence or local source attribution was lost: %+v", d.rows)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(d.from)
	if health.focus != focus || healthReader.scans.Load() != 1 || len(closureWriter.operations) != 0 {
		t.Fatal("evidence dismissal changed the active finding or wrote a closure")
	}
	h.press(key.NameReturn)
	wait()
	d = market.agentEvidence
	h.click("Record closed date", false)
	wait()
	if d.saveErr == nil || d.date != "2026-11-22" || len(closureWriter.operations) != 1 {
		t.Fatal("ambiguous completion did not retain the date")
	}
	h.click("Record closed date", false)
	wait()
	wait()
	health.workers.Wait()
	h.frame()
	if d.receipt == nil || !d.receipt.Replayed || len(closureWriter.operations) != 2 || closureWriter.operations[0] != closureWriter.operations[1] {
		t.Fatalf("retry did not reuse the Wails operation receipt: %+v", d.receipt)
	}
	if len(d.rows) != 1 || d.rows[0].record.Closure == nil || d.rows[0].day != "2026-11-22" ||
		d.rows[0].record.InstrumentQuote != nil || healthReader.scans.Load() != 2 {
		t.Fatalf("saved coverage evidence or recheck = %+v, scans=%d", d.rows, healthReader.scans.Load())
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(d.from)
	quotes, err := app.InstrumentQuoteHistory(ctx, asset.ID)
	if err != nil || len(quotes) != 1 || quotes[0].UnitPrice.Canonical() != "12.34567891" {
		t.Fatalf("closure changed stored prices: %+v, %v", quotes, err)
	}
}

func TestProjectAgentEvidenceRetainsScopedFXAndWithdrawal(t *testing.T) {
	q := domain.FXQuote{ID: domain.NewFXQuoteID(), BaseCurrency: "USD", QuoteCurrency: "JPY", EffectiveDate: "2026-11-20"}
	other := domain.FXQuote{ID: domain.NewFXQuoteID(), BaseCurrency: "USD", QuoteCurrency: "SGD", EffectiveDate: "2026-11-20"}
	records := []domain.AgentQuoteRecord{
		{Operation: domain.AgentQuoteAppend, FXQuote: &q},
		{Operation: domain.AgentQuoteAppend, FXQuote: &other},
		{Operation: domain.AgentQuoteRetract, TargetQuoteID: q.ID.String(), SourceTitle: "Synthetic correction reason"},
	}
	got := projectAgentEvidence(records, marketQuoteHistoryTarget{pair: marketFXPair{"JPY", "USD"}}, time.UTC)
	if len(got) != 2 || got[0].record.Operation != domain.AgentQuoteRetract || got[0].day != "2026-11-20" ||
		got[1].record.FXQuote != &q || !got[1].inactive {
		t.Fatalf("FX provenance was lost or disclosed an unrelated target: %+v", got)
	}
}
