package assistant_test

import (
	"context"
	"strings"
	"testing"

	ai "github.com/waltwang/nestworth-go/internal/assistant"
)

func TestProgressIsProvisionalAndCancellationPersistsOnlyActualToolDetails(t *testing.T) {
	ready := make(chan struct{})
	f := factory(func(ctx context.Context, _ []byte, _ string) (ai.Completion, error) {
		ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "tool", Tool: 1, Source: "get_financial_context", State: "running", Input: `{"disclosure":"minimal"}`})
		ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "text", Text: "Synthetic unfinished preview"})
		close(ready)
		<-ctx.Done()
		ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "text", Text: "late stale text"})
		return ai.Completion{}, ctx.Err()
	})
	dir := privateTestDirectory(t)
	creds := &credentials{available: true}
	r, store := open(t, dir, creds, f, 0)
	enable(t, r)
	q := request(t, r)
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	<-ready
	c, err := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
	if err != nil || c.Progress == nil || c.Progress.Text != "Synthetic unfinished preview" || c.Runs[0].Text != "" {
		t.Fatalf("provisional state=%+v err=%v", c, err)
	}
	saved, err := store.Load()
	if err != nil || strings.Contains(string(saved.Conversations[0].History), "preview") || saved.Conversations[0].Runs[0].Run.Text != "" {
		t.Fatal("partial text persisted")
	}
	if err := r.Cancel(t.Context(), q.Epoch, q.ConversationID, c.Runs[0].ID); err != nil {
		t.Fatal(err)
	}
	done := waitRun(t, r, q)
	if done.State != "cancelled" || done.Text != "" || len(done.Tools) != 1 || done.Tools[0].State != "interrupted" {
		t.Fatalf("cancel result=%+v", done)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := open(t, dir, creds, f, 0)
	defer reopened.Close()
	restored, err := reopened.Inspect(t.Context(), reopened.Snapshot().Epoch, q.ConversationID)
	if err != nil || restored.Progress != nil || len(restored.Runs[0].Tools) != 1 || restored.Runs[0].Text != "" {
		t.Fatalf("restart=%+v err=%v", restored, err)
	}
}

func TestFullProgressThenReadPersistsExactDetailsAndSafeStates(t *testing.T) {
	for _, state := range []string{"succeeded", "refused", "failed"} {
		t.Run(state, func(t *testing.T) {
			f := factory(func(ctx context.Context, _ []byte, _ string) (ai.Completion, error) {
				ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "tool", Tool: 1, Source: "execute_ledger_change", State: "running", Input: `{"amount":"12.34567890","token":"private"}`})
				ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "tool", Tool: 1, Source: "execute_ledger_change", State: state, Output: `{"status":"` + state + `","amount":"12.34567890"}`})
				ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "tool", Tool: 2, Source: "get_account_valuations", State: "running", Input: `{"id":"synthetic"}`})
				ai.EmitProgress(ctx, ai.ProgressEvent{Kind: "tool", Tool: 2, Source: "get_account_valuations", State: "succeeded", Output: `{"amount":"12.34567890"}`})
				return ai.Completion{History: []byte(`{"synthetic":true}`), Text: "Synthetic tool result"}, nil
			})
			dir := privateTestDirectory(t)
			creds := &credentials{available: true}
			r, _ := open(t, dir, creds, f, 0)
			enable(t, r)
			q := request(t, r)
			if _, err := r.Start(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			done := waitRun(t, r, q)
			if len(done.Tools) != 2 || done.Tools[0].Source != "execute_ledger_change" || done.Tools[0].State != state || done.Tools[1].Source != "get_account_valuations" || strings.Contains(done.Tools[0].Input, "private") || !strings.Contains(done.Tools[0].Input, "12.34567890") || !strings.Contains(done.Tools[0].Output, "12.34567890") {
				t.Fatalf("invalid full/read trace: %+v", done.Tools)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			next, _ := open(t, dir, creds, f, 0)
			defer next.Close()
			view, err := next.Inspect(t.Context(), next.Snapshot().Epoch, q.ConversationID)
			if err != nil || len(view.Runs[0].Tools) != 2 || view.Runs[0].Tools[0].State != state {
				t.Fatalf("lost durable trace: %+v %v", view, err)
			}
		})
	}
}
