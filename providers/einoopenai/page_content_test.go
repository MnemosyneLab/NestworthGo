package einoopenai

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestProviderInternalPageReadsUseLiveDesktopCaptureWithoutNavigationGrant(t *testing.T) {
	app, _, _ := ledgerSession(t)
	var snapshotID string
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		switch round {
		case 0:
			// Metadata is stable; only the current allowed subset grants access.
			tools := encode(request["tool_choice"])
			if !strings.Contains(encode(request["tools"]), "inspect_page") || !strings.Contains(tools, "inspect_page") || strings.Contains(tools, `"name":"navigate_page"`) {
				t.Error("page reading requires or mints navigation grant")
			}
			writeMode(w, request, []any{liveCallItem(round, "inspect_page", obj{"module": "filters", "limit": 1})})
			return
		case 1:
			raw := latestToolOutput(t, request)
			var result struct {
				SnapshotID string `json:"snapshotId"`
				HasMore    bool   `json:"hasMore"`
				NextCursor int    `json:"nextCursor"`
			}
			if jsoncodec.Unmarshal([]byte(raw), &result) != nil || !result.HasMore || result.NextCursor != 1 || !strings.Contains(raw, "2026-10-01") {
				t.Errorf("missing displayed filtered state: %s", raw)
			}
			snapshotID = result.SnapshotID
			writeMode(w, request, []any{liveCallItem(round, "inspect_page", obj{"module": "filters", "snapshotId": snapshotID, "cursor": 1})})
			return
		case 2:
			raw := latestToolOutput(t, request)
			if !strings.Contains(raw, "Synthetic USD") || !strings.Contains(raw, snapshotID) || !strings.Contains(raw, `"hasMore":false`) {
				t.Errorf("missing bounded continuation: %s", raw)
			}
		default:
			t.Errorf("unexpected round %d", round)
		}
		writeMode(w, request, []any{textItem()})
	})
	dir := t.TempDir()
	pool := builtinai.NewPool(app, filepath.Join(dir, "capabilities"))
	store, err := aistate.Open(filepath.Join(dir, "private"), "synthetic-page-read")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-page-read", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
		shared, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, f.model, builtinai.New(shared, assistant.SelectedPage(ctx), assistant.ModelNavigationEnabled(ctx)).WithPageReader(assistant.CurrentPageReader(ctx)).WithPageOperator(assistant.CurrentPageOperator(ctx)), nil)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	service := airuntime.NewService(runtime)
	status := service.Status()
	cfg := status.Config
	cfg.Model = "mock"
	cfg.Enabled = true
	if err := service.Configure(status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	status = service.Status()
	conversation, err := service.Create(status.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	q := assistant.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Prompt: "解释当前筛选 Explain current filters"}
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	// Desktop bridge: the tool requests capture without prepublication, read
	// approval, user refresh or a navigation grant.
	captureID := uuid.NewString()
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				requests, _ := service.PendingPageReads(status.Epoch)
				for _, request := range requests {
					// Capture waits must remain visible through the same Wails
					// snapshot as the conversation, before page data arrives.
					active, inspectErr := service.Inspect(status.Epoch, conversation.ID)
					if inspectErr != nil || active.Progress == nil || len(active.Progress.Tools) == 0 || active.Progress.Tools[len(active.Progress.Tools)-1].Source != "inspect_page" || active.Progress.Tools[len(active.Progress.Tools)-1].State != "running" || active.Runs[0].Text != "" {
						t.Errorf("page capture lost live tool progress: %+v error=%v", active.Progress, inspectErr)
					}
					err := service.DeliverPageContent(status.Epoch, request.ID, assistant.PageContent{ID: captureID, Page: "return-analysis", CapturedAt: time.Now(), Nodes: []assistant.PageNode{{ID: "from", Module: "filters", Role: "textbox", Name: "From", Value: "2026-10-01"}, {ID: "currency", Module: "filters", Role: "combobox", Name: "Currency", Value: "Synthetic USD"}}, Limitations: []string{"Synthetic semantic capture"}})
					if err != nil {
						t.Errorf("delivery: %v", err)
					}
				}
			}
		}
	}()
	view := waitAssistantRun(t, service, q)
	close(stop)
	<-stopped
	if len(view.Runs) != 1 || view.Runs[0].State != "succeeded" || len(view.Runs[0].Evidence) != 2 || view.Navigation != nil {
		t.Fatalf("page read failed or navigated: %+v", view)
	}
	if len(view.Runs[0].Tools) != 2 {
		t.Fatalf("page tools absent from completed conversation: %+v", view.Runs[0].Tools)
	}
	for _, detail := range view.Runs[0].Tools {
		if detail.Source != "inspect_page" || detail.State != "succeeded" || !strings.Contains(detail.Input, `"module":"filters"`) || !strings.Contains(detail.Output, snapshotID) {
			t.Fatalf("page tool details mismatch: %+v", detail)
		}
	}
	activities, err := app.ListActivities(t.Context(), 10)
	if err != nil || len(activities) != 0 {
		t.Fatal("page tool wrote financial data", err)
	}
}
