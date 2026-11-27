package desktop

import (
	"fmt"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fundsActionKind uint8

const (
	fundsActionPolicy fundsActionKind = iota + 1
	fundsActionReservations
)

// A visit carries source identity by value even when the projection is reloaded.
type fundsActionFocus struct {
	kind        fundsActionKind
	sourceKey   string
	account     domain.AccountID
	reservation domain.LiquidityReservationID
}

func upcomingFundsFocus(target domain.UpcomingTarget) fundsActionFocus {
	var focus fundsActionFocus
	switch target.Kind {
	case "source_policy":
		if target.SourceRef == nil || target.SourceRef.Validate() != nil {
			return focus
		}
		focus.kind = fundsActionPolicy
	case "reservations":
		if target.SourceRef != nil && target.SourceRef.Validate() != nil {
			return focus
		}
		focus.kind = fundsActionReservations
		if target.ReservationID != nil {
			focus.reservation = *target.ReservationID
		}
	default:
		return focus
	}
	if target.SourceRef != nil {
		focus.sourceKey, focus.account = target.SourceRef.Key(), target.SourceRef.AccountID
	}
	return focus
}

type upcomingActionLinkKey struct {
	row   string
	funds fundsActionFocus
}

type upcomingActionLinks struct {
	openFunds  func(fundsActionFocus, event.Tag)
	openHealth func(healthFindingFocus, event.Tag)
	buttons    map[upcomingActionLinkKey]*widget.Clickable
}

func (l *upcomingActionLinks) retained(items []domain.UpcomingAction) {
	visible := make(map[string]bool, len(items))
	for _, row := range items {
		visible[row.ID] = true
	}
	for key := range l.buttons {
		if !visible[key.row] {
			delete(l.buttons, key)
		}
	}
}

func (l *upcomingActionLinks) layoutLinks(view ui.View, row domain.UpcomingAction) []layout.Widget {
	var links []layout.Widget
	if l.openFunds == nil && l.openHealth == nil {
		return links
	}
	seen := make(map[upcomingActionLinkKey]bool)
	for _, target := range append([]domain.UpcomingTarget{row.PrimaryTarget}, row.Targets...) {
		focus := upcomingFundsFocus(target)
		var name string
		var open func(event.Tag)
		switch {
		case focus.kind == fundsActionPolicy && l.openFunds != nil:
			name = fmt.Sprintf(view.Language.Text(i18n.TasksPolicyLink), row.DisplayName)
			open = func(button event.Tag) { l.openFunds(focus, button) }
		case focus.kind == fundsActionReservations && l.openFunds != nil:
			name = fmt.Sprintf(view.Language.Text(i18n.TasksReservationLink), row.DisplayName)
			open = func(button event.Tag) { l.openFunds(focus, button) }
		case target.Kind == "data_health" && l.openHealth != nil:
			name = view.Language.Text(i18n.HealthOpen) + " · " + row.DisplayName
			open = func(button event.Tag) { l.openHealth(healthFindingFocus{}, button) }
		default:
			continue
		}
		key := upcomingActionLinkKey{row: row.ID, funds: focus}
		if seen[key] {
			continue
		}
		seen[key] = true
		if l.buttons == nil {
			l.buttons = make(map[upcomingActionLinkKey]*widget.Clickable)
		}
		button := portfolioButton(l.buttons, key)
		links = append(links, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				open(button)
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, button)
			return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: name}))
		})
	}
	return links
}

func taskReceiptDue(row domain.UpcomingAction) bool {
	return row.Kind == "product_schedule" && row.ProductKind != nil && *row.ProductKind == domain.ProductTermDeposit && row.Phase == "receipt_due"
}

func upcomingAmountLabel(role string) i18n.MessageID {
	switch role {
	case "reserve_shortfall":
		return i18n.FundsShortfall
	case "reservation_requested":
		return i18n.FundsRequested
	default:
		return i18n.ProductForecastNet
	}
}
