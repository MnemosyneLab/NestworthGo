package desktop

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fundsProductLedger interface {
	AccountValuations(context.Context, domain.AccountFilter) ([]domain.AccountValuation, error)
	PreviewProductOperation(context.Context, application.ProductCommand) (application.ProductOperationPreview, error)
	RecordProductOperation(context.Context, application.ProductCommand, string, string) (application.ProductOperationReceipt, error)
}

type fundsProductSetup struct {
	account          domain.Account
	choices          []domain.Account
	accountButtons   map[domain.AccountID]*widget.Clickable
	form             *holdingProductAdd
	currency         string
	cash             *domain.AccountValuation
	cashRead         <-chan historyResult[[]domain.AccountValuation]
	preview          <-chan historyResult[productReceiptPlan]
	post             <-chan historyResult[application.ProductOperationReceipt]
	plan             *productReceiptPlan
	problem          i18n.MessageID
	uncertain, focus bool
	cancel, review   widget.Clickable
	edit, confirm    widget.Clickable
}

func (p *availableFundsPage) productAccounts() []domain.Account {
	if p.current == nil || p.productLedger == nil {
		return nil
	}
	var eligible []domain.Account
	for _, account := range p.current.accounts {
		if domain.ProductAccountEligible(account) == nil {
			eligible = append(eligible, account)
		}
	}
	slices.SortFunc(eligible, func(a, b domain.Account) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return eligible
}

func (p *availableFundsPage) startProductSetup(accounts []domain.Account, defaultName string) {
	p.productSaved = false
	p.setup = &fundsProductSetup{choices: accounts, focus: true, accountButtons: make(map[domain.AccountID]*widget.Clickable)}
	if len(accounts) == 1 {
		p.chooseProductAccount(accounts[0], defaultName)
	}
}

func (p *availableFundsPage) chooseProductAccount(account domain.Account, defaultName string) {
	d := p.setup
	d.account, d.currency, d.form = account, account.DefaultCurrency.String(), newHoldingProductAdd(defaultName)
	d.focus = true
	p.readProductCash()
}

func (p *availableFundsPage) readProductCash() {
	d := p.setup
	accountID := d.account.ID
	d.cash, d.cashRead = nil, runHistoryRead(&p.workers, p.invalidate, func() ([]domain.AccountValuation, error) {
		return p.productLedger.AccountValuations(p.ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{accountID}})
	})
}

func (d *fundsProductSetup) cashValue(view ui.View) string {
	if d.cash == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	for _, component := range d.cash.Components {
		if component.HoldingID == nil && component.NativeCurrency.String() == d.currency {
			if component.NativeAmount != "" {
				return holdingSnapshotValue(view, component.NativeAmount, d.currency)
			}
			return view.Language.Text(i18n.OverviewNoValue)
		}
	}
	return holdingSnapshotValue(view, "0", d.currency)
}

func (p *availableFundsPage) reviewProductSetup() {
	d := p.setup
	if d.preview != nil {
		return
	}
	command, valid := d.form.command(d.account.ID.String(), d.currency)
	if !valid {
		d.problem = i18n.ProductEditValidDays
		return
	}
	d.problem = i18n.NoMessage
	d.preview = runHistoryRead(&p.workers, p.invalidate, func() (productReceiptPlan, error) {
		if open := command.Open; open != nil && open.EffectiveLocalDate == "" && open.EffectiveLocalTime == "" {
			// Opening supports an exact instant. Keep the reviewed instant on retries.
			// Record existing instead uses the service's recording time.
			open.EffectiveAt = p.productClock().UTC().Format(time.RFC3339Nano)
		}
		review, err := p.productLedger.PreviewProductOperation(p.ctx, command)
		return productReceiptPlan{command: command, review: review, mutation: domain.NewProductOperationID()}, err
	})
}

func (p *availableFundsPage) postProductSetup() {
	d := p.setup
	if d.post != nil || d.plan == nil {
		return
	}
	plan := *d.plan
	d.post = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductOperationReceipt, error) {
		return p.productLedger.RecordProductOperation(p.ctx, plan.command, plan.mutation.String(), plan.review.ReviewedStateHash)
	})
}

func (p *availableFundsPage) productSetupUnresolved() bool {
	d := p.setup
	return d != nil && (d.post != nil || d.uncertain)
}

func (p *availableFundsPage) pollProductSetup() {
	d := p.setup
	select {
	case result := <-d.cashRead:
		d.cashRead = nil
		if result.err == nil {
			if at := slices.IndexFunc(result.value, func(row domain.AccountValuation) bool { return row.Account.ID == d.account.ID }); at >= 0 {
				d.cash = &result.value[at]
			}
		}
	default:
	}
	select {
	case result := <-d.preview:
		d.preview = nil
		if result.err != nil {
			d.problem = productSetupProblem(result.err)
		} else {
			d.plan, d.focus = &result.value, true
		}
	default:
	}
	select {
	case result := <-d.post:
		d.post = nil
		if result.err == nil && result.value.OperationID == d.plan.mutation && result.value.Kind == d.plan.command.Kind &&
			len(result.value.ProductIDs) == 1 && len(result.value.ActivityIDs) != 0 {
			p.setup, p.productSaved, p.restoreAddFocus = nil, true, true
			p.read()
			return
		}
		if problem := productSetupProblem(result.err); problem != i18n.ProductAddFailed {
			// The shared service checks for an existing receipt before business validation.
			d.plan, d.uncertain, d.problem, d.focus = nil, false, problem, true
			p.readProductCash()
		} else {
			d.uncertain, d.focus = true, true
		}
	default:
	}
}

func productSetupProblem(err error) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return i18n.ProductAddFailed
	}
	switch issue.Code {
	case domain.ErrStalePreview, domain.ErrRevisionConflict:
		return i18n.HistoryAmendmentStale
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	case domain.ErrHistoryNotStarted:
		return i18n.CashMovementHistoryRequired
	case domain.ErrInvalidChangeTime, domain.ErrCashBalanceTimeConflict:
		return i18n.ProductReceiptTimeInvalid
	case domain.ErrValidation, domain.ErrDecimalOverflow:
		if issue.Field == "cashExcludesProduct" {
			return i18n.ProductAddConfirmCash
		}
		if issue.Field == "accountId" {
			return i18n.AccountUnavailable
		}
		return i18n.ProductSetupInvalid
	case domain.ErrConflict, domain.ErrNotFound:
		if issue.Field == "accountId" {
			return i18n.AccountUnavailable
		}
	}
	return i18n.ProductAddFailed
}
