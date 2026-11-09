package builtinai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestConsolidatedPageSurfaceAndRetiredIdentities(t *testing.T) {
	s := New(operationInventory{}, assistant.PageSelection{Kind: "overview"}, true).
		WithPageReader(func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"complete":true}`), nil
		}).
		WithPageOperator(func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"state":"dispatched","partial":false}`), nil
		})
	defs, err := s.Definitions()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"inspect_page": false, "navigate_page": false, "operate_page": false}
	for _, d := range defs {
		if _, ok := want[d.Name]; ok {
			want[d.Name] = true
		} else {
			t.Fatalf("unexpected desktop tool %s", d.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"get_current_page", "read_page_content", "get_navigation_targets", "open_page", "open_return_trend", "operate_page_control"} {
		if _, err := s.Call(t.Context(), name, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("retired identity executes: %s", name)
		}
	}
	raw, err := s.Call(t.Context(), "inspect_page", json.RawMessage(`{"source":"navigation"}`))
	if err != nil || !strings.Contains(string(raw), `"targets"`) {
		t.Fatalf("navigation inspection: %s %v", raw, err)
	}
	raw, err = s.Call(t.Context(), "inspect_page", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(string(raw), `"complete":true`) {
		t.Fatalf("content inspection: %s %v", raw, err)
	}
	for _, args := range []string{`{"source":"navigation","selector":"x"}`, `{"source":"unknown"}`, `{"source":null}`, `null`, `[]`} {
		if _, err = s.Call(t.Context(), "inspect_page", json.RawMessage(args)); err == nil {
			t.Fatalf("invalid inspection accepted: %s", args)
		}
	}
}

func TestNavigatePagePreservesTypedTrendAndSpentIntentFence(t *testing.T) {
	s := New(operationInventory{}, assistant.PageSelection{}, true)
	args := trendArgs(t, s, false)
	ref := args["targetRef"]
	delete(args, "targetRef")
	raw, err := s.Call(t.Context(), "navigate_page", mustJSON(t, map[string]any{"targetRef": ref, "trend": args}))
	if err != nil || !strings.Contains(string(raw), `"requested"`) {
		t.Fatalf("trend rejected: %s %v", raw, err)
	}
	target := s.Navigation()
	if target == nil || target.Trend == nil || target.Trend.Display != "linked_rate" || target.Trend.Currency != "USD" || target.Trend.IncludeCash {
		t.Fatalf("lost typed trend: %+v", target)
	}
	if _, err = s.Call(t.Context(), "navigate_page", json.RawMessage(`{"trend":null}`)); err == nil || s.Navigation() != nil {
		t.Fatal("malformed later attempt preserved prior authority")
	}
}

func TestOperatePagePassesBatchWithoutGrantingAuthority(t *testing.T) {
	raw := json.RawMessage(`{"snapshotId":"snapshot","actions":[{"targetRef":"one","action":"set_value","value":"a"},{"targetRef":"two","action":"activate"}]}`)
	s := New(operationInventory{}, assistant.PageSelection{})
	if _, err := s.Call(t.Context(), "operate_page", raw); err == nil {
		t.Fatal("operation without host")
	}
	var got json.RawMessage
	s.WithPageOperator(func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
		got = append(got[:0], input...)
		return json.RawMessage(`{"state":"blocked","partial":true}`), nil
	})
	result, err := s.Call(t.Context(), "operate_page", raw)
	if err != nil || string(got) != string(raw) || !strings.Contains(string(result), `"partial":true`) {
		t.Fatal("batch/result changed", err)
	}
	defs, _ := s.Definitions()
	for _, d := range defs {
		if d.Name == "operate_page" {
			encoded, _ := jsoncodec.Marshal(d.InputSchema)
			if !strings.Contains(string(encoded), `"maxItems":20`) {
				t.Fatal("unbounded operation schema")
			}
		}
	}
}
