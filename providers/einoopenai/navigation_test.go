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

func TestFakeProviderNavigationUsesCurrentLeaseAndKnownTarget(t *testing.T) {
	for _, trend := range []bool{false, true} {
		name := "account"
		if trend {
			name = "return-trend"
		}
		t.Run(name, func(t *testing.T) { providerNavigation(t, trend) })
	}
}
func providerNavigation(t *testing.T, trend bool) {
	app, _, accountID := ledgerSession(t)
	var targetRef string
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		if trend && request["stream"] != true {
			t.Error("trend path did not stream")
		}
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
			name, args := "open_page", obj{"targetRef": targetRef}
			if trend {
				name = "open_return_trend"
				args = obj{"targetRef": targetRef, "from": "2026-01-01", "to": "2026-02-28", "valuation": "base", "includeCash": true, "display": "linked_rate"}
			}
			writeMode(w, request, []any{liveCallItem(round, name, args)})
			return
		case 3:
			if !strings.Contains(latestToolOutput(t, request), "requested") {
				t.Error("navigation result claimed completion instead of request")
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
		return target.Valid() && (target.Kind == "account" || target.Kind == "return-trend") && target.AccountID == accountID
	}, Factory: func(ctx context.Context, cfg assistant.ModelConfig, key []byte, id string) (assistant.Engine, error) {
		shared, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, completionMode(f.model, trend), builtinai.New(shared, assistant.SelectedPage(ctx), assistant.ModelNavigationEnabled(ctx)), nil)
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
	if _, err := service.Start(q); err != nil {
		t.Fatal(err)
	}
	view := waitAssistantRun(t, service, q)
	if view.Navigation == nil {
		t.Fatal("completed model request did not offer typed intent")
	}
	target, err := service.ConsumeNavigation(q.Epoch, q.ConversationID, view.Navigation.RunID, view.Navigation.ID, lease.ID)
	if err != nil || target.AccountID != accountID || (trend && (target.Kind != "return-trend" || target.Trend == nil || target.Trend.To != "2026-02-28")) || (!trend && target.Kind != "account") {
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
	// Construct the actual v1 layout: common system, user, native assistant.
	old.Version = 1
	old.Messages = append(old.Messages[:1], old.Messages[2:]...)
	old.Messages[0] = schema.SystemAgenticMessage("obsolete-host-policy: period returns are unavailable")
	native := old.Messages[2].Extra[nativeKey]
	if native == nil {
		t.Fatal("legacy fixture lost native payload")
	}
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

func TestRecoveredNavigationRefusalCannotCompleteAnEarlierAction(t *testing.T) {
	for _, mode := range []string{"unknown", "missing", "null", "wrong-type", "oversized-local", "malformed-wire", "oversized-wire", "cancelled-wire"} {
		t.Run(mode, func(t *testing.T) {
			session := builtinai.New(&unsafeInventory{}, assistant.PageSelection{}, true)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var targetRef string
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				switch round {
				case 0:
					writeMode(w, request, []any{liveCallItem(round, "get_navigation_targets", obj{})})
				case 1:
					var inventory struct {
						Targets []struct {
							Ref   string `json:"targetRef"`
							Label string `json:"label"`
						} `json:"targets"`
					}
					if err := jsoncodec.Unmarshal([]byte(latestToolOutput(t, request)), &inventory); err != nil {
						t.Error(err)
					}
					for _, target := range inventory.Targets {
						if target.Label == "Return analysis: trend" {
							targetRef = target.Ref
						}
					}
					if targetRef == "" {
						t.Error("missing trend root")
					}
					writeMode(w, request, []any{liveCallItem(round, "open_return_trend", obj{"targetRef": targetRef, "from": "2026-01-01", "to": "2026-02-28", "valuation": "base", "includeCash": true, "display": "linked_rate"})})
				case 2:
					if !strings.Contains(latestToolOutput(t, request), "requested") || session.Navigation() == nil {
						t.Error("first action was not accepted")
					}
					// The second tool is deliberately different. The adapter forwards
					// schema-invalid JSON objects to Session, where refusal is recoverable.
					call := liveCallItem(round, "open_page", obj{"targetRef": "unknown"})
					switch mode {
					case "missing":
						call["arguments"] = `{}`
					case "null":
						call["arguments"] = `{"targetRef":null}`
					case "wrong-type":
						call["arguments"] = `{"targetRef":42}`
					case "oversized-local":
						call["arguments"] = strings.Repeat(" ", 4097) + `{}`
					case "malformed-wire":
						call["arguments"] = `{`
					case "oversized-wire":
						call["arguments"] = strings.Repeat(" ", 64<<10) + `{}`
					case "cancelled-wire":
						cancel()
					}
					writeMode(w, request, []any{call})
				case 3:
					if !strings.Contains(latestToolOutput(t, request), "controlled_read_refused") {
						t.Error("missing recoverable refusal")
					}
					writeMode(w, request, []any{textItem()})
				default:
					t.Errorf("unexpected round %d", round)
				}
			})
			engine, err := engineWithModel(ctx, assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, streamCompletionModel{fake.model}, session, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			var events []assistant.ProgressEvent
			ctx = assistant.WithProgress(ctx, func(event assistant.ProgressEvent) {
				if event.Kind == "tool" {
					events = append(events, event)
				}
			})
			result, err := engine.Complete(ctx, nil, "Open a synthetic return trend")
			if result.Navigation != nil {
				t.Fatal("completion retained the earlier action")
			}
			terminal := strings.HasSuffix(mode, "wire")
			if terminal {
				// Protocol/64 KiB/cancellation gates can stop before Session.Call.
				// They abort completion entirely, so no successful run can offer it.
				if err == nil || len(result.History) != 0 {
					t.Fatal("wire refusal did not abort completion", err)
				}
			} else {
				if err != nil || result.Text == "" || len(result.History) == 0 {
					t.Fatal("model could not recover to a successful reply", err)
				}
				if len(events) != 6 || events[3].Source != "open_return_trend" || events[3].State != "succeeded" || events[5].Source != "open_page" || events[5].State != "refused" {
					t.Fatalf("wrong tool events: %+v", events)
				}
				if session.Navigation() != nil {
					t.Fatal("refused second call retained session action")
				}
			}
		})
	}
}
