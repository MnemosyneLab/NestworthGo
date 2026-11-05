package einoopenai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
)

func TestLiveTextBeforeTerminalNeverCommitsBeforeEOF(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			preview := make(chan string, 16)
			sent := make(chan struct{})
			release := make(chan struct{}, 1)
			defer close(release)
			item := textItem()
			text := "Synthetic streaming answer with **Markdown** and exact USD 1234.5678."
			item["content"] = []any{obj{"type": "output_text", "text": text, "annotations": []any{}}}
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, _ int, request obj) {
				if request["stream"] != true {
					t.Error("live engine did not activate SSE")
				}
				es := contractEvents([]any{item}, "", nil)
				wire := sse(es)
				// Send through response.completed but deliberately keep wire EOF pending.
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, wire)
				w.(http.Flusher).Flush()
				close(sent)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if fail {
					fmt.Fprint(w, "data: malformed-trailing-frame\n\n")
				}
			})
			_, session, _ := ledgerSession(t)
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, session, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			engine.secret = []byte("synthetic-key")
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx = assistant.WithProgress(ctx, func(event assistant.ProgressEvent) {
				if event.Kind == "text" && event.Text != "" {
					preview <- event.Text
				}
			})
			go func() {
				result, err := engine.Complete(ctx, nil, "Synthetic request")
				if fail && len(result.History) > 0 {
					t.Error("failed stream exposed history")
				}
				done <- err
			}()
			select {
			case <-sent:
			case <-time.After(3 * time.Second):
				t.Fatal("no SSE")
			}
			select {
			case got := <-preview:
				if !strings.HasPrefix(text, got) || got == text {
					t.Fatal("not a provisional secret-safe prefix")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no live text")
			}
			select {
			case <-done:
				t.Fatal("completion escaped before EOF")
			default:
			}
			release <- struct{}{}
			select {
			case err := <-done:
				if (err != nil) != fail {
					t.Fatalf("completion=%v fail=%v", err, fail)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("completion stuck")
			}
		})
	}
}

func TestLiveToolsEmitActualOrderedDetailsAfterValidation(t *testing.T) {
	_, session, account := ledgerSession(t)
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		items := []any{textItem()}
		if round == 0 {
			items = []any{obj{"type": "function_call", "id": "fc_live", "call_id": "call_live", "name": "get_account_snapshot", "arguments": encode(obj{"id": account}), "status": "completed"}}
		}
		writeContract(w, request, items, "", nil)
	})
	engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, session, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	var events []assistant.ProgressEvent
	ctx := assistant.WithProgress(t.Context(), func(event assistant.ProgressEvent) {
		if event.Kind == "tool" {
			events = append(events, event)
		}
	})
	result, err := engine.Complete(ctx, nil, "Read synthetic account")
	if err != nil || result.Text == "" {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].State != "running" || events[1].State != "succeeded" || events[0].Tool != 1 || events[1].Tool != 1 {
		t.Fatalf("events=%+v", events)
	}
	if !strings.Contains(events[0].Input, account) || !strings.Contains(events[1].Output, "Synthetic checking 合成账户") {
		t.Fatalf("actual details missing: %+v", events)
	}
}

func TestLiveCredentialSplitAcrossTextDeltasNeverEscapes(t *testing.T) {
	const secret = "synthetic-private-key-canary"
	_, session, _ := ledgerSession(t)
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, _ int, request obj) {
		item := textItem()
		prefix := strings.Repeat("Synthetic safe text. ", 4)
		text := prefix + secret
		item["content"] = []any{obj{"type": "output_text", "text": text, "annotations": []any{}}}
		es := contractEvents([]any{item}, "", nil)
		var split []obj
		for _, event := range es {
			if event["type"] == "response.output_text.delta" {
				first, second := clone(event), clone(event)
				first["delta"] = prefix + secret[:12]
				second["delta"] = secret[12:]
				split = append(split, first, second)
			} else {
				split = append(split, event)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sse(split))
	})
	engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, session, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.secret = []byte(secret)
	var preview strings.Builder
	ctx := assistant.WithProgress(t.Context(), func(event assistant.ProgressEvent) {
		if event.Kind == "text" {
			preview.WriteString(event.Text)
		}
	})
	result, err := engine.Complete(ctx, nil, "Synthetic request")
	if err == nil || len(result.History) != 0 || strings.Contains(preview.String(), secret[:12]) {
		t.Fatal("split credential or successful history escaped")
	}
	if preview.Len() == 0 {
		t.Fatal("missing safe provisional prefix")
	}
}
