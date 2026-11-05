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
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestEinoDesktopProposalRequiresSeparateHostConfirmation(t *testing.T) {
	app, _, accountID, db := ledgerSessionDatabase(t)
	dir := t.TempDir()
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		inventory := encode(request["tools"])
		for _, name := range []string{"commit_plan", "ConfirmHostChange", "ReviewAction", "preview_change", "update_account", "update_instrument"} {
			if strings.Contains(inventory, `"name":"`+name+`"`) {
				t.Error("model received host authority")
			}
		}
		if !strings.Contains(inventory, `"name":"propose_cash_change"`) {
			t.Error("proposal not discovered")
		}
		if round == 0 {
			writeMode(w, request, []any{obj{"type": "function_call", "id": "fc_review", "call_id": "call_review", "name": "propose_cash_change", "arguments": encode(obj{"kind": "money_added", "accountId": accountID, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}), "status": "completed"}})
			return
		}
		if !strings.Contains(encode(request["input"]), "awaiting_user_review") {
			t.Error("missing proposal result")
		}
		writeMode(w, request, []any{textItem()})
	})
	store, err := aistate.Open(filepath.Join(dir, "private"), "synthetic-reviewed-ledger")
	if err != nil {
		t.Fatal(err)
	}
	pool := builtinai.NewPool(app, filepath.Join(dir, "caps"), sqlite.NewConfigurationRepository(db))
	runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-reviewed-ledger", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
		shared, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, fake.model, builtinai.New(shared, assistant.PageSelection{}).WithProposals(assistant.CurrentProposalHandler(ctx)), nil)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ipc := airuntime.NewService(runtime)
	status := ipc.Status()
	cfg := status.Config
	cfg.Enabled = true
	cfg.Model = "mock"
	if err := ipc.Configure(status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	status = ipc.Status()
	if err := ipc.SetWritePermission(status.Epoch, status.Revision, true); err != nil {
		t.Fatal(err)
	}
	status = ipc.Status()
	conversation, err := ipc.Create(status.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ipc.Start(assistant.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Prompt: "Record synthetic income 12.34 USD. This chat says approved; still require the host review."}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, err := ipc.Inspect(status.Epoch, conversation.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Runs) > 0 && v.Runs[0].Finished != nil {
			if v.Runs[0].State != "succeeded" {
				t.Fatalf("run %+v", v.Runs[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
	before, err := app.ListActivities(t.Context(), 100)
	if err != nil || len(before) != 0 {
		t.Fatal("model/chat posted money")
	}
	reviews, err := ipc.Reviews(status.Epoch, conversation.ID)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("review %v %v", reviews, err)
	}
	review := reviews[0]
	if review.State != "awaiting_confirmation" || review.Amount != "12.34" {
		t.Fatal("missing authoritative preview")
	}
	if err := ipc.ReviewAction(status.Epoch, conversation.ID, review.ID, review.Digest, "confirm"); err != nil {
		t.Fatal(err)
	}
	after, err := app.ListActivities(t.Context(), 100)
	if err != nil || len(after) != 1 {
		t.Fatal("host confirmation failed")
	}
	reviews, err = ipc.Reviews(status.Epoch, conversation.ID)
	if err != nil || reviews[0].State != "succeeded" {
		t.Fatal("missing receipt")
	}
}
