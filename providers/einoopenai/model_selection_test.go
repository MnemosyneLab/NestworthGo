package einoopenai

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aiusage"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

func TestSelectedReasoningWireAndUsage(t *testing.T) {
	for _, tc := range []struct {
		model, effort string
		status        int
	}{
		{"gpt-6-sol", "none", 200}, {"gpt-6.1-sol", "max", 200}, {"custom-model", "", 200}, {"gpt-6-luna", "high", 403}, {"gpt-6-astra", "low", 404},
	} {
		t.Run(tc.model, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, _ int, request obj) {
				if request["model"] != tc.model {
					t.Errorf("wrong wire model: %v", request["model"])
				}
				if tc.effort == "" {
					if _, ok := request["reasoning"]; ok {
						t.Error("sent unknown reasoning")
					}
				} else if encode(request["reasoning"]) != encode(obj{"effort": tc.effort}) {
					t.Errorf("wrong reasoning: %v", request["reasoning"])
				}
				if tc.status != 200 {
					w.WriteHeader(tc.status)
					return
				}
				writeMode(w, request, []any{textItem()})
			})
			m, err := newModel(t.Context(), Config{BaseURL: "http://" + f.calls.host, APIKey: "synthetic-canary-key", Model: tc.model, ReasoningEffort: tc.effort, Transport: f.calls}, true)
			if err != nil {
				t.Fatal(err)
			}
			var usage assistant.UsageCall
			ctx := assistant.WithUsage(context.Background(), func(c assistant.UsageCall) { usage = c })
			_, err = m.Generate(ctx, []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")})
			if tc.status == 200 && err != nil {
				t.Fatal(err)
			}
			if tc.status == 403 && !errors.Is(err, assistant.ErrProviderAuthentication) {
				t.Fatal(err)
			}
			if tc.status == 404 && !errors.Is(err, assistant.ErrProviderRequest) {
				t.Fatal(err)
			}
			if f.calls.calls.Load() != 1 || usage.RequestedModel != tc.model {
				t.Fatalf("retry or misattribution: %d %+v", f.calls.calls.Load(), usage)
			}
		})
	}
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-astra", "unknown"} {
		if _, err := New(t.Context(), Config{APIKey: "synthetic-key", Model: model, ReasoningEffort: "none"}); err == nil {
			t.Fatal("unsupported effort accepted", model)
		}
	}
}

func TestRuntimeSwitchReplaysOnlyVisibleConversation(t *testing.T) {
	app, _, _ := ledgerSession(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, _ *http.Request, n int, request obj) {
		input := encode(request["input"])
		if n == 1 || n == 3 {
			if strings.Contains(input, "encrypted_content") || strings.Contains(input, "rs_") || !strings.Contains(input, "first synthetic question") || !strings.Contains(input, "Synthetic complete") {
				t.Error("cross-model history lost visible turns or leaked native state")
			}
		}
		if n == 2 && !strings.Contains(input, "opaque_1") {
			t.Error("same-model native checkpoint lost")
		}
		if n == 0 && request["model"] != "gpt-6-sol" || n == 1 && request["model"] != "gpt-6.1-sol" {
			t.Error("selected model not on wire")
		}
		writeMode(w, request, []any{reasoningItem(fmt.Sprint(n)), textItem()})
	})
	dir := t.TempDir()
	pool := builtinai.NewPool(app, filepath.Join(dir, "capabilities"))
	store, err := aistate.Open(filepath.Join(dir, "private"), "synthetic-model-switch")
	if err != nil {
		t.Fatal(err)
	}
	usage, err := aiusage.Open(filepath.Join(dir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := assistant.NewRuntime(assistant.Options{Usage: usage, Ledger: "synthetic-model-switch", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
		session, err := pool.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		m, err := newModel(ctx, Config{BaseURL: "http://" + f.calls.host, APIKey: "synthetic-canary-key", Model: cfg.Model, ReasoningEffort: cfg.ReasoningEffort, Transport: f.calls}, true)
		if err != nil {
			return nil, err
		}
		return engineWithModel(ctx, cfg, m, builtinai.New(session, assistant.SelectedPage(ctx)), nil)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	service := airuntime.NewService(runtime)
	s := service.Status()
	cfg := s.Config
	cfg.Model = "gpt-6-sol"
	cfg.EnabledModels = []string{"gpt-6-sol", "gpt-6.1-sol"}
	cfg.Enabled = true
	if err = service.Configure(s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	s = service.Status()
	conversation, err := service.Create(s.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	expectedRuns := map[string]string{}
	for n, model := range []string{"gpt-6-sol", "gpt-6.1-sol", "gpt-6.1-sol", "gpt-6-sol"} {
		q := assistant.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Model: model, ReasoningEffort: "high", Prompt: "first synthetic question"}
		if n > 0 {
			q.Prompt = "follow-up synthetic question"
		}
		run, startErr := service.Start(q)
		if startErr != nil {
			t.Fatal(startErr)
		}
		expectedRuns[run.ID] = model
		waitAssistantRun(t, service, q)
	}
	calls, err := usage.Calls()
	if err != nil || len(calls) != 4 {
		t.Fatal("missing usage", calls, err)
	}
	for _, call := range calls {
		if call.RequestedModel != expectedRuns[call.RunID] || call.ConversationID != conversation.ID || call.Status != "succeeded" {
			t.Fatal("wrong usage attribution", call)
		}
	}
	if f.calls.calls.Load() != 4 {
		t.Fatal("unexpected fallback calls", f.calls.calls.Load())
	}
}
