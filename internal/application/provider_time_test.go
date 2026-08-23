package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestNormalizeProviderObservationTimeBoundsAndRoundTrips(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 123456000, time.FixedZone("SGT", 8*60*60))
	tests := []struct {
		name    string
		value   time.Time
		wantErr bool
	}{
		{name: "earliest", value: domain.ProviderObservationEarliest()},
		{name: "future tolerance", value: now.Add(domain.QuoteClockSkewTolerance)},
		{name: "future out of range", value: now.Add(domain.QuoteClockSkewTolerance + time.Millisecond), wantErr: true},
		{name: "largest unix value", value: time.Unix(1<<63-1, 0), wantErr: true},
		{name: "before lower bound", value: domain.ProviderObservationEarliest().Add(-time.Nanosecond), wantErr: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := NormalizeProviderObservationTime(testCase.value, now)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("NormalizeProviderObservationTime(%v) error = nil", testCase.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeProviderObservationTime(%v): %v", testCase.value, err)
			}
			if got.Nanosecond()%int(time.Millisecond) != 0 || got.Location() != time.UTC {
				t.Fatalf("normalized time = %v, want UTC millisecond precision", got)
			}
		})
	}
}
