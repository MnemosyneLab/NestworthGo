package desktop

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

func (p *accountsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	if d := p.productSetup; d != nil {
		if !d.poll() {
			return d.layout(gtx, view, p.productSetupCalendar(), p.cancelProductSetup)
		}
		p.cancelProductSetup()
		p.productSaved = true
		p.refresh()
		view.RestorePageScroll(0)
	}
	if f := p.statement; f != nil {
		row := p.freshDetail()
		if f.screen == nil && row != nil && (p.target.statement != "" || f.open.Clicked(gtx)) {
			if statementCard(row.record.Account) {
				p.startStatements(row.record.Account, p.target.statement)
			} else if p.target.statement != "" {
				return view.Message(l.Text(i18n.StatementLoadFailed), view.Colors.ErrorInk)(gtx)
			}
		}
		if f.screen != nil {
			return p.layoutStatements(gtx, view)
		}
	}
	if p.canCreateBankAccount() && p.createBank.Clicked(gtx) {
		p.startBankAccount()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.canCreateUnscoped() {
		if p.createAsset.Clicked(gtx) {
			p.startBalanceAccount(domain.TypeCashOnHand, domain.RoleAsset, domain.Institution{})
			gtx.Execute(op.InvalidateCmd{})
		}
		if p.createLiability.Clicked(gtx) {
			p.startBalanceAccount(domain.TypeCreditCard, domain.RoleLiability, domain.Institution{})
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if (p.canCreateBankAccount() || p.canCreateUnscoped()) && p.createShell.Clicked(gtx) {
		p.startAccount(domain.TypeProperty, domain.RoleAsset, domain.TrackingManualValue, p.current.institutions[p.target.institution])
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.creation != nil && p.creation.draft != nil {
		return p.layoutAccountCreation(gtx, view)
	}
	if p.maintenance != nil && p.maintenance.draft != nil {
		return p.layoutAccountSettings(gtx, view)
	}
	if f := p.liabilityPosting; f != nil {
		if f.draft == nil && p.canPostLiability() && f.open.Clicked(gtx) {
			p.startLiabilityPosting()
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.draft != nil {
			return p.layoutLiabilityPosting(gtx, view)
		}
	}
	if f := p.cashBalance; f != nil {
		if f.draft == nil && p.canSetCashBalance() && f.open.Clicked(gtx) {
			p.startCashBalance()
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.draft != nil {
			return p.layoutCashBalance(gtx, view)
		}
		if f.saved.AccountID == p.target.account && f.saved.ActivityID != nil && f.openActivity != nil && f.view.Clicked(gtx) {
			f.openActivity(*f.saved.ActivityID, &f.view)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if f := p.balanceTarget; f != nil {
		if f.draft == nil && p.canUpdateBalanceTarget() && f.open.Clicked(gtx) {
			p.startBalanceTarget()
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.draft != nil {
			return p.layoutBalanceTarget(gtx, view)
		}
		if f.saved.account == p.target.account && f.saved.activity != "" && f.openActivity != nil && f.view.Clicked(gtx) {
			f.openActivity(f.saved.activity, &f.view)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if f := p.opening; f != nil {
		if f.draft == nil && p.canOpenPosition() && f.open.Clicked(gtx) {
			f.saved = ""
			f.draft = &openingPositionDraft{account: p.freshDetail().record.Account, quantity: ui.NewTextField(true)}
			p.loadOpeningPosition(f.draft)
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.draft != nil {
			return p.layoutOpeningPosition(gtx, view)
		}
	}
	if f := p.investment; f != nil {
		if f.draft == nil {
			switch {
			case p.canTradeAccount() && f.buy.Clicked(gtx):
				p.startInvestment(domain.ActivityBuy)
				gtx.Execute(op.InvalidateCmd{})
			case p.canSell() && f.sell.Clicked(gtx):
				p.startInvestment(domain.ActivitySell)
				gtx.Execute(op.InvalidateCmd{})
			case p.canRecordDividend() && f.dividend.Clicked(gtx):
				p.startInvestment(domain.ActivityCashDividend)
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.importOpen.Clicked(gtx):
				p.startInvestment(domain.ActivityPositionTransfer)
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.custodyOpen.Clicked(gtx):
				f.startCustody(p.investmentRuntime(), positionScope{account: p.freshDetail().record.Account})
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.adjustmentOpen.Clicked(gtx):
				f.startAdjustment(p.investmentRuntime(), positionScope{account: p.freshDetail().record.Account})
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
		if f.draft != nil {
			return f.layoutForm(gtx, view, p.investmentRuntime())
		}
	}
	detail := p.target.account != ""
	if p.back.Clicked(gtx) && detail {
		p.backToList()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.clear.Clicked(gtx) && (p.target.institution != "" || p.target.accountList.active()) {
		p.open(navigationTarget{route: accountsRoute}, "", &p.clear)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.reload.Clicked(gtx) {
		p.read()
	}
	if p.archiveFilter.Clicked(gtx) && !detail && p.request == nil && !p.invalid {
		p.enter(p.target, !p.archived, layout.Position{})
	}
	return ui.Column{Gap: 12}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			reload := func(gtx layout.Context) layout.Dimensions {
				label := i18n.HistoryRefresh
				if p.err != nil {
					label = i18n.HistoryRetry
				}
				if p.request != nil {
					gtx = gtx.Disabled()
				}
				return view.Button(&p.reload, l.Text(label))(gtx)
			}
			var left, right []layout.Widget
			if detail {
				controls := []layout.FlexChild{layout.Rigid(view.Button(&p.back, l.Text(i18n.AccountBack)))}
				controls = append(controls, p.accountMaintenanceActions(gtx, view)...)
				if p.canSetCashBalance() {
					controls = append(controls, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						view.Focus.Watch(gtx, &p.cashBalance.open)
						return view.Button(&p.cashBalance.open, l.Text(i18n.AccountCashSet))(gtx)
					}))
				}
				if p.canUpdateBalanceTarget() {
					action := balanceTargetCopyFor(p.freshDetail().record.Account).action
					controls = append(controls, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						view.Focus.Watch(gtx, &p.balanceTarget.open)
						return view.Button(&p.balanceTarget.open, l.Text(action))(gtx)
					}))
				}
				if p.target.institution != "" {
					controls = append(controls, layout.Rigid(view.Button(&p.clear, l.Text(i18n.AccountClearInstitution))))
				} else if p.target.accountList.active() {
					controls = append(controls, layout.Rigid(returnMonthButton(view, &p.clear, i18n.AccountClearOverviewFilter, nil, false)))
				}
				left = append(left, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, controls...)
				})
			} else {
				primary := true
				add := func(button *widget.Clickable, id i18n.MessageID) {
					if primary {
						left = append(left, view.PrimaryButton(button, l.Text(id)))
						primary = false
						return
					}
					left = append(left, view.Button(button, l.Text(id)))
				}
				if p.canCreateBankAccount() {
					add(&p.createBank, i18n.AccountCreateAdd)
				}
				if p.canCreateUnscoped() {
					add(&p.createAsset, i18n.AccountCreateAssetAdd)
					add(&p.createLiability, i18n.AccountCreateLiabilityAdd)
				}
				if p.canCreateBankAccount() || p.canCreateUnscoped() {
					add(&p.createShell, i18n.AccountCreateShellAdd)
				}
				if p.target.institution != "" {
					right = append(right, view.Button(&p.clear, l.Text(i18n.AccountClearInstitution)))
				} else if p.target.accountList.active() {
					right = append(right, returnMonthButton(view, &p.clear, i18n.AccountClearOverviewFilter, nil, false))
				}
				if !p.invalid && (p.current == nil || !p.current.unavailable) {
					archive := i18n.InstitutionsShowArchived
					if p.archived {
						archive = i18n.InstitutionsHideArchived
					}
					right = append(right, func(gtx layout.Context) layout.Dimensions {
						if p.request != nil {
							gtx = gtx.Disabled()
						}
						variant := ui.ButtonOutline
						if p.archived {
							variant = ui.ButtonSoft
						}
						return view.ButtonOf(&p.archiveFilter, ui.ButtonSpec{Variant: variant, Title: l.Text(archive), Name: l.Text(archive), Toggle: true, Selected: p.archived})(gtx)
					})
				}
			}
			if !p.invalid {
				right = append(right, reload)
			}
			toolbar := func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(16), Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return ui.Wrap{Gap: ui.Space8}.Layout(gtx, len(left), func(gtx layout.Context, i int) layout.Dimensions { return left[i](gtx) })
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.Wrap{Gap: ui.Space8}.Layout(gtx, len(right), func(gtx layout.Context, i int) layout.Dimensions { return right[i](gtx) })
					}),
				)
			}
			if !p.canPostLiability() {
				return toolbar(gtx)
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Rigid(toolbar),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					f := p.liabilityPosting
					view.Focus.Watch(gtx, &f.open)
					if f.restore && gtx.Enabled() {
						gtx.Execute(key.FocusCmd{Tag: &f.open})
						f.restore = false
					}
					action := liabilityEntryLabel(p.freshDetail().record.Account)
					return layout.W.Layout(gtx, view.PrimaryButton(&f.open, l.Text(action)))
				}),
			)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil || p.current.unavailable || len(p.current.rows) == 0 {
				return layout.Dimensions{}
			}
			title := l.Text(i18n.AccountReadList)
			if detail {
				rows := p.detailRows(view)
				return pageListGap(gtx, view, &p.list, title, len(rows), 0, func(gtx layout.Context, i int) layout.Dimensions { return rows[i](gtx) })
			}
			return pageListGap(gtx, view, &p.list, title, len(p.rows), 0, func(gtx layout.Context, i int) layout.Dimensions {
				row := p.rows[i]
				if row.account == nil {
					return p.groupHeading(gtx, view, row, i == 0)
				}
				first := i == 0 || p.rows[i-1].account == nil
				last := i == len(p.rows)-1 || p.rows[i+1].account == nil
				return p.accountTile(gtx, view, *row.account, first, last)
			})
		}),
	)
}

func (p *accountsPage) status(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	id, color := i18n.NoMessage, view.Colors.SecondaryInk
	switch {
	case p.request != nil && p.current == nil:
		id = i18n.AccountLoading
	case p.request != nil:
		id = i18n.OverviewRefreshing
	case p.err != nil && p.current == nil:
		id, color = i18n.AccountLoadError, view.Colors.ErrorInk
	case p.err != nil:
		id, color = i18n.OverviewStale, view.Colors.ErrorInk
	case p.invalid || (p.current != nil && p.current.unavailable):
		id = i18n.AccountUnavailable
	case p.current != nil && len(p.current.rows) == 0:
		id = i18n.OverviewEmpty
		if p.target.accountList.active() {
			id = i18n.AccountOverviewFilterEmpty
		}
	}
	var lines []layout.FlexChild
	if p.target.accountList.active() {
		label := p.target.accountList.name(l, p.filterLabel)
		lines = append(lines, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountOverviewFilter), label), view.Colors.Ink)))
	}
	if p.createdID != "" && p.target.account == p.createdID {
		lines = append(lines, layout.Rigid(view.Message(l.Text(p.createdMessage), view.Colors.Ink)))
	}
	if f := p.cashBalance; f != nil && f.saved.ID != "" && p.target.account == f.saved.AccountID {
		lines = append(lines, layout.Rigid(balanceSavedLine(view, i18n.AccountCashSaved, &f.view, f.saved.ActivityID != nil && f.openActivity != nil)))
	}
	if f := p.balanceTarget; f != nil && p.target.account != "" {
		if f.saved.account == p.target.account {
			lines = append(lines, layout.Rigid(balanceSavedLine(view, f.savedMessage, &f.view, f.saved.activity != "" && f.openActivity != nil)))
		} else if f.unverified == p.target.account {
			lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountBankBalanceUnverified), view.Colors.ErrorInk)))
		}
	}
	if m := p.maintenance; m != nil && p.target.account != "" {
		if m.success != i18n.NoMessage {
			lines = append(lines, layout.Rigid(view.Message(l.Text(m.success), view.Colors.Ink)))
		}
		if m.issue != i18n.NoMessage {
			lines = append(lines, layout.Rigid(view.Message(l.Text(m.issue), view.Colors.ErrorInk)))
		}
		if row := p.freshDetail(); row != nil {
			switch {
			case !accountMaintenanceSupported(row.record.Account):
				lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountMaintenanceUnsupported), view.Colors.SecondaryInk)))
			case row.record.Account.ArchivedAt != nil:
				lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountMaintenanceReadOnly), view.Colors.SecondaryInk)))
			case !canEditAccount(row.record):
				lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountMaintenanceNoBalance), view.Colors.SecondaryInk)))
			}
		}
	}
	if p.current != nil && !p.current.unavailable {
		if p.target.institution != "" && len(p.current.rows) == 0 {
			institution := p.current.institutions[p.target.institution]
			lines = append(lines, layout.Rigid(view.Message(accountReference(l, institution.Name, institution.ArchivedAt != nil), view.Colors.Ink)))
		}
		lines = append(lines, layout.Rigid(view.Message(valuationReadTime(view, p.current.readAt), view.Colors.SecondaryInk)))
	}
	if id != i18n.NoMessage {
		lines = append(lines, layout.Rigid(view.Message(l.Text(id), color)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, lines...)
}

func balanceSavedLine(view ui.View, label i18n.MessageID, link *widget.Clickable, showLink bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := view.Language
		children := []layout.FlexChild{layout.Rigid(view.Message(l.Text(label), view.Colors.Ink))}
		if showLink {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, link)
				return view.Button(link, l.Text(i18n.AccountCashView))(gtx)
			}))
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx, children...)
	}
}

func (p *accountsPage) groupHeading(gtx layout.Context, view ui.View, row accountListRow, top bool) layout.Dimensions {
	inset := layout.Inset{Top: ui.Space8, Bottom: ui.Space8, Left: ui.Space4}
	if top {
		inset.Top = 0
	}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if row.icon == nil {
					return layout.Dimensions{}
				}
				return ui.LayoutGlyph(gtx, row.icon, 20, view.Colors.Accent)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextHeading, row.heading, view.Colors.Ink)
			}),
		)
	})
}

func (p *accountsPage) accountTile(gtx layout.Context, view ui.View, row accountRow, first, last bool) layout.Dimensions {
	account := row.record.Account
	button := p.buttons[account.ID]
	if button == nil {
		button = new(widget.Clickable)
		p.buttons[account.ID] = button
	}
	if button.Clicked(gtx) {
		p.open(navigationTarget{route: accountsRoute, account: account.ID}, account.Name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	content := material.ButtonLayout(view.Theme, button)
	content.Background, content.CornerRadius = color.NRGBA{}, 0
	inset := layout.Inset{}
	if last {
		inset.Bottom = ui.Space16
	}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, button)
		return view.RowCard(gtx, first, last, func(gtx layout.Context) layout.Dimensions {
			return accessibility.Button(gtx, button, account.Name, "", false, func(gtx layout.Context) layout.Dimensions {
				return content.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp(account.Name).Add(gtx.Ops)
					return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return accountTitle(gtx, view, account) }),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return accountAmountLines(gtx, view, row, true, layout.End)
								})
							}),
						)
					})
				})
			})
		})
	})
}

func accountTitle(gtx layout.Context, view ui.View, account domain.Account) layout.Dimensions {
	l := view.Language
	description := l.Text(accountTypes[account.AccountType])
	if account.IsLiability() {
		description += " · " + l.Text(i18n.AccountRoleLiability)
	} else if account.AccountType == domain.TypeOther {
		description += " · " + l.Text(i18n.AccountRoleAsset)
	}
	if account.TrackingMode == domain.TrackingManualValue || account.TrackingMode == domain.TrackingHoldings {
		description += " · " + l.Text(accountTrackingLabel(account.TrackingMode, account.AccountType))
	}
	if account.ArchivedAt != nil {
		description += " · " + l.Text(i18n.DirectoryArchived)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextLabel, account.Name, view.Colors.Ink)
		}),
		layout.Rigid(view.Body(ui.TextCaption, description, view.Colors.SecondaryInk)),
	)
}

func accountAmounts(gtx layout.Context, view ui.View, row accountRow, brief bool) layout.Dimensions {
	return accountAmountLines(gtx, view, row, brief, layout.Start)
}

func accountAmountLines(gtx layout.Context, view ui.View, row accountRow, brief bool, alignment layout.Alignment) layout.Dimensions {
	l := view.Language
	a, v := row.record.Account, row.value
	var native, base *domain.MoneyView
	var baseCurrency domain.CurrencyCode
	var nativePartial bool
	if v != nil {
		base = v.BaseValue
		if v.Composition != nil {
			baseCurrency = v.Composition.Currency
		}
		if base != nil {
			baseCurrency = base.Currency
		}
		if v.DisplayValue != nil {
			native, nativePartial = v.DisplayValue.Value, !v.DisplayValue.Complete
		}
	}
	if v == nil || v.DisplayValue == nil {
		if recorded := row.record.LatestValue; recorded != nil && a.TrackingMode != domain.TrackingHoldings && recorded.Amount.Currency() == a.DefaultCurrency {
			amount := recorded.Amount
			native = &domain.MoneyView{Amount: amount.CanonicalAmount(), Currency: amount.Currency()}
		} else if base != nil && base.Currency == a.DefaultCurrency {
			native, nativePartial = base, !v.Complete
		}
	}
	amount := func(m *domain.MoneyView, currency domain.CurrencyCode, partial bool) string {
		if m == nil {
			return fmt.Sprintf(l.Text(i18n.AccountValueUnavailable), currency)
		}
		label := m.Currency.String() + " · " + valuationMoney(view, *m, a.IsLiability())
		if partial {
			label += " · " + l.Text(i18n.OverviewKnownSubtotal)
		}
		return label
	}
	line := func(style ui.TextStyle, content string, ink color.NRGBA) layout.FlexChild {
		if alignment == layout.End {
			return layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.LineEnd(gtx, style, content, ink) })
		}
		return layout.Rigid(view.Body(style, content, ink))
	}
	lines := []layout.FlexChild{line(ui.TextLabel, amount(native, a.DefaultCurrency, nativePartial), view.Colors.Ink)}
	if baseCurrency != "" && baseCurrency != a.DefaultCurrency {
		lines = append(lines, line(ui.TextCaption, amount(base, baseCurrency, !v.Complete), view.Colors.SecondaryInk))
	}
	if v != nil {
		if !v.Complete || nativePartial {
			status := l.Text(i18n.OverviewIncomplete)
			if brief {
				status = accountIssueStatus(l, *v, status)
			}
			lines = append(lines, line(ui.TextCaption, status, view.Colors.SecondaryInk))
		}
		if accountStale(*v) {
			lines = append(lines, line(ui.TextCaption, l.Text(i18n.AccountStaleQuotes), view.Colors.SecondaryInk))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: alignment, Gap: gtx.Dp(2)}.Layout(gtx, lines...)
}

func accountIssueStatus(lang i18n.Language, value domain.AccountValuation, status string) string {
	labels := []string{status}
	seen := make(map[i18n.MessageID]bool)
	add := func(inputs []domain.MissingInputView) {
		for _, input := range inputs {
			if id := overviewMissing[input.Kind]; id != i18n.NoMessage && !seen[id] {
				seen[id] = true
				labels = append(labels, lang.Text(id))
			}
		}
	}
	add(value.MissingInputs)
	if value.DisplayValue != nil {
		add(value.DisplayValue.MissingInputs)
	}
	return strings.Join(labels, " · ")
}

func accountStale(value domain.AccountValuation) bool {
	stale := func(e *domain.QuoteEvidenceView) bool {
		return e != nil && (e.Freshness == domain.FreshnessStale || e.Carried) && !e.ClosedCarry
	}
	for _, c := range value.Components {
		if stale(c.PriceEvidence) || stale(c.FXEvidence) {
			return true
		}
	}
	if value.DisplayValue != nil {
		for _, fx := range value.DisplayValue.FXEvidence {
			if stale(&fx) {
				return true
			}
		}
	}
	return false
}

func accountReference(lang i18n.Language, name string, archived bool) string {
	if name == "" {
		return lang.Text(i18n.AccountReferenceUnavailable)
	}
	if archived {
		return name + " · " + lang.Text(i18n.DirectoryArchived)
	}
	return name
}

func (p *accountsPage) detailRows(view ui.View) []layout.Widget {
	l := view.Language
	row := p.current.rows[0]
	a := row.record.Account
	institution, group := l.Text(i18n.AccountNoInstitution), l.Text(i18n.AccountNoGroup)
	if a.InstitutionID != nil {
		institution = accountReference(l, row.record.InstitutionName, p.current.institutions[*a.InstitutionID].ArchivedAt != nil)
	}
	if a.GroupID != nil {
		group = accountReference(l, row.record.GroupName, p.current.groups[*a.GroupID].ArchivedAt != nil)
	}
	var owners []string
	for _, share := range row.record.Ownership.Shares() {
		member := p.current.members[share.MemberID]
		owners = append(owners, accountReference(l, member.Name, member.ArchivedAt != nil)+" · "+format.ShareBPS(share.ShareBPS))
	}
	owner := strings.Join(owners, "  /  ")
	if owner == "" {
		owner = l.Text(i18n.AccountReferenceUnavailable)
	}
	var rows []layout.Widget
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: ui.Space12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								side := gtx.Dp(32)
								gtx.Constraints = layout.Exact(image.Pt(side, side))
								return accountGlyph(a, p.current.institutions).Layout(gtx, view.Colors.Accent)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return accountTitle(gtx, view, a) }),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountAmounts(gtx, view, row, false) }),
					layout.Rigid(view.Hairline),
					layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.AccountInstitution), institution), view.Colors.SecondaryInk)),
					layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.AccountGroup), group), view.Colors.SecondaryInk)),
					layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.AccountOwners), owner), view.Colors.SecondaryInk)),
				)
			})
		})
	})
	buy, sell, dividend := p.canTradeAccount(), p.canSell(), p.canRecordDividend()
	opening := p.canOpenPosition()
	if p.opening != nil && p.opening.saved == a.ID {
		rows = append(rows, view.Message(l.Text(i18n.OpeningPositionSaved), view.Colors.Ink))
	}
	if opening {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, view.Button(&p.opening.open, l.Text(i18n.OpeningPositionOpen)))
		})
	}
	if buy || sell || dividend {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				var actions []layout.FlexChild
				f := p.investment
				action := func(button *widget.Clickable, label i18n.MessageID) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						view.Focus.Watch(gtx, button)
						if f.restore == button && gtx.Enabled() {
							gtx.Execute(key.FocusCmd{Tag: button})
							f.restore = nil
						}
						return view.Button(button, l.Text(label))(gtx)
					})
				}
				if buy {
					label := i18n.BuyOpen
					if !p.canFirstBuy() {
						label = i18n.BuyExistingOpen
					}
					actions = append(actions, action(&f.buy, label))
				}
				if sell {
					actions = append(actions, action(&f.sell, i18n.SellOpen))
				}
				if dividend {
					actions = append(actions, action(&f.dividend, i18n.DividendOpen))
				}
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, actions...)
			})
		})
	}
	if p.canImportPosition() {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			f := p.investment
			buttons := []*widget.Clickable{&f.importOpen, &f.custodyOpen, &f.adjustmentOpen}
			labels := []i18n.MessageID{i18n.OpeningPositionOpen, i18n.CustodyOpen, i18n.QuantityAdjustOpen}
			return ui.Wrap{Gap: ui.Space8}.Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
				return f.accountPositionAction(gtx, view, buttons[i], l.Text(labels[i]))
			})
		})
	}
	if p.statement != nil && statementCard(a) && p.freshDetail() != nil {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, view.Button(&p.statement.open, l.Text(i18n.StatementOpen)))
			})
		})
	}
	if a.OpenedOn != nil {
		if day, valid := ui.ParseCivilDay(*a.OpenedOn); valid {
			rows = append(rows, accountLine(view, fmt.Sprintf(l.Text(i18n.AccountCreateReviewDate), ui.FormatDay(day, l, view.DateFormat))))
		}
	}
	if row.value == nil {
		return p.appendProducts(rows, view, a)
	}
	value := row.value
	issues := make(map[string]bool)
	addIssues := func(inputs []domain.MissingInputView) {
		for _, input := range inputs {
			issue := overviewIssue(l, input)
			if !issues[issue] {
				issues[issue] = true
				rows = append(rows, accountLine(view, issue))
			}
		}
	}
	addIssues(value.MissingInputs)
	if value.DisplayValue != nil {
		addIssues(value.DisplayValue.MissingInputs)
	}
	if a.TrackingMode == domain.TrackingHoldings {
		var cash, investments []layout.Widget
		for _, component := range value.Components {
			if component.InstrumentID == nil {
				cash = append(cash, accountLine(view, component.NativeCurrency.String()+" · "+accountComponentAmount(view, component)))
				continue
			}
			if p.current.managed[*component.InstrumentID] {
				continue
			}
			name := domain.InstrumentDisplayLabel(component.InstrumentName, component.InstrumentSymbol, component.InstrumentShortName)
			instrumentName := name
			if name == "" {
				name = l.Text(i18n.OverviewMissingInstrument)
			}
			if component.HoldingID != nil {
				if holding, ok := p.current.holdings[*component.HoldingID]; ok {
					if holding.Quantity.IsZero() {
						continue
					}
					name += " · " + fmt.Sprintf(l.Text(i18n.AccountQuantity), holding.Quantity.Canonical())
				}
			}
			investments = append(investments, accountLine(view, name+" · "+accountComponentAmount(view, component)))
			if instrumentName != "" && p.openInstrument != nil {
				id := *component.InstrumentID
				button := portfolioButton(p.instruments, id)
				investments = append(investments, func(gtx layout.Context) layout.Dimensions {
					if button.Clicked(gtx) {
						p.openInstrument(id, instrumentName, button)
						gtx.Execute(op.InvalidateCmd{})
					}
					label := fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), instrumentName)
					return layout.W.Layout(gtx, view.Button(button, label))
				})
			}
			if p.canImportPosition() && component.HoldingID != nil {
				if holding, ok := p.current.holdings[*component.HoldingID]; ok && holding.ArchivedAt == nil && !holding.Quantity.IsZero() {
					label := instrumentName
					if label == "" {
						label = l.Text(i18n.OverviewMissingInstrument)
					}
					investments = append(investments, p.holdingPositionActions(view, a, holding, label))
				}
			}
		}
		rows = append(rows, accountSection(view, l.Text(i18n.AccountCash)))
		if len(cash) == 0 {
			cash = append(cash, accountLine(view, l.Text(i18n.AccountNoCash)))
		}
		rows = append(rows, cash...)
		if a.AccountType != domain.TypeCashOnHand {
			rows = append(rows, accountSection(view, l.Text(i18n.AccountInvestments)))
			if len(investments) == 0 {
				investments = append(investments, accountLine(view, l.Text(i18n.AccountNoInvestments)))
			}
			rows = append(rows, investments...)
		}
	}
	return p.appendProducts(rows, view, a)
}

func (f *accountInvestment) accountPositionAction(gtx layout.Context, view ui.View, button *widget.Clickable, label string) layout.Dimensions {
	view.Focus.Watch(gtx, button)
	if f.restore == button && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: button})
		f.restore = nil
	}
	return view.Button(button, label)(gtx)
}

func (p *accountsPage) holdingPositionActions(view ui.View, account domain.Account, holding domain.Holding, name string) layout.Widget {
	f := p.investment
	if f.custodyHoldings == nil {
		f.custodyHoldings = make(map[domain.HoldingID]*widget.Clickable)
	}
	if f.adjustmentHoldings == nil {
		f.adjustmentHoldings = make(map[domain.HoldingID]*widget.Clickable)
	}
	custody := portfolioButton(f.custodyHoldings, holding.ID)
	adjust := portfolioButton(f.adjustmentHoldings, holding.ID)
	return func(gtx layout.Context) layout.Dimensions {
		scope := positionScope{account: account, holding: holding}
		if custody.Clicked(gtx) {
			scope.action = custody
			f.startCustody(p.investmentRuntime(), scope)
		} else if adjust.Clicked(gtx) {
			scope.action = adjust
			f.startAdjustment(p.investmentRuntime(), scope)
		}
		if scope.action != nil {
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
		return ui.Wrap{Gap: ui.Space8}.Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
			button, label := custody, i18n.CustodyHoldingAction
			if i == 1 {
				button, label = adjust, i18n.QuantityAdjustHoldingAction
			}
			return f.accountPositionAction(gtx, view, button, fmt.Sprintf(view.Language.Text(label), name))
		})
	}
}

func (p *accountsPage) appendProducts(rows []layout.Widget, view ui.View, account domain.Account) []layout.Widget {
	canAdd := p.productLedger != nil && domain.ProductAccountEligible(account) == nil
	if len(p.current.products) == 0 && !canAdd {
		return rows
	}
	l := view.Language
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Flexed(1, accountSection(view, l.Text(i18n.ManagedProducts))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canAdd {
					return layout.Dimensions{}
				}
				if p.request != nil || p.err != nil {
					gtx = gtx.Disabled()
				} else {
					if p.restoreProductFocus {
						gtx.Execute(key.FocusCmd{Tag: &p.addProduct})
						p.restoreProductFocus = false
					}
					if p.addProduct.Clicked(gtx) {
						p.startProductSetup(account, l.Text(i18n.ProductAddDeposit))
						view.RestorePageScroll(0)
						gtx.Execute(op.InvalidateCmd{})
					}
				}
				view.Focus.Watch(gtx, &p.addProduct)
				return view.Button(&p.addProduct, l.Text(i18n.ProductAddAction))(gtx)
			}))
	})
	if p.productSaved {
		rows = append(rows, accountLine(view, l.Text(i18n.ProductSetupSaved)))
	}
	if len(p.current.products) == 0 {
		rows = append(rows, accountLine(view, l.Text(i18n.ManagedProductsEmpty)))
	}
	for _, product := range p.current.products {
		name, id := productName(product), product.Contract.ID
		fields := []layout.Widget{
			productState(view, product.Contract.Kind, product.DisplayState),
			fundsAmount(view, i18n.ProductPrincipal, &product.Contract.Principal),
			fundsAmount(view, i18n.ProductValue, product.CurrentValue),
			productMaturity(view, product.Contract.MaturityOn),
		}
		if p.openProduct != nil {
			fields = append(fields, productLink(view, id, name, portfolioButton(p.productButtons, id), p.openProduct))
		}
		rows = append(rows, fundsCard(view, name, fields...))
	}
	return rows
}

// accountSection is the heading of a block of detail lines.
func accountSection(view ui.View, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: ui.Space24, Bottom: ui.Space4, Left: ui.Space4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextHeading, label, view.Colors.Ink)
		})
	}
}

func accountLine(view ui.View, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 7, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, view.Message(label, view.Colors.Ink))
	}
}

func accountComponentAmount(view ui.View, component domain.ValuationComponent) string {
	if component.NativeAmount == "" {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	text := valuationMoney(view, domain.MoneyView{Amount: component.NativeAmount, Currency: component.NativeCurrency}, false)
	if base := component.BaseAmount; base != nil && base.Currency != component.NativeCurrency {
		text += " · " + base.Currency.String() + " " + valuationMoney(view, *base, false)
	}
	return text
}

var accountTypes = map[domain.AccountType]i18n.MessageID{
	domain.TypeCashOnHand: i18n.AccountTypeCash, domain.TypeBankAccount: i18n.AccountTypeBank,
	domain.TypeBrokerage: i18n.AccountTypeBrokerage, domain.TypeInvestmentAccount: i18n.AccountTypeInvestment,
	domain.TypeCryptoExchange: i18n.AccountTypeCrypto, domain.TypeDigitalWallet: i18n.AccountTypeWallet,
	domain.TypePension: i18n.AccountTypePension, domain.TypeInsurancePolicy: i18n.AccountTypeInsurance,
	domain.TypeProperty: i18n.AccountTypeProperty, domain.TypeVehicle: i18n.AccountTypeVehicle,
	domain.TypeCollectible: i18n.AccountTypeCollectible, domain.TypeReceivable: i18n.AccountTypeReceivable,
	domain.TypeCreditCard: i18n.AccountTypeCredit, domain.TypeLoan: i18n.AccountTypeLoan, domain.TypeOther: i18n.AccountTypeOther,
}
