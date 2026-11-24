package desktop

import (
	"context"
	"errors"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyStartLedger interface {
	PreviewBalanceHistoryStart(context.Context, string, string) (application.BalanceHistoryStartReview, error)
	ConfirmBalanceHistoryStart(context.Context, application.BalanceHistoryStartReview) (domain.HistoryOrigin, error)
}

type historyStartReview struct {
	plan     application.BalanceHistoryStartReview
	accounts map[domain.AccountID]domain.Account
}

type historyStart struct {
	service                                      historyStartLedger
	open, verifying, saved, ready                bool
	day                                          string
	problem                                      i18n.MessageID
	review                                       *historyStartReview
	preview                                      <-chan historyResult[historyStartReview]
	write                                        <-chan historyResult[domain.HistoryOrigin]
	read                                         <-chan historyResult[history.References]
	calendar                                     ui.DatePicker
	balances                                     widget.List
	start, cancel, prepare, edit, confirm, retry widget.Clickable
}

func (p *historyPage) previewStartHistory(zone, day string) {
	s, r := p.startPoint, p.reads
	s.problem = i18n.NoMessage
	s.preview = runHistoryRead(&r.workers, r.invalidate, func() (historyStartReview, error) {
		var review historyStartReview
		err := r.service.WithRead(r.ctx, func(ctx context.Context) error {
			plan, err := s.service.PreviewBalanceHistoryStart(ctx, zone, day)
			if err != nil {
				return err
			}
			rows, err := r.service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
			if err != nil {
				return err
			}
			review = historyStartReview{plan: plan, accounts: make(map[domain.AccountID]domain.Account, len(rows))}
			for _, row := range rows {
				review.accounts[row.Account.ID] = row.Account
			}
			return nil
		})
		return review, err
	})
}

func (p *historyPage) confirmStartHistory() {
	s, r := p.startPoint, p.reads
	plan := s.review.plan
	s.write = runHistoryRead(&r.workers, r.invalidate, func() (domain.HistoryOrigin, error) {
		return s.service.ConfirmBalanceHistoryStart(r.ctx, plan)
	})
}

func (p *historyPage) readStartingPoint() {
	s, r := p.startPoint, p.reads
	s.verifying = true
	s.read = runHistoryRead(&r.workers, r.invalidate, func() (history.References, error) {
		return history.LoadReferences(r.ctx, r.service)
	})
}

func (p *historyPage) pollStartingPoint() {
	s, r := p.startPoint, p.reads
	if s == nil || !s.open {
		return
	}
	select {
	case result := <-s.preview:
		s.preview = nil
		if result.err != nil {
			s.problem = startHistoryProblem(result.err, i18n.HistoryStartPreviewFailed)
			if s.problem == i18n.HistoryStartExists {
				p.readStartingPoint()
			}
		} else {
			s.review = &result.value
			s.balances.Position = layout.Position{}
		}
	default:
	}
	select {
	case result := <-s.write:
		s.write, s.review = nil, nil
		s.saved = result.value.ID != ""
		s.problem = startHistoryProblem(result.err, i18n.HistoryStartWriteFailed)
		p.readStartingPoint()
	default:
	}
	select {
	case result := <-s.read:
		s.read = nil
		if result.err != nil || (s.saved && result.value.Origin == nil) {
			return
		}
		s.verifying = false
		r.references, r.refsRead, r.refsErr = &result.value, nil, nil
		if result.value.Origin != nil {
			s.ready, s.open = true, false
			p.revision++
			p.model.Refresh()
			if p.onPosted != nil {
				p.onPosted()
			}
		}
	default:
	}
}

func startHistoryProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	if err == nil {
		return i18n.NoMessage
	}
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch {
	case issue.Code == domain.ErrCostBasisRequired:
		return i18n.HistoryStartHoldingCost
	case issue.Code == domain.ErrStalePreview:
		return i18n.HistoryAmendmentStale
	case issue.Code == domain.ErrConflict && issue.Field == "startDate":
		return i18n.HistoryStartPastBalances
	case issue.Code == domain.ErrConflict && issue.Field == "historyOrigin":
		return i18n.HistoryStartExists
	case issue.Field == "date" || issue.Field == "startDate" || issue.Field == "timezone":
		return i18n.OnboardingDateInvalid
	default:
		return fallback
	}
}

func (p *historyPage) startingPointUnresolved() bool {
	s := p.startPoint
	return s != nil && (s.write != nil || s.verifying)
}
