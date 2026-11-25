package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *tasksPage) holdingReconciliationRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	if c == nil || len(c.directory.accounts) == 0 {
		return nil
	}
	if c.sessionButtons == nil {
		c.sessionButtons = make(map[string]*widget.Clickable)
	}
	rows := []layout.Widget{tradeTitle(view, i18n.HoldingReconTitle)}
	for _, session := range c.directory.sessions {
		button := portfolioButton(c.sessionButtons, session.ID)
		name := strings.Join(session.AccountNames, " · ")
		status := l.Text(statementStatus(session))
		when := statementRecordedTime(view, session.StartedAt)
		rows = append(rows, fundsCard(view, name,
			view.Message(status+" · "+when, view.Colors.SecondaryInk),
			c.link(view, button, fmt.Sprintf(l.Text(i18n.HoldingReconOpen), name), func() {
				p.openHoldingReconciliation(session.ID, button)
			})))
	}
	return append(rows, c.link(view, &c.start, l.Text(i18n.HoldingReconStart), func() {
		c.draft = newHoldingReconDraft(&c.start)
		c.draft.action = statement.Start
	}))
}

func (c *holdingReconciliation) link(view ui.View, button *widget.Clickable, label string, open func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if c.restore == button {
			gtx.Execute(key.FocusCmd{Tag: button})
			c.restore = nil
		}
		return taskLink(view, button, label, func() {
			view.RestorePageScroll(0)
			open()
		})(gtx)
	}
}

func (p *tasksPage) layoutHoldingReconciliation(gtx layout.Context, view ui.View) layout.Dimensions {
	gtx, view = view.Measure(gtx, 690)
	c, l := p.holdingRecon, view.Language
	if c.draft != nil {
		return p.layoutHoldingReconDraft(gtx, view)
	}
	rows := []layout.Widget{
		c.link(view, &c.back, l.Text(i18n.HoldingReconBack), func() {
			entry := c.sessionEntry
			c.discard()
			c.restore = entry
		}),
		tradeTitle(view, i18n.HoldingReconTitle),
	}
	switch {
	case c.loading != nil:
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksLoading), view.Colors.SecondaryInk))
	case c.readFailed:
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksReadFailed), view.Colors.ErrorInk),
			c.link(view, &c.reload, l.Text(i18n.HistoryRetry), p.readHoldingReconciliation))
	case c.session != nil:
		rows = append(rows, p.holdingSessionRows(view)...)
	}
	return holdingReconColumn(gtx, rows)
}

func (p *tasksPage) holdingSessionRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	s := c.session
	rows := []layout.Widget{view.Message(l.Text(statementStatus(*s)), view.Colors.Ink)}
	if c.saved {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconSaved), view.Colors.Ink))
	}
	if s.Status == "paused" {
		rows = append(rows, c.link(view, &c.resume, l.Text(i18n.HoldingReconResume), func() {
			c.draft = newHoldingReconDraft(&c.resume)
			c.draft.action = statement.Resume
		}))
	}
	found := false
	for _, account := range s.Accounts {
		if !slices.ContainsFunc(c.directory.accounts, func(a domain.Account) bool { return a.ID.String() == account.AccountID }) {
			continue
		}
		rows = append(rows, view.Message(account.Name, view.Colors.Ink))
		for _, item := range account.Items {
			if item.Kind != "holding" || item.HoldingID == "" {
				continue
			}
			found = true
			fields := c.itemEvidence(view, item)
			if s.Status == "in_progress" && item.ProductID == "" && !item.ArchivedNonzero {
				at := account.AccountID + "/" + item.ID
				button := portfolioButton(c.itemButtons, at)
				fields = append(fields, c.link(view, button, l.Text(i18n.HoldingReconCorrect), func() {
					c.saved = false
					c.draft = newHoldingReconDraft(button)
					c.draft.accountID, c.draft.itemID = account.AccountID, item.ID
				}))
			}
			rows = append(rows, fundsCard(view, item.Label, fields...))
		}
	}
	if !found {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconEmpty), view.Colors.SecondaryInk))
	}
	return rows
}

func (c *holdingReconciliation) itemEvidence(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	l := view.Language
	basis := l.Text(i18n.OverviewNoValue)
	if item.Cost != "" {
		basis = portfolioDecimal(view, item.Cost) + " " + item.Currency
	}
	return []layout.Widget{
		view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, item.Value)), view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconBasis), basis), view.Colors.Ink),
	}
}

func (p *tasksPage) layoutHoldingReconDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	c, l := p.holdingRecon, view.Language
	d := c.draft
	if !c.unresolved() && d.cancel.Clicked(gtx) {
		c.draft, c.restore = nil, d.entry
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutHoldingReconReview(gtx, view)
	}
	title := i18n.HoldingReconCorrect
	switch d.action {
	case statement.Start:
		title = i18n.HoldingReconStart
	case statement.Resume:
		title = i18n.HoldingReconResume
	}
	origin := view.Origin
	rows := []layout.Widget{tradeTitle(view, title)}
	ready := c.loading == nil && !c.readFailed && d.reading == nil
	if d.itemID != "" {
		if a, item, ok := c.item(d); ok {
			rows = append(rows, view.Message(a.Name+" · "+item.Label, view.Colors.Ink))
			rows = append(rows, c.itemEvidence(view, item)...)
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				if d.initialFocus {
					gtx.Execute(key.FocusCmd{Tag: &d.total.Editor})
					d.initialFocus = false
				}
				return d.total.LayoutCurrency(gtx, view, ui.FieldOptions{
					Label: i18n.HoldingReconTotal, Disabled: !ready,
				}, item.Currency)
			})
		}
	} else if d.action == statement.Start {
		choices := make([]ui.Choice, 0, len(c.directory.accounts))
		for _, a := range c.directory.accounts {
			choices = append(choices, ui.Choice{ID: a.ID.String(), Label: a.Name})
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if d.initialFocus {
				gtx.Execute(key.FocusCmd{Tag: &d.accountPick.Trigger})
				d.initialFocus = false
			}
			d.accountPick.Layout(gtx, view, ui.PickerField{Label: i18n.AnalysisAccount, Origin: origin, Next: &d.note.Editor, Disabled: !ready},
				ui.SelectOptions{Items: choices, Searchable: true}, d.accountID, func(id string) {
					d.accountID, d.problem = id, i18n.NoMessage
				})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		})
	} else if c.session != nil {
		rows = append(rows, view.Message(holdingReconciliationAccountNames(c.session), view.Colors.Ink))
	}
	if d.action != statement.Resume {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Disabled: !ready})
		})
	}
	if d.problem != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(d.problem), view.Colors.ErrorInk))
	}
	if c.readFailed {
		rows = append(rows, c.link(view, &c.reload, l.Text(i18n.HistoryRetry), p.readHoldingReconciliation))
	}
	if !ready && !c.readFailed {
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksLoading), view.Colors.SecondaryInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		canReview := ready && c.canReview(d)
		if canReview && d.preview.Clicked(gtx) {
			p.previewHoldingReconciliation()
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(taskPagerButton(view, &d.cancel, i18n.InstitutionsCancel, true)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canReview {
					gtx = gtx.Disabled()
				}
				view.Focus.Watch(gtx, &d.preview)
				return view.PrimaryButton(&d.preview, l.Text(i18n.HoldingReconReview))(gtx)
			}))
	})
	// The same origin calculation places the picker correctly after scrolling.
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, row)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (p *tasksPage) layoutHoldingReconReview(gtx layout.Context, view ui.View) layout.Dimensions {
	c, l := p.holdingRecon, view.Language
	d := c.draft
	r := d.review
	rows := []layout.Widget{tradeTitle(view, i18n.HoldingReconReview)}
	if r.session != nil {
		rows = append(rows, view.Message(holdingReconciliationAccountNames(r.session), view.Colors.Ink),
			view.Message(l.Text(statementStatus(*r.session)), view.Colors.Ink))
	} else if a, item, ok := c.item(d); ok {
		rows = append(rows, view.Message(a.Name+" · "+item.Label, view.Colors.Ink))
		rows = append(rows, c.itemEvidence(view, item)...)
		for _, change := range r.changes {
			for _, effect := range change.Effects {
				if effect.Target == string(domain.EffectTargetHoldingCost) && effect.CostUnitPrice != nil {
					rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconNewBasis),
						portfolioDecimal(view, *effect.CostUnitPrice)+" "+item.Currency), view.Colors.Ink))
				}
			}
		}
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconReviewedTotal),
			strings.TrimSpace(d.total.Text())+" "+item.Currency), view.Colors.Ink))
	}
	if note := strings.TrimSpace(d.note.Text()); note != "" {
		rows = append(rows, view.Message(note, view.Colors.SecondaryInk))
	}
	if !c.unresolved() && d.edit.Clicked(gtx) {
		d.review, d.initialFocus = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.writing == nil && d.confirm.Clicked(gtx) {
		p.confirmHoldingReconciliation()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.uncertain {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconRetry), view.Colors.ErrorInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		if d.reviewFocus {
			gtx.Execute(key.FocusCmd{Tag: &d.confirm})
			d.reviewFocus = false
		}
		for _, b := range []*widget.Clickable{&d.cancel, &d.edit, &d.confirm} {
			if !c.unresolved() || (d.uncertain && d.writing == nil && b == &d.confirm) {
				view.Focus.Watch(gtx, b)
			}
		}
		return layoutTargetBalanceConfirmation(gtx, view, d.writing != nil, d.uncertain, &d.cancel, &d.edit, &d.confirm, i18n.HoldingReconConfirm)
	})
	return holdingReconColumn(gtx, rows)
}

func holdingReconciliationAccountNames(session *domain.ReconciliationSession) string {
	names := make([]string, 0, len(session.Accounts))
	for _, account := range session.Accounts {
		names = append(names, account.Name)
	}
	return strings.Join(names, " · ")
}

func holdingReconColumn(gtx layout.Context, rows []layout.Widget) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(row))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
