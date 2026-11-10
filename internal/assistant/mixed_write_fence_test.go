package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestPageUnknownFencesBothFullWriteChannelsButAllowsInspection(t *testing.T) {
	session := &capabilityTestSession{id: uuid.NewString(), state: "succeeded"}
	r, id, run, _ := capabilityRuntime(t, session)
	ctx := context.WithValue(t.Context(), runWriteFenceKey{}, &runWriteFence{})
	page := PageContent{ID: uuid.NewString(), Page: "synthetic", CapturedAt: time.Now(), Nodes: []PageNode{{ID: "button", Module: "page", Role: "button", TargetRef: uuid.NewString(), Actions: []string{"activate"}}}}
	args, _ := jsoncodec.Marshal(PageOperation{SnapshotID: page.ID, TargetRef: page.Nodes[0].TargetRef, Action: "activate"})
	done := make(chan error, 1)
	go func() { _, err := r.pageOperator(ctx, r.epoch, r.state.Revision)(ctx, args); done <- err }()
	deadline := time.Now().Add(time.Second)
	delivered := false
	queued := make(chan error, 2)
	for time.Now().Before(deadline) {
		reads, _ := r.PendingPageReads(ctx, r.epoch)
		if len(reads) > 0 {
			if err := r.DeliverPageContent(ctx, r.epoch, reads[0].ID, page); err != nil {
				t.Fatal(err)
			}
		}
		requests, _ := r.PendingPageOperations(ctx, r.epoch)
		if len(requests) > 0 {
			if _, err := r.ClaimPageOperation(ctx, r.epoch, requests[0].ID); err != nil {
				t.Fatal(err)
			}
			started := make(chan struct{}, 2)
			go func() {
				started <- struct{}{}
				_, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte("{}")})
				queued <- err
			}()
			go func() {
				started <- struct{}{}
				_, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "commit_plan", Arguments: []byte("{}")})
				queued <- err
			}()
			<-started
			<-started
			if err := r.DeliverPageOperation(ctx, r.epoch, requests[0].ID, PageOperationResult{State: "unverified", SnapshotID: page.ID}); err != nil {
				t.Fatal(err)
			}
			delivered = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !delivered {
		t.Fatal("missing page request")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte("{}")}); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("MCP writer bypassed page uncertainty: %v", err)
	}
	if _, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "commit_plan", Arguments: []byte("{}")}); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("ledger writer bypassed page uncertainty: %v", err)
	}
	for range 2 {
		if err := <-queued; !errors.Is(err, ErrRecoveryRequired) {
			t.Fatal("in-flight writer escaped unknown handoff", err)
		}
	}
	if session.calls != 0 {
		t.Fatal("write dispatched")
	}
	if _, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "get_operation", Arguments: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if session.calls != 1 {
		t.Fatal("inspection unavailable")
	}
}

func TestSharedUnknownAndInFlightWriteBlockPageClaims(t *testing.T) {
	session := &capabilityTestSession{id: uuid.NewString(), state: "unknown"}
	r, id, run, _ := capabilityRuntime(t, session)
	ctx := context.WithValue(t.Context(), runWriteFenceKey{}, &runWriteFence{})
	r.pageOperations = map[string]pendingPageOperation{"page": {operation: PageOperation{SnapshotID: uuid.NewString()}, revision: r.state.Revision, ctx: ctx, response: make(chan PageOperationResult, 1)}}
	session.invoke = func() error {
		if _, err := r.ClaimPageOperation(ctx, r.epoch, "page"); err == nil {
			t.Fatal("page claim crossed active MCP writer")
		}
		return nil
	}
	if _, err := r.executeCapability(ctx, r.epoch, id, run, r.state.Revision, FullCapabilityRequest{Name: "create_member", Arguments: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ClaimPageOperation(ctx, r.epoch, "page"); err == nil {
		t.Fatal("page claim crossed unresolved MCP outcome")
	}
}

func TestRunWriteGatePublishesUncertaintyBeforeQueuedWriter(t *testing.T) {
	ctx := context.WithValue(t.Context(), runWriteFenceKey{}, &runWriteFence{})
	release, err := acquireRunWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		next, err := acquireRunWrite(ctx)
		if err == nil {
			next(false)
		}
		done <- err
	}()
	<-started
	release(true)
	if err := <-done; !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("queued writer bypassed uncertainty", err)
	}
	fresh := context.WithValue(t.Context(), runWriteFenceKey{}, &runWriteFence{})
	release, err = acquireRunWrite(fresh)
	if err != nil {
		t.Fatal("fence persisted across runs", err)
	}
	release(false)
}

// Exercise Start's real handler injection, including a tool caller that supplies
// a detached context. Host authority belongs to the run, not caller context.
func TestStartInjectsSameWriteFenceIntoPageAndFullHandlers(t *testing.T) {
	session := &capabilityTestSession{id: uuid.NewString(), state: "succeeded"}
	r, id, _, _ := capabilityRuntime(t, session)
	r.find(id).Runs = nil
	delete(r.active, id)
	r.options.Credentials = operationCredentials{}
	r.options.Timeout = 5 * time.Second
	results := make(chan []error, 1)
	r.options.Factory = func(ctx context.Context, _ ModelConfig, _ []byte, _ string) (Engine, error) {
		if ctx.Value(runWriteFenceKey{}) == nil || CurrentPageOperator(ctx) == nil || CurrentFullHandler(ctx) == nil {
			results <- []error{errors.New("missing runtime-injected write fence or handlers")}
			return nil, ErrUnavailable
		}
		release, err := acquireRunWrite(ctx)
		if err != nil {
			results <- []error{err}
			return nil, ErrUnavailable
		}
		release(true)
		_, pageErr := CurrentPageOperator(ctx)(context.Background(), []byte("{}"))
		request, _ := jsoncodec.Marshal(FullCapabilityRequest{Name: "create_member", Arguments: []byte("{}")})
		_, mcpErr := CurrentFullHandler(ctx)(context.Background(), request)
		results <- []error{pageErr, mcpErr}
		return nil, ErrUnavailable
	}
	if _, err := r.Start(t.Context(), StartRequest{Epoch: r.epoch, Revision: r.state.Revision, ConversationID: id, RequestID: uuid.NewString(), Prompt: "Synthetic mixed write test"}); err != nil {
		t.Fatal(err)
	}
	select {
	case errs := <-results:
		for _, err := range errs {
			if !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("runtime handler lost run fence", err)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("runtime factory not invoked")
	}
	r.wg.Wait()
	if session.calls != 0 {
		t.Fatal("fenced runtime handler dispatched")
	}
}
