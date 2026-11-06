package airuntime

import (
	"context"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
)

func TestTranscribeNilRuntime(t *testing.T) {
	text, err := NewService(nil).Transcribe(context.Background(), assistant.AudioInput{})
	if text != "" || !errors.Is(err, assistant.ErrUnavailable) {
		t.Fatalf("nil runtime returned text=%q err=%v", text, err)
	}
}

type transcriptionState struct{ state assistant.State }

func (s *transcriptionState) Load() (assistant.State, error) { return s.state, nil }
func (s *transcriptionState) Save(state assistant.State) error {
	s.state = state
	return nil
}
func (*transcriptionState) Close() error { return nil }

type transcriptionCredentials struct{}

func (transcriptionCredentials) Available() bool                   { return true }
func (transcriptionCredentials) Put(context.Context, []byte) error { return nil }
func (transcriptionCredentials) Delete(context.Context) error      { return nil }
func (transcriptionCredentials) With(_ context.Context, use func([]byte) error) error {
	return use([]byte("synthetic-voice-key"))
}

func transcriptionService(t *testing.T, transcribe assistant.Transcriber) (*Service, assistant.AudioInput) {
	t.Helper()
	state := assistant.InitialState("synthetic-voice-ledger")
	state.Config = assistant.ModelConfig{Provider: "openai-responses", Model: "synthetic-model", Enabled: true}
	runtime, err := assistant.NewRuntime(assistant.Options{
		Ledger: state.Ledger, Store: &transcriptionState{state: state}, Credentials: transcriptionCredentials{},
		Validate: func(ctx context.Context) bool { return ctx.Err() == nil },
		Factory: func(context.Context, assistant.ModelConfig, []byte, string) (assistant.Engine, error) {
			t.Error("transcription invoked conversation engine")
			return nil, assistant.ErrUnavailable
		},
		Transcriber: transcribe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	service := NewService(runtime)
	status := service.Status()
	return service, assistant.AudioInput{
		Epoch: status.Epoch, Revision: status.Revision,
		Audio: base64.StdEncoding.EncodeToString([]byte("synthetic-audio")), Format: "webm", DurationMS: 1000,
	}
}

func TestTranscribeAlreadyCancelledContextDoesNotUpload(t *testing.T) {
	service, input := transcriptionService(t, func(context.Context, []byte, []byte, string) (string, error) {
		t.Error("cancelled IPC reached provider")
		return "unexpected transcript", nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	text, err := service.Transcribe(ctx, input)
	if text != "" || !errors.Is(err, assistant.ErrStale) {
		t.Fatalf("cancelled IPC returned text=%q err=%v", text, err)
	}
}

func TestTranscribeIPCCancellationAbortsProviderAndReleasesSlot(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	service, input := transcriptionService(t, func(ctx context.Context, _, _ []byte, _ string) (string, error) {
		if calls.Add(1) != 1 {
			return "fresh draft", nil
		}
		close(entered)
		select {
		case <-ctx.Done():
			// Even a provider that returns success after cancellation cannot emit text.
			return "late draft", nil
		case <-release:
			return "", assistant.ErrUnavailable
		}
	})
	// Release a regressed provider before runtime.Close waits for outstanding work.
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := service.Transcribe(ctx, input)
		done <- result{text, err}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("provider was not entered")
	}
	cancel()
	select {
	case got := <-done:
		if got.text != "" || !errors.Is(got.err, assistant.ErrStale) {
			t.Fatalf("cancelled IPC returned text=%q err=%v", got.text, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("IPC context cancellation did not reach provider")
	}
	text, err := service.Transcribe(context.Background(), input)
	if err != nil || text != "fresh draft" {
		t.Fatalf("cancelled transcription retained admission slot: text=%q err=%v", text, err)
	}
}
