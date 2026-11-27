package application

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoryOrigin returns the immutable Starting point. A fresh database has
// no household or origin; initialized sessions validate the origin at bootstrap.
func (s *Service) HistoryOrigin(ctx context.Context) (*domain.HistoryOrigin, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return nil, nil
	}
	return s.repository.HistoryOrigin(ctx, bootstrap.Household.ID)
}

// historyStartInstant resolves an explicitly chosen day in the immutable
// History timezone. Synthetic baseline fixtures may omit the day to capture
// at the fixture clock instant; onboarding always supplies a civil date.
func historyStartInstant(startDate, timezone string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(startDate) == "" {
		return now, nil
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	day := strings.TrimSpace(startDate)
	if len(day) != len("2006-01-02") || day > now.In(location).Format("2006-01-02") {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate", Message: "start date must be a valid day no later than today"}
	}
	label, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate", Message: "start date must be a valid day"}
	}
	return domain.ResolveCivilDateStart(label, location)
}
