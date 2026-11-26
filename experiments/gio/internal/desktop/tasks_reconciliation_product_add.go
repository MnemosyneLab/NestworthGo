package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type holdingProductAdd struct {
	terms                                     *holdingProductTerms
	mode                                      domain.ProductOperationKind
	currency, method, kind                    ui.Select
	principal, currentValue, cost, openingFee ui.TextField
	excludesCash                              widget.Bool
	time                                      holdingProductTime
	depositName                               string
}

func newHoldingProductAdd(depositName string) *holdingProductAdd {
	f := &holdingProductAdd{mode: domain.ProductOpOpen, principal: ui.NewTextField(true), currentValue: ui.NewTextField(true),
		cost: ui.NewTextField(true), openingFee: ui.NewTextField(true), depositName: depositName}
	f.openingFee.Editor.SetText("0")
	f.pickKind(string(domain.ProductTermDeposit))
	return f
}

func (f *holdingProductAdd) pickKind(kind string) {
	start, name := "", ""
	if f.terms != nil {
		start, name = f.terms.terms.StartOn, f.terms.name.Text()
	}
	form := newHoldingProductTerms()
	form.name.Editor.SetText(name)
	form.terms = application.ProductTermsInput{Kind: kind, StartOn: start, InterestMode: string(domain.InterestNone)}
	form.policy = application.ProductPolicyInput{AccessKind: string(domain.AccessOnRequest), EarlyKind: string(domain.EarlyNotAllowed)}
	form.days.Editor.SetText("0")
	form.fee.Editor.SetText("0")
	if kind == string(domain.ProductTermDeposit) {
		form.deposit = newHoldingDepositTerms()
	}
	f.terms = form
}

func (p *tasksPage) openHoldingProductAdd(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable, depositName string) {
	c := p.holdingRecon
	d := newHoldingReconDraft(entry)
	d.accountID, d.itemID = account.AccountID, item.ID
	d.product = &holdingProductDraft{add: newHoldingProductAdd(depositName), link: c.datedCheckpointLink(account.AccountID, item), name: account.Name, currency: account.Currency}
	c.draft, c.saved = d, i18n.NoMessage
}

func (f *holdingProductAdd) request(link domain.ReconciliationLink, currency string) (liquidity.ProductCommandRequest, bool) {
	command, valid := f.command(link.AccountID, currency)
	return liquidity.ProductCommandRequest{Kind: string(command.Kind), ReconciliationLink: &link, Open: command.Open, RecordExisting: command.RecordExisting}, valid
}

func (f *holdingProductAdd) command(accountID, currency string) (application.ProductCommand, bool) {
	terms, policy, valid := f.terms.inputs()
	if f.terms.deposit != nil && strings.TrimSpace(terms.Name) == "" {
		terms.Name = f.depositName
	}
	principal := strings.TrimSpace(f.principal.Text())
	req := application.ProductCommand{Kind: f.mode}
	if f.mode == domain.ProductOpRecordExisting {
		value, cost := strings.TrimSpace(f.currentValue.Text()), strings.TrimSpace(f.cost.Text())
		if f.terms.deposit != nil {
			if value == "" {
				value = principal
			}
			if cost == "" {
				cost = principal
			}
		}
		req.RecordExisting = &application.RecordExistingProductCommand{AccountID: accountID, Currency: currency, Principal: principal,
			CurrentValue: value, TotalCostBasis: cost, CashExcludesProduct: f.excludesCash.Value, Terms: terms, Policy: policy}
	} else {
		req.Open = &application.OpenProductCommand{AccountID: accountID, Currency: currency, Principal: principal,
			OpeningFee: productOptional(strings.TrimSpace(f.openingFee.Text())), EffectiveLocalDate: f.time.day, EffectiveLocalTime: f.time.minute, Terms: terms, Policy: policy}
	}
	return req, valid
}

func (c *holdingReconciliation) holdingProductAddFields(view ui.View, origin *image.Point, d *holdingReconDraft, ready bool) []layout.Widget {
	product := d.product
	cash := view.Language.Text(i18n.OverviewNoValue)
	if account, _, ok := c.checkpoint(d.accountID, d.itemID, false); ok {
		for _, item := range account.Items {
			if item.Kind == "cash" && item.Currency == product.currency && item.Value != "" {
				cash = holdingSnapshotValue(view, item.Value, item.Currency)
				break
			}
		}
	}
	return product.add.fields(view, origin, c.formCalendar(), product.name, &product.currency, cash, ready, d.focusAdjustmentField, func() { d.problem = i18n.NoMessage })
}

func (f *holdingProductAdd) fields(view ui.View, origin *image.Point, calendar productFormCalendar, name string, currency *string, cash string, ready bool, focus func(layout.Context, *ui.TextField), changed func()) []layout.Widget {
	l := view.Language
	pick := func(selectControl *ui.Select, label i18n.MessageID, options []ui.Choice, value string, searchable bool, change func(string)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			selectControl.Layout(gtx, view, ui.PickerField{Label: label, Origin: *origin, Disabled: !ready}, ui.SelectOptions{Items: options, Searchable: searchable}, value, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	text := func(field *ui.TextField, label i18n.MessageID) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if ready && field == &f.principal {
				focus(gtx, field)
			}
			return field.LayoutCurrency(gtx, view, ui.FieldOptions{Label: label, Disabled: !ready}, *currency)
		}
	}
	currencies := make([]ui.Choice, 0, len(domain.SupportedCurrencies()))
	for _, currency := range domain.SupportedCurrencies() {
		currencies = append(currencies, ui.Choice{ID: currency.String(), Label: currency.String()})
	}
	methods := []ui.Choice{{ID: string(domain.ProductOpOpen), Label: l.Text(i18n.ProductAddOpen)}, {ID: string(domain.ProductOpRecordExisting), Label: l.Text(i18n.ProductAddRecordExisting)}}
	kinds := []ui.Choice{{ID: string(domain.ProductTermDeposit), Label: l.Text(i18n.ProductAddDeposit)}, {ID: string(domain.ProductLockedProduct), Label: l.Text(i18n.ProductAddLocked)}}
	help := i18n.ProductAddOpenHelp
	if f.mode == domain.ProductOpRecordExisting {
		help = i18n.ProductAddExistingHelp
	}
	rows := []layout.Widget{view.Message(name, view.Colors.Ink),
		pick(&f.currency, i18n.ProductAddCurrency, currencies, *currency, true, func(v string) { *currency = v }),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductAddCash), cash), view.Colors.SecondaryInk),
		pick(&f.method, i18n.ProductAddMethod, methods, string(f.mode), false, func(v string) { f.mode = domain.ProductOperationKind(v) }),
		view.Message(l.Text(help), view.Colors.SecondaryInk),
		pick(&f.kind, i18n.ProductAddKind, kinds, f.terms.terms.Kind, false, f.pickKind),
		text(&f.principal, i18n.ProductAddPrincipal)}
	if f.mode == domain.ProductOpRecordExisting {
		rows = append(rows, text(&f.currentValue, i18n.ProductAddCurrent), text(&f.cost, i18n.ProductAddCost),
			checkpointCheckbox(view, &f.excludesCash, i18n.ProductAddExcludeCash, ready))
	}
	rows = append(rows, f.terms.fields(view, origin, calendar, *currency, f.principal.Text(), ready, false, focus)...)
	if f.mode == domain.ProductOpOpen {
		rows = append(rows, text(&f.openingFee, i18n.ProductRenewOpeningFee))
		rows = append(rows, f.time.fields(view, origin, calendar, &f.openingFee.Editor, ready, changed)...)
	}
	return rows
}

func holdingProductAddFacts(view ui.View, request []byte) []layout.Widget {
	var command application.ProductCommand
	if jsoncodec.Unmarshal(request, &command) != nil {
		return nil
	}
	l := view.Language
	fact := func(label i18n.MessageID, value, currency string) layout.Widget {
		return view.Message(fmt.Sprintf(l.Text(label), holdingSnapshotValue(view, value, currency)), view.Colors.Ink)
	}
	var terms application.ProductTermsInput
	var rows []layout.Widget
	switch {
	case command.Open != nil:
		terms = command.Open.Terms
		rows = append(rows, fact(i18n.ProductPrincipal, command.Open.Principal, command.Open.Currency))
		if command.Open.OpeningFee != nil {
			rows = append(rows, fact(i18n.ProductRenewOpeningFeeFact, *command.Open.OpeningFee, command.Open.Currency))
		}
	case command.RecordExisting != nil:
		existing := command.RecordExisting
		terms = existing.Terms
		rows = append(rows, fact(i18n.ProductPrincipal, existing.Principal, existing.Currency), fact(i18n.ProductAddValueFact, existing.CurrentValue, existing.Currency), fact(i18n.ProductAddCostFact, existing.TotalCostBasis, existing.Currency))
	default:
		return nil
	}
	rows = append(rows, view.Message(terms.Name, view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductStart), historyDay(view, terms.StartOn)), view.Colors.SecondaryInk),
		productMaturity(view, terms.MaturityOn))
	return rows
}
