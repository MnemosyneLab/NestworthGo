package desktop

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type reconciliationStartSheet struct {
	analysisSheet
	cancel         widget.Clickable
	restorePreview bool
}

func newReconciliationStartSheet() *reconciliationStartSheet {
	sheet := newAnalysisSheet()
	sheet.width, sheet.modal = 600, true
	return &reconciliationStartSheet{analysisSheet: sheet}
}

func (p *tasksPage) startReconciliationDraft() *holdingReconDraft {
	if p == nil || p.holdingRecon == nil {
		return nil
	}
	if d := p.holdingRecon.draft; d != nil && d.action == statement.Start {
		return d
	}
	return nil
}

func (s *shell) startReconciliationReviewOpen() bool {
	if s.activeRoute() != tasksRoute {
		return false
	}
	d := s.tasks.startReconciliationDraft()
	return d != nil && d.review != nil && d.review.session != nil
}

func (p *tasksPage) layoutStartReconciliationReview(gtx layout.Context, view ui.View) {
	c, l := p.holdingRecon, view.Language
	d := c.draft
	sheet := d.startSheet
	canClose := !c.unresolved()
	dismiss := sheet.dismissed(gtx, &sheet.cancel, &d.confirm)
	for {
		e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
	if canClose && (dismiss || sheet.cancel.Clicked(gtx)) {
		d.review, d.reviewFocus, sheet.restorePreview = nil, false, true
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if d.writing == nil && d.confirm.Clicked(gtx) {
		p.confirmHoldingReconciliation()
		gtx.Execute(op.InvalidateCmd{})
		canClose = false
	}
	session := d.review.session
	n := session.Counts
	counts := fmt.Sprintf(l.Text(i18n.ReconciliationStartCounts), n.Pending, n.Unresolved, n.Confirmed, n.Skipped)
	rows := []layout.Widget{
		view.Body(ui.TextHeading, holdingReconciliationAccountNames(session), view.Colors.Ink),
		view.Message(l.Text(statementStatus(*session)), view.Colors.Ink),
		view.Message(counts, view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.TasksStatementStarted), statementRecordedTime(view, session.StartedAt)), view.Colors.SecondaryInk),
	}
	if session.Note != "" {
		rows = append(rows, view.Message(session.Note, view.Colors.SecondaryInk))
	}
	rows = append(rows, view.Message(l.Text(i18n.ReconciliationExplicitReview), view.Colors.SecondaryInk))
	rows = append(rows, reconciliationStartEvidence(view, d.selection, session.Accounts)...)
	if d.uncertain {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconRetry), view.Colors.ErrorInk))
	}
	footer := func(gtx layout.Context) layout.Dimensions {
		if d.writing != nil {
			return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
		}
		if d.reviewFocus {
			gtx.Execute(key.FocusCmd{Tag: &d.confirm})
			d.reviewFocus = false
		}
		if d.uncertain {
			return view.PrimaryButton(&d.confirm, l.Text(i18n.ReconciliationRetryStart))(gtx)
		}
		return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(view.Button(&sheet.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(view.PrimaryButton(&d.confirm, l.Text(i18n.ReconciliationStart))))
	}
	sheet.layoutPanel(gtx, view, l.Text(i18n.ReconciliationStartPreview), func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.List(&sheet.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: ui.Space8}.Layout(gtx, rows[i])
		})
	}, footer, canClose)
}
