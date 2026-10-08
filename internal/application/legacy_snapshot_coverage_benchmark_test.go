package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"testing"
	"time"
)

func perfCoverageService(b *testing.B) *Service {
	b.Helper()
	db, err := sqlite.Open(b.TempDir() + "/synthetic.db")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(b.Context(), OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		b.Fatal(err)
	}
	bootstrap, err := s.Bootstrap(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	if _, err := s.CreateAccount(b.Context(), AccountInput{Name: "Synthetic cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}}); err != nil {
		b.Fatal(err)
	}
	if _, err := s.StartHistory(b.Context(), "UTC"); err != nil {
		b.Fatal(err)
	}
	now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	return s
}
func BenchmarkLegacySnapshotCoverageWarmBoundedTrend(b *testing.B) {
	s := perfCoverageService(b)
	if _, err := s.NetWorthTrend(b.Context(), domain.TrendAllTime); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.NetWorthTrend(b.Context(), domain.TrendRange("2026-08-01:2026-08-02")); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkLegacySnapshotCoverageColdBoundedTrend(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s := perfCoverageService(b)
		b.StartTimer()
		if _, err := s.NetWorthTrend(b.Context(), domain.TrendRange("2026-08-01:2026-08-02")); err != nil {
			b.Fatal(err)
		}
	}
}
