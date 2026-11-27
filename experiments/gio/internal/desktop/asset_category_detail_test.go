package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
)

type retryCategoryReader struct {
	*application.Service
	fail bool
}

func (r *retryCategoryReader) CategoryDetail(ctx context.Context, q domain.AnalysisQuery, kind application.AnalysisCategoryType, row string) (application.CategoryDetailResult, error) {
	if r.fail {
		r.fail = false
		return application.CategoryDetailResult{}, errors.New("synthetic category detail error")
	}
	return r.Service.CategoryDetail(ctx, q, kind, row)
}

func TestAssetCategoryRecordsMatchWailsAndRestoreOriginFocus(t *testing.T) {
	ctx := t.Context()
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sqlite.Open(path)
	check(err)
	t.Cleanup(func() { _ = db.Close() })
	seed := testfixture.NewService(&testfixture.BaselineRepository{Repository: testfixture.Repository(db)})
	testfixture.SetClock(seed, clock)
	check(seed.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic asset categories", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}))
	boot, err := seed.Bootstrap(ctx)
	check(err)
	bank, err := testfixture.CreateAccount(seed, ctx, application.AccountInput{
		Name: "Synthetic expense account", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	check(err)
	_, err = testfixture.CaptureSyntheticOrigin(seed, ctx, "UTC")
	check(err)
	now = now.AddDate(0, 0, 1)
	amount, err := domain.ParseMoney("25.1234", "USD")
	check(err)
	note := "Synthetic category receipt"
	activity, err := seed.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: boot.Household.ID,
		AccountID: bank.Account.ID, Amount: amount, EffectiveAt: now, Reason: domain.ReasonExpense, Note: &note})
	check(err)
	now = now.AddDate(0, 0, 2)
	check(db.CheckpointWAL(ctx))
	check(db.Close())
	data, err := os.ReadFile(path)
	check(err)
	openCopy := func() *application.Service {
		t.Helper()
		copyPath := filepath.Join(t.TempDir(), "copy.db")
		check(os.WriteFile(copyPath, data, 0o600))
		copyDB, err := sqlite.Open(copyPath)
		check(err)
		t.Cleanup(func() { _ = copyDB.Close() })
		service := testfixture.NewService(&testfixture.BaselineRepository{Repository: testfixture.Repository(copyDB)})
		testfixture.SetClock(service, clock)
		return service
	}

	wails := analysis.NewService(openCopy())
	gio := openCopy()
	p := &assetChangesPage{ctx: ctx, reader: &retryCategoryReader{Service: gio, fail: true}, clock: clock, invalidate: func() {}}
	ledger := newLedgerHistory(ctx, gio, clock, func() {})
	t.Cleanup(p.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	t.Cleanup(ledger.reads.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{assets: p, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1160, 850)
	h.frame()
	h.click("Asset changes", false)
	p.workers.Wait()
	h.frame()
	h.click("Categories", false)
	p.workers.Wait()
	h.frame()
	if p.categoryType() != application.CategorySpending || p.current == nil || p.current.categories == nil || len(p.current.categories.Rows) != 1 {
		t.Fatal("spending did not select the seeded category")
	}
	assertPageLabel(t, &h.router, "Total · -US$25.12")
	assertPageLabel(t, &h.router, "Synthetic expense account · -US$25.12")
	row := p.current.categories.Rows[0]
	button := p.categoryButtons[row.Key]
	h.click("Synthetic expense account · -US$25.12", false)
	p.workers.Wait()
	h.frame()
	if p.categoryDetail == nil || p.categoryDetail.err == nil {
		t.Fatal("category failure was not retained")
	}
	h.click("Retry", false)
	p.workers.Wait()
	h.frame()
	sheet := p.categoryDetail
	if sheet.value == nil || len(sheet.value.ActivityRefs) != 1 {
		t.Fatal("retry did not load the category details")
	}
	request := analysis.AnalysisQueryRequest{From: sheet.query.From, To: sheet.query.To, ScopeKind: string(sheet.query.Scope.Kind), ScopeID: sheet.query.Scope.ID, Valuation: string(sheet.query.Valuation), Basis: string(sheet.query.Basis), IncludeCash: sheet.query.IncludeCash}
	reference, err := wails.CategoryDetail(ctx, request, string(application.CategorySpending), row.Key)
	check(err)
	if len(reference.ActivityRefs) != 1 {
		t.Fatalf("Wails refs = %+v", reference.ActivityRefs)
	}
	actual, expected := sheet.value.ActivityRefs[0], reference.ActivityRefs[0]
	if actual.Amount == nil || expected.Amount == nil || actual.Amount.CanonicalAmount() != "-25.1234" ||
		expected.Amount.Amount != "-25.1234" || expected.Date != actual.Date || expected.AccountID != actual.AccountID ||
		expected.ActivityID != activity.Activity.ID.String() || expected.ActivityID != actual.ActivityID {
		t.Fatalf("Gio ref %+v does not match Wails %+v", actual, expected)
	}
	h.size = image.Pt(650, 480)
	h.frame()
	assertPageLabel(t, &h.router, "Records")
	h.router.Source().Execute(key.FocusCmd{Tag: &sheet.links[0]})
	h.frame()
	h.focused(&sheet.links[0])
	h.press(key.NameReturn)
	h.frame()
	if target := h.shell.trail.current(); h.shell.activeRoute() != historyRoute ||
		target.target.activity != activity.Activity.ID || target.historyFilters.From != "2026-08-02" ||
		target.historyFilters.To != "2026-08-02" || target.historyFilters.Account != bank.Account.ID {
		t.Fatalf("category opened the wrong Ledger target: %+v", target)
	}
	ledger.reads.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, note)
	h.click("Back", false)
	p.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != assetChangesRoute || p.state.tab != assetCategoriesTab ||
		p.categoryType() != application.CategorySpending || p.current.query != sheet.query {
		t.Fatal("Back lost the category and active filters")
	}
	h.focused(button)
	h.press(key.NameReturn)
	p.workers.Wait()
	h.frame()
	if p.categoryDetail == nil || p.categoryDetail.row.Key != row.Key {
		t.Fatal("keyboard could not reopen the original category")
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(button)
	h.press(key.NameReturn)
	p.workers.Wait()
	h.frame()
	h.click("View in Ledger activity", false)
	ledger.reads.workers.Wait()
	h.frame()
	h.click("Back to ledger activity", false)
	ledger.model.Wait()
	h.frame()
	filters := ledger.model.Filters()
	if h.shell.trail.current().target.activity != "" || filters.Account != bank.Account.ID ||
		filters.From != "2026-08-02" || filters.To != "2026-08-02" {
		t.Fatalf("closing the activity lost its scoped timeline: %+v", filters)
	}
}
