package einoopenai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type budgetSession struct {
	calls  int
	result json.RawMessage
	err    error
}

func (*budgetSession) Definitions() ([]agentcapability.Definition, error) {
	closed := false
	return []agentcapability.Definition{{Name: "get_overview", Effect: agentcapability.Read, Annotations: &agentcapability.Annotations{ReadOnlyHint: true, OpenWorldHint: &closed}, InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}}, nil
}
func (s *budgetSession) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.calls++
	if s.result == nil {
		return json.RawMessage(`{"hasMore":true,"synthetic":"financial-canary"}`), s.err
	}
	return s.result, s.err
}
func budgetEngine(t *testing.T, limits Limits, s *budgetSession, handler func(http.ResponseWriter, *http.Request, int, obj)) *Engine {
	t.Helper()
	fake := fakeModel(t, limits, handler)
	e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}
func budgetText(n int) obj {
	item := textItem()
	item["id"] = fmt.Sprintf("msg_budget_%d", n)
	return item
}
func wantBudget(t *testing.T, result assistant.Completion, err error, rule string) {
	t.Helper()
	want := "assistant_budget_exceeded." + rule
	if !errors.Is(err, assistant.ErrBudget) || err.Error() != want || assistant.FailureCode(err) != want || len(result.History) != 0 || result.Text != "" || len(result.Evidence) != 0 || result.Navigation != nil {
		t.Fatalf("unsafe or wrong budget failure: %v", err)
	}
}

func TestBudgetReservesEighthResponseAndRestoresOrdinaryFollowup(t *testing.T) {
	for _, violate := range []bool{false, true} {
		t.Run(fmt.Sprint(violate), func(t *testing.T) {
			s := &budgetSession{}
			e := budgetEngine(t, Limits{}, s, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if n < 7 {
					writeMode(w, request, []any{liveCallItem(n, "get_overview", obj{})})
					return
				}
				if n == 7 {
					if request["tool_choice"] != "none" || !strings.Contains(encode(request["input"]), "No more tools are available") || !strings.Contains(encode(request["input"]), "financial-canary") {
						t.Error("final response did not receive read evidence and forced no-tools policy")
					}
					if violate {
						writeMode(w, request, []any{liveCallItem(n, "get_overview", obj{})})
						return
					}
				} else {
					if request["tool_choice"] == "none" || strings.Contains(encode(request["input"]), "No more tools are available") {
						t.Error("finalization policy leaked into following request")
					}
					if !strings.Contains(encode(request["input"]), "call_live_6") {
						t.Error("native tool history lost in followup")
					}
				}
				writeMode(w, request, []any{budgetText(n)})
			})
			result, err := e.Complete(t.Context(), nil, "Synthetic account details")
			if s.calls != 7 {
				t.Fatalf("last round dispatched tools: %d", s.calls)
			}
			if violate {
				wantBudget(t, result, err, "model_rounds")
				return
			}
			if err != nil || len(result.History) == 0 {
				t.Fatal(err)
			}
			if bytes.Contains(result.History, []byte("No more tools are available")) {
				t.Fatal("transient finalization policy saved")
			}
			_, err = e.Complete(t.Context(), result.History, "Synthetic followup")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBudgetCallInventoryAndSixteenthCallSummary(t *testing.T) {
	for _, count := range []int{16, 17} {
		for _, violate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%t", count, violate), func(t *testing.T) {
				s := &budgetSession{}
				e := budgetEngine(t, Limits{}, s, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
					if n == 0 {
						items := []any{}
						for i := range count {
							items = append(items, liveCallItem(i, "get_overview", obj{}))
						}
						writeMode(w, request, items)
						return
					}
					if n != 1 || request["tool_choice"] != "none" {
						t.Error("tool cap did not force immediate summary")
					}
					if violate {
						writeMode(w, request, []any{liveCallItem(99, "get_overview", obj{})})
						return
					}
					writeMode(w, request, []any{budgetText(n)})
				})
				result, err := e.Complete(t.Context(), nil, "Synthetic details")
				if count == 17 {
					wantBudget(t, result, err, "tool_calls")
					if s.calls != 0 {
						t.Fatal("over-limit inventory partially dispatched")
					}
					return
				}
				if s.calls != 16 {
					t.Fatal("tool cap changed", s.calls)
				}
				if violate {
					wantBudget(t, result, err, "tool_calls")
				} else if err != nil || len(result.History) == 0 {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestBudgetSizeAndToolDiagnostics(t *testing.T) {
	for _, rule := range []string{"prompt_bytes", "history_bytes", "history_messages", "text_bytes", "provider_payload", "tool_result_bytes", "read_evidence"} {
		t.Run(rule, func(t *testing.T) {
			s := &budgetSession{}
			limits := Limits{}
			if rule == "provider_payload" {
				limits.Bytes = 16 << 10
			}
			if rule == "tool_result_bytes" {
				s.result = json.RawMessage(`"` + strings.Repeat("x", 64<<10) + `"`)
			}
			if rule == "read_evidence" {
				s.err = fmt.Errorf("synthetic-canary-key: %w", assistant.BudgetFailure(rule))
			}
			e := budgetEngine(t, limits, s, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if rule == "tool_result_bytes" || rule == "read_evidence" {
					writeMode(w, request, []any{liveCallItem(n, "get_overview", obj{})})
					return
				}
				item := budgetText(n)
				item["content"].([]any)[0].(obj)["text"] = strings.Repeat("x", assistant.MaxTextBytes+1)
				writeMode(w, request, []any{item})
			})
			prompt := "synthetic"
			var history []byte
			switch rule {
			case "prompt_bytes":
				prompt = strings.Repeat("x", assistant.MaxPromptBytes+1)
			case "history_bytes":
				history = bytes.Repeat([]byte("x"), assistant.MaxHistoryBytes+1)
			case "history_messages":
				saved := checkpoint{Version: 1, Provider: "openai-responses", Model: "mock", Messages: []*schema.AgenticMessage{schema.SystemAgenticMessage("synthetic")}}
				for range 128 {
					saved.Messages = append(saved.Messages, schema.UserAgenticMessage("synthetic"))
				}
				history, _ = jsoncodec.Marshal(saved)
			}
			result, err := e.Complete(t.Context(), history, prompt)
			wantBudget(t, result, err, rule)
		})
	}
}

func TestBudgetNativeReplayLimitDoesNotReportCorruptHistory(t *testing.T) {
	s := &budgetSession{}
	e := budgetEngine(t, Limits{Items: 1}, s, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
		writeMode(w, request, []any{budgetText(n)})
	})
	first, err := e.Complete(t.Context(), nil, "synthetic first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.Complete(t.Context(), first.History, "synthetic second")
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.Complete(t.Context(), second.History, "synthetic third")
	wantBudget(t, result, err, "provider_payload")
}
