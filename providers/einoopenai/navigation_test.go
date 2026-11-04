package einoopenai

import (
	"context"
	"github.com/cloudwego/eino/schema"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestFakeProviderNavigationRequiresExplicitGrantAndKnownTarget(t *testing.T) {
	app, _, accountID := ledgerSession(t)
	var targetRef string
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		switch round {
		case 0:
			writeMode(w, request, []any{liveCallItem(round, "get_account_snapshot", obj{"id": accountID})})
			return
		case 1:
			writeMode(w, request, []any{liveCallItem(round, "get_navigation_targets", obj{})})
			return
		case 2:
			raw := latestToolOutput(t, request)
			var inventory struct {
				Targets []struct {
					Ref  string `json:"targetRef"`
					Kind string `json:"kind"`
				} `json:"targets"`
			}
			if jsoncodec.Unmarshal([]byte(raw), &inventory) != nil {
				t.Error("invalid target inventory")
			}
			for _, target := range inventory.Targets {
				if target.Kind == "account" {
					targetRef = target.Ref
				}
			}
			if targetRef == "" {
				t.Error("disclosed account did not become an opaque target")
			}
			writeMode(w, request, []any{liveCallItem(round, "open_page", obj{"targetRef": targetRef})})
			return
		case 3:
			if latestToolOutput(t, request) == "" {
				t.Error("navigation request result missing")
			}
		default:
			t.Errorf("unexpected navigation provider round %d", round)
		}
		writeMode(w, request, []any{textItem()})
	})
	dir := t.TempDir()
	pool := builtinai.NewPool(app, filepath.Join(dir, "capabilities"))
	store, err := aistate.Open(filepath.Join(dir, "private"), "synthetic-navigation")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-navigation", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Validate: func(context.Context) bool { return true }, NavigationValidator: func(_ context.Context, target assistant.ModelNavigationTarget) bool {
		return target.Kind == "account" && target.AccountID == accountID
	}, Factory: func(ctx context.Context, cfg assistant.ModelConfig, key []byte, id string) (assistant.Engine, error) {
		shared, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, f.model, builtinai.New(shared, assistant.SelectedPage(ctx), assistant.ModelNavigationEnabled(ctx)), nil)
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
	q := assistant.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Prompt: "Open the synthetic checking account", Page: assistant.PageSelection{Kind: "accounts"}}
	lease, err := service.PublishPage(status.Epoch, status.Revision, q.Page)
	if err != nil {
		t.Fatal(err)
	}
	q.PageLeaseID = lease.ID
	if err := service.GrantNavigation(status.Epoch, status.Revision, conversation.ID, q.RequestID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	view := waitAssistantRun(t, service, q)
	if view.Navigation == nil {
		t.Fatal("completed model request did not offer typed intent")
	}
	target, err := service.ConsumeNavigation(q.Epoch, q.ConversationID, view.Navigation.RunID, view.Navigation.ID, lease.ID)
	if err != nil || target.Kind != "account" || target.AccountID != accountID {
		t.Fatal("host did not resolve live account", err)
	}
	if _, err := service.ConsumeNavigation(q.Epoch, q.ConversationID, view.Navigation.RunID, view.Navigation.ID, lease.ID); err == nil {
		t.Fatal("navigation replayed")
	}
	// A fresh non-granted wrapper over the same read-only session has no navigation
	// inventory and cannot adopt a ref from model history or the previous turn.
	shared, err := pool.Get(t.Context(), q.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := builtinai.New(shared, q.Page)
	if _, err := ordinary.Call(t.Context(), "open_page", []byte(`{"targetRef":"`+targetRef+`"}`)); err == nil {
		t.Fatal("old native-history target granted navigation")
	}
	activities, err := app.ListActivities(t.Context(), 10)
	if err != nil || len(activities) != 0 {
		t.Fatal("navigation wrote financial facts")
	}
}

func TestNewTurnRefreshesHostPolicyWithoutReconstructingNativeHistory(t *testing.T) {
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		input := encode(request["input"])
		if strings.Contains(input, "obsolete-host-policy") || !strings.Contains(input, "strictly read-only closed-period attribution") {
			t.Error("saved host policy suppressed current capability policy")
		}
		if round == 0 {
			item := textItem()
			item["id"] = "msg_policy_0"
			writeMode(w, request, []any{obj{"type": "reasoning", "id": "rs_policy", "summary": []any{}, "encrypted_content": "opaque-policy-upgrade"}, item})
			return
		}
		if !strings.Contains(input, "opaque-policy-upgrade") {
			t.Error("policy refresh lost native encrypted reasoning")
		}
		item := textItem()
		item["id"] = "msg_policy_1"
		writeMode(w, request, []any{item})
	})
	engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, &unsafeInventory{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	first, err := engine.Complete(t.Context(), nil, "synthetic previous turn")
	if err != nil {
		t.Fatal(err)
	}
	var old checkpoint
	if jsoncodec.Unmarshal(first.History, &old) != nil {
		t.Fatal("bad native checkpoint")
	}
	old.Messages[0] = schema.SystemAgenticMessage("obsolete-host-policy: period returns are unavailable")
	native := old.Messages[2].Extra[nativeKey]
	history, err := jsoncodec.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Complete(t.Context(), history, "new permitted period question")
	if err != nil {
		t.Fatal(err)
	}
	var upgraded checkpoint
	if jsoncodec.Unmarshal(result.History, &upgraded) != nil {
		t.Fatal("bad upgraded checkpoint")
	}
	if upgraded.Messages[2].Extra[nativeKey] != native {
		t.Fatal("native output was reconstructed while refreshing host policy")
	}
}
