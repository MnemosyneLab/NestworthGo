package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *accountsPage) layoutCashBalance(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.cashBalance, view.Language
	d := f.draft
	gtx, view = view.Measure(gtx, 620)
	if !p.cashBalanceUnresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &f.open})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutCashBalanceReview(gtx, view)
	}
	if d.readFailed && d.retry.Clicked(gtx) {
		p.loadCashBalanceSources()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.readFailed || d.reading != nil || d.sources == nil {
		status := view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		if d.readFailed {
			status = view.Message(l.Text(i18n.AccountLoadError), view.Colors.ErrorInk)
			buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(status), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
			}),
		)
	}
	if d.previewing == nil && d.prepare.Clicked(gtx) {
		p.previewCashBalance()
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	title, help := i18n.AccountCashSet, i18n.AccountCashOpeningHint
	if d.sources.history {
		title, help = i18n.HistoryCashReconciled, i18n.AccountCashReconcileHint
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(d.account.Name, view.Colors.Ink))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(l.Text(help), view.Colors.SecondaryInk))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					d.picker.Layout(gtx, view, ui.PickerField{Label: i18n.CashCurrency, Origin: origin, Disabled: d.previewing != nil},
						ui.SelectOptions{Items: supportedCurrencyChoices, Searchable: true}, d.currency.String(), func(id string) {
							d.currency, d.problem = domain.CurrencyCode(id), i18n.NoMessage
						})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if d.focus && gtx.Enabled() {
						d.amount.Focus(gtx)
						d.focus = false
					}
					return d.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.AccountCashTarget, Disabled: d.previewing != nil}, d.currency.String())
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := l.Text(i18n.AccountNoCash)
			if value, ok := d.sources.current[d.currency]; ok {
				label = fmt.Sprintf(l.Text(i18n.ContributionBefore), value.CanonicalAmount()+" "+d.currency.String())
			}
			return view.Message(label, view.Colors.SecondaryInk)(gtx)
		}),
	}
	if d.sources.history {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.reason.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCashReason, Disabled: d.previewing != nil})
		}))
	}
	if d.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		if d.previewing != nil {
			buttons = append(buttons, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
		} else {
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if strings.TrimSpace(d.amount.Text()) == "" {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.prepare, l.Text(i18n.AccountCashReview))(gtx)
			}))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (p *accountsPage) layoutCashBalanceReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.cashBalance.draft, view.Language
	if !p.cashBalanceUnresolved() && d.edit.Clicked(gtx) {
		d.review, d.focus = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.confirming == nil && confirm.Clicked(gtx) {
		p.confirmCashBalance()
		gtx.Execute(op.InvalidateCmd{})
	}
	plan := d.review.plan
	effective, err := targetBalanceTimestamp(view, plan.EffectiveAt, plan.Timezone)
	if err != nil {
		return view.Message(l.Text(i18n.AccountCashPreviewFailed), view.Colors.ErrorInk)(gtx)
	}
	help := i18n.AccountCashOpeningHint
	if d.review.history {
		help = i18n.AccountCashReconcileHint
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.AccountCashReview), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(d.account.Name, view.Colors.Ink)),
		layout.Rigid(view.Message(l.Text(help), view.Colors.SecondaryInk)),
	}
	children = append(children, targetBalanceFacts(view, i18n.AccountCashDelta, plan.Current, plan.Target, plan.Delta, plan.Currency, effective)...)
	if plan.Reason != nil && *plan.Reason != "" {
		children = append(children, layout.Rigid(view.Message(*plan.Reason, view.Colors.Ink)))
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.confirming != nil, d.uncertain,
			&d.cancel, &d.edit, confirm, i18n.AccountCashConfirm)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
