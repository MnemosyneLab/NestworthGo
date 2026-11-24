package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *accountsPage) layoutFirstBuy(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.firstBuy, view.Language
	d := f.draft
	if !p.firstBuyUnresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &f.open})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	gtx = ui.LimitWidth(gtx, 690)
	if d.review != nil {
		return p.layoutFirstBuyReview(gtx, view)
	}
	if d.sourceProblem != i18n.NoMessage && d.retry.Clicked(gtx) {
		p.loadFirstBuySources()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.previewing == nil && d.preview.Clicked(gtx) {
		p.previewFirstBuy()
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, buyTitle(view, i18n.BuyOpen))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(d.account.Name+" · "+d.account.DefaultCurrency.String(), view.Colors.Ink))
		}),
	}
	if d.sources != nil && len(d.sources.Instruments) != 0 {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, d.layoutBuyInstrument(view, origin))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, d.layoutBuyAmounts(view))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.layoutBuyTime(gtx, view, origin, p.clock())
			}),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), d.sources.Location), view.Colors.SecondaryInk)),
		)
	}
	switch {
	case d.loading != nil || d.previewing != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case d.sourceProblem != i18n.NoMessage:
		children = append(children, layout.Rigid(view.Message(l.Text(d.sourceProblem), view.Colors.ErrorInk)))
	case d.sources != nil && len(d.sources.Instruments) == 0:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.BuyNoInstruments), view.Colors.SecondaryInk)))
	}
	if d.problem != i18n.NoMessage && d.problem != d.sourceProblem {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		if d.sourceProblem != i18n.NoMessage {
			buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
		} else if d.sources != nil && len(d.sources.Instruments) != 0 {
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.previewing != nil || d.instrument == "" || strings.TrimSpace(d.quantity.Text()) == "" || strings.TrimSpace(d.gross.Text()) == "" {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.preview, l.Text(i18n.BuyReview))(gtx)
			}))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *firstBuyDraft) layoutBuyInstrument(view ui.View, origin image.Point) layout.Widget {
	items := make([]ui.Choice, 0, len(d.sources.Instruments))
	for _, item := range d.sources.Instruments {
		label := item.SelectionLabel()
		items = append(items, ui.Choice{ID: item.ID.String(), Label: label, SearchText: item.Name + " " + label})
	}
	return func(gtx layout.Context) layout.Dimensions {
		field := ui.PickerField{Label: i18n.BuyInstrument, Origin: origin, Disabled: d.previewing != nil, Next: &d.quantity.Editor}
		d.selectInstrument.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.instrument.String(), func(id string) {
			d.chooseInstrument(domain.InstrumentID(id))
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
}

func (d *firstBuyDraft) layoutBuyAmounts(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		currency := d.account.DefaultCurrency.String()
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.BuyQuantity, Disabled: d.previewing != nil})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return d.gross.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyGross, Disabled: d.previewing != nil}, currency)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return d.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: d.previewing != nil}, currency)
			}),
		)
	}
}

func (d *firstBuyDraft) layoutBuyTime(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	zone := d.sources.Location
	today := now.In(zone).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}
	var timeOrigin image.Point
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			timeOrigin = origin.Add(image.Pt(gtx.Constraints.Max.X+gtx.Dp(8), 0))
			field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: origin, Disabled: d.previewing != nil, Next: &d.timePicker.Trigger}
			d.datePicker.Layout(gtx, view, field, dates, d.day, func(day string) { d.day, d.problem = day, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: timeOrigin, Disabled: d.previewing != nil, Previous: &d.datePicker.Trigger, Next: &d.preview}
			d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, func(value string) {
				d.time, d.problem = value, i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
	)
}

func (p *accountsPage) layoutFirstBuyReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.firstBuy.draft, view.Language
	if !p.firstBuyUnresolved() && d.edit.Clicked(gtx) {
		d.review = nil
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.posting == nil && confirm.Clicked(gtx) {
		p.postFirstBuy()
		gtx.Execute(op.InvalidateCmd{})
	}
	review := d.review
	preview, trade := review.Preview, review.Preview.Activity.TradeDetail
	currency := trade.Gross.Currency()
	money := func(amount string) string { return portfolioDecimal(view, amount) + " " + currency.String() }
	fee := "0"
	if trade.Fee != nil {
		fee = trade.Fee.CanonicalAmount()
	}
	instrument := ""
	if at := slices.IndexFunc(d.sources.Instruments, func(i domain.Instrument) bool { return i.ID == trade.InstrumentID }); at >= 0 {
		instrument = d.sources.Instruments[at].SelectionLabel()
	}
	children := []layout.FlexChild{
		layout.Rigid(buyTitle(view, i18n.BuyReview)),
		layout.Rigid(view.Message(d.account.Name+" · "+instrument, view.Colors.Ink)),
		layout.Rigid(view.Message(contributionTimestamp(view, preview.Activity.EffectiveAt, review.Timezone), view.Colors.SecondaryInk)),
	}
	add := func(id i18n.MessageID, value string) {
		children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(id), value), view.Colors.Ink)))
	}
	add(i18n.AccountQuantity, portfolioDecimal(view, trade.Quantity.Canonical()))
	add(i18n.BuyExecutionPrice, money(trade.UnitPrice.Canonical()))
	add(i18n.BuyGrossFact, money(trade.Gross.CanonicalAmount()))
	add(i18n.BuyFeeFact, money(fee))
	add(i18n.BuyCostBasis, money(review.UnitCost.Canonical()))
	for _, endpoint := range preview.Review {
		if endpoint.Target == domain.EffectTargetAccountCash {
			add(i18n.ContributionBefore, money(endpoint.Before))
			add(i18n.BuyCashDelta, money(endpoint.Delta))
			add(i18n.ContributionAfter, money(endpoint.After))
		}
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.posting != nil, d.uncertain, &d.cancel, &d.edit, confirm, i18n.BuyConfirm)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx, children...)
}

func buyTitle(view ui.View, id i18n.MessageID) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Label(gtx, view.Language.Text(id), 19, font.Medium, view.Colors.Ink)
	}
}
