package desktop

import (
	"image"
	"slices"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var quantityAdjustmentLabels = tradeLabels{
	action: i18n.QuantityAdjustOpen, review: i18n.QuantityAdjustReview, confirm: i18n.QuantityAdjustConfirm,
	posted: i18n.QuantityAdjustPosted, scope: i18n.CustodyUnavailable, history: i18n.QuantityAdjustStart,
	empty: i18n.QuantityAdjustEmpty, previewFailed: i18n.QuantityAdjustFailed, stale: i18n.QuantityAdjustStale,
}

type quantityAdjustmentDraft struct {
	holding     domain.HoldingID
	adding      bool
	holdingPick ui.Select
	add, remove widget.Clickable
	sources     *history.PositionSources
	loading     <-chan historyResult[history.PositionSources]
}

func (f *accountInvestment) startAdjustment(run investmentRuntime) {
	f.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, domain.Account{})
	f.draft.adjustment = &quantityAdjustmentDraft{adding: true}
	f.loadSources(run)
}

func (f *accountInvestment) loadAdjustmentSources(run investmentRuntime) {
	a := f.draft.adjustment
	a.sources = nil
	a.loading = runHistoryRead(run.workers, run.invalidate, func() (history.PositionSources, error) {
		return history.LoadPositionSources(run.ctx, f.service)
	})
}

func (f *accountInvestment) pollAdjustmentSources(run investmentRuntime) {
	d := f.draft
	a := d.adjustment
	if a == nil {
		return
	}
	select {
	case result := <-a.loading:
		a.loading = nil
		if result.err != nil {
			d.sourceProblem = tradeProblem(result.err, quantityAdjustmentLabels, i18n.QuantityAdjustFailed)
			return
		}
		a.sources = &result.value
		d.sources = &history.InvestmentSources{Origin: result.value.Origin, Location: result.value.Location}
		if d.day == "" {
			d.captureTime(run.clock())
		}
		if _, ok := a.selected(); a.holding != "" && !ok {
			a.holding = ""
			d.problem = i18n.CustodyUnavailable
		}
	default:
	}
}

func (a *quantityAdjustmentDraft) selected() (history.PositionHolding, bool) {
	if a.sources != nil {
		if at := slices.IndexFunc(a.sources.Holdings, func(item history.PositionHolding) bool { return item.Holding.ID == a.holding }); at >= 0 {
			return a.sources.Holdings[at], true
		}
	}
	return history.PositionHolding{}, false
}

func (f *accountInvestment) previewAdjustment(run investmentRuntime) {
	d := f.draft
	if d.previewing != nil || d.sources == nil || !d.investmentInputComplete() {
		return
	}
	input := history.QuantityAdjustmentInput{Holding: d.adjustment.holding, Added: d.adjustment.adding,
		Quantity: d.quantity.Text(), UnitCost: d.unitCost.Text(), Note: d.note.Text(),
		Date: d.day, Time: d.time, EffectiveAt: d.effectiveAt()}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(run.workers, run.invalidate, func() (history.ReviewedInvestment, error) {
		return history.ReviewQuantityAdjustment(run.ctx, f.service, input)
	})
}

func (f *accountInvestment) adjustmentFields(view ui.View, origin *image.Point) []layout.FlexChild {
	d, a := f.draft, f.draft.adjustment
	if !d.investmentFieldsVisible() {
		return nil
	}
	l := view.Language
	item, selected := a.selected()
	choices := make([]ui.Choice, 0, len(a.sources.Holdings))
	for _, item := range a.sources.Holdings {
		choices = append(choices, ui.Choice{ID: item.Holding.ID.String(), Label: custodyHoldingLabel(view, item)})
	}
	field := func(w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions { return fieldPosition(gtx, origin, w) })
	}
	fields := []layout.FlexChild{
		field(func(gtx layout.Context) layout.Dimensions {
			if d.focusInput && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &a.holdingPick.Trigger})
				d.focusInput = false
			}
			options := ui.PickerField{Label: i18n.QuantityAdjustHolding, Origin: *origin, Disabled: d.previewing != nil, Next: &a.add}
			a.holdingPick.Layout(gtx, view, options, ui.SelectOptions{Items: choices, Searchable: true}, a.holding.String(), func(id string) {
				if a.holding.String() != id {
					a.holding = domain.HoldingID(id)
					d.quantity.Editor.SetText("")
					d.unitCost.Editor.SetText("")
					d.problem = i18n.NoMessage
				}
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		field(func(gtx layout.Context) layout.Dimensions {
			if d.previewing != nil {
				gtx = gtx.Disabled()
			}
			if a.add.Clicked(gtx) && !a.adding {
				a.adding, d.problem = true, i18n.NoMessage
			}
			if a.remove.Clicked(gtx) && a.adding {
				a.adding, d.problem = false, i18n.NoMessage
				d.unitCost.Editor.SetText("")
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.TabButton(&a.add, l.Text(i18n.QuantityAdjustAdd), a.adding)),
				layout.Rigid(view.TabButton(&a.remove, l.Text(i18n.QuantityAdjustRemove), !a.adding)))
		}),
		field(func(gtx layout.Context) layout.Dimensions {
			return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.QuantityAdjustQuantity, Disabled: !selected || d.previewing != nil})
		}),
	}
	if a.adding && selected {
		fields = append(fields, field(func(gtx layout.Context) layout.Dimensions {
			options := ui.FieldOptions{Label: i18n.QuantityAdjustCost, Disabled: d.previewing != nil}
			return d.unitCost.LayoutCurrency(gtx, view, options, item.Instrument.QuoteCurrency.String())
		}))
	}
	return fields
}

func (a *quantityAdjustmentDraft) reviewFacts(view ui.View, preview domain.ChangePreview, fact func(i18n.MessageID, string) layout.FlexChild) []layout.FlexChild {
	var facts []layout.FlexChild
	for _, effect := range preview.Effects {
		if effect.Quantity != nil {
			label := i18n.QuantityAdjustAdded
			if effect.Direction == domain.EffectRemoved {
				label = i18n.QuantityAdjustRemoved
			}
			facts = append(facts, fact(label, portfolioDecimal(view, effect.Quantity.Canonical())))
		}
	}
	for _, endpoint := range preview.Resulting {
		if endpoint.Target == domain.EffectTargetHoldingQuantity {
			facts = append(facts, fact(i18n.QuantityAdjustAfter, portfolioDecimal(view, endpoint.Quantity)))
		}
	}
	if cost := preview.DerivedUnitPrice; cost != nil {
		item, _ := a.selected()
		facts = append(facts, fact(i18n.PositionImportCostFact, portfolioDecimal(view, cost.Canonical())+" "+item.Instrument.QuoteCurrency.String()))
	}
	return facts
}
