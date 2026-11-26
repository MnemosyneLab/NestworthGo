package desktop

import (
	"context"
	"fmt"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var taskSummaryLabels = [...]i18n.MessageID{i18n.OverviewTaskAttention, i18n.OverviewTaskReview, i18n.OverviewTaskSessions}

type taskReviewReconciliationCounts struct {
	review, sessions    int
	reviewErr, reconErr error
}

func readReviewAndReconciliationCounts(ctx context.Context, checks taskChecksReader) taskReviewReconciliationCounts {
	page, reviewErr := checks.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "unignored"}, "", 50)
	sessions, reconErr := checks.ListReconciliationSessions(ctx)
	open := 0
	for _, session := range sessions {
		if session.Status == "in_progress" || session.Status == "paused" {
			open++
		}
	}
	return taskReviewReconciliationCounts{review: page.Summary.ActionableRules, sessions: open, reviewErr: reviewErr, reconErr: reconErr}
}

type taskCategoryCounts [len(taskTabLabels)]int

type taskCategoryBadges struct {
	values  taskCategoryCounts
	request <-chan historyResult[taskCategoryCounts]
	cancel  context.CancelFunc
	loaded  bool
}

func (p *tasksPage) invalidateTaskBadges() {
	badges := &p.taskBadges
	if badges.cancel != nil {
		badges.cancel()
	}
	*badges = taskCategoryBadges{}
}

func (p *tasksPage) readTaskBadges() {
	badges := &p.taskBadges
	if badges.loaded || badges.request != nil || p.checks == nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	badges.cancel = cancel
	badges.request = runHistoryRead(&p.workers, p.invalidate, func() (taskCategoryCounts, error) {
		var counts taskCategoryCounts
		data := readReviewAndReconciliationCounts(ctx, p.checks)
		if data.reviewErr == nil {
			counts[tasksInvestment] = data.review
		}
		if data.reconErr == nil {
			counts[tasksReconciliation] = data.sessions
		}
		if upcoming, err := p.service.UpcomingActions(ctx); err == nil {
			counts[tasksUpcoming] = upcoming.Counts.Attention
		}
		return counts, nil
	})
}

func (p *tasksPage) updateTaskBadges() {
	badges := &p.taskBadges
	select {
	case result := <-badges.request:
		badges.cancel()
		*badges = taskCategoryBadges{values: result.value, loaded: true}
	default:
	}
	p.readTaskBadges()
}

func (p *tasksPage) taskTabButton(gtx layout.Context, view ui.View, tab tasksTab) layout.Dimensions {
	l := view.Language
	button, title := &p.tabs[tab], l.Text(taskTabLabels[tab])
	count, selected := p.taskBadges.values[tab], p.tab == tab
	switch {
	case selected && tab == tasksUpcoming && p.current != nil:
		count = p.current.Counts.Attention
	case selected && tab == tasksInvestment && p.review != nil:
		count = p.review.Summary.ActionableRules
	}
	if count <= 0 {
		return view.TabSegment(button, title, selected)(gtx)
	}
	visible, tone := strconv.Itoa(count), ui.BadgeWarning
	if count > 99 {
		visible = "99+"
	}
	if tab == tasksReconciliation {
		tone = ui.BadgeNeutral
	}
	description := fmt.Sprintf(l.Text(taskSummaryLabels[tab]), count)
	return view.ButtonOf(button, ui.ButtonSpec{
		Variant: ui.ButtonSegment, Title: title, Name: title, Description: description, Selected: selected, Toggle: true,
		Content: func(gtx layout.Context) layout.Dimensions {
			return (layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}).Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextLabel, title, view.Colors.Ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return semanticRow(gtx, description, func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, visible) })
				}))
		},
	})(gtx)
}

func (p *tasksPage) upcomingDataMaintenance(view ui.View, counts domain.UpcomingCounts) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := view.Language
		label := fmt.Sprintf(l.Text(i18n.TasksDataChecks), counts.DataChecks, counts.ScopeNotes)
		details := view.Message(label, view.Colors.SecondaryInk)
		if p.upcomingLinks.openHealth == nil {
			return details(gtx)
		}
		if p.upcomingHealth.Clicked(gtx) {
			p.upcomingLinks.openHealth(healthFindingFocus{}, &p.upcomingHealth)
			gtx.Execute(op.InvalidateCmd{})
		}
		action := view.ButtonOf(&p.upcomingHealth, ui.ButtonSpec{
			Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: l.Text(i18n.TasksOpenDataMaintenance),
		})
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
			return [...]layout.Widget{details, action}[i](gtx)
		})
	}
}
