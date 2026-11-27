package desktop

import (
	"image"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestOverviewBreakdownChartsUseSharedAmountsAndShares(t *testing.T) {
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	liability := overviewSummary{label: i18n.OverviewLiabilitiesComposition, total: decimal.RequireFromString("25.10"), liability: true,
		items: []domain.BreakdownItem{{Key: domain.BucketCreditCard, Amount: decimal.RequireFromString("25.10"), ShareBPS: 10000}}}
	donut, _, useDonut := overviewSummarySpecs(view, liability, "USD")
	if !useDonut || donut.CenterValue != "-US$25.10" || donut.Rows()[0].Value != "-US$25.10" || donut.Rows()[0].Detail[0] != "100.00%" {
		t.Fatalf("liability summary = %+v", donut)
	}
	assets := overviewSummary{label: i18n.OverviewAssetsComposition, total: decimal.RequireFromString("3.23"),
		items: []domain.BreakdownItem{
			{Key: domain.BucketCash, Amount: decimal.RequireFromString("5.25"), ShareBPS: 16254},
			{Key: domain.BucketOtherAsset, Amount: decimal.RequireFromString("-2.02"), ShareBPS: -6254},
		}}
	donut, bars, useDonut := overviewSummarySpecs(view, assets, "USD")
	if useDonut || donut.CenterValue != "US$3.23" || bars.Rows()[0].Value != "US$5.25" || bars.Rows()[1].Value != "-US$2.02" || bars.Rows()[1].Detail[0] != "-62.54%" {
		t.Fatalf("signed asset summary = %+v", bars)
	}
}

func TestOverviewDimensionsFilterAccountsAndRestoreCompactFocus(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "dimensions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := application.NewService(sqlite.NewRepository(db))
	clock := func() time.Time { return time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC) }
	s.SetClock(clock)
	if err := s.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic allocation links", BaseCurrency: "USD", MemberNames: []string{"Sample owner", "Other owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	group, err := s.CreateGroup(ctx, "Sample group")
	if err != nil {
		t.Fatal(err)
	}
	institution, err := s.CreateInstitution(ctx, "Sample bank", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	owners := make(map[string]domain.MemberID)
	for _, member := range bootstrap.Members {
		owners[member.Name] = member.ID
	}
	makeAccount := func(name, kind, amount string, member domain.MemberID, bank domain.InstitutionID, group domain.GroupID) domain.AccountID {
		t.Helper()
		record, err := s.CreateAccount(ctx, application.AccountInput{Name: name, AccountType: kind, BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD",
			InitialAmount: amount, OwnerIDs: []domain.MemberID{member}, InstitutionID: bank.String(), GroupID: group.String(), IncludeInNetWorth: true})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	first := makeAccount("Selected savings", "bank_account", "100.25", owners["Sample owner"], institution.ID, group.ID)
	other := makeAccount("Unassigned cash", "cash_on_hand", "25.00", owners["Other owner"], "", "")
	page := newOverviewPage(ctx, s, clock, func() {})
	accounts := newAccountsPage(ctx, s, clock, func() {})
	t.Cleanup(accounts.workers.Wait)
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{overview: page, accounts: accounts})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1260, 1050)
	h.frame()
	page.workers.Wait()
	h.frame()
	if findPageLabel(&h.router, "View allocation breakdowns") != nil {
		t.Fatal("Overview still shows the page-level breakdown shortcut")
	}
	page.list.ScrollTo(page.dimensionsIndex())
	h.frame()
	bank := accountListScope{accountByInstitution, institution.ID.String()}
	for _, label := range []string{"By institution", "By account type", "By member", "By group"} {
		assertPageLabel(t, &h.router, label)
	}
	findLabel := func(scope accountListScope, name string, pair int) string {
		t.Helper()
		result := page.current.value
		for _, item := range overviewDimensions(result)[pair*2+(int(scope.dimension)-1)%2].items {
			if breakdownAccountScope(scope.dimension, item.Key) == scope {
				view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
				return name + " · " + overviewBreakdownMoney(view, result.Currency, item.Amount, false) + " · " + overviewBreakdownShare(view, item)
			}
		}
		t.Fatalf("Overview has no %q in dimension %d", scope.key, scope.dimension)
		return ""
	}
	for _, tc := range []struct {
		scope accountListScope
		name  string
		want  domain.AccountID
		pair  int
	}{
		{accountListScope{accountByInstitution, institution.ID.String()}, "Sample bank", first, 0},
		{accountListScope{accountByType, "bank_account"}, "Bank account", first, 0},
		{accountListScope{accountByMember, owners["Sample owner"].String()}, "Sample owner", first, 1},
		{accountListScope{accountByGroup, group.ID.String()}, "Sample group", first, 1},
		{accountListScope{dimension: accountByInstitution}, "Unassigned institution", other, 0},
		{accountListScope{dimension: accountByGroup}, "No group", other, 1},
	} {
		label := findLabel(tc.scope, tc.name, tc.pair)
		page.list.ScrollTo(page.dimensionsIndex() + tc.pair)
		h.frame()
		h.click(label, false)
		h.frame()
		accounts.workers.Wait()
		h.frame()
		var ids []domain.AccountID
		for _, row := range accounts.current.rows {
			ids = append(ids, row.record.Account.ID)
		}
		if !slices.Equal(ids, []domain.AccountID{tc.want}) {
			t.Fatalf("%s matched %v, want %s", label, ids, tc.want)
		}
		if tc.scope.dimension != accountByInstitution || tc.scope.unassigned() {
			assertPageLabel(t, &h.router, "Filtered accounts · "+tc.name)
		}
		h.click("Back", false)
		h.frame()
		h.focused(page.breakdowns.links[tc.scope])
	}
	h.size = image.Pt(700, 480)
	page.list.ScrollTo(page.dimensionsIndex())
	h.frame()
	h.router.Source().Execute(key.FocusCmd{Tag: page.breakdowns.links[bank]})
	h.frame()
	h.focused(page.breakdowns.links[bank])
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(page.breakdowns.links[accountListScope{dimension: accountByInstitution}])
	h.router.MoveFocus(key.FocusBackward)
	h.frame()
	h.focused(page.breakdowns.links[bank])
	h.press(key.NameReturn)
	h.frame()
	accounts.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Selected savings")
	h.click("Selected savings", false)
	h.frame()
	accounts.workers.Wait()
	h.frame()
	if accounts.target.account != first {
		t.Fatal("account detail lost the exact account ID")
	}
	h.click("Back to accounts", false)
	h.frame()
	accounts.workers.Wait()
	h.frame()
	if accounts.target.institution != institution.ID || len(accounts.current.rows) != 1 {
		t.Fatal("account detail Back lost its source filter")
	}
	h.click("Back", false)
	h.frame()
	h.focused(page.breakdowns.links[bank])
	h.press(key.NameReturn)
	h.frame()
	accounts.workers.Wait()
	h.frame()
	if accounts.target.institution != institution.ID {
		t.Fatal("restored Overview focus reopened a different filter")
	}
}

// Opening one composition card's data table stretches its peer in the same
// row; closing it must return both cards to their original height at the same
// window width, column count and card count.
func TestOverviewCompositionRowShrinksAfterDataTableCloses(t *testing.T) {
	view := ui.View{Theme: ui.NewTheme(), Language: i18n.English, Colors: &lightPalette, DecimalSeparator: ".", GroupingSeparator: ","}
	amount := decimal.RequireFromString
	result := domain.OverviewResult{Currency: "USD", Assets: amount("1500"), Liabilities: amount("300"),
		AssetsByType: []domain.BreakdownItem{
			{Key: domain.BucketCash, Amount: amount("600"), ShareBPS: 4000},
			{Key: domain.BucketStock, Amount: amount("450"), ShareBPS: 3000},
			{Key: domain.BucketOtherAsset, Amount: amount("300"), ShareBPS: 2000},
			{Key: domain.BucketBond, Amount: amount("150"), ShareBPS: 1000},
		},
		LiabilitiesByType: []domain.BreakdownItem{{Key: domain.BucketCreditCard, Amount: amount("300"), ShareBPS: 10000}}}
	var b overviewBreakdowns
	columns := (ui.Grid{MinColumn: 350, MaxColumns: 2, Gap: 16, Balanced: true}).Columns(chartTableContext(900), 900, 2)
	// A stretched row settles once two frames in a row agree on its height.
	settle := func() int {
		previous := -1
		for range 5 {
			height := b.composition(chartTableContext(900), view, result).Size.Y
			if height == previous {
				return height
			}
			previous = height
		}
		t.Fatal("the composition grid kept asking for frames")
		return 0
	}
	if columns != 2 || len(overviewSummaries(result)) != 2 {
		t.Fatalf("columns = %d, cards = %d; want two cards in one row", columns, len(overviewSummaries(result)))
	}
	closed := settle()
	b.summaries[0].table.open = true
	open := settle()
	if open <= closed {
		t.Fatalf("open data table height %d did not grow the row from %d", open, closed)
	}
	b.summaries[0].table.open = false
	if again := settle(); again != closed {
		t.Fatalf("closed again = %d, want both cards back to %d", again, closed)
	}
}
