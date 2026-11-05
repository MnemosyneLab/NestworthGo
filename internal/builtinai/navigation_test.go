package builtinai

import (
	"context"
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

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
