package desktop

import (
	"context"
	"fmt"
	"slices"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type taskChecksReader interface {
	WithRead(context.Context, func(context.Context) error) error
	ListReviewAlertGroupsV2(context.Context, domain.ReviewAlertFilter, string, int) (domain.ReviewAlertPage, error)
	ListReconciliationSessions(context.Context) ([]domain.ReconciliationSession, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
}

type taskStatement struct {
	session domain.ReconciliationSession
	account domain.Account
}

func readTaskReconciliation(ctx context.Context, service taskChecksReader, includeHoldings bool) (out []taskStatement, holdings holdingReconDirectory, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		sessions, err := service.ListReconciliationSessions(ctx)
		if err != nil {
			return err
		}
		var ids []domain.AccountID
		candidates := make(map[string]domain.AccountID)
		for _, s := range sessions {
			if len(s.AccountIDs) != 1 || (s.Status != "in_progress" && s.Status != "paused") {
				continue
			}
			id, err := domain.ParseAccountID(s.AccountIDs[0])
			if err != nil || s.ID == "" {
				continue
			}
			candidates[s.ID] = id
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if !includeHoldings && len(ids) == 0 {
			return nil
		}
		filter := domain.AccountFilter{AccountIDs: ids}
		if includeHoldings {
			filter.AccountIDs = nil
		}
		accounts, err := service.ListAccounts(ctx, filter)
		if err != nil {
			return err
		}
		cards := make(map[domain.AccountID]domain.Account)
		holdingIDs := make(map[string]bool)
		for _, row := range accounts {
			if row.Account.ArchivedAt != nil {
				continue
			}
			if statementCard(row.Account) {
				cards[row.Account.ID] = row.Account
			}
			if includeHoldings && history.TradeAccount(row.Account) {
				holdings.accounts = append(holdings.accounts, row.Account)
				holdingIDs[row.Account.ID.String()] = true
			}
		}
		for _, session := range sessions {
			if account, ok := cards[candidates[session.ID]]; ok {
				out = append(out, taskStatement{session, account})
			}
			if session.Status == "in_progress" || session.Status == "paused" {
				for _, id := range session.AccountIDs {
					if holdingIDs[id] {
						holdings.sessions = append(holdings.sessions, session)
						break
					}
				}
			}
		}
		return nil
	})
	return out, holdings, err
}

func (p *tasksPage) reviewRows(view ui.View) []layout.Widget {
	l := view.Language
	report := p.review
	var rows []layout.Widget
	if report.PublishedOn != "" {
		date := historyDay(view, report.PublishedOn)
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.TasksReviewPublished), date), view.Colors.SecondaryInk))
	}
	switch report.ProjectionStatus {
	case "complete":
		if len(report.Items) == 0 {
			rows = append(rows, view.Message(l.Text(i18n.TasksReviewEmpty), view.Colors.Ink))
		}
	case "stale":
		rows = append(rows, view.Message(l.Text(i18n.TasksReviewStale), view.Colors.SecondaryInk))
	case "needs_repair":
		rows = append(rows, view.Message(l.Text(i18n.TasksReviewRepair), view.Colors.ErrorInk))
	default:
		rows = append(rows, view.Message(l.Text(i18n.TasksReviewPending), view.Colors.SecondaryInk))
	}
	for _, group := range report.Items {
		var fields []layout.Widget
		for _, rule := range group.Rules {
			if !rule.Actionable {
				continue
			}
			fields = append(fields, view.Message(rule.Name, view.Colors.Ink))
			if rule.TriggeredOn != "" {
				fields = append(fields, view.Message(fmt.Sprintf(l.Text(i18n.TasksReviewTriggered), historyDay(view, rule.TriggeredOn)), view.Colors.SecondaryInk))
			}
			if rule.Window.WindowStart != "" && rule.Window.WindowEnd != "" {
				window := fmt.Sprintf(l.Text(i18n.TasksReviewWindow), historyDay(view, rule.Window.WindowStart),
					historyDay(view, rule.Window.WindowEnd), rule.Window.HitCount)
				fields = append(fields, view.Message(window, view.Colors.SecondaryInk))
			}
		}
		if len(fields) == 0 {
			continue
		}
		if id, err := domain.ParseInstrumentID(group.InstrumentID.String()); err == nil && p.openInstrument != nil {
			button := portfolioButton(p.instruments, id)
			label := fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), group.DisplayName)
			fields = append(fields, taskLink(view, button, label, func() { p.openInstrument(id, group.DisplayName, button) }))
		}
		rows = append(rows, fundsCard(view, group.DisplayName, fields...))
	}
	return rows
}

func (p *tasksPage) statementRows(view ui.View) []layout.Widget {
	l := view.Language
	if len(p.statements) == 0 {
		return []layout.Widget{view.Message(l.Text(i18n.TasksStatementsEmpty), view.Colors.Ink)}
	}
	var rows []layout.Widget
	for _, row := range p.statements {
		s := row.session
		status := l.Text(statementStatus(s))
		when := statementRecordedTime(view, s.StartedAt)
		counts := s.Counts
		fields := []layout.Widget{
			view.Message(status, view.Colors.Ink),
			view.Message(fmt.Sprintf(l.Text(i18n.TasksStatementStarted), when), view.Colors.SecondaryInk),
			view.Message(fmt.Sprintf(l.Text(i18n.TasksStatementCounts), counts.Pending, counts.Unresolved, counts.Confirmed, counts.Skipped), view.Colors.Ink),
		}
		name := row.account.Name
		if len(s.AccountNames) == 1 && s.AccountNames[0] != "" {
			name = s.AccountNames[0]
		}
		if p.openStatement != nil {
			button := portfolioButton(p.statementLinks, s.ID)
			label := fmt.Sprintf(l.Text(i18n.TasksOpenStatement), name, status, when)
			fields = append(fields, taskLink(view, button, label, func() { p.openStatement(row.account.ID, s.ID, name, button) }))
		}
		rows = append(rows, fundsCard(view, name, fields...))
	}
	return rows
}

func taskLink(view ui.View, button *widget.Clickable, label string, open func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			open()
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return layout.W.Layout(gtx, view.Button(button, label))
	}
}

func (p *tasksPage) layoutReviewPager(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.tab != tasksInvestment || p.request != nil || p.err != nil || p.review == nil {
		return layout.Dimensions{}
	}
	cursor := p.review.NextCursor
	if cursor == "" && len(p.reviewCursors) == 0 {
		return layout.Dimensions{}
	}
	if cursor != "" && p.next.Clicked(gtx) {
		p.reviewCursors = append(p.reviewCursors, cursor)
		p.list.Position = layout.Position{}
		p.read()
	}
	if len(p.reviewCursors) > 0 && p.previous.Clicked(gtx) {
		p.reviewCursors = p.reviewCursors[:len(p.reviewCursors)-1]
		p.list.Position = layout.Position{}
		p.read()
	}
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(taskPagerButton(view, &p.previous, i18n.TasksPrevious, len(p.reviewCursors) > 0)),
		layout.Rigid(taskPagerButton(view, &p.next, i18n.TasksNext, cursor != "")))
}

func taskPagerButton(view ui.View, button *widget.Clickable, label i18n.MessageID, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		view.Focus.Watch(gtx, button)
		return view.Button(button, view.Language.Text(label))(gtx)
	}
}

type openTaskStatement func(domain.AccountID, string, string, event.Tag)
