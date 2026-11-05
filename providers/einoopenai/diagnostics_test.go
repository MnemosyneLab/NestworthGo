package einoopenai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
)

func TestProviderHTTPFailuresAreFixedAndNeverReflectBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{400, assistant.ErrProviderRequest}, {401, assistant.ErrProviderAuthentication}, {403, assistant.ErrProviderAuthentication}, {429, assistant.ErrProviderRateLimited}, {503, assistant.ErrProviderUnavailable}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":{"message":"synthetic-canary-key financial-canary","code":"financial-canary"}}`)
			})
			_, session, _ := ledgerSession(t)
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, builtinai.New(session, assistant.PageSelection{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			_, err = engine.Complete(context.Background(), nil, "hello")
			if !errors.Is(err, tc.want) || err.Error() != tc.want.Error() {
				t.Fatalf("unsafe failure %v", err)
			}
			if fake.calls.calls.Load() != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}

func TestFixedProtocolDiagnosticsSurviveSDKAndEngine(t *testing.T) {
	for _, tc := range []struct {
		mode, rule string
		stream     bool
	}{{"content_type", "response_headers", false}, {"body_read", "response_body", false}, {"terminal", "response_terminal", false}, {"item", "response_item", false}, {"added_status", "stream_function_status", true}, {"arguments", "stream_arguments", true}, {"part", "stream_part", true}, {"unsupported", "stream_event", true}, {"eof", "stream_eof", true}} {
		t.Run(tc.mode, func(t *testing.T) {
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if tc.stream {
					es := contractEvents([]any{contractCall("diagnostic"), contractMessage("diagnostic")}, "", nil)
					switch tc.mode {
					case "added_status":
						eventOf(es, "response.output_item.added")["item"].(obj)["status"] = "incomplete"
					case "arguments":
						eventOf(es, "response.function_call_arguments.done")["arguments"] = `{"secret":"synthetic-canary-key"}`
					case "part":
						eventOf(es, "response.output_text.done")["text"] = "synthetic-canary-key financial-canary"
					case "unsupported":
						es[1]["type"] = "response.synthetic_unsupported"
					case "eof":
						es = es[:len(es)-1]
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, sse(es))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch tc.mode {
				case "content_type":
					w.Header().Set("Content-Type", "text/plain")
					fmt.Fprint(w, "synthetic-canary-key financial-canary")
				case "body_read":
					w.Header().Set("Content-Length", "100000")
					fmt.Fprint(w, "short")
				case "terminal":
					resp := response([]any{contractCall("diagnostic")})
					resp["status"] = "incomplete"
					fmt.Fprint(w, encode(resp))
				case "item":
					item := contractCall("diagnostic")
					item["status"] = "incomplete"
					fmt.Fprint(w, encode(response([]any{item})))
				}
			})
			_, session, _ := ledgerSession(t)
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, completionMode(fake.model, tc.stream), builtinai.New(session, assistant.PageSelection{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			result, err := engine.Complete(t.Context(), nil, "Synthetic question")
			want := "provider_response_rejected." + tc.rule
			if err == nil || assistant.FailureCode(err) != want || err.Error() != want || len(result.History) != 0 {
				t.Fatalf("unsafe or lost classification: %v", err)
			}
		})
	}
}

type failedSDKConversion struct{ model.AgenticModel }

func (failedSDKConversion) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return nil, errors.New("synthetic-canary-key financial-canary")
}
func TestUnknownSDKErrorHasFixedConversionDiagnostic(t *testing.T) {
	fake := fakeModel(t, Limits{}, func(http.ResponseWriter, *http.Request, int, obj) { t.Error("unexpected HTTP request") })
	fake.model.inner = failedSDKConversion{fake.model.inner}
	_, err := fake.model.Generate(t.Context(), []*schema.AgenticMessage{schema.UserAgenticMessage("Synthetic greeting")})
	if err == nil || err.Error() != "provider_response_rejected.sdk_conversion" {
		t.Fatalf("unknown SDK error escaped: %v", err)
	}
}
