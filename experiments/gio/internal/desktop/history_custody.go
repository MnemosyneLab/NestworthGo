package desktop

import (
	"image"
	"slices"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

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

type custodyTransferDraft struct {
	from       domain.HoldingID
	to         domain.AccountID
	fromPicker ui.Select
	toPicker   ui.Select
	sources    *history.CustodySources
	loading    <-chan historyResult[history.CustodySources]
}

func (p *historyPage) custodyAvailable() bool {
	return p.investment != nil && p.reads != nil && p.reads.references != nil && p.reads.references.Origin != nil
}

func (p *historyPage) investmentRuntime() investmentRuntime {
	r := p.reads
	return investmentRuntime{ctx: r.ctx, workers: &r.workers, clock: r.clock, invalidate: r.invalidate}
}

func (f *accountInvestment) startCustody(run investmentRuntime) {
	f.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, domain.Account{})
	f.draft.custody = new(custodyTransferDraft)
	f.loadSources(run)
}

func (f *accountInvestment) loadCustodySources(run investmentRuntime) {
	c := f.draft.custody
	c.sources = nil
	c.loading = runHistoryRead(run.workers, run.invalidate, func() (history.CustodySources, error) {
		return history.LoadCustodySources(run.ctx, f.service)
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
	default:
	}
}

func (c *custodyTransferDraft) source() (history.CustodyHolding, bool) {
	if c.sources != nil {
		for _, item := range c.sources.Holdings {
			if item.Holding.ID == c.from {
				return item, true
			}
		}
	}
	return history.CustodyHolding{}, false
}

func (f *accountInvestment) previewCustody(run investmentRuntime) {
	d := f.draft
	if d.previewing != nil || d.sources == nil || !d.investmentInputComplete() {
		return
	}
	input := history.CustodyTransferInput{From: d.custody.from, To: d.custody.to,
		Quantity: d.quantity.Text(), Note: d.note.Text(), Date: d.day, Time: d.time}
	if d.day == d.defaultTime.Format(time.DateOnly) && d.time == d.defaultTime.Format(time.TimeOnly) {
		input.EffectiveAt = d.defaultTime
	}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(run.workers, run.invalidate, func() (history.ReviewedInvestment, error) {
		return history.ReviewCustodyTransfer(run.ctx, f.service, input)
	})
}

func (f *accountInvestment) custodyFields(view ui.View, origin *image.Point) []layout.FlexChild {
	d, c := f.draft, f.draft.custody
	if !d.investmentFieldsVisible() {
		return nil
	}
	from := make([]ui.Choice, 0, len(c.sources.Holdings))
	for _, item := range c.sources.Holdings {
		label := item.Instrument.SelectionLabel() + " · " + item.Account.Name + " · " + portfolioDecimal(view, item.Holding.Quantity.Canonical())
		from = append(from, ui.Choice{ID: item.Holding.ID.String(), Label: label})
	}
	source, selected := c.source()
	to := make([]ui.Choice, 0, len(c.sources.Accounts))
	for _, a := range c.sources.Accounts {
		if a.ID != source.Account.ID {
			to = append(to, ui.Choice{ID: a.ID.String(), Label: a.Name})
		}
	}
	return []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, func(gtx layout.Context) layout.Dimensions {
				if d.focusInput && gtx.Enabled() {
					gtx.Execute(key.FocusCmd{Tag: &c.fromPicker.Trigger})
					d.focusInput = false
				}
				field := ui.PickerField{Label: i18n.CustodyFromHolding, Origin: *origin, Disabled: d.previewing != nil, Next: &c.toPicker.Trigger}
				c.fromPicker.Layout(gtx, view, field, ui.SelectOptions{Items: from, Searchable: true}, c.from.String(), func(id string) {
					if c.from.String() == id {
						return
					}
					c.from = domain.HoldingID(id)
					d.quantity.Editor.SetText("")
					d.problem = i18n.NoMessage
					if item, ok := c.source(); ok && item.Account.ID == c.to {
						c.to = ""
					}
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, func(gtx layout.Context) layout.Dimensions {
				field := ui.PickerField{Label: i18n.CustodyToAccount, Origin: *origin, Disabled: !selected || d.previewing != nil,
					Previous: &c.fromPicker.Trigger, Next: &d.quantity.Editor}
				c.toPicker.Layout(gtx, view, field, ui.SelectOptions{Items: to, Searchable: true}, c.to.String(), func(id string) {
					c.to, d.problem = domain.AccountID(id), i18n.NoMessage
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, func(gtx layout.Context) layout.Dimensions {
				return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.CustodyQuantity, Disabled: !selected || d.previewing != nil})
			})
		}),
	}
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
