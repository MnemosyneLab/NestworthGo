package einoopenai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

type pageRecoverySession struct{ failure error }

func (s pageRecoverySession) Definitions() ([]agentcapability.Definition, error) {
	closed := false
	return []agentcapability.Definition{{Name: "operate_page_control", Effect: agentcapability.Interaction, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}, InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}}, nil
}
func (s pageRecoverySession) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, s.failure
}

func TestPageRefusalContinuesButUncertainOperationStops(t *testing.T) {
	for _, refused := range []bool{true, false} {
		failure := error(assistant.ErrStale)
		if refused {
			failure = submissionTestRejection{}
		}
		rounds := 0
		fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
			rounds++
			if round == 0 {
				call := callItem()
				call["name"] = "operate_page_control"
				call["arguments"] = `{}`
				writeMode(w, request, []any{call})
				return
			}
			if !refused || !strings.Contains(encode(request["input"]), "page_operation_refused") {
				t.Error("missing safe recovery diagnostic")
			}
			writeMode(w, request, []any{textItem()})
		})
		engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, pageRecoverySession{failure}, nil)
		if err != nil {
			t.Fatal(err)
		}
		state := ""
		ctx := assistant.WithProgress(t.Context(), func(event assistant.ProgressEvent) {
			if event.Kind == "tool" {
				state = event.State
			}
		})
		_, err = engine.Complete(ctx, nil, "Open synthetic asset changes")
		engine.Close()
		if refused {
			if err != nil || rounds != 2 || state != "refused" {
				t.Fatalf("refusal ended turn: %v rounds=%d state=%s", err, rounds, state)
			}
		} else if !errors.Is(err, assistant.ErrToolResult) || rounds != 1 {
			t.Fatalf("uncertain operation continued: %v rounds=%d", err, rounds)
		}
	}
}
