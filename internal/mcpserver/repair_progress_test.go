package mcpserver

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
)

func TestRepairProgressPollingAvoidsCaptureChurn(t *testing.T) {
	s, app := dataHealthFixture(t, true)
	result, err := app.StartMarketDataSync(t.Context(), application.SyncRequest{Scope: application.SyncScopeRepairAll})
	if err != nil {
		t.Fatal(err)
	}
	app.CancelMarketDataSyncAndWait()
	client, recorder := connectPayload(t, s)
	before := len(s.reads.entries)
	var first map[string]any
	for i := 0; i < 3; i++ {
		p := call(t, client, "get_data_repair_job", skillExample(t, "data-health", "repair-progress", map[string]string{"jobId": result.Job.JobID}), false)["data"].(map[string]any)
		if p["captureId"] != nil || p["health"] != nil || p["healthIncluded"] != false || p["sections"] != nil {
			t.Fatal("progress captured evidence/health", p)
		}
		job := p["job"].(map[string]any)
		if job["jobId"] != result.Job.JobID || job["outcome"] == "running" {
			t.Fatal("wrong job/status", job)
		}
		if first != nil && !reflect.DeepEqual(first, p) {
			t.Fatal("unchanged job progress changed")
		}
		first = p
	}
	if len(s.reads.entries) != before {
		t.Fatal("progress allocated a capture")
	}
	metric := recorder.total()
	if metric.Calls != 3 || metric.Bytes > 6<<10 {
		t.Fatal("progress inflated", metric)
	}
	t.Logf("actual unchanged repair polling: calls=%d bytes=%d max=%d captures=%d", metric.Calls, metric.Bytes, metric.Max, len(s.reads.entries)-before)
	detail := scaleCapture(t, client, "get_data_repair_job", skillExample(t, "data-health", "repair-evidence", map[string]string{"jobId": result.Job.JobID}))
	if detail["summary"].(map[string]any)["healthIncluded"] != false {
		t.Fatal("evidence claims health")
	}
	oldID := detail["captureId"].(string)
	for section := range detail["sections"].(map[string]any) {
		scaleSection(t, client, detail, section)
	}
	s.reads.revoke()
	call(t, client, "get_read_item", map[string]any{"captureId": oldID, "section": "job_items", "id": "unknown"}, true)
	call(t, client, "get_data_repair_job", skillExample(t, "data-health", "repair-progress", map[string]string{"jobId": result.Job.JobID}), false)
}

func TestTaskReadNeedsNoContextOrCatalogRoundTrips(t *testing.T) {
	fx := newLedgerFixture(t)
	client, recorder := connectPayload(t, fx.service)
	call(t, client, "get_account_valuations", map[string]any{"accountIds": []string{fx.checking}}, false)
	m := recorder.total()
	if m.Calls != 1 {
		t.Fatal("targeted read added prerequisite calls", m)
	}
	t.Logf("known-account current snapshot: calls=%d bytes=%d max=%d", m.Calls, m.Bytes, m.Max)
	listed, err := listAllTools(t.Context(), client)
	if err != nil || len(listed.Tools) != 79 {
		t.Fatal("incomplete discovery", err)
	}
	if recorder.discoveryCalls < 2 || recorder.discoveryMax > contextWireLimit {
		t.Fatal("discovery was not bounded", recorder.discoveryCalls, recorder.discoveryMax)
	}
	t.Logf("complete discovery: tools=%d pages=%d total bytes=%d max page=%d", len(listed.Tools), recorder.discoveryCalls, recorder.discoveryBytes, recorder.discoveryMax)
}
