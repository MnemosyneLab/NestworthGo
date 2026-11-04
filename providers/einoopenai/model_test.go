package einoopenai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ao "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go/v3/responses"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type obj = map[string]any

func encode(v any) string {
	b, err := jsoncodec.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
func clone(v obj) obj {
	var out obj
	decoder := jsoncodec.NewDecoder(strings.NewReader(encode(v)))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil {
		panic("fixture")
	}
	return out
}
func callItem() obj {
	return obj{"type": "function_call", "id": "fc_0", "call_id": "call_0", "name": "mutate", "arguments": `{"amount":"12.34"}`, "status": "completed"}
}
func textItem() obj {
	return obj{"type": "message", "id": "msg_0", "role": "assistant", "status": "completed", "content": []any{obj{"type": "output_text", "text": "Synthetic complete", "annotations": []any{}}}}
}
func response(items []any) obj {
	return obj{"object": "response", "id": "resp_0", "model": "mock", "status": "completed", "output": items, "error": nil, "incomplete_details": nil}
}
func events(items []any) []obj {
	resp := response([]any{})
	resp["status"] = "in_progress"
	out := []obj{{"type": "response.created", "response": resp}}
	for i, raw := range items {
		item := raw.(obj)
		added := clone(item)
		added["status"] = "in_progress"
		switch item["type"] {
		case "function_call":
			added["arguments"] = ""
		case "message":
			added["content"] = []any{}
		case "reasoning":
			delete(added, "encrypted_content")
		}
		out = append(out, obj{"type": "response.output_item.added", "output_index": i, "item": added})
		if item["type"] == "function_call" {
			out = append(out, obj{"type": "response.function_call_arguments.delta", "output_index": i, "item_id": item["id"], "delta": item["arguments"]})
		}
		out = append(out, obj{"type": "response.output_item.done", "output_index": i, "item": item})
	}
	out = append(out, obj{"type": "response.completed", "response": response(items)})
	return out
}
func sse(es []obj) string {
	var b strings.Builder
	for i, e := range es {
		e = clone(e)
		if _, ok := e["sequence_number"]; !ok {
			e["sequence_number"] = i
		}
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", e["type"], encode(e))
	}
	return b.String()
}

type localOnly struct {
	host  string
	inner http.RoundTripper
	calls atomic.Int32
}

func (l *localOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != l.host {
		return nil, errors.New("external request blocked")
	}
	l.calls.Add(1)
	return l.inner.RoundTrip(r)
}

type fake struct {
	model    *Model
	mu       sync.Mutex
	requests []obj
	calls    *localOnly
}

func fakeModel(t *testing.T, limits Limits, handler func(http.ResponseWriter, *http.Request, int, obj)) *fake {
	t.Helper()
	f := &fake{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var req obj
		decoder := jsoncodec.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&req) != nil {
			t.Error("invalid request")
			return
		}
		f.mu.Lock()
		n := len(f.requests)
		f.requests = append(f.requests, req)
		f.mu.Unlock()
		if r.URL.Path != "/responses" || req["store"] != false || req["previous_response_id"] != nil || req["truncation"] != "disabled" || encode(req["include"]) != `["reasoning.encrypted_content"]` {
			t.Error("unsafe request configuration")
		}
		handler(w, r, n, req)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	tr := &http.Transport{Proxy: nil}
	t.Cleanup(tr.CloseIdleConnections)
	f.calls = &localOnly{host: u.Host, inner: tr}
	var errNew error
	f.model, errNew = newModel(context.Background(), Config{BaseURL: server.URL, APIKey: "synthetic-canary-key", Model: "mock", Limits: limits, Transport: f.calls}, true)
	if errNew != nil {
		t.Fatal(errNew)
	}
	return f
}

// No approval wrapper: the counter isolates completion gating in the real runner.
type mutation struct{ count atomic.Int32 }

func (m *mutation) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "mutate", Desc: "Synthetic mutation counter", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{"amount": {Type: schema.String, Required: true}})}, nil
}
func (m *mutation) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var in struct {
		Amount string `json:"amount"`
	}
	if jsoncodec.Unmarshal([]byte(args), &in) != nil || in.Amount != "12.34" {
		return "", errors.New("unexpected synthetic mutation")
	}
	m.count.Add(1)
	return `{"ok":true}`, nil
}
func run(t *testing.T, ctx context.Context, m model.AgenticModel, streaming bool) (int32, []error) {
	t.Helper()
	mut := &mutation{}
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{Name: "ProviderAcceptance", Model: m, MaxIterations: 4, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{mut}}}})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: streaming})
	it := runner.Query(ctx, "Use the synthetic tool")
	var errs []error
	for {
		event, ok := it.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			errs = append(errs, event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			if _, err := event.Output.MessageOutput.GetMessage(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return mut.count.Load(), errs
}
func writeMode(w http.ResponseWriter, req obj, items []any) {
	if req["stream"] == true {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sse(events(items)))
	} else {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, encode(response(items)))
	}
}

func TestRunnerStreamCompletionGate(t *testing.T) {
	tests := []struct {
		name    string
		change  func([]obj) string
		success bool
	}{
		{"complete", func(es []obj) string { return sse(es) }, true},
		{"eof-before-item-done", func(es []obj) string { return sse(es[:3]) }, false},
		{"eof-before-response-completed", func(es []obj) string { return sse(es[:4]) }, false},
		{"failed", func(es []obj) string {
			es[4] = obj{"type": "response.failed", "response": response([]any{callItem()})}
			return sse(es)
		}, false},
		{"incomplete", func(es []obj) string { es[4]["type"] = "response.incomplete"; return sse(es) }, false},
		{"cancelled", func(es []obj) string { es[4]["type"] = "response.cancelled"; return sse(es) }, false},
		{"provider-error", func(es []obj) string { es[4] = obj{"type": "error", "message": "synthetic-canary-key"}; return sse(es) }, false},
		{"malformed-json", func(es []obj) string { return sse(es[:4]) + "data: {broken\n\n" }, false},
		{"partial-arguments", func(es []obj) string { es[2]["delta"] = `{"amount":`; return sse(es) }, false},
		{"argument-mismatch", func(es []obj) string { es[2]["delta"] = `{"amount":"999"}`; return sse(es) }, false},
		{"terminal-mismatch", func(es []obj) string { es[4]["response"] = response([]any{textItem()}); return sse(es) }, false},
		{"failed-terminal-status", func(es []obj) string { es[4]["response"].(obj)["status"] = "failed"; return sse(es) }, false},
		{"failed-item-status", func(es []obj) string { es[3]["item"].(obj)["status"] = "incomplete"; return sse(es) }, false},
		{"changed-response-id", func(es []obj) string { es[4]["response"].(obj)["id"] = "resp_other"; return sse(es) }, false},
		{"changed-item-id", func(es []obj) string {
			es[3]["item"] = clone(es[3]["item"].(obj))
			es[3]["item"].(obj)["id"] = "other"
			return sse(es)
		}, false},
		{"changed-call-id", func(es []obj) string {
			es[3]["item"] = clone(es[3]["item"].(obj))
			es[3]["item"].(obj)["call_id"] = "other"
			return sse(es)
		}, false},
		{"wrong-delta-item", func(es []obj) string { es[2]["item_id"] = "other"; return sse(es) }, false},
		{"negative-index", func(es []obj) string { es[2]["output_index"] = -1; return sse(es) }, false},
		{"fractional-index", func(es []obj) string { es[2]["output_index"] = 0.5; return sse(es) }, false},
		{"duplicate-sequence", func(es []obj) string { es[2]["sequence_number"] = 1; return sse(es) }, false},
		{"missing-sequence", func(es []obj) string { return strings.Replace(sse(es), `"sequence_number":2,`, "", 1) }, false},
		{"duplicate-item-done", func(es []obj) string { return sse(append(es[:4], append([]obj{es[3]}, es[4:]...)...)) }, false},
		{"missing-created", func(es []obj) string { return sse(es[1:]) }, false},
		{"failure-after-complete", func(es []obj) string { return sse(append(es, obj{"type": "response.failed"})) }, false},
		{"trailing-garbage", func(es []obj) string { return sse(es) + "garbage" }, false},
		{"trailing-malformed-json", func(es []obj) string { return sse(es) + "data: {bad\n\n" }, false},
		{"incomplete-final-frame", func(es []obj) string { return strings.TrimSuffix(sse(es), "\n") }, false},
		{"early-done-sentinel", func(es []obj) string { return sse(es[:3]) + "data: [DONE]\n\n" }, false},
		{"duplicate-done-sentinel", func(es []obj) string { return sse(es) + "data: [DONE]\n\ndata: [DONE]\n\n" }, false},
		{"event-type-mismatch", func(es []obj) string {
			return strings.Replace(sse(es), "event: response.completed", "event: response.failed", 1)
		}, false},
		{"unsupported-event", func(es []obj) string { es[2]["type"] = "response.unknown"; return sse(es) }, false},
		{"invalid-utf8", func(es []obj) string { return sse(es) + "data: \xff\n\n" }, false},
		{"duplicate-json-key", func(es []obj) string {
			return strings.Replace(sse(es), `"status":"completed"`, `"status":"failed","st\u0061tus":"completed"`, 1)
		}, false},
		{"complete-with-sentinel-comments-crlf", func(es []obj) string {
			return strings.ReplaceAll(": heartbeat\n\n"+sse(es)+"data: [DONE]\n\n: eof\n\n", "\n", "\r\n")
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n > 0 {
					writeMode(w, req, []any{textItem()})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.change(events([]any{callItem()})))
			})
			count, errs := run(t, context.Background(), f.model, true)
			if tc.success {
				if count != 1 || len(errs) != 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
			} else if count != 0 || len(errs) == 0 || f.calls.calls.Load() != 1 {
				t.Fatalf("count=%d errors=%v requests=%d", count, errs, f.calls.calls.Load())
			}
			for _, err := range errs {
				if strings.Contains(err.Error(), "synthetic-canary-key") {
					t.Fatal("provider secret leaked")
				}
			}
		})
	}
}

func TestRunnerNonstreamCompletionGate(t *testing.T) {
	cases := []struct {
		name    string
		change  func(obj) string
		success bool
	}{
		{"complete", func(r obj) string { return encode(r) }, true},
		{"failed", func(r obj) string { r["status"] = "failed"; return encode(r) }, false},
		{"incomplete", func(r obj) string { r["status"] = "incomplete"; return encode(r) }, false},
		{"cancelled", func(r obj) string { r["status"] = "cancelled"; return encode(r) }, false},
		{"missing-status", func(r obj) string { delete(r, "status"); return encode(r) }, false},
		{"embedded-error", func(r obj) string { r["error"] = obj{"code": "synthetic"}; return encode(r) }, false},
		{"incomplete-details", func(r obj) string { r["incomplete_details"] = obj{"reason": "max_output_tokens"}; return encode(r) }, false},
		{"failed-item", func(r obj) string { r["output"].([]any)[0].(obj)["status"] = "in_progress"; return encode(r) }, false},
		{"duplicate-call-id", func(r obj) string {
			item := callItem()
			item["id"] = "fc_other"
			r["output"] = append(r["output"].([]any), item)
			return encode(r)
		}, false},
		{"duplicate-item-id", func(r obj) string { r["output"] = []any{callItem(), callItem()}; return encode(r) }, false},
		{"array-arguments", func(r obj) string { r["output"].([]any)[0].(obj)["arguments"] = "[]"; return encode(r) }, false},
		{"malformed-arguments", func(r obj) string { r["output"].([]any)[0].(obj)["arguments"] = "{"; return encode(r) }, false},
		{"duplicate-arguments", func(r obj) string {
			r["output"].([]any)[0].(obj)["arguments"] = `{"amount":"1","amount":"12.34"}`
			return encode(r)
		}, false},
		{"null-output", func(r obj) string { r["output"] = nil; return encode(r) }, false},
		{"unsupported-tool", func(r obj) string { r["output"].([]any)[0].(obj)["type"] = "mcp_call"; return encode(r) }, false},
		{"truncated-json", func(r obj) string { return strings.TrimSuffix(encode(r), "}") }, false},
		{"trailing-json", func(r obj) string { return encode(r) + "{}" }, false},
		{"trailing-garbage", func(r obj) string { return encode(r) + "bad" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n > 0 {
					writeMode(w, req, []any{textItem()})
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.change(response([]any{callItem()})))
			})
			count, errs := run(t, context.Background(), f.model, false)
			if tc.success {
				if count != 1 || len(errs) != 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
			} else if count != 0 || len(errs) == 0 || f.calls.calls.Load() != 1 {
				t.Fatalf("count=%d errors=%v requests=%d", count, errs, f.calls.calls.Load())
			}
		})
	}
}

func reasoningItem(id string) obj {
	return obj{"type": "reasoning", "id": id, "status": "completed", "encrypted_content": "opaque_" + id,
		"summary":             []any{obj{"type": "summary_text", "text": "first"}, obj{"type": "summary_text", "text": "second"}},
		"content":             []any{obj{"type": "reasoning_text", "text": "INTERNAL_SYNTHETIC_REASONING"}},
		"future_native_field": obj{"exact": json.Number("9007199254740993"), "parts": []any{"a", "b"}}}
}
func TestNativeReasoningAcrossActualRunnerRounds(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n == 2 {
					writeMode(w, req, []any{textItem()})
					return
				}
				reason := reasoningItem(fmt.Sprintf("rs_%d", n))
				call := callItem()
				call["id"] = fmt.Sprintf("fc_%d", n)
				call["call_id"] = fmt.Sprintf("call_%d", n)
				writeMode(w, req, []any{reason, call})
			})
			count, errs := run(t, context.Background(), f.model, stream)
			if count != 2 || len(errs) != 0 || len(f.requests) != 3 {
				t.Fatalf("count=%d requests=%d errors=%v", count, len(f.requests), errs)
			}
			for n := 1; n < 3; n++ {
				input := f.requests[n]["input"].([]any)
				var got []obj
				for _, entry := range input {
					item := entry.(map[string]any)
					if item["type"] == "reasoning" {
						got = append(got, item)
					}
				}
				if len(got) != n {
					t.Fatalf("round=%d reasoning count=%d", n, len(got))
				}
				for i, item := range got {
					if !same([]byte(encode(item)), []byte(encode(reasoningItem(fmt.Sprintf("rs_%d", i))))) {
						t.Fatalf("native reasoning changed: round=%d item=%d", n, i)
					}
				}
			}
		})
	}
}
func TestReasoningCheckpointRoundTripAndVisibleText(t *testing.T) {
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
		if n == 0 {
			writeMode(w, req, []any{reasoningItem("rs_0"), reasoningItem("rs_1"), textItem()})
			return
		}
		writeMode(w, req, []any{textItem()})
	})
	input := []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")}
	sr, err := f.model.Stream(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := sr.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sr.Recv(); err != io.EOF {
		t.Fatal("expected one committed message")
	}
	sr.Close()
	var visible string
	for _, block := range msg.ContentBlocks {
		if block.AssistantGenText != nil {
			visible += block.AssistantGenText.Text
		}
	}
	if visible != "Synthetic complete" || strings.Contains(visible, "INTERNAL") || strings.Contains(visible, "opaque") {
		t.Fatal("reasoning exposed as visible text")
	}
	encoded := encode(msg)
	var restored schema.AgenticMessage
	if jsoncodec.Unmarshal([]byte(encoded), &restored) != nil {
		t.Fatal("checkpoint decode")
	}
	if _, err := f.model.Generate(context.Background(), append(input, &restored)); err != nil {
		t.Fatal(err)
	}
	var got []obj
	for _, entry := range f.requests[1]["input"].([]any) {
		item := entry.(map[string]any)
		if item["type"] == "reasoning" {
			got = append(got, item)
		}
	}
	if len(got) != 2 || !same([]byte(encode(got[0])), []byte(encode(reasoningItem("rs_0")))) || !same([]byte(encode(got[1])), []byte(encode(reasoningItem("rs_1")))) {
		t.Fatal("checkpoint lost native reasoning identity, fields or order")
	}
	// If a future state serializer drops the sidecar, refuse a lossy continuation.
	delete(restored.Extra, nativeKey)
	if _, err := f.model.Generate(context.Background(), append(input, &restored)); err == nil {
		t.Fatal("accepted reasoning without native state")
	}
	if len(f.requests) != 2 {
		t.Fatal("lossy history reached provider")
	}
	restored.Extra[nativeKey] = msg.Extra[nativeKey]
	restored.ContentBlocks[0], restored.ContentBlocks[1] = restored.ContentBlocks[1], restored.ContentBlocks[0]
	if _, err := f.model.Generate(context.Background(), append(input, &restored)); err == nil {
		t.Fatal("accepted reordered native reasoning")
	}
	if len(f.requests) != 2 {
		t.Fatal("reordered reasoning reached provider")
	}
}

func TestBoundsCancellationAndEOF(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, kind := range []string{"bytes", "events", "items", "arguments", "timeout-after-completion", "cancel-after-arguments", "cancel-before-request", "request-bytes"} {
			if !stream && kind == "events" {
				continue
			}
			t.Run(fmt.Sprintf("stream=%v/%s", stream, kind), func(t *testing.T) {
				limits := Limits{}
				switch kind {
				case "bytes":
					limits.Bytes = 2048
				case "events":
					limits.Events = 3
				case "items":
					limits.Items = 1
				case "timeout-after-completion":
					limits.Timeout = 30 * time.Millisecond
				case "request-bytes":
					limits.Bytes = 32
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := fakeModel(t, limits, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
					items := []any{callItem()}
					switch kind {
					case "bytes":
						items[0].(obj)["padding"] = strings.Repeat("x", 3000)
					case "items":
						items = append(items, textItem())
					case "arguments":
						items[0].(obj)["arguments"] = `{"amount":"` + strings.Repeat("x", 65<<10) + `"}`
					case "cancel-after-arguments":
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, sse(events(items)[:3]))
							w.(http.Flusher).Flush()
							cancel()
							<-r.Context().Done()
							return
						}
						cancel()
					}
					writeMode(w, req, items)
					if kind == "timeout-after-completion" {
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}
				})
				if kind == "cancel-before-request" {
					cancel()
				}
				count, errs := run(t, ctx, f.model, stream)
				if count != 0 || len(errs) == 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
				expected := int32(1)
				if kind == "cancel-before-request" || kind == "request-bytes" {
					expected = 0
				}
				if f.calls.calls.Load() != expected {
					t.Fatalf("wire calls=%d want=%d", f.calls.calls.Load(), expected)
				}
			})
		}
	}
}
func TestHTTPPolicyNoRetriesRedirectOrBodyLeaks(t *testing.T) {
	for _, kind := range []string{"http-500", "redirect", "wrong-content-type", "io-error-after-complete"} {
		t.Run(kind, func(t *testing.T) {
			var destination atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destination.Add(1) }))
			defer target.Close()
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				switch kind {
				case "http-500":
					http.Error(w, "synthetic-canary-key", 500)
				case "redirect":
					http.Redirect(w, r, target.URL, 307)
				case "wrong-content-type":
					fmt.Fprint(w, sse(events([]any{callItem()})))
				case "io-error-after-complete":
					w.Header().Set("Content-Type", "text/event-stream")
					raw := sse(events([]any{callItem()}))
					w.Header().Set("Content-Length", fmt.Sprint(len(raw)+1))
					fmt.Fprint(w, raw)
				}
			})
			count, errs := run(t, context.Background(), f.model, true)
			if count != 0 || len(errs) == 0 || f.calls.calls.Load() != 1 || destination.Load() != 0 {
				t.Fatalf("count=%d errors=%v calls=%d redirect=%d", count, errs, f.calls.calls.Load(), destination.Load())
			}
			for _, err := range errs {
				if strings.Contains(err.Error(), "synthetic-canary-key") {
					t.Fatal("secret leaked")
				}
			}
		})
	}
}
func TestRequestOptionsCannotWeakenBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		option model.Option
		reject bool
	}{
		{"store-and-truncation", ao.WithExtraFields(obj{"store": true, "include": []any{}, "stream": false, "truncation": "auto"}), false},
		{"previous-response", ao.WithExtraFields(obj{"previous_response_id": "resp_old"}), true},
		{"background", ao.WithExtraFields(obj{"background": true}), true},
		{"remote-tools", ao.WithExtraFields(obj{"tools": []any{obj{"type": "mcp", "server_url": "https://invalid.test"}}}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) { writeMode(w, req, []any{textItem()}) })
			sr, err := f.model.Stream(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")}, tc.option)
			if (err != nil) != tc.reject {
				t.Fatalf("err=%v", err)
			}
			if sr != nil {
				sr.Close()
			}
			expected := int32(1)
			if tc.reject {
				expected = 0
			}
			if f.calls.calls.Load() != expected {
				t.Fatal("unsafe options reached wire")
			}
		})
	}
}
func TestConstructorAndStrictJSON(t *testing.T) {
	for _, base := range []string{"http://api.openai.com/v1", "https://user:pass@invalid.test/v1", "https://invalid.test/v1?key=x", "https://invalid.test/v1#x"} {
		if _, err := New(context.Background(), Config{BaseURL: base, APIKey: "fake", Model: "mock"}); err == nil {
			t.Fatal("accepted unsafe URL")
		}
	}
	for _, limits := range []Limits{{Bytes: -1}, {Bytes: 4<<20 + 1}, {Events: 4097}, {Items: 129}, {Timeout: 61 * time.Second}} {
		if _, err := New(context.Background(), Config{APIKey: "fake", Model: "mock", Limits: limits}); err == nil {
			t.Fatal("accepted unbounded limits")
		}
	}
	for _, raw := range []string{`{"a":1,"\u0061":2}`, `{"nested":{"x":1,"x":2}}`, strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66), "{}{}", "\xff", "{\"x\":\"\x01\"}"} {
		if strictJSON([]byte(raw)) == nil {
			t.Fatal("accepted ambiguous/malformed JSON")
		}
	}
	if strictJSON([]byte(`{"a":[1e99999999999999999999999,"escaped\\\"string",{},[]]}`)) != nil {
		t.Fatal("numeric lexeme or escaped string rejected")
	}
}

func TestPinnedUpstreamDefectsRemainReproduced(t *testing.T) {
	for _, kind := range []string{"eof-before-done", "eof-before-completed", "failed"} {
		t.Run(kind, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n > 0 {
					writeMode(w, req, []any{textItem()})
					return
				}
				es := events([]any{callItem()})
				switch kind {
				case "eof-before-done":
					es = es[:3]
				case "eof-before-completed":
					es = es[:4]
				case "failed":
					es[4]["type"] = "response.failed"
					es[4]["response"].(obj)["status"] = "failed"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sse(es))
			})
			store, retries := false, 0
			upstream, err := ao.NewResponsesModel(context.Background(), &ao.ResponsesConfig{APIKey: "synthetic-canary-key", BaseURL: "http://" + f.calls.host, Model: "mock", Store: &store, MaxRetries: &retries, Include: []responses.ResponseIncludable{"reasoning.encrypted_content"}, ExtraFields: obj{"truncation": "disabled"}, HTTPClient: &http.Client{Transport: f.calls, Timeout: time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			count, _ := run(t, context.Background(), upstream, true)
			if count != 1 {
				t.Fatalf("upstream behavior changed: count=%d; re-evaluate pinned adapter", count)
			}
			t.Log("unmodified upstream dispatches once; corrected adapter acceptance above requires zero")
		})
	}
	t.Run("done-only-reasoning", func(t *testing.T) {
		f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
			writeMode(w, req, []any{reasoningItem("rs_0"), textItem()})
		})
		store, retries := false, 0
		upstream, err := ao.NewResponsesModel(context.Background(), &ao.ResponsesConfig{APIKey: "synthetic-canary-key", BaseURL: "http://" + f.calls.host, Model: "mock", Store: &store, MaxRetries: &retries, Include: []responses.ResponseIncludable{"reasoning.encrypted_content"}, ExtraFields: obj{"truncation": "disabled"}, HTTPClient: &http.Client{Transport: f.calls, Timeout: time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		sr, err := upstream.Stream(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")})
		if err != nil {
			t.Fatal(err)
		}
		defer sr.Close()
		var chunks []*schema.AgenticMessage
		for {
			chunk, err := sr.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			chunks = append(chunks, chunk)
		}
		msg, err := schema.ConcatAgenticMessages(chunks)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, block := range msg.ContentBlocks {
			if block.Reasoning != nil {
				found = true
				if block.Reasoning.Signature != "" {
					t.Fatal("upstream behavior changed: reassess characterization")
				}
			}
		}
		if !found {
			t.Fatal("reasoning item itself missing")
		}
	})
}

func withTextEvents() []obj {
	msg := textItem()
	items := []any{callItem(), msg}
	es := events(items)
	// Events for the text item are inserted after its added event and before done.
	part := obj{"type": "output_text", "text": "", "annotations": []any{}}
	full := msg["content"].([]any)[0]
	extras := []obj{
		{"type": "response.content_part.added", "output_index": 1, "item_id": "msg_0", "content_index": 0, "part": part},
		{"type": "response.output_text.delta", "output_index": 1, "item_id": "msg_0", "content_index": 0, "delta": "Synthetic complete"},
		{"type": "response.output_text.done", "output_index": 1, "item_id": "msg_0", "content_index": 0, "text": "Synthetic complete"},
		{"type": "response.content_part.done", "output_index": 1, "item_id": "msg_0", "content_index": 0, "part": full},
	}
	return append(es[:5], append(extras, es[5:]...)...)
}
func TestTextEventIntegrityBeforeToolRelease(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]obj) []obj
		ok     bool
	}{
		{"valid", func(es []obj) []obj { return es }, true},
		{"null-text-delta", func(es []obj) []obj { es[6]["delta"] = nil; return es }, false},
		{"wrong-text-done", func(es []obj) []obj { es[7]["text"] = "other"; return es }, false},
		{"missing-part", func(es []obj) []obj { delete(es[5], "part"); return es }, false},
		{"null-part-text", func(es []obj) []obj { es[5]["part"].(obj)["text"] = nil; return es }, false},
		{"duplicate-part-added", func(es []obj) []obj { return append(es[:6], append([]obj{es[5]}, es[6:]...)...) }, false},
		{"incomplete-part-done", func(es []obj) []obj { es[8]["status"] = "incomplete"; return es }, false},
		{"incomplete-text-done", func(es []obj) []obj { es[7]["status"] = "incomplete"; return es }, false},
		{"missing-part-done", func(es []obj) []obj { return append(es[:8], es[9:]...) }, false},
		{"delta-after-text-done", func(es []obj) []obj { return append(es[:8], append([]obj{es[6]}, es[8:]...)...) }, false},
		{"huge-content-index", func(es []obj) []obj { es[5]["content_index"] = 1000000000; return es }, false},
		{"part-before-added", func(es []obj) []obj { es[4], es[5] = es[5], es[4]; return es }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n > 0 {
					writeMode(w, req, []any{textItem()})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sse(tc.change(withTextEvents())))
			})
			count, errs := run(t, context.Background(), f.model, true)
			if tc.ok {
				if count != 1 || len(errs) != 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
			} else if count != 0 || len(errs) == 0 {
				t.Fatalf("count=%d errors=%v", count, errs)
			}
		})
	}
}

// The documented added snapshot is provisional, including nonempty ciphertext.
// Acceptance still requires item done, response completed and clean EOF.
func TestProvisionalReasoningUsesDoneSnapshot(t *testing.T) {
	for _, value := range []struct {
		name      string
		encrypted any
		omit      bool
	}{
		{"absent", nil, true}, {"null", nil, false}, {"partial", "unfinished_ciphertext", false}, {"already-complete", "opaque_rs_0", false},
	} {
		for _, mode := range []string{"complete", "eof-before-done", "eof-before-completed", "failed", "incomplete"} {
			t.Run(value.name+"/"+mode, func(t *testing.T) {
				f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
					if n > 0 {
						writeMode(w, req, []any{textItem()})
						return
					}
					es := events([]any{reasoningItem("rs_0"), callItem()})
					if value.omit {
						delete(es[1]["item"].(obj), "encrypted_content")
					} else {
						es[1]["item"].(obj)["encrypted_content"] = value.encrypted
					}
					switch mode {
					case "eof-before-done":
						es = es[:5]
					case "eof-before-completed":
						es = es[:len(es)-1]
					case "failed", "incomplete":
						es[len(es)-1]["type"] = "response." + mode
						es[len(es)-1]["response"].(obj)["status"] = mode
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, sse(es))
				})
				count, errs := run(t, context.Background(), f.model, true)
				if mode != "complete" {
					if count != 0 || len(errs) == 0 {
						t.Fatalf("count=%d errors=%v", count, errs)
					}
					return
				}
				if count != 1 || len(errs) != 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
				var got any
				for _, entry := range f.requests[1]["input"].([]any) {
					item := entry.(map[string]any)
					if item["type"] == "reasoning" {
						got = item
					}
				}
				if !same([]byte(encode(got)), []byte(encode(reasoningItem("rs_0")))) {
					t.Fatal("provisional snapshot replaced authoritative done reasoning")
				}
			})
		}
	}
}

func TestConcurrentRoundIsolation(t *testing.T) {
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
		writeMode(w, req, []any{reasoningItem("rs_0"), textItem()})
	})
	seed, err := f.model.Generate(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("seed")})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(stream bool) {
			defer wg.Done()
			in := []*schema.AgenticMessage{schema.UserAgenticMessage("seed"), seed, schema.UserAgenticMessage("synthetic")}
			var msg *schema.AgenticMessage
			var err error
			if stream {
				var sr *schema.StreamReader[*schema.AgenticMessage]
				sr, err = f.model.Stream(context.Background(), in)
				if err == nil {
					msg, err = sr.Recv()
					sr.Close()
				}
			} else {
				msg, err = f.model.Generate(context.Background(), in)
			}
			if err != nil || msg == nil || msg.Extra[nativeKey] == nil {
				t.Errorf("concurrent round failed: %v", err)
			}
		}(i%2 == 0)
	}
	wg.Wait()
	if len(f.requests) != 9 {
		t.Fatalf("requests=%d", len(f.requests))
	}
}
func FuzzProviderParsing(f *testing.F) {
	for _, seed := range []string{`{}`, `{"a":[1,"x"]}`, `{"a":1,"a":2}`, `"escaped\\\"value"`, sse(events([]any{callItem()}))} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			t.Skip()
		}
		_ = strictJSON([]byte(s))
		_, _, _ = validateStream([]byte(s), Limits{Bytes: 8192, Events: 32, Items: 16})
	})
}

func TestNullableReasoningCiphertextAndContentContract(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, kind := range []string{"null-ciphertext", "absent-ciphertext", "absent-content", "null-content", "wrong-ciphertext-type"} {
			t.Run(fmt.Sprintf("stream=%v/%s", stream, kind), func(t *testing.T) {
				item := reasoningItem("rs_0")
				switch kind {
				case "null-ciphertext":
					item["encrypted_content"] = nil
				case "absent-ciphertext":
					delete(item, "encrypted_content")
				case "absent-content":
					delete(item, "content")
				case "null-content":
					item["content"] = nil
				case "wrong-ciphertext-type":
					item["encrypted_content"] = 17
				}
				f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
					if n == 0 {
						writeMode(w, req, []any{item, callItem()})
					} else {
						writeMode(w, req, []any{textItem()})
					}
				})
				count, errs := run(t, context.Background(), f.model, stream)
				if kind == "null-content" || kind == "wrong-ciphertext-type" {
					if count != 0 || len(errs) == 0 {
						t.Fatalf("unsupported reasoning shape: count=%d errors=%v", count, errs)
					}
					return
				}
				if count != 1 || len(errs) != 0 {
					t.Fatalf("count=%d errors=%v", count, errs)
				}
				var got any
				for _, entry := range f.requests[1]["input"].([]any) {
					native := entry.(map[string]any)
					if native["type"] == "reasoning" {
						got = native
					}
				}
				if !same([]byte(encode(got)), []byte(encode(item))) {
					t.Fatal("native nullable/absent field was changed")
				}
			})
		}
	}
}
func TestNativeAssistantSemanticsAcrossAdapterRounds(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			outputs := [][]any{}
			for n, phase := range []any{"commentary", "final_answer", nil} {
				msg := textItem()
				msg["id"] = fmt.Sprintf("msg_%d", n)
				msg["phase"] = phase
				msg["content"] = []any{
					obj{"type": "output_text", "text": "First segment", "annotations": []any{}, "logprobs": []any{}},
					obj{"type": "refusal", "refusal": "Synthetic refusal"},
					obj{"type": "output_text", "text": "Last segment", "annotations": []any{}},
				}
				outputs = append(outputs, []any{reasoningItem(fmt.Sprintf("rs_%d", n)), msg})
			}
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n < len(outputs) {
					writeMode(w, req, outputs[n])
				} else {
					last := textItem()
					last["id"] = "msg_last"
					writeMode(w, req, []any{last})
				}
			})
			history := []*schema.AgenticMessage{schema.UserAgenticMessage("start")}
			for n := 0; n < 4; n++ {
				var msg *schema.AgenticMessage
				var err error
				if stream {
					var sr *schema.StreamReader[*schema.AgenticMessage]
					sr, err = f.model.Stream(context.Background(), history)
					if err == nil {
						msg, err = sr.Recv()
						sr.Close()
					}
				} else {
					msg, err = f.model.Generate(context.Background(), history)
				}
				if err != nil {
					t.Fatal(err)
				}
				// Exercise the actual serialized schema handed back to the adapter, not a stub.
				var restored schema.AgenticMessage
				if jsoncodec.Unmarshal([]byte(encode(msg)), &restored) != nil {
					t.Fatal("history serialization failed")
				}
				history = append(history, &restored, schema.UserAgenticMessage("next"))
				var got []any
				for _, entry := range f.requests[n]["input"].([]any) {
					item := entry.(map[string]any)
					if item["type"] == "reasoning" || item["role"] == "assistant" {
						got = append(got, item)
					}
				}
				var expected []any
				for i := 0; i < n; i++ {
					expected = append(expected, outputs[i]...)
				}
				if !same([]byte(encode(got)), []byte(encode(expected))) {
					t.Fatalf("round %d changed native message phase, refusal, part grouping or order", n)
				}
				if strings.Contains(encode(f.requests[n]["input"]), "nestworth_native_") {
					t.Fatal("private replay marker reached provider")
				}
			}
		})
	}
}

func TestChangedNativeHistoryIsRefusedBeforeNetwork(t *testing.T) {
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
		msg := textItem()
		msg["phase"] = "commentary"
		writeMode(w, req, []any{msg})
	})
	first, err := f.model.Generate(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("start")})
	if err != nil {
		t.Fatal(err)
	}
	var state nativeState
	if jsoncodec.Unmarshal([]byte(first.Extra[nativeKey].(string)), &state) != nil {
		t.Fatal("native state decode")
	}
	item, err := object(state.Output[0])
	if err != nil {
		t.Fatal(err)
	}
	item["phase"] = json.RawMessage(`"final_answer"`)
	state.Output[0] = json.RawMessage(encode(item))
	first.Extra[nativeKey] = encode(state)
	if _, err := f.model.Generate(context.Background(), []*schema.AgenticMessage{first, schema.UserAgenticMessage("next")}); err == nil {
		t.Fatal("changed native phase accepted with stale digest")
	}
	if f.calls.calls.Load() != 1 {
		t.Fatal("changed native state reached provider")
	}
}
