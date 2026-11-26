package pilot

import (
	"context"
	"os"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPilotHistoryPagesAfterReopen(t *testing.T) {
	_, paths := testPaths(t)
	ctx := context.Background()
	if err := seedTestLedger(ctx, paths); err != nil {
		t.Fatal(err)
	}
	// Each request uses a newly opened handle; the keyset continuation must
	// remain valid after closing the previous handle to the fixture.
	read := func(query domain.ActivityQuery) domain.ActivityPage {
		t.Helper()
		session, err := OpenSession(ctx, paths)
		if err != nil {
			t.Fatal(err)
		}
		page, err := application.NewService(sqlite.NewRepository(session.DB)).ListActivityPage(ctx, query)
		closeErr := session.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read activity page: %v; close: %v", err, closeErr)
		}
		return page
	}
	want := []struct {
		date, note string
		kind       domain.ActivityKind
	}{
		{"2026-11-12", "Bank interest", domain.ActivityCashIn},
		{"2026-11-10", "Home utilities", domain.ActivityCashOut},
		{"2026-11-08", "Workshop honorarium", domain.ActivityCashIn},
		{"2026-11-05", "Transit pass", domain.ActivityCashOut},
		{"2026-11-03", "Groceries", domain.ActivityCashOut},
		{"2026-11-02", "November pay", domain.ActivityCashIn},
	}
	query := domain.ActivityQuery{Limit: 2}
	for offset := 0; offset < len(want); offset += query.Limit {
		page := read(query)
		if len(page.Activities) != query.Limit {
			t.Fatalf("page at %d: got %d activities", offset, len(page.Activities))
		}
		for i, activity := range page.Activities {
			record := want[offset+i]
			if activity.Note == nil || *activity.Note != record.note || activity.EffectiveLocalDate != record.date || activity.Kind != record.kind {
				t.Fatalf("activity %d: got %+v, want %+v", offset+i, activity, record)
			}
		}
		more := offset+query.Limit < len(want)
		if page.HasMore != more || (page.Next != nil) != more {
			t.Fatalf("page at %d: HasMore=%t Next=%v", offset, page.HasMore, page.Next)
		}
		query.After = page.Next
	}
	// The last credit is on the 11th in UTC, but the household saw it on the 12th.
	localDay := read(domain.ActivityQuery{FromLocalDate: "2026-11-12", ToLocalDate: "2026-11-12", Limit: 2})
	if len(localDay.Activities) != 1 {
		t.Fatalf("household-day filter: got %d activities", len(localDay.Activities))
	}
	interest := localDay.Activities[0]
	if interest.Reason != domain.ReasonInterest || interest.EffectiveAt.UTC().Day() != 11 || interest.Note == nil || *interest.Note != "Bank interest" {
		t.Fatalf("household-day filter returned %+v", interest)
	}
}

func TestOpenEarlierInstitutionsFixtureWithoutHistory(t *testing.T) {
	_, paths := testPaths(t)
	ctx := t.Context()
	if err := os.Mkdir(paths.data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := settings.NewStore(paths.settings).Save(settings.Default()); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := application.NewService(sqlite.NewRepository(db))
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Ordinary household", BaseCurrency: "USD", MemberNames: []string{"Sample member"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	page, err := application.NewService(sqlite.NewRepository(reopened.DB)).ListActivityPage(ctx, domain.ActivityQuery{Limit: 2})
	if err != nil || len(page.Activities) != 0 || page.HasMore || page.Next != nil {
		t.Fatalf("expected existing history to remain empty, got %+v and %v", page, err)
	}
}
