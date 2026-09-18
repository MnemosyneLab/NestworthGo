package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

// History starts at economic ownership, not instrument creation or today's quantity.
// Sold/archived holdings still need their past prices; reversed purchases do not.
func (s *Service) instrumentHistoryStarts(ctx context.Context, origin domain.HistoryOrigin, now time.Time) (map[domain.InstrumentID]string, error) {
	components, err := s.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		return nil, err
	}
	activities, err := s.repository.ListActivitiesUntil(ctx, origin.HouseholdID, now)
	if err != nil {
		return nil, err
	}
	return instrumentHistoryStartDates(origin, components, activities)
}

func instrumentHistoryStartDates(origin domain.HistoryOrigin, components []domain.HistoryOriginComponent, activities []domain.Activity) (map[domain.InstrumentID]string, error) {
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return nil, err
	}
	first := origin.StartedAt.In(location).Format("2006-01-02")
	starts := map[domain.InstrumentID]string{}
	record := func(id domain.InstrumentID, date string) {
		if date < first {
			date = first
		}
		if starts[id] == "" || date < starts[id] {
			starts[id] = date
		}
	}
	for _, component := range components {
		if component.Kind == domain.HistoryOriginHoldingQuantity && component.InstrumentID != nil && component.Quantity != nil && !component.Quantity.IsZero() {
			record(*component.InstrumentID, first)
		}
	}
	excluded := map[domain.ActivityID]bool{}
	for _, activity := range activities {
		if activity.ReversesActivityID != nil {
			excluded[activity.ID] = true
			excluded[*activity.ReversesActivityID] = true
		}
	}
	for _, activity := range activities {
		if excluded[activity.ID] {
			continue
		}
		for _, effect := range activity.Effects {
			if effect.Target == domain.EffectTargetHoldingQuantity && effect.Direction == domain.EffectAdded && effect.InstrumentID != nil && effect.Quantity != nil && !effect.Quantity.IsZero() {
				record(*effect.InstrumentID, activity.EffectiveAt.In(location).Format("2006-01-02"))
			}
		}
	}
	return starts, nil
}
