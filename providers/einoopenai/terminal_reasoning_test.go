package einoopenai

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/diagnostics"
)

// Full part lifecycles use the item.done snapshot; only the terminal cipher changes.
func refreshedReasoningEvents() []obj {
	es := contractEvents([]any{reasoningItem("rs_canary"), callItem()}, "", nil)
	terminal := clone(es[len(es)-1]["response"].(obj))
	terminal["output"].([]any)[0].(obj)["encrypted_content"] = "terminal-cipher-canary"
	es[len(es)-1]["response"] = terminal
	return es
}

func TestReasoningCipherRefreshGateAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func([]obj) []obj
		success bool
	}{
		{"refresh", nil, true},
		{"identity", func(es []obj) []obj { terminalReason(es)["id"] = "changed"; return es }, false},
		{"summary", func(es []obj) []obj {
			terminalReason(es)["summary"] = []any{obj{"type": "summary_text", "text": "changed"}}
			return es
		}, false},
		{"reasoning_text", func(es []obj) []obj {
			terminalReason(es)["content"] = []any{obj{"type": "reasoning_text", "text": "changed"}}
			return es
		}, false},
		{"unknown_native_field", func(es []obj) []obj { terminalReason(es)["future_native_field"] = obj{"exact": "changed"}; return es }, false},
		{"status", func(es []obj) []obj { terminalReason(es)["status"] = "incomplete"; return es }, false},
		{"cipher_missing", func(es []obj) []obj { delete(terminalReason(es), "encrypted_content"); return es }, false},
		{"cipher_null", func(es []obj) []obj { terminalReason(es)["encrypted_content"] = nil; return es }, false},
		{"cipher_empty", func(es []obj) []obj { terminalReason(es)["encrypted_content"] = ""; return es }, false},
		{"cipher_type", func(es []obj) []obj { terminalReason(es)["encrypted_content"] = 123; return es }, false},
		{"done_cipher_missing", func(es []obj) []obj {
			delete(eventOf(es, "response.output_item.done")["item"].(obj), "encrypted_content")
			return es
		}, false},
		{"response_id", func(es []obj) []obj { es[len(es)-1]["response"].(obj)["id"] = "changed"; return es }, false},
		{"item_count", func(es []obj) []obj {
			r := es[len(es)-1]["response"].(obj)
			r["output"] = r["output"].([]any)[:1]
			return es
		}, false},
		{"item_done_missing", func(es []obj) []obj {
			for i, e := range es {
				if e["type"] == "response.output_item.done" {
					return append(es[:i], es[i+1:]...)
				}
			}
			panic("fixture")
		}, false},
		{"arguments", func(es []obj) []obj {
			es[len(es)-1]["response"].(obj)["output"].([]any)[1].(obj)["arguments"] = `{"amount":"99"}`
			return es
		}, false},
		{"call_id", func(es []obj) []obj {
			es[len(es)-1]["response"].(obj)["output"].([]any)[1].(obj)["call_id"] = "changed"
			return es
		}, false},
		{"no_terminal", func(es []obj) []obj { return es[:len(es)-1] }, false},
		{"after_terminal", func(es []obj) []obj { return append(es, obj{"type": "response.in_progress"}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, req obj) {
				if n > 0 {
					writeMode(w, req, []any{textItem()})
					return
				}
				es := refreshedReasoningEvents()
				if tc.change != nil {
					es = tc.change(es)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sse(es))
			})
			count, errs := run(t, t.Context(), f.model, true)
			if !tc.success {
				if count != 0 || len(errs) == 0 || len(f.requests) != 1 {
					t.Fatalf("unverified output escaped: count=%d errors=%v requests=%d", count, errs, len(f.requests))
				}
				return
			}
			if count != 1 || len(errs) != 0 || len(f.requests) != 2 {
				t.Fatalf("count=%d errors=%v requests=%d", count, errs, len(f.requests))
			}
			found := false
			for _, raw := range f.requests[1]["input"].([]any) {
				item := raw.(obj)
				if item["type"] == "reasoning" {
					found = true
					if !same([]byte(encode(item)), []byte(encode(terminalReason(refreshedReasoningEvents())))) {
						t.Fatal("terminal native snapshot was not replayed exactly")
					}
				}
			}
			if !found {
				t.Fatal("terminal reasoning missing from replay")
			}
		})
	}
}

func terminalReason(es []obj) obj { return es[len(es)-1]["response"].(obj)["output"].([]any)[0].(obj) }

func TestCipherExceptionDoesNotApplyToMessagesOrCalls(t *testing.T) {
	for _, item := range []obj{textItem(), callItem()} {
		a := clone(item)
		b := clone(item)
		a["encrypted_content"] = "old"
		b["encrypted_content"] = "new"
		if ok, _ := sameTerminalItem([]byte(encode(a)), []byte(encode(b))); ok {
			t.Fatalf("exception applied to %s", item["type"])
		}
	}
}

func TestStreamDiagnosticsReachApplicationLog(t *testing.T) {
	for _, tc := range []struct {
		name, level, want string
		reject            bool
	}{
		{"accepted", "info", "AI provider reasoning ciphertext refreshed", false},
		{"rejected", "warn", "item_mismatch", true},
		{"off", "off", "", true},
		{"info_filtered", "warn", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "logs", "nestworth.log")
			sink := diagnostics.New(path)
			if err := sink.Configure(tc.level); err != nil {
				t.Fatal(err)
			}
			prior := slog.Default()
			slog.SetDefault(sink.Logger())
			defer func() { slog.SetDefault(prior); _ = sink.Close() }()
			es := refreshedReasoningEvents()
			if tc.reject {
				terminalReason(es)["summary"] = []any{obj{"type": "summary_text", "text": "secret-financial-canary"}}
			}
			_, _, err := validateStream([]byte(sse(es)), Limits{Events: 4096, Items: 128})
			if (err != nil) != tc.reject {
				t.Fatalf("unexpected result: %v", err)
			}
			if tc.reject && assistant.FailureCode(err) != "provider_response_rejected.stream_terminal" {
				t.Fatal("lost diagnostic code")
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil && !(tc.level == "off" && os.IsNotExist(readErr)) {
				t.Fatal(readErr)
			}
			logs := string(raw)
			if tc.want == "" {
				if logs != "" {
					t.Fatal("disabled/filtered log emitted")
				}
			} else if !strings.Contains(logs, tc.want) {
				t.Fatalf("missing diagnostic: %s", logs)
			}
			if tc.reject && tc.level != "off" && !strings.Contains(logs, "provider_response_rejected.stream_terminal") {
				t.Fatal("missing failure code")
			}
			for _, secret := range []string{"canary", "INTERNAL_SYNTHETIC_REASONING", "12.34", "encrypted_content", "rs_", "fc_"} {
				if strings.Contains(logs, secret) {
					t.Fatalf("private value escaped: %s", secret)
				}
			}
		})
	}
}
