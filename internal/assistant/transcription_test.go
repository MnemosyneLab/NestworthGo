package assistant

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func voiceRuntime(t *testing.T, transcribe Transcriber) (*Runtime, AudioInput) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r := &Runtime{ctx: ctx, cancel: cancel, epoch: "test-epoch", active: map[string]context.CancelFunc{}, state: InitialState("synthetic"), options: Options{Validate: func(ctx context.Context) bool { return ctx.Err() == nil }, Credentials: voiceCredentials{}, Transcriber: transcribe}}
	r.state.Config = ModelConfig{Provider: "openai-responses", Enabled: true, Model: "test"}
	return r, AudioInput{Epoch: r.epoch, Revision: r.state.Revision, Audio: base64.StdEncoding.EncodeToString([]byte("synthetic-audio")), Format: "webm", DurationMS: 1000}
}
func TestTranscriptionDraftOnly(t *testing.T) {
	r, q := voiceRuntime(t, func(_ context.Context, key, audio []byte, format string) (string, error) {
		if len(key) == 0 || string(audio) != "synthetic-audio" || format != "webm" {
			t.Fatal("input")
		}
		return "  你好  ", nil
	})
	text, err := r.Transcribe(context.Background(), q)
	if err != nil || text != "你好" || len(r.state.Conversations) != 0 || len(r.active) != 0 {
		t.Fatalf("text=%q err=%v", text, err)
	}
}
func TestTranscriptionGuards(t *testing.T) {
	for _, change := range []func(*Runtime, *AudioInput){
		func(_ *Runtime, q *AudioInput) { q.DurationMS = 999 }, func(_ *Runtime, q *AudioInput) { q.DurationMS = 120001 }, func(_ *Runtime, q *AudioInput) { q.Audio = "!" }, func(_ *Runtime, q *AudioInput) { q.Format = "ogg" }, func(_ *Runtime, q *AudioInput) { q.Epoch = "old" }, func(_ *Runtime, q *AudioInput) { q.Revision++ }, func(r *Runtime, _ *AudioInput) { r.state.Config.Enabled = false }, func(r *Runtime, _ *AudioInput) { r.active[transcriptionSlot] = func() {} },
	} {
		r, q := voiceRuntime(t, func(context.Context, []byte, []byte, string) (string, error) {
			t.Fatal("unexpected upload")
			return "", nil
		})
		change(r, &q)
		if _, err := r.Transcribe(context.Background(), q); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
}
func TestTranscriptionCancellationAndSanitization(t *testing.T) {
	r, q := voiceRuntime(t, func(context.Context, []byte, []byte, string) (string, error) {
		return "", errors.New("private provider payload")
	})
	if _, err := r.Transcribe(context.Background(), q); err != ErrUnavailable {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.options.Transcriber = func(ctx context.Context, _, _ []byte, _ string) (string, error) {
		cancel()
		<-ctx.Done()
		return "late text", nil
	}
	if text, err := r.Transcribe(ctx, q); err != ErrStale || text != "" {
		t.Fatalf("%q %v", text, err)
	}
}

type voiceCredentials struct{}

func (voiceCredentials) Available() bool                   { return true }
func (voiceCredentials) Put(context.Context, []byte) error { return nil }
func (voiceCredentials) Delete(context.Context) error      { return nil }
func (voiceCredentials) With(_ context.Context, f func([]byte) error) error {
	return f([]byte("synthetic-secret"))
}

func TestTranscriptionRevocationAndConcurrentAdmission(t *testing.T) {
	entered := make(chan struct{})
	r, q := voiceRuntime(t, func(ctx context.Context, _, _ []byte, _ string) (string, error) {
		close(entered)
		<-ctx.Done()
		return "late text", nil
	})
	done := make(chan error, 1)
	go func() { _, err := r.Transcribe(context.Background(), q); done <- err }()
	<-entered
	if _, err := r.Transcribe(context.Background(), q); err != ErrConflict {
		t.Fatal(err)
	}
	r.Revoke()
	if err := <-done; err != ErrStale {
		t.Fatal(err)
	}
	if len(r.active) != 0 {
		t.Fatal("slot leaked")
	}
}
func TestTranscriptionRejectsInvalidAndStaleOutput(t *testing.T) {
	for _, text := range []string{"", "   ", "synthetic-secret", string([]byte{0xff}), strings.Repeat("a", 16385)} {
		r, q := voiceRuntime(t, func(context.Context, []byte, []byte, string) (string, error) { return text, nil })
		if got, err := r.Transcribe(context.Background(), q); err == nil || got != "" {
			t.Fatal("invalid output accepted")
		}
	}
	r, q := voiceRuntime(t, nil)
	r.options.Transcriber = func(context.Context, []byte, []byte, string) (string, error) {
		r.mu.Lock()
		r.state.Revision++
		r.mu.Unlock()
		return "stale", nil
	}
	if got, err := r.Transcribe(context.Background(), q); got != "" || err != ErrStale {
		t.Fatal("stale output accepted")
	}
}
