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
	fee ui.TextField
	holdingProductTime
}

func (f *holdingProductSettlement) request(link domain.ReconciliationLink, id domain.ProductContractID, amount string) liquidity.ProductCommandRequest {
	command := f.command(id, amount)
	return liquidity.ProductCommandRequest{Kind: string(domain.ProductOpSettle), ReconciliationLink: &link, Settle: &command}
}

func (f *holdingProductSettlement) command(id domain.ProductContractID, amount string) application.SettleProductCommand {
	return application.SettleProductCommand{ProductID: id.String(), GrossProceeds: &amount, Fee: productOptional(strings.TrimSpace(f.fee.Text())),
		EffectiveLocalDate: f.day, EffectiveLocalTime: f.minute}
}

func (c *holdingReconciliation) holdingProductSettlementFields(view ui.View, origin *image.Point, d *holdingReconDraft, f *holdingProductSettlement, ready bool) []layout.Widget {
	rows := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		return f.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: !ready}, d.product.currency)
	}}
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
	if terms.GrossProceeds == nil {
		return nil
	}
	fact := func(label i18n.MessageID, amount string) layout.Widget {
		return view.Message(fmt.Sprintf(view.Language.Text(label), holdingSnapshotValue(view, amount, currency)), view.Colors.Ink)
	}
	rows := []layout.Widget{fact(i18n.HoldingReconGrossProceedsFact, *terms.GrossProceeds)}
	if terms.Fee != nil {
		return append(rows, fact(i18n.ProductFeeFact, *terms.Fee))
	}
	return append(rows, view.Message(view.Language.Text(i18n.ProductNoReceiptFee), view.Colors.SecondaryInk))
}
