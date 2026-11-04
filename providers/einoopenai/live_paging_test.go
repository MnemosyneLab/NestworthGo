package einoopenai

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func latestToolOutput(t *testing.T, request obj) string {
	t.Helper()
	items, ok := request["input"].([]any)
	if !ok {
		t.Fatal("missing input")
	}
	for i := len(items) - 1; i >= 0; i-- {
		if item, ok := items[i].(map[string]any); ok && item["type"] == "function_call_output" {
			if value, ok := item["output"].(string); ok {
				return value
			}
			if blocks, ok := item["output"].([]any); ok {
				for _, block := range blocks {
					if part, ok := block.(map[string]any); ok {
						if text, ok := part["text"].(string); ok {
							return text
						}
					}
				}
			}
		}
	}
	t.Fatalf("missing tool output: %s", encode(request["input"]))
	return ""
}
func liveCallItem(round int, name string, args any) obj {
	return obj{"type": "function_call", "id": fmt.Sprintf("fc_live_%d", round), "call_id": fmt.Sprintf("call_live_%d", round), "name": name, "arguments": encode(args), "status": "completed"}
}
func waitAssistantRun(t *testing.T, service *airuntime.Service, q assistant.StartRequest) assistant.Conversation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, err := service.Inspect(q.Epoch, q.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Runs) > 0 {
			run := v.Runs[0]
			if run.State == "succeeded" {
				return v
			}
			if run.State == "failed" || run.State == "interrupted" || run.State == "cancelled" {
				t.Fatalf("run failed: %s / %s", run.State, run.Code)
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("run did not finish")
	return assistant.Conversation{}
}
func TestLiveConversationPagingAndRestartRecapture(t *testing.T) {
	app, _, _ := ledgerSession(t)
	members, _ := app.ListMembers(t.Context(), false)
	for i := range 2 {
		if _, err := app.CreateAccount(t.Context(), application.AccountInput{Name: fmt.Sprintf("Synthetic page %d", i), AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "0", OwnerIDs: []domain.MemberID{members[0].ID}}); err != nil {
			t.Fatal(err)
		}
	}
	var capture, cursor, expiry string
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		switch round {
		case 0:
			writeMode(w, request, []any{liveCallItem(round, "list_accounts", obj{"limit": 1})})
			return
		case 1:
			raw := latestToolOutput(t, request)
			var v struct {
				Data struct {
					CaptureID string `json:"captureId"`
					ExpiresAt string `json:"expiresAt"`
					Sections  map[string]struct {
						Page struct {
							NextCursor string `json:"nextCursor"`
						} `json:"page"`
					} `json:"sections"`
				} `json:"data"`
			}
			if jsoncodec.Unmarshal([]byte(raw), &v) != nil {
				t.Fatal("bad capture")
			}
			expiry = v.Data.ExpiresAt
			capture = v.Data.CaptureID
			cursor = v.Data.Sections["accounts"].Page.NextCursor
			if capture == "" || cursor == "" {
				t.Error("paging fixture missing cursor")
			}
		case 2, 4:
			writeMode(w, request, []any{liveCallItem(round, "get_read_page", obj{"captureId": capture, "section": "accounts", "cursor": cursor, "limit": 1})})
			return
		case 3:
			if !strings.Contains(latestToolOutput(t, request), expiry) {
				t.Error("continuation changed fixed expiry")
			}
			if strings.Contains(latestToolOutput(t, request), "controlled_read_refused") {
				t.Error("live second turn lost its capture")
			}
		case 5:
			raw := latestToolOutput(t, request)
			if !strings.Contains(raw, "controlled_read_refused") || strings.Contains(raw, "synthetic-canary-key") {
				t.Error("restart did not refuse old capture safely")
			}
			writeMode(w, request, []any{liveCallItem(round, "list_accounts", obj{"limit": 1})})
			return
		case 6:
			if strings.Contains(latestToolOutput(t, request), capture) {
				t.Error("recapture resurrected old identity")
			}
		default:
			t.Errorf("unexpected provider round %d", round)
		}
		item := textItem()
		item["id"] = fmt.Sprintf("msg_live_%d", round)
		writeMode(w, request, []any{item})
	})
	dir := t.TempDir()
	open := func() (*assistant.Runtime, *airuntime.Service) {
		pool := builtinai.NewPool(app, filepath.Join(dir, "capabilities"))
		store, err := aistate.Open(filepath.Join(dir, "private"), "synthetic-live")
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-live", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, key []byte, id string) (assistant.Engine, error) {
			shared, err := pool.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return engineWithModel(ctx, cfg, f.model, builtinai.New(shared, assistant.SelectedPage(ctx)), nil)
		}})
		if err != nil {
			t.Fatal(err)
		}
		return runtime, airuntime.NewService(runtime)
	}
	runtime, service := open()
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
	q := assistant.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Prompt: "Read first account page"}
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	waitAssistantRun(t, service, q)
	q.RequestID = uuid.NewString()
	q.Prompt = "Continue that same frozen account page"
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	waitAssistantRun(t, service, q)
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, service = open()
	defer runtime.Close()
	status = service.Status()
	q.Epoch = status.Epoch
	q.Revision = status.Revision
	q.RequestID = uuid.NewString()
	q.Prompt = "Continue the old page after restart, recapture if expired"
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	waitAssistantRun(t, service, q)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 7 {
		t.Fatalf("unexpected bounded recovery: %d requests", len(f.requests))
	}
}
