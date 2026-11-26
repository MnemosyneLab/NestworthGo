package desktop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type productReader interface {
	availableFundsReader
	Product(context.Context, domain.ProductContractID) (application.ProductDetail, error)
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ReadProductOperationReceipt(context.Context, domain.ProductOperationID) (application.ProductOperationReceipt, error)
	ListProductOperations(context.Context, domain.ProductContractID, string, int) (application.ProductOperationPage, error)
	Activity(context.Context, domain.ActivityID) (domain.Activity, error)
}

type productRead struct {
	detail              *application.ProductDetail
	source              *domain.LiquiditySourceResult
	account             string
	accountActive       bool
	localDate, timezone string
	unpaidInterest      *domain.Money
	settlement          []domain.Activity
}

func readProduct(ctx context.Context, service productReader, id domain.ProductContractID) (out productRead, err error) {
	if _, err := domain.ParseProductContractID(id.String()); err != nil {
		return out, nil
	}
	err = service.WithRead(ctx, func(ctx context.Context) error {
		detail, err := service.Product(ctx, id)
		var issue *domain.Error
		if errors.As(err, &issue) && issue.Code == domain.ErrNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if detail.Contract.Kind != domain.ProductTermDeposit && detail.Contract.Kind != domain.ProductLockedProduct {
			return nil
		}
		out.detail = &detail
		accounts, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{detail.Contract.AccountID}, IncludeArchived: true})
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if account.Account.ID == detail.Contract.AccountID {
				out.account, out.accountActive = account.Account.Name, account.Account.ArchivedAt == nil
				break
			}
		}
		if detail.Contract.State == domain.ProductStateSettled && detail.Contract.ClosedOperationID != nil {
			receipt, err := service.ReadProductOperationReceipt(ctx, *detail.Contract.ClosedOperationID)
			if err != nil {
				return err
			}
			if receipt.Kind == domain.ProductOpSettle {
				for _, id := range receipt.ActivityIDs {
					activity, err := service.Activity(ctx, id)
					if err != nil {
						return err
					}
					if link := activity.ProductContext; link != nil && link.ProductID == detail.Contract.ID &&
						link.OperationID == receipt.OperationID {
						out.settlement = append(out.settlement, activity)
					}
				}
			}
		}
		if detail.Contract.State != domain.ProductStateOpen {
			return nil
		}
		out.unpaidInterest, err = detail.Contract.EstimatedUnpaidMaturityInterest()
		if err != nil {
			return err
		}
		report, err := service.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
		if err != nil {
			return err
		}
		out.localDate, out.timezone = report.Overview.LocalDate, report.Overview.Timezone
		for _, row := range report.Sources {
			if row.Result.ProductID != nil && *row.Result.ProductID == id {
				out.source = &row.Result
				break
			}
		}
		return nil
	})
	return out, err
}

type productDetailPage struct {
	ctx                  context.Context
	service              productReader
	invalidate           func()
	workers              sync.WaitGroup
	id                   domain.ProductContractID
	cancel               context.CancelFunc
	request              <-chan historyResult[productRead]
	current              *productRead
	err                  error
	history              productHistory
	list                 widget.List
	reload               widget.Clickable
	accounts             map[domain.AccountID]*widget.Clickable
	openAccount          func(domain.AccountID, string, event.Tag)
	openActivity         func(domain.ActivityID, event.Tag)
	activities           map[domain.ActivityID]*widget.Clickable
	receipts             productReceiptLedger
	receipt              *productReceiptDraft
	valuations           productValuationLedger
	valuation            *productValuationDraft
	termsLedger          productTermsLedger
	terms                *productTermsDraft
	saved                i18n.MessageID
	focusAction          *widget.Clickable
	receiptRequested     bool
	receiptChanged       bool
	startConfirmation    widget.Clickable
	startInterestReceipt widget.Clickable
	startGroupedUndo     widget.Clickable
	startTermsEdit       widget.Clickable
	startValuation       widget.Clickable
	clock                func() time.Time
	onProductSaved       func()
}

func newProductDetailPage(ctx context.Context, service productReader, invalidate func()) *productDetailPage {
	return &productDetailPage{ctx: ctx, service: service, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, accounts: make(map[domain.AccountID]*widget.Clickable), activities: make(map[domain.ActivityID]*widget.Clickable)}
}

func (p *productDetailPage) enter(id domain.ProductContractID, position layout.Position, verifyReceipt bool) {
	p.history.stop()
	p.history = productHistory{}
	p.id, p.current, p.list.Position = id, nil, position
	p.receipt, p.valuation, p.terms, p.saved, p.focusAction = nil, nil, nil, i18n.NoMessage, nil
	p.receiptRequested, p.receiptChanged = verifyReceipt, false
	p.read()
}

func (p *productDetailPage) unresolved() bool {
	return p != nil && (p.receiptUnresolved() || p.valuationUnresolved() || p.terms != nil && p.terms.posting != nil)
}

func (p *productDetailPage) refreshAfterProductWrite(message i18n.MessageID) {
	p.saved = message
	p.list.Position = layout.Position{}
	if p.onProductSaved != nil {
		p.onProductSaved()
	} else {
		p.read()
	}
}

func (p *productDetailPage) leave() {
	p.finishRead()
	p.history.stop()
}

func (p *productDetailPage) finishRead() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *productDetailPage) read() {
	p.leave()
	p.err = nil
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	id := p.id
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (productRead, error) {
		return readProduct(ctx, p.service, id)
	})
}

func (p *productDetailPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	select {
	case result := <-p.request:
		p.finishRead()
		p.current, p.err = &result.value, result.err
		if p.err == nil && p.current.detail != nil {
			p.loadProductHistory("")
		}
		if p.receipt != nil && p.receipt.received != nil && p.current.detail != nil {
			p.receipt.received.receive(p.current.detail.Contract)
		}
		if p.receiptRequested && p.err == nil {
			// This is a one-shot navigation intent. Only a freshly read,
			// eligible contract can open the draft, and opening it never posts.
			p.receiptRequested = false
			if p.receipts != nil && p.current.canConfirmReceipt() {
				p.startReceipt(true)
			} else if p.current.detail != nil {
				p.receiptChanged = true
			}
		}
	default:
	}
	p.pollProductHistory()
	p.pollReceipt()
	p.pollValuation()
	p.pollTerms()
	if p.receipt != nil {
		return p.layoutReceipt(gtx, view)
	}
	if p.valuation != nil {
		return p.layoutValuation(gtx, view)
	}
	if p.terms != nil {
		return p.layoutTerms(gtx, view)
	}
	if p.request == nil && p.reload.Clicked(gtx) {
		p.read()
	}
	l := view.Language
	var status i18n.MessageID
	reload := i18n.HistoryRefresh
	switch {
	case p.request != nil:
		status = i18n.ProductLoading
	case p.err != nil:
		status, reload = i18n.ProductReadFailed, i18n.HistoryRetry
	case p.current == nil || p.current.detail == nil:
		status = i18n.ProductUnavailable
	}
	return ui.Column{Gap: 10}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, view.Message(l.Text(status), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					} else if p.focusAction != nil && (status != i18n.NoMessage || !p.actionAvailable(p.focusAction)) {
						gtx.Execute(key.FocusCmd{Tag: &p.reload})
						p.focusAction = nil
					}
					view.Focus.Watch(gtx, &p.reload)
					return view.Button(&p.reload, l.Text(reload))(gtx)
				}))
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.saved != i18n.NoMessage {
				return view.Message(l.Text(p.saved), view.Colors.Ink)(gtx)
			}
			if p.receiptChanged {
				return view.Message(l.Text(i18n.ProductReceiptChanged), view.Colors.SecondaryInk)(gtx)
			}
			return layout.Dimensions{}
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if status != i18n.NoMessage {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return view.List(&p.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, rows[i])
			})
		}))
}

func (p *productDetailPage) rows(view ui.View) []layout.Widget {
	l := view.Language
	product := p.current.detail
	contract := product.Contract
	fields := []layout.Widget{
		productState(view, contract.Kind, product.DisplayState),
		fundsAmount(view, i18n.ProductPrincipal, &contract.Principal),
		fundsAmount(view, i18n.ProductValue, product.CurrentValue),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductStart), historyDay(view, contract.StartOn)), view.Colors.Ink),
		productMaturity(view, contract.MaturityOn),
	}
	if name := p.current.account; name != "" && p.openAccount != nil {
		button := portfolioButton(p.accounts, contract.AccountID)
		fields = append(fields, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				p.openAccount(contract.AccountID, name, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, button)
			return layout.W.Layout(gtx, view.Button(button, fmt.Sprintf(l.Text(i18n.HistoryOpenAccount), name)))
		})
	} else {
		fields = append(fields, view.Message(l.Text(i18n.AccountReferenceUnavailable), view.Colors.SecondaryInk))
	}
	rows := []layout.Widget{fundsCard(view, productName(*product), fields...)}
	if contract.State == domain.ProductStateOpen {
		var estimated *domain.Money
		var receipt string
		var missing []string
		if source := p.current.source; source != nil {
			// A source without a normal route can still explain why it could
			// not be evaluated. For an evaluated route use its own reasons.
			missing = source.Reasons
			if route := source.NormalRoute; route != nil {
				estimated = route.NetNative
				if route.ReceiptOn != nil {
					receipt = historyDay(view, *route.ReceiptOn)
				}
				missing = route.MissingReasons
			}
		}
		if receipt == "" {
			receipt = l.Text(i18n.OverviewNoValue)
		}
		fields := []layout.Widget{
			fundsAmount(view, i18n.ProductForecastInterest, p.current.unpaidInterest),
			fundsAmount(view, i18n.ProductForecastNet, estimated),
			view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptDate), receipt), view.Colors.Ink),
		}
		var reasons []i18n.MessageID
		for _, reason := range missing {
			label := fundsReason(reason)
			if !slices.Contains(reasons, label) {
				reasons = append(reasons, label)
				fields = append(fields, view.Message(l.Text(label), view.Colors.SecondaryInk))
			}
		}
		rows = append(rows, fundsCard(view, l.Text(i18n.ProductForecast), fields...))
	}
	if len(p.current.settlement) > 0 {
		var links []layout.Widget
		for _, activity := range p.current.settlement {
			if p.openActivity == nil {
				break
			}
			label := i18n.ProductReceiptHistory
			if contract.Kind == domain.ProductLockedProduct {
				label = i18n.ProductRedemptionHistory
			} else if activity.ProductContext.Purpose == domain.ProductPurposeInterest {
				label = i18n.ProductInterestHistory
			}
			button := portfolioButton(p.activities, activity.ID)
			links = append(links, func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					p.openActivity(activity.ID, button)
					gtx.Execute(op.InvalidateCmd{})
				}
				view.Focus.Watch(gtx, button)
				return layout.W.Layout(gtx, view.Button(button, l.Text(label)))
			})
		}
		if len(links) != 0 {
			rows = append(rows, fundsCard(view, l.Text(i18n.ProductReceiptRecorded), links...))
		}
	}
	if id := p.history.interest; id != "" && p.openActivity != nil {
		button := portfolioButton(p.activities, id)
		rows = append(rows, fundsCard(view, l.Text(i18n.ProductStandaloneInterestHistory), func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				p.openActivity(id, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, button)
			return layout.W.Layout(gtx, view.Button(button, l.Text(i18n.ProductInterestHistory)))
		}))
	}
	canRecord := p.receipts != nil && p.current.canRecordReceipt()
	actions := productActions(view, *product)
	if canRecord {
		actions[0] = p.lifecycleAction(view, &p.startConfirmation, i18n.ProductReceiptAction, true, func() { p.startReceipt(false) })
	}
	if p.receipts != nil && p.current.canRecordInterest() {
		actions[1] = p.lifecycleAction(view, &p.startInterestReceipt, i18n.HoldingReconProductInterest, false, p.startInterest)
	}
	if p.valuations != nil && canUpdateProductValuation(*product) {
		actions = append(actions, p.lifecycleAction(view, &p.startValuation, i18n.HoldingReconProductValuation, false, p.beginValuation))
	}
	if p.termsLedger != nil {
		actions = append([]layout.Widget{p.lifecycleAction(view, &p.startTermsEdit, i18n.ProductEditTerms, false, p.startTerms)}, actions...)
	}
	if p.canStartGroupedUndo() {
		actions = append(actions, p.lifecycleAction(view, &p.startGroupedUndo, i18n.ProductGroupedUndo, false, p.startUndo))
	}
	rows = append(rows, fundsCard(view, l.Text(i18n.ProductActions), actions...))
	return append(rows, p.productHistoryRows(view)...)
}

func (p *productDetailPage) actionAvailable(button *widget.Clickable) bool {
	switch button {
	case &p.startConfirmation:
		return p.current.canRecordReceipt()
	case &p.startInterestReceipt:
		return p.current.canRecordInterest()
	case &p.startGroupedUndo:
		return p.canStartGroupedUndo()
	case &p.startValuation:
		return canUpdateProductValuation(*p.current.detail)
	default:
		return true
	}
}

func (p *productDetailPage) lifecycleAction(view ui.View, button *widget.Clickable, label i18n.MessageID, primary bool, begin func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.focusAction == button && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: button})
			p.focusAction = nil
		}
		if button.Clicked(gtx) {
			begin()
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		title := view.Language.Text(label)
		if primary {
			return layout.W.Layout(gtx, view.PrimaryButton(button, title))
		}
		return layout.W.Layout(gtx, view.Button(button, title))
	}
}

func productActions(view ui.View, product application.ProductDetail) []layout.Widget {
	l := view.Language
	settle, interest := i18n.ProductNotPermitted, i18n.ProductNotPermitted
	for _, action := range product.PermittedActions {
		switch action {
		case "settle":
			settle = i18n.ProductCanReview
		case "receive_interest":
			interest = i18n.ProductCanReview
		}
	}
	for action, reason := range product.DisabledReasons {
		if action == "settle" && reason == "the contract is not open" {
			settle = i18n.ProductContractClosed
		}
	}
	return []layout.Widget{
		view.Message(fmt.Sprintf(l.Text(i18n.ProductConfirmReceipt), l.Text(settle)), view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiveInterest), l.Text(interest)), view.Colors.Ink),
	}
}

func productName(product application.ProductDetail) string {
	return domain.InstrumentDisplayLabel(product.Contract.Name, product.Symbol, product.Contract.ShortName)
}

func productState(view ui.View, kind domain.ProductKind, state domain.ProductDisplayState) layout.Widget {
	l := view.Language
	var label i18n.MessageID
	switch state {
	case domain.ProductDisplayLocked:
		label = i18n.ProductLocked
	case domain.ProductDisplayRedeemable:
		label = i18n.ProductRedeemable
	case domain.ProductDisplayDueUnconfirmed:
		label = i18n.FundsDue
	case domain.ProductDisplayNeedsInfo:
		label = i18n.ProductNeedsInfo
	case domain.ProductDisplaySettled:
		label = i18n.ProductSettled
	case domain.ProductDisplayCancelled:
		label = i18n.ProductCancelled
	}
	name := i18n.ProductTermDeposit
	if kind == domain.ProductLockedProduct {
		name = i18n.ProductAddLocked
	}
	text := l.Text(name)
	if label != i18n.NoMessage {
		text += " · " + l.Text(label)
	}
	return view.Message(text, view.Colors.SecondaryInk)
}

func productMaturity(view ui.View, maturity *string) layout.Widget {
	value := view.Language.Text(i18n.OverviewNoValue)
	if maturity != nil {
		value = historyDay(view, *maturity)
	}
	return view.Message(fmt.Sprintf(view.Language.Text(i18n.ProductMaturity), value), view.Colors.Ink)
}

func productLink(view ui.View, id domain.ProductContractID, name string, button *widget.Clickable, open func(domain.ProductContractID, string, event.Tag)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			open(id, name, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return layout.W.Layout(gtx, view.Button(button, fmt.Sprintf(view.Language.Text(i18n.ProductOpen), name)))
	}
}
