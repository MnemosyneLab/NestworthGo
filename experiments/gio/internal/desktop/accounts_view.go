package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

const (
	accountWideContent   = 900
	accountRowStackWidth = 480
)

func (p *accountsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	if gtx.Enabled() && !p.accountActionOpen() {
		p.updateAccountActionTriggers(gtx)
	}
	if f := p.statement; f != nil {
		row := p.freshDetail()
		if f.screen == nil && row != nil && (p.target.statement != "" || f.restore.open || f.open.Clicked(gtx)) {
			if statementCard(row.record.Account) {
				selected := p.target.statement
				if f.restore.open {
					selected = f.restore.selected
				}
				p.startStatements(row.record.Account, selected)
				f.screen.list.Position = f.restore.position
				f.restore = statementVisit{}
			} else if p.target.statement != "" {
				return view.Message(l.Text(i18n.StatementLoadFailed), view.Colors.ErrorInk)(gtx)
			}
		}
		if f.screen != nil {
			return p.layoutStatements(gtx, view)
		}
	}
	detail := p.target.account != ""
	if p.clear.Clicked(gtx) && (p.target.institution != "" || p.target.accountList.active()) {
		p.open(navigationTarget{route: accountsRoute}, "", &p.clear)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.err != nil && p.reload.Clicked(gtx) {
		p.read()
	}
	if !detail && p.request == nil && !p.invalid {
		p.archiveFilter.Update(gtx)
		if p.archiveFilter.Value != p.archived {
			p.enter(p.target, p.archiveFilter.Value, layout.Position{})
		}
	}
	return ui.Column{Gap: 12}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			reload := func(gtx layout.Context) layout.Dimensions {
				label := l.Text(i18n.HistoryRetry)
				if p.request != nil {
					gtx = gtx.Disabled()
				}
				if detail {
					return view.ButtonOf(&p.reload, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: label})(gtx)
				}
				return view.Button(&p.reload, label)(gtx)
			}
			var left, right []layout.Widget
			if detail {

				if p.target.institution != "" {
					right = append(right, view.ButtonOf(&p.clear, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(i18n.AccountClearInstitution)}))
				} else if p.target.accountList.active() {
					right = append(right, returnMonthButton(view, &p.clear, i18n.AccountClearOverviewFilter, nil, false))
				}
			} else {
				left = append(left, view.Message(l.Text(i18n.AccountListDescription), view.Colors.SecondaryInk))
				if p.canCreateBankAccount() {
					left = append(left, p.createAction(view, &p.createShell, i18n.AccountCreateShellAdd, false))
				}
				if p.target.institution != "" {
					right = append(right, view.Button(&p.clear, l.Text(i18n.AccountClearInstitution)))
				} else if p.target.accountList.active() {
					right = append(right, returnMonthButton(view, &p.clear, i18n.AccountClearOverviewFilter, nil, false))
				}
				if !p.invalid && (p.current == nil || !p.current.unavailable) {
					right = append(right, func(gtx layout.Context) layout.Dimensions {
						if p.request != nil {
							gtx = gtx.Disabled()
						}
						label := l.Text(i18n.InstitutionsShowArchived)
						return accessibility.Checkbox(gtx, &p.archiveFilter, label, view.Checkbox(&p.archiveFilter, label))
					})
				}
			}
			if !p.invalid && p.err != nil {
				right = append(right, reload)
			}
			if detail {
				actions := append(left, right...)
				return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
					return actions[i](gtx)
				})
			}
			intro := func(gtx layout.Context) layout.Dimensions {
				dims := ui.Wrap{Gap: ui.Space8, Middle: true}.Layout(gtx, len(left), func(gtx layout.Context, i int) layout.Dimensions { return left[i](gtx) })
				dims.Size = gtx.Constraints.Constrain(dims.Size)
				return dims
			}
			controls := func(gtx layout.Context) layout.Dimensions {
				return ui.Wrap{Gap: ui.Space8, Middle: true}.Layout(gtx, len(right), func(gtx layout.Context, i int) layout.Dimensions { return right[i](gtx) })
			}
			return ui.Column{Gap: ui.Space12}.Layout(gtx,
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					if gtx.Constraints.Max.X < gtx.Dp(accountWideContent) {
						return ui.Column{Gap: ui.Space8}.Layout(gtx, ui.Rigid(intro), ui.Rigid(controls))
					}
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
						layout.Flexed(1, intro), layout.Rigid(controls))
				}), ui.Rigid(view.Hairline))
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil || p.current.unavailable || len(p.current.rows) == 0 {
				return layout.Dimensions{}
			}
			title := l.Text(i18n.AccountReadList)
			if detail {
				rows := p.detailRows(view)
				dims := pageListGap(gtx, view, &p.list, title, len(rows), 0, func(gtx layout.Context, i int) layout.Dimensions { return rows[i](gtx) })
				if f := p.balanceTarget; f != nil && f.restore == &f.open && p.restoreDetailActionFocus(gtx, &f.open) {
					f.restore = nil
				}
				if f := p.statement; f != nil && f.restoreFocus && p.restoreDetailActionFocus(gtx, &f.open) {
					f.restoreFocus = false
				}
				return dims
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
	if p.target.institution != "" && p.current != nil {
		institution := p.current.institutions[p.target.institution]
		lines = append(lines, layout.Rigid(view.Message(accountReference(l, institution.Name, institution.ArchivedAt != nil), view.Colors.Ink)))
	}
	if p.target.accountList.active() {
		label := p.target.accountList.name(l, p.filterLabel)
		lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Badge(gtx, ui.BadgeNeutral, fmt.Sprintf(l.Text(i18n.AccountOverviewFilter), label))
		}))
		if p.target.accountList.dimension == accountByMember {
			lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountMemberScopeHelp), view.Colors.SecondaryInk)))
		}
	}
	if p.createdID != "" && p.target.account == p.createdID {
		lines = append(lines, layout.Rigid(view.Message(l.Text(p.createdMessage), view.Colors.Ink)))
	}
	if p.productSaved {
		lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.ProductSetupSaved), view.Colors.Ink)))
	}
	if f := p.cashBalance; f != nil && f.saved.ID != "" && p.target.account == f.saved.AccountID {
		lines = append(lines, layout.Rigid(balanceSavedLine(view, i18n.AccountCashSaved, &f.view, f.saved.ActivityID != nil && f.openActivity != nil)))
	}
	if f := p.balanceTarget; f != nil && p.target.account != "" {
		if f.saved.account == p.target.account {
			lines = append(lines, layout.Rigid(balanceSavedLine(view, f.savedMessage, &f.view, f.saved.activity != "" && f.openActivity != nil)))

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
		children := []layout.FlexChild{layout.Flexed(1, view.Body(ui.TextLabel, row.heading, view.Colors.Ink))}
		if row.subtotal != nil {
			label := fmt.Sprintf(view.Language.Text(i18n.AccountInstitutionNet), valuationMoney(view, *row.subtotal, false))
			width := gtx.Constraints.Max.X / 2
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, width)
				return layout.E.Layout(gtx, view.Body(ui.TextCaptionStrong, label, view.Colors.SecondaryInk))
			}))
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
	})
}

func (p *accountsPage) accountTile(gtx layout.Context, view ui.View, row accountRow, first, last bool) layout.Dimensions {
	view = view.WithoutTextSelection()
	account := row.record.Account
	amounts := accountAmountLines(view, row, true)
	labels := make([]string, 0, len(amounts)+2)
	for _, amount := range amounts {
		labels = append(labels, amount.label)
	}
	status, _ := accountValuationStatus(row.value)
	if status != i18n.NoMessage {
		label := view.Language.Text(status)
		if status == i18n.AccountValuationPartial {
			label = accountIssueStatus(view.Language, *row.value, label)
		}
		labels = append(labels, label)
		if accountStale(*row.value) {
			labels = append(labels, view.Language.Text(i18n.AccountStaleQuotes))
		}
	}
	value := strings.Join(labels, " · ")
	button := p.buttons[account.ID]
	if button == nil {
		button = new(widget.Clickable)
		p.buttons[account.ID] = button
	}
	if button.Clicked(gtx) {
		p.open(navigationTarget{route: accountsRoute, account: account.ID}, account.Name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	inset := layout.Inset{}
	if last {
		inset.Bottom = ui.Space16
	}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, button)
		return view.RowCard(gtx, first, last, func(gtx layout.Context) layout.Dimensions {
			return accessibility.Button(gtx, button, account.Name, value, false, func(gtx layout.Context) layout.Dimensions {
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					semantic.Button.Add(gtx.Ops)
					semantic.LabelOp(account.Name).Add(gtx.Ops)
					semantic.DescriptionOp(value).Add(gtx.Ops)
					hover := func(gtx layout.Context) layout.Dimensions {
						if button.Hovered() || gtx.Focused(button) {
							paint.FillShape(gtx.Ops, ui.Mix(view.Colors.Surface, view.Colors.HoverSurface, 0.55), clip.Rect{Max: gtx.Constraints.Min}.Op())
						}
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}
					return layout.Background{}.Layout(gtx, hover, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							title := func(gtx layout.Context) layout.Dimensions { return accountTitle(gtx, view, row, status) }
							width := gtx.Constraints.Max.X
							balance := func(gtx layout.Context) layout.Dimensions {
								return layoutAccountAmounts(gtx, view, amounts, layout.Start)
							}
							return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return accountTypeTile(gtx, view, account, p.current.institutions)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if width < gtx.Dp(accountRowStackWidth) {
										return ui.Column{Gap: ui.Space8}.Layout(gtx, ui.Rigid(title), ui.Rigid(balance))
									}
									return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
										layout.Flexed(1, title),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layoutAccountAmounts(gtx, view, amounts, layout.End)
											})
										}))
								}),
							)
						})
					})
				})
			})
		})
	})
}

func accountTypeDescription(l i18n.Language, account domain.Account) string {
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
	return description
}

func accountTypeTile(gtx layout.Context, view ui.View, account domain.Account, institutions map[domain.InstitutionID]domain.Institution) layout.Dimensions {
	tone := 7
	switch account.AccountType {
	case domain.TypeBankAccount:
		tone = 0
	case domain.TypeCashOnHand, domain.TypeProperty, domain.TypePension:
		tone = 1
	case domain.TypeCreditCard:
		tone = 2
	case domain.TypeVehicle, domain.TypeReceivable:
		tone = 3
	case domain.TypeBrokerage, domain.TypeInvestmentAccount:
		tone = 4
	case domain.TypeCryptoExchange:
		tone = 5
	case domain.TypeLoan, domain.TypeCollectible:
		tone = 6
	}
	return view.IconTile(gtx, accountGlyph(account, institutions), 40, view.Colors.CategorySoft(tone), view.Colors.Category[tone])
}

func accountTitle(gtx layout.Context, view ui.View, row accountRow, status i18n.MessageID) layout.Dimensions {
	l := view.Language
	account := row.record.Account
	labels := []layout.Widget{view.Body(ui.TextHeading, account.Name, view.Colors.Ink)}
	badge := func(id i18n.MessageID, tone ui.BadgeTone) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(id)) }
	}
	labels = append(labels, badge(accountTypes[account.AccountType], ui.BadgeOutline))
	if account.IsLiability() {
		labels = append(labels, badge(i18n.AccountRoleLiability, ui.BadgeOutline))
	}
	if account.ArchivedAt != nil {
		labels = append(labels, badge(i18n.DirectoryArchived, ui.BadgeNeutral))
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(labels), func(gtx layout.Context, i int) layout.Dimensions {
				return labels[i](gtx)
			})
		}),
	}
	if status != i18n.NoMessage {
		valuation := []layout.Widget{view.Body(ui.TextCaption, l.Text(status), view.Colors.SecondaryInk)}
		if accountStale(*row.value) {
			valuation = append(valuation, view.Body(ui.TextCaption, l.Text(i18n.AccountStaleQuotes), view.Colors.WarningInk))
		}
		rows = append(rows, ui.Rigid(accountDetailActions(view, valuation)))
	}
	return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
}

func accountValuationStatus(value *domain.AccountValuation) (i18n.MessageID, ui.BadgeTone) {
	if value == nil {
		return i18n.NoMessage, ui.BadgeNeutral
	}
	if !value.Complete || (value.DisplayValue != nil && !value.DisplayValue.Complete) {
		return i18n.AccountValuationPartial, ui.BadgeWarning
	}
	return i18n.AccountValuationComplete, ui.BadgeSuccess
}

type accountAmountLine struct {
	style ui.TextStyle
	label string
}

func accountAmountLines(view ui.View, row accountRow, brief bool) []accountAmountLine {
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
		label := valuationMoney(view, *m, a.IsLiability())
		if partial {
			label += " · " + l.Text(i18n.AccountValuedSubtotal)
		}
		return label
	}
	primaryStyle := ui.TextMetricMD
	if !brief {
		primaryStyle = ui.TextMetricLG
	}
	primaryCurrency := a.DefaultCurrency
	if native != nil {
		primaryCurrency = native.Currency
	} else {
		primaryStyle = ui.TextLabel
	}
	lines := []accountAmountLine{{primaryStyle, amount(native, a.DefaultCurrency, nativePartial)}}
	if baseCurrency != "" && baseCurrency != primaryCurrency {
		label := amount(base, baseCurrency, !v.Complete)
		if base == nil && brief {
			pending := native != nil && slices.ContainsFunc(v.MissingInputs, func(input domain.MissingInputView) bool {
				return input.Kind == domain.MissingFXRate && input.BaseCurrency != primaryCurrency
			})
			if !pending {
				return lines
			}
			label = l.Text(i18n.AccountPendingConversion)
		}
		lines = append(lines, accountAmountLine{ui.TextCaption, label})
	}
	return lines
}

func layoutAccountAmounts(gtx layout.Context, view ui.View, amounts []accountAmountLine, alignment layout.Alignment) layout.Dimensions {
	lines := make([]layout.FlexChild, 0, len(amounts))
	for i, amount := range amounts {
		ink := view.Colors.SecondaryInk
		if i == 0 {
			ink = view.Colors.Ink
		}
		if alignment == layout.End {
			lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.LineEnd(gtx, amount.style, amount.label, ink)
			}))
		} else {
			lines = append(lines, layout.Rigid(view.Body(amount.style, amount.label, ink)))
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
	p.detail.investmentActionsRow = -1
	clear(p.detail.actionRows)
	rows := []layout.Widget{accountDetailSummary(view, row, valuationReadTime(view, p.current.readAt), institution, group, owner)}
	inclusion := i18n.AccountIncludedInNetWorth
	switch {
	case a.ArchivedAt != nil:
		inclusion = i18n.AccountArchivedExclusion
	case !a.IncludeInNetWorth:
		inclusion = i18n.AccountExcludedFromNetWorth
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: ui.Space12}.Layout(gtx, accountLine(view, l.Text(inclusion)))
	})
	if f := p.statement; f != nil && statementCard(a) && p.freshDetail() != nil {
		p.trackDetailActionRow(&f.open, len(rows))
	}
	rows = append(rows, accountSectionGap(p.accountReconciliationRow(view, a)))
	if value := row.value; value != nil {
		var issues []layout.Widget
		seen := make(map[string]bool)
		addIssues := func(inputs []domain.MissingInputView) {
			for _, input := range inputs {
				issue := overviewIssue(l, input)
				if !seen[issue] {
					seen[issue] = true
					issues = append(issues, view.Message(issue, view.Colors.WarningInk))
				}
			}
		}
		addIssues(value.MissingInputs)
		if value.DisplayValue != nil {
			addIssues(value.DisplayValue.MissingInputs)
		}
		if len(issues) > 0 {
			rows = append(rows, accountSectionGap(sectionCard(view, issues)))
		}
	}
	if row.value != nil && a.TrackingMode == domain.TrackingHoldings && !a.IsLiability() {
		if composition := p.accountComposition(view, row.value.Composition); composition != nil {
			rows = append(rows, accountSectionGap(composition))
		}
	}
	debt := p.accountDebtShortcut(view)
	if a.TrackingMode != domain.TrackingHoldings {
		var actions []layout.Widget
		if p.canUpdateBalanceTarget() {
			copy := balanceTargetCopyFor(a)
			button := &p.balanceTarget.open
			p.trackDetailActionRow(button, len(rows))
			actions = append(actions, func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, button)
				return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(copy.action)})(gtx)
			})
		}
		actions = append(actions, p.cashMovementActions(view)...)
		if debt != nil {
			actions = append(actions, debt)
		}
		rows = append(rows, accountSectionGap(accountSimpleValue(view, row, actions)))
	}
	if a.OpenedOn != nil {
		if day, valid := ui.ParseCivilDay(*a.OpenedOn); valid {
			rows = append(rows, accountLine(view, fmt.Sprintf(l.Text(i18n.AccountCreateReviewDate), ui.FormatDay(day, l, view.DateFormat))))
		}
	}
	if a.TrackingMode == domain.TrackingHoldings {
		rows = append(rows, accountSection(view, l.Text(i18n.AccountCash)), p.accountCashSection(view, row.value, debt))
	}
	if a.TrackingMode == domain.TrackingHoldings && a.AccountType != domain.TypeCashOnHand {
		rows = append(rows, p.accountPositionRows(view, a, len(rows))...)
	}
	rows = p.appendProducts(rows, view, a)
	if history := p.accountCashHistory(view); history != nil {
		rows = append(rows, accountSectionGap(history))
	}
	return rows
}

func (p *accountsPage) accountCashSection(view ui.View, value *domain.AccountValuation, debt layout.Widget) layout.Widget {
	l := view.Language
	var rows []layout.Widget
	if value != nil {
		for _, component := range value.Components {
			if component.InstrumentID == nil {
				currency, amount := component.NativeCurrency.String(), accountComponentAmount(view, component)
				rows = append(rows, func(gtx layout.Context) layout.Dimensions {
					base := component.BaseAmount
					var amounts = []accountAmountLine{{ui.TextLabel, amount}}
					if base != nil && base.Currency != component.NativeCurrency {
						amounts = append(amounts, accountAmountLine{ui.TextCaption, valuationMoney(view, *base, false)})
					}
					return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
						layout.Flexed(1, view.Body(ui.TextLabel, currency, view.Colors.Ink)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layoutAccountAmounts(gtx, view, amounts, layout.End)
						}),
					)
				})
			}
		}
	}
	if len(rows) == 0 {
		label := i18n.AccountNoCash
		if value == nil {
			label = i18n.OverviewNoValue
		}
		rows = append(rows, accountLine(view, l.Text(label)))
	}
	var actions []layout.Widget
	if p.canSetCashBalance() {
		button := &p.cashBalance.open
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, button)
			return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
				Title: l.Text(i18n.AccountCashSet)})(gtx)
		})
	}
	actions = append(actions, p.cashMovementActions(view)...)
	if debt != nil {
		actions = append(actions, debt)
	}
	if len(actions) != 0 {
		rows = append(rows, accountDetailActions(view, actions))
	}
	return sectionCard(view, rows)
}

func (f *accountInvestment) watchAccountAction(gtx layout.Context, view ui.View, button *widget.Clickable) {
	view.Focus.Watch(gtx, button)
	if f != nil && f.restore == button && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: button})
		f.restore = nil
	}
}

func (f *accountInvestment) accountPositionAction(gtx layout.Context, view ui.View, button *widget.Clickable, label string) layout.Dimensions {
	f.watchAccountAction(gtx, view, button)
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
	l := view.Language
	if account.IsLiability() {
		return rows
	}
	if account.TrackingMode != domain.TrackingHoldings || account.AccountType == domain.TypeCashOnHand {
		fields := []layout.Widget{
			view.Body(ui.TextHeading, l.Text(i18n.AccountProductsTitle), view.Colors.Ink),
			view.Message(l.Text(i18n.AccountProductsGuide), view.Colors.SecondaryInk),
		}
		if account.ArchivedAt == nil && p.creation != nil {
			button := &p.createProductAccount
			p.trackDetailActionRow(button, len(rows))
			fields = append(fields, func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, button)
				if p.focusCreate == button && gtx.Enabled() {
					p.focusCreate = nil
					gtx.Execute(key.FocusCmd{Tag: button})
				}
				if button.Clicked(gtx) && p.freshDetail() != nil {
					kind := account.AccountType
					if !slices.Contains(accountModes(kind, domain.RoleAsset), domain.TrackingHoldings) || kind == domain.TypeCashOnHand {
						kind = domain.TypeBankAccount
					}
					p.creation.sheet = newAccountCreateSheet(accountDetails, button)
					p.startAccount(kind, domain.RoleAsset, domain.TrackingHoldings, domain.Institution{})
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
					Title: l.Text(i18n.AccountProductsCreateHoldings)})(gtx)
			})
		}
		return append(rows, accountSectionGap(sectionCard(view, fields)))
	}
	productCount := len(p.current.products)
	heading := view.Body(ui.TextHeading, l.Text(i18n.AccountProductsTitle), view.Colors.Ink)
	if canAdd {
		p.trackDetailActionRow(&p.addProduct, len(rows))
		heading = accountDetailActions(view, []layout.Widget{heading, func(gtx layout.Context) layout.Dimensions {
			if p.request != nil || p.err != nil {
				gtx = gtx.Disabled()
			} else {
				if p.restoreProductFocus {
					gtx.Execute(key.FocusCmd{Tag: &p.addProduct})
					p.restoreProductFocus = false
				}
				if p.addProduct.Clicked(gtx) {
					p.startProductSetup(account, l.Text(i18n.ProductAddDeposit))
					gtx.Execute(op.InvalidateCmd{})
				}
			}
			view.Focus.Watch(gtx, &p.addProduct)
			return view.ButtonOf(&p.addProduct, ui.ButtonSpec{Variant: ui.ButtonSoft, Size: ui.ButtonSmall,
				Title: l.Text(i18n.ProductAddAction)})(gtx)
		}})
	}
	rows = append(rows, accountSectionGap(accountProductCardRow(view, true, false, heading)))
	if productCount == 0 {
		return append(rows, accountProductCardRow(view, false, true, view.Message(l.Text(i18n.ManagedProductsEmpty), view.Colors.SecondaryInk)))
	}
	for i, product := range p.current.products {
		name, id := productName(product), product.Contract.ID
		title := view.Body(ui.TextLabel, name, view.Colors.Ink)
		if p.openProduct != nil {
			button := portfolioButton(p.productButtons, id)
			p.trackDetailActionRow(button, len(rows))
			title = func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, button)
				if button.Clicked(gtx) {
					p.openProduct(id, name, button)
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: name,
					Name: fmt.Sprintf(l.Text(i18n.ProductOpen), name)})(gtx)
			}
		}
		fields := []layout.Widget{
			labeledAmount(view, i18n.ProductPrincipal, fundsAmountText(view, &product.Contract.Principal), view.Colors.Ink),
			labeledAmount(view, i18n.ProductValue, fundsAmountText(view, product.CurrentValue), view.Colors.Ink),
			productMaturity(view, product.Contract.MaturityOn),
		}
		rows = append(rows, accountProductCardRow(view, false, i == productCount-1, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx,
				ui.Rigid(accountDetailActions(view, []layout.Widget{title,
					productState(view, product.Contract.Kind, product.DisplayState)})),
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (ui.Grid{MinColumn: 210, MaxColumns: 3, Gap: ui.Space12}).Layout(gtx, view, len(fields),
						func(gtx layout.Context, _ ui.View, i int) layout.Dimensions { return fields[i](gtx) })
				}),
			)
		}))
	}
	return rows
}

func accountProductCardRow(view ui.View, first, last bool, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.RowCard(gtx, first, last, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, content)
		})
	}
}

func (p *accountsPage) trackDetailActionRow(button *widget.Clickable, row int) {
	if p.detail.actionRows == nil {
		p.detail.actionRows = make(map[*widget.Clickable]int)
	}
	p.detail.actionRows[button] = row
}

func (p *accountsPage) restoreDetailActionFocus(gtx layout.Context, button *widget.Clickable) bool {
	if !gtx.Enabled() || p.request != nil || p.err != nil {
		return false
	}
	row, found := p.detail.actionRows[button]
	if !found {
		return true
	}
	position := p.list.Position
	last := position.First + position.Count - 1
	// A receipt or reconciliation action can shrink the viewport on return.
	// Preserve the existing offset when the action row is already visible.
	clipped := row < position.First || row > last || row == position.First && position.Offset > 0 ||
		row == last && position.OffsetLast < 0 && row != position.First
	if clipped && p.revealAccountDetailAction(button) {
		gtx.Execute(op.InvalidateCmd{})
		return false
	}
	p.detail.revealedShortcut = nil
	gtx.Execute(key.FocusCmd{Tag: button})
	return true
}

// accountSection is the heading of a block of detail lines.
func accountSection(view ui.View, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: ui.Space24, Bottom: ui.Space4, Left: ui.Space4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextHeading, label, view.Colors.Ink)
		})
	}
}

// stackWidgets lays widgets out in a column with a fixed gap.
func sectionCard(view ui.View, items []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			var children []layout.FlexChild
			for i, item := range items {
				if i > 0 {
					children = append(children, layout.Rigid(view.Hairline))
				}
				children = append(children, layout.Rigid(item))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, children...)
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
	return valuationMoney(view, domain.MoneyView{Amount: component.NativeAmount, Currency: component.NativeCurrency}, false)
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
