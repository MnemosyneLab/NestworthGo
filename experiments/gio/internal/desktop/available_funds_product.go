package desktop

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
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

type productSetupRuntime struct {
	ctx        context.Context
	ledger     fundsProductLedger
	clock      func() time.Time
	workers    *sync.WaitGroup
	invalidate func()
}

type fundsProductSetup struct {
	runtime          productSetupRuntime
	account          domain.Account
	choices          []domain.Account
	accountButtons   map[domain.AccountID]*widget.Clickable
	form             *holdingProductAdd
	sheet            analysisSheet
	overlay          ui.Overlay
	currencyFocus    bool
	requiredField    i18n.MessageID
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

func newProductSetup(runtime productSetupRuntime, accounts []domain.Account, defaultName string) *fundsProductSetup {
	sheet := newAnalysisSheet()
	sheet.modal, sheet.focus = true, false
	d := &fundsProductSetup{runtime: runtime, choices: accounts, focus: true, sheet: sheet, accountButtons: make(map[domain.AccountID]*widget.Clickable)}
	if len(accounts) == 1 {
		d.chooseAccount(accounts[0], defaultName)
	}
	return d
}

func (d *fundsProductSetup) chooseAccount(account domain.Account, defaultName string) {
	d.account, d.currency, d.form = account, account.DefaultCurrency.String(), newHoldingProductAdd(defaultName)
	d.focus, d.currencyFocus = true, true
	d.readCash()
}

func (d *fundsProductSetup) readCash() {
	run, accountID := d.runtime, d.account.ID
	d.cash, d.cashRead = nil, runHistoryRead(run.workers, run.invalidate, func() ([]domain.AccountValuation, error) {
		return run.ledger.AccountValuations(run.ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{accountID}})
	})
}

func (d *fundsProductSetup) cashValue(view ui.View) string {
	amount, ok := d.cashAmount()
	if !ok {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return holdingSnapshotValue(view, amount, d.currency)
}

func (d *fundsProductSetup) cashAmount() (string, bool) {
	if d.cash == nil {
		return "", false
	}
	for _, component := range d.cash.Components {
		if component.HoldingID == nil && component.NativeCurrency.String() == d.currency {
			return component.NativeAmount, component.NativeAmount != ""
		}
	}
	return "0", true
}

func (d *fundsProductSetup) requestReview() {
	run := d.runtime
	if d.preview != nil {
		return
	}
	command, valid := d.form.command(d.account.ID.String(), d.currency)
	if !valid {
		d.problem = i18n.ProductEditValidDays
		return
	}
	d.problem = i18n.NoMessage
	d.preview = runHistoryRead(run.workers, run.invalidate, func() (productReceiptPlan, error) {
		if open := command.Open; open != nil && open.EffectiveLocalDate == "" && open.EffectiveLocalTime == "" {
			// Opening supports an exact instant. Keep the reviewed instant on retries.
			// Record existing instead uses the service's recording time.
			open.EffectiveAt = run.clock().UTC().Format(time.RFC3339Nano)
		}
		review, err := run.ledger.PreviewProductOperation(run.ctx, command)
		return productReceiptPlan{command: command, review: review, mutation: domain.NewProductOperationID()}, err
	})
}

func (d *fundsProductSetup) confirmReview() {
	run := d.runtime
	if d.post != nil || d.plan == nil {
		return
	}
	plan := *d.plan
	d.post = runHistoryRead(run.workers, run.invalidate, func() (application.ProductOperationReceipt, error) {
		return run.ledger.RecordProductOperation(run.ctx, plan.command, plan.mutation.String(), plan.review.ReviewedStateHash)
	})
}

func (d *fundsProductSetup) unresolved() bool {
	return d != nil && (d.post != nil || d.uncertain)
}

func (d *fundsProductSetup) poll() bool {
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
			return true
		}
		if problem := productSetupProblem(result.err); problem != i18n.ProductAddFailed {
			// The shared service checks for an existing receipt before business validation.
			d.plan, d.uncertain, d.problem, d.focus = nil, false, problem, true
			d.readCash()
		} else {
			d.uncertain, d.focus = true, true
		}
	default:
	}
	return false
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
	case domain.ErrMissingHistoryOrigin:
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
