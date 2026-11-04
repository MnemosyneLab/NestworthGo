package assistant_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const canary = "synthetic-key-never-persist-884920"

type credentials struct {
	mu        sync.Mutex
	key       []byte
	available bool
	fail      bool
}

func (c *credentials) Available() bool { return c.available }
func (c *credentials) Put(_ context.Context, k []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key = append([]byte(nil), k...)
	return nil
}
func (c *credentials) Delete(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.key)
	c.key = nil
	return nil
}
func (c *credentials) With(ctx context.Context, fn func([]byte) error) error {
	c.mu.Lock()
	k := append([]byte(nil), c.key...)
	c.mu.Unlock()
	defer clear(k)
	if c.fail || len(k) == 0 {
		return errors.New(canary)
	}
	return fn(k)
}

type engine struct {
	fn func(context.Context, []byte, string) (ai.Completion, error)
}

func (e *engine) Complete(ctx context.Context, h []byte, p string) (ai.Completion, error) {
	return e.fn(ctx, h, p)
}
func (*engine) Close() {}
func factory(fn func(context.Context, []byte, string) (ai.Completion, error)) ai.EngineFactory {
	return func(_ context.Context, _ ai.ModelConfig, key []byte, _ string) (ai.Engine, error) {
		if string(key) != canary {
			return nil, errors.New(canary)
		}
		return &engine{fn}, nil
	}
}

// TempDir's numbered directory uses 0777 subject to the process umask. Give
// Open a dedicated child so the production code creates its required 0700
// directory under both ordinary 0022 and restrictive 0077 test environments.
func privateTestDirectory(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "assistant-private")
}
func open(t *testing.T, dir string, c *credentials, f ai.EngineFactory, timeout time.Duration) (*ai.Runtime, *aistate.Store) {
	t.Helper()
	store, err := aistate.Open(dir, "synthetic-ledger")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-ledger", Store: store, Credentials: c, Factory: f, Validate: func(ctx context.Context) bool { return ctx.Err() == nil }, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return r, store
}
func enable(t *testing.T, r *ai.Runtime) {
	t.Helper()
	s := r.Snapshot()
	if err := r.SetCredential(t.Context(), s.Epoch, []byte(canary)); err != nil {
		t.Fatal(err)
	}
	s = r.Snapshot()
	cfg := s.Config
	cfg.Model = "mock"
	cfg.Enabled = true
	if err := r.Configure(t.Context(), s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
}
func request(t *testing.T, r *ai.Runtime) ai.StartRequest {
	t.Helper()
	s := r.Snapshot()
	c, err := r.Create(t.Context(), s.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	return ai.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: c.ID, RequestID: uuid.NewString(), Prompt: "Read synthetic balance"}
}
func waitRun(t *testing.T, r *ai.Runtime, q ai.StartRequest) ai.Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, e := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
		if e != nil {
			t.Fatal(e)
		}
		if len(c.Runs) > 0 {
			v := c.Runs[len(c.Runs)-1]
			if v.State == "succeeded" || v.State == "failed" || v.State == "cancelled" || v.State == "interrupted" {
				return v
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("run never completed")
	return ai.Run{}
}
func TestDurableHistoryIdempotencyAndPublicPrivacy(t *testing.T) {
	dir := privateTestDirectory(t)
	c := &credentials{available: true}
	var calls atomic.Int32
	f := factory(func(_ context.Context, h []byte, p string) (ai.Completion, error) {
		n := calls.Add(1)
		if n == 1 && len(h) != 0 {
			t.Error("unexpected initial history")
		}
		if n == 2 && string(h) != "opaque-native-complete-history" {
			t.Error("native history lost after restart")
		}
		return ai.Completion{History: []byte("opaque-native-complete-history"), Text: "Exact synthetic USD 12.34"}, nil
	})
	r, store := open(t, dir, c, f, 0)
	enable(t, r)
	q := request(t, r)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := r.Start(context.Background(), q); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	run := waitRun(t, r, q)
	if run.State != "succeeded" || calls.Load() != 1 {
		t.Fatalf("duplicate request dispatched %d times, %s", calls.Load(), run.State)
	}
	bad := q
	bad.Prompt = "different"
	if _, e := r.Start(t.Context(), bad); !errors.Is(e, ai.ErrConflict) {
		t.Fatal("changed request admitted")
	}
	events, e := r.Events(t.Context(), q.Epoch, q.ConversationID, 0)
	if e != nil || len(events.Events) != 3 || events.Latest != 3 {
		t.Fatalf("bad durable events: %+v %v", events, e)
	}
	raw, _ := jsoncodec.Marshal(events)
	public, _ := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
	display, _ := jsoncodec.Marshal(public)
	if bytes.Contains(raw, []byte(canary)) || bytes.Contains(display, []byte("opaque-native")) {
		t.Fatal("secret/native state reached public output")
	}
	state, e := store.Load()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = jsoncodec.Marshal(state)
	if bytes.Contains(raw, []byte(canary)) {
		t.Fatal("credential persisted")
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	r, _ = open(t, dir, c, f, 0)
	defer r.Close()
	s := r.Snapshot()
	q.Epoch = s.Epoch
	q.Revision = s.Revision
	prior, e := r.Start(t.Context(), q)
	if e != nil || prior.ID != run.ID || calls.Load() != 1 {
		t.Fatal("reconnect replayed provider")
	}
	q.RequestID = uuid.NewString()
	q.Prompt = "Next explicit turn"
	if _, e := r.Start(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	if waitRun(t, r, q).State != "succeeded" || calls.Load() != 2 {
		t.Fatal("explicit continuation failed")
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e == nil && bytes.Contains(raw, []byte(canary)) {
			t.Fatal("credential reached disk")
		}
	}
}
func TestCancelTimeoutFailureAndTaintNeverAdvanceHistory(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "failure", "text_key", "history_key", "oversize", "panic"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{})
			var calls atomic.Int32
			f := factory(func(ctx context.Context, h []byte, p string) (ai.Completion, error) {
				calls.Add(1)
				close(started)
				switch mode {
				case "cancel", "timeout":
					<-ctx.Done()
					return ai.Completion{History: []byte("must-not-save"), Text: "late"}, nil
				case "panic":
					panic(canary)
				case "failure":
					return ai.Completion{}, errors.New(canary)
				case "text_key":
					return ai.Completion{History: []byte("valid"), Text: canary}, nil
				case "history_key":
					return ai.Completion{History: []byte(canary), Text: "safe"}, nil
				default:
					return ai.Completion{History: bytes.Repeat([]byte("a"), ai.MaxHistoryBytes+1), Text: "safe"}, nil
				}
			})
			timeout := time.Second
			if mode == "timeout" {
				timeout = 20 * time.Millisecond
			}
			r, store := open(t, privateTestDirectory(t), &credentials{available: true}, f, timeout)
			defer r.Close()
			enable(t, r)
			q := request(t, r)
			run, e := r.Start(t.Context(), q)
			if e != nil {
				t.Fatal(e)
			}
			<-started
			if mode == "cancel" {
				if e := r.Cancel(t.Context(), q.Epoch, q.ConversationID, run.ID); e != nil {
					t.Fatal(e)
				}
			}
			got := waitRun(t, r, q)
			if got.State == "succeeded" || got.Text != "" || strings.Contains(got.Code, canary) {
				t.Fatalf("unsafe completion: %+v", got)
			}
			state, e := store.Load()
			if e != nil || len(state.Conversations[0].History) != 0 {
				t.Fatal("failed history saved")
			}
			if _, e := r.Start(t.Context(), q); e != nil || calls.Load() != 1 {
				t.Fatal("failed/cancelled request replayed")
			}
		})
	}
}
func TestAdmissionBoundsAndCredentialTaint(t *testing.T) {
	r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, factory(func(context.Context, []byte, string) (ai.Completion, error) {
		t.Fatal("unexpected provider")
		return ai.Completion{}, nil
	}), 0)
	defer r.Close()
	enable(t, r)
	q := request(t, r)
	q.Prompt = "Please store " + canary
	if _, e := r.Start(t.Context(), q); !errors.Is(e, ai.ErrUnavailable) {
		t.Fatal("key prompt persisted")
	}
	q.Prompt = strings.Repeat("a", ai.MaxPromptBytes+1)
	if _, e := r.Start(t.Context(), q); !errors.Is(e, ai.ErrBudget) {
		t.Fatal("oversized prompt admitted")
	}
	q.Prompt = "valid"
	q.Epoch = uuid.NewString()
	if _, e := r.Start(t.Context(), q); !errors.Is(e, ai.ErrStale) {
		t.Fatal("stale host admitted")
	}
	closed, _ := open(t, privateTestDirectory(t), &credentials{}, factory(nil), 0)
	defer closed.Close()
	s := closed.Snapshot()
	cfg := s.Config
	cfg.Enabled = true
	if closed.Configure(t.Context(), s.Epoch, s.Revision, cfg) == nil || closed.Snapshot().Available {
		t.Fatal("unsupported secure storage enabled")
	}
}
func TestRevokeCancelsAndFencesLateResults(t *testing.T) {
	start := make(chan struct{})
	release := make(chan struct{})
	dir := privateTestDirectory(t)
	r, _ := open(t, dir, &credentials{available: true}, factory(func(context.Context, []byte, string) (ai.Completion, error) {
		close(start)
		<-release
		return ai.Completion{History: []byte("late-native"), Text: "late"}, nil
	}), 0)
	enable(t, r)
	q := request(t, r)
	if _, e := r.Start(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	<-start
	done := make(chan struct{})
	go func() { r.Revoke(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("restore fence drained a model")
	}
	if _, e := r.Inspect(t.Context(), q.Epoch, q.ConversationID); !errors.Is(e, ai.ErrStale) {
		t.Fatal("revoked host received messages")
	}
	close(release)
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := aistate.Open(dir, "synthetic-ledger")
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	state, e := reopened.Load()
	if e != nil || len(state.Conversations) != 1 || len(state.Conversations[0].History) != 0 || state.Conversations[0].Runs[0].Run.State != "interrupted" {
		t.Fatal("late revoked result persisted")
	}

}
func TestCrashHelper(t *testing.T) {
	dir := os.Getenv("NESTWORTH_AI_CRASH_FIXTURE")
	if dir == "" {
		return
	}
	r, _ := open(t, dir, &credentials{available: true}, factory(func(context.Context, []byte, string) (ai.Completion, error) { os.Exit(0); return ai.Completion{}, nil }), 0)
	enable(t, r)
	q := request(t, r)
	if _, e := r.Start(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	select {}
}
func TestAbruptProcessExitRecoversWithoutReplay(t *testing.T) {
	dir := privateTestDirectory(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
	cmd.Env = append(os.Environ(), "NESTWORTH_AI_CRASH_FIXTURE="+dir)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("crash fixture: %v %s", e, out)
	}
	var calls atomic.Int32
	r, _ := open(t, dir, &credentials{available: true, key: []byte(canary)}, factory(func(context.Context, []byte, string) (ai.Completion, error) {
		calls.Add(1)
		return ai.Completion{}, nil
	}), 0)
	defer r.Close()
	s := r.Snapshot()
	list, e := r.List(t.Context(), s.Epoch)
	if e != nil || len(list) != 1 {
		t.Fatal("durable admission lost")
	}
	c, e := r.Inspect(t.Context(), s.Epoch, list[0].ID)
	if e != nil || len(c.Runs) != 1 || c.Runs[0].State != "interrupted" || calls.Load() != 0 {
		t.Fatalf("bad crash recovery %+v %v", c, e)
	}
	q := ai.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: c.ID, RequestID: c.Runs[0].RequestID, Prompt: c.Runs[0].Prompt}
	got, e := r.Start(t.Context(), q)
	if e != nil || got.State != "interrupted" || calls.Load() != 0 {
		t.Fatal("crashed paid request automatically replayed")
	}
}

func TestRestorePurgesHistoryAndKeepsActivationClosed(t *testing.T) {
	dir := privateTestDirectory(t)
	creds := &credentials{available: true}
	r, _ := open(t, dir, creds, factory(func(context.Context, []byte, string) (ai.Completion, error) {
		return ai.Completion{History: []byte("pre-restore-native"), Text: "pre-restore"}, nil
	}), 0)
	enable(t, r)
	q := request(t, r)
	if _, e := r.Start(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	if waitRun(t, r, q).State != "succeeded" {
		t.Fatal("setup failed")
	}
	if e := r.FenceForRestore(); e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	r, _ = open(t, dir, creds, factory(nil), 0)
	defer r.Close()
	s := r.Snapshot()
	list, e := r.List(t.Context(), s.Epoch)
	if e != nil || len(list) != 0 || s.Config.Enabled {
		t.Fatal("old ledger conversations/activation restored")
	}
	q.Epoch = s.Epoch
	q.Revision = s.Revision
	if _, e := r.Start(t.Context(), q); !errors.Is(e, ai.ErrStale) {
		t.Fatal("old ledger request admitted")
	}
}
func TestPageHookRequiresCurrentHostLeaseAndStaysClosed(t *testing.T) {
	now := time.Now().UTC()
	lease := ai.PageBinding{Epoch: uuid.NewString(), Ledger: "synthetic-ledger", ConversationID: uuid.NewString(), PermissionRevision: 1, Surface: "desktop", ContextID: "opaque-host-context", PageVersion: 1, CapturedAt: now, ExpiresAt: now.Add(time.Minute)}
	if !ai.MatchesCurrentPage(lease, lease, now) {
		t.Fatal("valid prerequisite rejected")
	}
	changed := lease
	changed.PageVersion++
	if ai.MatchesCurrentPage(lease, changed, now) || ai.MatchesCurrentPage(lease, lease, now.Add(2*time.Minute)) {
		t.Fatal("stale page admitted")
	}
	host := ai.ClosedPageHost{}
	if _, e := host.Snapshot(t.Context(), lease); e == nil {
		t.Fatal("untrusted page supplied data")
	}
	if host.Navigate(t.Context(), ai.NavigationIntent{Version: 1, Binding: lease, Action: "open_object"}) == nil {
		t.Fatal("model/client asserted navigation authority")
	}
}

type failingStore struct {
	ai.Persistence
	fail atomic.Bool
}

func (s *failingStore) Save(state ai.State) error {
	if s.fail.Load() {
		return errors.New(canary)
	}
	return s.Persistence.Save(state)
}
func TestPersistenceFailureCannotAdmitOrPublishSuccess(t *testing.T) {
	for _, phase := range []string{"admission", "completion"} {
		t.Run(phase, func(t *testing.T) {
			dir := privateTestDirectory(t)
			base, e := aistate.Open(dir, "synthetic-ledger")
			if e != nil {
				t.Fatal(e)
			}
			store := &failingStore{Persistence: base}
			var calls atomic.Int32
			r, e := ai.NewRuntime(ai.Options{Ledger: "synthetic-ledger", Store: store, Credentials: &credentials{available: true}, Validate: func(context.Context) bool { return true }, Factory: factory(func(context.Context, []byte, string) (ai.Completion, error) {
				calls.Add(1)
				store.fail.Store(true)
				return ai.Completion{History: []byte("must-not-publish"), Text: "must-not-publish"}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			enable(t, r)
			q := request(t, r)
			if phase == "admission" {
				store.fail.Store(true)
			}
			_, e = r.Start(t.Context(), q)
			if phase == "admission" {
				if !errors.Is(e, ai.ErrStorage) || calls.Load() != 0 {
					t.Fatal("provider invoked before durable admission")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				deadline := time.Now().Add(time.Second)
				for r.Snapshot().Available && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
			}
			if _, e := r.Inspect(t.Context(), q.Epoch, q.ConversationID); !errors.Is(e, ai.ErrStale) {
				t.Fatal("undurable success was published")
			}
			if e := r.Close(); e != nil {
				t.Fatal(e)
			}
			r, _ = open(t, dir, &credentials{available: true, key: []byte(canary)}, factory(nil), 0)
			defer r.Close()
			s := r.Snapshot()
			c, e := r.Inspect(t.Context(), s.Epoch, q.ConversationID)
			if e != nil {
				t.Fatal(e)
			}
			if phase == "admission" && len(c.Runs) != 0 {
				t.Fatal("failed admission became durable")
			}
			if phase == "completion" && (len(c.Runs) != 1 || c.Runs[0].State != "interrupted" || c.Runs[0].Text != "") {
				t.Fatal("lost completion was replayed/published")
			}
		})
	}
}
func TestImmediateRestoreAfterAdmissionCannotStartOrRestoreOldConversation(t *testing.T) {
	r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, factory(func(ctx context.Context, _ []byte, _ string) (ai.Completion, error) {
		<-ctx.Done()
		return ai.Completion{}, ctx.Err()
	}), 0)
	enable(t, r)
	q := request(t, r)
	if _, e := r.Start(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	if e := r.FenceForRestore(); e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestNativeHistoryCannotBeSwappedBetweenConversations(t *testing.T) {
	dir := privateTestDirectory(t)
	creds := &credentials{available: true}
	r, store := open(t, dir, creds, factory(func(_ context.Context, _ []byte, p string) (ai.Completion, error) {
		return ai.Completion{History: []byte("private-history:" + p), Text: "synthetic"}, nil
	}), 0)
	enable(t, r)
	first := request(t, r)
	first.Prompt = "first conversation"
	if _, e := r.Start(t.Context(), first); e != nil {
		t.Fatal(e)
	}
	waitRun(t, r, first)
	second := request(t, r)
	second.Prompt = "second conversation"
	if _, e := r.Start(t.Context(), second); e != nil {
		t.Fatal(e)
	}
	waitRun(t, r, second)
	state, e := store.Load()
	if e != nil {
		t.Fatal(e)
	}
	state.Conversations[0].History = state.Conversations[1].History
	if e := store.Save(state); e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	if s, e := aistate.Open(dir, "synthetic-ledger"); e == nil {
		_ = s.Close()
		t.Fatal("cross-conversation checkpoint swap accepted")
	}
}

func TestFailureAndPanicValuesNeverReachLogs(t *testing.T) {
	var logs bytes.Buffer
	priorSlog := slog.Default()
	priorLog := log.Writer()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	log.SetOutput(&logs)
	defer func() { slog.SetDefault(priorSlog); log.SetOutput(priorLog) }()
	for _, mode := range []string{"failure", "panic"} {
		r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, factory(func(context.Context, []byte, string) (ai.Completion, error) {
			if mode == "panic" {
				panic(canary + " opaque-private-reasoning")
			}
			return ai.Completion{}, errors.New(canary + " opaque-private-reasoning")
		}), 0)
		enable(t, r)
		q := request(t, r)
		if _, err := r.Start(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		run := waitRun(t, r, q)
		if strings.Contains(run.Code, canary) || strings.Contains(run.Text, "opaque-private-reasoning") {
			t.Fatal("failure value disclosed")
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(logs.String(), canary) || strings.Contains(logs.String(), "opaque-private-reasoning") {
		t.Fatal("private failure/panic logged")
	}
}

type refusedDeletion struct{ credentials }

func (*refusedDeletion) Delete(context.Context) error { return errors.New(canary) }
func TestCredentialDeletionFailureStillDurablyDisablesActivation(t *testing.T) {
	dir := privateTestDirectory(t)
	store, err := aistate.Open(dir, "synthetic-ledger")
	if err != nil {
		t.Fatal(err)
	}
	creds := &refusedDeletion{credentials: credentials{available: true}}
	r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-ledger", Store: store, Credentials: creds, Factory: factory(nil), Validate: func(context.Context) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	enable(t, r)
	s := r.Snapshot()
	if err := r.DeleteCredential(t.Context(), s.Epoch); !errors.Is(err, ai.ErrUnavailable) {
		t.Fatal("unsafe credential error")
	}
	if r.Snapshot().Config.Enabled || r.Snapshot().Available {
		t.Fatal("deletion refusal retained activation")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, _ = open(t, dir, &creds.credentials, factory(nil), 0)
	defer r.Close()
	if r.Snapshot().Config.Enabled {
		t.Fatal("revoked activation returned after restart")
	}
}

func TestDesktopDisableCancelsActiveRunAndModelChangeResetsNativeHistory(t *testing.T) {
	c := &credentials{available: true}
	started := make(chan struct{})
	var count atomic.Int32
	f := factory(func(ctx context.Context, h []byte, _ string) (ai.Completion, error) {
		n := count.Add(1)
		if n == 1 {
			return ai.Completion{Text: "synthetic completed", History: []byte("old-model-native")}, nil
		}
		if n == 2 {
			close(started)
			<-ctx.Done()
			return ai.Completion{Text: "must not publish", History: []byte("cancelled-native")}, nil
		}
		if len(h) != 0 {
			t.Error("new model received old native history")
		}
		return ai.Completion{Text: "new model", History: []byte("new-model-native")}, nil
	})
	r, _ := open(t, privateTestDirectory(t), c, f, 0)
	defer r.Close()
	enable(t, r)
	q := request(t, r)
	q.Page = ai.PageSelection{Kind: "overview"}
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if waitRun(t, r, q).State != "succeeded" {
		t.Fatal("first run failed")
	}
	changed := q
	changed.Page = ai.PageSelection{Kind: "accounts"}
	if _, err := r.Start(t.Context(), changed); err != ai.ErrConflict {
		t.Fatal("same request ID changed page selector")
	}
	q.RequestID = uuid.NewString()
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	<-started
	status := r.Snapshot()
	cfg := status.Config
	cfg.Enabled = false
	if err := r.Configure(t.Context(), status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, r, q)
	if run.State == "succeeded" || run.Text != "" {
		t.Fatal("disabled run published")
	}
	status = r.Snapshot()
	cfg.Model = "second-model"
	cfg.Enabled = true
	if err := r.Configure(t.Context(), status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	q.Revision = r.Snapshot().Revision
	q.RequestID = uuid.NewString()
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if waitRun(t, r, q).State != "succeeded" {
		t.Fatal("model switch failed")
	}
	// A current page selector is data selection only and cannot carry routes,
	// arbitrary client metrics, or a principal.
	q.RequestID = uuid.NewString()
	q.Page = ai.PageSelection{Kind: "settings"}
	if _, err := r.Start(t.Context(), q); err == nil {
		t.Fatal("unsupported page accepted")
	}
}
