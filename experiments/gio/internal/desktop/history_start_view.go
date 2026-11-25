package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

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
		s.loading, s.holding, s.cost = nil, nil, ui.NewTextField(true)
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
	if s.loading == nil && s.draftProblem != i18n.NoMessage && s.retry.Clicked(gtx) {
		p.loadStartingPointHolding()
		gtx.Execute(op.InvalidateCmd{})
	}
	canReview := func() bool {
		return s.loading == nil && s.draftProblem == i18n.NoMessage && s.preview == nil &&
			(s.holding == nil || strings.TrimSpace(s.cost.Text()) != "")
	}
	if canReview() && s.prepare.Clicked(gtx) {
		p.previewStartHistory(zone, day)
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	children := []layout.FlexChild{
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
	}
	if s.holding != nil && s.draftProblem == i18n.NoMessage {
		holding := s.holding
		children = append(children,
			layout.Rigid(view.Message(holding.InstrumentName+" · "+fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, holding.Quantity)), view.Colors.Ink)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.cost.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HistoryStartOriginalCost,
					Help: i18n.HistoryStartOriginalCostHelp, Disabled: s.preview != nil}, holding.Currency.String())
			}))
	}
	problem := s.problem
	if s.draftProblem != i18n.NoMessage {
		problem = s.draftProblem
	}
	if problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&s.cancel, l.Text(i18n.InstitutionsCancel)))}
		switch {
		case s.loading != nil || s.preview != nil:
			buttons = append(buttons, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
		case s.draftProblem != i18n.NoMessage:
			buttons = append(buttons, layout.Rigid(view.Button(&s.retry, l.Text(i18n.HistoryRetry))))
		default:
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canReview() {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&s.prepare, l.Text(i18n.HistoryStartReview))(gtx)
			}))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
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
	editLabel := i18n.HistoryStartEdit
	if s.review.holding != nil {
		editLabel = i18n.AccountCreateBackToDetails
	}
	if s.review.plan.CaptureCurrent {
		day := historyDay(view, origin.StartedAt.In(location).Format(time.DateOnly))
		start = fmt.Sprintf(l.Text(i18n.HistoryStartCaptureNow), day, origin.Timezone)
	}
	rows := []layout.Widget{
		view.Message(start, view.Colors.Ink),
		view.Message(l.Text(i18n.HistoryStartBaseline), view.Colors.SecondaryInk),
	}
	for _, part := range s.review.plan.Components {
		if part.AccountID == nil {
			continue
		}
		account := s.review.accounts[*part.AccountID]
		name := account.Name
		if account.ArchivedAt != nil {
			name += " · " + l.Text(i18n.DirectoryArchived)
		}
		if part.Amount != nil {
			rows = append(rows, view.Message(name+" · "+part.Amount.CanonicalAmount()+" "+part.Amount.Currency().String(), view.Colors.Ink))
		}
		if holding := s.review.holding; holding != nil && part.UnitCost != nil && part.Quantity != nil && part.HoldingID != nil && *part.HoldingID == holding.HoldingID {
			rows = append(rows, view.Message(name+" · "+holding.InstrumentName, view.Colors.Ink),
				view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, part.Quantity.Canonical())), view.Colors.Ink),
				view.Message(fmt.Sprintf(l.Text(i18n.HistoryStartReviewedCost), portfolioDecimal(view, part.UnitCost.Canonical())+" "+holding.Currency.String()), view.Colors.Ink))
		}
	}
	if len(rows) == 2 {
		rows = append(rows, view.Message(l.Text(i18n.HistoryStartEmpty), view.Colors.SecondaryInk))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.HistoryStartReview), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return view.List(&s.balances).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, rows[index])
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&s.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.Button(&s.edit, l.Text(editLabel))),
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
