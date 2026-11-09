package einoopenai

import (
	"context"
	"errors"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

func TestFakeProviderFullDirectWrite(t *testing.T) {
	app, _, accountID, db := ledgerSessionDatabase(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		inventory := encode(request["tools"])
		if round == 0 {
			if strings.Contains(inventory, "execute_ledger_change") || !strings.Contains(inventory, loadToolGroups) {
				t.Error("initial catalog exposed an unloaded write or omitted discovery")
			}
			writeMode(w, request, []any{loadGroupsCall(round, "changes")})
			return
		}
		if !strings.Contains(inventory, "execute_ledger_change") || !strings.Contains(inventory, "commit_plan") || strings.Contains(inventory, "propose_cash_change") {
			t.Error("wrong Full inventory after group load")
		}
		if round == 1 {
			writeMode(w, request, []any{liveCallItem(round, "execute_ledger_change", obj{"kind": "money_added", "accountId": accountID, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"})})
			return
		}
		if round == 2 {
			if !strings.Contains(latestToolOutput(t, request), "succeeded") {
				t.Error("missing actual receipt")
			}
			writeMode(w, request, []any{liveCallItem(round, "get_account_valuations", obj{"accountIds": []string{accountID}})})
			return
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
	view := waitAssistantRun(t, ipc, q)
	if len(view.Runs[0].Tools) != 3 || view.Runs[0].Tools[0].Source != loadToolGroups {
		t.Fatalf("missing Full discovery sequence: %+v", view.Runs[0].Tools)
	}
	operation := view.Runs[0].Tools[1:]
	if operation[0].Source != "execute_ledger_change" || operation[1].Source != "get_account_valuations" || operation[0].State != "succeeded" || operation[1].State != "succeeded" || !strings.Contains(operation[0].Input, "12.34") || !strings.Contains(operation[0].Output, "succeeded") {
		t.Fatalf("Full/read sequence lost: %+v", view.Runs[0].Tools)
	}
	saved, err := store.Load()
	if err != nil || len(saved.Conversations[0].Runs[0].Run.Tools) != 3 {
		t.Fatalf("tool details not durable: %+v %v", saved, err)
	}
	rows, err := app.ListActivities(t.Context(), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("activities=%d err=%v", len(rows), err)
	}
	reviews, err := ipc.Reviews(s.Epoch, c.ID)
	if err != nil || len(reviews) != 1 || reviews[0].State != "succeeded" || reviews[0].Source != "ai_full" {
		t.Fatalf("audit=%+v err=%v", reviews, err)
	}
}

func TestFakeProviderFullDirectoryWrite(t *testing.T) {
	app, _, _, db := ledgerSessionDatabase(t)
	operationID := uuid.NewString()
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		inventory := encode(request["tools"])
		if round == 0 {
			if strings.Contains(inventory, "execute_ledger_change") || !strings.Contains(inventory, loadToolGroups) {
				t.Error("initial catalog exposed an unloaded write or omitted discovery")
			}
			writeMode(w, request, []any{loadGroupsCall(round, "changes")})
			return
		}
		if !strings.Contains(inventory, "execute_ledger_change") || !strings.Contains(inventory, "commit_plan") || strings.Contains(inventory, "propose_cash_change") {
			t.Error("wrong Full inventory after group load")
		}
		if round == 1 {
			writeMode(w, request, []any{liveCallItem(round, "create_member", obj{"operationId": operationID, "input": obj{"name": "Synthetic Full member"}})})
			return
		}
		if round == 2 {
			if !strings.Contains(latestToolOutput(t, request), "succeeded") {
				t.Error("missing actual receipt")
			}
			writeMode(w, request, []any{liveCallItem(round, "get_operation", obj{"id": operationID})})
			return
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
	view := waitAssistantRun(t, ipc, q)
	if len(view.Runs[0].Tools) != 3 || view.Runs[0].Tools[0].Source != loadToolGroups {
		t.Fatalf("missing Full discovery sequence: %+v", view.Runs[0].Tools)
	}
	operation := view.Runs[0].Tools[1:]
	if operation[0].Source != "create_member" || operation[1].Source != "get_operation" || operation[0].State != "succeeded" || operation[1].State != "succeeded" || !strings.Contains(operation[0].Input, "Synthetic Full member") || !strings.Contains(operation[0].Output, "succeeded") {
		t.Fatalf("Full/read sequence lost: %+v", view.Runs[0].Tools)
	}
	saved, err := store.Load()
	if err != nil || len(saved.Conversations[0].Runs[0].Run.Tools) != 3 {
		t.Fatalf("tool details not durable: %+v %v", saved, err)
	}
	rows, err := app.ListMembers(t.Context(), false)
	count := 0
	for _, row := range rows {
		if row.Name == "Synthetic Full member" {
			count++
		}
	}
	if err != nil || count != 1 {
		t.Fatalf("members=%d err=%v", count, err)
	}
	reviews, err := ipc.Reviews(s.Epoch, c.ID)
	if err != nil || len(reviews) != 1 || reviews[0].State != "succeeded" || reviews[0].Source != "ai_full" {
		t.Fatalf("audit=%+v err=%v", reviews, err)
	}
}

// Fail precisely when executeFull saves the resolved reference, after the
// actual application transaction and both host receipt layers have succeeded.
type failResolvedFullStore struct {
	assistant.Persistence
	saveThen bool
	failed   atomic.Bool
	hit      chan struct{}
	refID    string
}

func (s *failResolvedFullStore) Save(state assistant.State) error {
	for _, c := range state.Conversations {
		for _, ref := range c.Reviews {
			if ref.Resolved && s.failed.CompareAndSwap(false, true) {
				s.refID = ref.ID
				if s.saveThen {
					if err := s.Persistence.Save(state); err != nil {
						return err
					}
				}
				close(s.hit)
				return errors.New("synthetic resolved reference save failure")
			}
		}
	}
	return s.Persistence.Save(state)
}
func TestFullResolvedSaveFailureClosesRuntimeBeforeRetry(t *testing.T) {
	for _, saveThen := range []bool{false, true} {
		t.Run(fmt.Sprint("saveThen=", saveThen), func(t *testing.T) {
			app, _, accountID, db := ledgerSessionDatabase(t)
			input := obj{"kind": "money_added", "accountId": accountID, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}
			var rounds atomic.Int32
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				if round == 0 {
					writeMode(w, request, []any{loadGroupsCall(round, "changes")})
					return
				}
				rounds.Add(1)
				// Deliberately retry on any recovered tool refusal; no prompt compliance.
				writeMode(w, request, []any{liveCallItem(round, "execute_ledger_change", input)})
			})
			dir := t.TempDir()
			statePath := filepath.Join(dir, "state")
			capsPath := filepath.Join(dir, "caps")
			base, err := aistate.Open(statePath, "synthetic-full-failure")
			if err != nil {
				t.Fatal(err)
			}
			store := &failResolvedFullStore{Persistence: base, saveThen: saveThen, hit: make(chan struct{})}
			pool := builtinai.NewPool(app, capsPath, sqlite.NewConfigurationRepository(db))
			var captured *builtinai.Session
			runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-full-failure", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
				shared, err := pool.Get(ctx, id)
				if err != nil {
					return nil, err
				}
				captured = builtinai.New(shared, assistant.PageSelection{}).WithFullAccess(assistant.CurrentFullHandler(ctx))
				return engineWithModel(ctx, cfg, f.model, captured, nil)
			}})
			if err != nil {
				t.Fatal(err)
			}
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
			select {
			case <-store.hit:
			case <-time.After(10 * time.Second):
				t.Fatal("fault injection not reached")
			}
			// Even a caller deliberately discarding the cancelled run context cannot
			// reuse the captured callback to mint another operation in this runtime.
			raw, _ := jsoncodec.Marshal(input)
			if _, err := captured.Call(context.Background(), "execute_ledger_change", raw); err == nil {
				t.Fatal("fresh context bypassed closed runtime")
			}
			q.RequestID = uuid.NewString()
			if _, err := ipc.Start(q); err == nil {
				t.Fatal("new run admitted after storage failure")
			}
			if ipc.Status().FullAccess {
				t.Fatal("closed runtime advertised writes")
			}
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			if rounds.Load() != 1 {
				t.Fatalf("provider continued after failure: %d", rounds.Load())
			}
			rows, err := app.ListActivities(t.Context(), 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("duplicate activities=%d err=%v", len(rows), err)
			}
			// Reopen the real sidecar and resolve only the original ledger receipt.
			reopened, err := aistate.Open(statePath, "synthetic-full-failure")
			if err != nil {
				t.Fatal(err)
			}
			nextPool := builtinai.NewPool(app, capsPath, sqlite.NewConfigurationRepository(db))
			next, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-full-failure", Store: reopened, Credentials: runtimeCredentials{}, Capabilities: nextPool, Reviews: nextPool, Validate: func(context.Context) bool { return true }, Factory: func(context.Context, assistant.ModelConfig, []byte, string) (assistant.Engine, error) {
				return nil, assistant.ErrUnavailable
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			fresh := airuntime.NewService(next)
			status := fresh.Status()
			if status.FullAccess {
				t.Fatal("restart retained Full")
			}
			reviews, err := fresh.Reviews(status.Epoch, c.ID)
			if err != nil || len(reviews) != 1 || reviews[0].State != "succeeded" || reviews[0].Source != "ai_full" || reviews[0].ID != store.refID {
				t.Fatalf("original receipt recovery: %+v %v", reviews, err)
			}
			rows, err = app.ListActivities(t.Context(), 10)
			if err != nil || len(rows) != 1 {
				t.Fatal("recovery replayed writer")
			}
		})
	}
}

func TestSharedFullResolvedSaveFailureClosesRuntimeBeforeRetry(t *testing.T) {
	for _, saveThen := range []bool{false, true} {
		t.Run(fmt.Sprint("saveThen=", saveThen), func(t *testing.T) {
			app, _, _, db := ledgerSessionDatabase(t)
			input := obj{"operationId": uuid.NewString(), "input": obj{"name": "Synthetic shared failure"}}
			var rounds atomic.Int32
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				if round == 0 {
					writeMode(w, request, []any{loadGroupsCall(round, "changes")})
					return
				}
				rounds.Add(1)
				// Deliberately retry on any recovered tool refusal; no prompt compliance.
				writeMode(w, request, []any{liveCallItem(round, "create_member", input)})
			})
			dir := t.TempDir()
			statePath := filepath.Join(dir, "state")
			capsPath := filepath.Join(dir, "caps")
			base, err := aistate.Open(statePath, "synthetic-full-failure")
			if err != nil {
				t.Fatal(err)
			}
			store := &failResolvedFullStore{Persistence: base, saveThen: saveThen, hit: make(chan struct{})}
			pool := builtinai.NewPool(app, capsPath, sqlite.NewConfigurationRepository(db))
			var captured *builtinai.Session
			runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-full-failure", Store: store, Credentials: runtimeCredentials{}, Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, cfg assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
				shared, err := pool.Get(ctx, id)
				if err != nil {
					return nil, err
				}
				captured = builtinai.New(shared, assistant.PageSelection{}).WithFullAccess(assistant.CurrentFullHandler(ctx))
				return engineWithModel(ctx, cfg, f.model, captured, nil)
			}})
			if err != nil {
				t.Fatal(err)
			}
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
			select {
			case <-store.hit:
			case <-time.After(10 * time.Second):
				t.Fatal("fault injection not reached")
			}
			// Even a caller deliberately discarding the cancelled run context cannot
			// reuse the captured callback to mint another operation in this runtime.
			raw, _ := jsoncodec.Marshal(input)
			if _, err := captured.Call(context.Background(), "create_member", raw); err == nil {
				t.Fatal("fresh context bypassed closed runtime")
			}
			q.RequestID = uuid.NewString()
			if _, err := ipc.Start(q); err == nil {
				t.Fatal("new run admitted after storage failure")
			}
			if ipc.Status().FullAccess {
				t.Fatal("closed runtime advertised writes")
			}
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			if rounds.Load() != 1 {
				t.Fatalf("provider continued after failure: %d", rounds.Load())
			}
			count := func() int {
				rows, e := app.ListMembers(t.Context(), false)
				if e != nil {
					t.Fatal(e)
				}
				n := 0
				for _, row := range rows {
					if row.Name == "Synthetic shared failure" {
						n++
					}
				}
				return n
			}
			if count() != 1 {
				t.Fatal("duplicate members")
			}
			// Reopen the real sidecar and resolve only the original ledger receipt.
			reopened, err := aistate.Open(statePath, "synthetic-full-failure")
			if err != nil {
				t.Fatal(err)
			}
			nextPool := builtinai.NewPool(app, capsPath, sqlite.NewConfigurationRepository(db))
			next, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-full-failure", Store: reopened, Credentials: runtimeCredentials{}, Capabilities: nextPool, Reviews: nextPool, Validate: func(context.Context) bool { return true }, Factory: func(context.Context, assistant.ModelConfig, []byte, string) (assistant.Engine, error) {
				return nil, assistant.ErrUnavailable
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			fresh := airuntime.NewService(next)
			status := fresh.Status()
			if status.FullAccess {
				t.Fatal("restart retained Full")
			}
			reviews, err := fresh.Reviews(status.Epoch, c.ID)
			if err != nil || len(reviews) != 1 || reviews[0].State != "succeeded" || reviews[0].Source != "ai_full" || reviews[0].ID != store.refID {
				t.Fatalf("original receipt recovery: %+v %v", reviews, err)
			}
			if count() != 1 {
				t.Fatal("recovery replayed writer")
			}
		})
	}
}
