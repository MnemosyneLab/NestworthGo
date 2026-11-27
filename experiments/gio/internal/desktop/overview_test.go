package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type overviewReaderFunc func(context.Context, domain.AccountFilter) (domain.OverviewResult, error)

func (f overviewReaderFunc) Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
	return f(ctx, filter)
}

func TestOverviewRetryKeepsIncompletePriorResultAndItsReadTime(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	responses := make(chan overviewRead)
	calls := make(chan domain.AccountFilter, 1)
	first := time.Date(2026, 8, 17, 17, 34, 0, 0, time.UTC)
	now := first
	page := newOverviewPage(ctx, overviewReaderFunc(func(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
		calls <- filter
		select {
		case r := <-responses:
			return r.value, r.err
		case <-ctx.Done():
			return domain.OverviewResult{}, ctx.Err()
		}
	}), func() time.Time { return now }, func() {})
	t.Cleanup(func() { cancel(); page.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{overview: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.prefs.saved.Timezone = "Asia/Singapore"
	h.shell.prefs.savedLocation = time.FixedZone("Asia/Singapore", 8*60*60)
	h.frame()
	label := func(s string) { assertPageLabel(t, &h.router, s) }
	absent := func(s string) {
		if findPageLabel(&h.router, s) != nil {
			t.Fatalf("unexpected label %q", s)
		}
	}
	finish := func(r overviewRead) {
		t.Helper()
		select {
		case filter := <-calls:
			if !reflect.DeepEqual(filter, domain.AccountFilter{}) {
				t.Fatalf("Overview requested a scoped query: %+v", filter)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Overview did not issue a read")
		}
		responses <- r
		page.workers.Wait()
		h.frame()
	}
	label("Loading valuation…")
	absent("Net worth")
	failure := overviewRead{err: errors.New("synthetic read failure")}
	finish(failure)
	label("Could not load the valuation.")
	h.click("Retry", false)
	label("Loading valuation…")
	known := &domain.MoneyView{Amount: "128.75", Currency: "AUD"}
	partial := domain.ValuationAmount{Currency: "AUD", KnownSubtotal: known, Quality: domain.ValuationQuality{Availability: domain.ValuationPartial}}
	unknown := domain.ValuationAmount{Currency: "AUD", Quality: domain.ValuationQuality{Availability: domain.ValuationNone}}
	finish(overviewRead{value: domain.OverviewResult{
		Currency: "AUD", AccountCount: 2, DisplayValuation: domain.HouseholdDisplayValuation{Assets: partial, Liabilities: unknown, NetWorth: partial},
		MissingInputs: []domain.MissingInputView{{Kind: domain.MissingFXRate, BaseCurrency: "AUD", QuoteCurrency: "USD", AccountName: "Foreign debt"}},
	}})
	label("Incomplete valuation")
	label("Base currency AUD")
	label("Net worth · Known net contribution")
	label("A$128.75")
	label("Unavailable")
	absent("A$0.00")
	label("Missing exchange rate · AUD/USD · Foreign debt")
	stamp := "Updated 2026-08-18 01:34"
	label(stamp)
	absent("Reload")
	h.click("Settings", false)
	h.click("Back", false)
	label(stamp)
	now = first.Add(24 * time.Hour)
	h.shell.refreshLedgerReads()
	h.frame()
	label("Updating valuation…")
	finish(failure)
	label("Update failed. Showing the previous result.")
	label(stamp)
	label("Incomplete valuation")
	h.click("Retry", false)
	finish(overviewRead{value: domain.OverviewResult{Currency: "AUD", Complete: true}})
	label("Updated 2026-08-19 01:34")
	label("No accounts to show.")
	absent("Net worth · Known net contribution")
	absent("A$0.00")
	h.shell.refreshLedgerReads()
	h.frame()
	finish(overviewRead{value: domain.OverviewResult{
		Currency: "AUD", AccountCount: 1, DisplayValuation: domain.HouseholdDisplayValuation{Assets: unknown, Liabilities: unknown, NetWorth: unknown},
		MissingInputs: []domain.MissingInputView{{Kind: domain.MissingInstrumentPrice, InstrumentSymbol: "DEMO"}},
	}})
	label("Net worth")
	label("Unavailable")
	label("Missing price · DEMO")
	absent("Net worth · Known net contribution")
	absent("A$0.00")
}

type unavailableOverviewProvider struct{ requests int }

func (*unavailableOverviewProvider) Key() string { return application.FrankfurterProviderKey }
func (*unavailableOverviewProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
}
func (p *unavailableOverviewProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	p.requests++
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
}
func (p *unavailableOverviewProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	p.requests++
	return application.LatestInstrumentQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
}

func TestOverviewMatchesWailsWithPersistedFXAndAnUnavailableProvider(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	seed := application.NewService(repo)
	seed.SetClock(clock)
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Overview", BaseCurrency: "AUD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []application.AccountInput{
		{Name: "USD Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100.05"},
		{Name: "AUD Debt", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "25.10"},
	} {
		input.IncludeInNetWorth = true
		input.OwnerIDs = []domain.MemberID{bootstrap.Members[0].ID}
		if _, err := seed.CreateAccount(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []struct{ provider, rate string }{{application.FrankfurterProviderKey, "1.2345"}, {"other-provider", "10"}} {
		rate, err := domain.ParseFxRate(q.rate)
		if err != nil {
			t.Fatal(err)
		}
		quoted := now
		quote := domain.FXQuote{
			ID: domain.NewFXQuoteID(), HouseholdID: bootstrap.Household.ID, BaseCurrency: "USD", QuoteCurrency: "AUD", Rate: rate,
			SourceKind: domain.QuoteSourceProvider, SourceKey: q.provider, QuotedAt: quoted, CreatedAt: quoted,
		}
		if q.provider == application.FrankfurterProviderKey {
			quote.ObservationKind = string(application.FXObservationDailyReference)
			quote.EffectiveDate = "2026-08-17"
			quote.ValueEffectiveAt = now.Add(-3 * time.Hour)
			quote.QuotedAt, quote.CreatedAt = quote.ValueEffectiveAt, quote.ValueEffectiveAt
		}
		if err := repo.AppendFXQuote(ctx, quote); err != nil {
			t.Fatal(err)
		}
		if q.provider == application.FrankfurterProviderKey {
			// Preserve the daily reference alongside the other provider's latest
			// observation so both survive the repository's candidate projection.
			if _, err := db.SQL.ExecContext(ctx, `UPDATE fx_quotes SET observation_kind=?, effective_date=?, value_effective_at=?, source_policy_version=?, timestamp_basis=? WHERE id=?`,
				quote.ObservationKind, quote.EffectiveDate, quote.ValueEffectiveAt.Format(time.RFC3339), domain.FrankfurterV2BlendedPolicy, string(application.TimestampBasisPolicyDerived), quote.ID.String()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openCopy := func(registry ...application.MarketDataRegistryPort) *application.Service {
		t.Helper()
		copy := filepath.Join(t.TempDir(), "independent.db")
		if err := os.WriteFile(copy, bytes, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, copy)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		s := application.NewService(sqlite.NewRepository(db), registry...)
		s.SetClock(clock)
		s.SetQuoteCacheTTL(time.Hour)
		return s
	}
	provider := &unavailableOverviewProvider{}
	wails := openCopy(application.NewMarketDataRegistry(provider))
	if err := wails.SetFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	ref, err := portfolio.NewService(wails).Overview(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	gio := openCopy()
	if err := gio.SetStoredFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	page := newOverviewPage(ctx, gio, clock, func() {})
	accountsPage := newAccountsPage(ctx, gio, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page, accounts: accountsPage})
	t.Cleanup(accountsPage.workers.Wait)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	page.workers.Wait()
	h.frame()
	if page.current == nil {
		t.Fatalf("Gio Overview failed: %v", page.err)
	}
	value := page.current.value
	if !reflect.DeepEqual(wire.FromHouseholdDisplay(value.DisplayValuation), ref.DisplayValuation) || !reflect.DeepEqual(wire.FromMissingInputs(value.MissingInputs), ref.MissingInputs) ||
		value.Assets.String() != ref.Assets || value.Liabilities.String() != ref.Liabilities || value.NetWorth.String() != ref.NetWorth || value.Complete != ref.Complete || provider.requests != 0 {
		t.Fatalf("Gio returned %+v, Wails returned %+v, provider calls %d", value.DisplayValuation, ref, provider.requests)
	}
	if ref.NetWorth != "98.411725" || !ref.Complete || value.DisplayValuation.NetWorth.Quality.StaleCount == 0 {
		t.Fatalf("expected exact net worth with stale cached FX, got %+v", ref)
	}
	for _, label := range []string{"A$98.41", "A$123.51", "-A$25.10", "Some cached quotes await an update.", "Current estimate"} {
		assertPageLabel(t, &h.router, label)
	}
	model := page.allocation.model
	if len(model.rows) != len(ref.AssetComposition.Paths) || len(model.spec.Nodes) != len(ref.AssetComposition.Nodes) {
		t.Fatalf("path or node omitted by Gio: %+v", model)
	}
	row := model.rows[0]
	if row.account.id.String() != ref.AssetComposition.Paths[0].AccountID || row.amount != "A$123.51" {
		t.Fatalf("allocation row did not use the Wails path: %+v", row)
	}
	page.list.ScrollTo(page.compositionIndex())
	h.frame()
	scrollProductControl(t, h, "View allocation details")
	h.click("View allocation details", false)
	h.frame()
	if !page.allocation.table.open {
		t.Fatal("allocation disclosure did not open the chart table")
	}
	h.click("Open account · USD Cash", false)
	h.frame()
	if h.shell.trail.current().target.account != row.account.id {
		t.Fatal("allocation row did not open the exact account")
	}
	h.click("Back", false)
	h.frame()
	h.focused(page.allocation.buttons[string(row.id)])
	h.press(key.NameReturn)
	h.frame()
	if h.shell.trail.current().target.account != row.account.id {
		t.Fatal("restored allocation row could not reopen the account")
	}
}

func TestOverviewSankeyKeepsEveryPositivePathAndBusinessLabel(t *testing.T) {
	const count = 160
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	unit := decimal.RequireFromString("2.75")
	total := unit.Mul(decimal.NewFromInt(count))
	data := domain.OverviewResult{Currency: "USD", Assets: total,
		AssetComposition: domain.OverviewAssetComposition{Positive: total, MissingCount: 2, ZeroCount: 1}}
	c := &data.AssetComposition
	c.Nodes = []domain.OverviewAssetNode{
		{ID: "assets", Kind: "total", Amount: total},
		{ID: "assets/cash", ParentID: "assets", Key: "cash", Kind: "assetClass", Amount: total},
		{ID: "assets/cash/institution:", ParentID: "assets/cash", Kind: "institution", Amount: total},
	}
	for i := range count {
		id := fmt.Sprintf("account-%03d", i)
		c.Paths = append(c.Paths, domain.OverviewAssetPath{AssetClass: "cash", AccountID: id, AccountName: "Savings", Amount: unit})
		c.Nodes = append(c.Nodes, domain.OverviewAssetNode{ID: "assets/cash/institution:/account:" + id, ParentID: "assets/cash/institution:", Kind: "account", Key: id, Amount: unit})
	}
	model := overviewAssetAllocation(view, data)
	if len(model.spec.Validate()) != 0 || len(model.rows) != count || len(model.spec.Nodes) != count+3 ||
		len(model.spec.Links) != count+2 || len(model.accounts) != count*2 {
		t.Fatal("dense allocation lost or combined real paths")
	}
	last := model.rows[count-1]
	if last.account.id != domain.AccountID("account-159") || last.account.label != "Savings (160)" || last.institution != "Unassigned institution" ||
		last.amount != "US$2.75" || model.spec.Nodes[0].Label != "Known subtotal" {
		t.Fatalf("last real path and partial summary: %+v, %+v", last, model.spec.Nodes[0])
	}
	node := charts.ID(c.Nodes[len(c.Nodes)-1].ID)
	if model.accounts[node] != last.account || model.accounts[charts.ID("/")+node] != last.account {
		t.Fatal("chart node and path do not target the same account")
	}
}

func TestOverviewSankeyPreservesBothPathsToOneAccountAndSmallFlow(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "allocation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := application.NewService(sqlite.NewRepository(db))
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic allocation paths", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	makeAccount := func(name, accountType, tracking, amount string) domain.AccountID {
		t.Helper()
		record, err := service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: accountType, TrackingMode: tracking, BalanceSheetRole: "asset",
			DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: amount, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	mixed := makeAccount("Mixed holdings", "brokerage", "holdings", "")
	small := makeAccount("Small flow", "bank_account", "balance", "0.25")
	if _, err := service.AppendAccountCashValue(ctx, mixed, "123.45", "USD", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	fund, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Sample equity", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, application.HoldingInput{AccountID: mixed.String(), InstrumentID: fund.ID.String(), Quantity: "2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, fund.ID, "45.67", now.Format(time.RFC3339Nano), false); err != nil {
		t.Fatal(err)
	}
	read, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := portfolio.NewService(service).Overview(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	got := overviewAssetAllocation(view, read)
	if len(got.spec.Validate()) != 0 || len(got.rows) != 3 || len(got.rows) != len(reference.AssetComposition.Paths) ||
		len(got.spec.Nodes) != len(reference.AssetComposition.Nodes) {
		t.Fatalf("projection and Wails path counts: %+v", got.rows)
	}
	want := map[string]string{mixed.String() + "/cash": "123.45", mixed.String() + "/stock": "91.34", small.String() + "/cash": "0.25"}
	total := decimal.Zero
	for i, path := range reference.AssetComposition.Paths {
		amount, ok := want[path.AccountID+"/"+path.AssetClass]
		if !ok || path.Amount != amount || got.rows[i].account.id.String() != path.AccountID ||
			got.rows[i].amount != "US$"+amount {
			t.Fatalf("allocation row %d = %+v, Wails = %+v", i, got.rows[i], path)
		}
		total = total.Add(decimal.RequireFromString(path.Amount))
	}
	leaves := map[domain.AccountID]int{}
	for _, link := range got.spec.Links {
		target, ok := got.accounts[link.Target]
		if !ok {
			continue
		}
		if target.id.String() == string(link.Target) || got.accounts[link.ID] != target || !link.Value.Valid() || link.Value.Number <= 0 {
			t.Fatalf("path-scoped link or exact account route lost: %+v, %+v", link, target)
		}
		leaves[target.id]++
	}
	if leaves[mixed] != 2 || leaves[small] != 1 || !total.Equal(read.AssetComposition.Positive) ||
		total.String() != "215.04" || total.String() != reference.Assets || got.spec.Nodes[0].Text != "US$215.04" {
		t.Fatalf("leaf coverage %v, exact total %s, Wails assets %s", leaves, total, reference.Assets)
	}
}

func TestOverviewSankeyDistinguishesZeroNegativeAndMissingValues(t *testing.T) {
	for _, tt := range []struct {
		name          string
		amount        decimal.Decimal
		zero, missing int
		labels        []string
	}{
		{"zero", decimal.Zero, 1, 0, []string{"Total assets · US$0.00", "Zero-value asset components · 1", "No positive asset values to chart."}},
		{"missing", decimal.Zero, 0, 1, []string{"Asset components without a base-currency value · 1", "Asset values are unavailable."}},
		{"negative", decimal.RequireFromString("-12.25"), 0, 1, []string{"Negative components · -US$12.25", "Known subtotal · -US$12.25", "No positive asset values to chart."}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			quoteDate, carriedDate := "2026-11-01", "2026-10-29"
			read := overviewRead{readAt: time.Now(), value: domain.OverviewResult{Currency: "USD", AccountCount: 1, Assets: tt.amount,
				ValuationReadDate: "2026-11-25",
				DisplayValuation: domain.HouseholdDisplayValuation{Assets: domain.ValuationAmount{Quality: domain.ValuationQuality{
					OldestMarketDate: &carriedDate, NewestMarketDate: &quoteDate, OldestCarriedMarketDate: &carriedDate, CarriedCount: 1,
				}}},
				AssetComposition: domain.OverviewAssetComposition{Negative: tt.amount, ZeroCount: tt.zero, MissingCount: tt.missing}}}
			p := newOverviewPage(t.Context(), nil, time.Now, func() {})
			p.current = &read
			h := newShellHarnessWithPages(t, ledgerPages{overview: p})
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			h.frame()
			for _, label := range tt.labels {
				assertPageLabel(t, &h.router, label)
			}
			if len(p.allocation.model.spec.Links) != 0 || findPageLabel(&h.router, "View allocation details") != nil {
				t.Fatal("nonpositive or unavailable assets appeared as chart paths")
			}
		})
	}
}

// The allocation table behind a dense chart keeps every real path, lays out
// only the rows near the viewport, and still opens the last account.
func TestOverviewAllocationDetailsVirtualizeLongTables(t *testing.T) {
	const count = 600
	unit := decimal.RequireFromString("2.75")
	total := unit.Mul(decimal.NewFromInt(count))
	data := domain.OverviewResult{Currency: "USD", AccountCount: count, Assets: total,
		AssetComposition: domain.OverviewAssetComposition{Positive: total}}
	c := &data.AssetComposition
	c.Nodes = []domain.OverviewAssetNode{
		{ID: "assets", Kind: "total", Amount: total},
		{ID: "assets/cash", ParentID: "assets", Key: "cash", Kind: "assetClass", Amount: total},
		{ID: "assets/cash/institution:bank", ParentID: "assets/cash", Key: "bank", Kind: "institution", Amount: total},
	}
	for i := range count {
		id := fmt.Sprintf("account-%03d", i)
		c.Paths = append(c.Paths, domain.OverviewAssetPath{AssetClass: "cash", InstitutionID: "bank", InstitutionName: "Synthetic bank",
			AccountID: id, AccountName: fmt.Sprintf("Synthetic account %03d", i), Amount: unit})
		c.Nodes = append(c.Nodes, domain.OverviewAssetNode{ID: "assets/cash/institution:bank/account:" + id, ParentID: "assets/cash/institution:bank", Kind: "account", Key: id, Amount: unit})
	}
	read := overviewRead{readAt: time.Now(), value: data}
	p := newOverviewPage(t.Context(), nil, time.Now, func() {})
	p.current = &read
	h := newShellHarnessWithPages(t, ledgerPages{overview: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1000, 950)
	var opened domain.AccountID
	p.allocation.openAccount = func(id domain.AccountID, _ string, _ event.Tag) { opened = id }
	h.frame()
	table := p.allocation.pathTable(ui.View{Language: i18n.English})
	if len(table.Rows) != count || p.dimensionsIndex() != p.compositionIndex()+1 {
		t.Fatalf("details contain %d paths; expected %d inline chart rows", len(table.Rows), count)
	}
	p.list.ScrollTo(p.compositionIndex())
	h.frame()
	p.allocation.table.open = true
	h.frame()
	if laid := len(p.allocation.buttons); laid == 0 || laid > 40 {
		t.Fatalf("laid out %d detail rows at the top of a %d-row table; want only the visible ones", laid, count)
	}
	p.allocation.table.list.ScrollTo(count - 1)
	h.frame()
	if laid := len(p.allocation.buttons); laid > 80 {
		t.Fatalf("laid out %d detail rows after jumping to the end; want only rows near the viewport", laid)
	}
	last := p.allocation.buttons[string(p.allocation.model.rows[count-1].id)]
	if last == nil {
		t.Fatal("the table did not render its last account link after scrolling")
	}
	h.router.Source().Execute(key.FocusCmd{Tag: last})
	h.frame()
	h.focused(last)
	h.press(key.NameReturn)
	if opened != "account-599" {
		t.Fatalf("last allocation row opened %q, want account-599", opened)
	}
}
