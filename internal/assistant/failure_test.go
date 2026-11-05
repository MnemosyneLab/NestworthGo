package assistant_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ai "github.com/waltwang/nestworth-go/internal/assistant"
)

func TestFixedRunDiagnosticsPersistWithoutAdvancingHistory(t *testing.T) {
	for _, failure := range []error{ai.ErrProviderAuthentication, ai.ErrProviderRateLimited, ai.ErrProviderRequest, ai.ErrProviderUnavailable, ai.ErrProviderConnection, ai.ErrProviderResponse, ai.ProviderResponseFailure("stream_item_added"), ai.ProviderResponseFailure("sdk_conversion"), ai.ProviderResponseFailure("private-canary"), ai.ErrModelHistory, ai.ErrToolResult, ai.ErrBudget, ai.BudgetFailure("model_rounds"), ai.BudgetFailure("tool_calls"), ai.BudgetFailure("history_bytes"), ai.BudgetFailure("history_messages"), ai.BudgetFailure("prompt_bytes"), ai.BudgetFailure("text_bytes"), ai.BudgetFailure("provider_payload"), ai.BudgetFailure("tool_result_bytes"), ai.BudgetFailure("read_evidence"), ai.BudgetFailure(canary), context.DeadlineExceeded, errors.New(canary)} {
		t.Run(ai.FailureCode(failure), func(t *testing.T) {
			dir := privateTestDirectory(t)
			f := factory(func(_ context.Context, h []byte, p string) (ai.Completion, error) {
				if p == "hello" {
					return ai.Completion{Text: "hello", History: []byte("saved-native-history")}, nil
				}
				if string(h) != "saved-native-history" {
					t.Error("lost prior checkpoint")
				}
				if p == "recovery" {
					return ai.Completion{Text: "recovered", History: []byte("next-successful-history")}, nil
				}
				return ai.Completion{Text: canary, History: []byte(canary)}, fmt.Errorf("%s financial-canary: %w", canary, failure)
			})
			credentials := &credentials{available: true}
			r, store := open(t, dir, credentials, f, time.Second)
			enable(t, r)
			q := request(t, r)
			q.Prompt = "hello"
			if _, err := r.Start(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			if got := waitRun(t, r, q); got.State != "succeeded" {
				t.Fatal(got.State)
			}
			q.RequestID = "a609379f-16e7-482a-8d5b-9727bf9c049a"
			q.Prompt = "financial overview"
			if _, err := r.Start(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			got := waitRun(t, r, q)
			if got.State != "failed" || got.Code != ai.FailureCode(failure) || got.Text != "" || len(got.Evidence) != 0 {
				t.Fatalf("unsafe diagnostic: state=%s code=%s", got.State, got.Code)
			}
			state, err := store.Load()
			if err != nil || string(state.Conversations[0].History) != "saved-native-history" {
				t.Fatal("failure advanced checkpoint", err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			r, _ = open(t, dir, credentials, f, time.Second)
			defer r.Close()
			conversations, err := r.List(t.Context(), r.Snapshot().Epoch)
			if err != nil || len(conversations) != 1 {
				t.Fatal("diagnostic state did not reopen", err)
			}
			restored, err := r.Inspect(t.Context(), r.Snapshot().Epoch, q.ConversationID)
			if err != nil || len(restored.Runs) != 1 || restored.Runs[0].Code != got.Code {
				t.Fatal("diagnostic not durable", err)
			}
			q.Epoch = r.Snapshot().Epoch
			q.Revision = r.Snapshot().Revision
			q.RequestID = "e60e7cbe-c6bb-4a88-9b80-13c3dccd869e"
			q.Prompt = "recovery"
			if _, err := r.Start(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			if recovered := waitRun(t, r, q); recovered.State != "succeeded" || recovered.Text != "recovered" {
				t.Fatal("failed turn poisoned subsequent history", recovered.State)
			}
			for _, v := range restored.Runs {
				if strings.Contains(v.Code, canary) || strings.Contains(v.Text, canary) {
					t.Fatal("secret escaped")
				}
			}
		})
	}
}

func TestBudgetDiagnosticVocabularyIsClosed(t *testing.T) {
	for _, rule := range []string{"model_rounds", "tool_calls", "history_bytes", "history_messages", "prompt_bytes", "text_bytes", "provider_payload", "tool_result_bytes", "read_evidence"} {
		err := ai.BudgetFailure(rule)
		if !errors.Is(err, ai.ErrBudget) || !ai.IsBudgetFailureCode(ai.FailureCode(err)) {
			t.Fatal("lost budget identity", rule)
		}
	}
	for _, rule := range []string{canary, "model_rounds " + canary, "", "tool_calls." + canary} {
		if ai.BudgetFailure(rule) != ai.ErrBudget || ai.IsBudgetFailureCode("assistant_budget_exceeded."+rule) {
			t.Fatal("untrusted diagnostic accepted")
		}
	}
	if ai.NormalizeBudgetFailure(errors.New(canary)) != nil {
		t.Fatal("unknown error classified as budget")
	}
}
