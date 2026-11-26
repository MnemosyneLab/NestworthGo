package desktop

import (
	"context"
	"errors"
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type settingsDataRebuild struct {
	cancelLatest func()
	run          func(context.Context) (application.DerivedDataRebuildResult, error)
	rebuild      widget.Clickable
	request      <-chan historyResult[application.DerivedDataRebuildResult]
	result       *application.DerivedDataRebuildResult
	problem      i18n.MessageID
}

func (d *settingsDataRebuild) update() {
	select {
	case response := <-d.request:
		d.request = nil
		var problem *domain.Error
		switch {
		case response.err == nil:
			d.result = &response.value
		case errors.As(response.err, &problem) && problem.Code == domain.ErrHistoryNotStarted:
			d.problem = i18n.SettingsDataHistoryRequired
		default:
			d.problem = i18n.SettingsDataRebuildError
		}
	default:
	}
}

func (d *settingsDataRebuild) card(gtx layout.Context, view ui.View, prefs *preferenceState) layout.Dimensions {
	l := view.Language
	return settingsCard(gtx, view, l.Text(i18n.SettingsDataRebuildTitle), func(gtx layout.Context, view ui.View) layout.Dimensions {
		if d.run != nil && d.request == nil && !prefs.busy && d.rebuild.Clicked(gtx) {
			d.result, d.problem = nil, i18n.NoMessage
			if d.cancelLatest != nil {
				d.cancelLatest()
			}
			d.request = runHistoryRead(&prefs.workers, prefs.invalidate, func() (application.DerivedDataRebuildResult, error) {
				return d.run(prefs.ctx)
			})
		}
		rows := []ui.ColumnChild{
			ui.Rigid(view.Message(l.Text(i18n.SettingsDataRebuildDescription), view.Colors.SecondaryInk)),
			ui.Rigid(view.Message(l.Text(i18n.SettingsDataRebuildIncomplete), view.Colors.SecondaryInk)),
		}
		if d.run != nil {
			label := i18n.SettingsDataRebuildAction
			if d.request != nil {
				label = i18n.SettingsDataRebuilding
			}
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if prefs.busy || d.request != nil {
					gtx = gtx.Disabled()
				}
				return layout.W.Layout(gtx, view.Button(&d.rebuild, l.Text(label)))
			}))
		}
		switch {
		case d.request != nil:
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.SettingsDataRebuildProgress), view.Colors.SecondaryInk)))
		case d.problem != i18n.NoMessage:
			rows = append(rows, ui.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
		case d.result != nil:
			result := d.result
			rows = append(rows,
				ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.SettingsDataRebuildComplete), view.Colors.Ink)),
				ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.SettingsDataRebuildRange), healthDateRange(view, result.From, result.To)), view.Colors.SecondaryInk)),
				ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.SettingsDataRebuildCounts), result.SnapshotDays, result.IncompleteDays), view.Colors.SecondaryInk)),
			)
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	})
}
