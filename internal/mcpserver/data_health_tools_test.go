package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func dataHealthFixture(t *testing.T, startHistory bool) (*Service, *application.Service) {
	t.Helper()
	app := wailstest.NewService(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if startHistory {
		now = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	}
	app.SetClock(func() time.Time { return now })
	// The sync worker must finish before wailstest closes its temporary SQLite DB.
	t.Cleanup(app.CancelMarketDataSyncAndWait)
	input := application.OnboardingInput{
		HouseholdName: "MCP Data Health", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}
	if startHistory {
		input.Timezone = "UTC"
		input.HistoryStartDate = "2026-09-01"
	}
	if err := app.CompleteOnboarding(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	s := New(app, t.TempDir(), nil)
	t.Cleanup(s.Close)
	return s, app
}

func decodeDataHealthDTO[T any](t *testing.T, value any) T {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded T
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestDataHealthToolsAvailableAcrossModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app := dataHealthFixture(t, true)
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			c := connect(t, s)
			listed, err := c.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := make(map[string]*mcp.Tool, len(listed.Tools))
			for _, tool := range listed.Tools {
				found[tool.Name] = tool
			}
			for _, name := range []string{"scan_data_health", "preview_data_repair", "get_data_repair_job"} {
				tool := found[name]
				if tool == nil {
					t.Fatalf("%s missing in %s mode", name, mode)
				}
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
					t.Fatalf("%s must describe a local read: %+v", name, tool.Annotations)
				}
			}
			if (found["start_data_repair"] != nil) != (mode == LedgerWrite) {
				t.Fatalf("start_data_repair availability in %s mode: %t", mode, found["start_data_repair"] != nil)
			}
			if start := found["start_data_repair"]; start != nil {
				if start.Annotations == nil || !start.Annotations.IdempotentHint || start.Annotations.OpenWorldHint == nil || !*start.Annotations.OpenWorldHint {
					t.Fatalf("start_data_repair must describe an idempotent provider write: %+v", start.Annotations)
				}
			}

			wails := marketdata.NewService(app, nil)
			wantHealth, err := wails.ScanMarketDataHealth()
			if err != nil {
				t.Fatal(err)
			}
			gotHealth := decodeDataHealthDTO[marketdata.MarketDataHealthReportDTO](t, call(t, c, "scan_data_health", Empty{}, false)["data"])
			if !reflect.DeepEqual(gotHealth, wantHealth) {
				t.Fatalf("MCP health differs from Wails: got %+v, want %+v", gotHealth, wantHealth)
			}

			wantPreview, err := wails.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: "repair_all"})
			if err != nil {
				t.Fatal(err)
			}
			gotPreview := decodeDataHealthDTO[marketdata.SyncPlanPreviewDTO](t, call(t, c, "preview_data_repair", Empty{}, false)["data"])
			if gotPreview.Scope.Scope != "repair_all" || !reflect.DeepEqual(gotPreview, wantPreview) {
				t.Fatalf("MCP preview differs from Wails repair_all: got %+v, want %+v", gotPreview, wantPreview)
			}
			if _, started := app.GetCurrentSyncJob(); started {
				t.Fatal("local health reads unexpectedly started a repair")
			}

			call(t, c, "get_data_repair_job", map[string]any{"jobId": uuid.NewString()}, true)
			call(t, c, "get_data_repair_job", map[string]any{"jobId": "invalid"}, true)
			if mode != LedgerWrite {
				if _, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "start_data_repair", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{}}}); err == nil {
					t.Fatalf("%s mode accepted start_data_repair", mode)
				}
			}
		})
	}
}

func TestDataRepairDurableRetryAndExactJobStatus(t *testing.T) {
	t.Parallel()
	s, app := dataHealthFixture(t, false)
	ctx := context.Background()
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil || len(bootstrap.Members) != 1 {
		t.Fatalf("bootstrap members: %+v, %v", bootstrap.Members, err)
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manual, err := app.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Unpriced holding", Type: "bank_investment_product", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateHolding(ctx, application.HoldingInput{
		AccountID: account.Account.ID.String(), InstrumentID: manual.ID.String(), Quantity: "1", UnitCost: "1",
	}); err != nil {
		t.Fatal(err)
	}
	app.SetClock(func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) })
	completed := make(chan application.SyncJobSnapshot, 1)
	app.SetMarketDataSyncListener(func(event string, job application.SyncJobSnapshot) {
		if event == application.SyncEventCompleted {
			select {
			case completed <- job:
			default:
			}
		}
	})
	if _, err := s.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	// MCP search previously constructed a second event-owning Wails service.
	// Neither search nor the new health reads may replace the desktop listener.
	call(t, c, "search_market_instruments", SearchInput{Query: "", InstrumentType: "stock"}, true)
	call(t, c, "scan_data_health", Empty{}, false)
	call(t, c, "preview_data_repair", Empty{}, false)
	operationID := uuid.NewString()
	args := map[string]any{"operationId": operationID, "input": map[string]any{}}
	first := call(t, c, "start_data_repair", args, false)["data"].(map[string]any)
	if first["status"] != "succeeded" {
		t.Fatalf("start receipt: %v", first)
	}
	result := decodeDataHealthDTO[marketdata.SyncStartResultDTO](t, first["result"])
	if _, err := uuid.Parse(result.Job.JobID); err != nil || result.Job.Scope.Scope != "repair_all" {
		t.Fatalf("start result: %+v, ID error: %v", result, err)
	}
	second := call(t, c, "start_data_repair", args, false)["data"]
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("same operationId returned a different receipt: first=%v, second=%v", first, second)
	}
	if _, found := app.GetSyncJob(result.Job.JobID); !found {
		t.Fatalf("job %s was not retained in application state", result.Job.JobID)
	}
	if got := call(t, c, "get_operation", IDInput{ID: operationID}, false)["data"].(map[string]any); got["status"] != "succeeded" {
		t.Fatalf("durable operation receipt: %v", got)
	}

	// Reopen only the MCP listener; the application job and temp database remain in place.
	s.Close()
	resumed := New(app, s.dir, nil)
	t.Cleanup(resumed.Close)
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	c2 := connect(t, resumed)
	third := call(t, c2, "start_data_repair", args, false)["data"]
	if !reflect.DeepEqual(third, first) {
		t.Fatalf("retry after MCP restart changed receipt: first=%v, third=%v", first, third)
	}

	deadline := time.Now().Add(5 * time.Second)
	var status map[string]any
	var terminal marketdata.SyncJobDTO
	for {
		status = call(t, c2, "get_data_repair_job", map[string]any{"jobId": result.Job.JobID}, false)["data"].(map[string]any)
		job := decodeDataHealthDTO[marketdata.SyncJobDTO](t, status["job"])
		if job.JobID != result.Job.JobID {
			t.Fatalf("requested job %s, got %+v", result.Job.JobID, job)
		}
		if job.Outcome != "running" {
			if job.Phase != "complete" || job.Outcome != "partial" {
				t.Fatalf("unexpected terminal repair job: %+v", job)
			}
			terminal = job
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("repair job did not finish: %+v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if terminal.SnapshotDaysPlanned == 0 || terminal.SnapshotDaysRebuilt != terminal.SnapshotDaysPlanned {
		t.Fatalf("expected rebuilt snapshots with unresolved provider data: %+v", terminal)
	}
	blockedByIncompleteSnapshot := false
	for _, blocker := range terminal.Blockers {
		if blocker.Reason == "snapshot_incomplete" {
			blockedByIncompleteSnapshot = true
		}
	}
	if !blockedByIncompleteSnapshot {
		t.Fatalf("rebuilt snapshots did not report incomplete valuation: %+v", terminal.Blockers)
	}
	select {
	case eventJob := <-completed:
		if eventJob.JobID != result.Job.JobID {
			t.Fatalf("sync listener observed job %s, want %s", eventJob.JobID, result.Job.JobID)
		}
	case <-time.After(time.Second):
		t.Fatal("preinstalled sync listener missed MCP-started completion")
	}
	gotHealth := decodeDataHealthDTO[marketdata.MarketDataHealthReportDTO](t, status["health"])
	if gotHealth.Healthy || gotHealth.IssueCount == 0 {
		t.Fatalf("rebuilt snapshots obscured remaining health issues: %+v", gotHealth)
	}
	manualPriceMissing := false
	for _, issue := range gotHealth.Issues {
		if issue.Kind == application.HealthKindMissingManualPrice && issue.InstrumentID == manual.ID.String() {
			manualPriceMissing = true
		}
	}
	if !manualPriceMissing {
		t.Fatalf("missing manual price was not retained in health report: %+v", gotHealth.Issues)
	}
	wantHealth, err := marketdata.NewService(app, nil).ScanMarketDataHealth()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotHealth, wantHealth) {
		t.Fatalf("status health is not the current local scan: got %+v, want %+v", gotHealth, wantHealth)
	}
	call(t, c2, "get_data_repair_job", map[string]any{"jobId": uuid.NewString()}, true)
	// A newer job must not replace the requested job in status responses.
	newResult := decodeDataHealthDTO[marketdata.SyncStartResultDTO](t, mutate(t, c2, "start_data_repair", Empty{}))
	if newResult.Job.JobID == result.Job.JobID || newResult.Attached || newResult.Conflict {
		t.Fatalf("expected a new repair after completion: %+v", newResult)
	}
	oldStatus := call(t, c2, "get_data_repair_job", map[string]any{"jobId": result.Job.JobID}, false)["data"].(map[string]any)
	oldJob := decodeDataHealthDTO[marketdata.SyncJobDTO](t, oldStatus["job"])
	if !reflect.DeepEqual(oldJob, terminal) {
		t.Fatalf("old job was replaced by a newer repair: got %+v, want %+v", oldJob, terminal)
	}
}
