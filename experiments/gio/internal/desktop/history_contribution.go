package desktop

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type contributionResult[T any] struct {
	value T
	err   error
}

type contributionDraft struct {
	sources                                   *history.ContributionSources
	loading                                   <-chan contributionResult[history.ContributionSources]
	sourceError                               i18n.MessageID
	account                                   domain.AccountID
	amount                                    ui.TextField
	day, time                                 string
	focusAmount                               bool
	accountPicker                             ui.Select
	datePicker                                ui.DatePicker
	timePicker                                ui.TimePicker
	cancel, retrySources, preview, edit, post widget.Clickable

	previewing <-chan contributionResult[history.ReviewedContribution]
	review     *history.ReviewedContribution
	posting    <-chan contributionResult[domain.ChangePreview]
	uncertain  bool
	problem    i18n.MessageID
}

type contributionVisit struct {
	id          domain.ActivityID
	posted      bool
	pending     <-chan contributionResult[history.ContributionDetail]
	record      *history.ContributionDetail
	err         error
	back, retry widget.Clickable
}

type historyContributions struct {
	ctx        context.Context
	service    history.ContributionLedger
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	add        widget.Clickable
	buttons    map[domain.ActivityID]*widget.Clickable
	draft      *contributionDraft
	detail     *contributionVisit
}

func newHistoryContributions(ctx context.Context, service history.ContributionLedger, clock func() time.Time, invalidate func()) *historyContributions {
	return &historyContributions{ctx: ctx, service: service, clock: clock, invalidate: invalidate, buttons: make(map[domain.ActivityID]*widget.Clickable)}
}

func contributionRead[T any](p *historyContributions, read func() (T, error)) <-chan contributionResult[T] {
	results := make(chan contributionResult[T], 1)
	p.workers.Go(func() {
		value, err := read()
		results <- contributionResult[T]{value: value, err: err}
		p.invalidate()
	})
	return results
}

func (p *historyContributions) start() {
	p.draft = &contributionDraft{amount: ui.NewTextField(true), focusAmount: true}
	p.loadSources()
}

func (p *historyContributions) loadSources() {
	d := p.draft
	d.sourceError = i18n.NoMessage
	d.loading = contributionRead(p, func() (history.ContributionSources, error) {
		return history.LoadContributionSources(p.ctx, p.service)
	})
}

func (d *contributionDraft) receiveSources(sources history.ContributionSources, now time.Time) {
	first := d.sources == nil
	d.sources = &sources
	if first {
		local := now.In(sources.Location)
		d.day, d.time = local.Format(time.DateOnly), local.Format("15:04")
		if len(sources.Accounts) == 1 {
			d.account = sources.Accounts[0].ID
		}
	}
}

func (d *contributionDraft) selected() (domain.Account, bool) {
	if d.sources != nil {
		for _, account := range d.sources.Accounts {
			if account.ID == d.account {
				return account, true
			}
		}
	}
	return domain.Account{}, false
}

func (p *historyContributions) preview() {
	d := p.draft
	if d.previewing != nil {
		return
	}
	if _, ok := d.selected(); !ok {
		d.problem = i18n.ContributionInvalidAccount
		return
	}
	input := history.ContributionInput{Account: d.account, Amount: d.amount.Text(), Date: d.day, Time: d.time}
	d.problem = i18n.NoMessage
	d.previewing = contributionRead(p, func() (history.ReviewedContribution, error) {
		return history.ReviewContribution(p.ctx, p.service, input)
	})
}

func (p *historyContributions) post() {
	d := p.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	review := *d.review
	d.posting = contributionRead(p, func() (domain.ChangePreview, error) {
		return review.Commit(p.ctx, p.service)
	})
}

func (p *historyContributions) writing() bool {
	return p != nil && p.draft != nil && p.draft.posting != nil
}

func (p *historyContributions) open(id domain.ActivityID, posted bool) {
	p.detail = &contributionVisit{id: id, posted: posted}
	p.readDetail()
}

func (p *historyContributions) readDetail() {
	visit := p.detail
	id := visit.id
	visit.err = nil
	visit.pending = contributionRead(p, func() (history.ContributionDetail, error) {
		return history.LoadContributionDetail(p.ctx, p.service, id)
	})
}

func (p *historyContributions) update() bool {
	if visit := p.detail; visit != nil {
		select {
		case result := <-visit.pending:
			visit.pending, visit.err = nil, result.err
			if result.err == nil {
				visit.record = &result.value
			}
		default:
		}
	}
	d := p.draft
	if d == nil {
		return false
	}
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sourceError = contributionProblem(result.err, i18n.ContributionSourcesFailed)
		} else {
			d.receiveSources(result.value, p.clock())
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = contributionProblem(result.err, i18n.ContributionPreviewFailed)
			if d.problem == i18n.ContributionInvalidAccount {
				p.loadSources()
			}
		} else {
			d.review = &result.value
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.Activity.ID != "" {
			p.draft = nil
			p.open(result.value.Activity.ID, true)
			return true
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// Replay runs before the guard. A stale token here means this
			// mutation has no durable outcome and needs a new preview.
			d.review, d.uncertain, d.problem = nil, false, i18n.ContributionStale
			p.loadSources()
		} else {
			// A transport or storage failure may follow the commit. Keep the
			// exact request frozen until the idempotent retry resolves it.
			d.uncertain = true
		}
	default:
	}
	return false
}

func contributionProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrHistoryNotStarted:
		return i18n.ContributionHistoryRequired
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	}
	switch issue.Field {
	case "accountId", "account":
		return i18n.ContributionInvalidAccount
	case "amount":
		return i18n.ContributionInvalidAmount
	}
	return fallback
}

func (p *historyContributions) detailButton(id domain.ActivityID) *widget.Clickable {
	if p.buttons[id] == nil {
		p.buttons[id] = new(widget.Clickable)
	}
	return p.buttons[id]
}

func (p *historyContributions) pruneButtons(rows []history.Row) {
	for id := range p.buttons {
		if !slices.ContainsFunc(rows, func(row history.Row) bool { return row.ID == id }) {
			delete(p.buttons, id)
		}
	}
}
