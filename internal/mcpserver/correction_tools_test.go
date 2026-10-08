package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type failedCorrectionSnapshotRepository struct {
	*sqlite.Repository
	fail atomic.Bool
}

func (r *failedCorrectionSnapshotRepository) LoadHistoricalSnapshotBatch(ctx context.Context, householdID domain.HouseholdID, until time.Time) (domain.HistoricalSnapshotBatch, error) {
	if r.fail.Load() {
		return domain.HistoricalSnapshotBatch{}, &domain.Error{Code: domain.ErrUnavailable, Message: "injected snapshot rebuild failure"}
	}
	return r.Repository.LoadHistoricalSnapshotBatch(ctx, householdID, until)
}

func correctionPreview(t *testing.T, c *mcp.ClientSession, input map[string]any) map[string]any {
	t.Helper()
	return call(t, c, "preview_correction", input, false)["data"].(map[string]any)
}

func correctionCommit(t *testing.T, c *mcp.ClientSession, operationID, planID string) map[string]any {
	t.Helper()
	receipt := call(t, c, "commit_correction", map[string]any{
		"operationId": operationID, "input": map[string]any{"planId": planID},
	}, false)["data"].(map[string]any)
	if receipt["status"] != "succeeded" {
		t.Fatalf("correction receipt: %+v", receipt)
	}
	return receipt["result"].(map[string]any)
}

func TestCorrectionToolsRequireLedgerWrite(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		c := connect(t, fx.service)
		listed, err := c.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, tool := range listed.Tools {
			found[tool.Name] = true
		}
		if found["preview_correction"] != (mode == LedgerWrite) || found["commit_correction"] != (mode == LedgerWrite) {
			t.Fatalf("mode=%s correction tools=%v", mode, found)
		}
	}
}

func TestMCPHistoricalFixKeepsDateReplaysLaterBalanceAndRecoversReceipt(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	original := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	originalID := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), original["planId"].(string)))
	later := ledgerPreview(t, c, map[string]any{"kind": "money_removed", "accountId": fx.checking, "amount": "20", "currency": "USD", "reason": "expense", "effectiveAt": "2026-09-22T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), later["planId"].(string))
	if got := ledgerCash(t, c, fx.checking); got != "80" {
		t.Fatalf("cash before fix = %s", got)
	}
	plan := correctionPreview(t, c, map[string]any{
		"action": "fix", "activityId": originalID,
		"replacement": map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "150", "currency": "USD", "reason": "income"},
	})
	if plan["action"] != "fix" || plan["replacement"].(map[string]any)["effectiveAt"] != "2026-09-20T12:00:00Z" {
		t.Fatalf("fix plan did not freeze original timestamp: %+v", plan)
	}
	if got := ledgerActivityCount(t, fx.app); got != 2 || ledgerCash(t, c, fx.checking) != "80" {
		t.Fatalf("fix preview wrote ledger: count=%d cash=%s", got, ledgerCash(t, c, fx.checking))
	}
	planID := plan["planId"].(string)
	opID := uuid.NewString()
	fixed := correctionCommit(t, c, opID, planID)
	fixedID := ledgerActivityID(t, fixed)
	activity := fixed["activity"].(map[string]any)
	effectiveAt, err := time.Parse(time.RFC3339Nano, activity["effectiveAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !effectiveAt.Equal(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)) || activity["effectiveLocalDate"] != "2026-09-20" {
		t.Fatalf("fix moved historical date: %+v", activity)
	}
	if got := ledgerCash(t, c, fx.checking); got != "130" {
		t.Fatalf("cash after fix = %s", got)
	}
	if count := ledgerActivityCount(t, fx.app); count != 4 {
		t.Fatalf("fix activities=%d, want original, later, inverse, replacement", count)
	}
	if got := ledgerActivityID(t, correctionCommit(t, c, opID, planID)); got != fixedID {
		t.Fatalf("same operation replayed %s, want %s", got, fixedID)
	}
	// Simulate a crash after the database commit and before the receipt save.
	path, err := fx.service.operationPath(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fx.service.GetOperation(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Status, receipt.Result = "pending", nil
	if err := fx.service.writePrivate(path, receipt); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(fx.service.dir, "plans", planID+".json")
	raw, err := fx.service.readPrivate(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved correctionPlan
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved.ExpiresAt = time.Now().Add(-time.Second)
	if err := fx.service.writePrivate(planPath, saved); err != nil {
		t.Fatal(err)
	}
	fx.service.Close()
	resumed := New(fx.app, fx.service.dir, nil)
	t.Cleanup(resumed.Close)
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	c2 := connect(t, resumed)
	if got := ledgerActivityID(t, correctionCommit(t, c2, opID, planID)); got != fixedID {
		t.Fatalf("recovered activity=%s, want %s", got, fixedID)
	}
	if got := ledgerActivityID(t, correctionCommit(t, c2, uuid.NewString(), planID)); got != fixedID {
		t.Fatalf("same expired plan created %s, want %s", got, fixedID)
	}
	if ledgerActivityCount(t, fx.app) != 4 || ledgerCash(t, c2, fx.checking) != "130" {
		t.Fatal("recovery duplicated correction")
	}
}

func TestMCPUndoFreezesPreviewTimeAndPreservesOriginal(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fx := newLedgerFixtureWithClock(t, func() time.Time { return now })
	c := ledgerSession(t, fx)
	original := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	originalID := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), original["planId"].(string)))
	extra := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "10", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-22T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), extra["planId"].(string))
	plan := correctionPreview(t, c, map[string]any{"action": "undo", "activityId": originalID})
	previewActivity := plan["preview"].(map[string]any)["activity"].(map[string]any)
	previewAt := previewActivity["effectiveAt"].(string)
	parsedPreviewAt, err := time.Parse(time.RFC3339Nano, previewAt)
	if err != nil {
		t.Fatal(err)
	}
	if got := ledgerActivityCount(t, fx.app); got != 2 || ledgerCash(t, c, fx.checking) != "110" {
		t.Fatalf("undo preview wrote ledger: count=%d cash=%s", got, ledgerCash(t, c, fx.checking))
	}
	now = now.Add(time.Hour)
	undo := correctionCommit(t, c, uuid.NewString(), plan["planId"].(string))
	activity := undo["activity"].(map[string]any)
	committedAt, err := time.Parse(time.RFC3339Nano, activity["effectiveAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, activity["createdAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !committedAt.Equal(parsedPreviewAt) || !createdAt.Equal(now) || activity["reversesActivityId"] != originalID {
		t.Fatalf("undo times/target = %+v, preview time=%s", activity, previewAt)
	}
	if got := ledgerCash(t, c, fx.checking); got != "10" {
		t.Fatalf("undo cash = %s", got)
	}
	if count := ledgerActivityCount(t, fx.app); count != 3 {
		t.Fatalf("undo activities=%d, expected original and later record preserved", count)
	}
	if code := ledgerErrorCode(t, c, "preview_correction", map[string]any{"action": "undo", "activityId": originalID}); code != "already_undone" {
		t.Fatalf("duplicate reversal code=%s", code)
	}
}

func TestMCPCorrectionRejectsInvalidAndStalePlans(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	original := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	originalID := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), original["planId"].(string)))
	for _, input := range []map[string]any{
		{"action": "fix", "activityId": originalID},
		{"action": "undo", "activityId": originalID, "replacement": map[string]any{"kind": "money_added"}},
		{"action": "fix", "activityId": originalID, "replacement": map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "150", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-21T12:00:00Z"}},
		{"action": "fix", "activityId": originalID, "replacement": map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "1", "unitCost": "1", "currency": "USD"}},
	} {
		if code := ledgerErrorCode(t, c, "preview_correction", input); code != "validation" {
			t.Fatalf("invalid correction code=%s input=%v", code, input)
		}
	}
	plan := correctionPreview(t, c, map[string]any{"action": "undo", "activityId": originalID})
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": plan["planId"]}}); code != "validation" {
		t.Fatalf("wrong commit tool code=%s", code)
	}
	_, err := history.NewService(fx.app).RecordChange(context.Background(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checking, Amount: "10", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-21T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_correction", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": plan["planId"]}}); code != "stale_preview" {
		t.Fatalf("stale correction code=%s", code)
	}
	if ledgerActivityCount(t, fx.app) != 2 || ledgerCash(t, c, fx.checking) != "110" {
		t.Fatal("invalid or stale correction changed ledger")
	}
}

func TestMCPCorrectionRecoversAfterCommittedWriteButFailedSnapshotRebuild(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "correction.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := &failedCorrectionSnapshotRepository{Repository: sqlite.NewRepository(database)}
	app := application.NewService(repository)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repository.Repository)
	app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Snapshot failure", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "UTC", HistoryStartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Checking", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	service := New(app, t.TempDir(), nil)
	t.Cleanup(service.Close)
	fx := ledgerFixture{service: service, app: app, checking: account.Account.ID.String()}
	c := ledgerSession(t, fx)
	original := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	originalID := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), original["planId"].(string)))
	plan := correctionPreview(t, c, map[string]any{"action": "fix", "activityId": originalID, "replacement": map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "150", "currency": "USD", "reason": "income"}})
	planID := plan["planId"].(string)
	opID := uuid.NewString()
	repository.fail.Store(true)
	if code := ledgerErrorCode(t, c, "commit_correction", map[string]any{"operationId": opID, "input": map[string]any{"planId": planID}}); code != "internal" {
		t.Fatalf("postcommit snapshot failure code=%s", code)
	}
	receipt, err := service.GetOperation(opID)
	if err != nil || receipt.Status != "unknown" {
		t.Fatalf("postcommit operation receipt=%+v err=%v", receipt, err)
	}
	if count := ledgerActivityCount(t, app); count != 3 || ledgerCash(t, c, fx.checking) != "150" {
		t.Fatalf("financial write did not commit atomically: activities=%d cash=%s", count, ledgerCash(t, c, fx.checking))
	}
	repository.fail.Store(false)
	replayed := correctionCommit(t, c, opID, planID)
	if ledgerActivityID(t, replayed) == originalID || ledgerActivityCount(t, app) != 3 || ledgerCash(t, c, fx.checking) != "150" {
		t.Fatal("retry duplicated or lost committed correction")
	}
	receipt, err = service.GetOperation(opID)
	if err != nil || receipt.Status != "succeeded" {
		t.Fatalf("recovered operation receipt=%+v err=%v", receipt, err)
	}
}
