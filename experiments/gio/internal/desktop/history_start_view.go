package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func (p *historyPage) layoutStartingPoint(gtx layout.Context, view ui.View) layout.Dimensions {
	s, l := p.startPoint, view.Language
	gtx = ui.LimitWidth(gtx, 660)
	if s.verifying {
		return p.layoutStartingPointRead(gtx, view)
	}
	if s.write != nil {
		return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
	}
	if s.cancel.Clicked(gtx) {
		s.open, s.day, s.problem, s.preview, s.review = false, "", i18n.NoMessage, nil, nil
		gtx.Execute(key.FocusCmd{Tag: &s.start})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if s.review != nil {
		return p.layoutStartingPointReview(gtx, view)
	}

	zone := view.TimeLocation.String()
	today := p.reads.clock().In(view.TimeLocation).Format(time.DateOnly)
	day := s.day
	if day == "" {
		day = today
	}
	if s.preview == nil && s.prepare.Clicked(gtx) {
		p.previewStartHistory(zone, day)
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, l.Text(i18n.HistoryStartAction), 19, font.Medium, view.Colors.Ink)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(fmt.Sprintf(l.Text(i18n.OnboardingTimezone), zone), view.Colors.SecondaryInk))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s.calendar.Layout(gtx, view, ui.PickerField{Label: i18n.OnboardingDate, Origin: origin, Disabled: s.preview != nil},
				ui.DateOptions{Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}, day, func(value string) {
					s.day, s.problem = value, i18n.NoMessage
				})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Rigid(view.Message(l.Text(i18n.HistoryStartScope), view.Colors.SecondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.problem == i18n.NoMessage {
				return layout.Dimensions{}
			}
			return view.Message(l.Text(s.problem), view.Colors.ErrorInk)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(view.Button(&s.cancel, l.Text(i18n.InstitutionsCancel)))}
			if s.preview == nil {
				children = append(children, layout.Rigid(view.PrimaryButton(&s.prepare, l.Text(i18n.HistoryStartReview))))
			} else {
				children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, children...)
		}),
	)
}

func (p *historyPage) layoutStartingPointReview(gtx layout.Context, view ui.View) layout.Dimensions {
	s, l := p.startPoint, view.Language
	if s.edit.Clicked(gtx) {
		s.review = nil
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if s.confirm.Clicked(gtx) {
		p.confirmStartHistory()
		gtx.Execute(op.InvalidateCmd{})
		return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
	}
	origin := s.review.plan.Origin
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return view.Message(l.Text(i18n.OnboardingDateInvalid), view.Colors.ErrorInk)(gtx)
	}
	start := contributionTimestamp(view, origin.StartedAt, location)
	if s.review.plan.CaptureCurrent {
		day := historyDay(view, origin.StartedAt.In(location).Format(time.DateOnly))
		start = fmt.Sprintf(l.Text(i18n.HistoryStartCaptureNow), day, origin.Timezone)
	}
	rows := []layout.Widget{
		view.Message(start, view.Colors.Ink),
		view.Message(l.Text(i18n.HistoryStartBaseline), view.Colors.SecondaryInk),
	}
	for _, part := range s.review.plan.Components {
		if part.Amount == nil || part.AccountID == nil {
			continue
		}
		account := s.review.accounts[*part.AccountID]
		name := account.Name
		if account.ArchivedAt != nil {
			name += " · " + l.Text(i18n.DirectoryArchived)
		}
		rows = append(rows, view.Message(name+" · "+part.Amount.CanonicalAmount()+" "+part.Amount.Currency().String(), view.Colors.Ink))
	}
	if len(rows) == 2 {
		rows = append(rows, view.Message(l.Text(i18n.HistoryStartEmpty), view.Colors.SecondaryInk))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.HistoryStartReview), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(view.Theme, &s.balances).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, rows[index])
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&s.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.Button(&s.edit, l.Text(i18n.HistoryStartEdit))),
				layout.Rigid(view.PrimaryButton(&s.confirm, l.Text(i18n.HistoryStartConfirm))),
			)
		}),
	)
}

func (p *historyPage) layoutStartingPointRead(gtx layout.Context, view ui.View) layout.Dimensions {
	s, l := p.startPoint, view.Language
	if s.read == nil && s.retry.Clicked(gtx) {
		p.readStartingPoint()
		gtx.Execute(op.InvalidateCmd{})
	}
	var children []layout.FlexChild
	if s.saved {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryStartSaved), view.Colors.Ink)))
	}
	if s.read != nil {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	} else {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryStartReadFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&s.retry, l.Text(i18n.HistoryRetry))))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
