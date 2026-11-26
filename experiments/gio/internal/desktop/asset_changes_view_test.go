package desktop

import (
	"testing"

	"gioui.org/io/input"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAssetCategoryTotalRendersPlainLocalizedLabelAndMoney(t *testing.T) {
	for _, tc := range []struct {
		language i18n.Language
		label    string
	}{
		{i18n.English, "Category total"},
		{i18n.SimplifiedChinese, "分类合计"},
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
				ui.Column{}.Layout(gtx, page.categoryRows(view, application.CategoriesResult{Total: &total})...)
				var router input.Router
				router.Frame(gtx.Ops)
				assertPageLabel(t, &router, tc.label+" · "+figure.display)
				if tc.language.Text(i18n.AssetCategoryTotal) != tc.label || total.CanonicalAmount() != figure.amount {
					t.Fatal("category display changed the translation key or the exact amount")
				}
			})
		}
	}
}
