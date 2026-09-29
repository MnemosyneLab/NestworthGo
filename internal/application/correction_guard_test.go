package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func guardedCorrectionFixture(t *testing.T) (*Service, *sqlite.DB, domain.MoneyAddedInput, domain.ActivityID, *time.Time) {
	t.Helper()
	database, err := sqlite.Open(t.TempDir() + "/guarded-correction.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Corrections", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
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
	now = now.Add(24 * time.Hour)
	amount, err := domain.ParseMoney("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	command := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: now}
	original, err := service.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	return service, database, command, original.Activity.ID, &now
}

func requireCorrectionError(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	got, ok := err.(*domain.Error)
	if !ok || got.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func activityCount(t *testing.T, database *sqlite.DB) int {
	t.Helper()
	var count int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestGuardedFixRejectsStalePreviewAndReplaysAfterRestart(t *testing.T) {
	service, database, command, originalID, now := guardedCorrectionFixture(t)
	ctx := context.Background()
	amount, _ := domain.ParseMoney("150", "USD")
	replacement := command
	replacement.Amount = amount
	preview, token, err := service.PreviewFixChangeGuardedWith(ctx, originalID, func(context.Context) (any, error) { return replacement, nil })
	if err != nil || token == "" || activityCount(t, database) != 1 {
		t.Fatalf("guarded fix preview: %+v, token=%q, err=%v", preview, token, err)
	}
	*now = now.Add(time.Hour)
	extraAmount, _ := domain.ParseMoney("10", "USD")
	extra := command
	extra.Amount = extraAmount
	extra.EffectiveAt = *now
	if _, err := service.RecordChange(ctx, extra); err != nil {
		t.Fatal(err)
	}
	id := domain.NewMutationID().String()
	hash := strings.Repeat("ab", 32)
	_, err = service.RecordFixChangeGuarded(ctx, originalID, replacement, id, hash, token)
	requireCorrectionError(t, err, domain.ErrStalePreview)
	if count := activityCount(t, database); count != 2 {
		t.Fatalf("stale fix wrote activities: %d", count)
	}
	preview, token, err = service.PreviewFixChangeGuardedWith(ctx, originalID, func(context.Context) (any, error) { return replacement, nil })
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := service.RecordFixChangeGuarded(ctx, originalID, replacement, id, hash, token)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Resulting) != len(fixed.Resulting) || preview.Resulting[0].Amount != fixed.Resulting[0].Amount {
		t.Fatalf("preview resulting = %+v; commit resulting = %+v", preview.Resulting, fixed.Resulting)
	}
	if count := activityCount(t, database); count != 4 {
		t.Fatalf("activities = %d, want original, extra, inverse, replacement", count)
	}
	restarted := NewService(sqlite.NewRepository(database))
	restarted.setClock(func() time.Time { return *now })
	replay, err := restarted.RecordFixChangeGuarded(ctx, originalID, replacement, id, hash, token)
	if err != nil || replay.Activity.ID != fixed.Activity.ID || activityCount(t, database) != 4 {
		t.Fatalf("restart replay = %+v, err=%v", replay, err)
	}
	_, err = restarted.RecordFixChangeGuarded(ctx, originalID, replacement, id, strings.Repeat("cd", 32), token)
	requireCorrectionError(t, err, domain.ErrConflict)
}

func TestGuardedUndoUsesCurrentStateAndDurableReplay(t *testing.T) {
	service, database, command, originalID, now := guardedCorrectionFixture(t)
	ctx := context.Background()
	_, token, err := service.PreviewUndoChangeGuarded(ctx, originalID)
	if err != nil || activityCount(t, database) != 1 {
		t.Fatalf("guarded undo preview token=%q err=%v", token, err)
	}
	*now = now.Add(time.Hour)
	extraAmount, _ := domain.ParseMoney("10", "USD")
	extra := command
	extra.Amount = extraAmount
	extra.EffectiveAt = *now
	if _, err := service.RecordChange(ctx, extra); err != nil {
		t.Fatal(err)
	}
	id := domain.NewMutationID().String()
	hash := strings.Repeat("ef", 32)
	_, err = service.RecordUndoChangeGuarded(ctx, originalID, id, hash, token)
	requireCorrectionError(t, err, domain.ErrStalePreview)
	preview, token, err := service.PreviewUndoChangeGuarded(ctx, originalID)
	if err != nil {
		t.Fatal(err)
	}
	reviewedAt := preview.Activity.EffectiveAt
	*now = now.Add(time.Hour)
	undo, err := service.RecordUndoChangeGuardedAt(ctx, originalID, reviewedAt, id, hash, token)
	if err != nil {
		t.Fatal(err)
	}
	if !undo.Activity.EffectiveAt.Equal(reviewedAt) || !undo.Activity.CreatedAt.Equal(*now) {
		t.Fatalf("reversal effective/audit time = %v / %v, want %v / %v", undo.Activity.EffectiveAt, undo.Activity.CreatedAt, reviewedAt, *now)
	}
	if undo.Activity.ReversesActivityID == nil || *undo.Activity.ReversesActivityID != originalID || len(preview.Resulting) != len(undo.Resulting) || preview.Resulting[0].Amount != undo.Resulting[0].Amount {
		t.Fatalf("undo preview = %+v; commit = %+v", preview, undo)
	}
	if undo.Resulting[0].Amount != "1010" || activityCount(t, database) != 3 {
		t.Fatalf("undo resulting = %+v", undo.Resulting)
	}
	restarted := NewService(sqlite.NewRepository(database))
	restarted.setClock(func() time.Time { return *now })
	replay, err := restarted.RecordUndoChangeGuarded(ctx, originalID, id, hash, token)
	if err != nil || replay.Activity.ID != undo.Activity.ID || activityCount(t, database) != 3 {
		t.Fatalf("undo replay = %+v, err=%v", replay, err)
	}
	_, err = restarted.RecordUndoChangeGuarded(ctx, originalID, domain.NewMutationID().String(), hash, token)
	requireCorrectionError(t, err, domain.ErrStalePreview)
	_, freshToken, err := restarted.PreviewUndoChangeGuarded(ctx, originalID)
	requireCorrectionError(t, err, domain.ErrAlreadyUndone)
	if freshToken != "" {
		t.Fatalf("undone activity returned token %q", freshToken)
	}
}

func TestGuardedFixRejectsReplacementThatNeedsNewHolding(t *testing.T) {
	service, database, _, originalID, _ := guardedCorrectionFixture(t)
	_, _, err := service.PreviewFixChangeGuardedWith(context.Background(), originalID, func(context.Context) (any, error) {
		return domain.PositionImportInput{}, nil
	})
	requireCorrectionError(t, err, domain.ErrCannotFixChange)
	if count := activityCount(t, database); count != 1 {
		t.Fatalf("unsupported preview wrote activities: %d", count)
	}
}

func TestGuardedFixRollsBackInverseWhenReplacementInsertFails(t *testing.T) {
	service, database, command, originalID, _ := guardedCorrectionFixture(t)
	ctx := context.Background()
	amount, _ := domain.ParseMoney("150", "USD")
	replacement := command
	replacement.Amount = amount
	_, token, err := service.PreviewFixChangeGuardedWith(ctx, originalID, func(context.Context) (any, error) { return replacement, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.Exec(`CREATE TRIGGER reject_correction_replacement BEFORE INSERT ON activities WHEN NEW.correction_group_id IS NOT NULL AND NEW.reverses_activity_id IS NULL BEGIN SELECT RAISE(ABORT, 'blocked replacement'); END`); err != nil {
		t.Fatal(err)
	}
	id := domain.NewMutationID().String()
	hash := strings.Repeat("ab", 32)
	if _, err := service.RecordFixChangeGuarded(ctx, originalID, replacement, id, hash, token); err == nil {
		t.Fatal("replacement insert failure should abort correction")
	}
	if got := activityCount(t, database); got != 1 {
		t.Fatalf("partial inverse survived rollback: activities=%d", got)
	}
	var mutationCount int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_mutation_keys WHERE mutation_id=?", id).Scan(&mutationCount); err != nil {
		t.Fatal(err)
	}
	if mutationCount != 0 {
		t.Fatalf("failed correction stored mutation key: %d", mutationCount)
	}
	state, err := service.changeState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if balance := state.Accounts[command.AccountID].Current.CanonicalAmount(); balance != "1100" {
		t.Fatalf("failed correction changed balance to %s", balance)
	}
}
