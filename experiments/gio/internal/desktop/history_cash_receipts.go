package desktop

import (
	"context"
	"slices"
	"sync"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type cashReceiptStore interface {
	Remember(context.Context, string, string) error
	Pending(context.Context) ([]pilot.PendingCashReceipt, error)
	Forget(string) error
}

type cashReceiptReader interface {
	InspectChangeReceipt(context.Context, string, string) (*domain.ChangePreview, error)
}

type cashReceiptNotice struct {
	mutation string
	message  i18n.MessageID
	activity domain.ActivityID
}

type cashReceiptRecovery struct {
	ctx        context.Context
	store      cashReceiptStore
	reader     cashReceiptReader
	invalidate func()
	workers    sync.WaitGroup

	notices             []cashReceiptNotice
	liveMutation        string
	reading             <-chan historyResult[[]cashReceiptNotice]
	forgetting          <-chan historyResult[string]
	problem             i18n.MessageID
	retry, open, forget widget.Clickable
}

func (r *cashReceiptRecovery) read() {
	r.reading = runHistoryRead(&r.workers, r.invalidate, func() ([]cashReceiptNotice, error) {
		pending, err := r.store.Pending(r.ctx)
		if err != nil {
			return nil, err
		}
		notices := make([]cashReceiptNotice, 0, len(pending))
		for _, entry := range pending {
			notice := cashReceiptNotice{mutation: entry.MutationID, message: i18n.CashReceiptUnconfirmed}
			if entry.MatchesLedger {
				receipt, err := r.reader.InspectChangeReceipt(r.ctx, entry.MutationID, entry.SHA256)
				switch {
				case err == nil && receipt == nil:
					notice.message = i18n.CashReceiptAbsent
				case err == nil && receipt.Activity.ID != "" && isCashReceiptActivity(receipt.Activity):
					notice.message, notice.activity = i18n.CashReceiptRecorded, receipt.Activity.ID
				}
			}
			notices = append(notices, notice)
		}
		return notices, nil
	})
}

func isCashReceiptActivity(activity domain.Activity) bool {
	return activity.Kind == domain.ActivityCashTransfer || activity.Kind == domain.ActivityFXConversion
}

func (r *cashReceiptRecovery) remember(notice cashReceiptNotice) {
	if i := slices.IndexFunc(r.notices, func(row cashReceiptNotice) bool { return row.mutation == notice.mutation }); i >= 0 {
		r.notices[i] = notice
	} else {
		r.notices = append(r.notices, notice)
	}
}

func (r *cashReceiptRecovery) dismiss(mutation string) {
	r.forgetting = runHistoryRead(&r.workers, r.invalidate, func() (string, error) {
		return mutation, r.store.Forget(mutation)
	})
}

func (r *cashReceiptRecovery) poll() {
	select {
	case result := <-r.reading:
		r.reading = nil
		if result.err != nil {
			r.problem = i18n.CashReceiptUnconfirmed
			break
		}
		r.problem = i18n.NoMessage
		for _, notice := range result.value {
			if notice.mutation == r.liveMutation {
				continue
			}
			// A live acknowledgement can arrive while the earlier lookup
			// is in flight. Keep that known outcome.
			if !slices.ContainsFunc(r.notices, func(row cashReceiptNotice) bool {
				return row.mutation == notice.mutation && row.activity != ""
			}) {
				r.remember(notice)
			}
		}
	default:
	}
	select {
	case result := <-r.forgetting:
		r.forgetting = nil
		if result.err != nil {
			r.problem = i18n.CashReceiptDismissFailed
		} else {
			r.problem = i18n.NoMessage
			r.notices = slices.DeleteFunc(r.notices, func(row cashReceiptNotice) bool { return row.mutation == result.value })
		}
	default:
	}
}

func (r *cashReceiptRecovery) layout(gtx layout.Context, view ui.View, open func(domain.ActivityID, event.Tag)) layout.Dimensions {
	r.poll()
	var notice cashReceiptNotice
	if len(r.notices) > 0 {
		notice = r.notices[0]
	}
	message := notice.message
	if r.problem != i18n.NoMessage {
		message = r.problem
	}
	if message == i18n.NoMessage {
		return layout.Dimensions{}
	}
	disabled := r.reading != nil || r.forgetting != nil
	if !disabled {
		switch {
		case notice.activity != "" && r.open.Clicked(gtx):
			open(notice.activity, &r.open)
			gtx.Execute(op.InvalidateCmd{})
		case notice.mutation != "" && r.forget.Clicked(gtx):
			r.dismiss(notice.mutation)
		case r.retry.Clicked(gtx):
			r.read()
		}
	}
	return layout.Inset{Top: 8, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{}
		if notice.activity != "" {
			buttons = append(buttons, layout.Rigid(view.Button(&r.open, view.Language.Text(i18n.CashReceiptOpen))))
		}
		if message == i18n.CashReceiptUnconfirmed || message == i18n.CashReceiptAbsent {
			buttons = append(buttons, layout.Rigid(view.Button(&r.retry, view.Language.Text(i18n.CashReceiptRetry))))
		}
		if notice.mutation != "" {
			buttons = append(buttons, layout.Rigid(view.Button(&r.forget, view.Language.Text(i18n.CashReceiptDismiss))))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx,
			layout.Rigid(view.Message(view.Language.Text(message), view.Colors.Ink)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if disabled {
					gtx = gtx.Disabled()
				}
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
			}),
		)
	})
}
