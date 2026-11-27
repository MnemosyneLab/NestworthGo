package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func assetDriverBars(view ui.View, data application.AssetChangeResult) charts.BarSpec {
	l := view.Language
	spec := charts.BarSpec{Label: l.Text(i18n.AssetDriverChart), Summary: l.Text(i18n.AssetDriverChartHelp), Symmetric: true, ActionableRows: true,
		Messages: charts.Messages{Empty: l.Text(i18n.AssetDriverNoChanges), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
	var currency domain.CurrencyCode
	mixedCurrencies := false
	for _, row := range data.Waterfall {
		if row.Amount == nil || row.Amount.IsZero() {
			continue
		}
		if currency == "" {
			currency = row.Amount.Currency()
		}
		mixedCurrencies = mixedCurrencies || currency != row.Amount.Currency()
		bar := charts.Bar{ID: charts.ID(row.Key), Label: assetBucketLabel(l, row.Bucket),
			Value: charts.Num(row.Amount.Amount().InexactFloat64()), Text: assetSignedMoney(view, row.Amount)}
		if row.Bucket == domain.BucketResidual {
			bar.Tone = charts.ToneWarning
		}
		spec.Bars = append(spec.Bars, bar)
	}
	if mixedCurrencies {
		// Keep the authoritative figures and row actions available when a
		// common scale would compare incompatible currencies.
		for i := range spec.Bars {
			spec.Bars[i].Value = charts.Value{}
		}
	}
	return spec
}

// This diagnostic only validates the shared report. Boundary values never
// feed the chart's geometry or replace any amount in the report.
func assetDriverNotice(data application.AssetChangeResult) i18n.MessageID {
	beginning, ending := data.Summary.BeginningValue, data.Summary.EndingValue
	if beginning == nil || ending == nil {
		return i18n.AssetDriverIncomplete
	}
	currency, running := beginning.Currency(), beginning.Amount()
	if ending.Currency() != currency {
		return i18n.AssetDriverMismatch
	}
	for _, row := range data.Waterfall {
		if row.Amount == nil || row.Amount.IsZero() {
			continue
		}
		if row.Amount.Currency() != currency {
			return i18n.AssetDriverMismatch
		}
		running = running.Add(row.Amount.Amount())
	}
	if !running.Sub(ending.Amount()).RoundBank(2).IsZero() {
		return i18n.AssetDriverMismatch
	}
	return i18n.NoMessage
}

func (p *assetChangesPage) driverContributions(view ui.View, data application.AssetChangeResult) layout.Widget {
	spec, notice := assetDriverBars(view, data), assetDriverNotice(data)
	return func(gtx layout.Context) layout.Dimensions {
		chart := func(gtx layout.Context) layout.Dimensions {
			p.restoreDriverFocus(gtx, view, &p.driverChart)
			dims, events := p.driverChart.Layout(gtx, view, spec)
			if selection := charts.Apply(charts.Selection{}, events); !selection.IsZero() {
				for _, row := range data.Waterfall {
					if charts.ID(row.Key) == selection.Item {
						p.openDriver(row, &p.driverChart)
						gtx.Execute(op.InvalidateCmd{})
						break
					}
				}
			}
			return dims
		}
		rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, spec.Label, view.Colors.Ink))}
		if notice != i18n.NoMessage {
			rows = append(rows, ui.Rigid(view.Message(view.Language.Text(notice), view.Colors.WarningInk)))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Card(gtx, chart) }))
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	}
}

func assetSignedMoney(view ui.View, value *domain.SignedMoney) string {
	text := returnMoney(view, value)
	if value != nil && value.Amount().RoundBank(int32(domain.CurrencyFractionDigits(value.Currency()))).IsPositive() {
		return "+" + text
	}
	return text
}

func (p *assetChangesPage) driverButton(view ui.View, row application.AssetChangeRow) layout.Widget {
	if p.driverButtons == nil {
		p.driverButtons = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(p.driverButtons, row.Key)
	name, money := assetBucketLabel(view.Language, row.Bucket), assetSignedMoney(view, row.Amount)
	label := name + " · " + money
	return func(gtx layout.Context) layout.Dimensions {
		p.restoreDriverFocus(gtx, view, button)
		if button.Clicked(gtx) {
			p.openDriver(row, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.ButtonOf(button, ui.ButtonSpec{Name: label, Variant: ui.ButtonGhost, Fill: true, Content: valueRow(view, name, money, "")})(gtx)
	}
}
