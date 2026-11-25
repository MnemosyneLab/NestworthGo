package desktop

import (
	"errors"
	"fmt"
	"image"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type holdingCheckpointDraft struct {
	outcome, day, time string
	daySet             bool
	picker             ui.Select
	datePicker         ui.DatePicker
	timePicker         ui.TimePicker
	reference, value   ui.TextField
	comparable         widget.Bool
	scopeReviewed      widget.Bool
}

func (p *tasksPage) openHoldingCheckpoint(a domain.ReconciliationAccount, item domain.ReconciliationItem, button *widget.Clickable) {
	c := p.holdingRecon
	d := newHoldingReconDraft(button)
	d.action, d.accountID, d.itemID = statement.CheckItem, a.AccountID, item.ID
	d.check = &holdingCheckpointDraft{outcome: statement.Confirmed, day: item.Source.AsOfDate,
		daySet: item.Source.AsOfDate != "", time: item.Source.AsOfTime,
		reference: ui.NewTextField(true), value: ui.NewTextField(true)}
	d.check.reference.Editor.SetText(item.Source.Reference)
	d.check.value.Editor.SetText(item.ReferenceValue)
	d.check.comparable.Value = item.ReferenceComparable
	d.note.Editor.SetText(item.Note)
	c.saved, c.draft = i18n.NoMessage, d
}

func (p *tasksPage) openHoldingAccountCheck(account domain.ReconciliationAccount, scope domain.ReconciliationItem, button *widget.Clickable) {
	d := newHoldingReconDraft(button)
	d.action, d.accountID, d.itemID = statement.Check, account.AccountID, scope.ID
	d.check = &holdingCheckpointDraft{outcome: statement.Confirmed, reference: ui.NewTextField(true)}
	p.holdingRecon.saved, p.holdingRecon.draft = i18n.NoMessage, d
}

func uncheckedHoldingItem(item domain.ReconciliationItem) bool {
	return item.Outcome != statement.Confirmed || item.RecheckRequired
}

func holdingBatchItem(view ui.View, item domain.ReconciliationItem) layout.Widget {
	var rows []layout.Widget
	switch item.Kind {
	case "scope":
		rows = append(rows, view.Message(view.Language.Text(i18n.HoldingReconScopeHelp), view.Colors.SecondaryInk))
	case "holding":
		rows = holdingPositionEvidence(view, item)
	default:
		rows = holdingCheckpointFacts(view, item)
	}
	rows = append(rows, holdingCheckEvidence(view, item)...)
	return fundsCard(view, holdingCheckLabel(view, item), rows...)
}

func (c *holdingReconciliation) checkDate(f *holdingCheckpointDraft) (day, today string) {
	if c.zone != nil {
		today = c.clock().In(c.zone).Format(time.DateOnly)
	}
	if f.daySet {
		return f.day, today
	}
	return today, today
}

func holdingCheckable(item domain.ReconciliationItem) bool {
	switch item.Kind {
	case "scope", "cash", "holding", "product", "removed":
		return true
	}
	return false
}

func holdingCheckComparable(item domain.ReconciliationItem) bool {
	return item.Kind == "cash" || item.Kind == "holding"
}

func holdingCheckLabel(view ui.View, item domain.ReconciliationItem) string {
	if item.Kind == "scope" {
		return view.Language.Text(i18n.StatementScope)
	}
	if item.Kind == "cash" {
		return fmt.Sprintf(view.Language.Text(i18n.FundsCashSource), item.Currency)
	}
	return item.Label
}

func holdingCheckOutcome(item domain.ReconciliationItem) i18n.MessageID {
	if item.RecheckRequired {
		return i18n.StatementRecheck
	}
	switch item.Outcome {
	case "confirmed":
		return i18n.StatementChecked
	case "unresolved":
		return i18n.StatementUnresolved
	case "skipped":
		return i18n.StatementSkipped
	default:
		return i18n.StatementPending
	}
}

func holdingCheckValue(view ui.View, item domain.ReconciliationItem) layout.Widget {
	if item.Value == "" {
		return view.Message(view.Language.Text(i18n.OverviewNoValue), view.Colors.SecondaryInk)
	}
	if item.Kind == "holding" || item.Kind == "removed" && item.HoldingID != "" && item.ProductID == "" {
		return view.Message(fmt.Sprintf(view.Language.Text(i18n.AccountQuantity), portfolioDecimal(view, item.Value)), view.Colors.Ink)
	}
	return view.Message(portfolioDecimal(view, item.Value)+" "+item.Currency, view.Colors.Ink)
}

func holdingCheckEvidence(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	l := view.Language
	rows := []layout.Widget{view.Message(l.Text(holdingCheckOutcome(item)), view.Colors.SecondaryInk)}
	if item.Source.AsOfDate != "" {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconEvidenceDate), holdingEvidenceSourceDate(view, item.Source)), view.Colors.SecondaryInk))
	}
	if item.Source.Reference != "" {
		rows = append(rows, view.Message(item.Source.Reference, view.Colors.Ink))
	}
	if item.ReferenceValue != "" {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconEvidenceValue), item.ReferenceValue), view.Colors.Ink))
	}
	if item.Note != "" {
		rows = append(rows, view.Message(item.Note, view.Colors.Ink))
	}
	return rows
}

func (c *holdingReconciliation) holdingCheckFields(view ui.View, d *holdingReconDraft, origin *image.Point, ready bool) []layout.Widget {
	a, item, ok := c.checkpoint(d.accountID, d.itemID, false)
	if !ok {
		return nil
	}
	l, f := view.Language, d.check
	day, today := c.checkDate(f)
	batch := d.action == statement.Check
	var rows []layout.Widget
	if batch {
		rows = append(rows, view.Message(a.Name+" · "+a.Currency, view.Colors.Ink),
			view.Message(l.Text(i18n.HoldingReconCheckAccountHelp), view.Colors.SecondaryInk))
		for _, candidate := range a.Items {
			if uncheckedHoldingItem(candidate) {
				rows = append(rows, holdingBatchItem(view, candidate))
			}
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if d.initialFocus && ready {
				gtx.Execute(key.FocusCmd{Tag: &f.scopeReviewed})
				d.initialFocus = false
			}
			return checkpointCheckbox(view, &f.scopeReviewed, i18n.HoldingReconScopeReviewed, ready)(gtx)
		})
	} else if item.Kind != "scope" {
		rows = append(rows, view.Message(a.Name+" · "+holdingCheckLabel(view, item), view.Colors.Ink))
		rows = append(rows, holdingCheckpointFacts(view, item)...)
	} else {
		rows = append(rows, view.Message(a.Name+" · "+holdingCheckLabel(view, item), view.Colors.Ink),
			view.Message(l.Text(i18n.HoldingReconScopeHelp), view.Colors.SecondaryInk))
	}
	if c.zone != nil {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), c.zone), view.Colors.SecondaryInk))
	} else {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconSourceZone), view.Colors.SecondaryInk))
	}
	if !batch {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if d.initialFocus {
				gtx.Execute(key.FocusCmd{Tag: &f.picker.Trigger})
				d.initialFocus = false
			}
			f.picker.Layout(gtx, view, ui.PickerField{Label: i18n.HoldingReconOutcome, Origin: *origin, Next: &f.reference.Editor, Disabled: !ready},
				ui.SelectOptions{Items: []ui.Choice{
					{ID: "confirmed", Label: l.Text(i18n.StatementChecked)}, {ID: "skipped", Label: l.Text(i18n.StatementSkipped)},
					{ID: "unresolved", Label: l.Text(i18n.StatementUnresolved)}, {ID: "pending", Label: l.Text(i18n.StatementPending)},
				}}, f.outcome, func(id string) { f.outcome, d.problem = id, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		})
	}
	rows = append(rows,
		func(gtx layout.Context) layout.Dimensions {
			return f.reference.Layout(gtx, view, ui.FieldOptions{Label: i18n.HoldingReconSourceReference, MaxBytes: 512, Disabled: !ready})
		},
		func(gtx layout.Context) layout.Dimensions {
			return c.holdingSourceFields(gtx, view, d, *origin, day, today, ready)
		})
	if !batch && holdingCheckComparable(item) {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return f.value.Layout(gtx, view, ui.FieldOptions{Label: i18n.HoldingReconSourceValue, MaxBytes: 256, Disabled: !ready})
				}),
				layout.Rigid(checkpointCheckbox(view, &f.comparable, i18n.HoldingReconComparable, ready)),
				layout.Rigid(view.Message(l.Text(i18n.HoldingReconComparisonHelp), view.Colors.SecondaryInk)),
			)
		})
	}
	return rows
}

func (c *holdingReconciliation) holdingSourceFields(gtx layout.Context, view ui.View, d *holdingReconDraft, origin image.Point, day, today string, ready bool) layout.Dimensions {
	f := d.check
	week := time.Monday
	if view.WeekStart == "sunday" {
		week = time.Sunday
	}
	gap := gtx.Dp(8)
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			f.datePicker.Layout(gtx, view, ui.PickerField{Label: i18n.HoldingReconSourceDate, Origin: origin, Disabled: !ready || c.zone == nil},
				ui.DateOptions{Today: today, Max: today, Clearable: true, WeekStart: week, Format: view.DateFormat}, day,
				func(value string) { f.day, f.daySet, d.problem = value, true, i18n.NoMessage })
			origin.X += gtx.Constraints.Max.X + gap
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			f.timePicker.Layout(gtx, view, ui.PickerField{Label: i18n.HoldingReconSourceTime, Origin: origin, Disabled: !ready || c.zone == nil},
				f.time, view.TimeFormat == settings.TimeFormat12, true, func(value string) { f.time, d.problem = value, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}))
}

func checkpointCheckbox(view ui.View, box *widget.Bool, label i18n.MessageID, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		return view.Checkbox(box, view.Language.Text(label))(gtx)
	}
}

func holdingReconCounts(view ui.View, s domain.ReconciliationSession) layout.Widget {
	n := s.Counts
	return view.Message(fmt.Sprintf(view.Language.Text(i18n.TasksStatementCounts), n.Pending, n.Unresolved, n.Confirmed, n.Skipped), view.Colors.Ink)
}

func holdingCheckProblem(err error, batch bool) i18n.MessageID {
	var issue *apierror.WireError
	if errors.As(err, &issue) {
		switch issue.Field {
		case "allowPartial":
			return i18n.HoldingReconPartialRequired
		case "referenceValue":
			return i18n.HoldingReconMismatch
		case "referenceComparable":
			return i18n.HoldingReconHistoricalSource
		case "asOfDate", "asOfTime":
			return i18n.HoldingReconSourceInvalid
		case "note":
			return i18n.HoldingReconCheckNoteRequired
		case "itemId":
			return i18n.HoldingReconCheckUnavailable
		case "accountId":
			if batch {
				return i18n.HoldingReconCheckAccountExceptions
			}
		case "scopeReviewed":
			return i18n.HoldingReconScopeReviewed
		}
	}
	return i18n.HoldingReconFailed
}
