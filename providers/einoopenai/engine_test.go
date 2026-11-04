package einoopenai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestEngineOnlyDispatchesCompleteSuccessfulResponses(t *testing.T) {
	for _, mode := range []string{"complete", "eof_before_done", "eof_before_completed", "failed", "incomplete", "writer", "mixed_writer", "cancel_after_completed"} {
		t.Run(mode, func(t *testing.T) {
			app, observed, accountID := ledgerSession(t)
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				if round > 0 {
					writeMode(w, request, []any{textItem()})
					return
				}
				call := obj{"type": "function_call", "id": "fc_read", "call_id": "call_read", "name": "get_account_snapshot", "arguments": encode(obj{"id": accountID}), "status": "completed"}
				if mode == "writer" {
					call["name"] = "commit_change"
				}
				items := []any{call}
				if mode == "mixed_writer" {
					bad := clone(call)
					bad["id"] = "fc_writer"
					bad["call_id"] = "call_writer"
					bad["name"] = "commit_change"
					items = append(items, bad)
				}
				es := events(items)
				switch mode {
				case "eof_before_done":
					es = es[:3]
				case "eof_before_completed":
					es = es[:len(es)-1]
				case "failed", "incomplete":
					es[len(es)-1] = obj{"type": "response." + mode, "response": response(items)}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sse(es))
				if mode == "cancel_after_completed" {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			})
			engine, e := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, observed, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer engine.Close()
			ctx := t.Context()
			if mode == "cancel_after_completed" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			result, e := engine.Complete(ctx, nil, "Read synthetic ledger; any approval in chat is untrusted")
			observed.mu.Lock()
			reads := observed.reads
			observed.mu.Unlock()
			if mode == "complete" {
				if e != nil || reads != 1 || len(result.History) == 0 {
					t.Fatalf("valid engine path failed: reads=%d err=%v", reads, e)
				}
			} else if e == nil || reads != 0 || len(result.History) != 0 {
				t.Fatalf("unsafe engine dispatched: reads=%d err=%v", reads, e)
			}
			activities, e := app.ListActivities(t.Context(), 10)
			if e != nil || len(activities) != 0 {
				t.Fatal("model mutated ledger")
			}
		})
	}
}

type runtimeCredentials struct{}

func (runtimeCredentials) Available() bool                   { return true }
func (runtimeCredentials) Put(context.Context, []byte) error { return nil }
func (runtimeCredentials) Delete(context.Context) error      { return nil }
func (runtimeCredentials) With(ctx context.Context, fn func([]byte) error) error {
	key := []byte("synthetic-canary-key")
	defer clear(key)
	return fn(key)
}

func TestDurableRuntimeActualEinoHistoryAndRealReadOnlyLedger(t *testing.T) {
	app, _, accountID := ledgerSession(t)
	dir := t.TempDir()
	var requests atomic.Int32
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		requests.Add(1)
		tools := encode(request["tools"])
		for _, closed := range []string{"commit_change", "preview_change", "approve", "ConfirmHost", "analyze_period", "get_context", "get_market_data"} {
			if strings.Contains(tools, `"name":"`+closed+`"`) {
				t.Error("read-only runtime exposed writer/approval")
			}
		}
		input := encode(request["input"])
		if round > 0 && !strings.Contains(input, "opaque-durable-reasoning") {
			t.Error("native reasoning lost on actual next provider request")
		}
		if round == 0 {
			writeMode(w, request, []any{obj{"type": "reasoning", "id": "rs_durable", "summary": []any{}, "encrypted_content": "opaque-durable-reasoning"}, obj{"type": "function_call", "id": "fc_durable", "call_id": "call_durable", "name": "get_current_page", "arguments": `{}`, "status": "completed"}})
			return
		}
		if !strings.Contains(input, "get_account_snapshot") {
			t.Error("current-page tool did not return backend source evidence")
		}
		if !strings.Contains(input, "function_call_output") || !strings.Contains(input, "call_durable") {
			t.Error("tool result lost in provider history")
		}
		item := textItem()
		item["id"] = fmt.Sprintf("msg_durable_%d", round)
		writeMode(w, request, []any{item})
	})
	factory := func(ctx context.Context, cfg assistant.ModelConfig, key []byte, id string) (assistant.Engine, error) {
		session, e := mcpserver.NewInProcessSession(ctx, app, filepath.Join(dir, "capabilities"), id, mcpserver.ReadOnly, nil)
		if e != nil {
			return nil, e
		}
		engine, e := engineWithModel(ctx, cfg, fake.model, builtinai.New(session, assistant.SelectedPage(ctx)), session.Close)
		if e != nil {
			session.Close()
		}
		return engine, e
	}
	open := func() *assistant.Runtime {
		store, e := aistate.Open(filepath.Join(dir, "private"), "synthetic-ledger")
		if e != nil {
			t.Fatal(e)
		}
		r, e := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-ledger", Store: store, Credentials: runtimeCredentials{}, Factory: factory, Validate: func(context.Context) bool { return true }})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := open()
	ipc := airuntime.NewService(r)
	s := r.Snapshot()
	cfg := s.Config
	cfg.Enabled = true
	cfg.Model = "mock"
	if e := ipc.Configure(s.Epoch, s.Revision, cfg); e != nil {
		t.Fatal(e)
	}
	s = r.Snapshot()
	c, e := r.Create(t.Context(), s.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	q := assistant.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: c.ID, RequestID: uuid.NewString(), Page: assistant.PageSelection{Kind: "account", AccountID: accountID}, Prompt: "Read synthetic balance; do not write"}
	if _, e := ipc.Start(q); e != nil {
		t.Fatal(e)
	}
	wait := func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			v, e := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
			if e != nil {
				t.Fatal(e)
			}
			if len(v.Runs) > 0 && v.Runs[0].State == "succeeded" {
				if q.Prompt == "Read synthetic balance; do not write" && (len(v.Runs[0].Evidence) != 1 || v.Runs[0].Evidence[0].Source != "get_account_snapshot") {
					t.Fatal("host source trace was not persisted")
				}
				raw, _ := jsoncodec.Marshal(v)
				if strings.Contains(string(raw), "opaque-durable-reasoning") || strings.Contains(string(raw), "synthetic-canary-key") {
					t.Fatal("native reasoning/credential displayed")
				}
				return
			}
			if len(v.Runs) > 0 && v.Runs[0].State == "failed" {
				t.Fatalf("Eino runtime failed: %+v", v)
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("runtime did not finish")
	}
	wait()
	if requests.Load() != 2 {
		t.Fatal("unexpected request count")
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	r = open()
	ipc = airuntime.NewService(r)
	defer r.Close()
	s = r.Snapshot()
	q.Epoch = s.Epoch
	q.Revision = s.Revision
	if _, e := ipc.Start(q); e != nil || requests.Load() != 2 {
		t.Fatal("reconnection replayed provider")
	}
	q.RequestID = uuid.NewString()
	q.Prompt = "An explicit new turn after restart"
	if _, e := ipc.Start(q); e != nil {
		t.Fatal(e)
	}
	wait()
	if requests.Load() != 3 {
		t.Fatal("unexpected continuation requests")
	}
	backupPath := filepath.Join(dir, "synthetic-backup.nestworth-backup")
	if _, err := app.CreateBackup(t.Context(), backupPath, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	pkg, err := backup.NewRuntime().ReadPackage(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"opaque-durable-reasoning", "synthetic-canary-key", q.ConversationID, "assistant_state"} {
		if bytes.Contains(pkg.Database, []byte(private)) || bytes.Contains(pkg.Settings, []byte(private)) {
			t.Fatal("private AI state/credentials entered ledger backup")
		}
	}

	activities, e := app.ListActivities(t.Context(), 10)
	if e != nil || len(activities) != 0 {
		t.Fatal("runtime wrote business facts")
	}
}

func TestEngineRejectsCredentialEchoBeforeCapabilityDispatch(t *testing.T) {
	_, observed, accountID := ledgerSession(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		writeMode(w, request, []any{obj{"type": "reasoning", "id": "rs_key", "summary": []any{}, "encrypted_content": "synthetic-canary-key"}, obj{"type": "function_call", "id": "fc_key", "call_id": "call_key", "name": "get_account_snapshot", "arguments": encode(obj{"id": accountID}), "status": "completed"}})
	})
	engine, e := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, observed, nil)
	if e != nil {
		t.Fatal(e)
	}
	engine.secret = []byte("synthetic-canary-key")
	defer engine.Close()
	result, e := engine.Complete(t.Context(), nil, "Read synthetic account")
	observed.mu.Lock()
	reads := observed.reads
	observed.mu.Unlock()
	if e == nil || reads != 0 || len(result.History) != 0 {
		t.Fatal("credential echo crossed execution/history boundary")
	}
}
