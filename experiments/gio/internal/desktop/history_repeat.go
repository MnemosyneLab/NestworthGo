package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type historyRepeatAPI interface {
	PreviewRepeatChange(context.Context, string, historyapi.ChangeCommandRequest) (historyapi.RepeatPreviewDTO, error)
	ConfirmRepeatChange(context.Context, string) (wire.ChangePreviewDTO, error)
}

type historyRepeatDraft struct {
	added        bool
	account      domain.AccountID
	currency     domain.CurrencyCode
	reason       domain.ActivityReason
	amount, note ui.TextField
	exactAt      time.Time
	day, minute  string
	sources      *history.CashMovementSources
	loadFailed   bool
	problem      i18n.MessageID
	focusAmount  bool
	refused      bool
	uncertain    bool
	missingPlan  bool
	review       *historyapi.RepeatPreviewDTO
	loading      <-chan historyResult[history.CashMovementSources]
	previewing   <-chan historyResult[historyapi.RepeatPreviewDTO]
	committing   <-chan historyResult[wire.ChangePreviewDTO]
	reviewList   widget.List

	accountPicker, currencyPicker, reasonPicker               ui.Select
	datePicker                                                ui.DatePicker
	timePicker                                                ui.TimePicker
	cancel, retryLoad, prepare, edit, confirm, retry, inspect widget.Clickable
}

func (r *historyReads) startRepeat() {
	source := r.detail.record.Activity
	effect := source.Effects[0] // The shared eligibility check requires one money effect.
	d := &historyRepeatDraft{
		added: source.Kind == domain.ActivityCashIn, account: *effect.AccountID, currency: effect.Money.Currency(),
		reason: source.Reason, amount: ui.NewTextField(true), note: ui.NewTextField(false),
		exactAt: r.clock().UTC().Truncate(time.Millisecond), focusAmount: true,
		reviewList: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	d.amount.Editor.SetText(effect.Money.CanonicalAmount())
	if source.Note != nil {
		d.note.Editor.SetText(*source.Note)
	}
	r.detail.repeatDraft = d
	r.loadRepeatSources()
}

func (r *historyReads) loadRepeatSources() {
	d := r.detail.repeatDraft
	d.loadFailed = false
	d.loading = runHistoryRead(&r.workers, r.invalidate, func() (history.CashMovementSources, error) {
		return history.LoadCashMovementSources(r.ctx, r.service, history.CashContribution)
	})
}

func (d *historyRepeatDraft) selectedAccount() (domain.Account, bool) {
	if d.sources != nil {
		for _, account := range d.sources.Accounts {
			if account.ID == d.account {
				return account, true
			}
		}
	}
	return domain.Account{}, false
}

func (d *historyRepeatDraft) request() historyapi.ChangeCommandRequest {
	kind := historyapi.ChangeMoneyRemoved
	if d.added {
		kind = historyapi.ChangeMoneyAdded
	}
	note := d.note.Text()
	in := historyapi.ChangeCommandRequest{
		Kind: kind, AccountID: d.account.String(), Amount: strings.TrimSpace(d.amount.Text()), Currency: d.currency.String(),
		Reason: string(d.reason), Note: &note, EffectiveLocalDate: d.day, EffectiveLocalTime: d.minute,
	}
	if !d.exactAt.IsZero() {
		in.EffectiveAt = d.exactAt.Format(time.RFC3339Nano)
		in.EffectiveLocalDate, in.EffectiveLocalTime = "", ""
	}
	return in
}

func (r *historyReads) previewRepeat() {
	visit := r.detail
	d := visit.repeatDraft
	if d.previewing != nil || d.refused || d.loading != nil || d.loadFailed {
		return
	}
	account, ok := d.selectedAccount()
	if !ok {
		d.problem = i18n.HistoryRepeatAccountUnavailable
		return
	}
	if _, err := domain.ParseSupportedCurrency(d.currency.String()); err != nil ||
		(account.TrackingMode != domain.TrackingHoldings && d.currency != account.DefaultCurrency) {
		d.problem = i18n.CashCurrencyRequired
		return
	}
	id, request := visit.id.String(), d.request()
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&r.workers, r.invalidate, func() (historyapi.RepeatPreviewDTO, error) {
		return r.repeatAPI.PreviewRepeatChange(r.ctx, id, request)
	})
}

func (r *historyReads) confirmRepeat() {
	d := r.detail.repeatDraft
	if d.review == nil || d.committing != nil || d.missingPlan {
		return
	}
	id := d.review.PlanID
	d.committing = runHistoryRead(&r.workers, r.invalidate, func() (wire.ChangePreviewDTO, error) {
		return r.repeatAPI.ConfirmRepeatChange(r.ctx, id)
	})
}

func (p *historyPage) repeatUnresolved() bool {
	if p.reads == nil || p.reads.detail == nil {
		return false
	}
	d := p.reads.detail.repeatDraft
	return d != nil && (d.committing != nil || d.uncertain)
}

func (p *historyPage) pollRepeat() {
	visit := p.reads.detail
	if visit == nil || visit.repeatDraft == nil {
		return
	}
	d := visit.repeatDraft
	select {
	case result := <-d.loading:
		d.loading, d.loadFailed = nil, result.err != nil
		if result.err == nil {
			d.sources = &result.value
			if d.day == "" {
				local := d.exactAt.In(d.sources.Location)
				d.day, d.minute = local.Format(time.DateOnly), local.Format("15:04")
			}
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		switch {
		case result.err != nil:
			d.problem = repeatProblem(result.err)
			d.refused = repeatSourceRefused(result.err)
		case result.value.PlanID != "" && len(result.value.Review) == 1:
			d.review = &result.value
			d.reviewList.Position = layout.Position{}
		default:
			d.problem = i18n.CashMovementPreviewFailed
		}
	default:
	}
	select {
	case result := <-d.committing:
		d.committing = nil
		if saved := result.value.Activity; saved.ID != "" {
			visit.repeatDraft = nil
			p.showPosted(domain.Activity{ID: domain.ActivityID(saved.ID), Kind: domain.ActivityKind(saved.Kind),
				Reason: domain.ActivityReason(saved.Reason)}, i18n.HistoryRepeatSaved, &visit.repeat)
			return
		}
		switch {
		case repeatSourceRefused(result.err):
			d.review, d.uncertain, d.refused, d.problem = nil, false, true, i18n.HistoryRepeatRefused
		case repeatPlanUnavailable(result.err):
			// This result did not reach the receipt lookup. Never offer a
			// replacement preview from this review.
			d.uncertain, d.missingPlan = false, true
			p.ledgerChanged()
		case repeatDefinitelyRejected(result.err):
			d.review, d.uncertain, d.problem = nil, false, repeatProblem(result.err)
			p.reads.loadRepeatSources()
		default:
			d.uncertain = true
		}
	default:
	}
}

func repeatSourceRefused(err error) bool {
	var issue *apierror.WireError
	return errors.As(err, &issue) && (issue.Code == string(domain.ErrAlreadyUndone) || issue.Field == "sourceActivityId")
}

func repeatPlanUnavailable(err error) bool {
	var issue *apierror.WireError
	return errors.As(err, &issue) && issue.Code == string(domain.ErrStalePreview) && issue.Field != "previewToken"
}

func repeatDefinitelyRejected(err error) bool {
	var issue *apierror.WireError
	if !errors.As(err, &issue) {
		return false
	}
	switch domain.ErrorCode(issue.Code) {
	case domain.ErrStalePreview:
		return issue.Field == "previewToken"
	case domain.ErrValidation, domain.ErrNotFound, domain.ErrConflict, domain.ErrDecimalOverflow, domain.ErrMissingHistoryOrigin,
		domain.ErrHistoryTimezoneRequired, domain.ErrInvalidChange, domain.ErrInvalidChangeTime, domain.ErrCashBalanceTimeConflict,
		domain.ErrNoChange, domain.ErrInsufficientBalance, domain.ErrInsufficientQuantity, domain.ErrManagedPosition:
		return true
	}
	return false
}

func repeatProblem(err error) i18n.MessageID {
	if repeatSourceRefused(err) {
		return i18n.HistoryRepeatRefused
	}
	var issue *apierror.WireError
	if errors.As(err, &issue) {
		switch domain.ErrorCode(issue.Code) {
		case domain.ErrStalePreview:
			return i18n.CashMovementStale
		case domain.ErrInsufficientBalance:
			return i18n.CashMovementInsufficient
		case domain.ErrInvalidChangeTime, domain.ErrCashBalanceTimeConflict:
			return i18n.ContributionInvalidTime
		}
		switch issue.Field {
		case "note":
			return i18n.HistoryRepeatNoteTooLong
		case "accountId", "account":
			return i18n.HistoryRepeatAccountUnavailable
		case "amount":
			return i18n.ContributionInvalidAmount
		}
	}
	return i18n.CashMovementPreviewFailed
}
