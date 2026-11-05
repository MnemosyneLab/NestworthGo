package assistant_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
)

func titleRuntime(t *testing.T, dir string, gen ai.TitleGenerator, timeout time.Duration) *ai.Runtime {
	t.Helper()
	store, err := aistate.Open(dir, "synthetic-ledger")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-ledger", Store: store, Credentials: &credentials{available: true, key: []byte(canary)}, Factory: factory(func(context.Context, []byte, string) (ai.Completion, error) {
		return ai.Completion{Text: "Synthetic answer with unrelated ledger canary", History: []byte("private native history")}, nil
	}), TitleGenerator: gen, TitleTimeout: timeout, Validate: func(ctx context.Context) bool { return ctx.Err() == nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}
func waitTitle(t *testing.T, r *ai.Runtime, q ai.StartRequest, source string) ai.Conversation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		if c.TitleSource == source {
			return c
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("title did not settle to " + source)
	return ai.Conversation{}
}
func TestTitleOnceMinimalInputPersistsAndDoesNotBlockAnswer(t *testing.T) {
	entered, release := make(chan string, 1), make(chan struct{})
	var calls atomic.Int32
	dir := privateTestDirectory(t)
	r := titleRuntime(t, dir, func(ctx context.Context, cfg ai.ModelConfig, key []byte, prompt string) (string, error) {
		calls.Add(1)
		if cfg.Model != ai.TitleModel || cfg.Provider != "openai-responses" || string(key) != canary {
			t.Error("wrong provider/model/key")
		}
		entered <- prompt
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return "家庭资产趋势", nil
	}, time.Second)
	enable(t, r)
	q := request(t, r)
	q.Prompt = strings.Repeat("投资趋势", 200)
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if run := waitRun(t, r, q); run.State != "succeeded" {
		t.Fatal(run)
	}
	if prompt := <-entered; len([]rune(prompt)) != 512 || strings.Contains(prompt, "ledger canary") {
		t.Fatal("wrong disclosure")
	}
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	waitTitle(t, r, q, "pending")
	close(release)
	c := waitTitle(t, r, q, "luna")
	if c.Title != "家庭资产趋势" {
		t.Fatal(c.Title)
	}
	q.RequestID = uuid.NewString()
	q.Prompt = "Follow up"
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	waitRun(t, r, q)
	if calls.Load() != 1 {
		t.Fatal("duplicate title call")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r2 := titleRuntime(t, dir, func(context.Context, ai.ModelConfig, []byte, string) (string, error) {
		t.Error("restart replayed title")
		return "", nil
	}, time.Second)
	c, err := r2.Inspect(t.Context(), r2.Snapshot().Epoch, q.ConversationID)
	if err != nil || c.Title != "家庭资产趋势" || c.TitleSource != "luna" {
		t.Fatal(c, err)
	}
}
func TestLateTitleCannotOverrideManualDeleteOrConfiguration(t *testing.T) {
	for _, mode := range []string{"manual", "delete", "config", "credential", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			r := titleRuntime(t, privateTestDirectory(t), func(context.Context, ai.ModelConfig, []byte, string) (string, error) {
				close(entered)
				<-release
				return "Late generated title", nil
			}, time.Second)
			enable(t, r)
			q := request(t, r)
			_, err := r.Start(t.Context(), q)
			if err != nil {
				t.Fatal(err)
			}
			waitRun(t, r, q)
			<-entered
			switch mode {
			case "manual":
				err = r.Rename(t.Context(), q.Epoch, q.ConversationID, "我的标题")
			case "delete":
				err = r.Delete(t.Context(), q.Epoch, q.ConversationID)
			case "config":
				s := r.Snapshot()
				cfg := s.Config
				cfg.Enabled = false
				err = r.Configure(t.Context(), s.Epoch, s.Revision, cfg)
			case "credential":
				err = r.DeleteCredential(t.Context(), q.Epoch)
			case "revoke":
				r.Revoke()
			}
			close(release)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "revoke" {
				return
			}
			if mode == "delete" {
				if _, err = r.Inspect(t.Context(), q.Epoch, q.ConversationID); err == nil {
					t.Fatal("deleted conversation returned")
				}
				return
			}
			source := "local"
			if mode == "manual" {
				source = "manual"
			}
			c := waitTitle(t, r, q, source)
			// Close waits for the deliberately late provider before reopening persisted state.
			if err = r.Close(); err != nil {
				t.Fatal(err)
			}
			if mode == "manual" && c.Title != "我的标题" {
				t.Fatal(c)
			}
		})
	}
}
func TestTitleFailuresKeepLocalAndRestartDoesNotReplay(t *testing.T) {
	for _, mode := range []string{"unsupported", "empty", "secret", "panic", "timeout", "restart", "unconfigured"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			gen := ai.TitleGenerator(func(ctx context.Context, _ ai.ModelConfig, _ []byte, _ string) (string, error) {
				calls.Add(1)
				switch mode {
				case "unsupported":
					return "", errors.New("provider unavailable")
				case "empty":
					return " \n ", nil
				case "secret":
					return canary, nil
				case "panic":
					panic(canary)
				case "timeout", "restart":
					<-ctx.Done()
					return "late", nil
				}
				return "", nil
			})
			if mode == "unconfigured" {
				gen = nil
			}
			dir := privateTestDirectory(t)
			r := titleRuntime(t, dir, gen, 20*time.Millisecond)
			enable(t, r)
			q := request(t, r)
			_, err := r.Start(t.Context(), q)
			if err != nil {
				t.Fatal(err)
			}
			waitRun(t, r, q)
			if mode == "restart" {
				waitTitle(t, r, q, "pending")
				if err = r.Close(); err != nil {
					t.Fatal(err)
				}
				r = titleRuntime(t, dir, gen, time.Second)
				q.Epoch = r.Snapshot().Epoch
			}
			c := waitTitle(t, r, q, "local")
			if c.Title != "Read synthetic balance" {
				t.Fatal(c.Title)
			}
		})
	}
}
func TestManualTitleBeforeFirstTurnAndConcurrentConversations(t *testing.T) {
	var calls atomic.Int32
	r := titleRuntime(t, privateTestDirectory(t), func(_ context.Context, _ ai.ModelConfig, _ []byte, p string) (string, error) {
		calls.Add(1)
		return "Topic " + p, nil
	}, time.Second)
	enable(t, r)
	q := request(t, r)
	if err := r.Rename(t.Context(), q.Epoch, q.ConversationID, "Manual first"); err != nil {
		t.Fatal(err)
	}
	_, _ = r.Start(t.Context(), q)
	waitRun(t, r, q)
	waitTitle(t, r, q, "manual")
	q2, q3 := request(t, r), request(t, r)
	q2.Prompt = "Alpha"
	q3.Prompt = "Beta"
	_, _ = r.Start(t.Context(), q2)
	_, _ = r.Start(t.Context(), q3)
	waitRun(t, r, q2)
	waitRun(t, r, q3)
	if waitTitle(t, r, q2, "luna").Title != "Topic Alpha" || waitTitle(t, r, q3, "luna").Title != "Topic Beta" || calls.Load() != 2 {
		t.Fatal("cross-conversation title")
	}
	for _, bad := range []string{"", strings.Repeat("界", 65), "line\nbreak", canary} {
		if err := r.Rename(t.Context(), q.Epoch, q.ConversationID, bad); err == nil {
			t.Fatal("accepted invalid title")
		}
	}
}
