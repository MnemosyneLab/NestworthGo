package desktop

import (
	"fmt"
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

const reconciliationSessionsPerPage = 25

type reconciliationSessionFilter int

const (
	reconciliationCompleted reconciliationSessionFilter = iota
	reconciliationAll
	reconciliationInProgress
	reconciliationPaused
)

var reconciliationSessionFilters = [...]struct {
	filter reconciliationSessionFilter
	label  i18n.MessageID
}{
	{reconciliationAll, i18n.ReconciliationAllSessions},
	{reconciliationInProgress, i18n.StatementInProgress},
	{reconciliationPaused, i18n.StatementPaused},
	{reconciliationCompleted, i18n.StatementCompleted},
}

func (f reconciliationSessionFilter) matches(session domain.ReconciliationSession) bool {
	switch f {
	case reconciliationCompleted:
		return session.Status == "completed"
	case reconciliationInProgress:
		return session.Status == "in_progress"
	case reconciliationPaused:
		return session.Status == "paused"
	default:
		return true
	}
}

type reconciliationListEntry struct {
	working, present bool
	scroll           int
	position         layout.Position
}

type reconciliationListVisit struct {
	filter   reconciliationSessionFilter
	page     int
	position layout.Position
	entry    reconciliationListEntry
}

type reconciliationSessionList struct {
	reconciliationListVisit
	working                 map[string]*widget.Clickable
	filters                 [len(reconciliationSessionFilters)]widget.Clickable
	previous, next          widget.Clickable
	scrollHistory, focusTab bool
	returned                string
}

func (p *tasksPage) holdingReconciliationRows(view ui.View) []layout.Widget {
	c, state, l := p.holdingRecon, &p.reconciliationList, view.Language
	if c == nil {
		return nil
	}
	if c.sessionButtons == nil {
		c.sessionButtons = make(map[string]*widget.Clickable)
	}
	if state.working == nil {
		state.working = make(map[string]*widget.Clickable)
	}

	rows := []layout.Widget{tradeTitle(view, i18n.ReconciliationSessions)}
	accountID := p.reconciliationAccount.String()
	if accountID != "" {
		if account, ok := c.directory.account(accountID); ok {
			rows = append(rows, view.Message(account.Name, view.Colors.SecondaryInk))
		}
	}
	var matching []domain.ReconciliationSession
	working := false
	for _, session := range c.directory.sessions {
		if accountID != "" && !slices.Contains(session.AccountIDs, accountID) {
			continue
		}
		if session.Status != "completed" {
			if session.ID == state.returned && state.entry.working {
				working = true
				c.restore = portfolioButton(state.working, session.ID)
			}
			rows = append(rows, p.reconciliationSessionCard(view, session, true))
		}
		if state.filter.matches(session) {
			matching = append(matching, session)
		}
	}
	_, selectedAccountActive := c.directory.account(accountID)
	if len(c.directory.accounts) != 0 && (accountID == "" || selectedAccountActive) {
		rows = append(rows, c.link(view, &c.start, l.Text(i18n.ReconciliationStart), func() {
			c.draft = newHoldingReconDraft(&c.start)
			c.draft.action = statement.Start
			c.draft.selection = &reconciliationSelection{checks: make(map[domain.AccountID]*widget.Bool)}
			if accountID != "" {
				c.draft.selection.freeze([]string{accountID})
			}
		}))
	}
	pages := max(1, (len(matching)+reconciliationSessionsPerPage-1)/reconciliationSessionsPerPage)
	state.page = min(state.page, pages-1)
	if state.returned != "" && !working {
		if at := slices.IndexFunc(matching, func(row domain.ReconciliationSession) bool { return row.ID == state.returned }); at >= 0 {
			page := at / reconciliationSessionsPerPage
			if state.entry.working || page != state.page {
				state.page = page
				p.list.Position = layout.Position{First: len(rows) + 2 + at%reconciliationSessionsPerPage}
			}
			c.restore = portfolioButton(c.sessionButtons, state.returned)
		} else {
			state.scrollHistory, state.focusTab = true, true
		}
	}
	state.returned = ""
	state.entry = reconciliationListEntry{}
	if state.scrollHistory {
		p.list.Position = layout.Position{First: len(rows)}
		state.scrollHistory = false
	}
	rows = append(rows, tradeTitle(view, i18n.ReconciliationHistoryTitle), p.reconciliationSessionStatus(view))
	if len(matching) == 0 {
		empty := i18n.ReconciliationHistoryNoMatches
		if state.filter == reconciliationAll {
			empty = i18n.ReconciliationHistoryEmpty
		}
		return append(rows, view.Message(l.Text(empty), view.Colors.SecondaryInk))
	}
	start := state.page * reconciliationSessionsPerPage
	for _, session := range matching[start:min(start+reconciliationSessionsPerPage, len(matching))] {
		rows = append(rows, p.reconciliationSessionCard(view, session, false))
	}
	if pages > 1 {
		rows = append(rows, p.reconciliationSessionPager(view, pages))
	}
	return rows
}

func (p *tasksPage) reconciliationSessionCard(view ui.View, session domain.ReconciliationSession, working bool) layout.Widget {
	c, state, l := p.holdingRecon, &p.reconciliationList, view.Language
	buttons := c.sessionButtons
	if working {
		buttons = state.working
	}
	button := portfolioButton(buttons, session.ID)
	name := strings.Join(session.AccountNames, " · ")
	status := l.Text(statementStatus(session))
	if session.Status == "completed" && session.Partial {
		status = l.Text(i18n.StatementCompleted) + " · " + status
	}
	when := statementRecordedTime(view, session.StartedAt)
	openLabel := i18n.ReconciliationOpen
	if len(session.AccountIDs) == 1 {
		if a, ok := c.directory.account(session.AccountIDs[0]); ok && a.TrackingMode == domain.TrackingHoldings {
			openLabel = i18n.HoldingReconOpen
		}
	}
	label, context := fmt.Sprintf(l.Text(openLabel), name), status+" · "+when
	fields := []layout.Widget{
		view.Message(context, view.Colors.SecondaryInk),
		func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				state.entry = reconciliationListEntry{working: working, present: true, scroll: view.PageScrollOffset(), position: p.list.Position}
				view.RestorePageScroll(0)
				p.openHoldingReconciliation(session.ID, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			c.restoreFocus(gtx, button)
			view.Focus.Watch(gtx, button)
			return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Title: label, Description: context,
			}))
		},
	}
	if working {
		fields = append(fields, holdingReconCounts(view, session))
		for _, row := range p.statements {
			if row.session.ID == session.ID && p.openStatement != nil {
				fields = append(fields, p.taskStatementAction(view, row, name))
				break
			}
		}
	}
	return fundsCard(view, name, fields...)
}

func (p *tasksPage) reconciliationSessionStatus(view ui.View) layout.Widget {
	state, l := &p.reconciliationList, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(reconciliationSessionFilters), func(gtx layout.Context, index int) layout.Dimensions {
			filter, button := reconciliationSessionFilters[index], &state.filters[index]
			if filter.filter == state.filter && state.focusTab && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: button})
				state.focusTab = false
			}
			if button.Clicked(gtx) && filter.filter != state.filter {
				state.filter, state.page = filter.filter, 0
				state.scrollHistory, state.focusTab = true, true
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.TabSegment(button, l.Text(filter.label), state.filter == filter.filter)(gtx)
		})
	}
}

func (p *tasksPage) reconciliationSessionPager(view ui.View, pages int) layout.Widget {
	state, l := &p.reconciliationList, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case state.page > 0 && state.previous.Clicked(gtx):
			state.page--
			state.scrollHistory, state.focusTab = true, true
			gtx.Execute(op.InvalidateCmd{})
		case state.page+1 < pages && state.next.Clicked(gtx):
			state.page++
			state.scrollHistory, state.focusTab = true, true
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(taskPagerButton(view, &state.previous, i18n.ReconciliationHistoryPrevious, state.page > 0)),
			layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.ReconciliationHistoryPage), state.page+1, pages), view.Colors.SecondaryInk)),
			layout.Rigid(taskPagerButton(view, &state.next, i18n.ReconciliationHistoryNext, state.page+1 < pages)),
		)
	}
}

func (p *tasksPage) backToReconciliationList(view ui.View) {
	c, state := p.holdingRecon, &p.reconciliationList
	id, entry, trigger := c.id, state.entry, c.sessionEntry
	c.discard()
	if entry.present {
		p.list.Position = entry.position
		view.RestorePageScroll(entry.scroll)
		state.returned = id
		return
	}
	if trigger == &c.start {
		c.restore = trigger
		return
	}
	state.scrollHistory, state.focusTab = true, true
}
