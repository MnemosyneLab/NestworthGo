package desktop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type productReader interface {
	availableFundsReader
	Product(context.Context, domain.ProductContractID) (application.ProductDetail, error)
}

type productRead struct {
	detail  *application.ProductDetail
	source  *domain.LiquiditySourceResult
	account string
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
		if detail.Contract.Kind != domain.ProductTermDeposit {
			return nil
		}
		out.detail = &detail
		accounts, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{detail.Contract.AccountID}, IncludeArchived: true})
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if account.Account.ID == detail.Contract.AccountID {
				out.account = account.Account.Name
				break
			}
		}
		if detail.Contract.State != domain.ProductStateOpen {
			return nil
		}
		report, err := service.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
		if err != nil {
			return err
		}
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
	ctx         context.Context
	service     productReader
	invalidate  func()
	workers     sync.WaitGroup
	id          domain.ProductContractID
	cancel      context.CancelFunc
	request     <-chan historyResult[productRead]
	current     *productRead
	err         error
	list        widget.List
	reload      widget.Clickable
	accounts    map[domain.AccountID]*widget.Clickable
	openAccount func(domain.AccountID, string, event.Tag)
}

func newProductDetailPage(ctx context.Context, service productReader, invalidate func()) *productDetailPage {
	return &productDetailPage{ctx: ctx, service: service, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, accounts: make(map[domain.AccountID]*widget.Clickable)}
}

func (p *productDetailPage) enter(id domain.ProductContractID, position layout.Position) {
	p.id, p.current, p.list.Position = id, nil, position
	p.read()
}

func (p *productDetailPage) leave() {
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
		p.leave()
		p.current, p.err = &result.value, result.err
	default:
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
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(ui.LimitWidth(gtx, 820),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, view.Message(l.Text(status), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					view.Focus.Watch(gtx, &p.reload)
					return view.Button(&p.reload, l.Text(reload))(gtx)
				}))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if status != i18n.NoMessage {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return material.List(view.Theme, &p.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, rows[i])
			})
		}))
}

func (p *productDetailPage) rows(view ui.View) []layout.Widget {
	l := view.Language
	product := p.current.detail
	contract := product.Contract
	fields := []layout.Widget{
		productState(view, product.DisplayState),
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
	return append(rows, fundsCard(view, l.Text(i18n.ProductActions), productActions(view, *product)...))
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

func productState(view ui.View, state domain.ProductDisplayState) layout.Widget {
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
	text := l.Text(i18n.ProductTermDeposit)
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
