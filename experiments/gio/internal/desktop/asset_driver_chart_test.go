package desktop

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAssetDriverScaleUsesOnlySignedContributions(t *testing.T) {
	money := func(text string) *domain.SignedMoney { return assetTestMoney(t, text, "USD") }
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	data := application.AssetChangeResult{Summary: application.AssetChangeSummary{BeginningValue: money("40400.1234"), EndingValue: money("40402.8734")},
		Waterfall: []application.AssetChangeRow{
			{Key: "increase", Bucket: domain.BucketPriceChange, Amount: money("5.25")},
			{Key: "zero", Amount: money("0")},
			{Key: "decrease", Bucket: domain.BucketResidual, Amount: money("-2.50")},
			{Key: "missing"},
		}}
	spec := assetDriverBars(view, data)
	if !spec.Symmetric || len(spec.Bars) != 2 || len(spec.Validate()) != 0 || assetDriverNotice(data) != i18n.NoMessage {
		t.Fatalf("expected two reconciled drivers sharing a centered zero: %+v", spec)
	}
	if bar := spec.Bars[0]; bar.ID != "increase" || bar.Value != charts.Num(5.25) || bar.Text != "+US$5.25" {
		t.Fatalf("positive contribution lost its value or sign: %+v", bar)
	}
	if bar := spec.Bars[1]; bar.ID != "decrease" || bar.Value != charts.Num(-2.50) || bar.Text != "-US$2.50" || bar.Tone != charts.ToneWarning {
		t.Fatalf("negative contribution lost its value, sign, or warning: %+v", bar)
	}
	data.Summary.BeginningValue = money("50000000")
	data.Summary.EndingValue = money("50000002.755")
	if assetDriverNotice(data) != i18n.NoMessage {
		t.Fatal("the ending difference rounds to zero at the Wails comparison precision")
	}
	data.Summary.EndingValue = money("50000002.76")
	if assetDriverNotice(data) != i18n.AssetDriverMismatch {
		t.Fatal("the summary concealed a discrepancy in the reported ending value")
	}
	if updated := assetDriverBars(view, data); !reflect.DeepEqual(updated.Bars, spec.Bars) {
		t.Fatalf("boundary figures changed the chart geometry: %+v", updated.Bars)
	}
	data.Summary.BeginningValue = nil
	if assetDriverNotice(data) != i18n.AssetDriverIncomplete || !reflect.DeepEqual(assetDriverBars(view, data).Bars, spec.Bars) {
		t.Fatal("a missing beginning value hid known contributions")
	}
	data.Summary.BeginningValue = money("50000000")
	data.Waterfall[2].Amount = assetTestMoney(t, "-2.50", "AUD")
	spec = assetDriverBars(view, data)
	if assetDriverNotice(data) != i18n.AssetDriverMismatch || spec.Bars[0].Value.Valid() || spec.Bars[1].Value.Valid() ||
		spec.Bars[0].Text != "+US$5.25" || spec.Bars[1].Text != "-A$2.50" {
		t.Fatalf("mixed currencies shared a scale or lost the known amounts: %+v", spec.Bars)
	}
}

func assetTestMoney(t *testing.T, text string, currency domain.CurrencyCode) *domain.SignedMoney {
	t.Helper()
	amount, err := domain.ParseSignedMoney(text, currency)
	if err != nil {
		t.Fatal(err)
	}
	return &amount
}
