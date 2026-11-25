package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *accountsPage) layoutStatements(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.statement, view.Language
	d := f.screen
	p.pollStatement()
	gtx, view = view.Measure(gtx, 680)
	if d.review != nil {
		return p.layoutStatementReview(gtx, view)
	}

	if d.previewing == nil && d.back.Clicked(gtx) {
		switch {
		case d.form != nil:
			d.form, d.problem = nil, i18n.NoMessage
			gtx.Execute(key.FocusCmd{Tag: &d.check})
		case p.target.statement != "":
			p.open(navigationTarget{route: accountsRoute, account: d.account.ID}, d.account.Name, nil)
		case d.selected != "":
			p.selectStatement("")
		default:
			f.screen = nil
			gtx.Execute(key.FocusCmd{Tag: &f.open})
		}
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.loadFailed && d.reload.Clicked(gtx) {
		p.loadStatements()
	}

	if d.form != nil && d.data != nil {
		a := statementAccount(d.data.detail, d.account.ID.String())
		if d.canEdit() && d.data.detail.Status == "in_progress" && statement.BatchAvailable(a) {
			return p.layoutStatementEvidence(gtx, view, *a)
		}
		d.form = nil
	}

	backLabel := i18n.StatementBackAccount
	if d.selected != "" && p.target.statement == "" {
		backLabel = i18n.StatementBackList
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, &d.back)
			return view.Button(&d.back, l.Text(backLabel))(gtx)
		}),
		layout.Rigid(statementTitle(view, i18n.StatementOpen, d.account.Name)),
	}
	children = append(children, d.feedback(view)...)
	if d.data != nil {
		if d.selected == "" {
			children = append(children, p.statementRecords(gtx, view)...)
		} else {
			detail := d.data.detail
			a := statementAccount(detail, d.account.ID.String())
			children = append(children, p.statementActions(gtx, view, *a)...)
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return statementLines(gtx, view, &d.list, statementDetails(view, detail, *a))
			}))
		}
	}
	gtx = ui.FitWindow(gtx)
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *statementScreen) feedback(view ui.View) []layout.FlexChild {
	l := view.Language
	var rows []layout.FlexChild
	if d.saved {
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.StatementSaved), view.Colors.Ink)))
	}
	if d.problem != i18n.NoMessage {
		rows = append(rows, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	switch {
	case d.loading != nil || d.previewing != nil:
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case d.loadFailed:
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.StatementLoadFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&d.reload, l.Text(i18n.HistoryRetry))))
	}
	return rows
}

func (p *accountsPage) statementRecords(gtx layout.Context, view ui.View) []layout.FlexChild {
	d, l := p.statement.screen, view.Language
	var rows []layout.FlexChild
	if d.account.ArchivedAt == nil {
		if d.previewing == nil && d.start.Clicked(gtx) {
			p.previewStatement(d.action(statement.Start))
		}
		rows = append(rows, layout.Rigid(statementButton(view, &d.start, i18n.StatementStart, d.previewing == nil)))
	}
	if len(d.data.records) == 0 {
		return append(rows, layout.Rigid(view.Message(l.Text(i18n.StatementEmpty), view.Colors.SecondaryInk)))
	}
	var records []layout.Widget
	for _, row := range d.data.records {
		click := d.records[row.ID]
		if d.previewing == nil && click.Clicked(gtx) {
			p.selectStatement(row.ID)
			break
		}
		when := row.StartedAt
		if row.CompletedAt != "" {
			when = row.CompletedAt
		}
		name := l.Text(statementStatus(row)) + " · " + statementRecordedTime(view, when)
		records = append(records, view.Button(click, name))
	}
	return append(rows, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return statementLines(gtx, view, &d.list, records)
	}))
}

func (d *statementScreen) canEdit() bool {
	return d.account.ArchivedAt == nil && len(d.data.detail.Accounts) == 1
}

func (p *accountsPage) statementActions(gtx layout.Context, view ui.View, account domain.ReconciliationAccount) []layout.FlexChild {
	d := p.statement.screen
	if d.data.detail.Status == "completed" {
		return nil
	}
	if d.canEdit() {
		var clicked *widget.Clickable
		var label i18n.MessageID
		var action statement.Action
		switch {
		case d.data.detail.Status == "paused":
			clicked, label, action = &d.resume, i18n.StatementResume, statement.Resume
		case statement.BatchAvailable(&account):
			clicked, label = &d.check, i18n.StatementCheck
		case account.Outcome == "confirmed" && !account.RecheckRequired:
			clicked, label, action = &d.finish, i18n.StatementFinish, statement.Finish
		}
		if clicked != nil {
			if d.previewing == nil && clicked.Clicked(gtx) {
				if action == "" {
					d.form = &statementEvidence{reference: ui.NewTextField(true), note: ui.NewTextField(true)}
					d.problem, d.saved = i18n.NoMessage, false
					gtx.Execute(op.InvalidateCmd{})
				} else {
					p.previewStatement(d.action(action))
				}
			}
			return []layout.FlexChild{layout.Rigid(statementButton(view, clicked, label, d.previewing == nil))}
		}
	}
	return []layout.FlexChild{layout.Rigid(view.Message(view.Language.Text(i18n.StatementViewOnly), view.Colors.SecondaryInk))}
}

func (p *accountsPage) layoutStatementEvidence(gtx layout.Context, view ui.View, account domain.ReconciliationAccount) layout.Dimensions {
	d, l := p.statement.screen, view.Language
	f := d.form
	today := p.clock().In(d.data.zone).Format(time.DateOnly)
	day := f.day
	if day == "" {
		day = today
	}
	if d.previewing == nil && f.completeness.Value && d.prepare.Clicked(gtx) {
		in := d.action(statement.Check)
		in.AccountID, in.FinancialRevision = account.AccountID, account.FinancialRevision
		in.Outcome, in.ScopeReviewed = statement.Confirmed, true
		in.Source = domain.ReconciliationSource{Reference: f.reference.Text(), AsOfDate: day, AsOfTime: f.time}
		in.Note = f.note.Text()
		if _, valid := ui.ParseCivilDay(day); !valid || day > today {
			d.problem = i18n.StatementBadDate
		} else {
			p.previewStatement(in)
		}
	}

	origin := view.Origin
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Button(&d.back, l.Text(i18n.InstitutionsCancel)))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, statementTitle(view, i18n.StatementCheck, account.Name))
		}),
	}
	for _, item := range account.Items {
		if item.Kind == "balance" {
			line := statementDebt(view, item)
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, line)
			}))
		}
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return fieldPosition(gtx, &origin, view.Message(l.Text(i18n.StatementEvidenceHint), view.Colors.SecondaryInk))
	}))
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return statementPickers(gtx, view, d, origin, day, today)
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return f.reference.Layout(gtx, view, ui.FieldOptions{Label: i18n.StatementReference, Disabled: d.previewing != nil})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return f.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.StatementNote, Disabled: d.previewing != nil})
			}),
		)
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, &f.completeness)
		if d.previewing != nil {
			gtx = gtx.Disabled()
		}
		return material.CheckBox(view.Theme, &f.completeness, l.Text(i18n.StatementCompleteness)).Layout(gtx)
	}))
	children = append(children, d.feedback(view)...)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return statementButton(view, &d.prepare, i18n.StatementPreview, d.previewing == nil && f.completeness.Value)(gtx)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func statementPickers(gtx layout.Context, view ui.View, d *statementScreen, origin image.Point, day, today string) layout.Dimensions {
	f := d.form
	var children []layout.FlexChild
	gap := gtx.Dp(8)
	week := time.Monday
	if view.WeekStart == "sunday" {
		week = time.Sunday
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		f.datePicker.Layout(gtx, view, ui.PickerField{Label: i18n.StatementDate, Origin: origin, Disabled: d.previewing != nil},
			ui.DateOptions{Today: today, Max: today, WeekStart: week, Format: view.DateFormat}, day, func(day string) {
				f.day, d.problem = day, i18n.NoMessage
			})
		origin.X += gtx.Constraints.Max.X + gap
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}))
	if d.data.hasOrigin {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			f.timePicker.Layout(gtx, view, ui.PickerField{Label: i18n.StatementTime, Origin: origin, Disabled: d.previewing != nil},
				f.time, view.TimeFormat == settings.TimeFormat12, true, func(clock string) { f.time, d.problem = clock, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}))
	}
	return layout.Flex{Gap: gap}.Layout(gtx, children...)
}

func (p *accountsPage) layoutStatementReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.statement.screen, view.Language
	if !p.statementUnresolved() && d.edit.Clicked(gtx) {
		d.review = nil
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.replay
	}
	if d.committing == nil && confirm.Clicked(gtx) {
		p.confirmStatement()
	}
	detail := d.review.Result
	account := statementAccount(detail, d.account.ID.String())
	if account == nil {
		return view.Message(l.Text(i18n.StatementPreviewFailed), view.Colors.ErrorInk)(gtx)
	}
	children := []layout.FlexChild{layout.Rigid(statementTitle(view, i18n.StatementPreview, account.Name))}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		switch {
		case d.committing != nil:
			return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
		case d.uncertain:
			return view.PrimaryButton(confirm, l.Text(i18n.ContributionRetryPost))(gtx)
		default:
			label := i18n.InstitutionsCancel
			if d.form != nil {
				label = i18n.StatementEditEvidence
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&d.edit, l.Text(label))),
				layout.Rigid(view.PrimaryButton(confirm, l.Text(i18n.StatementConfirm))),
			)
		}
	}))
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return statementLines(gtx, view, &d.list, statementDetails(view, detail, *account))
	}))
	gtx = ui.FitWindow(gtx)
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func statementButton(view ui.View, button *widget.Clickable, label i18n.MessageID, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		return view.PrimaryButton(button, view.Language.Text(label))(gtx)
	}
}

func statementTitle(view ui.View, label i18n.MessageID, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Label(gtx, view.Language.Text(label)+" · "+name, 19, font.Medium, view.Colors.Ink)
	}
}

func statementLines(gtx layout.Context, view ui.View, list *widget.List, rows []layout.Widget) layout.Dimensions {
	return view.List(list).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Bottom: 7}.Layout(gtx, rows[index])
	})
}

func statementRecordedTime(view ui.View, value string) string {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return view.Language.Text(i18n.AccountReferenceUnavailable)
	}
	return format.LocalDateTime(at.In(view.TimeLocation), settings.Settings{
		DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
	}, settings.Language(view.Language))
}

func statementStatus(record domain.ReconciliationSession) i18n.MessageID {
	switch record.Status {
	case "paused":
		return i18n.StatementPaused
	case "completed":
		if record.Partial {
			return i18n.StatementPartial
		}
		return i18n.StatementCompleted
	default:
		return i18n.StatementInProgress
	}
}

func statementDetails(view ui.View, session domain.ReconciliationSession, account domain.ReconciliationAccount) []layout.Widget {
	l := view.Language
	rows := []layout.Widget{view.Message(l.Text(statementStatus(session)), view.Colors.Ink)}
	if session.CompletedAt != "" {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.StatementCompletedAt), statementRecordedTime(view, session.CompletedAt)), view.Colors.SecondaryInk))
	}
	for _, note := range []string{session.Note, session.ClosingNote} {
		if note != "" {
			rows = append(rows, view.Message(note, view.Colors.Ink))
		}
	}
	if account.RecheckRequired || account.InventoryCoverageGap {
		rows = append(rows, view.Message(l.Text(i18n.StatementRecheck), view.Colors.ErrorInk))
	}
	for _, item := range account.Items {
		rows = append(rows, statementCheckpoint(view, item)...)
	}
	return rows
}

func statementDebt(view ui.View, item domain.ReconciliationItem) layout.Widget {
	l := view.Language
	currency := domain.CurrencyCode(item.Currency)
	owed, err := domain.ParseMoney(item.Value, currency)
	if err != nil {
		return view.Message(fmt.Sprintf(l.Text(i18n.AccountValueUnavailable), item.Currency), view.Colors.SecondaryInk)
	}
	return view.Message(fmt.Sprintf(l.Text(i18n.StatementDebt), owed.CanonicalAmount()+" "+item.Currency,
		owed.Amount().Neg().String()+" "+item.Currency), view.Colors.Ink)
}

func statementCheckpoint(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	l := view.Language
	label := item.Label
	switch item.Kind {
	case "scope":
		label = l.Text(i18n.StatementScope)
	case "balance":
		label = l.Text(i18n.StatementBalance)
	}
	outcome := i18n.StatementPending
	switch item.Outcome {
	case "confirmed":
		outcome = i18n.StatementChecked
	case "skipped":
		outcome = i18n.StatementSkipped
	case "unresolved":
		outcome = i18n.StatementUnresolved
	}
	if item.RecheckRequired {
		outcome = i18n.StatementRecheck
	}
	lines := []layout.Widget{view.Message(label+" · "+l.Text(outcome), view.Colors.Ink)}
	if item.Kind == "balance" {
		lines = append(lines, statementDebt(view, item))
	}
	if item.Source.AsOfDate != "" {
		date := item.Source.AsOfDate
		if day, ok := ui.ParseCivilDay(date); ok {
			date = ui.FormatDay(day, l, view.DateFormat)
		}
		if item.Source.AsOfTime != "" {
			date += " " + item.Source.AsOfTime + " (" + item.Source.Timezone + ")"
		}
		lines = append(lines, view.Message(fmt.Sprintf(l.Text(i18n.StatementEvidenceDate), date), view.Colors.SecondaryInk))
	}
	if item.CheckedAt != "" {
		lines = append(lines, view.Message(fmt.Sprintf(l.Text(i18n.StatementCheckedAt), statementRecordedTime(view, item.CheckedAt)), view.Colors.SecondaryInk))
	}
	if item.Source.Reference != "" {
		lines = append(lines, view.Message(fmt.Sprintf(l.Text(i18n.StatementEvidenceReference), item.Source.Reference), view.Colors.Ink))
	}
	if item.ReferenceValue != "" {
		lines = append(lines, view.Message(fmt.Sprintf(l.Text(i18n.StatementEvidenceValue), item.ReferenceValue), view.Colors.Ink))
	}
	if item.Note != "" {
		lines = append(lines, view.Message(item.Note, view.Colors.Ink))
	}
	return lines
}
