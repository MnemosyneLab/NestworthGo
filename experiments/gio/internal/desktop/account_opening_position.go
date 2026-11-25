package desktop

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountOpeningPosition struct {
	service history.OpeningPositionLedger
	open    widget.Clickable
	draft   *openingPositionDraft
	saved   domain.AccountID
	onSaved func()
}

type openingPositionDraft struct {
	account            domain.Account
	sources            []domain.Instrument
	instrument         domain.InstrumentID
	quantity           ui.TextField
	picker             ui.Select
	cancel, edit, next widget.Clickable
	confirm, retry     widget.Clickable
	loading            <-chan historyResult[history.OpeningPositionSources]
	preview            <-chan historyResult[history.OpeningPositionReview]
	writing            <-chan historyResult[domain.Holding]
	read               <-chan historyResult[bool]
	checking           bool
	saved              domain.HoldingID
	review             *history.OpeningPositionReview
	problem            i18n.MessageID
}

func (p *accountsPage) canOpenPosition() bool {
	row := p.freshDetail()
	return p.opening != nil && row != nil && p.current.beforeHistory && history.TradeAccount(row.record.Account)
}

func (p *accountsPage) openingPositionUnresolved() bool {
	if p.opening == nil || p.opening.draft == nil {
		return false
	}
	d := p.opening.draft
	return d.writing != nil || d.checking
}

func (p *accountsPage) loadOpeningPosition(d *openingPositionDraft) {
	id, service := d.account.ID, p.opening.service
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.OpeningPositionSources, error) {
		return history.LoadOpeningPositionSources(p.ctx, service, id)
	})
}

func (p *accountsPage) verifyOpeningPosition(d *openingPositionDraft) {
	plan, id, service := *d.review, d.saved, p.opening.service
	d.read = runHistoryRead(&p.workers, p.invalidate, func() (bool, error) {
		return plan.Verify(p.ctx, service, id)
	})
}

func (p *accountsPage) pollOpeningPosition() {
	f := p.opening
	if f == nil || f.draft == nil {
		return
	}
	d := f.draft
	select {
	case r := <-d.loading:
		d.loading = nil
		if r.err != nil {
			d.problem = openingPositionProblem(r.err, i18n.OpeningPositionLoadFailed)
		} else {
			d.account, d.sources = r.value.Account, r.value.Instruments
			if !slices.ContainsFunc(d.sources, func(i domain.Instrument) bool { return i.ID == d.instrument }) {
				d.instrument = ""
			}
			if len(d.sources) == 1 && d.instrument == "" {
				d.instrument = d.sources[0].ID
			}
		}
	default:
	}
	select {
	case r := <-d.preview:
		d.preview = nil
		if r.err != nil {
			d.problem = openingPositionProblem(r.err, i18n.OpeningPositionLoadFailed)
			if d.problem == i18n.OpeningPositionUnavailable {
				p.loadOpeningPosition(d)
			}
		} else {
			d.review = &r.value
		}
	default:
	}
	select {
	case r := <-d.writing:
		d.writing = nil
		d.problem = openingPositionProblem(r.err, i18n.OpeningPositionNoSave)
		if d.problem == i18n.HistoryAmendmentStale || d.problem == i18n.HistoryStartExists {
			d.review = nil
			p.loadOpeningPosition(d)
		} else {
			d.saved, d.checking = r.value.ID, true
			p.verifyOpeningPosition(d)
		}
	default:
	}
	select {
	case r := <-d.read:
		d.read = nil
		if r.err != nil && openingPositionProblem(r.err, i18n.NoMessage) == i18n.OpeningPositionUnavailable {
			d.checking, d.review, d.problem = false, nil, i18n.OpeningPositionUnavailable
			p.loadOpeningPosition(d)
			return
		}
		if r.err != nil || (d.saved != "" && !r.value) {
			return
		}
		d.checking, d.review = false, nil
		if r.value {
			f.saved, f.draft = d.account.ID, nil
			if f.onSaved != nil {
				f.onSaved()
			} else {
				p.refresh()
			}
		} else {
			d.problem = i18n.OpeningPositionNoSave
			p.loadOpeningPosition(d)
		}
	default:
	}
}

func openingPositionProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	if err == nil {
		return i18n.NoMessage
	}
	var issue *domain.Error
	if errors.As(err, &issue) {
		switch {
		case issue.Code == domain.ErrStalePreview:
			return i18n.HistoryAmendmentStale
		case issue.Field == "historyOrigin":
			return i18n.HistoryStartExists
		case issue.Field == "openingHoldings":
			return i18n.HistoryStartHoldingCost
		case issue.Field == "quantity":
			return i18n.BuyInvalidQuantity
		case issue.Field == "instrumentId" || issue.Field == "holdingId":
			return i18n.OpeningPositionUnavailable
		}
	}
	return fallback
}

func (p *accountsPage) layoutOpeningPosition(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.opening, view.Language
	d := f.draft
	gtx, view = view.Measure(gtx, 690)
	if d.checking {
		if d.read != nil {
			return view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)(gtx)
		}
		if d.retry.Clicked(gtx) {
			p.verifyOpeningPosition(d)
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.OpeningPositionReadFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
	}
	if d.writing != nil {
		return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
	}
	if d.cancel.Clicked(gtx) {
		f.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &f.open})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		if d.edit.Clicked(gtx) {
			d.review = nil
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
		plan := *d.review
		if d.confirm.Clicked(gtx) {
			d.writing = runHistoryRead(&p.workers, p.invalidate, func() (domain.Holding, error) {
				return plan.Confirm(p.ctx, f.service)
			})
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(tradeTitle(view, i18n.OpeningPositionReview)),
			layout.Rigid(view.Message(plan.Account.Name+" · "+plan.Instrument.SelectionLabel(), view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, plan.Quantity.Canonical())), view.Colors.Ink)),
			layout.Rigid(view.Message(l.Text(i18n.OpeningPositionHelp), view.Colors.SecondaryInk)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutTargetBalanceConfirmation(gtx, view, d.writing != nil, false, &d.cancel, &d.edit, &d.confirm, i18n.OpeningPositionConfirm)
			}))
	}

	if d.loading == nil && d.preview == nil && d.next.Clicked(gtx) {
		id, instrument, quantity := d.account.ID, d.instrument, d.quantity.Text()
		d.problem = i18n.NoMessage
		d.preview = runHistoryRead(&p.workers, p.invalidate, func() (history.OpeningPositionReview, error) {
			return history.PreviewOpeningPosition(p.ctx, f.service, id, instrument, quantity)
		})
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.loading == nil && d.retry.Clicked(gtx) {
		d.problem = i18n.NoMessage
		p.loadOpeningPosition(d)
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, tradeTitle(view, i18n.OpeningPositionOpen))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(d.account.Name+" · "+d.account.DefaultCurrency.String(), view.Colors.Ink))
		}),
	}
	if d.loading == nil && len(d.sources) > 0 && d.problem != i18n.HistoryStartExists {
		items := make([]ui.Choice, 0, len(d.sources))
		for _, i := range d.sources {
			items = append(items, ui.Choice{ID: i.ID.String(), Label: i.SelectionLabel()})
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				field := ui.PickerField{Label: i18n.BuyInstrument, Origin: origin, Next: &d.quantity.Editor, Disabled: d.preview != nil}
				d.picker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.instrument.String(), func(id string) {
					d.instrument, d.problem = domain.InstrumentID(id), i18n.NoMessage
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.OpeningPositionQuantity, Disabled: d.preview != nil})
			}))
	}
	children = append(children, layout.Rigid(view.Message(l.Text(i18n.OpeningPositionHelp), view.Colors.SecondaryInk)))
	switch {
	case d.loading != nil || d.preview != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case d.problem != i18n.NoMessage:
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	case len(d.sources) == 0:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.OpeningPositionEmpty), view.Colors.SecondaryInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		switch {
		case d.problem == i18n.OpeningPositionLoadFailed:
			buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
		case len(d.sources) > 0 && d.problem != i18n.HistoryStartExists:
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.loading != nil || d.preview != nil || d.instrument == "" || strings.TrimSpace(d.quantity.Text()) == "" {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.next, l.Text(i18n.OpeningPositionReview))(gtx)
			}))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
