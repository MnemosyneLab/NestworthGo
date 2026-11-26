package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type overviewUpcoming struct {
	reader      upcomingActionsReader
	checks      taskChecksReader
	request     <-chan historyResult[domain.UpcomingActions]
	cancel      context.CancelFunc
	current     *domain.UpcomingActions
	err         error
	dirty       bool
	retry, all  widget.Clickable
	products    map[string]*widget.Clickable
	links       upcomingActionLinks
	openTasks   func(tasksTab, event.Tag)
	openProduct func(domain.ProductContractID, string, bool, event.Tag)
	summary     overviewTaskCounts
}

type overviewTaskCounts struct {
	request             <-chan historyResult[overviewCountResult]
	cancel              context.CancelFunc
	review, sessions    *int
	reviewErr, reconErr error
	dirty               bool
	links               [3]widget.Clickable
	retry               widget.Clickable
}

type overviewCountResult struct {
	review, sessions    int
	reviewErr, reconErr error
}

func (u *overviewUpcoming) retire() {
	if u.cancel != nil {
		u.cancel()
	}
	u.cancel, u.request = nil, nil
}

func (u *overviewUpcoming) invalidate() {
	u.retire()
	u.dirty = true
	u.summary.invalidate()
}

func (s *overviewTaskCounts) retire() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.request = nil, nil
}

func (s *overviewTaskCounts) invalidate() {
	s.retire()
	s.dirty = true
}

func (s *overviewTaskCounts) reviewChanged() {
	s.review, s.reviewErr = nil, nil
	s.invalidate()
}

func (p *overviewPage) updateUpcoming() {
	u := &p.upcoming
	if p.current == nil || p.current.value.AccountCount == 0 || u.reader == nil {
		return
	}
	select {
	case result := <-u.request:
		u.retire()
		u.err = result.err
		if result.err == nil {
			u.current = &result.value
			u.links.retained(result.value.Items)
		}
	default:
	}
	if u.request == nil && (u.dirty || u.current == nil && u.err == nil) && p.ctx.Err() == nil {
		ctx, cancel := context.WithCancel(p.ctx)
		u.cancel, u.dirty, u.err = cancel, false, nil
		u.request = runHistoryRead(&p.workers, p.invalidate, func() (domain.UpcomingActions, error) {
			return u.reader.UpcomingActions(ctx)
		})
	}
	if u.checks != nil && u.openTasks != nil {
		p.updateTaskSummary()
	}
}

func (p *overviewPage) updateTaskSummary() {
	u := &p.upcoming
	s := &u.summary
	select {
	case result := <-s.request:
		s.retire()
		data := result.value
		s.reviewErr, s.reconErr = data.reviewErr, data.reconErr
		if data.reviewErr == nil {
			s.review = &data.review
		}
		if data.reconErr == nil {
			s.sessions = &data.sessions
		}
	default:
	}
	if s.request != nil || p.ctx.Err() != nil || !s.dirty && (s.review != nil || s.reviewErr != nil) {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	s.cancel, s.dirty = cancel, false
	s.request = runHistoryRead(&p.workers, p.invalidate, func() (overviewCountResult, error) {
		page, reviewErr := u.checks.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "unignored"}, "", 50)
		sessions, reconErr := u.checks.ListReconciliationSessions(ctx)
		open := 0
		for _, session := range sessions {
			if session.Status == "in_progress" || session.Status == "paused" {
				open++
			}
		}
		return overviewCountResult{review: page.Summary.ActionableRules, sessions: open, reviewErr: reviewErr, reconErr: reconErr}, nil
	})
}

func (p *overviewPage) overviewTaskSummary(view ui.View) layout.Widget {
	u, l := &p.upcoming, view.Language
	s := &u.summary
	attention := (*int)(nil)
	if u.current != nil {
		attention = &u.current.Counts.Attention
	}
	counts := [3]*int{attention, s.review, s.sessions}
	formats := [3]i18n.MessageID{i18n.OverviewTaskAttention, i18n.OverviewTaskReview, i18n.OverviewTaskSessions}
	fields := []ui.ColumnChild{ui.Rigid(view.Hairline), ui.Rigid(tradeTitle(view, i18n.NavTasks))}
	for i, count := range counts {
		fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			button := &s.links[i]
			if button.Clicked(gtx) {
				u.openTasks(tasksTab(i), button)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := l.Text(taskTabLabels[i])
			if count != nil {
				label = fmt.Sprintf(l.Text(formats[i]), *count)
			}
			return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: label}))
		}))
	}
	if s.reviewErr != nil || s.reconErr != nil {
		fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.retry.Clicked(gtx) {
				s.invalidate()
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Flexed(1, view.Message(l.Text(i18n.OverviewTaskReadFailed), view.Colors.ErrorInk)),
				layout.Rigid(returnMonthButton(view, &s.retry, i18n.HistoryRetry, nil, s.request == nil)))
		}))
	}
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...) }
}

func (p *overviewPage) overviewUpcomingCard(gtx layout.Context, view ui.View) layout.Dimensions {
	u, l := &p.upcoming, view.Language
	var fields []layout.Widget
	switch {
	case u.err != nil:
		if u.retry.Clicked(gtx) {
			u.invalidate()
			gtx.Execute(op.InvalidateCmd{})
		}
		message := i18n.OverviewUpcomingReadFailed
		if u.current != nil {
			message = i18n.OverviewUpcomingReadStale
		}
		fields = append(fields, view.Message(l.Text(message), view.Colors.ErrorInk),
			returnMonthButton(view, &u.retry, i18n.HistoryRetry, nil, true))
	case u.request != nil:
		fields = append(fields, view.Message(l.Text(i18n.FundsLoading), view.Colors.SecondaryInk))
	}
	if data := u.current; data != nil {
		window := fmt.Sprintf(l.Text(i18n.TasksWindow), historyDay(view, data.WindowStartOn), historyDay(view, data.WindowEndOn), data.Timezone)
		fields = append(fields, view.Body(ui.TextCaption, window, view.Colors.SecondaryInk))
		if zone, err := time.LoadLocation(data.Timezone); err == nil && !data.AsOf.IsZero() {
			stamp := tradeTimestamp(view, data.AsOf, zone)
			fields = append(fields, view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.TasksReadAt), stamp), view.Colors.SecondaryInk))
		}
		if data.Counts.Total > 0 {
			counts := fmt.Sprintf(l.Text(i18n.OverviewUpcomingCounts), data.Counts.Attention, data.Counts.Scheduled)
			fields = append(fields, view.Body(ui.TextLabel, counts, view.Colors.Ink))
		} else {
			fields = append(fields, view.Message(l.Text(i18n.OverviewUpcomingEmpty), view.Colors.SecondaryInk))
		}
		seen := 0
		for _, row := range data.Items {
			if row.Group != "attention" && row.Group != "scheduled" {
				continue
			}
			fields = append(fields, view.Hairline, p.overviewUpcomingItem(view, row))
			seen++
			if seen == 3 {
				break
			}
		}
		if data.Counts.DataChecks > 0 || data.Counts.Eligible > 0 || data.Counts.ScopeNotes > 0 {
			details := fmt.Sprintf(l.Text(i18n.OverviewUpcomingOther), data.Counts.Eligible, data.Counts.DataChecks, data.Counts.ScopeNotes)
			fields = append(fields, view.Body(ui.TextCaption, details, view.Colors.SecondaryInk))
		}
		if data.Completeness == domain.StatusPartial {
			fields = append(fields, view.Message(l.Text(i18n.OverviewIncomplete), view.Colors.SecondaryInk))
		}
		if u.openTasks != nil && (data.Counts.Total > 0 || data.Counts.DataChecks > 0 || data.Counts.Eligible > 0 || data.Counts.ScopeNotes > 0) {
			if u.all.Clicked(gtx) {
				u.openTasks(tasksUpcoming, &u.all)
				gtx.Execute(op.InvalidateCmd{})
			}
			fields = append(fields, returnMonthButton(view, &u.all, i18n.OverviewUpcomingAll, nil, true))
		}
	}
	return fundsCard(view, l.Text(i18n.OverviewUpcomingTitle), fields...)(gtx)
}

func (p *overviewPage) overviewUpcomingItem(view ui.View, row domain.UpcomingAction) layout.Widget {
	u, l := &p.upcoming, view.Language
	fields := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, row.DisplayName, view.Colors.Ink))}
	state := taskPhase(row)
	if state != i18n.OverviewNoValue {
		fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, l.Text(state), view.Colors.SecondaryInk)))
	}
	if row.MaturityOn != nil {
		fields = append(fields, ui.Rigid(productMaturity(view, row.MaturityOn)))
	} else if row.EligibilityOn != nil {
		label := fmt.Sprintf(l.Text(i18n.TasksEligibilityOn), historyDay(view, *row.EligibilityOn))
		fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, label, view.Colors.SecondaryInk)))
	}
	if row.EstimatedReceiptOn != nil {
		label := fmt.Sprintf(l.Text(i18n.ProductReceiptDate), historyDay(view, *row.EstimatedReceiptOn))
		fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, label, view.Colors.SecondaryInk)))
	}
	if row.RequestedReserve != nil {
		fields = append(fields, ui.Rigid(fundsAmount(view, i18n.FundsRequested, row.RequestedReserve)))
	}
	if row.Amount != nil {
		fields = append(fields, ui.Rigid(fundsAmount(view, upcomingAmountLabel(row.AmountRole), row.Amount)))
	}
	if id := taskProductID(row); id != "" && u.openProduct != nil {
		if u.products == nil {
			u.products = make(map[string]*widget.Clickable)
		}
		button := portfolioButton(u.products, row.ID)
		fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !taskReceiptDue(row) {
				return productLink(view, id, row.DisplayName, button, func(id domain.ProductContractID, name string, from event.Tag) {
					u.openProduct(id, name, false, from)
				})(gtx)
			}
			if button.Clicked(gtx) {
				u.openProduct(id, row.DisplayName, true, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := fmt.Sprintf(l.Text(i18n.TasksVerify), row.DisplayName)
			return layout.W.Layout(gtx, view.Button(button, label))
		}))
	}
	for _, link := range u.links.layoutLinks(view, row) {
		fields = append(fields, ui.Rigid(link))
	}
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...) }
}

func (p *overviewPage) overviewRecentActivity(gtx layout.Context, view ui.View, result domain.OverviewResult) layout.Dimensions {
	l := view.Language
	var fields []layout.Widget
	if p.openLedger != nil {
		if p.recent.Clicked(gtx) {
			p.openLedger(&p.recent)
			gtx.Execute(op.InvalidateCmd{})
		}
		fields = append(fields, returnMonthButton(view, &p.recent, i18n.OverviewRecentAll, nil, true))
	}
	if len(result.RecentActivities) == 0 {
		fields = append(fields, view.Message(l.Text(i18n.OverviewRecentEmpty), view.Colors.SecondaryInk))
	}
	names := make(map[string]string)
	for _, row := range result.AccountLabels {
		if row.Name != row.ID {
			names[row.ID] = row.Name
		}
	}
	for _, row := range result.InstrumentLabels {
		names[row.ID] = domain.InstrumentDisplayLabel(row.Name, row.Symbol, row.ShortName)
	}
	day := ""
	for _, activity := range result.RecentActivities {
		if day != activity.EffectiveLocalDate {
			day = activity.EffectiveLocalDate
			fields = append(fields, view.Body(ui.TextCaption, historyDay(view, day), view.Colors.SecondaryInk))
		}
		headline := l.Text(historyKindLabel(activity.Kind, activity.Reason))
		var facts []string
		add := func(value string) {
			if value != "" && !slices.Contains(facts, value) {
				facts = append(facts, value)
			}
		}
		for _, effect := range activity.Effects {
			if effect.AccountID != nil {
				add(names[effect.AccountID.String()])
			}
			if effect.InstrumentID != nil {
				add(names[effect.InstrumentID.String()])
			}
		}
		if activity.TradeDetail != nil {
			add(overviewRecentMoney(view, activity.TradeDetail.Gross))
		} else if activity.DividendDetail != nil {
			add(names[activity.DividendDetail.InstrumentID.String()])
			add(overviewRecentMoney(view, activity.DividendDetail.Amount))
		} else {
			for _, effect := range activity.Effects {
				if effect.Money != nil && effect.Role != domain.EffectRoleFee && effect.Role != domain.EffectRoleCost {
					add(overviewRecentMoney(view, *effect.Money))
				}
			}
		}
		fields = append(fields, view.Body(ui.TextBody, strings.Join(append([]string{headline}, facts...), " · "), view.Colors.Ink))
	}
	return fundsCard(view, l.Text(i18n.OverviewRecentTitle), fields...)(gtx)
}

func overviewRecentMoney(view ui.View, money domain.Money) string {
	return valuationMoney(view, domain.MoneyView{Amount: money.CanonicalAmount(), Currency: money.Currency()}, false)
}
