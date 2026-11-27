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
	household, err := s.repository.Household(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return nil, nil
	}
	return s.requireHistoryOrigin(ctx, household.ID)
}

func (s *Service) requireHistoryOrigin(ctx context.Context, id domain.HouseholdID) (*domain.HistoryOrigin, error) {
	origin, err := s.repository.HistoryOrigin(ctx, id)
	if err != nil {
		return nil, err
	}
	if origin == nil {
		return nil, missingHistoryOrigin()
	}
	return origin, nil
}

func missingHistoryOrigin() error {
	return &domain.Error{Code: domain.ErrMissingHistoryOrigin, Message: "ledger history origin is missing"}
}

// historyStartInstant resolves an explicitly chosen day in the immutable
// History timezone.
func historyStartInstant(startDate, timezone string, now time.Time) (time.Time, error) {
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
