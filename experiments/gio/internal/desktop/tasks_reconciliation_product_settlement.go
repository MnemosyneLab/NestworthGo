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

type holdingProductSettlement struct {
	kind          domain.ProductKind
	fee, interest ui.TextField
	holdingProductTime
}

func newHoldingProductSettlement(kind domain.ProductKind) holdingProductSettlement {
	return holdingProductSettlement{kind: kind, fee: ui.NewTextField(true), interest: ui.NewTextField(true)}
}

func (f *holdingProductSettlement) request(link domain.ReconciliationLink, id domain.ProductContractID, amount string) liquidity.ProductCommandRequest {
	command := f.command(id, amount)
	return liquidity.ProductCommandRequest{Kind: string(domain.ProductOpSettle), ReconciliationLink: &link, Settle: &command}
}

func (f *holdingProductSettlement) command(id domain.ProductContractID, amount string) application.SettleProductCommand {
	command := application.SettleProductCommand{ProductID: id.String(), Fee: productOptional(strings.TrimSpace(f.fee.Text())),
		EffectiveLocalDate: f.day, EffectiveLocalTime: f.minute}
	if f.kind == domain.ProductTermDeposit {
		command.ReturnedPrincipal = &amount
		command.Interest = productOptional(strings.TrimSpace(f.interest.Text()))
	} else {
		command.GrossProceeds = &amount
	}
	return command
}

func (c *holdingReconciliation) holdingProductSettlementFields(view ui.View, origin *image.Point, d *holdingReconDraft, f *holdingProductSettlement, ready bool) []layout.Widget {
	var rows []layout.Widget
	if f.kind == domain.ProductTermDeposit {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return f.interest.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductActualInterest, Disabled: !ready}, d.product.currency)
		})
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return f.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: !ready}, d.product.currency)
	})
	return append(rows, c.holdingProductTimeFields(view, origin, d, &f.holdingProductTime, &f.fee.Editor, ready)...)
}

func holdingSettlementFacts(view ui.View, request []byte, currency string) []layout.Widget {
	var command application.ProductCommand
	if jsoncodec.Unmarshal(request, &command) != nil || command.Settle == nil {
		return nil
	}
	return holdingSettlementAmounts(view, *command.Settle, currency)
}

func holdingSettlementAmounts(view ui.View, terms application.SettleProductCommand, currency string) []layout.Widget {
	fact := func(label i18n.MessageID, amount string) layout.Widget {
		return view.Message(fmt.Sprintf(view.Language.Text(label), holdingSnapshotValue(view, amount, currency)), view.Colors.Ink)
	}
	var rows []layout.Widget
	if terms.ReturnedPrincipal != nil {
		rows = append(rows, fact(i18n.ProductPrincipalFact, *terms.ReturnedPrincipal))
		if terms.Interest != nil {
			rows = append(rows, fact(i18n.ProductInterestFact, *terms.Interest))
		}
	} else if terms.GrossProceeds != nil {
		rows = append(rows, fact(i18n.HoldingReconGrossProceedsFact, *terms.GrossProceeds))
	} else {
		return nil
	}
	if terms.Fee != nil {
		return append(rows, fact(i18n.ProductFeeFact, *terms.Fee))
	}
	return append(rows, view.Message(view.Language.Text(i18n.ProductNoReceiptFee), view.Colors.SecondaryInk))
}
