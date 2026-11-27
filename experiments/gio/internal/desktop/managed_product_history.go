package desktop

import (
	"context"
	"fmt"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const productHistoryPageSize = 25

type productHistoryRead struct {
	page     application.ProductOperationPage
	zone     *time.Location
	interest domain.ActivityID
}

type productHistory struct {
	rows     []domain.ProductOperationHistoryRow
	next     *string
	latest   domain.ProductOperationID
	zone     *time.Location
	interest domain.ActivityID
	undo     application.ProductUndoAvailability
	loaded   bool
	cursor   string
	problem  i18n.MessageID
	cancel   context.CancelFunc
	pending  <-chan historyResult[productHistoryRead]
	refresh  widget.Clickable
	more     widget.Clickable
	focus    *widget.Clickable
}

func readProductHistoryPage(ctx context.Context, service productReader, id domain.ProductContractID, cursor string) (out productHistoryRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		var err error
		out.page, err = service.ListProductOperations(ctx, id, cursor, productHistoryPageSize)
		if err != nil || cursor != "" {
			return err
		}
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin != nil {
			out.zone, err = time.LoadLocation(origin.Timezone)
			if err != nil {
				return err
			}
		}
		out.interest, err = productInterestActivity(ctx, service, out.page.Operations)
		return err
	})
	return out, err
}

func productInterestActivity(ctx context.Context, service productReader, rows []domain.ProductOperationHistoryRow) (domain.ActivityID, error) {
	for _, row := range rows {
		if row.Kind != domain.ProductOpReceiveInterest || row.ReversedByOperationID != nil {
			continue
		}
		receipt, err := service.ReadProductOperationReceipt(ctx, row.ID)
		if err != nil {
			return "", err
		}
		if len(receipt.ActivityIDs) == 1 {
			return receipt.ActivityIDs[0], nil
		}
	}
	return "", nil
}

func (h *productHistory) stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.cancel, h.pending = nil, nil
}

func (p *productDetailPage) loadProductHistory(cursor string) {
	h := &p.history
	if h.pending != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	h.cancel, h.cursor, h.problem = cancel, cursor, i18n.NoMessage
	id := p.id
	h.pending = runHistoryRead(&p.workers, p.invalidate, func() (productHistoryRead, error) {
		return readProductHistoryPage(ctx, p.service, id, cursor)
	})
}

func (p *productDetailPage) pollProductHistory() {
	h := &p.history
	select {
	case reply := <-h.pending:
		h.stop()
		if reply.err != nil {
			h.problem = i18n.ProductHistoryFailed
			return
		}
		page := reply.value.page
		var latest domain.ProductOperationID
		if page.LatestRecordedOperationID != nil {
			latest = *page.LatestRecordedOperationID
		}
		if h.cursor == "" {
			h.rows, h.latest, h.zone = page.Operations, latest, reply.value.zone
			h.interest, h.loaded = reply.value.interest, true
		} else {
			// A new operation can also update reversal status on the earlier
			// page. Restart the read instead of mixing the two snapshots.
			if h.latest != latest {
				h.problem, h.focus = i18n.ProductHistoryChanged, &h.refresh
				return
			}
			h.rows = append(h.rows, page.Operations...)
		}
		h.undo = page.Undo
		h.next = page.Next
		if h.next == nil && h.focus == &h.more {
			h.focus = &h.refresh
		}
	default:
	}
}

func (p *productDetailPage) productHistoryRows(view ui.View) []layout.Widget {
	h, l := &p.history, view.Language
	status := l.Text(i18n.ProductHistoryLoading)
	color := view.Colors.SecondaryInk
	switch {
	case h.problem != i18n.NoMessage:
		status, color = l.Text(h.problem), view.Colors.ErrorInk
	case h.pending != nil:
	case h.loaded && len(h.rows) == 0:
		status = l.Text(i18n.ProductHistoryEmpty)
	case h.loaded && h.next == nil:
		status = fmt.Sprintf(l.Text(i18n.ProductHistoryComplete), len(h.rows))
	case h.loaded:
		status = fmt.Sprintf(l.Text(i18n.ProductHistoryCount), len(h.rows))
	}
	refresh := i18n.ProductHistoryRefresh
	if h.problem == i18n.ProductHistoryFailed && h.cursor == "" {
		refresh = i18n.HistoryRetry
	}
	fields := []layout.Widget{view.Message(status, color), p.productHistoryButton(view, &h.refresh, refresh, "")}
	if h.pending == nil && h.problem == i18n.NoMessage && h.undo.Reason != nil {
		if reason := productUndoReason(*h.undo.Reason); reason != i18n.NoMessage {
			fields = append(fields, view.Message(l.Text(reason), view.Colors.SecondaryInk))
		}
	}
	if h.next != nil && h.problem != i18n.ProductHistoryChanged {
		label := i18n.ProductHistoryOlder
		if h.problem == i18n.ProductHistoryFailed && h.cursor != "" {
			label = i18n.ProductHistoryRetryOlder
		}
		fields = append(fields, p.productHistoryButton(view, &h.more, label, *h.next))
	}
	rows := make([]layout.Widget, 0, len(h.rows)+1)
	rows = append(rows, fundsCard(view, l.Text(i18n.ProductHistoryHeading), fields...))
	zone := h.zone
	if zone == nil {
		zone = time.UTC
	}
	for _, row := range h.rows {
		fields := []layout.Widget{view.Message(tradeTimestamp(view, row.EffectiveAt, zone), view.Colors.SecondaryInk)}
		if row.ReversedByOperationID != nil {
			fields = append(fields, view.Message(l.Text(i18n.ProductHistoryReversed), view.Colors.SecondaryInk))
		}
		rows = append(rows, fundsCard(view, l.Text(productHistoryKind(row.Kind)), fields...))
	}
	return rows
}

func (p *productDetailPage) productHistoryButton(view ui.View, button *widget.Clickable, label i18n.MessageID, cursor string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h := &p.history
		if h.pending != nil {
			gtx = gtx.Disabled()
		} else if h.focus == button {
			gtx.Execute(key.FocusCmd{Tag: button})
			h.focus = nil
		}
		if button.Clicked(gtx) && gtx.Enabled() {
			h.focus = button
			p.loadProductHistory(cursor)
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return layout.W.Layout(gtx, view.Button(button, view.Language.Text(label)))
	}
}

func productHistoryKind(kind domain.ProductOperationKind) i18n.MessageID {
	switch kind {
	case domain.ProductOpOpen:
		return i18n.ProductHistoryOpened
	case domain.ProductOpRecordExisting:
		return i18n.ProductHistoryRecorded
	case domain.ProductOpReceiveInterest:
		return i18n.ProductHistoryInterest
	case domain.ProductOpSettle:
		return i18n.ProductHistorySettled
	case domain.ProductOpRenew:
		return i18n.ProductHistoryRenewed
	case domain.ProductOpUndo:
		return i18n.ProductHistoryUndo
	case domain.ProductOpValueObservation:
		return i18n.ProductHistoryValuation
	default:
		return i18n.OverviewNoValue
	}
}
