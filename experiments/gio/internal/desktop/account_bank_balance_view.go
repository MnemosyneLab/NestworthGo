package desktop

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func (p *accountsPage) layoutBankBalance(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.bankBalance, view.Language
	d := f.draft
	gtx = ui.LimitWidth(gtx, 620)
	if !p.bankBalanceUnresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &f.open})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutBankBalanceReview(gtx, view)
	}
	if d.previewing == nil && d.prepare.Clicked(gtx) {
		p.previewBankBalance()
		gtx.Execute(op.InvalidateCmd{})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.AccountBankBalanceAction), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(d.account.Name, view.Colors.Ink)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.focus && gtx.Enabled() {
				d.amount.Focus(gtx)
				d.focus = false
			}
			return d.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.AccountCashTarget, Disabled: d.previewing != nil}, d.account.DefaultCurrency.String())
		}),
	}
	if d.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.previewing != nil {
					return view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)(gtx)
				}
				if strings.TrimSpace(d.amount.Text()) == "" {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.prepare, l.Text(i18n.AccountBankBalanceReview))(gtx)
			}),
		)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (p *accountsPage) layoutBankBalanceReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.bankBalance.draft, view.Language
	if !p.bankBalanceUnresolved() && d.edit.Clicked(gtx) {
		d.review, d.focus = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.confirming == nil && confirm.Clicked(gtx) {
		p.confirmBankBalance()
		gtx.Execute(op.InvalidateCmd{})
	}
	plan := d.review
	help := i18n.AccountBankBalanceOpening
	effective := l.Text(i18n.AccountBankBalanceOnConfirm)
	if plan.command != nil {
		help = i18n.AccountBankBalanceReconcile
		var err error
		effective, err = targetBalanceTimestamp(view, plan.at, plan.zone)
		if err != nil {
			return view.Message(l.Text(i18n.AccountBankBalancePreviewFailed), view.Colors.ErrorInk)(gtx)
		}
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.AccountBankBalanceReview), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(d.account.Name, view.Colors.Ink)),
		layout.Rigid(view.Message(l.Text(help), view.Colors.SecondaryInk)),
	}
	children = append(children, targetBalanceFacts(view, plan.before, plan.target, plan.delta, plan.currency.String(), effective)...)
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.confirming != nil, d.uncertain,
			&d.cancel, &d.edit, confirm, i18n.AccountBankBalanceConfirm)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func targetBalanceTimestamp(view ui.View, instant time.Time, zone string) (string, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), instant.In(location).Format("2006-01-02 15:04:05.000"), location), nil
}

func targetBalanceFacts(view ui.View, before, target, delta, currency, effective string) []layout.FlexChild {
	l := view.Language
	amount := func(value string) string { return value + " " + currency }
	rows := []layout.FlexChild{
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionBefore), amount(before)), view.Colors.Ink)),
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionAfter), amount(target)), view.Colors.Ink)),
	}
	if delta != "" {
		if !strings.HasPrefix(delta, "-") && !strings.HasPrefix(delta, "+") {
			delta = "+" + delta
		}
		rows = append(rows, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountCashDelta), amount(delta)), view.Colors.Ink)))
	}
	return append(rows, layout.Rigid(view.Message(effective, view.Colors.SecondaryInk)))
}

func layoutTargetBalanceConfirmation(gtx layout.Context, view ui.View, pending, uncertain bool,
	cancel, edit, confirm *widget.Clickable, label i18n.MessageID) layout.Dimensions {
	l := view.Language
	if pending {
		return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
	}
	if uncertain {
		return view.PrimaryButton(confirm, l.Text(i18n.ContributionRetryPost))(gtx)
	}
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(view.Button(cancel, l.Text(i18n.InstitutionsCancel))),
		layout.Rigid(view.Button(edit, l.Text(i18n.AccountCreateBackToDetails))),
		layout.Rigid(view.PrimaryButton(confirm, l.Text(label))),
	)
}
