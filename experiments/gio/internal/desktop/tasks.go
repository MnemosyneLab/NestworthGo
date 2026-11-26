package desktop

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type upcomingActionsReader interface {
	UpcomingActions(context.Context) (domain.UpcomingActions, error)
}

type tasksTab int

const (
	tasksUpcoming tasksTab = iota
	tasksInvestment
	tasksReconciliation
)

var taskTabLabels = [...]i18n.MessageID{i18n.TasksUpcomingTab, i18n.TasksInvestmentTab, i18n.TasksReconciliationTab}

type tasksVisit struct {
	tab                          tasksTab
	showEligible                 bool
	upcomingFilter               taskUpcomingFilter
	upcomingLimit                int
	position                     layout.Position
	reviewCursors                []string
	reviewGeneration             uint64
	reconciliationAccount        domain.AccountID
	reconciliationList           reconciliationListVisit
	holdingSession               string
	holdingReceipt               domain.ActivityID
	holdingAudit                 []domain.ActivityID
	holdingAttempts              []string
	linkedJournal, recentJournal reconciliationJournalVisit
	journalReceipt               reconciliationReceiptKey
}

type tasksRead struct {
	upcoming    *domain.UpcomingActions
	review      *domain.ReviewAlertPage
	reviewKnown map[domain.ReviewEpisodeID]taskReviewKnown
	statements  []taskStatement
	holdings    holdingReconDirectory
}

type taskLinks struct {
	product, receipt widget.Clickable
}

type tasksPage struct {
	ctx            context.Context
	service        upcomingActionsReader
	invalidate     func()
	workers        sync.WaitGroup
	cancel         context.CancelFunc
	request        <-chan historyResult[tasksRead]
	current        *domain.UpcomingActions
	err            error
	list           widget.List
	reload         widget.Clickable
	links          map[string]*taskLinks
	openProduct    func(domain.ProductContractID, string, event.Tag)
	upcomingLinks  upcomingActionLinks
	showEligible   bool
	eligible       widget.Clickable
	upcomingHealth widget.Clickable
	upcomingFilter taskUpcomingFilter
	upcomingLimit  int
	upcomingTabs   [len(taskUpcomingFilters)]widget.Clickable
	upcomingMore   widget.Clickable
	verify         func(domain.ProductContractID, string, event.Tag)
	checks         taskChecksReader
	tab            tasksTab
	tabs           [len(taskTabLabels)]widget.Clickable
	taskBadges     taskCategoryBadges
	review         *domain.ReviewAlertPage
	reviewKnown    map[domain.ReviewEpisodeID]taskReviewKnown
	reviewCursors  []string
	reviewAck      taskReviewAcknowledgement
	statements     []taskStatement
	instruments    map[domain.InstrumentID]*widget.Clickable
	statementLinks map[string]*widget.Clickable
	previous, next widget.Clickable
	openInstrument func(domain.InstrumentID, string, event.Tag)
	openStatement  openTaskStatement
	openActivity   func(domain.ActivityID, event.Tag)
	holdingRecon   *holdingReconciliation

	reconciliationAccount domain.AccountID
	reconciliationList    reconciliationSessionList
}

func newTasksPage(ctx context.Context, service upcomingActionsReader, invalidate func()) *tasksPage {
	return &tasksPage{ctx: ctx, service: service, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, links: make(map[string]*taskLinks),
		instruments: make(map[domain.InstrumentID]*widget.Clickable), statementLinks: make(map[string]*widget.Clickable)}
}

func (p *tasksPage) enter(visit tasksVisit, scroll int) event.Tag {
	p.tab, p.list.Position, p.showEligible = visit.tab, visit.position, visit.showEligible
	p.reconciliationAccount = visit.reconciliationAccount
	p.reconciliationList.reconciliationListVisit = visit.reconciliationList
	p.reconciliationList.returned = ""
	p.reconciliationList.scrollHistory, p.reconciliationList.focusTab = false, false
	p.upcomingFilter, p.upcomingLimit = visit.upcomingFilter, visit.upcomingLimit
	p.reviewCursors = nil
	if visit.reviewGeneration == p.reviewAck.generation {
		p.reviewCursors = slices.Clone(visit.reviewCursors)
	} else if len(visit.reviewCursors) > 0 {
		p.list.Position = layout.Position{}
	}
	p.read()
	if c := p.holdingRecon; c != nil && visit.holdingSession != "" {
		if c.sessionButtons == nil {
			c.sessionButtons = make(map[string]*widget.Clickable)
		}
		buttons := c.sessionButtons
		if visit.reconciliationList.entry.working {
			if p.reconciliationList.working == nil {
				p.reconciliationList.working = make(map[string]*widget.Clickable)
			}
			buttons = p.reconciliationList.working
		}
		p.openHoldingReconciliation(visit.holdingSession, portfolioButton(buttons, visit.holdingSession))
		// The loading placeholder cannot keep the session's previous offset.
		// Restore it when the receipt and its preceding evidence are drawn.
		c.editScroll, c.restoreEditScroll = scroll, scroll != 0
		for _, id := range visit.holdingAudit {
			c.audit[id] = &holdingEvidenceDisclosure{open: true}
		}
		for _, item := range visit.holdingAttempts {
			c.itemButtons[item] = &holdingReconItemActions{attempts: holdingEvidenceDisclosure{open: true}}
		}
		c.linkedJournal.reconciliationJournalVisit, c.recentJournal.reconciliationJournalVisit = visit.linkedJournal, visit.recentJournal
		if visit.journalReceipt.operation != "" {
			c.journalReturn = visit.journalReceipt
			c.restore = nil
			return c.journalReceiptButton(visit.journalReceipt)
		}
		if visit.holdingReceipt != "" {
			c.restore = nil
			return portfolioButton(c.receiptButtons, visit.holdingReceipt)
		}
	}
	return nil
}

func (p *tasksPage) visit(focus event.Tag) tasksVisit {
	visit := tasksVisit{
		tab: p.tab, position: p.list.Position, showEligible: p.showEligible, reviewCursors: slices.Clone(p.reviewCursors),
		reconciliationAccount: p.reconciliationAccount,
		reconciliationList:    p.reconciliationList.reconciliationListVisit,
		reviewGeneration:      p.reviewAck.generation,
		upcomingFilter:        p.upcomingFilter, upcomingLimit: p.upcomingLimit,
	}
	if c := p.holdingRecon; c != nil && c.id != "" {
		visit.holdingSession = c.id
		visit.linkedJournal, visit.recentJournal = c.linkedJournal.reconciliationJournalVisit, c.recentJournal.reconciliationJournalVisit
		for id, button := range c.journalReceipts {
			if focus == button {
				visit.journalReceipt = id
				break
			}
		}
		for id, state := range c.audit {
			if state.open {
				visit.holdingAudit = append(visit.holdingAudit, id)
			}
		}
		for id, state := range c.itemButtons {
			if state.attempts.open {
				visit.holdingAttempts = append(visit.holdingAttempts, id)
			}
		}
		for id, button := range c.receiptButtons {
			if focus == button {
				visit.holdingReceipt = id
				break
			}
		}
	}
	return visit
}

func (p *tasksPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *tasksPage) read() {
	p.leave()
	p.err = nil
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	tab := p.tab
	includeHoldings := p.holdingRecon != nil
	cursor := ""
	if len(p.reviewCursors) != 0 {
		cursor = p.reviewCursors[len(p.reviewCursors)-1]
	}
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (tasksRead, error) {
		switch tab {
		case tasksInvestment:
			page, err := p.checks.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "pending"}, cursor, 50)
			if err != nil {
				return tasksRead{}, err
			}
			return tasksRead{review: &page, reviewKnown: readTaskReviewKnown(ctx, p.checks, page)}, nil
		case tasksReconciliation:
			sessions, holdings, err := readTaskReconciliation(ctx, p.checks, includeHoldings)
			return tasksRead{statements: sessions, holdings: holdings}, err
		default:
			actions, err := p.service.UpcomingActions(ctx)
			return tasksRead{upcoming: &actions}, err
		}
	})
}

func (p *tasksPage) update() {
	p.updateTaskBadges()
	select {
	case result := <-p.request:
		p.leave()
		p.err, p.current, p.review, p.reviewKnown, p.statements = result.err, nil, nil, nil, nil
		if p.err != nil {
			return
		}
		p.review, p.reviewKnown, p.statements = result.value.review, result.value.reviewKnown, result.value.statements
		if p.tab == tasksReconciliation && p.holdingRecon != nil {
			p.holdingRecon.directory = result.value.holdings
		}
		actions := result.value.upcoming
		if actions == nil {
			return
		}
		p.current = actions
		p.upcomingLinks.retained(actions.Items)
		present := make(map[string]bool, len(actions.Items))
		for _, row := range actions.Items {
			present[row.ID] = true
		}
		for id := range p.links {
			if !present[id] {
				delete(p.links, id)
			}
		}
	default:
	}
}

func (p *tasksPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	if p.holdingRecon.active() {
		if p.startReconciliationDraft() == nil {
			p.pollHoldingReconciliation()
		}
		return p.layoutHoldingReconciliation(gtx, view)
	}
	if p.checks != nil {
		for i := range p.tabs {
			if p.tabs[i].Clicked(gtx) && p.tab != tasksTab(i) {
				if p.tab == tasksReconciliation {
					p.reconciliationList.position = p.list.Position
				}
				visit := tasksVisit{tab: tasksTab(i), showEligible: p.showEligible,
					upcomingFilter: p.upcomingFilter, upcomingLimit: p.upcomingLimit,
					reconciliationAccount: p.reconciliationAccount,
					reconciliationList:    p.reconciliationList.reconciliationListVisit}
				if visit.tab == tasksReconciliation {
					visit.position = visit.reconciliationList.position
				}
				p.enter(visit, 0)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	if p.request == nil && p.reload.Clicked(gtx) {
		p.invalidateTaskBadges()
		if p.tab == tasksInvestment && len(p.reviewCursors) > 0 {
			p.enter(tasksVisit{tab: p.tab}, 0)
		} else {
			p.read()
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	l := view.Language
	reload := i18n.HistoryRefresh
	if p.err != nil {
		reload = i18n.HistoryRetry
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if p.checks == nil {
						return tradeTitle(view, i18n.TasksUpcomingActions)(gtx)
					}
					return view.TabTrack(gtx, len(p.tabs), func(gtx layout.Context, i int) layout.Dimensions {
						return p.taskTabButton(gtx, view, tasksTab(i))
					})
				}),
				layout.Rigid(taskPagerButton(view, &p.reload, reload, p.request == nil)))
		}),
	}
	if p.tab == tasksInvestment && p.reviewAck.status != i18n.NoMessage {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutReviewAcknowledgement(gtx, view) }))
	}
	rows = append(rows,
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			loading, failed := i18n.TasksLoading, i18n.TasksReadFailed
			if p.tab != tasksUpcoming {
				loading, failed = i18n.TasksChecksLoading, i18n.TasksChecksReadFailed
			}
			switch {
			case p.request != nil:
				return view.Message(l.Text(loading), view.Colors.SecondaryInk)(gtx)
			case p.err != nil:
				return view.Message(l.Text(failed), view.Colors.ErrorInk)(gtx)
			}
			var rows []layout.Widget
			switch p.tab {
			case tasksInvestment:
				if p.review != nil {
					rows = p.reviewRows(view)
				}
			case tasksReconciliation:
				if p.holdingRecon == nil {
					rows = p.statementRows(view)
				} else {
					rows = p.holdingReconciliationRows(view)
				}
			default:
				if p.current != nil {
					rows = p.rows(view)
				}
			}
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return pageListGap(gtx, view, &p.list, l.Text(i18n.NavTasks), len(rows), ui.Space12, func(gtx layout.Context, i int) layout.Dimensions {
				return rows[i](gtx)
			})
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutReviewPager(gtx, view) }))
	return ui.Column{Gap: 10}.Layout(gtx, rows...)
}

func (p *tasksPage) rows(view ui.View) []layout.Widget {
	l := view.Language
	data := p.current
	window := fmt.Sprintf(l.Text(i18n.TasksWindow), historyDay(view, data.WindowStartOn), historyDay(view, data.WindowEndOn), data.Timezone)
	var rows []layout.Widget
	if p.checks != nil {
		rows = append(rows, tradeTitle(view, i18n.TasksUpcomingActions))
	}
	rows = append(rows, view.Body(ui.TextCaption, window, view.Colors.SecondaryInk))
	if zone, err := time.LoadLocation(data.Timezone); err == nil && !data.AsOf.IsZero() {
		readAt := format.LocalDateTime(data.AsOf.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(l))
		rows = append(rows, view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.TasksReadAt), readAt), view.Colors.SecondaryInk))
	}
	rows = append(rows, p.upcomingFilterTabs(view))
	rows = p.upcomingRows(view, rows, data.Items)
	if data.Counts.Eligible > 0 {
		label := fmt.Sprintf(l.Text(i18n.TasksEligible), data.Counts.Eligible)
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if p.eligible.Clicked(gtx) {
				p.showEligible = !p.showEligible
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, &p.eligible)
			return analysisDisclosure(view, &p.eligible, label, p.showEligible)(gtx)
		})
		if p.showEligible {
			for _, row := range data.Items {
				if row.Group == "eligible" {
					rows = append(rows, p.card(view, row))
				}
			}
		}
	}
	rows = append(rows, p.upcomingDataMaintenance(view, data.Counts))
	return rows
}

func (p *tasksPage) card(view ui.View, row domain.UpcomingAction) layout.Widget {
	l := view.Language
	phase := taskPhase(row)
	tone := ui.BadgeAccent
	switch phase {
	case i18n.TasksOverdue, i18n.FundsDue, i18n.TasksBlocked:
		tone = ui.BadgeWarning
	}
	fields := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(phase)) })
	}}
	if row.AccountName != nil && *row.AccountName != "" {
		fields = append(fields, view.Message(*row.AccountName, view.Colors.SecondaryInk))
	}
	if row.MaturityOn != nil {
		fields = append(fields, productMaturity(view, row.MaturityOn))
	}
	if row.EligibilityOn != nil && row.MaturityOn == nil {
		eligibility := fmt.Sprintf(l.Text(i18n.TasksEligibilityOn), historyDay(view, *row.EligibilityOn))
		fields = append(fields, view.Message(eligibility, view.Colors.SecondaryInk))
	}
	receipt := l.Text(i18n.OverviewNoValue)
	if row.EstimatedReceiptOn != nil {
		receipt = historyDay(view, *row.EstimatedReceiptOn)
	}
	if row.Kind == "product_schedule" || row.EstimatedReceiptOn != nil {
		fields = append(fields, view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptDate), receipt), view.Colors.SecondaryInk))
	}
	if row.RequestedReserve != nil {
		fields = append(fields, fundsAmount(view, i18n.FundsRequested, row.RequestedReserve))
	}
	amountLabel := upcomingAmountLabel(row.AmountRole)
	if row.AmountRole == "estimated_net_proceeds" && row.Amount != nil && row.AmountStatus == "partial" {
		amountLabel = i18n.TasksPartialEstimate
	}
	fields = append(fields, fundsAmount(view, amountLabel, row.Amount))
	if slices.Contains(row.Assumptions, string(domain.AssumptionWeekdaysNoHolidays)) {
		fields = append(fields, view.Message(l.Text(i18n.TasksWeekdays), view.Colors.SecondaryInk))
	}
	var labels []i18n.MessageID
	for _, reason := range row.ReasonCodes {
		label := taskReason(reason)
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
			fields = append(fields, view.Message(l.Text(label), view.Colors.SecondaryInk))
		}
	}
	if id := taskProductID(row); id != "" && p.openProduct != nil {
		buttons := p.links[row.ID]
		if buttons == nil {
			buttons = new(taskLinks)
			p.links[row.ID] = buttons
		}
		fields = append(fields, productLink(view, id, row.DisplayName, &buttons.product, p.openProduct))
		if taskReceiptDue(row) && p.verify != nil {
			fields = append(fields, func(gtx layout.Context) layout.Dimensions {
				if buttons.receipt.Clicked(gtx) {
					p.verify(id, row.DisplayName, &buttons.receipt)
					gtx.Execute(op.InvalidateCmd{})
				}
				view.Focus.Watch(gtx, &buttons.receipt)
				return layout.W.Layout(gtx, view.Button(&buttons.receipt, fmt.Sprintf(l.Text(i18n.TasksVerify), row.DisplayName)))
			})
		}
	}
	return fundsCard(view, row.DisplayName, append(fields, p.upcomingLinks.layoutLinks(view, row)...)...)
}

func taskProductID(row domain.UpcomingAction) domain.ProductContractID {
	target := row.PrimaryTarget
	if target.Kind != "product" || target.ProductID == nil || row.ProductID == nil || *target.ProductID != *row.ProductID {
		return ""
	}
	id, err := domain.ParseProductContractID(target.ProductID.String())
	if err != nil {
		return ""
	}
	return id
}

func taskPhase(row domain.UpcomingAction) i18n.MessageID {
	switch row.Kind {
	case "reservation_shortfall":
		return i18n.OverviewUpcomingShortfall
	case "reservation_unresolved":
		return i18n.OverviewUpcomingUnresolved
	}
	switch row.Phase {
	case "receipt_due":
		if row.DaysFromToday != nil && *row.DaysFromToday < 0 {
			return i18n.TasksOverdue
		}
		return i18n.FundsDue
	case "blocked":
		return i18n.TasksBlocked
	case "today":
		if row.AnchorKind == "eligibility" {
			return i18n.TasksAccessToday
		}
		return i18n.TasksMaturesToday
	case "upcoming":
		if row.AnchorKind == "eligibility" {
			return i18n.TasksAccessUpcoming
		}
		return i18n.TasksUpcoming
	case "eligible":
		return i18n.TasksAccessEligible
	case "awaiting_receipt", "scheduled_receipt":
		return i18n.TasksAwaiting
	default:
		return i18n.OverviewNoValue
	}
}

func taskReason(reason string) i18n.MessageID {
	switch reason {
	case "missing_policy", "unknown_access":
		return i18n.FundsMissingRules
	case "unknown_receipt_timing":
		return i18n.FundsMissingTiming
	case "unknown_amount":
		return i18n.FundsMissingAmount
	case "unknown_fee":
		return i18n.FundsMissingFee
	case "missing_fx":
		return i18n.FundsMissingBaseFX
	case "currency_mismatch":
		return i18n.FundsReservationCurrency
	case "source_missing", "source_archived", "source_zero_quantity", "source_liability":
		return i18n.FundsUnresolvedReason
	default:
		return i18n.FundsUnknownRoute
	}
}
