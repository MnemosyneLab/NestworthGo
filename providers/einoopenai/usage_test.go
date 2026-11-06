package einoopenai

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

func TestWireUsageCapturesNullableDetailsOncePerRequest(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, _ obj) {
				resp := response([]any{textItem()})
				resp["model"] = "actual-snapshot"
				resp["usage"] = obj{"input_tokens": 100, "input_tokens_details": obj{"cached_tokens": 80, "cache_write_tokens": 10}, "output_tokens": 30, "output_tokens_details": obj{"reasoning_tokens": 20}}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					es := events([]any{textItem()})
					es[len(es)-1]["response"] = resp
					_, _ = io.WriteString(w, sse(es))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, encode(resp))
				}
			})
			records := map[string]assistant.UsageCall{}
			observations := 0
			ctx := assistant.WithUsage(context.Background(), func(c assistant.UsageCall) { observations++; records[c.ID] = c })
			for range 2 {
				if stream {
					reader, err := f.model.Stream(ctx, []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")})
					if err != nil {
						t.Fatal(err)
					}
					reader.Close()
				} else {
					if _, err := f.model.Generate(ctx, []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if len(records) != 2 || observations != 4 {
				t.Fatalf("double counting: %d/%d", len(records), observations)
			}
			for _, c := range records {
				if c.Model != "actual-snapshot" || c.Status != "succeeded" || c.Input == nil || *c.Input != 100 || c.CacheWrite == nil || *c.CacheWrite != 10 || c.Reasoning == nil || *c.Reasoning != 20 {
					t.Fatalf("lost wire metadata: %+v", c)
				}
			}
		})
	}
}
func TestFailedStreamRetainsUsageWithoutAdmittingOutput(t *testing.T) {
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, _ *http.Request, _ int, _ obj) {
		w.Header().Set("Content-Type", "text/event-stream")
		es := events([]any{textItem()})
		resp := es[len(es)-1]["response"].(obj)
		resp["status"] = "incomplete"
		resp["usage"] = obj{"input_tokens": 100, "output_tokens": 7}
		es[len(es)-1]["type"] = "response.incomplete"
		_, _ = io.WriteString(w, sse(es))
	})
	var record assistant.UsageCall
	ctx := assistant.WithUsage(context.Background(), func(c assistant.UsageCall) { record = c })
	if _, err := f.model.Stream(ctx, []*schema.AgenticMessage{schema.UserAgenticMessage("synthetic")}); err == nil {
		t.Fatal("incomplete output admitted")
	}
	if record.Status != "failed" || record.Input == nil || *record.Input != 100 || record.Output == nil || *record.Output != 7 {
		t.Fatalf("lost failed usage: %+v", record)
	}
}
