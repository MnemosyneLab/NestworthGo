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
	// The fixture closes SQLite at cleanup; let any asynchronous sync worker
	// stop before that happens, including when an assertion fails early.
	t.Cleanup(app.CancelMarketDataSyncAndWait)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "Sync", BaseCurrency: "SGD", MemberNames: []string{"Owner"}, Timezone: "Asia/Singapore",
	}); err != nil {
		t.Fatal(err)
	}
	emitter := newFakeEmitter()
	// This test reads the event history directly. Saturate the wakeup channel
	// to ensure neither starting nor finishing a sync depends on draining it.
	for index := 0; index < cap(emitter.notify); index++ {
		emitter.notify <- struct{}{}
	}
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
	if attached.Job.JobID == start.Job.JobID {
		if !attached.Attached {
			t.Fatalf("same running job should attach, got %+v", attached)
		}
	} else {
		first, found := app.GetSyncJob(start.Job.JobID)
		if !found || first.Outcome == application.SyncOutcomeRunning || attached.Attached {
			t.Fatalf("new job requires completed predecessor, first=%+v found=%t next=%+v", first, found, attached)
		}
	}
	current, err := service.GetCurrentSyncJob()
	if err != nil {
		t.Fatal(err)
	}
	if current.JobID != attached.Job.JobID {
		t.Fatalf("current job %s, latest start %s", current.JobID, attached.Job.JobID)
	}
	deadline := time.Now().Add(3 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		job, found := app.GetSyncJob(attached.Job.JobID)
		if !found {
			t.Fatal("started job not found in application state")
		}
		if job.Outcome != "" && job.Outcome != application.SyncOutcomeRunning {
			if job.Outcome != application.SyncOutcomeSucceeded {
				t.Fatalf("sync did not succeed: %+v", job)
			}
			completed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !completed {
		t.Fatal("sync did not complete before the deadline")
	}
	// A terminal snapshot becomes visible before its listener returns. Join
	// the worker so all events are recorded and cleanup is known to return.
	app.CancelMarketDataSyncAndWait()
	jobs := map[string][]marketdata.SyncJobDTO{start.Job.JobID: nil, attached.Job.JobID: nil}
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	for _, event := range emitter.events {
		payload, ok := event.data.(marketdata.SyncJobDTO)
		if !ok {
			t.Fatalf("event %s has unexpected payload: %#v", event.name, event.data)
		}
		snapshots, known := jobs[payload.JobID]
		if !known {
			t.Fatalf("event %s refers to unknown job %s", event.name, payload.JobID)
		}
		if payload.Sequence != len(snapshots)+1 {
			t.Fatalf("job %s event sequence = %d, want %d", payload.JobID, payload.Sequence, len(snapshots)+1)
		}
		wantEvent := marketdata.SyncProgressEvent
		if len(snapshots) == 0 {
			wantEvent = marketdata.SyncStartedEvent
		} else if payload.Outcome != application.SyncOutcomeRunning {
			wantEvent = marketdata.SyncCompletedEvent
		}
		if event.name != wantEvent {
			t.Fatalf("job %s sequence %d event = %s, want %s", payload.JobID, payload.Sequence, event.name, wantEvent)
		}
		jobs[payload.JobID] = append(snapshots, payload)
	}
	for jobID, snapshots := range jobs {
		job, found := app.GetSyncJob(jobID)
		if !found || job.Outcome != application.SyncOutcomeSucceeded {
			t.Fatalf("job %s did not succeed: %+v found=%t", jobID, job, found)
		}
		if len(snapshots) != job.Sequence {
			t.Fatalf("job %s captured %d events, want %d", jobID, len(snapshots), job.Sequence)
		}
		last := snapshots[len(snapshots)-1]
		if last.Outcome != job.Outcome || last.Phase != job.Phase {
			t.Fatalf("job %s completion event = %+v, want phase %s outcome %s", jobID, last, job.Phase, job.Outcome)
		}
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
