package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Drive the same request/claim/deliver protocol as the mounted frontend, with
// fresh synthetic snapshots after each acknowledged gesture.
func TestPageOperationBatchSequentialFreshState(t *testing.T) {
	for _, mode := range []string{"complete", "blocked", "unverified", "replaced", "review", "readonly"} {
		t.Run(mode, func(t *testing.T) {
			r := operationRuntime()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			snapshot := PageContent{ID: uuid.NewString(), Page: "synthetic", CapturedAt: time.Now(), Nodes: []PageNode{
				{ID: "first", Module: "page", Role: "textbox", TargetRef: uuid.NewString(), Actions: []string{"set_value"}},
				{ID: "second", Module: "page", Role: "button", TargetRef: uuid.NewString(), Actions: []string{"activate"}},
				{ID: "third", Module: "page", Role: "button", TargetRef: uuid.NewString(), Actions: []string{"activate"}},
			}}
			initialID := snapshot.ID
			args, _ := jsoncodec.Marshal(map[string]any{"snapshotId": snapshot.ID, "actions": []any{
				map[string]any{"targetRef": snapshot.Nodes[0].TargetRef, "action": "set_value", "value": "12.3400"},
				map[string]any{"targetRef": snapshot.Nodes[1].TargetRef, "action": "activate"},
				map[string]any{"targetRef": snapshot.Nodes[2].TargetRef, "action": "activate"},
			}})
			operator := r.pageOperator(ctx, r.epoch, 1)
			done := make(chan json.RawMessage, 1)
			failures := make(chan error, 1)
			go func() { raw, err := operator(ctx, args); done <- raw; failures <- err }()
			dispatched, captures := 0, 0
			for {
				select {
				case raw := <-done:
					if err := <-failures; err != nil {
						t.Fatal(err)
					}
					var result PageOperationBatchResult
					if err := jsoncodec.Unmarshal(raw, &result); err != nil {
						t.Fatal(err)
					}
					if mode == "complete" {
						if result.State != "dispatched" || result.Partial || len(result.Results) != 3 || dispatched != 3 {
							t.Fatalf("complete result: %+v, dispatches %d", result, dispatched)
						}
					} else {
						if !result.Partial || len(result.Results) != 2 || result.Results[0].State != "dispatched" || result.State == "dispatched" {
							t.Fatalf("lost partial outcome: %+v", result)
						}
						if mode == "blocked" || mode == "unverified" {
							if dispatched != 2 {
								t.Fatal(dispatched)
							}
						} else if dispatched != 1 {
							t.Fatal(dispatched)
						}
					}
					if captures < len(result.Results) {
						t.Fatalf("missing fresh capture: %d", captures)
					}
					if mode == "unverified" {
						if _, err := operator(ctx, args); err == nil {
							t.Fatal("unknown outcome retried")
						}
					}
					return
				case <-ctx.Done():
					t.Fatal("batch did not finish")
				default:
				}
				reads, _ := r.PendingPageReads(ctx, r.epoch)
				for _, read := range reads {
					snapshot.CapturedAt = time.Now()
					if err := r.DeliverPageContent(ctx, r.epoch, read.ID, snapshot); err != nil {
						t.Fatal(err)
					}
					captures++
				}
				requests, _ := r.PendingPageOperations(ctx, r.epoch)
				for _, request := range requests {
					op, err := r.ClaimPageOperation(ctx, r.epoch, request.ID)
					if err != nil {
						t.Fatal(err)
					}
					if op.SnapshotID != snapshot.ID || dispatched > 0 && op.SnapshotID == initialID {
						t.Fatal("used stale snapshot", op)
					}
					state := "dispatched"
					if dispatched == 1 && (mode == "blocked" || mode == "unverified") {
						state = mode
					}
					if err := r.DeliverPageOperation(ctx, r.epoch, request.ID, PageOperationResult{State: state, SnapshotID: op.SnapshotID}); err != nil {
						t.Fatal(err)
					}
					dispatched++
					snapshot.ID = uuid.NewString()
					snapshot.Nodes[0].Value = "12.3400"
					if dispatched == 1 {
						switch mode {
						case "replaced":
							snapshot.Nodes[1].TargetRef = uuid.NewString()
						case "review":
							r.mu.Lock()
							r.state.Conversations = []ConversationState{{Reviews: []ReviewRef{{ID: "pending"}}}}
							r.mu.Unlock()
						case "readonly":
							r.mu.Lock()
							r.fullEnabled = false
							r.mu.Unlock()
						}
					}
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestPageOperationBatchValidatesWholeEnvelope(t *testing.T) {
	snapshot, target := uuid.NewString(), uuid.NewString()
	valid := map[string]any{"targetRef": target, "action": "activate"}
	for _, bad := range []any{
		map[string]any{"targetRef": target, "action": "set_value"},
		map[string]any{"targetRef": target, "action": "activate", "snapshotId": snapshot},
		map[string]any{"targetRef": target, "action": "set_value", "value": "synthetic-secret"},
		map[string]any{"targetRef": target, "action": "activate", "selector": "button"},
		nil,
	} {
		r := operationRuntime()
		args, _ := jsoncodec.Marshal(map[string]any{"snapshotId": snapshot, "actions": []any{valid, bad}})
		if _, err := r.pageOperator(t.Context(), r.epoch, 1)(t.Context(), args); err == nil {
			t.Fatal("invalid later action accepted")
		}
		if len(r.pageReads) != 0 || len(r.pageOperations) != 0 {
			t.Fatal("invalid batch began execution")
		}
	}
}

func TestPageOperationBatchCancellationAfterClaimIsPartialUnknown(t *testing.T) {
	r := operationRuntime()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	snapshot := PageContent{ID: uuid.NewString(), Page: "synthetic", CapturedAt: time.Now(), Nodes: []PageNode{{ID: "button", Module: "page", Role: "button", TargetRef: uuid.NewString(), Actions: []string{"activate"}}}}
	args, _ := jsoncodec.Marshal(map[string]any{"snapshotId": snapshot.ID, "actions": []any{map[string]any{"targetRef": snapshot.Nodes[0].TargetRef, "action": "activate"}}})
	done := make(chan json.RawMessage, 1)
	failures := make(chan error, 1)
	operator := r.pageOperator(ctx, r.epoch, 1)
	go func() { raw, err := operator(ctx, args); done <- raw; failures <- err }()
	deadline := time.Now().Add(time.Second)
	claimed := false
	for time.Now().Before(deadline) && !claimed {
		reads, _ := r.PendingPageReads(ctx, r.epoch)
		for _, read := range reads {
			if err := r.DeliverPageContent(ctx, r.epoch, read.ID, snapshot); err != nil {
				t.Fatal(err)
			}
		}
		requests, _ := r.PendingPageOperations(ctx, r.epoch)
		for _, request := range requests {
			if _, err := r.ClaimPageOperation(ctx, r.epoch, request.ID); err != nil {
				t.Fatal(err)
			}
			claimed = true
			cancel()
		}
		time.Sleep(time.Millisecond)
	}
	if !claimed {
		t.Fatal("missing claim")
	}
	select {
	case raw := <-done:
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
		var result PageOperationBatchResult
		if err := jsoncodec.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if !result.Partial || result.State != "unverified" || len(result.Results) != 1 {
			t.Fatalf("unknown first dispatch lost: %+v", result)
		}
		if _, err := operator(t.Context(), args); err == nil {
			t.Fatal("canceled unknown batch retried")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not return")
	}
}
