package desktop

import (
	"context"
	"errors"
	"strings"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type cashBalanceLedger interface {
	WithRead(context.Context, func(context.Context) error) error
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccountCashValues(context.Context, domain.AccountID) ([]domain.AccountCashValue, error)
	PreviewCashBalance(context.Context, domain.AccountID, string, string, *string) (*application.CashBalancePlan, error)
	ConfirmCashBalance(context.Context, *application.CashBalancePlan) (domain.AccountCashValue, error)
}

type cashBalanceSources struct {
	current map[domain.CurrencyCode]domain.Money
	history bool
}

type cashBalanceReview struct {
	plan    *application.CashBalancePlan
	history bool
}

type cashBalanceDraft struct {
	account                                             domain.Account
	currency                                            domain.CurrencyCode
	amount, reason                                      ui.TextField
	picker                                              ui.Select
	sources                                             *cashBalanceSources
	reading                                             <-chan historyResult[cashBalanceSources]
	previewing                                          <-chan historyResult[cashBalanceReview]
	confirming                                          <-chan historyResult[domain.AccountCashValue]
	review                                              *cashBalanceReview
	problem                                             i18n.MessageID
	readFailed, uncertain                               bool
	focus                                               bool
	cancel, edit, prepare, confirm, retry, retryConfirm widget.Clickable
}

type accountCashBalance struct {
	service      cashBalanceLedger
	draft        *cashBalanceDraft
	saved        domain.AccountCashValue
	open, view   widget.Clickable
	openActivity func(domain.ActivityID, event.Tag)
}

func (p *accountsPage) canSetCashBalance() bool {
	row := p.freshDetail()
	return p.cashBalance != nil && row != nil && row.record.Account.ArchivedAt == nil &&
		row.record.Account.TrackingMode == domain.TrackingHoldings
}

func (p *accountsPage) startCashBalance() {
	a := p.freshDetail().record.Account
	p.cashBalance.draft = &cashBalanceDraft{account: a, currency: a.DefaultCurrency,
		amount: ui.NewTextField(true), reason: ui.NewTextField(true), focus: true}
	p.loadCashBalanceSources()
}

func (p *accountsPage) loadCashBalanceSources() {
	feature := p.cashBalance
	d := feature.draft
	id := d.account.ID
	d.readFailed = false
	d.reading = runHistoryRead(&p.workers, p.invalidate, func() (cashBalanceSources, error) {
		var source cashBalanceSources
		err := feature.service.WithRead(p.ctx, func(ctx context.Context) error {
			origin, err := feature.service.HistoryOrigin(ctx)
			if err != nil {
				return err
			}
			values, err := feature.service.ListAccountCashValues(ctx, id)
			if err != nil {
				return err
			}
			source = cashBalanceSources{current: make(map[domain.CurrencyCode]domain.Money), history: origin != nil}
			// The shared cash history returns effective/creation order, newest first.
			for _, value := range values {
				code := value.Amount.Currency()
				if _, exists := source.current[code]; !exists {
					source.current[code] = value.Amount
				}
			}
			return nil
		})
		return source, err
	})
}

func (p *accountsPage) previewCashBalance() {
	f, d := p.cashBalance, p.cashBalance.draft
	id, amount, currency := d.account.ID, strings.TrimSpace(d.amount.Text()), d.currency.String()
	var reason *string
	if note := d.reason.Text(); d.sources.history && strings.TrimSpace(note) != "" {
		reason = &note
	}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (cashBalanceReview, error) {
		var review cashBalanceReview
		err := f.service.WithRead(p.ctx, func(ctx context.Context) error {
			origin, err := f.service.HistoryOrigin(ctx)
			if err != nil {
				return err
			}
			plan, err := f.service.PreviewCashBalance(ctx, id, amount, currency, reason)
			review = cashBalanceReview{plan: plan, history: origin != nil}
			return err
		})
		return review, err
	})
}

func (p *accountsPage) confirmCashBalance() {
	f, d := p.cashBalance, p.cashBalance.draft
	plan := d.review.plan
	d.confirming = runHistoryRead(&p.workers, p.invalidate, func() (domain.AccountCashValue, error) {
		// The operation's existing immutable observation/mutation identity is
		// retained on every retry, including after a lost reply.
		return f.service.ConfirmCashBalance(p.ctx, plan)
	})
}

func (p *accountsPage) pollCashBalance() domain.AccountCashValue {
	f := p.cashBalance
	if f == nil || f.draft == nil {
		return domain.AccountCashValue{}
	}
	d := f.draft
	select {
	case result := <-d.reading:
		d.reading, d.readFailed = nil, result.err != nil
		if !d.readFailed {
			d.sources = &result.value
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = cashBalanceProblem(result.err, i18n.AccountCashPreviewFailed)
		} else {
			d.review = &result.value
		}
	default:
	}
	select {
	case result := <-d.confirming:
		d.confirming = nil
		if result.value.ID != "" {
			f.saved, f.draft = result.value, nil
			p.leave()
			p.read()
			return result.value
		}
		d.problem = cashBalanceProblem(result.err, i18n.NoMessage)
		if d.problem == i18n.NoMessage {
			d.uncertain = true
		} else {
			d.review, d.uncertain, d.focus = nil, false, true
			p.loadCashBalanceSources()
		}
	default:
	}
	return domain.AccountCashValue{}
}

func (p *accountsPage) cashBalanceUnresolved() bool {
	f := p.cashBalance
	return f != nil && f.draft != nil && (f.draft.confirming != nil || f.draft.uncertain)
}

func cashBalanceProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrNoChange:
		return i18n.AccountCashNoChange
	case domain.ErrStalePreview:
		if issue.Field == "previewToken" {
			return i18n.HistoryAmendmentStale
		}
	case domain.ErrCashBalanceTimeConflict:
		return i18n.AccountCashTimeConflict
	case domain.ErrNotFound, domain.ErrConflict:
		return i18n.AccountUnavailable
	case domain.ErrValidation:
		if issue.Field == "amount" {
			return i18n.AccountCashInvalidTarget
		}
		return i18n.AccountCashPreviewFailed
	case domain.ErrInvalidChange, domain.ErrInvalidChangeTime, domain.ErrInsufficientBalance:
		return i18n.AccountCashPreviewFailed
	}
	return fallback
}
