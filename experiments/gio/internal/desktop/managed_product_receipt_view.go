package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *productDetailPage) layoutReceipt(gtx layout.Context, view ui.View) layout.Dimensions {
	d := p.receipt
	if d.focusEditor {
		d.principal.Focus(gtx)
		d.focusEditor = false
	}
	if !p.receiptUnresolved() && d.cancel.Clicked(gtx) {
		p.receipt = nil
		gtx.Execute(key.FocusCmd{Tag: &p.startConfirmation})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	gtx = ui.LimitWidth(gtx, 720)
	if d.plan != nil {
		return p.layoutReceiptReview(gtx, view)
	}

	l := view.Language
	origin := view.Origin
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, tradeTitle(view, i18n.ProductReceiptAction))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(d.name+" · "+d.currency, view.Colors.Ink))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(l.Text(i18n.ProductReceiptHelp), view.Colors.SecondaryInk))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return d.principal.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductReturnedPrincipal, Disabled: d.reviewing != nil}, d.currency)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return d.interest.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductActualInterest, Disabled: d.reviewing != nil}, d.currency)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return d.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: d.reviewing != nil}, d.currency)
					}))
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.layoutReceiptTime(gtx, view, origin, p.clock()) }),
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptTimeHelp), d.timezone), view.Colors.SecondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.problem != i18n.NoMessage {
				return view.Message(l.Text(d.problem), view.Colors.ErrorInk)(gtx)
			}
			if d.reviewing != nil {
				return view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)(gtx)
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.preview.Clicked(gtx) {
				p.reviewReceipt()
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.reviewing != nil || strings.TrimSpace(d.principal.Text()) == "" {
						gtx = gtx.Disabled()
					}
					return view.PrimaryButton(&d.preview, l.Text(i18n.ProductReceiptReview))(gtx)
				}))
		}))
}

func (d *productReceiptDraft) layoutReceiptTime(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	zone, err := time.LoadLocation(d.timezone)
	if err != nil {
		zone = time.UTC
	}
	today := now.In(zone).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: d.maturity, Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat, Clearable: true}
	var timeOrigin image.Point
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			timeOrigin = origin.Add(image.Pt(gtx.Constraints.Max.X+gtx.Dp(8), 0))
			field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: origin, Disabled: d.reviewing != nil, Next: &d.time.Trigger}
			d.date.Layout(gtx, view, field, dates, d.day, func(day string) { d.day, d.problem = day, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: timeOrigin, Disabled: d.reviewing != nil, Previous: &d.date.Trigger, Next: &d.preview}
			d.time.Layout(gtx, view, field, d.minute, view.TimeFormat == settings.TimeFormat12, true, func(value string) {
				d.minute, d.problem = value, i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}))
}

func (p *productDetailPage) layoutReceiptReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.receipt, view.Language
	if !p.receiptUnresolved() && d.edit.Clicked(gtx) {
		d.plan, d.focusEditor = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.replay
	}
	if confirm.Clicked(gtx) {
		p.postReceipt()
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(tradeTitle(view, i18n.ProductReceiptReview)),
		layout.Rigid(view.Message(d.name+" · "+d.currency, view.Colors.Ink)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			rows := d.reviewRows(view)
			return material.List(view.Theme, &d.reviewList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 6}.Layout(gtx, rows[i])
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.uncertain {
				return view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)(gtx)
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutTargetBalanceConfirmation(gtx, view, d.posting != nil, d.uncertain, &d.cancel, &d.edit, confirm, i18n.ProductReceiptPost)
		}))
}

func (d *productReceiptDraft) reviewRows(view ui.View) []layout.Widget {
	review := d.plan.review
	l := view.Language
	zone, err := time.LoadLocation(review.Timezone)
	if err != nil {
		zone = time.UTC
	}
	rows := []layout.Widget{view.Message(contributionTimestamp(view, review.EffectiveAt, zone), view.Colors.SecondaryInk)}
	amount := func(label i18n.MessageID, values []domain.Money) layout.Widget {
		if len(values) == 1 {
			return fundsAmount(view, label, &values[0])
		}
		return fundsAmount(view, label, nil)
	}
	var interest, fee *domain.Money
	for _, preview := range review.Activities {
		if trade := preview.Activity.TradeDetail; trade != nil {
			rows = append(rows, fundsAmount(view, i18n.ProductPrincipalFact, &trade.Gross))
			fee = trade.Fee
		}
		if preview.Activity.Kind == domain.ActivityCashIn && preview.Activity.Reason == domain.ReasonInterest {
			for _, effect := range preview.Effects {
				if effect.Target == domain.EffectTargetAccountCash && effect.Role == domain.EffectRoleAmount && effect.Direction == domain.EffectAdded {
					interest = effect.Money
					break
				}
			}
		}
	}
	if interest == nil {
		rows = append(rows, view.Message(l.Text(i18n.ProductNoInterestPosted), view.Colors.Ink))
	} else {
		rows = append(rows, fundsAmount(view, i18n.ProductInterestFact, interest))
	}
	if fee == nil {
		rows = append(rows, view.Message(l.Text(i18n.ProductNoReceiptFee), view.Colors.Ink))
	} else {
		rows = append(rows, fundsAmount(view, i18n.ProductFeeFact, fee))
	}
	rows = append(rows, amount(i18n.ProductCashBefore, review.CashBefore), amount(i18n.ProductCashAfter, review.CashAfter),
		fundsAmount(view, i18n.ProductValueBefore, review.ProductBefore), fundsAmount(view, i18n.ProductValueAfter, review.ProductAfter))
	for _, released := range review.ReservationReleaseDetails {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptReleases), released.Label,
			portfolioDecimal(view, released.Amount.CanonicalAmount()), released.Amount.Currency()), view.Colors.Ink))
	}
	return rows
}
