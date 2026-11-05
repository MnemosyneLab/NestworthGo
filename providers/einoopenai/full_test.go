package einoopenai

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestFakeProviderFullDirectWrite(t *testing.T) {
	app, _, accountID, db := ledgerSessionDatabase(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		inventory := encode(request["tools"])
		if !strings.Contains(inventory, "execute_ledger_change") || strings.Contains(inventory, "commit_plan") || strings.Contains(inventory, "propose_cash_change") {
			t.Error("wrong Full inventory")
		}
		if round == 0 {
			writeMode(w, request, []any{liveCallItem(round, "execute_ledger_change", obj{"kind": "money_added", "accountId": accountID, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"})})
			return
		}
		if !strings.Contains(latestToolOutput(t, request), "succeeded") {
			t.Error("missing actual receipt")
		}
		writeMode(w, request, []any{textItem()})
	})
	dir := t.TempDir()
	pool := builtinai.NewPool(app, filepath.Join(dir, "caps"), sqlite.NewConfigurationRepository(db))
	store, err := aistate.Open(filepath.Join(dir, "state"), "synthetic-full")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-full", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, key []byte, id string) (assistant.Engine, error) {
		shared, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, f.model, builtinai.New(shared, assistant.PageSelection{}).WithFullAccess(assistant.CurrentFullHandler(ctx)), nil)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ipc := airuntime.NewService(runtime)
	s := ipc.Status()
	cfg := s.Config
	cfg.Enabled = true
	cfg.Model = "mock"
	if err := ipc.Configure(s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	s = ipc.Status()
	if err := ipc.SetFullAccess(s.Epoch, s.Revision, true); err != nil {
		t.Fatal(err)
	}
	s = ipc.Status()
	c, err := ipc.Create(s.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	q := assistant.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: c.ID, RequestID: uuid.NewString(), Prompt: "Record the completed synthetic deposit"}
	if _, err := ipc.Start(q); err != nil {
		t.Fatal(err)
	}
	waitAssistantRun(t, ipc, q)
	rows, err := app.ListActivities(t.Context(), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("activities=%d err=%v", len(rows), err)
	}
	reviews, err := ipc.Reviews(s.Epoch, c.ID)
	if err != nil || len(reviews) != 1 || reviews[0].State != "succeeded" || reviews[0].Source != "ai_full" {
		t.Fatalf("audit=%+v err=%v", reviews, err)
	}
}
