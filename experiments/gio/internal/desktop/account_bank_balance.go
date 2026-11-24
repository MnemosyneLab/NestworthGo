package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type bankBalanceLedger interface {
	WithRead(context.Context, func(context.Context) error) error
	WithWrite(context.Context, func(context.Context) error) error
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	AppendAccountValue(context.Context, domain.AccountID, string, string) (domain.AccountValue, error)
	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
}

type bankBalancePlan struct {
	account                      domain.AccountID
	observation                  domain.AccountValueID
	currency                     domain.CurrencyCode
	before, target, delta        string
	at                           time.Time
	zone                         string
	command                      any
	mutation, payloadHash, token string
}

type bankBalanceReceipt struct {
	account     domain.AccountID
	observation domain.AccountValueID
	activity    domain.ActivityID
}

func (r bankBalanceReceipt) known() bool { return r.observation != "" || r.activity != "" }

type bankBalanceDraft struct {
	account                                      domain.Account
	amount                                       ui.TextField
	focus, uncertain                             bool
	problem                                      i18n.MessageID
	review                                       *bankBalancePlan
	previewing                                   <-chan historyResult[bankBalancePlan]
	confirming                                   <-chan historyResult[bankBalanceReceipt]
	prepare, confirm, retryConfirm, edit, cancel widget.Clickable
}

type accountBankBalance struct {
	service      bankBalanceLedger
	draft        *bankBalanceDraft
	saved        bankBalanceReceipt
	unverified   domain.AccountID
	open, view   widget.Clickable
	openActivity func(domain.ActivityID, event.Tag)
}

func supportsBankBalance(row domain.AccountRecord) bool {
	a := row.Account
	return a.ArchivedAt == nil && a.AccountType == domain.TypeBankAccount && a.BalanceSheetRole == domain.RoleAsset &&
		a.TrackingMode == domain.TrackingBalance && row.LatestValue != nil
}

func (p *accountsPage) canUpdateBankBalance() bool {
	row := p.freshDetail()
	return p.bankBalance != nil && row != nil && supportsBankBalance(row.record)
}

func (p *accountsPage) startBankBalance() {
	row := p.freshDetail().record
	d := &bankBalanceDraft{account: row.Account, amount: ui.NewTextField(true), focus: true}
	d.amount.Editor.SetText(row.LatestValue.Amount.CanonicalAmount())
	p.bankBalance.draft = d
}

func readBankBalance(ctx context.Context, service bankBalanceLedger, id domain.AccountID) (domain.AccountRecord, error) {
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if len(rows) != 1 || rows[0].Account.ID != id || !supportsBankBalance(rows[0]) {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrNotFound}
	}
	return rows[0], nil
}

func reviewBankBalance(ctx context.Context, service bankBalanceLedger, id domain.AccountID, raw string, clock func() time.Time) (bankBalancePlan, error) {
	var plan bankBalancePlan
	err := service.WithRead(ctx, func(ctx context.Context) error {
		row, err := readBankBalance(ctx, service, id)
		if err != nil {
			return err
		}
		money, err := domain.ParseMoney(strings.TrimSpace(raw), row.Account.DefaultCurrency)
		if err != nil {
			return err
		}
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		plan = bankBalancePlan{account: id, observation: row.LatestValue.ID, currency: money.Currency(),
			before: row.LatestValue.Amount.CanonicalAmount(), target: money.CanonicalAmount()}
		if origin == nil {
			if plan.before == plan.target {
				return &domain.Error{Code: domain.ErrNoChange}
			}
			return nil
		}
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeValueUpdate, AccountID: id.String(), NewValue: plan.target, NewValueCurrency: money.Currency().String(),
			Reason: string(domain.ReasonReconciliation), EffectiveAt: clock().UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano),
			MutationID: domain.NewMutationID().String(),
		}
		plan.command, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
		if err != nil {
			return err
		}
		plan.mutation, plan.payloadHash, err = request.MutationEnvelope()
		if err != nil {
			return err
		}
		preview, token, err := service.PreviewChangeGuardedWith(ctx, func(context.Context) (any, error) { return plan.command, nil })
		if err != nil {
			return err
		}
		if len(preview.Review) != 1 || preview.Review[0].Target != domain.EffectTargetAccountValue {
			return &domain.Error{Code: domain.ErrIntegrity}
		}
		line := preview.Review[0]
		plan.before, plan.target, plan.delta = line.Before, line.After, line.Delta
		plan.at, plan.zone, plan.token = preview.Activity.EffectiveAt, origin.Timezone, token
		return nil
	})
	return plan, err
}

func (plan bankBalancePlan) commit(ctx context.Context, service bankBalanceLedger) (bankBalanceReceipt, error) {
	receipt := bankBalanceReceipt{account: plan.account}
	if plan.command != nil {
		result, err := service.RecordChangeGuarded(ctx, plan.command, plan.mutation, plan.payloadHash, plan.token)
		receipt.activity = result.Activity.ID
		return receipt, err
	}
	err := service.WithWrite(ctx, func(ctx context.Context) error {
		// The legacy append has no mutation receipt. Keep its baseline-only
		// decision and the reviewed observation under the same write permit.
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		row, err := readBankBalance(ctx, service, plan.account)
		if err != nil {
			return err
		}
		if origin != nil || row.LatestValue.ID != plan.observation || row.Account.DefaultCurrency != plan.currency {
			return &domain.Error{Code: domain.ErrStalePreview}
		}
		result, err := service.AppendAccountValue(ctx, plan.account, plan.target, "")
		receipt.observation = result.ID
		return err
	})
	return receipt, err
}

func (p *accountsPage) previewBankBalance() {
	f, d := p.bankBalance, p.bankBalance.draft
	id, raw := d.account.ID, d.amount.Text()
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (bankBalancePlan, error) {
		return reviewBankBalance(p.ctx, f.service, id, raw, p.clock)
	})
}

func (p *accountsPage) confirmBankBalance() {
	f, d := p.bankBalance, p.bankBalance.draft
	plan := *d.review
	f.unverified = ""
	d.confirming = runHistoryRead(&p.workers, p.invalidate, func() (bankBalanceReceipt, error) {
		return plan.commit(p.ctx, f.service)
	})
}

func (p *accountsPage) pollBankBalance() bankBalanceReceipt {
	f := p.bankBalance
	if f == nil || f.draft == nil {
		return bankBalanceReceipt{}
	}
	d := f.draft
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = bankBalanceProblem(result.err, i18n.AccountBankBalancePreviewFailed)
		} else {
			d.review = &result.value
		}
	default:
	}
	select {
	case result := <-d.confirming:
		d.confirming = nil
		if result.value.known() {
			f.saved, f.draft = result.value, nil
			p.leave()
			p.read()
			return result.value
		}
		d.problem = bankBalanceProblem(result.err, i18n.NoMessage)
		switch {
		case d.problem != i18n.NoMessage:
			d.review, d.uncertain, d.focus = nil, false, true
		case d.review.command != nil:
			d.uncertain = true
		default:
			// An unkeyed append may already have committed. Return to the
			// account and read its latest observation without reissuing it.
			f.unverified, f.saved, f.draft = d.account.ID, bankBalanceReceipt{}, nil
			p.leave()
			p.read()
			return bankBalanceReceipt{account: d.account.ID}
		}
	default:
	}
	return bankBalanceReceipt{}
}

func (p *accountsPage) bankBalanceUnresolved() bool {
	f := p.bankBalance
	return f != nil && f.draft != nil && (f.draft.confirming != nil || f.draft.uncertain)
}

func bankBalanceProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrNoChange:
		return i18n.AccountBankBalanceNoChange
	case domain.ErrStalePreview:
		return i18n.HistoryAmendmentStale
	case domain.ErrNotFound, domain.ErrConflict:
		return i18n.AccountUnavailable
	case domain.ErrValidation:
		if issue.Field == "amount" {
			return i18n.AccountCashInvalidTarget
		}
		return i18n.AccountBankBalancePreviewFailed
	case domain.ErrInvalidChange, domain.ErrInvalidChangeTime, domain.ErrInsufficientBalance:
		return i18n.AccountBankBalancePreviewFailed
	}
	return fallback
}
