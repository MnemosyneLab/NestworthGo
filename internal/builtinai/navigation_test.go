package builtinai

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func trendArgs(t *testing.T, s *Session, account bool) map[string]any {
	t.Helper()
	if account {
		s.addNavigationTarget(assistant.ModelNavigationTarget{Kind: "account", AccountID: uuid.NewString()}, "Synthetic same name · USD")
	}
	var ref string
	for key, target := range s.targets {
		if account && target.Kind == "account" || !account && target.target.Tab == "trend" {
			ref = key
		}
	}
	return map[string]any{"targetRef": ref, "from": "2026-01-01", "to": "2026-02-28", "valuation": "base", "includeCash": false, "display": "linked_rate", "currency": "USD", "assetClass": "stock"}
}

func TestReturnTrendUsesOnlyAuthorizedFiniteActions(t *testing.T) {
	for _, account := range []bool{false, true} {
		s := New(nil, assistant.PageSelection{}, true)
		args := trendArgs(t, s, account)
		raw, _ := jsoncodec.Marshal(args)
		result, err := s.Call(t.Context(), "open_return_trend", raw)
		if err != nil || string(result) == "" {
			t.Fatal(err)
		}
		target := s.Navigation()
		if target == nil || !target.Valid() || target.Kind != "return-trend" || (target.AccountID != "") != account || target.Trend.From != "2026-01-01" || target.Trend.IncludeCash {
			t.Fatalf("wrong action: %+v", target)
		}
		// Conflicts withdraw the first action and permanently spend the turn, even
		// if a provider attempts a third call after the second failed.
		for range 3 {
			if _, err = s.Call(t.Context(), "open_return_trend", raw); err == nil || s.Navigation() != nil {
				t.Fatal("repeated action survived")
			}
		}
		ordinary := New(nil, assistant.PageSelection{})
		if _, err = ordinary.Call(t.Context(), "open_return_trend", raw); err == nil {
			t.Fatal("ordinary read gained authority")
		}
	}
}

func TestReturnTrendRejectsAmbiguousMalformedAndInjectedArguments(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"from", "2026-02-30"}, {"from", "last month"}, {"from", "2026-2-01"}, {"from", "2026-03-01"}, {"to", "2026-01-00"},
		{"from", nil}, {"includeCash", nil}, {"includeCash", "false"}, {"valuation", "guess"}, {"display", "click"}, {"currency", "ZZZ"}, {"assetClass", "sql"},
		{"targetRef", uuid.NewString()}, {"accountId", uuid.NewString()}, {"memberId", uuid.NewString()}, {"selector", "#confirm"},
	} {
		t.Run(tc.key+"/"+string(mustJSON(t, tc.value)), func(t *testing.T) {
			s := New(nil, assistant.PageSelection{}, true)
			args := trendArgs(t, s, false)
			args[tc.key] = tc.value
			if _, err := s.Call(t.Context(), "open_return_trend", mustJSON(t, args)); err == nil || s.Navigation() != nil {
				t.Fatal("invalid action accepted")
			}
		})
	}
	s := New(nil, assistant.PageSelection{}, true)
	args := trendArgs(t, s, false)
	delete(args, "includeCash")
	if _, err := s.Call(t.Context(), "open_return_trend", mustJSON(t, args)); err == nil {
		t.Fatal("implicit boolean accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Call(ctx, "open_return_trend", mustJSON(t, trendArgs(t, s, false))); err == nil {
		t.Fatal("cancelled action accepted")
	}
}

func TestConcurrentTrendCallsCannotLeaveAnIntent(t *testing.T) {
	s := New(nil, assistant.PageSelection{}, true)
	raw := mustJSON(t, trendArgs(t, s, false))
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { _, _ = s.Call(t.Context(), "open_return_trend", raw) })
	}
	wg.Wait()
	if s.Navigation() != nil {
		t.Fatal("concurrent competing actions left authority")
	}
}

func TestEveryLaterNavigationAttemptRevokesTheAcceptedRequest(t *testing.T) {
	for _, first := range []string{"open_page", "open_return_trend"} {
		for _, second := range []string{"open_page", "open_return_trend"} {
			for _, tc := range []struct {
				name, raw string
				cancelled bool
			}{
				{"malformed", `{`, false}, {"oversized", strings.Repeat(" ", 4097), false},
				{"missing", `{}`, false}, {"null", `{"targetRef":null}`, false},
				{"wrong-type", `{"targetRef":42}`, false}, {"unknown", `{"targetRef":"unknown"}`, false},
				{"extra-field", `{"targetRef":"unknown","selector":"#apply"}`, false},
				{"cancelled", `{}`, true}, {"valid", "", false},
			} {
				t.Run(first+"/"+second+"/"+tc.name, func(t *testing.T) {
					s := New(nil, assistant.PageSelection{}, true)
					args := trendArgs(t, s, false)
					valid := map[string][]byte{"open_page": mustJSON(t, map[string]any{"targetRef": args["targetRef"]}), "open_return_trend": mustJSON(t, args)}
					if _, err := s.Call(t.Context(), first, valid[first]); err != nil || s.Navigation() == nil {
						t.Fatal("initial action refused", err)
					}
					ctx := t.Context()
					if tc.cancelled {
						var cancel context.CancelFunc
						ctx, cancel = context.WithCancel(ctx)
						cancel()
					}
					raw := []byte(tc.raw)
					if tc.name == "valid" {
						raw = valid[second]
					}
					if _, err := s.Call(ctx, second, raw); err == nil {
						t.Fatal("second attempt accepted")
					}
					if s.Navigation() != nil {
						t.Fatal("rejected second attempt retained the old request")
					}
					if _, err := s.Call(t.Context(), first, valid[first]); err == nil || s.Navigation() != nil {
						t.Fatal("third attempt revived a spent turn")
					}
				})
			}
		}
	}
}

func TestNavigationReadsDoNotSpendOrRevokeAnAction(t *testing.T) {
	s := New(nil, assistant.PageSelection{}, true)
	args := mustJSON(t, trendArgs(t, s, false))
	// Only an accepted action spends the grant. A first malformed attempt may
	// be corrected while the same run and page lease remain valid.
	if _, err := s.Call(t.Context(), "open_return_trend", []byte(`{`)); err == nil {
		t.Fatal("malformed initial request accepted")
	}
	if _, err := s.Call(t.Context(), "open_return_trend", args); err != nil {
		t.Fatal("initial refusal spent the grant", err)
	}
	for _, raw := range []string{`{}`, `{`, `{"unexpected":true}`} {
		_, _ = s.Call(t.Context(), "get_navigation_targets", []byte(raw))
		if s.Navigation() == nil {
			t.Fatal("read-only inventory call revoked action")
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
