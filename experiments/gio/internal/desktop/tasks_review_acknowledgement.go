package desktop

import (
	"context"
	"errors"
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type taskReviewAcknowledger interface {
	AcknowledgeReviewEpisodeV2(context.Context, string, string, int) (domain.ReviewAcknowledgement, error)
}

type taskReviewAcknowledgementIntent struct {
	episode  domain.ReviewEpisodeID
	mutation domain.MutationID
	revision int
}

type taskReviewAcknowledgement struct {
	intent     *taskReviewAcknowledgementIntent
	request    <-chan historyResult[domain.ReviewAcknowledgement]
	status     i18n.MessageID
	generation uint64
	buttons    map[domain.ReviewEpisodeID]*widget.Clickable
	retry      widget.Clickable
	onChange   func()
}

func (p *tasksPage) reviewAcknowledgeButton(view ui.View, instrument string, rule domain.ReviewAlertRule) layout.Widget {
	id, err := domain.ParseReviewEpisodeID(string(rule.EpisodeID))
	writer, supported := p.checks.(taskReviewAcknowledger)
	if !supported || !rule.Actionable || err != nil || rule.EpisodeRevision < 1 || p.review.PublishedOn == "" {
		return nil
	}
	a := &p.reviewAck
	if a.buttons == nil {
		a.buttons = make(map[domain.ReviewEpisodeID]*widget.Clickable)
	}
	button := portfolioButton(a.buttons, id)
	title := view.Language.Text(i18n.TasksReviewAcknowledge)
	name := fmt.Sprintf("%s · %s · %s", title, instrument, rule.Name)
	return func(gtx layout.Context) layout.Dimensions {
		if a.intent == nil && p.request == nil && p.err == nil && button.Clicked(gtx) {
			a.intent = &taskReviewAcknowledgementIntent{episode: id, mutation: domain.NewMutationID(), revision: rule.EpisodeRevision}
			p.submitReviewAcknowledgement(gtx, writer, button)
		}
		if a.intent != nil || p.request != nil || p.err != nil {
			gtx = gtx.Disabled()
		}
		return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{
			Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: title, Name: name,
		}))
	}
}

func (p *tasksPage) submitReviewAcknowledgement(gtx layout.Context, writer taskReviewAcknowledger, button *widget.Clickable) {
	a := &p.reviewAck
	intent := *a.intent
	a.status = i18n.TasksReviewAcknowledging
	// Route changes cancel reads. A submitted mutation belongs to the window and
	// keeps the same identity until the service confirms its outcome.
	a.request = runHistoryRead(&p.workers, p.invalidate, func() (domain.ReviewAcknowledgement, error) {
		return writer.AcknowledgeReviewEpisodeV2(p.ctx, string(intent.episode), intent.mutation.String(), intent.revision)
	})
	if gtx.Focused(button) {
		gtx.Execute(key.FocusCmd{Tag: &p.tabs[tasksInvestment]})
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *tasksPage) pollReviewAcknowledgement(active bool) {
	a := &p.reviewAck
	select {
	case result := <-a.request:
		a.request = nil
		receipt, intent := result.value, a.intent
		var issue *domain.Error
		errors.As(result.err, &issue)
		switch {
		case result.err == nil && receipt.EpisodeID == intent.episode &&
			receipt.Revision >= intent.revision && !receipt.AcknowledgedAt.IsZero():
			a.intent, a.status = nil, i18n.TasksReviewAcknowledged
		case issue != nil && (issue.Code == domain.ErrConflict || issue.Code == domain.ErrNotFound):
			a.intent, a.status = nil, i18n.TasksReviewAcknowledgeChanged
		case issue != nil && (issue.Code == domain.ErrValidation || issue.Code == domain.ErrUnavailable):
			a.intent, a.status = nil, i18n.TasksReviewAcknowledgeFailed
		default:
			a.status = i18n.TasksReviewAcknowledgeUncertain
		}
		if a.intent == nil {
			delete(a.buttons, intent.episode)
		}
		// A write may commit before its response arrives. Refresh both projections,
		// including after an uncertain response, without replaying the write.
		a.generation++
		if len(p.reviewCursors) > 0 {
			p.reviewCursors, p.list.Position = nil, layout.Position{}
		}
		p.invalidateTaskBadges()
		if active && p.tab == tasksInvestment {
			p.read()
		}
		if a.onChange != nil {
			a.onChange()
		}
	default:
	}
}

func (p *tasksPage) layoutReviewAcknowledgement(gtx layout.Context, view ui.View) layout.Dimensions {
	a := &p.reviewAck
	l := view.Language
	ink := view.Colors.SecondaryInk
	switch a.status {
	case i18n.TasksReviewAcknowledgeChanged, i18n.TasksReviewAcknowledgeFailed, i18n.TasksReviewAcknowledgeUncertain:
		ink = view.Colors.ErrorInk
	}
	message := view.Message(l.Text(a.status), ink)
	if a.status != i18n.TasksReviewAcknowledgeUncertain {
		return message(gtx)
	}
	return ui.Column{Gap: ui.Space4}.Layout(gtx,
		ui.Rigid(message),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if writer, ok := p.checks.(taskReviewAcknowledger); ok && a.retry.Clicked(gtx) {
				p.submitReviewAcknowledgement(gtx, writer, &a.retry)
			}
			return layout.W.Layout(gtx, view.ButtonOf(&a.retry, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(i18n.TasksReviewAcknowledgeRetry),
			}))
		}),
	)
}
