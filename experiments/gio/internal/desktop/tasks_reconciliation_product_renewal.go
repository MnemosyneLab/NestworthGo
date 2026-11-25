package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type holdingProductRenewal struct {
	settlement            holdingProductSettlement
	terms                 *holdingProductTerms
	principal, openingFee ui.TextField
}

func newHoldingProductRenewal(kind domain.ProductKind) *holdingProductRenewal {
	return &holdingProductRenewal{
		settlement: newHoldingProductSettlement(kind), terms: newHoldingProductTerms(),
		principal: ui.NewTextField(true), openingFee: ui.NewTextField(true),
	}
}

func (f *holdingProductRenewal) request(link domain.ReconciliationLink, id domain.ProductContractID, amount string) (liquidity.ProductCommandRequest, bool) {
	terms, policy, ok := f.terms.inputs()
	command := &application.RenewProductCommand{Settle: f.settlement.command(id, amount), Principal: strings.TrimSpace(f.principal.Text()),
		OpeningFee: productOptional(strings.TrimSpace(f.openingFee.Text())), Terms: terms, Policy: policy}
	return liquidity.ProductCommandRequest{Kind: string(domain.ProductOpRenew), ReconciliationLink: &link, Renew: command}, ok
}

func (c *holdingReconciliation) holdingProductRenewalFields(view ui.View, origin *image.Point, d *holdingReconDraft, ready bool) []layout.Widget {
	f := d.product.renewal
	rows := c.holdingProductSettlementFields(view, origin, d, &f.settlement, ready)
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return f.principal.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductRenewPrincipal, Disabled: !ready}, d.product.currency)
	}, func(gtx layout.Context) layout.Dimensions {
		return f.openingFee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductRenewOpeningFee, Disabled: !ready}, d.product.currency)
	})
	return append(rows, c.productTermsFields(view, origin, d, f.terms, ready, false)...)
}

func holdingRenewalFacts(view ui.View, request []byte, currency string) []layout.Widget {
	var command application.ProductCommand
	if jsoncodec.Unmarshal(request, &command) != nil || command.Renew == nil {
		return nil
	}
	r := command.Renew
	l := view.Language
	rows := holdingSettlementAmounts(view, r.Settle, currency)
	rows = append(rows,
		view.Message(l.Text(i18n.ProductRenewSuccessor)+" · "+r.Terms.Name, view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductRenewPrincipalFact), holdingSnapshotValue(view, r.Principal, currency)), view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductStart), historyDay(view, r.Terms.StartOn)), view.Colors.SecondaryInk),
		productMaturity(view, r.Terms.MaturityOn))
	if r.OpeningFee != nil {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.ProductRenewOpeningFeeFact), holdingSnapshotValue(view, *r.OpeningFee, currency)), view.Colors.Ink))
	}
	return rows
}
