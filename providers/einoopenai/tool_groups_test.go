package einoopenai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func loadGroupsCall(round int, groups ...string) obj {
	return liveCallItem(round, loadToolGroups, obj{"groups": groups})
}

func TestTaskToolGroupsLoadOnlyCurrentTurnAndKeepEarlierSchemas(t *testing.T) {
	_, shared, _ := ledgerSession(t)
	var products atomic.Int32
	var core, investments []any
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		tools := request["tools"].([]any)
		has := func(name string) bool { return strings.Contains(encode(tools), `"name":"`+name+`"`) }
		allowed := func(name string) bool { return strings.Contains(encode(request["tool_choice"]), `"name":"`+name+`"`) }
		switch round {
		case 0:
			core = tools
			for _, name := range []string{"get_financial_context", "get_read_page", "get_operation", loadToolGroups} {
				if !has(name) || !allowed(name) {
					t.Errorf("initial core missing %s", name)
				}
			}
			if strings.Contains(encode(tools), "Business recipe:") {
				t.Error("optional business recipes entered core")
			}
			if has("get_products") || has("list_activities") || has("commit_plan") {
				t.Error("initial catalog included unrelated schemas")
			}
			writeMode(w, request, []any{loadGroupsCall(round, "investments", "untrusted-group")})
		case 1:
			if encode(tools) != encode(core) || has("get_products") {
				t.Error("partially loaded an invalid group request")
			}
			result := latestToolOutput(t, request)
			if !strings.Contains(result, `"code":"validation"`) || !strings.Contains(result, `"field":"groups"`) ||
				!strings.Contains(result, `"retryAction":"correct_input"`) || strings.Contains(result, "untrusted-group") {
				t.Errorf("group validation not safe: %s", result)
			}
			writeMode(w, request, []any{loadGroupsCall(round, "investments")})
		case 2:
			investments = tools
			if !strings.Contains(encode(tools), "A managed deposit or locked product") || strings.Contains(encode(tools), "Record completed facts with exact amounts") {
				t.Error("recipes not scoped to loaded investments")
			}
			if len(tools) <= len(core) || encode(tools[:len(core)]) != encode(core) ||
				!has("get_products") || !allowed("get_products") || has("list_activities") || has("commit_plan") {
				t.Error("investments group changed prefix, failed to load or exposed unrelated schemas")
			}
			writeMode(w, request, []any{liveCallItem(round*10, "get_products", obj{}), loadGroupsCall(round, "history")})
		case 3:
			if len(tools) <= len(investments) || encode(tools[:len(investments)]) != encode(investments) ||
				!has("get_products") || !allowed("get_products") || !has("list_activities") || !allowed("list_activities") {
				t.Error("second group did not append to the active catalog")
			}
			writeMode(w, request, []any{textItem()})
		case 4:
			if strings.Contains(encode(tools), "Business recipe:") {
				t.Error("restored history reloaded business recipes")
			}
			if encode(tools) != encode(core) || has("get_products") || allowed("list_activities") ||
				!strings.Contains(encode(request["input"]), `"name":"get_products"`) {
				t.Error("fresh turn inherited tool groups or lost saved native calls")
			}
			item := textItem()
			item["id"] = "msg_catalog_restore"
			writeMode(w, request, []any{item})
		case 5:
			if !has(loadToolGroups) || has("get_products") || has("commit_plan") || !allowed("get_operation") {
				t.Error("downgrade lost legitimate reads or inherited loaded groups/writes")
			}
			writeMode(w, request, []any{liveCallItem(round, "commit_plan", obj{})})
		default:
			t.Errorf("unexpected model round %d", round)
		}
	})
	cfg := assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}
	full := func() *Engine {
		session := builtinai.New(shared, assistant.PageSelection{}).WithFullAccess(func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			var q assistant.FullCapabilityRequest
			if err := jsoncodec.Unmarshal(args, &q); err != nil {
				return nil, err
			}
			if q.Name == "get_products" {
				products.Add(1)
			}
			return shared.CallFull(ctx, q.Name, q.Arguments)
		})
		e, err := engineWithModel(t.Context(), cfg, f.model, session, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(e.Close)
		return e
	}
	first, err := full().Complete(t.Context(), nil, "List investment products")
	if err != nil || products.Load() != 1 || len(first.Evidence) != 1 || first.Evidence[0].Source != "get_products" {
		t.Fatalf("loaded read did not use the business handler: evidence=%v calls=%d err=%v", first.Evidence, products.Load(), err)
	}
	restored, err := full().Complete(t.Context(), first.History, "Follow-up after restoring history")
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := engineWithModel(t.Context(), cfg, f.model, builtinai.New(shared, assistant.PageSelection{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if _, err := readonly.Complete(t.Context(), restored.History, "Read-only follow-up"); err == nil {
		t.Fatal("downgraded provider response executed a hidden write")
	}
}

func TestToolGroupLoadCannotAuthorizeSameResponseCall(t *testing.T) {
	_, shared, accountID := ledgerSession(t)
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
		writeMode(w, request, []any{
			loadGroupsCall(0, "changes"),
			liveCallItem(1, "get_account_valuations", obj{"accountIds": []string{accountID}}),
			liveCallItem(2, "commit_plan", obj{}),
		})
	})
	session := builtinai.New(shared, assistant.PageSelection{}).WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) {
		t.Error("business call dispatched before the invalid inventory was rejected")
		return nil, assistant.ErrUnavailable
	})
	e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err = e.Complete(t.Context(), nil, "Commit a change"); err == nil {
		t.Fatal("accepted a call to a group not exposed in this response")
	}
}

func TestReadOnlyLoadsAllReadGroupsWithoutMutationAuthority(t *testing.T) {
	_, shared, _ := ledgerSession(t)
	var core []any
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		tools := request["tools"].([]any)
		has := func(name string) bool { return strings.Contains(encode(tools), `"name":"`+name+`"`) }
		allowed := func(name string) bool { return strings.Contains(encode(request["tool_choice"]), `"name":"`+name+`"`) }
		switch round {
		case 0:
			core = tools
			if !has(loadToolGroups) || has("get_products") || has("get_context") {
				t.Error("read-only initial catalog is not on demand")
			}
			// Deliberately reversed request order must not determine schema order.
			writeMode(w, request, []any{loadGroupsCall(round, "maintenance", "reconciliation", "market_data", "investments", "history", "accounts", "changes")})
		case 1:
			if len(tools) <= len(core) || encode(tools[:len(core)]) != encode(core) {
				t.Error("read groups changed cacheable core prefix")
			}
			for _, name := range []string{"get_context", "get_products", "list_activities", "get_market_data", "list_reconciliation_sessions"} {
				if !has(name) || !allowed(name) {
					t.Errorf("missing legitimate read %s", name)
				}
			}
			for _, name := range []string{"commit_plan", "preview_batch", "execute_ledger_change", "analyze_period", "search_market_instruments"} {
				if has(name) || allowed(name) {
					t.Errorf("read-only loading added authority: %s", name)
				}
			}
			if !strings.Contains(latestToolOutput(t, request), `"unavailableGroups":["changes"]`) {
				t.Error("changes group should remain unavailable")
			}
			writeMode(w, request, []any{liveCallItem(round, "get_products", obj{})})
		case 2:
			writeMode(w, request, []any{textItem()})
		case 3:
			if encode(tools) != encode(core) || has("get_products") || allowed("get_products") {
				t.Error("restored read-only history loaded prior groups")
			}
			if !strings.Contains(encode(request["input"]), `"name":"get_products"`) {
				t.Error("restore lost historical read")
			}
			writeMode(w, request, []any{loadGroupsCall(round, "changes")})
		case 4:
			if encode(tools) != encode(core) || has("commit_plan") || allowed("commit_plan") {
				t.Error("requesting changes widened restored permissions")
			}
			item := textItem()
			item["id"] = "msg_readonly_restore"
			writeMode(w, request, []any{item})
		default:
			t.Errorf("unexpected model round %d", round)
		}
	})
	create := func() *Engine {
		e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, builtinai.New(shared, assistant.PageSelection{}), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(e.Close)
		return e
	}
	first, err := create().Complete(t.Context(), nil, "Read investment products")
	if err != nil || len(first.Evidence) != 1 || first.Evidence[0].Source != "get_products" {
		t.Fatalf("readonly grouped read failed: evidence=%v err=%v", first.Evidence, err)
	}
	if _, err := create().Complete(t.Context(), first.History, "Continue read-only after restore"); err != nil {
		t.Fatal(err)
	}
}
