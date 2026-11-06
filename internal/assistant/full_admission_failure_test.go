package assistant

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// This persistence fake stores detached JSON, can fail before or after storing
// admission, and pauses the actual runtime finish save for crash-window checks.
type admissionFailureStore struct {
	mu                                   sync.Mutex
	data                                 []byte
	inject, saveThen, failed, repaired   bool
	finishWaiting, allowFinish, finished chan struct{}
}

func (s *admissionFailureStore) Load() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state State
	err := jsoncodec.Unmarshal(s.data, &state)
	return state, err
}
func (s *admissionFailureStore) Save(state State) error {
	raw, err := jsoncodec.Marshal(state)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.inject && !s.failed {
		unresolved := false
		for _, c := range state.Conversations {
			for _, ref := range c.Reviews {
				unresolved = unresolved || !ref.Resolved
			}
		}
		if unresolved {
			s.failed = true
			if s.saveThen {
				s.data = raw
			}
			s.mu.Unlock()
			return errors.New("synthetic one-shot admission persistence failure")
		}
	}
	if s.failed && !s.repaired {
		close(s.finishWaiting)
		s.mu.Unlock()
		<-s.allowFinish
		s.mu.Lock()
		s.repaired = true
		s.data = raw
		close(s.finished)
	} else {
		s.data = raw
	}
	s.mu.Unlock()
	return nil
}
func (*admissionFailureStore) Close() error { return nil }

type admissionCredentials struct{ capabilityTestCredentials }

func (admissionCredentials) Available() bool { return true }

type admissionEngine struct {
	handler ToolHandler
	input   []byte
}

func (e admissionEngine) Complete(ctx context.Context, _ []byte, _ string) (Completion, error) {
	_, err := e.handler(ctx, e.input)
	return Completion{}, err
}
func (admissionEngine) Close() {}

func TestFullAdmissionSaveFailureFinishAndRestart(t *testing.T) {
	for _, saveThen := range []bool{false, true} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("saveThen=%v/retry=%v", saveThen, retry), func(t *testing.T) {
				initial := InitialState("synthetic-admission-restart")
				initial.Config = ModelConfig{Provider: "openai-responses", Model: "mock", Enabled: true}
				raw, _ := jsoncodec.Marshal(initial)
				store := &admissionFailureStore{data: raw, inject: true, saveThen: saveThen, finishWaiting: make(chan struct{}), allowFinish: make(chan struct{}), finished: make(chan struct{})}
				session := &capabilityTestSession{id: uuid.NewString(), state: "unknown"}
				request, _ := jsoncodec.Marshal(FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)})
				options := func(p Persistence) Options {
					return Options{Ledger: initial.Ledger, Store: p, Credentials: admissionCredentials{}, Reviews: session, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, _ ModelConfig, _ []byte, _ string) (Engine, error) {
						return admissionEngine{CurrentFullHandler(ctx), request}, nil
					}}
				}
				r, err := NewRuntime(options(store))
				if err != nil {
					t.Fatal(err)
				}
				var release sync.Once
				unblock := func() { release.Do(func() { close(store.allowFinish) }) }
				t.Cleanup(func() { unblock(); _ = r.Close() })
				if err := r.SetFullAccess(t.Context(), r.epoch, r.state.Revision, true); err != nil {
					t.Fatal(err)
				}
				c, err := r.Create(t.Context(), r.epoch)
				if err != nil {
					t.Fatal(err)
				}
				var previous []ReviewRef
				if retry {
					previous = []ReviewRef{{ID: session.id, Tool: "create_member", Resolved: true}}
					r.mu.Lock()
					r.find(c.ID).Reviews = append([]ReviewRef(nil), previous...)
					err = r.persist()
					r.mu.Unlock()
					if err != nil {
						t.Fatal(err)
					}
				}
				_, err = r.Start(t.Context(), StartRequest{Epoch: r.epoch, Revision: r.state.Revision, ConversationID: c.ID, RequestID: uuid.NewString(), Prompt: "Create a synthetic member"})
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-store.finishWaiting:
				case <-time.After(5 * time.Second):
					t.Fatal("actual finish did not reach the recovery save")
				}
				// Restart from the state that was durable at the failure, before finish can
				// repair it. A landed unresolved reference remains fenced; no replay occurs.
				before, err := store.Load()
				if err != nil {
					t.Fatal(err)
				}
				snapshot, _ := jsoncodec.Marshal(before)
				crashStore := &admissionFailureStore{data: snapshot}
				crash, err := NewRuntime(options(crashStore))
				if err != nil {
					t.Fatal(err)
				}
				if crash.pendingReview(crash.find(c.ID)) != saveThen {
					t.Fatal("crash window lost conservative durable fence")
				}
				if session.calls != 0 {
					t.Fatal("failed admission dispatched a mutation")
				}
				if err := crash.Close(); err != nil {
					t.Fatal(err)
				}
				unblock()
				select {
				case <-store.finished:
				case <-time.After(5 * time.Second):
					t.Fatal("actual finish save did not recover")
				}
				if err := r.Close(); err != nil {
					t.Fatal(err)
				}
				persisted, err := store.Load()
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(persisted.Conversations[0].Reviews, previous) {
					t.Fatalf("finish saved ghost/changed references: %+v want %+v", persisted.Conversations[0].Reviews, previous)
				}
				if persisted.Conversations[0].Runs[0].Run.Finished == nil {
					t.Fatal("did not exercise actual run completion")
				}
				session.state = "succeeded"
				next, err := NewRuntime(options(store))
				if err != nil {
					t.Fatal(err)
				}
				defer next.Close()
				if next.pendingReview(next.find(c.ID)) || next.fullEnabled {
					t.Fatal("restart retained ghost fence or Full grant")
				}
				reviews, err := next.Reviews(t.Context(), next.epoch, c.ID)
				if err != nil || len(reviews) != len(previous) {
					t.Fatalf("restart recovery: %+v %v", reviews, err)
				}
				if err := next.Delete(t.Context(), next.epoch, c.ID); err != nil {
					t.Fatal("ghost reference blocked deletion", err)
				}
				if session.calls != 0 {
					t.Fatal("finish/restart recovery dispatched a mutation")
				}
			})
		}
	}
}
