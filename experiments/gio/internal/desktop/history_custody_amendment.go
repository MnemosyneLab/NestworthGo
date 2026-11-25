package desktop

import (
	"fmt"
	"image"
	"slices"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type custodyAmendmentDraft struct {
	transfer            history.RecordedCustody
	from, to            domain.HoldingID
	note                ui.TextField
	source, destination ui.Select
	reload              widget.Clickable

	sources    history.PositionSources
	loading    <-chan historyResult[history.PositionSources]
	readFailed bool
}

func newCustodyAmendment(transfer history.RecordedCustody, note *string) *custodyAmendmentDraft {
	d := &custodyAmendmentDraft{transfer: transfer, from: *transfer.From.HoldingID, to: *transfer.To.HoldingID, note: ui.NewTextField(true)}
	if note != nil {
		d.note.Editor.SetText(*note)
	}
	return d
}

func (r *historyReads) loadCustodyAmendment(form *historyAmendmentDraft) {
	c := form.custody
	c.readFailed = false
	c.loading = runHistoryRead(&r.workers, r.invalidate, func() (history.PositionSources, error) {
		return history.LoadPositionSources(r.ctx, r.service)
	})
}

func (c *custodyAmendmentDraft) holding(id domain.HoldingID) (history.PositionHolding, bool) {
	index := slices.IndexFunc(c.sources.Holdings, func(item history.PositionHolding) bool { return item.Holding.ID == id })
	if index < 0 {
		return history.PositionHolding{}, false
	}
	return c.sources.Holdings[index], true
}

func (c *custodyAmendmentDraft) selected() bool {
	source, hasSource := c.holding(c.from)
	target, hasTarget := c.holding(c.to)
	return c.loading == nil && !c.readFailed && hasSource && hasTarget &&
		source.Account.ID != target.Account.ID && source.Instrument.ID == target.Instrument.ID
}

func (c *custodyAmendmentDraft) accountName(view ui.View, record *history.Detail, id domain.HoldingID) string {
	if item, ok := c.holding(id); ok {
		return item.Account.Name
	}
	if name := record.Accounts[record.HoldingAccounts[id]]; name != "" {
		return name
	}
	return view.Language.Text(i18n.OverviewAllocationUnknownAccount)
}

func (p *historyPage) layoutCustodyAmendment(gtx layout.Context, view ui.View, fix, busy bool, action, prepare, reviewTitle, confirm i18n.MessageID) layout.Dimensions {
	gtx, view = view.Measure(gtx, 640)
	d, l := p.reads.detail, view.Language
	form := d.amendment
	title := action
	if form.review != nil {
		title = reviewTitle
	}
	heading := func(label i18n.MessageID) layout.Widget {
		return view.Message(l.Text(label), view.Colors.SecondaryInk)
	}
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
		},
		heading(i18n.HistoryOriginalMovement),
	}
	transfer := form.custody.transfer
	rows = append(rows, custodyAmendmentFacts(view, d, []domain.ActivityEffect{transfer.From, transfer.To})...)
	at, description := d.record.Activity.EffectiveAt, i18n.HistoryCorrectDescription
	if !fix {
		description = i18n.HistoryReverseDescription
		if form.review != nil {
			at = form.review.Preview.Activity.EffectiveAt
		}
	}
	rows = append(rows, view.Message(contributionTimestamp(view, at, d.record.Location), view.Colors.SecondaryInk), heading(description))
	origin := view.Origin
	if form.review != nil {
		label := i18n.HistoryReversalMovement
		if fix {
			label = i18n.HistoryReplacementMovement
		}
		preview := form.review.Preview
		rows = append(rows, heading(label))
		rows = append(rows, custodyAmendmentFacts(view, d, preview.Effects)...)
		rows = append(rows, custodyAmendmentResults(view, d, fix, preview)...)
		if fix && preview.Activity.Note != nil && *preview.Activity.Note != "" {
			rows = append(rows, view.Message(*preview.Activity.Note, view.Colors.SecondaryInk))
		}
	} else if fix {
		rows = append(rows, custodyAmendmentFields(p.reads, form, view, &origin, busy)...)
	}
	if form.uncertain {
		rows = append(rows, view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk))
	} else if form.problem != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(form.problem), view.Colors.ErrorInk))
	}
	rows = append(rows, amendmentActions(view, form, fix, prepare, confirm))
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, row)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func custodyAmendmentFields(r *historyReads, form *historyAmendmentDraft, view ui.View, origin *image.Point, busy bool) []layout.Widget {
	c, l := form.custody, view.Language
	if c.loading != nil {
		return []layout.Widget{view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)}
	}
	if c.readFailed {
		return []layout.Widget{
			view.Message(l.Text(i18n.CustodyPreviewFailed), view.Colors.ErrorInk),
			taskLink(view, &c.reload, l.Text(i18n.HistoryRetry), func() { r.loadCustodyAmendment(form) }),
		}
	}
	if len(c.sources.Holdings) == 0 {
		return []layout.Widget{view.Message(l.Text(i18n.CustodyEmpty), view.Colors.SecondaryInk)}
	}
	choices := make([]ui.Choice, 0, len(c.sources.Holdings))
	var targets []ui.Choice
	source, haveSource := c.holding(c.from)
	for _, item := range c.sources.Holdings {
		choices = append(choices, ui.Choice{ID: item.Holding.ID.String(), Label: custodyHoldingLabel(view, item)})
		if haveSource && item.Account.ID != source.Account.ID && item.Instrument.ID == source.Instrument.ID {
			targets = append(targets, ui.Choice{ID: item.Holding.ID.String(), Label: item.Account.Name})
		}
	}
	sourceID, targetID := "", ""
	if haveSource {
		sourceID = c.from.String()
	}
	if slices.ContainsFunc(targets, func(choice ui.Choice) bool { return choice.ID == c.to.String() }) {
		targetID = c.to.String()
	}
	return []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			c.source.Layout(gtx, view, ui.PickerField{
				Label: i18n.CustodyFromHolding, Origin: *origin, Next: &c.destination.Trigger, Disabled: busy,
			}, ui.SelectOptions{Items: choices, Searchable: true}, sourceID, func(id string) {
				c.from = domain.HoldingID(id)
				if !c.selected() {
					c.to = ""
				}
				form.problem = i18n.NoMessage
				gtx.Execute(op.InvalidateCmd{})
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		},
		func(gtx layout.Context) layout.Dimensions {
			c.destination.Layout(gtx, view, ui.PickerField{
				Label: i18n.CustodyToAccount, Origin: *origin, Next: &form.amount.Editor, Disabled: busy || len(targets) == 0,
			}, ui.SelectOptions{Items: targets, Searchable: true}, targetID, func(id string) {
				c.to, form.problem = domain.HoldingID(id), i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		},
		func(gtx layout.Context) layout.Dimensions {
			if form.focus && !busy {
				form.amount.Focus(gtx)
				form.focus = false
			}
			return form.amount.Layout(gtx, view, ui.FieldOptions{Label: i18n.CustodyQuantity, Disabled: busy})
		},
		func(gtx layout.Context) layout.Dimensions {
			return c.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Disabled: busy})
		},
	}
}

func custodyAmendmentFacts(view ui.View, d *historyDetailVisit, effects []domain.ActivityEffect) []layout.Widget {
	l, c := view.Language, d.amendment.custody
	var rows []layout.Widget
	for _, leg := range effects {
		if leg.HoldingID == nil || leg.InstrumentID == nil || leg.Quantity == nil {
			continue
		}
		direction := i18n.HistoryAdded
		if leg.Direction == domain.EffectRemoved {
			direction = i18n.HistoryRemoved
		}
		label := c.accountName(view, d.record, *leg.HoldingID) + " · " + d.record.Instruments[*leg.InstrumentID]
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(direction), label, portfolioDecimal(view, leg.Quantity.Canonical())), view.Colors.Ink))
	}
	return rows
}

func custodyAmendmentResults(view ui.View, d *historyDetailVisit, fix bool, preview domain.ChangePreview) []layout.Widget {
	l := view.Language
	var rows []layout.Widget
	if fix {
		for _, endpoint := range preview.Review {
			if endpoint.Target == domain.EffectTargetHoldingQuantity {
				amount := fmt.Sprintf(l.Text(i18n.CustodyAmendmentBeforeAfter), portfolioDecimal(view, endpoint.Before), portfolioDecimal(view, endpoint.After))
				rows = append(rows, view.Message(endpoint.AccountName+" · "+amount, view.Colors.Ink))
			}
		}
	} else {
		for _, endpoint := range preview.Resulting {
			if endpoint.HoldingID != nil {
				amount := fmt.Sprintf(l.Text(i18n.CustodyAmendmentAfterReversal), portfolioDecimal(view, endpoint.Quantity))
				account := d.amendment.custody.accountName(view, d.record, *endpoint.HoldingID)
				rows = append(rows, view.Message(account+" · "+amount, view.Colors.Ink))
			}
		}
	}
	return rows
}
