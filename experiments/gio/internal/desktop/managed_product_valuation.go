package desktop

import (
	"context"
	"errors"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type productValuationLedger interface {
	AppendProductValuation(context.Context, application.AppendProductValuationInput) (application.ProductDetail, error)
}

type productValuationDraft struct {
	name, currency string
	before         *domain.Money
	amount         ui.TextField
	cancel, save   widget.Clickable
	submission     *application.AppendProductValuationInput
	posting        <-chan historyResult[application.ProductDetail]
	problem        i18n.MessageID
	focusAmount    bool
	focusRetry     bool
}

func canUpdateProductValuation(detail application.ProductDetail) bool {
	return detail.Contract.Kind == domain.ProductLockedProduct && detail.Contract.State == domain.ProductStateOpen
}

func (p *productDetailPage) beginValuation() {
	detail := p.current.detail
	p.saved = i18n.NoMessage
	p.valuation = &productValuationDraft{name: productName(*detail), currency: detail.Contract.Currency.String(),
		before: detail.CurrentValue, amount: ui.NewTextField(true), focusAmount: true}
}

func (p *productDetailPage) valuationUnresolved() bool {
	return p.valuation != nil && p.valuation.submission != nil
}

func (p *productDetailPage) postValuation() {
	d := p.valuation
	if d.posting != nil {
		return
	}
	if d.submission == nil {
		amount := strings.TrimSpace(d.amount.Text())
		if amount == "" || d.problem == i18n.ProductValuationUnavailable {
			return
		}
		// Wails records a standalone observation at service time. Keep its
		// blank observed time and mutation identity unchanged for a retry.
		d.submission = &application.AppendProductValuationInput{ProductID: p.id, Amount: amount,
			MutationID: domain.NewProductOperationID().String()}
	}
	input := *d.submission
	d.problem = i18n.NoMessage
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductDetail, error) {
		return p.valuations.AppendProductValuation(p.ctx, input)
	})
}

func (p *productDetailPage) pollValuation() {
	d := p.valuation
	if d == nil {
		return
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.Contract.ID == d.submission.ProductID {
			p.valuation = nil
			p.refreshAfterProductWrite(i18n.ProductValuationSaved)
			return
		}
		// The service checks the original mutation before validation. Only
		// explicit pre-write refusals release the submission for editing.
		var issue *domain.Error
		if errors.As(result.err, &issue) {
			switch {
			case issue.Field == "amount" && (issue.Code == domain.ErrValidation || issue.Code == domain.ErrDecimalOverflow):
				d.submission, d.problem, d.focusAmount = nil, i18n.ProductValuationInvalid, true
				return
			case issue.Field == "productId" && (issue.Code == domain.ErrConflict || issue.Code == domain.ErrManagedPosition):
				d.submission, d.problem = nil, i18n.ProductValuationUnavailable
				p.read()
				return
			}
		}
		d.problem, d.focusRetry = i18n.ContributionUncertain, true
	default:
	}
}

func (p *productDetailPage) layoutValuation(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.valuation, view.Language
	if !p.valuationUnresolved() && d.cancel.Clicked(gtx) {
		p.valuation, p.focusAction = nil, &p.startValuation
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	gtx, view = view.Measure(gtx, 660)
	if d.focusAmount && gtx.Enabled() {
		d.amount.Focus(gtx)
		d.focusAmount = false
	}
	pending := d.posting != nil
	title := i18n.HoldingReconProductValuation
	rows := []layout.Widget{
		tradeTitle(view, title),
		view.Message(d.name+" · "+d.currency, view.Colors.Ink),
		fundsAmount(view, i18n.ProductValue, d.before),
		view.Message(l.Text(i18n.ProductValuationHelp), view.Colors.SecondaryInk),
		func(gtx layout.Context) layout.Dimensions {
			return d.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ProductValuationAmount, Disabled: p.valuationUnresolved()}, d.currency)
		},
	}
	if pending {
		rows = append(rows, view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk))
	} else if d.problem != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(d.problem), view.Colors.ErrorInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		saveLabel := i18n.HoldingReconConfirm
		var buttons []layout.FlexChild
		if !p.valuationUnresolved() {
			buttons = append(buttons, layout.Rigid(taskPagerButton(view, &d.cancel, i18n.InstitutionsCancel, true)))
		} else {
			saveLabel = i18n.ContributionRetryPost
		}
		canSave := !pending && d.problem != i18n.ProductValuationUnavailable &&
			(d.submission != nil || strings.TrimSpace(d.amount.Text()) != "")
		buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canSave {
				gtx = gtx.Disabled()
			}
			if d.focusRetry && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &d.save})
				d.focusRetry = false
			}
			if d.save.Clicked(gtx) && canSave {
				p.postValuation()
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, &d.save)
			return view.PrimaryButton(&d.save, l.Text(saveLabel))(gtx)
		}))
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	})
	return holdingReconColumn(gtx, rows)
}
