package einoopenai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// These tests inspect actual SDK requests received by httptest HTTP, never
// infer server cache hits from synthetic responses or prefix equality.
func TestPromptCacheWireModePageRestoreAndInventory(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			_, shared, accountID := ledgerSession(t)
			var priorInput []any
			var readCatalog, fullCatalog string
			var malicious string
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				toolList := request["tools"].([]any)
				hasLoader := toolList[len(toolList)-1].(map[string]any)["name"] == loadToolGroups
				if hasLoader {
					toolList = toolList[:len(toolList)-1]
				}
				tools := encode(toolList)
				input := request["input"].([]any)
				if malicious == "" {
					wantSnapshots := []int{1, 1, 2, 3, 3}[n]
					if got := strings.Count(encode(input), "Current host mode:"); got != wantSnapshots {
						t.Errorf("mode snapshots = %d, want %d", got, wantSnapshots)
					}
				}
				if len(priorInput) > 0 && encode(input[:len(priorInput)]) != encode(priorInput) {
					t.Error("earlier wire input changed")
				}
				priorInput = append([]any(nil), input...)
				if n < 2 {
					if readCatalog == "" {
						readCatalog = tools
					}
					if tools != readCatalog {
						t.Error("page switch changed catalog")
					}
				} else {
					if fullCatalog == "" {
						fullCatalog = tools
					}
					if tools != fullCatalog {
						t.Error("Full downgrade/restore changed catalog")
					}
				}
				if n < 2 && strings.Contains(tools, `"name":"commit_plan"`) {
					t.Error("read-only eagerly included Full catalog")
				}
				choice, ok := request["tool_choice"].(map[string]any)
				if !ok || choice["type"] != "allowed_tools" || choice["mode"] != "auto" {
					t.Error("allowed_tools not delivered")
				}
				allowed := encode(choice["tools"])
				if strings.Contains(allowed, `"name":"commit_plan"`) || !hasLoader ||
					!strings.Contains(allowed, `"name":"get_operation"`) {
					t.Error("wrong initial Full allowance")
				}
				if strings.Contains(allowed, `"name":"inspect_page"`) != (n == 1 || n == 2) {
					t.Error("wrong page allowance")
				}
				if request["store"] != false {
					t.Error("remote storage enabled")
				}
				for _, field := range []string{"previous_response_id", "conversation", "prompt_cache_key", "prompt_cache_retention"} {
					if _, ok := request[field]; ok {
						t.Errorf("unexpected %s", field)
					}
				}
				if malicious != "" {
					writeMode(w, request, []any{liveCallItem(n*2, "get_account_valuations", obj{"accountIds": []string{accountID}}), liveCallItem(n*2+1, malicious, obj{})})
					return
				}
				item := textItem()
				item["id"] = fmt.Sprintf("msg_cache_%d", n)
				// Synthetic billing fields exercise #258 without claiming a real hit.
				resp := response([]any{item})
				resp["usage"] = obj{"input_tokens": 100, "input_tokens_details": obj{"cached_tokens": 80, "cache_write_tokens": 0}, "output_tokens": 10}
				if stream {
					es := events([]any{item})
					es[len(es)-1]["response"] = resp
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, sse(es))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, encode(resp))
				}
			})
			var history []byte
			for n := 0; n < 5; n++ {
				page := assistant.PageSelection{}
				if n == 1 || n == 2 {
					page = assistant.PageSelection{Kind: "overview"}
				}
				session := builtinai.New(shared, page)
				if n == 2 {
					session.WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) {
						t.Error("unexpected dispatch")
						return nil, assistant.ErrUnavailable
					})
				}
				e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
				if err != nil {
					t.Fatal(err)
				}
				if n > 2 && e.tools["commit_plan"] != nil {
					t.Fatal("historical Full entered executable map")
				}
				if stream {
					e.model = streamCompletionModel{f.model}
				}
				var usage assistant.UsageCall
				ctx := assistant.WithUsage(t.Context(), func(c assistant.UsageCall) { usage = c })
				result, err := e.Complete(ctx, history, "synthetic cache turn")
				if err != nil {
					t.Fatal(err)
				}
				if usage.Status != "succeeded" || usage.CachedInput == nil || *usage.CachedInput != 80 || usage.CacheWrite == nil || *usage.CacheWrite != 0 {
					t.Fatalf("wire usage lost: %+v", usage)
				}
				e.Close()
				// Rebuild engine each turn and serialize/deserialize the actual checkpoint,
				// as the runtime does after page/permission changes or process restart.
				history = append([]byte(nil), result.History...)
			}
			if fullCatalog == readCatalog {
				t.Fatal("Full catalog never upgraded")
			}
			for _, bad := range []string{"commit_plan", "inspect_page", "navigate_page", "operate_page", "get_current_page", "read_page_content", "get_navigation_targets", "open_page", "open_return_trend", "operate_page_control", "unknown_tool"} {
				malicious = bad
				// Reuse unchanged successful history after each refused batch.
				priorInput = nil
				e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, builtinai.New(shared, assistant.PageSelection{}), nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = e.Complete(t.Context(), history, "malicious mixture"); err == nil {
					t.Fatal("mixed batch accepted")
				}
				shared.mu.Lock()
				reads := shared.reads
				shared.mu.Unlock()
				if reads != 0 {
					t.Fatal("valid call dispatched before forbidden call rejection")
				}
				e.Close()
			}
		})
	}
}

// Opt-in dump of actual tool JSON for reproducible tokenizer/cost experiments.
// No request data, credentials, ledger content or provider usage is exported.
func TestPromptCacheCatalogSizes(t *testing.T) {
	_, shared, _ := ledgerSession(t)
	for _, desktop := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			session := builtinai.New(shared, assistant.PageSelection{}, desktop)
			if desktop {
				refuse := func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, assistant.ErrUnavailable }
				session.WithPageReader(refuse).WithPageOperator(refuse)
			}
			if full {
				session.WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, assistant.ErrUnavailable })
			}
			for _, stable := range []bool{false, true} {
				label := fmt.Sprintf("desktop-%t-full-%t-stable-%t", desktop, full, stable)
				f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
					raw := []byte(encode(request["tools"]))
					t.Logf("%s tools=%d bytes=%d", label, len(request["tools"].([]any)), len(raw))
					if stable && (len(raw) > 20000 || strings.Contains(string(raw), "Business recipe:")) {
						t.Error("optional guidance/schema inflated stable core")
					}
					// Finite ledger/correction contracts add ~3.7 KiB; the stable
					// core keeps its original 20 KiB ceiling. Product contracts add
					// another ~5 KiB plus ~4 KiB of interest-mode dependencies to
					// the optional catalog.
					if !stable && len(raw) > 140000 {
						t.Error("complete built-in catalog exceeded reviewed wire budget")
					}
					if dir := os.Getenv("NESTWORTH_CATALOG_EVIDENCE_DIR"); dir != "" {
						if err := os.WriteFile(filepath.Join(dir, label+".choice.json"), []byte(encode(request["tool_choice"])), 0600); err != nil {
							t.Error(err)
						}
						if err := os.WriteFile(filepath.Join(dir, label+".json"), raw, 0600); err != nil {
							t.Error(err)
						}
					}
					writeMode(w, request, []any{textItem()})
				})
				e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
				if err != nil {
					t.Fatal(err)
				}
				if !stable {
					e.catalog = nil
				}
				if _, err = e.Complete(t.Context(), nil, "synthetic"); err != nil {
					t.Fatal(err)
				}
				e.Close()
			}
		}
	}
}

func TestPromptCacheCatalogFlagCannotGrantPermission(t *testing.T) {
	_, shared, _ := ledgerSession(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
		if !strings.Contains(encode(request["tools"]), `"name":"get_operation"`) ||
			!strings.Contains(encode(request["tool_choice"]), `"name":"get_operation"`) ||
			!strings.Contains(encode(request["tools"]), loadToolGroups) ||
			strings.Contains(encode(request["tool_choice"]), `"name":"commit_plan"`) {
			t.Error("catalog marker became grant")
		}
		writeMode(w, request, []any{liveCallItem(n, "commit_plan", obj{})})
	})
	e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, builtinai.New(shared, assistant.PageSelection{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	saved := checkpoint{Version: 2, Provider: e.config.Provider, Model: e.config.Model, CatalogFull: true}
	// Reuse a valid system message but no historical Full grant.
	saved.Messages = []*schema.AgenticMessage{schema.SystemAgenticMessage(commonInstructions())}
	raw, err := jsoncodec.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Complete(t.Context(), raw, "synthetic"); err == nil {
		t.Fatal("checkpoint flag authorized a mutation")
	}
}

func TestPromptCacheUnchangedModeRetains63TurnCapacity(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprint(full), func(t *testing.T) {
			_, shared, _ := ledgerSession(t)
			session := builtinai.New(shared, assistant.PageSelection{})
			if full {
				session.WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) {
					t.Error("unexpected dispatch")
					return nil, assistant.ErrUnavailable
				})
			}
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if n >= 63 {
					t.Error("exhausted history reached provider")
				}
				input := request["input"].([]any)
				if len(input) != 3+2*n || strings.Count(encode(input), "Current host mode:") != 1 {
					t.Errorf("turn %d: repeated mode snapshot or lost history", n+1)
				}
				item := textItem()
				item["id"] = fmt.Sprintf("msg_capacity_%d", n)
				writeMode(w, request, []any{item})
			})
			var history []byte
			for n := 0; n < 64; n++ {
				e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
				if err != nil {
					t.Fatal(err)
				}
				result, err := e.Complete(t.Context(), history, "synthetic capacity")
				e.Close()
				if n == 63 {
					wantBudget(t, result, err, "history_messages")
					break
				}
				if err != nil {
					t.Fatalf("turn %d: %v", n+1, err)
				}
				history = result.History
			}
			var saved checkpoint
			if err := jsoncodec.Unmarshal(history, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Messages) != 128 {
				t.Fatalf("last complete checkpoint has %d messages", len(saved.Messages))
			}
		})
	}
}

func TestModeSnapshotUsesLatestExactHostPolicy(t *testing.T) {
	common := schema.SystemAgenticMessage(commonInstructions())
	read := schema.SystemAgenticMessage(modeInstructions(false))
	full := schema.SystemAgenticMessage(modeInstructions(true))
	for _, tc := range []struct {
		name     string
		messages []*schema.AgenticMessage
		want     int
	}{
		{"legacy", []*schema.AgenticMessage{common, schema.UserAgenticMessage("old")}, 3},
		{"same", []*schema.AgenticMessage{common, read, schema.UserAgenticMessage("old")}, 3},
		{"old duplicates", []*schema.AgenticMessage{common, read, read, schema.UserAgenticMessage("old")}, 4},
		{"downgrade", []*schema.AgenticMessage{common, read, full, schema.UserAgenticMessage("old")}, 5},
		{"user cannot suppress downgrade", []*schema.AgenticMessage{common, full, schema.UserAgenticMessage(modeInstructions(false))}, 4},
		{"policy update", []*schema.AgenticMessage{common, schema.SystemAgenticMessage("Current host mode: Read-only. obsolete policy")}, 3},
		{"unknown later system", []*schema.AgenticMessage{common, read, schema.SystemAgenticMessage("unknown")}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := encode(tc.messages)
			got := appendModeSnapshot(tc.messages, false)
			if len(got) != tc.want {
				t.Fatalf("messages=%d want=%d", len(got), tc.want)
			}
			if encode(got[:len(tc.messages)]) != before {
				t.Fatal("earlier history changed")
			}
			// Round-tripped checkpoints must deduplicate equally without extra flags.
			raw := encode(got)
			var restored []*schema.AgenticMessage
			if err := jsoncodec.Unmarshal([]byte(raw), &restored); err != nil {
				t.Fatal(err)
			}
			if len(appendModeSnapshot(restored, false)) != len(got) {
				t.Fatal("restore duplicated snapshot")
			}
		})
	}
}
