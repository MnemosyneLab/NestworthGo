package desktop

import (
	"fmt"
	"image"
	"slices"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var custodyTransferLabels = tradeLabels{
	action: i18n.CustodyOpen, review: i18n.CustodyReview, confirm: i18n.CustodyConfirm, posted: i18n.CustodyPosted,
	scope: i18n.CustodyUnavailable, history: i18n.CustodyStartRequired, empty: i18n.CustodyEmpty,
	previewFailed: i18n.CustodyPreviewFailed, stale: i18n.CustodyStale,
}

type positionScope struct {
	account domain.Account
	holding domain.Holding
	action  *widget.Clickable
}

type custodyTransferDraft struct {
	scope         positionScope
	incoming      bool
	send, receive widget.Clickable
	from          domain.HoldingID
	to            domain.AccountID
	fromPicker    ui.Select
	toPicker      ui.Select
	sources       *history.PositionSources
	loading       <-chan historyResult[history.PositionSources]
}

func (p *historyPage) positionActionsAvailable() bool {
	return p.investment != nil && p.reads != nil && p.reads.references != nil && p.reads.references.Origin != nil
}

func (p *historyPage) investmentRuntime() investmentRuntime {
	r := p.reads
	return investmentRuntime{ctx: r.ctx, workers: &r.workers, clock: r.clock, invalidate: r.invalidate}
}

func (f *accountInvestment) startCustody(run investmentRuntime, scope positionScope) {
	f.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, scope.account)
	f.draft.custody = &custodyTransferDraft{scope: scope, from: scope.holding.ID}
	f.loadSources(run)
}

func (f *accountInvestment) loadCustodySources(run investmentRuntime) {
	c := f.draft.custody
	c.sources = nil
	c.loading = runHistoryRead(run.workers, run.invalidate, func() (history.PositionSources, error) {
		return history.LoadPositionSources(run.ctx, f.service)
	})
}

func (f *accountInvestment) pollCustodySources(run investmentRuntime) {
	d := f.draft
	c := d.custody
	if c == nil {
		return
	}
	select {
	case result := <-c.loading:
		c.loading = nil
		if result.err != nil {
			d.sourceProblem = tradeProblem(result.err, custodyTransferLabels, i18n.CustodyPreviewFailed)
			return
		}
		c.sources = &result.value
		d.sources = &history.InvestmentSources{Origin: result.value.Origin, Location: result.value.Location}
		if d.day == "" {
			d.captureTime(run.clock())
		}
		if _, ok := c.source(); c.from != "" && !ok {
			c.from = ""
			d.problem = i18n.CustodyUnavailable
		}
		if c.to != "" && !slices.ContainsFunc(result.value.Accounts, func(a domain.Account) bool { return a.ID == c.to }) {
			c.to = ""
			d.problem = i18n.CustodyUnavailable
		}
		if c.scope.account.ID != "" && !c.scopeAvailable() {
			d.problem = i18n.CustodyUnavailable
		}
	default:
	}
}

func (c *custodyTransferDraft) source() (history.PositionHolding, bool) {
	if c.sources != nil {
		for _, item := range c.sources.Holdings {
			if item.Holding.ID == c.from && c.offersSource(item) {
				return item, true
			}
		}
	}
	return history.PositionHolding{}, false
}

func (c *custodyTransferDraft) scopeAvailable() bool {
	if c.sources == nil || !slices.ContainsFunc(c.sources.Accounts, func(a domain.Account) bool { return a.ID == c.scope.account.ID }) {
		return false
	}
	return c.scope.holding.ID == "" || slices.ContainsFunc(c.sources.Holdings, func(item history.PositionHolding) bool {
		return item.Holding.ID == c.scope.holding.ID && item.Account.ID == c.scope.account.ID
	})
}

func (c *custodyTransferDraft) offersSource(item history.PositionHolding) bool {
	if item.Holding.Quantity.IsZero() {
		return false
	}
	if c.scope.account.ID == "" {
		return true
	}
	if c.incoming {
		return item.Account.ID != c.scope.account.ID && (c.scope.holding.ID == "" || item.Instrument.ID == c.scope.holding.InstrumentID)
	}
	return item.Account.ID == c.scope.account.ID && (c.scope.holding.ID == "" || item.Holding.ID == c.scope.holding.ID)
}

func (c *custodyTransferDraft) available() bool {
	return c.sources != nil && len(c.sources.Accounts) >= 2 &&
		(c.scope.account.ID == "" || c.scopeAvailable()) && slices.ContainsFunc(c.sources.Holdings, c.offersSource)
}

func (d *accountInvestmentDraft) setCustodyDirection(incoming bool) {
	c := d.custody
	c.incoming, c.from, c.to = incoming, c.scope.holding.ID, ""
	if incoming {
		c.from, c.to = "", c.scope.account.ID
	}
	d.quantity.Editor.SetText("")
	d.problem = i18n.NoMessage
}

func (f *accountInvestment) previewCustody(run investmentRuntime) {
	d := f.draft
	if d.previewing != nil || d.sources == nil || !d.investmentInputComplete() {
		return
	}
	input := history.CustodyTransferInput{From: d.custody.from, To: d.custody.to,
		Quantity: d.quantity.Text(), Note: d.note.Text(), Date: d.day, Time: d.time}
	if d.custody.incoming && d.custody.scope.holding.ID != "" {
		input.To, input.ToHolding = "", d.custody.scope.holding.ID
	}
	input.EffectiveAt = d.effectiveAt()
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(run.workers, run.invalidate, func() (history.ReviewedInvestment, error) {
		return history.ReviewCustodyTransfer(run.ctx, f.service, input)
	})
}

func (f *accountInvestment) custodyFields(view ui.View, origin *image.Point) []layout.FlexChild {
	d, c := f.draft, f.draft.custody
	l := view.Language
	var fields []layout.FlexChild
	field := func(w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions { return fieldPosition(gtx, origin, w) })
	}
	static := func(s string) layout.FlexChild { return field(view.Message(s, view.Colors.Ink)) }
	if c.scope.account.ID != "" {
		fields = append(fields, static(c.scope.account.Name))
		fields = append(fields, field(func(gtx layout.Context) layout.Dimensions {
			if d.previewing != nil {
				gtx = gtx.Disabled()
			}
			if c.send.Clicked(gtx) && c.incoming {
				d.setCustodyDirection(false)
			}
			if c.receive.Clicked(gtx) && !c.incoming {
				d.setCustodyDirection(true)
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.TabButton(&c.send, l.Text(i18n.CustodySend), !c.incoming)),
				layout.Rigid(view.TabButton(&c.receive, l.Text(i18n.CustodyReceive), c.incoming)))
		}))
	}
	if !c.available() {
		return fields
	}
	source, selected := c.source()
	lockedSource := c.scope.holding.ID != "" && !c.incoming
	if lockedSource {
		fields = append(fields, static(fmt.Sprintf(l.Text(i18n.CustodyLockedHolding), custodyHoldingLabel(view, source))))
	} else {
		var from []ui.Choice
		for _, item := range c.sources.Holdings {
			if c.offersSource(item) {
				from = append(from, ui.Choice{ID: item.Holding.ID.String(), Label: custodyHoldingLabel(view, item)})
			}
		}
		fields = append(fields, field(func(gtx layout.Context) layout.Dimensions {
			if d.focusInput && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &c.fromPicker.Trigger})
				d.focusInput = false
			}
			next := any(&c.toPicker.Trigger)
			if c.incoming {
				next = &d.quantity.Editor
			}
			picker := ui.PickerField{Label: i18n.CustodyFromHolding, Origin: *origin, Disabled: d.previewing != nil, Next: next}
			c.fromPicker.Layout(gtx, view, picker, ui.SelectOptions{Items: from, Searchable: true}, c.from.String(), func(id string) {
				if c.from.String() != id {
					c.from = domain.HoldingID(id)
					d.quantity.Editor.SetText("")
					d.problem = i18n.NoMessage
					if item, ok := c.source(); ok && item.Account.ID == c.to {
						c.to = ""
					}
				}
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}))
	}
	if c.incoming {
		destination := c.scope.account.Name
		if c.scope.holding.ID != "" {
			for _, item := range c.sources.Holdings {
				if item.Holding.ID == c.scope.holding.ID {
					destination += " · " + item.Instrument.DisplayLabel()
					break
				}
			}
		}
		fields = append(fields, static(fmt.Sprintf(l.Text(i18n.CustodyToFact), destination)))
	} else {
		var to []ui.Choice
		for _, a := range c.sources.Accounts {
			if a.ID != source.Account.ID && a.ID != c.scope.account.ID {
				to = append(to, ui.Choice{ID: a.ID.String(), Label: a.Name})
			}
		}
		fields = append(fields, field(func(gtx layout.Context) layout.Dimensions {
			if lockedSource && d.focusInput && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &c.toPicker.Trigger})
				d.focusInput = false
			}
			picker := ui.PickerField{Label: i18n.CustodyToAccount, Origin: *origin, Disabled: !selected || d.previewing != nil,
				Next: &d.quantity.Editor}
			if !lockedSource {
				picker.Previous = &c.fromPicker.Trigger
			}
			c.toPicker.Layout(gtx, view, picker, ui.SelectOptions{Items: to, Searchable: true}, c.to.String(), func(id string) {
				c.to, d.problem = domain.AccountID(id), i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}))
	}
	return append(fields, field(func(gtx layout.Context) layout.Dimensions {
		return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.CustodyQuantity, Disabled: !selected || d.previewing != nil})
	}))
}

func custodyHoldingLabel(view ui.View, item history.PositionHolding) string {
	return item.Instrument.SelectionLabel() + " · " + item.Account.Name + " · " + portfolioDecimal(view, item.Holding.Quantity.Canonical())
}

func (c *custodyTransferDraft) reviewFacts(view ui.View, preview domain.ChangePreview, fact func(i18n.MessageID, string) layout.FlexChild) []layout.FlexChild {
	source, _ := c.source()
	destination := ""
	for _, a := range c.sources.Accounts {
		if a.ID == c.to {
			destination = a.Name
			break
		}
	}
	facts := []layout.FlexChild{
		fact(i18n.CustodyFromFact, source.Account.Name),
		fact(i18n.CustodyToFact, destination),
	}
	for _, effect := range preview.Effects {
		if effect.Role == domain.EffectRoleTransferFrom && effect.Quantity != nil {
			facts = append(facts, fact(i18n.AccountQuantity, portfolioDecimal(view, effect.Quantity.Canonical())))
		}
		if effect.HoldingID == nil {
			continue
		}
		for _, endpoint := range preview.Resulting {
			if endpoint.HoldingID == nil || *endpoint.HoldingID != *effect.HoldingID {
				continue
			}
			label := i18n.CustodySourceAfter
			if effect.Role == domain.EffectRoleTransferTo {
				label = i18n.CustodyDestinationAfter
			}
			facts = append(facts, fact(label, portfolioDecimal(view, endpoint.Quantity)))
		}
	}
	return append(facts, layout.Rigid(view.Message(view.Language.Text(i18n.CustodyNoCash), view.Colors.SecondaryInk)))
}
