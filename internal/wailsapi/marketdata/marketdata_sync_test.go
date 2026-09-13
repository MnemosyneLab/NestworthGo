package marketdata_test

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestMarketDataSyncPreviewStartGetAndEvents(t *testing.T) {
	app := wailstest.NewService(t)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "Sync", BaseCurrency: "SGD", MemberNames: []string{"Owner"}, Timezone: "Asia/Singapore",
	}); err != nil {
		t.Fatal(err)
	}
	emitter := newFakeEmitter()
	service := marketdata.NewService(app, emitter)
	preview, err := service.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: "repair_all"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ConfigRevision == "" {
		t.Fatalf("preview = %+v", preview)
	}
	start, err := service.StartMarketDataSync(marketdata.SyncRequestDTO{Scope: "repair_all"})
	if err != nil || start.Job.JobID == "" {
		t.Fatalf("start = %+v err=%v", start, err)
	}
	attached, err := service.StartMarketDataSync(marketdata.SyncRequestDTO{Scope: "repair_all"})
	if err != nil {
		t.Fatal(err)
	}
	if attached.Job.JobID != start.Job.JobID {
		t.Fatalf("expected attach or same completed job, got %+v", attached)
	}
	current, err := service.GetCurrentSyncJob()
	if err != nil {
		t.Fatal(err)
	}
	if current.JobID != "" && current.JobID != start.Job.JobID {
		t.Fatalf("current job %s, start %s", current.JobID, start.Job.JobID)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.GetSyncJob(start.Job.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Outcome != "" && job.Outcome != application.SyncOutcomeRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	gotStarted := false
	emitter.mu.Lock()
	for _, event := range emitter.events {
		if event.name == marketdata.SyncStartedEvent {
			gotStarted = true
		}
	}
	emitter.mu.Unlock()
	if !gotStarted {
		t.Fatal("missing marketdata.sync.started")
	}
}

func TestScanMarketDataHealthIsLocal(t *testing.T) {
	app := wailstest.NewService(t)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "Health", BaseCurrency: "SGD", MemberNames: []string{"Owner"}, Timezone: "Asia/Singapore",
	}); err != nil {
		t.Fatal(err)
	}
	service := marketdata.NewService(app, newFakeEmitter())
	report, err := service.ScanMarketDataHealth()
	if err != nil {
		t.Fatal(err)
	}
	if report.IssueCount < 0 {
		t.Fatalf("report = %+v", report)
	}
}
