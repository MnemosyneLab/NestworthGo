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
			tools := encode(request["tools"])
			if !strings.Contains(tools, "read_page_content") || strings.Contains(tools, `"name":"open_page"`) {
				t.Error("page reading requires or mints navigation grant")
			}
			writeMode(w, request, []any{liveCallItem(round, "read_page_content", obj{"module": "filters", "limit": 1})})
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
			writeMode(w, request, []any{liveCallItem(round, "read_page_content", obj{"module": "filters", "snapshotId": snapshotID, "cursor": 1})})
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
		return engineWithModel(ctx, cfg, f.model, builtinai.New(shared, assistant.SelectedPage(ctx), assistant.ModelNavigationEnabled(ctx)).WithPageReader(assistant.CurrentPageReader(ctx)), nil)
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
}
