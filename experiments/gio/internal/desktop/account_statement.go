package desktop

import (
	"context"
	"errors"
	"slices"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	reconciliation "github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type statementSessions interface {
	List(context.Context) ([]domain.ReconciliationSession, error)
	Get(context.Context, string) (domain.ReconciliationSession, error)
	Preview(context.Context, application.ReconciliationAction) (reconciliation.SessionPreview, error)
	Commit(context.Context, string) (domain.ReconciliationSession, error)
}

type statementData struct {
	records   []domain.ReconciliationSession
	detail    domain.ReconciliationSession
	zone      *time.Location
	hasOrigin bool
}

type statementEvidence struct {
	day, time       string
	reference, note ui.TextField
	datePicker      ui.DatePicker
	timePicker      ui.TimePicker
	completeness    widget.Bool
}

type statementScreen struct {
	account    domain.Account
	selected   string
	data       *statementData
	loadFailed bool
	problem    i18n.MessageID
	saved      bool
	uncertain  bool
	form       *statementEvidence
	review     *reconciliation.SessionPreview
	loading    <-chan historyResult[statementData]
	previewing <-chan historyResult[reconciliation.SessionPreview]
	committing <-chan historyResult[domain.ReconciliationSession]

	back, reload, start, resume, check, finish, prepare, edit, confirm, replay widget.Clickable
	records                                                                    map[string]*widget.Clickable
	list                                                                       widget.List
}

type statementVisit struct {
	open     bool
	selected string
	position layout.Position
}

type accountStatement struct {
	service statementSessions
	origin  interface {
		HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	}
	open, reconciliation widget.Clickable
	screen               *statementScreen
	restore              statementVisit
	restoreFocus         bool
	openReconciliation   func(session, name string, focus event.Tag)
}

func statementCard(a domain.Account) bool {
	return a.AccountType == domain.TypeCreditCard && a.IsLiability() && a.TrackingMode == domain.TrackingBalance
}

func statementAccount(session domain.ReconciliationSession, id string) *domain.ReconciliationAccount {
	for i := range session.Accounts {
		if session.Accounts[i].AccountID == id {
			return &session.Accounts[i]
		}
	}
	return nil
}

func (p *accountsPage) startStatements(account domain.Account, session string) {
	p.statement.screen = &statementScreen{account: account, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
	p.selectStatement(session)
}

func (f *accountStatement) visit() statementVisit {
	if f.screen == nil {
		return statementVisit{}
	}
	return statementVisit{open: true, selected: f.screen.selected, position: f.screen.list.Position}
}

func (p *accountsPage) selectStatement(id string) {
	d := p.statement.screen
	d.selected, d.form, d.problem, d.saved = id, nil, i18n.NoMessage, false
	d.list.Position = layout.Position{}
	p.loadStatements()
}

func (p *accountsPage) loadStatements() {
	f, d := p.statement, p.statement.screen
	id, card := d.selected, d.account.ID.String()
	d.data, d.loadFailed = nil, false
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (statementData, error) {
		var data statementData
		if id == "" {
			rows, err := f.service.List(p.ctx)
			if err != nil {
				return data, err
			}
			for _, row := range rows {
				if slices.Contains(row.AccountIDs, card) {
					data.records = append(data.records, row)
				}
			}
			return data, nil
		}
		detail, err := f.service.Get(p.ctx, id)
		if err != nil {
			return data, err
		}
		if detail.ID != id || statementAccount(detail, card) == nil {
			return data, &domain.Error{Code: domain.ErrNotFound}
		}
		data.detail, data.zone = detail, time.UTC
		if detail.Status != "completed" {
			origin, err := f.origin.HistoryOrigin(p.ctx)
			if err != nil {
				return data, err
			}
			if origin != nil {
				data.zone, err = time.LoadLocation(origin.Timezone)
				data.hasOrigin = err == nil
				return data, err
			}
		}
		return data, nil
	})
}

func (d *statementScreen) action(action statement.Action) application.ReconciliationAction {
	id := d.account.ID.String()
	if action == statement.Start {
		return application.ReconciliationAction{Action: string(action), AccountIDs: []string{id}}
	}
	session := d.data.detail
	return application.ReconciliationAction{Action: string(action), SessionID: session.ID, ExpectedRevision: session.Revision}
}

func (p *accountsPage) previewStatement(in application.ReconciliationAction) {
	f, d := p.statement, p.statement.screen
	d.problem, d.saved = i18n.NoMessage, false
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (reconciliation.SessionPreview, error) {
		return f.service.Preview(p.ctx, in)
	})
}

func (p *accountsPage) confirmStatement() {
	f, d := p.statement, p.statement.screen
	plan := d.review.PlanID
	d.committing = runHistoryRead(&p.workers, p.invalidate, func() (domain.ReconciliationSession, error) {
		return f.service.Commit(p.ctx, plan)
	})
}

func (p *accountsPage) statementUnresolved() bool {
	return p.statement != nil && p.statement.screen != nil &&
		(p.statement.screen.committing != nil || p.statement.screen.uncertain)
}

func (p *accountsPage) pollStatement() {
	d := p.statement.screen
	select {
	case result := <-d.loading:
		d.loading, d.loadFailed = nil, result.err != nil
		if result.err == nil {
			d.data = &result.value
			d.records = make(map[string]*widget.Clickable, len(d.data.records))
			for _, row := range d.data.records {
				d.records[row.ID] = new(widget.Clickable)
			}
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		switch {
		case result.err == nil:
			d.review = &result.value
			d.list.Position = layout.Position{}
		case statementPreviewChanged(result.err):
			d.problem = i18n.StatementChanged
			if d.form != nil {
				d.form.completeness.Value = false
			}
			p.loadStatements()
		default:
			d.problem = statementPreviewProblem(result.err)
		}
	default:
	}
	select {
	case result := <-d.committing:
		d.committing = nil
		switch {
		case result.err == nil && result.value.ID != "":
			d.review, d.uncertain = nil, false
			p.selectStatement(result.value.ID)
			d.saved = true
		case statementCommitRefused(result.err):
			d.review, d.uncertain, d.problem = nil, false, i18n.StatementChanged
			if d.form != nil {
				d.form.completeness.Value = false
			}
			p.loadStatements()
		default:
			d.uncertain = true
		}
	default:
	}
}

func statementPreviewChanged(err error) bool {
	var issue *apierror.WireError
	if !errors.As(err, &issue) {
		return false
	}
	switch domain.ErrorCode(issue.Code) {
	case domain.ErrStalePreview:
		return issue.Field == "expiresAt"
	case domain.ErrConflict:
		switch issue.Field {
		case "financialRevision", "expectedRevision", "revision", "preview":
			return true
		}
	}
	return false
}

func statementCommitRefused(err error) bool {
	if statementPreviewChanged(err) {
		return true
	}
	var issue *apierror.WireError
	return errors.As(err, &issue) && issue.Code == string(domain.ErrValidation)
}

func statementPreviewProblem(err error) i18n.MessageID {
	var issue *apierror.WireError
	if errors.As(err, &issue) && issue.Code == string(domain.ErrValidation) {
		switch issue.Field {
		case "asOfDate":
			return i18n.StatementBadDate
		case "asOfTime":
			return i18n.StatementBadTime
		case "source.reference", "note":
			return i18n.StatementTextLimit
		}
	}
	return i18n.StatementPreviewFailed
}
