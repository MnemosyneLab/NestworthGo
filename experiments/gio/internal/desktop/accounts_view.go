package desktop

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
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
	if p.canCreateBankAccount() && p.createBank.Clicked(gtx) {
		p.startBankAccount()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.creation != nil && p.creation.draft != nil {
		return p.layoutBankAccount(gtx, view)
	}
	detail := p.target.account != ""
	if p.back.Clicked(gtx) && detail {
		p.backToList()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.clear.Clicked(gtx) && p.target.institution != "" {
		p.open(navigationTarget{route: accountsRoute}, "", &p.clear)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.reload.Clicked(gtx) {
		p.read()
	}
	if p.archiveFilter.Clicked(gtx) && !detail && p.request == nil && !p.invalid {
		p.enter(p.target, !p.archived, layout.Position{})
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(ui.LimitWidth(gtx, 830),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var controls []layout.FlexChild
			if detail {
				controls = append(controls, layout.Rigid(view.Button(&p.back, l.Text(i18n.AccountBack))))
			} else if !p.invalid && (p.current == nil || !p.current.unavailable) {
				archive := i18n.InstitutionsShowArchived
				if p.archived {
					archive = i18n.InstitutionsHideArchived
				}
				controls = append(controls, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					return view.TabButton(&p.archiveFilter, l.Text(archive), p.archived)(gtx)
				}))
			}
			if p.target.institution != "" {
				controls = append(controls, layout.Rigid(view.Button(&p.clear, l.Text(i18n.AccountClearInstitution))))
			}
			if p.canCreateBankAccount() {
				controls = append(controls, layout.Rigid(view.PrimaryButton(&p.createBank, l.Text(i18n.AccountCreateAdd))))
			}
			controls = append(controls, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }))
			if !p.invalid {
				reload := i18n.HistoryRefresh
				if p.err != nil {
					reload = i18n.HistoryRetry
				}
				controls = append(controls, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reload, l.Text(reload))(gtx)
				}))
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, controls...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if p.current == nil || p.current.unavailable || len(p.current.rows) == 0 {
				return layout.Dimensions{}
			}
			if detail {
				rows := p.detailRows(view)
				return p.scrolling(gtx, view, len(rows), func(gtx layout.Context, i int) layout.Dimensions { return rows[i](gtx) })
			}
			return p.scrolling(gtx, view, len(p.rows), func(gtx layout.Context, i int) layout.Dimensions {
				if row := p.rows[i]; row.account != nil {
					return p.accountTile(gtx, view, *row.account)
				} else {
					return layout.Inset{Top: 9, Bottom: 6, Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, row.heading, 15, font.Medium, view.Colors.Ink)
					})
				}
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
	}
	var lines []layout.FlexChild
	if p.createdID != "" && p.target.account == p.createdID {
		lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountCreateSucceeded), view.Colors.Ink)))
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

func (p *accountsPage) scrolling(gtx layout.Context, view ui.View, count int, row layout.ListElement) layout.Dimensions {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	view.Focus.Watch(gtx, &p.list)
	event.Op(gtx.Ops, &p.list)
	for _, pressed := range ui.PressedKeys(gtx, &p.list, key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd) {
		page := float32(max(1, p.list.Position.Count-1))
		switch pressed.Name {
		case key.NameUpArrow:
			p.list.ScrollBy(-1)
		case key.NameDownArrow:
			p.list.ScrollBy(1)
		case key.NamePageUp:
			p.list.ScrollBy(-page)
		case key.NamePageDown:
			p.list.ScrollBy(page)
		case key.NameHome:
			p.list.ScrollTo(0)
		case key.NameEnd:
			p.list.ScrollTo(count - 1)
		}
	}
	return accessibility.List(gtx, &p.list, view.Language.Text(i18n.AccountReadList), func(gtx layout.Context) layout.Dimensions {
		return material.List(view.Theme, &p.list).Layout(gtx, count, row)
	})
}

func (p *accountsPage) accountTile(gtx layout.Context, view ui.View, row accountRow) layout.Dimensions {
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
	content.Background, content.CornerRadius = view.Colors.Surface, 8
	return layout.Inset{Bottom: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, button)
		return accessibility.Button(gtx, button, account.Name, "", false, func(gtx layout.Context) layout.Dimensions {
			return content.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.Button.Add(gtx.Ops)
				semantic.DescriptionOp(account.Name).Add(gtx.Ops)
				return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(10)}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return accountTitle(gtx, view, account) }),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return accountAmounts(gtx, view, row, true) }),
					)
				})
			})
		})
	})
}

func accountTitle(gtx layout.Context, view ui.View, account domain.Account) layout.Dimensions {
	l := view.Language
	description := l.Text(accountTypes[account.AccountType])
	if account.ArchivedAt != nil {
		description += " · " + l.Text(i18n.DirectoryArchived)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, account.Name, 17, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(description, view.Colors.SecondaryInk)),
	)
}

func accountAmounts(gtx layout.Context, view ui.View, row accountRow, brief bool) layout.Dimensions {
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
	lines := []layout.FlexChild{layout.Rigid(view.Message(amount(native, a.DefaultCurrency, nativePartial), view.Colors.Ink))}
	if baseCurrency != "" && baseCurrency != a.DefaultCurrency {
		lines = append(lines, layout.Rigid(view.Message(amount(base, baseCurrency, !v.Complete), view.Colors.SecondaryInk)))
	}
	if v != nil {
		if !v.Complete || nativePartial {
			status := l.Text(i18n.OverviewIncomplete)
			if brief {
				status = accountIssueStatus(l, *v, status)
			}
			lines = append(lines, layout.Rigid(view.Message(status, view.Colors.SecondaryInk)))
		}
		if accountStale(*v) {
			lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountStaleQuotes), view.Colors.SecondaryInk)))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx, lines...)
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
		return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(7)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountTitle(gtx, view, a) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountAmounts(gtx, view, row, false) }),
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountInstitution), institution), view.Colors.SecondaryInk)),
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountGroup), group), view.Colors.SecondaryInk)),
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountOwners), owner), view.Colors.SecondaryInk)),
				)
			})
		})
	})
	if row.value == nil {
		return rows
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
			name := domain.InstrumentDisplayLabel(component.InstrumentName, component.InstrumentSymbol, component.InstrumentShortName)
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
		}
		rows = append(rows, accountLine(view, l.Text(i18n.AccountCash)))
		if len(cash) == 0 {
			cash = append(cash, accountLine(view, l.Text(i18n.AccountNoCash)))
		}
		rows = append(rows, cash...)
		if a.AccountType != domain.TypeCashOnHand {
			rows = append(rows, accountLine(view, l.Text(i18n.AccountInvestments)))
			if len(investments) == 0 {
				investments = append(investments, accountLine(view, l.Text(i18n.AccountNoInvestments)))
			}
			rows = append(rows, investments...)
		}
	}
	return rows
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
