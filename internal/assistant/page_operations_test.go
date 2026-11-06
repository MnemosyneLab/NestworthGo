package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type operationCredentials struct{}

func (operationCredentials) Available() bool                   { return true }
func (operationCredentials) Put(context.Context, []byte) error { return nil }
func (operationCredentials) Delete(context.Context) error      { return nil }
func (operationCredentials) With(_ context.Context, f func([]byte) error) error {
	return f([]byte("synthetic-secret"))
}
func operationRuntime() *Runtime {
	return &Runtime{ctx: context.Background(), epoch: "synthetic-epoch", fullEnabled: true, state: State{Revision: 1, Config: ModelConfig{Enabled: true}}, options: Options{Validate: func(ctx context.Context) bool { return ctx.Err() == nil }, Credentials: operationCredentials{}}, pageOperations: map[string]pendingPageOperation{}}
}
func TestPageOperationClaimFences(t *testing.T) {
	for _, mode := range []string{"normal", "readonly", "revision", "cancel", "review", "busy", "epoch"} {
		t.Run(mode, func(t *testing.T) {
			r := operationRuntime()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			op := PageOperation{SnapshotID: uuid.NewString(), TargetRef: uuid.NewString(), Action: "activate"}
			r.pageOperations["request"] = pendingPageOperation{operation: op, revision: 1, ctx: ctx, response: make(chan PageOperationResult, 1)}
			epoch := r.epoch
			switch mode {
			case "readonly":
				r.fullEnabled = false
			case "revision":
				r.state.Revision++
			case "cancel":
				cancel()
			case "review":
				r.state.Conversations = []ConversationState{{Reviews: []ReviewRef{{ID: "unresolved"}}}}
			case "busy":
				r.reviewBusy = map[string]bool{"active": true}
			case "epoch":
				epoch = "old"
			}
			_, err := r.ClaimPageOperation(t.Context(), epoch, "request")
			if mode != "normal" {
				if err == nil {
					t.Fatal("accepted revoked authority")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = r.ClaimPageOperation(t.Context(), epoch, "request"); err == nil {
				t.Fatal("replayed claim")
			}
			pending, err := r.PendingPageOperations(t.Context(), epoch)
			if err != nil || len(pending) != 0 {
				t.Fatal("claimed request republished")
			}
			if err = r.DeliverPageOperation(t.Context(), epoch, "request", PageOperationResult{State: "succeeded", SnapshotID: op.SnapshotID}); err == nil {
				t.Fatal("financial success assertion accepted")
			}
			if err = r.DeliverPageOperation(t.Context(), epoch, "request", PageOperationResult{State: "dispatched", SnapshotID: op.SnapshotID}); err != nil {
				t.Fatal(err)
			}
			if err = r.DeliverPageOperation(t.Context(), epoch, "request", PageOperationResult{State: "dispatched", SnapshotID: op.SnapshotID}); err == nil {
				t.Fatal("replayed result")
			}
		})
	}
}
func TestPageOperationFreshCaptureAndUnknownFence(t *testing.T) {
	r := operationRuntime()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	operator := r.pageOperator(ctx, r.epoch, 1)
	snapshot := PageContent{ID: uuid.NewString(), Page: "synthetic", CapturedAt: time.Now(), Nodes: []PageNode{{ID: "button", Module: "page", Role: "button", TargetRef: uuid.NewString(), Actions: []string{"activate"}}}}
	op := PageOperation{SnapshotID: snapshot.ID, TargetRef: snapshot.Nodes[0].TargetRef, Action: "activate"}
	args, _ := jsoncodec.Marshal(op)
	done := make(chan json.RawMessage, 1)
	fail := make(chan error, 1)
	go func() { raw, err := operator(ctx, args); done <- raw; fail <- err }()
	deadline := time.Now().Add(time.Second)
	var reads []PageReadRequest
	for time.Now().Before(deadline) {
		reads, _ = r.PendingPageReads(ctx, r.epoch)
		if len(reads) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(reads) != 1 {
		t.Fatal("missing fresh capture")
	}
	if err := r.DeliverPageContent(ctx, r.epoch, reads[0].ID, snapshot); err != nil {
		t.Fatal(err)
	}
	var requests []PageReadRequest
	for time.Now().Before(deadline) {
		requests, _ = r.PendingPageOperations(ctx, r.epoch)
		if len(requests) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(requests) != 1 {
		t.Fatal("missing operation")
	}
	if _, err := r.ClaimPageOperation(ctx, r.epoch, requests[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := r.DeliverPageOperation(ctx, r.epoch, requests[0].ID, PageOperationResult{State: "unverified", SnapshotID: snapshot.ID}); err != nil {
		t.Fatal(err)
	}
	<-done
	if err := <-fail; err != nil {
		t.Fatal(err)
	}
	op.SnapshotID = uuid.NewString()
	args, _ = jsoncodec.Marshal(op)
	if _, err := operator(ctx, args); err == nil {
		t.Fatal("unknown outcome permitted another dispatch")
	}
}

func TestPageOperationTimeoutClassification(t *testing.T) {
	for _, mode := range []string{"unclaimed", "claimed", "expired_claim", "accepted", "acknowledged_no_dispatch"} {
		t.Run(mode, func(t *testing.T) {
			r := operationRuntime()
			snapshot := uuid.NewString()
			response := make(chan PageOperationResult, 1)
			expiry := time.Now().Add(time.Second)
			if mode == "expired_claim" {
				expiry = time.Now().Add(-time.Second)
			}
			r.pageOperations["request"] = pendingPageOperation{
				operation: PageOperation{SnapshotID: snapshot, ExpiresAt: &expiry}, revision: 1,
				ctx: t.Context(), claimed: mode != "unclaimed", response: response,
			}
			want := "unverified"
			switch mode {
			case "unclaimed":
				want = "not_dispatched"
			case "accepted", "acknowledged_no_dispatch":
				want = "dispatched"
				if mode == "acknowledged_no_dispatch" {
					want = "not_dispatched"
				}
				if err := r.DeliverPageOperation(t.Context(), r.epoch, "request", PageOperationResult{State: want, SnapshotID: snapshot}); err != nil {
					t.Fatal(err)
				}
			}
			result := r.expirePageOperation("request", snapshot, response)
			if result.State != want || result.SnapshotID != snapshot {
				t.Fatalf("got %+v, want %s", result, want)
			}
			if _, err := r.ClaimPageOperation(t.Context(), r.epoch, "request"); err == nil {
				t.Fatal("timeout left request claimable")
			}
			if err := r.DeliverPageOperation(t.Context(), r.epoch, "request", PageOperationResult{State: "dispatched", SnapshotID: snapshot}); err == nil {
				t.Fatal("late result accepted")
			}
		})
	}
}

func TestPageOperationDeliveryRejectsUnseenSnapshot(t *testing.T) {
	r := operationRuntime()
	snapshot := uuid.NewString()
	response := make(chan PageOperationResult, 1)
	r.pageOperations["request"] = pendingPageOperation{operation: PageOperation{SnapshotID: snapshot}, revision: 1, ctx: t.Context(), claimed: true, response: response}
	if err := r.DeliverPageOperation(t.Context(), r.epoch, "request", PageOperationResult{State: "stale", SnapshotID: uuid.NewString()}); err == nil {
		t.Fatal("accepted unseen replacement snapshot")
	}
	if len(response) != 0 {
		t.Fatal("published unseen observation")
	}
	if err := r.DeliverPageOperation(t.Context(), r.epoch, "request", PageOperationResult{State: "stale", SnapshotID: snapshot}); err != nil {
		t.Fatal(err)
	}
}
