package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	assetBeginningStep charts.ID = "beginning"
	assetEndingStep    charts.ID = "ending"
)

func assetWaterfall(view ui.View, data application.AssetChangeResult) (charts.WaterfallSpec, bool) {
	l := view.Language
	spec := charts.WaterfallSpec{Label: l.Text(i18n.AssetDriverChart), Summary: l.Text(i18n.AssetDriverChartHelp),
		Messages: charts.Messages{Empty: l.Text(i18n.AssetNoData), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
	beginning := data.Summary.BeginningValue
	if beginning == nil {
		return spec, false
	}
	currency, running := beginning.Currency(), beginning.Amount()
	spec.FormatValue = func(value float64) string {
		return valuationMoney(view, domain.MoneyView{Amount: decimal.NewFromFloat(value).String(), Currency: currency}, false)
	}
	total := func(id charts.ID, label i18n.MessageID, amount *domain.SignedMoney) charts.WaterfallStep {
		step := charts.WaterfallStep{ID: id, Label: l.Text(label), Kind: charts.StepTotal,
			Text: returnMoney(view, amount), Tone: charts.ToneNeutral}
		if amount.Currency() == currency {
			step.To = charts.Num(amount.Amount().InexactFloat64())
		}
		return step
	}
	spec.Steps = append(spec.Steps, total(assetBeginningStep, i18n.AssetBeginning, beginning))
	comparable := true
	for _, row := range data.Waterfall {
		if row.Amount == nil || row.Amount.IsZero() {
			continue
		}
		step := charts.WaterfallStep{ID: charts.ID(row.Key), Label: assetBucketLabel(l, row.Bucket), Kind: charts.StepDelta,
			Text: returnMoney(view, row.Amount)}
		if row.Bucket == domain.BucketResidual {
			step.Tone = charts.ToneWarning
		}
		comparable = comparable && row.Amount.Currency() == currency
		if comparable {
			step.From = charts.Num(running.InexactFloat64())
			running = running.Add(row.Amount.Amount())
			step.To = charts.Num(running.InexactFloat64())
		}
		spec.Steps = append(spec.Steps, step)
	}
	if ending := data.Summary.EndingValue; ending != nil {
		spec.Steps = append(spec.Steps, total(assetEndingStep, i18n.AssetEnding, ending))
		return spec, !comparable || ending.Currency() != currency || !running.Sub(ending.Amount()).RoundBank(2).IsZero()
	}
	return spec, false
}

func (p *assetChangesPage) driverWaterfall(view ui.View, data application.AssetChangeResult) layout.Widget {
	spec, mismatch := assetWaterfall(view, data)
	spec.Selected = p.waterfallSelected
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			chart := func(gtx layout.Context) layout.Dimensions {
				p.restoreDriverFocus(gtx, view, &p.waterfall)
				dims, events := p.waterfall.Layout(gtx, view, spec)
				if len(events) > 0 {
					p.waterfallSelected = charts.Apply(p.waterfallSelected, events)
					for _, row := range data.Waterfall {
						if charts.ID(row.Key) == p.waterfallSelected.Item {
							p.waterfallSelected = charts.Selection{}
							p.openDriver(row, &p.waterfall)
							break
						}
					}
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			}
			rows := []ui.ColumnChild{ui.Rigid(chart)}
			if mismatch {
				rows = append(rows, ui.Rigid(view.Message(view.Language.Text(i18n.AssetDriverMismatch), view.Colors.ErrorInk)))
			}
			return (ui.Column{Gap: 6}).Layout(gtx, rows...)
		})
	}
}

func (p *assetChangesPage) driverButton(view ui.View, row application.AssetChangeRow) layout.Widget {
	if p.driverButtons == nil {
		p.driverButtons = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(p.driverButtons, row.Key)
	label := assetBucketLabel(view.Language, row.Bucket) + " · " + returnMoney(view, row.Amount)
	return func(gtx layout.Context) layout.Dimensions {
		p.restoreDriverFocus(gtx, view, button)
		if button.Clicked(gtx) {
			p.openDriver(row, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.ButtonOf(button, ui.ButtonSpec{Title: label, Variant: ui.ButtonGhost, Fill: true})(gtx)
	}
}
