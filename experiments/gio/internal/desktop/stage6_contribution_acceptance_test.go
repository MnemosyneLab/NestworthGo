package desktop

import (
	"image"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wailsanalysis "github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	wailshistory "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const stage6ContributionNote = "Harbor October dividend evidence"

func seedStage6Contribution(t *testing.T, start time.Time) ([]byte, domain.AccountID, domain.InstrumentID, wire.ActivityDTO) {
	t.Helper()
	ctx := t.Context()
	seed, harbor, fund := seedStage6ReturnCalendar(t, start)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "contribution"), seed)
	db, _, svc := openStage6Database(t, path, func() time.Time { return start.AddDate(0, 0, 2) })
	accounts, err := svc.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) != 2 {
		t.Fatalf("contribution seed accounts: %+v (%v)", accounts, err)
	}
	day := start.AddDate(0, 0, 1).Format(time.DateOnly)
	api := wailshistory.NewService(svc)
	var expected wire.ActivityDTO
	for _, account := range accounts {
		id := account.Account.ID
		holdings, err := svc.ListHoldings(ctx, id, false)
		if err != nil || len(holdings) != 1 {
			t.Fatalf("contribution seed holdings: %+v (%v)", holdings, err)
		}
		amount, note := "7.125", "Second Brokerage dividend control"
		if id == harbor {
			amount, note = "12.3456", stage6ContributionNote
		}
		result, err := api.RecordChange(ctx, wailshistory.ChangeCommandRequest{
			Kind: wailshistory.ChangeCashDividend, HoldingID: holdings[0].ID.String(), Currency: "EUR", Amount: amount,
			EffectiveLocalDate: day, EffectiveLocalTime: "21:00", Note: &note,
		})
		if err != nil {
			t.Fatalf("seed %s: %v", note, err)
		}
		if id == harbor {
			expected = result.Activity
		}
	}
	controlNote := "Harbor cash-only control"
	if _, err := api.RecordChange(ctx, wailshistory.ChangeCommandRequest{
		Kind: wailshistory.ChangeMoneyAdded, AccountID: harbor.String(), Currency: "EUR", Amount: "3", Reason: "contribution",
		EffectiveLocalDate: day, EffectiveLocalTime: "21:01", Note: &controlNote,
	}); err != nil {
		t.Fatal(err)
	}
	return stage6SeedBytes(t, db), harbor, fund, expected
}

func TestStage6PairedContributionFindsExactLedgerRecord(t *testing.T) {
	const from, to, selected = "2026-10-01", "2026-10-31", "2026-10-02"
	ctx := t.Context()
	clock := func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	seed, account, fund, expected := seedStage6Contribution(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	gioProvider, wailsProvider := &unavailableOverviewProvider{}, &unavailableOverviewProvider{}
	_, _, gio := openStage6Database(t, gioPath, clock, application.NewMarketDataRegistry(gioProvider))
	_, _, wails := openStage6Database(t, wailsPath, clock, application.NewMarketDataRegistry(wailsProvider))
	accountID, currency := account.String(), "EUR"
	request := wailsanalysis.AnalysisQueryRequest{ScopeKind: "instrument", ScopeID: fund.String(),
		From: from, To: to, Valuation: "native", Basis: "investment", IncludeCash: false, AccountID: &accountID, Currency: &currency}
	wantQuery, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	item, err := wailsanalysis.NewService(wails).ContributionItem(ctx, request, string(application.ContributionDividendInterest),
		string(application.ContributionGroupInstrument), fund.String())
	if err != nil || item.Amount == nil || item.Amount.Amount != "12.3456" || item.Amount.Currency != currency {
		t.Fatalf("Wails contribution = %+v (%v)", item, err)
	}
	hint := item.HistoryHint
	if hint.From != from || hint.To != to || hint.AccountID != accountID || hint.InstrumentID != fund.String() || len(hint.Kinds) != 0 {
		t.Fatalf("Wails sent an unexpected Ledger hint: %+v", hint)
	}
	reference := wailshistory.NewService(wails)
	timeline, err := reference.ListActivityPage(ctx, wailshistory.ActivityQueryRequest{FromLocalDate: hint.From, ToLocalDate: hint.To,
		AccountID: &hint.AccountID, InstrumentID: &hint.InstrumentID, Kinds: hint.Kinds})
	if err != nil || len(timeline.Activities) != 1 || timeline.HasMore || timeline.Activities[0].ID != expected.ID {
		t.Fatalf("Wails filtered Ledger = %+v (%v)", timeline, err)
	}
	record, err := reference.Activity(ctx, expected.ID)
	if err != nil || record.Kind != string(domain.ActivityCashDividend) || record.EffectiveLocalDate != selected || record.Note == nil || *record.Note != stage6ContributionNote ||
		record.DividendDetail == nil || record.DividendDetail.InstrumentID != fund.String() || record.DividendDetail.Amount.Amount != "12.3456" || record.DividendDetail.Amount.Currency != currency {
		t.Fatalf("Wails linked cash dividend = %+v (%v)", record, err)
	}

	page := &returnAnalysisPage{ctx: ctx, reader: gio, clock: clock, invalidate: func() {}}
	ledger := newLedgerHistory(ctx, gio, clock, func() {})
	t.Cleanup(page.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	t.Cleanup(ledger.reads.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: page, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1140, 1040)
	settle := func() {
		for range 3 {
			h.frame()
			page.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	choose := func(field, name string) { t.Helper(); h.click(field, true); click(name) }
	settle()
	click("Return analysis")
	choose("Scope", "Instrument")
	choose("Instrument", domain.InstrumentSelectionLabel("Synthetic euro fund", "EURO", "", "DE", "EUR", "", "", "", ""))
	click("More filters")
	choose("Valuation", "Native currency")
	click("Include cash")
	choose("Account filter", "Harbor Brokerage")
	h.click("Currency", true)
	h.router.Queue(key.EditEvent{Text: "EUR"})
	h.frame()
	click("EUR")
	click("More filters")
	click("Previous month")
	click("Contribution")
	choose("Return type", "Dividend & Interest")
	click("Negative")
	h.click("Search contributions", true)
	h.router.Queue(key.EditEvent{Text: "EURO"})
	settle()
	assertPageLabel(t, &h.router, "EURO · +€12.35")
	trigger := page.output.rankButtons[fund.String()]
	h.router.Source().Execute(key.FocusCmd{Tag: trigger})
	h.frame()
	h.focused(trigger)
	h.press(key.NameReturn)
	settle()
	if page.item == nil || page.item.value == nil || page.item.value.Amount == nil || page.item.value.Amount.CanonicalAmount() != item.Amount.Amount {
		t.Fatalf("Gio Contribution did not match Wails: %+v", page.item)
	}
	for _, label := range []string{"Return rate · Unavailable", "Rated days 0/0", "Ledger activity will open for all accounts and instruments in the selected period."} {
		if findPageLabel(&h.router, label) != nil {
			t.Fatalf("the scoped dividend detail shows unrelated metrics or scope: %q", label)
		}
	}
	assertPageLabel(t, &h.router, "Ledger activity keeps dates, account, instrument and activity type filters. Currency and asset-class filters do not carry over.")
	click("View in Ledger activity")
	want := history.Filters{From: hint.From, To: hint.To, Account: account, Instrument: fund}
	rows := ledger.model.Rows()
	if h.shell.activeRoute() != historyRoute || !ledger.model.Filters().Equal(want) || len(rows) != 1 ||
		rows[0].ID.String() != record.ID || rows[0].EffectiveLocalDate != selected {
		t.Fatalf("Gio Ledger did not match Wails: filters=%+v rows=%+v", ledger.model.Filters(), rows)
	}
	click("View")
	assertPageLabel(t, &h.router, stage6ContributionNote)
	if d := ledger.reads.detail; d == nil || d.record == nil || d.record.Activity.ID.String() != record.ID ||
		d.record.Activity.DividendDetail == nil || d.record.Activity.DividendDetail.Amount.CanonicalAmount() != record.DividendDetail.Amount.Amount {
		t.Fatal("Gio did not open the matching Ledger record and exact dividend")
	}
	click("Close")
	h.focused(ledger.reads.buttons[rows[0].ID])
	click("Back")
	h.focused(trigger)
	if page.current == nil || !reflect.DeepEqual(page.current.query, wantQuery) || page.current.contribution == nil || page.current.contribution.ReturnType != application.ContributionDividendInterest ||
		page.output.ranking.search.Field.Text() != "EURO" || page.output.ranking.direction != contributionNegative || page.output.selectedKey != fund.String() {
		t.Fatal("Back did not restore Contribution type, search and selected instrument")
	}
	h.press(key.NameReturn)
	settle()
	if page.item == nil || page.item.value == nil || page.item.key != fund.String() || gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatal("Back reopened the wrong contribution or analysis requested a live quote")
	}
}

func TestStage6PairedContributionShowsFullLedgerScope(t *testing.T) {
	const from, to = "2026-10-01", "2026-10-31"
	const scopeNotice = "Ledger activity will open for all accounts and instruments in the selected period."
	ctx := t.Context()
	clock := func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	seed, _, _, _ := seedStage6Contribution(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	gioProvider, wailsProvider := &unavailableOverviewProvider{}, &unavailableOverviewProvider{}
	_, _, gio := openStage6Database(t, gioPath, clock, application.NewMarketDataRegistry(gioProvider))
	_, _, wails := openStage6Database(t, wailsPath, clock, application.NewMarketDataRegistry(wailsProvider))
	request := wailsanalysis.AnalysisQueryRequest{ScopeKind: string(domain.ScopeHousehold), From: from, To: to, Valuation: "native", Basis: "investment", IncludeCash: true}
	wantQuery, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	item, err := wailsanalysis.NewService(wails).ContributionItem(ctx, request, string(application.ContributionTotalReturn), string(application.ContributionGroupCurrency), "EUR")
	if err != nil || item.Amount == nil || item.Rate == nil || len(item.ByAccount) != 2 {
		t.Fatalf("expected a rated, multi-account contribution in the Wails fixture: %+v (%v)", item, err)
	}
	hint := item.HistoryHint
	if hint.From != from || hint.To != to || hint.AccountID != "" || hint.InstrumentID != "" || len(hint.Kinds) != 0 {
		t.Fatalf("expected the entire period in the Wails hint: %+v", hint)
	}
	t.Logf("EUR %s, rate %s; Ledger period %s through %s across %d contribution accounts", item.Amount.Amount, *item.Rate, hint.From, hint.To, len(item.ByAccount))
	wantLedger, err := wailshistory.NewService(wails).ListActivityPage(ctx, wailshistory.ActivityQueryRequest{FromLocalDate: hint.From, ToLocalDate: hint.To})
	if err != nil || len(wantLedger.Activities) < 3 || wantLedger.HasMore {
		t.Fatalf("the Ledger fixture did not include both accounts and the cash-only control: %+v (%v)", wantLedger, err)
	}

	page := &returnAnalysisPage{ctx: ctx, reader: gio, clock: clock, invalidate: func() {}}
	ledger := newLedgerHistory(ctx, gio, clock, func() {})
	t.Cleanup(page.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	t.Cleanup(ledger.reads.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: page, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1140, 960)
	settle := func() {
		for range 3 {
			h.frame()
			page.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	choose := func(field, name string) { t.Helper(); h.click(field, true); click(name) }
	settle()
	click("Return analysis")
	click("Previous month")
	choose("Scope", "Portfolio")
	click("More filters")
	choose("Valuation", "Native currency")
	click("More filters")
	click("Contribution")
	choose("Group by", "By currency")
	if page.current == nil || !reflect.DeepEqual(page.current.query, wantQuery) {
		t.Fatalf("Gio selected a different Contribution query: got %+v, want %+v", page.current, wantQuery)
	}
	h.click("Search contributions", true)
	h.router.Queue(key.EditEvent{Text: "EUR"})
	settle()
	trigger := page.output.rankButtons["EUR"]
	h.router.Source().Execute(key.FocusCmd{Tag: trigger})
	h.frame()
	h.focused(trigger)
	h.press(key.NameReturn)
	settle()
	if page.item == nil || page.item.value == nil || page.item.value.Amount == nil || page.item.value.Amount.CanonicalAmount() != item.Amount.Amount ||
		page.item.value.Rate == nil || page.item.value.Rate.String() != *item.Rate || len(page.item.value.ByAccount) != len(item.ByAccount) {
		t.Fatalf("Gio detail did not match Wails: %+v / %+v", page.item, item)
	}
	for i, account := range page.item.value.ByAccount {
		want := item.ByAccount[i]
		if account.AccountID != want.AccountID || account.Amount == nil || want.Amount == nil || account.Amount.CanonicalAmount() != want.Amount.Amount {
			t.Fatalf("Gio account composition did not match Wails: %+v / %+v", account, want)
		}
	}
	assertPageLabel(t, &h.router, scopeNotice)
	h.size = image.Pt(665, 480)
	h.frame()
	notice := assertPageLabel(t, &h.router, scopeNotice)
	action := assertPageLabel(t, &h.router, "View in Ledger activity")
	if notice.Min.Y < 0 || notice.Min.Y >= action.Min.Y || action.Max.Y > h.size.Y {
		t.Fatalf("scope information and action must remain visible at compact size: %v / %v", notice, action)
	}
	click("View in Ledger activity")
	gotLedger := ledger.model.Rows()
	if h.shell.activeRoute() != historyRoute || !ledger.model.Filters().Equal(history.Filters{From: from, To: to}) || len(gotLedger) != len(wantLedger.Activities) {
		t.Fatalf("Gio Ledger did not open the full period: %+v / %+v", ledger.model.Filters(), gotLedger)
	}
	for i, row := range gotLedger {
		if row.ID.String() != wantLedger.Activities[i].ID {
			t.Fatalf("Ledger activity %d is %s, Wails returned %s", i, row.ID, wantLedger.Activities[i].ID)
		}
	}
	h.size = image.Pt(1140, 960)
	click("Back")
	h.focused(trigger)
	if page.current == nil || !reflect.DeepEqual(page.current.query, wantQuery) || page.output.contributionGroup() != application.ContributionGroupCurrency ||
		page.output.ranking.search.Field.Text() != "EUR" || page.output.selectedKey != "EUR" {
		t.Fatal("Back discarded the Contribution period, group, search or selection")
	}
	if gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatal("the broad Contribution read requested live quotes")
	}
}
