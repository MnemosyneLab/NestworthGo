package desktop

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/io/input"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestAssetCategoryTotalRendersPlainLocalizedLabelAndMoney(t *testing.T) {
	for _, tc := range []struct {
		language i18n.Language
		label    string
		coverage string
	}{
		{i18n.English, "Category total", "One or more asset days are incomplete."},
		{i18n.SimplifiedChinese, "分类合计", "此期间有部分日期的资产数据不完整。"},
	} {
		for _, figure := range []struct {
			amount   string
			currency domain.CurrencyCode
			display  string
		}{
			{"1234.5678", "USD", "US$1,234.57"},
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
