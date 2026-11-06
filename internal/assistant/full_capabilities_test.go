package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

type capabilityTestStore struct {
	Persistence
	saves int
}

func (s *capabilityTestStore) Save(State) error { s.saves++; return nil }

type capabilityTestCredentials struct{ Credentials }

func (capabilityTestCredentials) With(_ context.Context, f func([]byte) error) error {
	return f([]byte("synthetic-capability-test-key"))
}

type capabilityTestSession struct {
	preflightError error
	ReviewSession
	id     string
	state  string
	calls  int
	invoke func() error
}

func (s *capabilityTestSession) AcquireReview(context.Context, string) (ReviewSession, error) {
	return s, nil
}
func (s *capabilityTestSession) Capability(_ context.Context, name string, _ json.RawMessage) (agentcapability.Definition, string, error) {
	if s.preflightError != nil {
		return agentcapability.Definition{}, "", s.preflightError
	}
	effect := agentcapability.Write
	if name == "get_operation" {
		effect = agentcapability.Read
	}
	return agentcapability.Definition{Name: name, Effect: effect}, s.id, nil
}
func (s *capabilityTestSession) CallCapability(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.calls++
	if s.invoke != nil {
		if err := s.invoke(); err != nil {
			return nil, err
		}
	}
	return []byte(`{"data":{"status":"succeeded"}}`), nil
}
func (s *capabilityTestSession) InspectCapability(_ context.Context, id, name string) (ChangeReview, error) {
	return ChangeReview{ID: id, State: s.state, Source: "ai_full", Kind: "capability", Summary: name}, nil
}

type testUnsubmitted struct{}

func (testUnsubmitted) Error() string       { return "synthetic cancelled before dispatch" }
func (testUnsubmitted) NotDispatched() bool { return true }

func capabilityRuntime(t *testing.T, s *capabilityTestSession) (*Runtime, string, string, *capabilityTestStore) {
	t.Helper()
	id, runID := uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store := &capabilityTestStore{}
	r := &Runtime{ctx: ctx, cancel: cancel, epoch: uuid.NewString(), fullEnabled: true, writeEnabled: true, reviewBusy: map[string]bool{}, active: map[string]context.CancelFunc{id: func() {}}, state: InitialState("synthetic"), options: Options{Reviews: s, Store: store, Credentials: capabilityTestCredentials{}, Validate: func(context.Context) bool { return true }}}
	r.state.Config.Enabled = true
	r.state.Conversations = []ConversationState{{ID: id, Runs: []RunState{{Run: Run{ID: runID, State: "running"}}}}}
	return r, id, runID, store
}

func TestSharedFullAdmissionAndRecoveryFence(t *testing.T) {
	for _, state := range []string{"succeeded", "failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			s := &capabilityTestSession{id: uuid.NewString(), state: state}
			r, id, run, store := capabilityRuntime(t, s)
			s.invoke = func() error {
				if store.saves != 1 || len(r.find(id).Reviews) != 1 || r.find(id).Reviews[0].Resolved {
					t.Fatal("dispatch preceded durable unresolved reference")
				}
				if _, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)}); err == nil {
					t.Fatal("concurrent mutation admitted")
				}
				return nil
			}
			q := FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)}
			if _, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, q); err != nil {
				t.Fatal(err)
			}
			if r.pendingReview(r.find(id)) != (state == "unknown") {
				t.Fatal("wrong recovery fence")
			}
			s.invoke = nil
			if state == "unknown" {
				if _, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, q); !errors.Is(err, ErrRecoveryRequired) {
					t.Fatalf("unknown write needs existing-operation diagnostic: %v", err)
				} else {
					var marker interface{ NotDispatched() bool }
					if errors.As(err, &marker) {
						t.Fatal("prior uncertainty mislabeled as new argument rejection")
					}
				}
				if _, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "get_operation", Arguments: []byte(`{}`)}); err != nil {
					t.Fatal("receipt inspection blocked", err)
				}
			}
			r.fullEnabled = false
			before := s.calls
			if _, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, q); err == nil || s.calls != before {
				t.Fatal("mode switch retained authority")
			}
		})
	}
}

func TestSharedFullOnlyProvenUnsubmittedReleasesMissingReceipt(t *testing.T) {
	for _, err := range []error{testUnsubmitted{}, context.Canceled, errors.New("synthetic lost reply")} {
		s := &capabilityTestSession{id: uuid.NewString(), state: "unknown", invoke: func() error { return err }}
		r, id, run, _ := capabilityRuntime(t, s)
		_, _ = r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)})
		_, proven := err.(testUnsubmitted)
		ref := r.find(id).Reviews[0]
		if ref.Resolved != proven || ref.Unsubmitted != proven {
			t.Fatalf("incorrect cancellation inference for %v: %+v", err, ref)
		}
	}
}

func TestSharedFullArgumentRejectionHasNoRecoveryFence(t *testing.T) {
	s := &capabilityTestSession{id: uuid.NewString(), preflightError: agentcapability.ErrInvalidArguments}
	r, id, run, store := capabilityRuntime(t, s)
	_, err := r.executeCapability(t.Context(), r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)})
	var marker interface{ NotDispatched() bool }
	if !errors.As(err, &marker) || !marker.NotDispatched() {
		t.Fatalf("preflight lacks submission proof: %v", err)
	}
	if s.calls != 0 || store.saves != 0 || len(r.find(id).Reviews) != 0 {
		t.Fatal("argument rejection created an unresolved operation")
	}
}
