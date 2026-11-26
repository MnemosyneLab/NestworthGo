package desktop

import (
	"context"
	"errors"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyStartLedger interface {
	PreviewHistoryStart(context.Context, string, string, map[domain.HoldingID]string) (application.HistoryStartReview, error)
	ConfirmHistoryStart(context.Context, application.HistoryStartReview) (domain.HistoryOrigin, error)
	history.OpeningHoldingReader
}

type historyStartReview struct {
	plan     application.HistoryStartReview
	accounts map[domain.AccountID]domain.Account
	holdings map[domain.HoldingID]history.StartingPointHolding
}

type historyStartHolding struct {
	history.StartingPointHolding
	cost ui.TextField
}

type historyStart struct {
	service                                      historyStartLedger
	accountName                                  string
	focusDate, focusConfirm                      bool
	open, verifying, saved, ready                bool
	day                                          string
	problem                                      i18n.MessageID
	draftProblem                                 i18n.MessageID
	holdings                                     []*historyStartHolding
	loading                                      <-chan historyResult[[]history.StartingPointHolding]
	review                                       *historyStartReview
	preview                                      <-chan historyResult[historyStartReview]
	write                                        <-chan historyResult[domain.HistoryOrigin]
	read                                         <-chan historyResult[history.References]
	calendar                                     ui.DatePicker
	balances                                     widget.List
	start, cancel, prepare, edit, confirm, retry widget.Clickable
}

func newHistoryStart(service historyStartLedger) *historyStart {
	return &historyStart{service: service, balances: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

func (p *historyPage) beginStartingPoint(s *historyStart) {
	s.open = true
	p.loadStartingPointHoldings(s)
}

func (p *historyPage) loadStartingPointHoldings(s *historyStart) {
	r := p.reads
	s.draftProblem = i18n.NoMessage
	s.loading = runHistoryRead(&r.workers, r.invalidate, func() ([]history.StartingPointHolding, error) {
		var draft []history.StartingPointHolding
		err := r.service.WithRead(r.ctx, func(ctx context.Context) error {
			var err error
			draft, err = history.LoadStartingPointHoldings(ctx, s.service)
			return err
		})
		return draft, err
	})
}

func (s *historyStart) setHoldings(rows []history.StartingPointHolding) {
	old := make(map[domain.HoldingID]*historyStartHolding, len(s.holdings))
	for _, row := range s.holdings {
		old[row.HoldingID] = row
	}
	s.holdings = s.holdings[:0]
	for _, row := range rows {
		field := old[row.HoldingID]
		if field == nil || field.Currency != row.Currency {
			field = &historyStartHolding{cost: ui.NewTextField(true)}
		}
		field.StartingPointHolding = row
		s.holdings = append(s.holdings, field)
	}
}

func (s *historyStart) canPreview() bool {
	if s.loading != nil || s.draftProblem != i18n.NoMessage || s.preview != nil {
		return false
	}
	for _, row := range s.holdings {
		if strings.TrimSpace(row.cost.Text()) == "" {
			return false
		}
	}
	return true
}

func (p *historyPage) previewStartHistory(s *historyStart, zone, day string) {
	r := p.reads
	s.problem = i18n.NoMessage
	costs := make(map[domain.HoldingID]string, len(s.holdings))
	presented := make(map[domain.HoldingID]history.StartingPointHolding, len(s.holdings))
	for _, holding := range s.holdings {
		costs[holding.HoldingID] = holding.cost.Text()
		presented[holding.HoldingID] = holding.StartingPointHolding
	}
	s.preview = runHistoryRead(&r.workers, r.invalidate, func() (historyStartReview, error) {
		var review historyStartReview
		err := r.service.WithRead(r.ctx, func(ctx context.Context) error {
			holdings, err := history.LoadStartingPointHoldings(ctx, s.service)
			if err != nil {
				return err
			}
			if len(holdings) != len(presented) {
				return &domain.Error{Code: domain.ErrStalePreview}
			}
			for _, holding := range holdings {
				old := presented[holding.HoldingID]
				if old.HoldingID == "" || old.Currency != holding.Currency || old.Quantity != holding.Quantity {
					return &domain.Error{Code: domain.ErrStalePreview}
				}
			}
			plan, err := s.service.PreviewHistoryStart(ctx, zone, day, costs)
			if err != nil {
				return err
			}
			rows, err := r.service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
			if err != nil {
				return err
			}
			review = historyStartReview{plan: plan, accounts: make(map[domain.AccountID]domain.Account, len(rows))}
			review.holdings = presented
			for _, row := range rows {
				review.accounts[row.Account.ID] = row.Account
			}
			return nil
		})
		return review, err
	})
}

func (p *historyPage) confirmStartHistory(s *historyStart) {
	r := p.reads
	plan := s.review.plan
	s.write = runHistoryRead(&r.workers, r.invalidate, func() (domain.HistoryOrigin, error) {
		return s.service.ConfirmHistoryStart(r.ctx, plan)
	})
}

func (p *historyPage) readStartingPoint(s *historyStart) {
	r := p.reads
	s.verifying = true
	s.read = runHistoryRead(&r.workers, r.invalidate, func() (history.References, error) {
		return history.LoadReferences(r.ctx, r.service)
	})
}

func (p *historyPage) pollStartingPoint(s *historyStart) {
	r := p.reads
	if s == nil || !s.open {
		return
	}
	select {
	case result := <-s.loading:
		s.loading = nil
		if result.err != nil {
			s.draftProblem = startHistoryProblem(result.err, i18n.HistoryStartDraftFailed)
		} else {
			s.setHoldings(result.value)
		}
	default:
	}
	select {
	case result := <-s.preview:
		s.preview = nil
		if result.err != nil {
			s.problem = startHistoryProblem(result.err, i18n.HistoryStartPreviewFailed)
			if s.problem == i18n.HistoryStartExists {
				p.readStartingPoint(s)
			} else if s.problem == i18n.HistoryAmendmentStale || s.problem == i18n.HistoryStartHoldingCost {
				p.loadStartingPointHoldings(s)
			}
		} else {
			s.review = &result.value
			s.balances.Position = layout.Position{}
			s.focusConfirm = s.accountName != ""
		}
	default:
	}
	select {
	case result := <-s.write:
		s.write, s.review = nil, nil
		s.saved = result.value.ID != ""
		s.problem = startHistoryProblem(result.err, i18n.HistoryStartWriteFailed)
		p.readStartingPoint(s)
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
			p.ledgerChanged()
		} else if s.problem == i18n.HistoryAmendmentStale {
			p.loadStartingPointHoldings(s)
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
		return i18n.HistoryStartQuoteRequired
	case issue.Field == "unitPrice":
		return i18n.HistoryStartInvalidCost
	case issue.Field == "openingHoldings":
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

func (s *historyStart) unresolved() bool {
	return s != nil && (s.write != nil || s.verifying)
}
