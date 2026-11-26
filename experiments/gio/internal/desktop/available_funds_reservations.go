package desktop

import (
	"context"
	"errors"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fundsReservationLedger interface {
	availableFundsReader
	WithWrite(context.Context, func(context.Context) error) error
	MobileDataVersion() string
	ListLiquidityReservations(context.Context) ([]domain.LiquidityReservation, error)
	SaveLiquidityReservation(context.Context, application.SaveReservationInput) (domain.LiquidityReservation, error)
	ReleaseLiquidityReservation(context.Context, domain.LiquidityReservationID, int) (domain.LiquidityReservation, error)
}

type fundsReservationRead struct {
	sources      []domain.LiquiditySourceResult
	accounts     map[domain.AccountID]domain.Account
	horizon      string
	records      []domain.LiquidityReservation
	activeLabels map[string]int
	version      string
}

func (r fundsReservationRead) source(ref domain.LiquiditySourceRef) *domain.LiquiditySourceResult {
	at := slices.IndexFunc(r.sources, func(row domain.LiquiditySourceResult) bool { return row.Ref.Key() == ref.Key() })
	if at < 0 {
		return nil
	}
	return &r.sources[at]
}

func (r fundsReservationRead) available(ref domain.LiquiditySourceRef) *domain.Money {
	source := r.source(ref)
	if source != nil {
		if result := fundsResultForHorizon(*source, r.horizon); result != nil {
			return result.UnreservedNative
		}
	}
	return nil
}

func reservableFundsSource(source *domain.LiquiditySourceResult) bool {
	return source != nil && !source.Excluded && source.NativeCurrency != ""
}

func readFundsReservation(ctx context.Context, service fundsReservationLedger, ref domain.LiquiditySourceRef, state fundsViewState) (out fundsReservationRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		// Keep the funding projection, revisions and recovery readback in one
		// operation gate and on the same horizon as the source list.
		report, err := readAvailableFundsQuery(ctx, service, state.query())
		if err != nil {
			return err
		}
		records, err := service.ListLiquidityReservations(ctx)
		if err != nil {
			return err
		}
		if ref.Kind != "" {
			records = slices.DeleteFunc(records, func(row domain.LiquidityReservation) bool { return row.Source.Key() != ref.Key() })
		}
		out = fundsReservationRead{accounts: report.accounts, horizon: report.today.HorizonOn, records: records,
			activeLabels: make(map[string]int), version: service.MobileDataVersion()}
		for _, record := range records {
			if record.Active() {
				out.activeLabels[record.Label]++
			}
		}
		if fundsBucketForHorizon(report.report.Overview.Buckets, state.horizonOn) != nil {
			out.horizon = state.horizonOn
		}
		for _, row := range report.report.Sources {
			out.sources = append(out.sources, row.Result)
		}
		return nil
	})
	return out, err
}

type fundsReservationPlan struct {
	source   domain.LiquiditySourceRef
	version  string
	before   []domain.LiquidityReservation
	label    string
	amount   domain.Money
	edit     *domain.LiquidityReservationID
	release  domain.LiquidityReservationID
	revision int
}

func (p fundsReservationPlan) commit(ctx context.Context, service fundsReservationLedger) (row domain.LiquidityReservation, err error) {
	err = service.WithWrite(ctx, func(ctx context.Context) error {
		if p.version == "" || service.MobileDataVersion() != p.version {
			return &domain.Error{Code: domain.ErrStalePreview}
		}
		if p.release != "" {
			row, err = service.ReleaseLiquidityReservation(ctx, p.release, p.revision)
		} else {
			row, err = service.SaveLiquidityReservation(ctx, application.SaveReservationInput{
				ID: p.edit, ExpectedRevision: p.revision, Source: p.source, Label: p.label, Amount: p.amount.CanonicalAmount(),
			})
		}
		return err
	})
	return row, err
}

type reservationVerification uint8

const (
	reservationAbsent reservationVerification = iota
	reservationVerified
	reservationUncertain
)

// Save allocates its own ID. A lost response can be recovered only from a new
// matching row, never from a pre-existing reservation with the same label.
func (p fundsReservationPlan) verify(after []domain.LiquidityReservation, acknowledged domain.LiquidityReservationID) reservationVerification {
	if p.release != "" {
		for _, row := range after {
			if row.ID == p.release && row.ReleasedAt != nil && row.Revision > p.revision {
				return reservationVerified
			}
		}
	} else if p.edit != nil {
		for _, row := range after {
			if row.ID == *p.edit && row.Source.Key() == p.source.Key() && row.Active() &&
				row.Revision == p.revision+1 && row.Label == p.label && row.Amount.Currency() == p.amount.Currency() && row.Amount.CanonicalAmount() == p.amount.CanonicalAmount() {
				return reservationVerified
			}
		}
	} else {
		matched := 0
		for _, row := range after {
			if row.Source.Key() != p.source.Key() {
				continue
			}
			if acknowledged != "" {
				if row.ID == acknowledged {
					return reservationVerified
				}
				continue
			}
			if row.Label != p.label || row.Amount.Currency() != p.amount.Currency() || row.Amount.CanonicalAmount() != p.amount.CanonicalAmount() {
				continue
			}
			if slices.ContainsFunc(p.before, func(old domain.LiquidityReservation) bool { return old.ID == row.ID }) {
				continue
			}
			matched++
		}
		if matched == 1 {
			return reservationVerified
		}
		if matched > 1 {
			return reservationUncertain
		}
	}
	if acknowledged != "" {
		return reservationUncertain
	}
	return reservationAbsent
}

type fundsReservationSession struct {
	source      domain.LiquiditySourceRef
	name        string
	entry       *widget.Clickable
	current     *fundsReservationRead
	read        <-chan historyResult[fundsReservationRead]
	write       <-chan historyResult[domain.LiquidityReservation]
	plan        *fundsReservationPlan
	checking    bool
	readError   bool
	ack         domain.LiquidityReservationID
	focusTarget domain.LiquidityReservationID
	notice      i18n.MessageID
	failure     i18n.MessageID

	editing                   bool
	editRow                   *domain.LiquidityReservation
	selected                  string
	picker                    ui.Select
	actionFocus               *widget.Clickable
	restoreFocus              bool
	label, amount             ui.TextField
	list                      widget.List
	close, add, review, retry widget.Clickable
	loadLatest                widget.Clickable
	confirm                   widget.Clickable
	actions                   map[domain.LiquidityReservationID]*fundsReservationActions
}

type fundsReservationActions struct {
	edit, release widget.Clickable
}

func (p *availableFundsPage) openReservations(source domain.LiquiditySourceRef, name string, entry *widget.Clickable) {
	p.reservation = &fundsReservationSession{
		source: source, name: name, entry: entry, label: ui.NewTextField(true), amount: ui.NewTextField(true),
		actions: make(map[domain.LiquidityReservationID]*fundsReservationActions),
		list:    widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	p.readReservations()
}

func (p *availableFundsPage) reservationUnresolved() bool {
	r := p.reservation
	return r != nil && (r.write != nil || r.checking)
}

func (p *availableFundsPage) readReservations() {
	r := p.reservation
	ref, state := r.source, p.state
	r.readError = false
	r.read = runHistoryRead(&p.workers, p.invalidate, func() (fundsReservationRead, error) {
		return readFundsReservation(p.ctx, p.reservationLedger, ref, state)
	})
}

func (p *availableFundsPage) confirmReservation() {
	r := p.reservation
	plan := *r.plan
	r.notice = i18n.NoMessage
	r.write = runHistoryRead(&p.workers, p.invalidate, func() (domain.LiquidityReservation, error) {
		return plan.commit(p.ctx, p.reservationLedger)
	})
}

func (p *availableFundsPage) pollReservations() {
	r := p.reservation
	select {
	case result := <-r.write:
		r.write = nil
		var issue *domain.Error
		if errors.As(result.err, &issue) && (issue.Code == domain.ErrStalePreview || issue.Code == domain.ErrRevisionConflict) {
			r.plan, r.notice = nil, i18n.HistoryAmendmentStale
		} else {
			r.ack, r.checking, r.failure = result.value.ID, true, i18n.ReserveNotSaved
			if errors.As(result.err, &issue) && issue.Code == domain.ErrValidation && issue.Field == "label" {
				r.failure = i18n.ReserveInvalidLabel
			}
		}
		p.readReservations()
	default:
	}
	select {
	case result := <-r.read:
		r.read = nil
		r.readError = result.err != nil
		if r.readError {
			return
		}
		r.current = &result.value
		if r.focusTarget != "" && !slices.ContainsFunc(r.current.records, func(row domain.LiquidityReservation) bool { return row.ID == r.focusTarget }) {
			r.focusTarget, r.notice, r.restoreFocus = "", i18n.TasksTargetMissing, true
		}
		if !r.checking {
			if r.notice == i18n.HistoryAmendmentStale && r.editRow != nil {
				if latest, _ := r.latestEdit(); latest == nil || !latest.Active() {
					r.editing, r.editRow, r.actionFocus, r.restoreFocus = false, nil, &r.close, true
					r.notice = i18n.TasksTargetMissing
				}
			}
			return
		}
		switch r.plan.verify(result.value.records, r.ack) {
		case reservationVerified:
			r.notice = i18n.ReserveSaved
			if r.plan.release != "" {
				r.notice = i18n.ReserveReleasedNotice
			}
			r.editing, r.editRow, r.restoreFocus = false, nil, true
			if r.plan.release != "" {
				r.actionFocus = &r.add
			}
			p.read()
		case reservationAbsent:
			r.notice = r.failure
		case reservationUncertain:
			r.readError = true
			return
		}
		r.checking, r.plan = false, nil
	default:
	}
}

func (r *fundsReservationSession) draftSource() *domain.LiquiditySourceResult {
	ref := r.source
	if r.editRow != nil {
		ref = r.editRow.Source
	}
	if ref.Kind != "" {
		return r.current.source(ref)
	}
	for i, row := range r.current.sources {
		if row.Ref.Key() == r.selected {
			return &r.current.sources[i]
		}
	}
	return nil
}

func (r *fundsReservationSession) startDraft(gtx layout.Context, row *domain.LiquidityReservation, trigger *widget.Clickable) {
	r.editing, r.editRow, r.notice, r.actionFocus = true, row, i18n.NoMessage, trigger
	r.label.Editor.SetText("")
	r.amount.Editor.SetText("")
	r.selected = ""
	if row != nil {
		r.label.Editor.SetText(row.Label)
		r.amount.Editor.SetText(row.Amount.CanonicalAmount())
	} else if r.source.Kind == "" {
		for _, source := range r.current.sources {
			if reservableFundsSource(&source) {
				if r.selected != "" {
					r.selected = ""
					break
				}
				r.selected = source.Ref.Key()
			}
		}
	}
	if row != nil || r.source.Kind != "" {
		r.label.Focus(gtx)
	}
}

// Keep the draft tied to the revision it originally reviewed. Loading another
// revision is an explicit action so a concurrent edit cannot be overwritten.
func (r *fundsReservationSession) latestEdit() (*domain.LiquidityReservation, bool) {
	if r.editRow == nil {
		return nil, false
	}
	at := slices.IndexFunc(r.current.records, func(row domain.LiquidityReservation) bool { return row.ID == r.editRow.ID })
	if at < 0 {
		return nil, true
	}
	latest := &r.current.records[at]
	return latest, !latest.Active() || latest.Revision != r.editRow.Revision
}

func (r *fundsReservationSession) reviewNew() {
	if _, changed := r.latestEdit(); changed {
		r.notice = i18n.HistoryAmendmentStale
		return
	}
	r.notice = i18n.NoMessage
	source := r.draftSource()
	var ref domain.LiquiditySourceRef
	var currency domain.CurrencyCode
	if r.editRow != nil {
		ref, currency = r.editRow.Source, r.editRow.Currency
	} else if reservableFundsSource(source) {
		ref, currency = source.Ref, source.NativeCurrency
	} else {
		r.notice = i18n.ReserveChooseSource
		return
	}
	label := strings.TrimSpace(r.label.Text())
	amount, err := domain.ParseMoney(strings.TrimSpace(r.amount.Text()), currency)
	switch {
	case label == "":
		r.notice = i18n.ReserveInvalidLabel
	case err != nil || amount.IsZero():
		r.notice = i18n.ReserveInvalidAmount
	default:
		plan := &fundsReservationPlan{
			source: ref, version: r.current.version, before: r.current.records, label: label, amount: amount,
		}
		if row := r.editRow; row != nil {
			if source != nil && row.Currency != source.NativeCurrency || !slices.ContainsFunc(r.current.records, func(current domain.LiquidityReservation) bool {
				return current.ID == row.ID && current.Active() && current.Revision == row.Revision
			}) {
				r.notice = i18n.HistoryAmendmentStale
				return
			}
			plan.edit, plan.revision = &row.ID, row.Revision
		}
		r.plan = plan
	}
}
