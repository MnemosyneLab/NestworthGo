package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Regression test for docs/development/code-review-2026-08-21.md BUG-6:
// sort_order must be assigned atomically inside the INSERT transaction, so
// concurrent CreateMember calls never race on a stale read-then-write value.
func TestCreateMemberAssignsUniqueSortOrderUnderConcurrency(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)
	ctx := context.Background()

	now := time.Now()
	currency, err := domain.ParseCurrency("CNY")
	if err != nil {
		t.Fatalf("ParseCurrency: %v", err)
	}
	household, err := domain.NewHousehold("Test", currency, now)
	if err != nil {
		t.Fatalf("NewHousehold: %v", err)
	}
	seedMember, err := domain.NewMember(household.ID, "Seed", now)
	if err != nil {
		t.Fatalf("NewMember (seed): %v", err)
	}
	if err := repository.CreateOnboarding(ctx, household, []domain.Member{seedMember}); err != nil {
		t.Fatalf("CreateOnboarding: %v", err)
	}

	const concurrentCreates = 8
	var wg sync.WaitGroup
	errs := make(chan error, concurrentCreates)
	for i := 0; i < concurrentCreates; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			member, err := domain.NewMember(household.ID, fmt.Sprintf("Member %d", index), now)
			if err != nil {
				errs <- err
				return
			}
			errs <- repository.CreateMember(ctx, member)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("CreateMember: %v", err)
		}
	}

	members, err := repository.ListMembers(ctx, true)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if want := concurrentCreates + 1; len(members) != want {
		t.Fatalf("member count = %d, want %d", len(members), want)
	}
	seen := make(map[int]bool, len(members))
	for _, member := range members {
		if seen[member.SortOrder] {
			t.Fatalf("duplicate sort_order %d among concurrently created members", member.SortOrder)
		}
		seen[member.SortOrder] = true
	}
}
