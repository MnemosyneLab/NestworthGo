package desktop

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestAssetDriverSummaryKeepsScopeAndFigures(t *testing.T) {
	rate := decimal.RequireFromString("0.004949009901")
	report := application.AssetChangeResult{Summary: application.AssetChangeSummary{
		BeginningValue: assetTestMoney(t, "40400", "USD"),
		EndingValue:    assetTestMoney(t, "40599.9375", "USD"),
		Change:         assetTestMoney(t, "199.9375", "USD"),
		ChangeRate:     &rate,
	}}
	for _, tc := range []struct {
		scope    domain.ScopeKind
		language i18n.Language
		title    string
		labels   [4]string
	}{
		{domain.ScopeHousehold, i18n.English, "Net Worth Change", [4]string{"Beginning value", "Ending value", "Change", "Period change (%)"}},
		{domain.ScopeAccount, i18n.English, "Account Value Change", [4]string{"Beginning value", "Ending value", "Change", "Period change (%)"}},
		{domain.ScopeInstrument, i18n.SimplifiedChinese, "投资资产变化", [4]string{"期初价值", "期末价值", "变化", "区间变化率"}},
	} {
		t.Run(string(tc.scope), func(t *testing.T) {
			view := ui.View{Theme: ui.NewTheme(), Language: tc.language, Colors: &lightPalette, DecimalSeparator: ".", GroupingSeparator: ","}
			gtx := chartTableContext(650)
			assetDriverSummary(view, report, tc.scope)(gtx)
			var router input.Router
			router.Frame(gtx.Ops)
			assertPageLabel(t, &router, tc.title)
			for _, label := range tc.labels {
				assertPageLabel(t, &router, label)
			}
			for _, figure := range []string{"US$40,400.00", "US$40,599.94", "+US$199.94", "+0.49%"} {
				assertPageLabel(t, &router, figure)
			}
		})
	}
}

func TestAssetCategoryEmptyHasLocalizedNextStepWithoutAnInventedTotal(t *testing.T) {
	for _, tc := range []struct {
		language i18n.Language
		result   application.CategoriesResult
		title    string
		hint     string
	}{
		{i18n.English, application.CategoriesResult{AnalysisAvailability: application.AnalysisAvailability{Available: true}},
			"No category data for this period.", "Try a different period or category type."},
		{i18n.SimplifiedChinese, application.CategoriesResult{AnalysisAvailability: application.AnalysisAvailability{Available: true}},
			"此期间没有分类数据。", "请尝试其他期间或分类类型。"},
		{i18n.English, application.CategoriesResult{}, "No investment assets are available for this scope.", "Add an investment position or choose another scope."},
		{i18n.English, application.CategoriesResult{Rows: []application.CategoryRow{{Key: "cash"}}},
			"Not enough historical data to calculate returns.", "Try a more recent period."},
	} {
		t.Run(tc.title, func(t *testing.T) {
			view := ui.View{Theme: ui.NewTheme(), Language: tc.language, Colors: &lightPalette}
			gtx := chartTableContext(650)
			ui.Column{}.Layout(gtx, new(assetChangesPage).categoryRows(view, tc.result)...)
			var router input.Router
			router.Frame(gtx.Ops)
			assertPageLabel(t, &router, tc.title)
			assertPageLabel(t, &router, tc.hint)
			if findPageLabel(&router, view.Language.Text(i18n.AssetCategoryTotal)+" · "+view.Language.Text(i18n.OverviewNoValue)) != nil {
				t.Fatal("empty category includes an invented total")
			}
		})
	}
}

func TestAssetCategoryTotalRendersPlainLocalizedLabelAndMoney(t *testing.T) {
	for _, tc := range []struct {
		language i18n.Language
		label    string
		coverage string
	}{
		{i18n.English, "Total", "One or more asset days are incomplete."},
		{i18n.SimplifiedChinese, "合计", "此期间有部分日期的资产数据不完整。"},
	} {
		for _, figure := range []struct {
			amount   string
			currency domain.CurrencyCode
			display  string
		}{
			{"1234.5678", "USD", "+US$1,234.57"},
			{"-1234.5649", "AUD", "-A$1,234.56"},
		} {
			t.Run(string(tc.language)+"/"+figure.amount, func(t *testing.T) {
				total, err := domain.ParseSignedMoney(figure.amount, figure.currency)
				if err != nil {
					t.Fatal(err)
				}
				view := ui.View{Theme: ui.NewTheme(), Language: tc.language, Colors: &lightPalette, DecimalSeparator: ".", GroupingSeparator: ","}
				page := new(assetChangesPage)
				gtx := chartTableContext(650)
				gtx.Constraints.Min.Y = 0
				result := application.CategoriesResult{Total: &total, AnalysisAvailability: application.AnalysisAvailability{
					Available: true, Status: domain.CompletenessPartial, MissingReason: "one or more asset days are incomplete",
				}}
				ui.Column{}.Layout(gtx, page.categoryRows(view, result)...)
				var router input.Router
				router.Frame(gtx.Ops)
				assertPageLabel(t, &router, tc.label+" · "+figure.display)
				assertPageLabel(t, &router, tc.coverage)
				if tc.language.Text(i18n.AssetCategoryTotal) != tc.label || total.CanonicalAmount() != figure.amount {
					t.Fatal("category display changed the translation key or the exact amount")
				}
			})
		}
	}
}

type assetChangesReadFault struct {
	*application.Service
	origin, categories bool
}

func (r *assetChangesReadFault) HistoryOrigin(ctx context.Context) (*domain.HistoryOrigin, error) {
	if r.origin {
		r.origin = false
		return nil, errors.New("synthetic origin read failure")
	}
	return r.Service.HistoryOrigin(ctx)
}

func (r *assetChangesReadFault) Categories(ctx context.Context, query domain.AnalysisQuery, category application.AnalysisCategoryType) (application.CategoriesResult, error) {
	if r.categories {
		r.categories = false
		return application.CategoriesResult{}, errors.New("synthetic categories read failure")
	}
	return r.Service.Categories(ctx, query, category)
}

func TestAssetChangesRetriesBeforeAnySuccessfulResult(t *testing.T) {
	service, _, _, _ := openPortfolioFixture(t, "complete")
	reader := &assetChangesReadFault{Service: service, origin: true, categories: true}
	page := &assetChangesPage{ctx: t.Context(), reader: reader, clock: func() time.Time { return accountTestClock }, invalidate: func() {}}
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{assets: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1280, 820)
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		for range 4 {
			page.workers.Wait()
			h.frame()
		}
	}
	h.frame()
	click("Asset changes")
	assertPageLabel(t, &h.router, "Could not load asset changes.")
	click("Retry")
	if page.err != nil || page.current == nil || page.current.trend == nil {
		t.Fatal("retry did not load the asset trend after the origin was unavailable")
	}
	click("Categories")
	assertPageLabel(t, &h.router, "Could not load asset changes.")
	click("Retry")
	if page.err != nil || page.current == nil || page.current.categories == nil || page.categoryType() != application.CategorySpending {
		t.Fatal("retry did not load the selected spending category")
	}
}
