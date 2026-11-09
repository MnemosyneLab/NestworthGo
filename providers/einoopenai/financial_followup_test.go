package einoopenai

import (
	"fmt"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"net/http"
	"strings"
	"testing"
)

// Actual SDK requests and controlled SQLite read after a greeting, with no live provider.
func TestGreetingThenCurrentFinancialContext(t *testing.T) {
	_, observed, _ := ledgerSession(t)
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		for _, raw := range request["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["strict"] != false {
				t.Error("optional capability schemas need explicit strict:false")
			}
			if tool["name"] == "get_financial_context" {
				parameters := tool["parameters"].(map[string]any)
				if required, ok := parameters["required"].([]any); ok && len(required) != 0 {
					t.Error("optional context fields became required")
				}
			}
		}
		switch round {
		case 0:
			writeMode(w, request, []any{obj{"type": "reasoning", "id": "rs_greeting", "summary": []any{}, "encrypted_content": "synthetic-opaque-reasoning"}, textItem()})
		case 1:
			if !strings.Contains(encode(request["input"]), "Synthetic complete") || !strings.Contains(encode(request["input"]), "synthetic-opaque-reasoning") {
				t.Error("lost greeting history")
			}
			items := []any{obj{"type": "function_call", "id": "fc_context", "call_id": "call_context", "name": "get_financial_context", "arguments": `{}`, "status": "completed"}}
			es := events(items)
			es[2] = obj{"type": "response.function_call_arguments.done", "output_index": 0, "item_id": "fc_context", "arguments": `{}`}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sse(es))
		case 2:
			output := latestToolOutput(t, request)
			if !strings.Contains(output, `"contextId"`) || strings.Contains(output, `"retryAction"`) {
				t.Error("financial read did not reach next request")
			}
			item := textItem()
			item["id"] = "msg_financial"
			writeMode(w, request, []any{item})
		default:
			t.Error("unexpected provider request")
		}
	})
	engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, streamCompletionModel{fake.model}, builtinai.New(observed, assistant.PageSelection{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	greeting, err := engine.Complete(t.Context(), nil, "你好")
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Complete(t.Context(), greeting.History, "能给我介绍下现在的财务情况吗")
	if err != nil || len(result.History) == 0 || len(result.Evidence) != 1 {
		t.Fatalf("followup failed: evidence=%d err=%v", len(result.Evidence), err)
	}
}

func TestArgumentDoneSnapshotStillRequiresMatchingSuccessfulCompletion(t *testing.T) {
	for _, mode := range []string{"valid", "valid_nonempty", "mismatch_item", "mismatch_delta", "duplicate_done", "late_delta", "invalid_object", "missing_completed", "failed", "trailing_data"} {
		t.Run(mode, func(t *testing.T) {
			item := callItem()
			item["arguments"] = `{}`
			if mode == "valid_nonempty" {
				item["arguments"] = `{"amount":"12.34"}`
			}
			es := events([]any{item})
			done := obj{"type": "response.function_call_arguments.done", "output_index": 0, "item_id": "fc_0", "arguments": item["arguments"]}
			es[2] = done
			switch mode {
			case "mismatch_item":
				done["arguments"] = `{"x":1}`
			case "mismatch_delta":
				es = append(es[:2], append([]obj{{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc_0", "delta": `{"x":1}`}}, es[2:]...)...)
			case "duplicate_done":
				es = append(es[:3], append([]obj{clone(done)}, es[3:]...)...)
			case "late_delta":
				es = append(es[:3], append([]obj{{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc_0", "delta": ""}}, es[3:]...)...)
			case "invalid_object":
				done["arguments"] = `[]`
			case "missing_completed":
				es = es[:len(es)-1]
			case "failed":
				es[len(es)-1]["type"] = "response.failed"
			}
			wire := sse(es)
			if mode == "trailing_data" {
				wire += "data: invalid\n\n"
			}
			_, _, err := validateStream([]byte(wire), Limits{Items: 128, Events: 4096})
			if (err == nil) != (mode == "valid" || mode == "valid_nonempty") {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}
