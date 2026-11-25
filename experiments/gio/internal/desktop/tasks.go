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
	"gioui.org/widget/material"

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
	tab           tasksTab
	position      layout.Position
	reviewCursors []string
}

type tasksRead struct {
	upcoming   *domain.UpcomingActions
	review     *domain.ReviewAlertPage
	statements []taskStatement
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
	verify         func(domain.ProductContractID, string, event.Tag)
	checks         taskChecksReader
	tab            tasksTab
	tabs           [len(taskTabLabels)]widget.Clickable
	review         *domain.ReviewAlertPage
	reviewCursors  []string
	statements     []taskStatement
	instruments    map[domain.InstrumentID]*widget.Clickable
	statementLinks map[string]*widget.Clickable
	previous, next widget.Clickable
	openInstrument func(domain.InstrumentID, string, event.Tag)
	openStatement  openTaskStatement
}

func newTasksPage(ctx context.Context, service upcomingActionsReader, invalidate func()) *tasksPage {
	return &tasksPage{ctx: ctx, service: service, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, links: make(map[string]*taskLinks),
		instruments: make(map[domain.InstrumentID]*widget.Clickable), statementLinks: make(map[string]*widget.Clickable)}
}

func (p *tasksPage) enter(visit tasksVisit) {
	p.tab, p.list.Position = visit.tab, visit.position
	p.reviewCursors = slices.Clone(visit.reviewCursors)
	p.read()
}

func (p *tasksPage) visit() tasksVisit {
	return tasksVisit{p.tab, p.list.Position, slices.Clone(p.reviewCursors)}
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
	cursor := ""
	if len(p.reviewCursors) != 0 {
		cursor = p.reviewCursors[len(p.reviewCursors)-1]
	}
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (tasksRead, error) {
		switch tab {
		case tasksInvestment:
			page, err := p.checks.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "pending"}, cursor, 50)
			return tasksRead{review: &page}, err
		case tasksReconciliation:
			sessions, err := readTaskStatements(ctx, p.checks)
			return tasksRead{statements: sessions}, err
		default:
			actions, err := p.service.UpcomingActions(ctx)
			return tasksRead{upcoming: &actions}, err
		}
	})
}

func termDepositTasks(items []domain.UpcomingAction) []domain.UpcomingAction {
	var rows []domain.UpcomingAction
	for _, row := range items {
		if row.Kind == "product_schedule" && row.ProductKind != nil && *row.ProductKind == domain.ProductTermDeposit &&
			(row.Group == "attention" || row.Group == "scheduled") {
			rows = append(rows, row)
		}
	}
	return rows
}

func (p *tasksPage) update() {
	select {
	case result := <-p.request:
		p.leave()
		p.err, p.current, p.review, p.statements = result.err, nil, nil, nil
		if p.err != nil {
			return
		}
		p.review, p.statements = result.value.review, result.value.statements
		actions := result.value.upcoming
		if actions == nil {
			return
		}
		actions.Items = termDepositTasks(actions.Items)
		p.current = actions
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
	if p.checks != nil {
		for i := range p.tabs {
			if p.tabs[i].Clicked(gtx) && p.tab != tasksTab(i) {
				p.enter(tasksVisit{tab: tasksTab(i)})
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	if p.request == nil && p.reload.Clicked(gtx) {
		if p.tab == tasksInvestment && len(p.reviewCursors) > 0 {
			p.enter(tasksVisit{tab: p.tab})
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
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(ui.LimitWidth(gtx, 820),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if p.checks == nil {
						return tradeTitle(view, i18n.TasksTermDeposits)(gtx)
					}
					tabs := make([]layout.FlexChild, 0, len(p.tabs))
					for i, label := range taskTabLabels {
						tabs = append(tabs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							view.Focus.Watch(gtx, &p.tabs[i])
							return view.TabButton(&p.tabs[i], l.Text(label), p.tab == tasksTab(i))(gtx)
						}))
					}
					return layout.Flex{Gap: gtx.Dp(6)}.Layout(gtx, tabs...)
				}),
				layout.Rigid(taskPagerButton(view, &p.reload, reload, p.request == nil)))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
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
				rows = p.statementRows(view)
			default:
				if p.current != nil {
					rows = p.rows(view)
				}
			}
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return material.List(view.Theme, &p.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, rows[i])
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutReviewPager(gtx, view) }))
}

func (p *tasksPage) rows(view ui.View) []layout.Widget {
	l := view.Language
	data := p.current
	window := fmt.Sprintf(l.Text(i18n.TasksWindow), historyDay(view, data.WindowStartOn), historyDay(view, data.WindowEndOn), data.Timezone)
	var rows []layout.Widget
	if p.checks != nil {
		rows = append(rows, tradeTitle(view, i18n.TasksTermDeposits))
	}
	rows = append(rows, view.Message(window, view.Colors.SecondaryInk))
	if zone, err := time.LoadLocation(data.Timezone); err == nil && !data.AsOf.IsZero() {
		readAt := format.LocalDateTime(data.AsOf.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(l))
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.TasksReadAt), readAt), view.Colors.SecondaryInk))
	}
	if len(data.Items) == 0 {
		return append(rows, view.Message(l.Text(i18n.TasksEmpty), view.Colors.Ink))
	}
	lastGroup := ""
	for _, row := range data.Items {
		if row.Group != lastGroup {
			heading := i18n.TasksScheduled
			if row.Group == "attention" {
				heading = i18n.TasksAttention
			}
			rows = append(rows, tradeTitle(view, heading))
			lastGroup = row.Group
		}
		rows = append(rows, p.card(view, row))
	}
	return rows
}

func (p *tasksPage) card(view ui.View, row domain.UpcomingAction) layout.Widget {
	l := view.Language
	fields := []layout.Widget{view.Message(l.Text(taskPhase(row)), view.Colors.Ink)}
	if row.AccountName != nil && *row.AccountName != "" {
		fields = append(fields, view.Message(*row.AccountName, view.Colors.SecondaryInk))
	}
	fields = append(fields, productMaturity(view, row.MaturityOn))
	receipt := l.Text(i18n.OverviewNoValue)
	if row.EstimatedReceiptOn != nil {
		receipt = historyDay(view, *row.EstimatedReceiptOn)
	}
	fields = append(fields, view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptDate), receipt), view.Colors.SecondaryInk))
	amountLabel := i18n.ProductForecastNet
	if row.Amount != nil && row.AmountStatus == "partial" {
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
		if row.Phase == "receipt_due" && p.verify != nil {
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
	return fundsCard(view, row.DisplayName, fields...)
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
	switch row.Phase {
	case "receipt_due":
		if row.DaysFromToday != nil && *row.DaysFromToday < 0 {
			return i18n.TasksOverdue
		}
		return i18n.FundsDue
	case "blocked":
		return i18n.TasksBlocked
	case "today":
		return i18n.TasksMaturesToday
	case "upcoming":
		return i18n.TasksUpcoming
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
	default:
		return i18n.FundsUnknownRoute
	}
}
