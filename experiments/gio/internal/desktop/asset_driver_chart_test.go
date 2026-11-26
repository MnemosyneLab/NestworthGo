package desktop

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAssetWaterfallUsesExactDriversAndReportedEnding(t *testing.T) {
	money := func(text string) *domain.SignedMoney {
		t.Helper()
		amount, err := domain.NewSignedMoney(decimal.RequireFromString(text), "USD")
		if err != nil {
			t.Fatal(err)
		}
		return &amount
	}
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	data := application.AssetChangeResult{Summary: application.AssetChangeSummary{BeginningValue: money("100"), EndingValue: money("102.75")},
		Waterfall: []application.AssetChangeRow{
			{Key: "increase", Bucket: domain.BucketPriceChange, Amount: money("5.25")},
			{Key: "zero", Amount: money("0")},
			{Key: "decrease", Bucket: domain.BucketResidual, Amount: money("-2.50")},
			{Key: "missing"},
		}}
	spec, mismatch := assetWaterfall(view, data)
	if mismatch || len(spec.Steps) != 4 || len(spec.Validate()) != 0 || len(spec.Breaks()) != 0 {
		t.Fatalf("expected beginning, two drivers and a reconciled ending: %+v", spec)
	}
	if step := spec.Steps[1]; step.From != charts.Num(100) || step.To != charts.Num(105.25) || step.Text != "US$5.25" {
		t.Fatalf("positive step lost its exact levels: %+v", step)
	}
	if step := spec.Steps[2]; step.From != charts.Num(105.25) || step.To != charts.Num(102.75) || step.Text != "-US$2.50" || step.Tone != charts.ToneWarning {
		t.Fatalf("residual step lost its exact levels or warning: %+v", step)
	}
	data.Summary.EndingValue = money("102.755")
	if _, mismatch := assetWaterfall(view, data); mismatch {
		t.Fatal("the ending difference rounds to zero at the Wails comparison precision")
	}
	data.Summary.EndingValue = money("102.76")
	spec, mismatch = assetWaterfall(view, data)
	if !mismatch || spec.Steps[3].To != charts.Num(102.76) || spec.Steps[2].To != charts.Num(102.75) {
		t.Fatal("the chart concealed the discrepancy or replaced the reported ending")
	}
	data.Summary.BeginningValue = nil
	if empty, _ := assetWaterfall(view, data); len(empty.Steps) != 0 {
		t.Fatal("the chart invented a beginning level")
	}
}
