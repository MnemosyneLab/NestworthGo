package application

import (
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const providerPersistenceTimestampLayout = "2006-01-02T15:04:05.000Z07:00"

// NormalizeProviderObservationTime validates and canonicalizes an externally
// supplied observation time before it can enter the domain or persistence.
// SQLite stores portfolio timestamps at millisecond precision, so the
// round-trip check is intentionally performed against that exact format.
func NormalizeProviderObservationTime(value, now time.Time) (time.Time, error) {
	if value.IsZero() || now.IsZero() {
		return time.Time{}, errors.New("provider observation time is invalid")
	}
	value = value.UTC()
	now = now.UTC()
	if value.Before(domain.ProviderObservationEarliest()) || value.After(now.Add(domain.QuoteClockSkewTolerance)) {
		return time.Time{}, errors.New("provider observation time is outside the supported window")
	}
	canonical := value.Truncate(time.Millisecond).Format(providerPersistenceTimestampLayout)
	parsed, err := time.Parse(time.RFC3339Nano, canonical)
	if err != nil || !parsed.Equal(value.Truncate(time.Millisecond)) {
		return time.Time{}, errors.New("provider observation time cannot be persisted")
	}
	return parsed.UTC(), nil
}
