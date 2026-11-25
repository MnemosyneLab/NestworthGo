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
	currency domain.CurrencyCode
	eligible bool
	day      domain.LiquidityBucketResult
	records  []domain.LiquidityReservation
	version  string
}

// The Gio slice uses the standard balance-mode bank account source. Its
// default currency is the only denomination accepted by the shared command.
func fundsReservationSource(account domain.Account, source domain.LiquiditySourceResult) bool {
	return account.ID != "" && account.ID == source.Ref.AccountID && account.ArchivedAt == nil &&
		account.AccountType == domain.TypeBankAccount && account.TrackingMode == domain.TrackingBalance && !account.IsLiability() &&
		!source.Excluded && source.Ref.Kind == domain.SourceAccountValue && source.NativeCurrency == account.DefaultCurrency
}

func readFundsReservation(ctx context.Context, service fundsReservationLedger, ref domain.LiquiditySourceRef) (out fundsReservationRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		// Read the funding projection and reservation identities in the same
		// operation gate, including released rows needed for verification.
		report, err := readAvailableFunds(ctx, service)
		if err != nil {
			return err
		}
		records, err := service.ListLiquidityReservations(ctx)
		if err != nil {
			return err
		}
		out.records = slices.DeleteFunc(records, func(row domain.LiquidityReservation) bool { return row.Source.Key() != ref.Key() })
		out.version = service.MobileDataVersion()
		account := report.accounts[ref.AccountID]
		out.currency = account.DefaultCurrency
		for _, row := range report.report.Sources {
			if row.Result.Ref.Key() != ref.Key() {
				continue
			}
			out.eligible = fundsReservationSource(account, row.Result)
			if at := slices.IndexFunc(row.Result.BucketResults, func(b domain.LiquidityBucketResult) bool {
				return b.HorizonOn == report.today.HorizonOn
			}); at >= 0 {
				out.day = row.Result.BucketResults[at]
			}
			break
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
				Source: p.source, Label: p.label, Amount: p.amount.CanonicalAmount(),
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
	source    domain.LiquiditySourceRef
	name      string
	entry     *widget.Clickable
	current   *fundsReservationRead
	read      <-chan historyResult[fundsReservationRead]
	write     <-chan historyResult[domain.LiquidityReservation]
	plan      *fundsReservationPlan
	checking  bool
	readError bool
	ack       domain.LiquidityReservationID
	notice    i18n.MessageID
	failure   i18n.MessageID

	editing                   bool
	label, amount             ui.TextField
	list                      widget.List
	close, add, review, retry widget.Clickable
	confirm                   widget.Clickable
	release                   map[domain.LiquidityReservationID]*widget.Clickable
}

func (p *availableFundsPage) openReservations(source domain.LiquiditySourceRef, name string, entry *widget.Clickable) {
	p.reservation = &fundsReservationSession{
		source: source, name: name, entry: entry, label: ui.NewTextField(true), amount: ui.NewTextField(true),
		release: make(map[domain.LiquidityReservationID]*widget.Clickable),
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
	ref := r.source
	r.readError = false
	r.read = runHistoryRead(&p.workers, p.invalidate, func() (fundsReservationRead, error) {
		return readFundsReservation(p.ctx, p.reservationLedger, ref)
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
		if !r.checking {
			return
		}
		switch r.plan.verify(result.value.records, r.ack) {
		case reservationVerified:
			r.notice = i18n.ReserveSaved
			if r.plan.release != "" {
				r.notice = i18n.ReserveReleasedNotice
			}
			r.editing, r.list.Position = false, layout.Position{}
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

func (r *fundsReservationSession) reviewNew() {
	r.notice = i18n.NoMessage
	label := strings.TrimSpace(r.label.Text())
	amount, err := domain.ParseMoney(strings.TrimSpace(r.amount.Text()), r.current.currency)
	switch {
	case label == "":
		r.notice = i18n.ReserveInvalidLabel
	case err != nil || amount.IsZero():
		r.notice = i18n.ReserveInvalidAmount
	default:
		r.plan = &fundsReservationPlan{
			source: r.source, version: r.current.version, before: r.current.records, label: label, amount: amount,
		}
	}
}
