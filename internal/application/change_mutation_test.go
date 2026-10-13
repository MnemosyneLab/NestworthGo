package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestRecordChangeWithMutationReplaysSamePayloadAndConflictsOnMismatch(t *testing.T) {
	t.Parallel()
	database, err := sqlite.Open(t.TempDir() + "/mutation.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	originNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := originNow
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Mutations", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = originNow.Add(24 * time.Hour)
	amount, _ := domain.ParseMoney("100", "USD")
	command := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock}
	mutationID := domain.NewMutationID().String()
	hash := strings.Repeat("ab", 32)

	first, err := service.RecordChangeWithMutation(ctx, command, mutationID, hash)
	if err != nil {
		t.Fatalf("first RecordChangeWithMutation: %v", err)
	}
	replay, err := service.RecordChangeWithMutation(ctx, command, mutationID, hash)
	if err != nil {
		t.Fatalf("replay RecordChangeWithMutation: %v", err)
	}
	if replay.Activity.ID != first.Activity.ID {
		t.Fatalf("replay activity ID = %s, want %s", replay.Activity.ID, first.Activity.ID)
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 1 {
		t.Fatalf("activities = %d, want 1 after same-payload retry", activities)
	}

	otherHash := strings.Repeat("cd", 32)
	_, err = service.RecordChangeWithMutation(ctx, command, mutationID, otherHash)
	if err == nil {
		t.Fatal("expected conflict for same mutation ID and different payload hash")
	}
	domainErr, ok := err.(*domain.Error)
	if !ok || domainErr.Code != domain.ErrConflict || domainErr.Field != "mutationId" {
		t.Fatalf("mismatch error = %v, want conflict mutationId", err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 1 {
		t.Fatalf("activities = %d, want 1 after conflicting retry", activities)
	}
}

func TestFixChangeWithMutationStoresKeyOnReplacement(t *testing.T) {
	t.Parallel()
	database, err := sqlite.Open(t.TempDir() + "/fix-mutation.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	originNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := originNow
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Fix", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = originNow.Add(24 * time.Hour)
	amount, _ := domain.ParseMoney("100", "USD")
	original, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	replacementAmount, _ := domain.ParseMoney("150", "USD")
	replacement := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: replacementAmount, Reason: domain.ReasonIncome, EffectiveAt: clock}
	mutationID := domain.NewMutationID().String()
	hash := strings.Repeat("ef", 32)
	fixed, err := service.FixChangeWithMutation(ctx, original.Activity.ID, replacement, mutationID, hash)
	if err != nil {
		t.Fatalf("FixChangeWithMutation: %v", err)
	}
	replay, err := service.FixChangeWithMutation(ctx, original.Activity.ID, replacement, mutationID, hash)
	if err != nil {
		t.Fatalf("replay FixChangeWithMutation: %v", err)
	}
	if replay.Activity.ID != fixed.Activity.ID {
		t.Fatalf("replay activity ID = %s, want replacement %s", replay.Activity.ID, fixed.Activity.ID)
	}
	var keys, activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_mutation_keys").Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if keys != 1 {
		t.Fatalf("mutation keys = %d, want 1", keys)
	}
	if activities != 3 {
		t.Fatalf("activities = %d, want 3 (original, inverse, replacement)", activities)
	}
}

func TestParseActivityMutationRejectsInvalidHash(t *testing.T) {
	t.Parallel()
	_, err := parseActivityMutation(domain.NewMutationID().String(), "not-a-hash")
	if err == nil {
		t.Fatal("expected invalid payload hash to fail")
	}
	domainErr, ok := err.(*domain.Error)
	if !ok || domainErr.Code != domain.ErrValidation {
		t.Fatalf("err = %v, want validation", err)
	}
}

type fixSnapshotFailureRepository struct {
	*sqlite.Repository
	failAfterCommit bool
	committed       bool
}

func (r *fixSnapshotFailureRepository) CommitActivityBatch(ctx context.Context, commits []domain.ActivityCommit, at time.Time) error {
	if err := r.Repository.CommitActivityBatch(ctx, commits, at); err != nil {
		return err
	}
	r.committed = true
	return nil
}

func (r *fixSnapshotFailureRepository) DailySnapshotState(ctx context.Context, id domain.HouseholdID) (domain.DailySnapshotState, error) {
	if r.failAfterCommit && r.committed {
		return domain.DailySnapshotState{}, &domain.Error{Code: domain.ErrConflict, Field: "inputGeneration", Message: "injected postcommit snapshot failure"}
	}
	return r.Repository.DailySnapshotState(ctx, id)
}

func TestFixMutationRetryRecoversPostcommitSnapshotDomainError(t *testing.T) {
	t.Parallel()
	database, err := sqlite.Open(t.TempDir() + "/fix-recovery.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := &fixSnapshotFailureRepository{Repository: sqlite.NewRepository(database)}
	service := NewService(repo)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := t.Context()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic recovery", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Synthetic Cash", IncludeInNetWorth: true, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = clock.AddDate(0, 0, 1)
	amount, _ := domain.ParseMoney("100", "USD")
	command := domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock}
	original, err := service.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.AddDate(0, 0, 2)
	if _, err := service.RebuildHistoricalSnapshots(ctx, "2026-01-01", "2026-01-03"); err != nil {
		t.Fatal(err)
	}
	command.Amount, _ = domain.ParseMoney("150", "USD")
	id := domain.NewMutationID()
	hash := strings.Repeat("ac", 32)
	repo.failAfterCommit = true
	fixed, err := service.FixChangeWithMutation(ctx, original.Activity.ID, command, id.String(), hash)
	if problem, ok := err.(*domain.Error); !ok || problem.Code != domain.ErrConflict || problem.Field != "inputGeneration" {
		t.Fatalf("expected postcommit domain error; got %v", err)
	}
	assertCounts := func() {
		t.Helper()
		var activities, keys int
		if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
			t.Fatal(err)
		}
		if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_mutation_keys").Scan(&keys); err != nil {
			t.Fatal(err)
		}
		if activities != 3 || keys != 1 {
			t.Fatalf("activities=%d keys=%d; want one original, inverse, replacement and key", activities, keys)
		}
	}
	assertCounts()
	// A same-ID replay must remain recoverable while maintenance still fails.
	if _, err := service.FixChangeWithMutation(ctx, original.Activity.ID, command, id.String(), hash); err == nil {
		t.Fatal("expected maintenance error on receipt replay")
	}
	assertCounts()
	repo.failAfterCommit = false
	recovered, err := service.FixChangeWithMutation(ctx, original.Activity.ID, command, id.String(), hash)
	if err != nil || recovered.Activity.ID != fixed.Activity.ID {
		t.Fatalf("same-ID recovery=%+v error=%v; want original replacement %s", recovered, err, fixed.Activity.ID)
	}
	assertCounts()
	state, err := repo.DailySnapshotState(ctx, boot.Household.ID)
	if err != nil || state.DirtyFrom != nil || (state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-01-03") {
		t.Fatalf("maintenance did not finish: state=%+v error=%v", state, err)
	}
	snapshots, err := repo.ListDailyValuationSnapshots(ctx, boot.Household.ID, time.Time{}, time.Time{})
	if err != nil || len(snapshots) != 3 {
		t.Fatalf("snapshots=%+v error=%v; want three closed days", snapshots, err)
	}
	for _, snapshot := range snapshots {
		if snapshot.LocalDate >= "2026-01-02" && (snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "1150") {
			t.Fatalf("snapshot %s=%+v; want corrected 1150", snapshot.LocalDate, snapshot)
		}
	}
}
