package einoopenai

import (
	"context"
	"errors"
	"fmt"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Unlike events(), this emits full reasoning/message part and text lifecycles,
// argument.done, explicit response progress and realistically optional function
// status. All IDs, text, encrypted content and ledger facts are synthetic.
func contractEvents(items []any, seed string, addedStatus any) []obj {
	progress := response([]any{})
	progress["status"] = "in_progress"
	es := []obj{{"type": "response.created", "response": progress}, {"type": "response.in_progress", "response": clone(progress)}}
	for i, raw := range items {
		item := raw.(obj)
		added := clone(item)
		switch item["type"] {
		case "function_call":
			added["arguments"] = seed
			if addedStatus == nil {
				delete(added, "status")
			} else {
				added["status"] = addedStatus
			}
		case "message":
			added["status"] = "in_progress"
			added["content"] = []any{}
		case "reasoning":
			delete(added, "status")
			added["summary"] = []any{}
			added["content"] = []any{}
			added["encrypted_content"] = "provisional-synthetic"
		}
		es = append(es, obj{"type": "response.output_item.added", "output_index": i, "item": added})
		event := func(typ string, fields obj) {
			fields["type"] = typ
			fields["output_index"] = i
			fields["item_id"] = item["id"]
			es = append(es, fields)
		}
		if item["type"] == "function_call" {
			args := item["arguments"].(string)
			if !strings.HasPrefix(args, seed) {
				panic("invalid synthetic seed")
			}
			for _, c := range args[len(seed):] {
				event("response.function_call_arguments.delta", obj{"delta": string(c)})
			}
			event("response.function_call_arguments.done", obj{"arguments": args, "name": item["name"]})
		} else {
			for _, group := range []string{"summary", "content"} {
				parts, _ := item[group].([]any)
				for idx, rawPart := range parts {
					part := rawPart.(obj)
					kind := part["type"].(string)
					addedPart := clone(part)
					field := "text"
					if kind == "refusal" {
						field = "refusal"
					}
					addedPart[field] = ""
					partPrefix, indexKey, textPrefix := "response.content_part.", "content_index", "response."+kind+"."
					if group == "summary" {
						partPrefix, indexKey, textPrefix = "response.reasoning_summary_part.", "summary_index", "response.reasoning_summary_text."
					}
					event(partPrefix+"added", obj{indexKey: idx, "part": addedPart})
					for _, chunk := range []string{part[field].(string)} {
						event(textPrefix+"delta", obj{indexKey: idx, "delta": chunk})
					}
					event(textPrefix+"done", obj{indexKey: idx, field: part[field]})
					event(partPrefix+"done", obj{indexKey: idx, "part": part})
				}
			}
		}
		es = append(es, obj{"type": "response.output_item.done", "output_index": i, "item": item})
	}
	return append(es, obj{"type": "response.completed", "response": response(items)})
}

func contractCall(id string) obj {
	return obj{"type": "function_call", "id": "fc_" + id, "call_id": "call_" + id, "name": "get_financial_context", "arguments": `{}`}
}
func contractMessage(id string) obj {
	m := textItem()
	m["id"] = "msg_" + id
	m["phase"] = "final_answer"
	return m
}
func contractReason(id string) obj {
	return obj{"type": "reasoning", "id": "rs_" + id, "summary": []any{obj{"type": "summary_text", "text": "Synthetic summary"}}, "content": []any{obj{"type": "reasoning_text", "text": "Synthetic private reasoning"}}, "encrypted_content": "synthetic-cipher-" + id}
}
func writeContract(w http.ResponseWriter, request obj, items []any, seed string, status any) {
	if request["stream"] != true {
		writeMode(w, request, items)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, sse(contractEvents(items, seed, status)))
}

// Test adapter exercises exactly the production orchestration with the public
// Stream contract; production itself always uses Model.Generate.
type streamCompletionModel struct{ *Model }

func (m streamCompletionModel) Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	stream, err := m.Model.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	msg, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	_, tail := stream.Recv()
	if tail != io.EOF {
		return nil, assistant.ErrProviderResponse
	}
	return msg, nil
}
func completionMode(m *Model, stream bool) model.AgenticModel {
	if stream {
		return streamCompletionModel{m}
	}
	return m
}

func TestFullStreamContractThreeUserTurns(t *testing.T) {
	for _, wireStream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", wireStream), func(t *testing.T) {
			for _, tc := range []struct {
				name, seed string
				status     any
			}{{"optional_status", "", nil}, {"seeded_arguments", "{", "in_progress"}, {"completed_snapshot", "{}", "completed"}} {
				t.Run(tc.name, func(t *testing.T) {
					_, observed, _ := ledgerSession(t)
					var native []any
					fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
						input := request["input"].([]any)
						var replay []any
						users := 0
						outputs := 0
						for _, v := range input {
							item := v.(map[string]any)
							if item["role"] == "user" {
								users++
							}
							if item["type"] == "function_call_output" {
								outputs++
								if !strings.Contains(encode(item), "contentHash") {
									t.Error("controlled result missing")
								}
							}
							if item["type"] == "reasoning" || item["role"] == "assistant" || item["type"] == "function_call" {
								replay = append(replay, item)
							}
						}
						if encode(replay) != encode(native) {
							t.Error("native replay changed output groups")
						}
						expectedUsers := 1
						if n >= 1 {
							expectedUsers = 2
						}
						if n >= 3 {
							expectedUsers = 3
						}
						if users != expectedUsers {
							t.Errorf("user messages=%d want %d", users, expectedUsers)
						}
						expectedOutputs := 0
						if n >= 2 {
							expectedOutputs = 1
						}
						if n >= 4 {
							expectedOutputs = 2
						}
						if outputs != expectedOutputs {
							t.Error("tool output order/count lost")
						}
						id := fmt.Sprint(n)
						items := []any{contractReason(id), contractMessage(id)}
						if n == 1 || n == 3 {
							items = []any{contractReason(id), contractCall(id)}
						}
						native = append(native, items...)
						writeContract(w, request, items, tc.seed, tc.status)
					})
					engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, completionMode(fake.model, wireStream), builtinai.New(observed, assistant.PageSelection{}), nil)
					if err != nil {
						t.Fatal(err)
					}
					defer engine.Close()
					var history []byte
					for _, prompt := range []string{"你好", "看看资产情况", "再看看现在的资产"} {
						result, err := engine.Complete(t.Context(), history, prompt)
						if err != nil {
							t.Fatal(err)
						}
						// Force the exact durable JSON checkpoint encoding/decoding between turns.
						var saved checkpoint
						if jsoncodec.Unmarshal(result.History, &saved) != nil {
							t.Fatal("checkpoint decode")
						}
						history, err = jsoncodec.Marshal(saved)
						if err != nil {
							t.Fatal(err)
						}
					}
					if len(fake.requests) != 5 {
						t.Fatalf("requests=%d", len(fake.requests))
					}
				})
			}
		})
	}
}

func TestGenerateEngineCompletionGate(t *testing.T) {
	for _, mode := range []string{"valid", "function_incomplete", "function_in_progress", "function_null_status", "function_numeric_status", "response_incomplete", "response_failed", "partial_json", "trailing_json", "cancel_after_completed"} {
		t.Run(mode, func(t *testing.T) {
			_, observed, accountID := ledgerSession(t)
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if request["stream"] != false {
					t.Error("production engine requested SSE")
				}
				if n > 0 {
					writeMode(w, request, []any{contractMessage("final")})
					return
				}
				call := contractCall("gate")
				call["name"] = "get_account_snapshot"
				call["arguments"] = encode(obj{"id": accountID})
				resp := response([]any{call})
				switch mode {
				case "function_incomplete":
					call["status"] = "incomplete"
				case "function_in_progress":
					call["status"] = "in_progress"
				case "function_null_status":
					call["status"] = nil
				case "function_numeric_status":
					call["status"] = 1
				case "response_incomplete":
					resp["status"] = "incomplete"
				case "response_failed":
					resp["status"] = "failed"
				}
				wire := encode(resp)
				if mode == "partial_json" {
					wire = wire[:len(wire)-1]
				}
				if mode == "trailing_json" {
					wire += " {}"
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, wire)
				if mode == "cancel_after_completed" {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			})
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, builtinai.New(observed, assistant.PageSelection{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			ctx := t.Context()
			if mode == "cancel_after_completed" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			result, err := engine.Complete(ctx, nil, "Synthetic current context")
			observed.mu.Lock()
			reads := observed.reads
			observed.mu.Unlock()
			if mode == "valid" {
				if err != nil || reads != 1 || len(result.History) == 0 {
					t.Fatalf("valid JSON rejected: %v", err)
				}
			} else if err == nil || reads != 0 || len(result.History) != 0 {
				t.Fatalf("unsafe release: reads=%d err=%v", reads, err)
			}
		})
	}
}

type cancelOnEncode struct{ cancel context.CancelFunc }

func (c cancelOnEncode) MarshalJSON() ([]byte, error) { c.cancel(); return []byte("null"), nil }

type cancelBeforeDispatchModel struct {
	*Model
	cancel context.CancelFunc
}

func (m cancelBeforeDispatchModel) Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	msg, err := m.Model.Generate(ctx, in, opts...)
	if err == nil {
		msg.Extra["synthetic_cancel_hook"] = cancelOnEncode{m.cancel}
	}
	return msg, err
}
func TestCancellationAfterResponseValidationBeforeToolDispatch(t *testing.T) {
	_, observed, accountID := ledgerSession(t)
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
		call := contractCall("cancel")
		call["name"] = "get_account_snapshot"
		call["arguments"] = encode(obj{"id": accountID})
		writeMode(w, request, []any{call})
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	engine, err := engineWithModel(ctx, assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, cancelBeforeDispatchModel{fake.model, cancel}, builtinai.New(observed, assistant.PageSelection{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	result, err := engine.Complete(ctx, nil, "Synthetic read")
	observed.mu.Lock()
	defer observed.mu.Unlock()
	if !errors.Is(err, context.Canceled) || observed.reads != 0 || len(result.History) != 0 {
		t.Fatalf("cancel window misclassified or dispatched: %v", err)
	}
}

func eventOf(es []obj, typ string) obj {
	for _, e := range es {
		if e["type"] == typ {
			return e
		}
	}
	panic("synthetic event absent")
}
func removeEvent(es []obj, typ string) []obj {
	for i, e := range es {
		if e["type"] == typ {
			return append(es[:i], es[i+1:]...)
		}
	}
	panic("synthetic event absent")
}
func TestFullSSEContractAdversarialGate(t *testing.T) {
	for _, mode := range []string{"valid", "initial_prefix", "initial_complete", "empty_initial_identity", "added_incomplete", "added_null_status", "added_bad_status_type", "final_incomplete", "final_in_progress", "final_null_status", "final_bad_status_type", "changed_name", "changed_call_id", "argument_done_name_conflict", "null_initial_name", "null_initial_arguments", "bad_initial_prefix", "argument_done_mismatch", "summary_done_mismatch", "reasoning_done_mismatch", "text_done_mismatch", "refusal_done_mismatch", "missing_part_done", "part_identity_mismatch", "unknown_event", "missing_completed", "terminal_mismatch", "trailing_data"} {
		t.Run(mode, func(t *testing.T) {
			call := callItem()
			delete(call, "status")
			msg := contractMessage("matrix")
			refusal := obj{"type": "refusal", "refusal": "Synthetic refusal"}
			msg["content"] = append(msg["content"].([]any), refusal)
			seed := ""
			var status any
			switch mode {
			case "initial_prefix":
				seed = `{"amount":`
				status = "in_progress"
			case "initial_complete":
				seed = call["arguments"].(string)
				status = "completed"
			}
			es := contractEvents([]any{contractReason("matrix"), call, msg}, seed, status)
			// There is a reasoning item first; find the function's provisional snapshot.
			var added obj
			for _, e := range es {
				if e["type"] == "response.output_item.added" && e["item"].(obj)["type"] == "function_call" {
					added = e["item"].(obj)
				}
			}
			switch mode {
			case "empty_initial_identity":
				added["name"] = ""
				added["call_id"] = ""
			case "added_incomplete":
				added["status"] = "incomplete"
			case "added_null_status":
				added["status"] = nil
			case "added_bad_status_type":
				added["status"] = 1
			case "final_incomplete":
				call["status"] = "incomplete"
			case "final_in_progress":
				call["status"] = "in_progress"
			case "final_null_status":
				call["status"] = nil
			case "final_bad_status_type":
				call["status"] = 1
			case "changed_name":
				added["name"] = "different"
			case "changed_call_id":
				added["call_id"] = "different"
			case "argument_done_name_conflict":
				eventOf(es, "response.function_call_arguments.done")["name"] = "different"
			case "null_initial_name":
				added["name"] = nil
			case "null_initial_arguments":
				added["arguments"] = nil
			case "bad_initial_prefix":
				added["arguments"] = "wrong-prefix"
			case "argument_done_mismatch":
				eventOf(es, "response.function_call_arguments.done")["arguments"] = `{}`
			case "summary_done_mismatch":
				eventOf(es, "response.reasoning_summary_text.done")["text"] = "conflict"
			case "reasoning_done_mismatch":
				eventOf(es, "response.reasoning_text.done")["text"] = "conflict"
			case "text_done_mismatch":
				eventOf(es, "response.output_text.done")["text"] = "conflict"
			case "refusal_done_mismatch":
				eventOf(es, "response.refusal.done")["refusal"] = "conflict"
			case "missing_part_done":
				es = removeEvent(es, "response.content_part.done")
			case "part_identity_mismatch":
				eventOf(es, "response.content_part.done")["item_id"] = "different"
			case "unknown_event":
				es = append(es[:2], append([]obj{{"type": "response.synthetic_unsupported"}}, es[2:]...)...)
			case "missing_completed":
				es = es[:len(es)-1]
			case "terminal_mismatch":
				terminal := clone(response([]any{contractReason("matrix"), call, msg}))
				terminal["id"] = "different"
				eventOf(es, "response.completed")["response"] = terminal
			}
			wire := sse(es)
			if mode == "trailing_data" {
				wire += "data: invalid\n\n"
			}
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if n > 0 {
					writeMode(w, request, []any{contractMessage("last")})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, wire)
			})
			count, errs := run(t, t.Context(), fake.model, true)
			valid := mode == "valid" || mode == "initial_prefix" || mode == "initial_complete" || mode == "empty_initial_identity"
			if valid {
				if count != 1 || len(errs) != 0 {
					t.Fatalf("valid contract refused: count=%d errors=%v", count, errs)
				}
			} else if count != 0 || len(errs) == 0 {
				t.Fatalf("invalid contract dispatched: count=%d errors=%v", count, errs)
			}
		})
	}
}

func TestCompleteRefusalIsDisplayableAndReplays(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			_, observed, _ := ledgerSession(t)
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if n > 0 && !strings.Contains(encode(request["input"]), "Synthetic refusal") {
					t.Error("native refusal lost")
				}
				msg := contractMessage(fmt.Sprint(n))
				msg["content"] = []any{obj{"type": "refusal", "refusal": "Synthetic refusal"}}
				writeContract(w, request, []any{msg}, "", nil)
			})
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, completionMode(fake.model, stream), builtinai.New(observed, assistant.PageSelection{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			result, err := engine.Complete(t.Context(), nil, "Synthetic question")
			if err != nil || result.Text != "Synthetic refusal" {
				t.Fatalf("completed refusal discarded: %v", err)
			}
			_, err = engine.Complete(t.Context(), result.History, "Synthetic followup")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
