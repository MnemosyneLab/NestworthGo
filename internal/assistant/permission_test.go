package assistant

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func (capabilityTestCredentials) Available() bool { return true }

func TestFullIsTheOnlyWritePermission(t *testing.T) {
	session := &capabilityTestSession{id: uuid.NewString(), state: "succeeded"}
	r, id, _, _ := capabilityRuntime(t, session)
	revision := r.state.Revision
	if err := r.SetFullAccess(t.Context(), r.epoch, revision, false); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().FullAccess {
		t.Fatal("revocation left Full enabled")
	}
	if err := r.SetFullAccess(t.Context(), r.epoch, r.state.Revision, true); err != ErrUnavailable {
		t.Fatalf("enabled during run: %v", err)
	}
	delete(r.active, id)
	if err := r.SetFullAccess(t.Context(), r.epoch, revision, true); err != ErrStale {
		t.Fatalf("old revision enabled writes: %v", err)
	}
	if err := r.SetFullAccess(t.Context(), r.epoch, r.state.Revision, true); err != nil {
		t.Fatal(err)
	}
	if !r.Snapshot().FullAccess {
		t.Fatal("explicit Full not enabled")
	}
}

type retiredPreviewSession struct {
	ReviewSession
	state   string
	retired int
}

func (s *retiredPreviewSession) AcquireReview(context.Context, string) (ReviewSession, error) {
	return s, nil
}
func (s *retiredPreviewSession) Inspect(_ context.Context, id string) (ChangeReview, error) {
	return ChangeReview{ID: id, State: s.state}, nil
}
func (s *retiredPreviewSession) RetireUnsubmitted(_ context.Context, id string) (ChangeReview, error) {
	s.retired++
	return ChangeReview{ID: id, State: "cancelled"}, nil
}

func TestIdlePreviewCleanupPreservesActiveAndUnknownOperations(t *testing.T) {
	for _, state := range []string{"awaiting_confirmation", "needs_preview", "unknown", "succeeded"} {
		for _, active := range []bool{false, true} {
			r, id, _, _ := capabilityRuntime(t, &capabilityTestSession{})
			session := &retiredPreviewSession{state: state}
			r.options.Reviews = session
			r.find(id).Reviews = []ReviewRef{{ID: uuid.NewString()}}
			if !active {
				delete(r.active, id)
			}
			result, err := r.Reviews(t.Context(), r.epoch, id)
			if err != nil {
				t.Fatal(err)
			}
			shouldRetire := !active && (state == "awaiting_confirmation" || state == "needs_preview")
			if (session.retired == 1) != shouldRetire {
				t.Fatalf("state=%s active=%v retired=%d", state, active, session.retired)
			}
			if state == "unknown" && (result[0].State != "unknown" || r.find(id).Reviews[0].Resolved) {
				t.Fatal("unknown receipt lost its fence")
			}
		}
	}
}

func TestRecoveredReceiptKeepsItsOriginalToolInvocation(t *testing.T) {
	session := &capabilityTestSession{id: uuid.NewString(), state: "unknown"}
	r, id, runID, _ := capabilityRuntime(t, session)
	r.progress = map[string]*Progress{id: {RunID: runID, Tools: []ToolProgress{
		{Source: "create_member", State: "succeeded"},
		{Source: "create_member", State: "running"},
	}}}
	_, err := r.executeCapability(t.Context(), r.epoch, id, runID, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	ref := r.find(id).Reviews[0]
	if ref.RunID != runID || ref.ToolIndex == nil || *ref.ToolIndex != 1 {
		t.Fatalf("wrong invocation: %+v", ref)
	}
	delete(r.active, id)
	result, err := r.Reviews(t.Context(), r.epoch, id)
	if err != nil || len(result) != 1 || result[0].State != "unknown" {
		t.Fatalf("unknown result: %+v %v", result, err)
	}
	session.state = "succeeded"
	result, err = r.Reviews(t.Context(), r.epoch, id)
	if err != nil || len(result) != 1 || result[0].State != "succeeded" || result[0].RunID != runID || result[0].ToolIndex == nil || *result[0].ToolIndex != 1 || result[0].ToolName != "create_member" {
		t.Fatalf("recovered result lost invocation: %+v %v", result, err)
	}
	if session.calls != 1 {
		t.Fatalf("inspection replayed write: %d", session.calls)
	}
}
