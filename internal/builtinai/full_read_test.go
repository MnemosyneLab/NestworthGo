package builtinai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type fullReadInventory struct{ definition agentcapability.Definition }

func (f fullReadInventory) Definitions() ([]agentcapability.Definition, error) {
	return []agentcapability.Definition{f.definition}, nil
}
func (f fullReadInventory) FullDefinition(name string) (agentcapability.Definition, error) {
	if name != f.definition.Name {
		return agentcapability.Definition{}, assistant.ErrUnavailable
	}
	return f.definition, nil
}
func (fullReadInventory) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("Full read incorrectly used restricted dispatcher")
}

func TestFullReadPreservesEvidenceAndAccountNavigation(t *testing.T) {
	for _, name := range []string{"list_accounts", "get_account_valuations", "get_read_page"} {
		for _, action := range []string{"open_page", "open_return_trend"} {
			t.Run(name+"/"+action, func(t *testing.T) {
				accountID := uuid.NewString()
				output := json.RawMessage(`{"data":{"sections":{"accounts":{"items":[{"id":"` + accountID + `","name":"Synthetic disclosed account","defaultCurrency":"USD"}],"nextCursor":"synthetic-next","complete":false}}}}`)
				calls := 0
				s := New(fullReadInventory{agentcapability.Definition{Name: name, Effect: agentcapability.Read}}, assistant.PageSelection{}, true).WithFullAccess(func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
					calls++
					var q assistant.FullCapabilityRequest
					if jsoncodec.Unmarshal(raw, &q) != nil || q.Name != name || string(q.Arguments) != `{}` {
						t.Fatal("wrong Full request", string(raw))
					}
					return output, nil
				})
				if _, err := s.Call(t.Context(), name, []byte(`{}`)); err != nil {
					t.Fatal(err)
				}
				evidence := s.Evidence()
				if calls != 1 || len(evidence) != 1 || evidence[0].Source != name || evidence[0].CapturedAt.IsZero() || !evidence[0].HasMore || evidence[0].Missing != "reported" {
					t.Fatalf("lost Full read evidence: %+v", evidence)
				}
				if _, err := s.callInternal(t.Context(), "get_navigation_targets", []byte(`{}`)); err != nil {
					t.Fatal(err)
				}
				ref := ""
				for key, target := range s.targets {
					if target.target.Kind == "account" && target.target.AccountID == accountID {
						ref = key
					}
				}
				if ref == "" {
					t.Fatal("Full read did not disclose account target")
				}
				args := map[string]any{"targetRef": ref}
				if action == "open_return_trend" {
					args["from"] = "2026-01-01"
					args["to"] = "2026-02-28"
					args["valuation"] = "base"
					args["includeCash"] = false
					args["display"] = "linked_rate"
				}
				raw, _ := jsoncodec.Marshal(args)
				if _, err := s.callInternal(t.Context(), action, raw); err != nil {
					t.Fatal(err)
				}
				target := s.Navigation()
				if target == nil || target.AccountID != accountID {
					t.Fatalf("wrong Full navigation: %+v", target)
				}
			})
		}
	}
}

func TestFullReadPostprocessingUsesTrustedEffectAndPreservesBounds(t *testing.T) {
	for _, effect := range []agentcapability.Effect{agentcapability.Read, agentcapability.Analysis, agentcapability.Preview, agentcapability.Write} {
		t.Run(string(effect), func(t *testing.T) {
			accountID := uuid.NewString()
			output := json.RawMessage(`{"data":{"section":"accounts","items":[{"id":"` + accountID + `","name":"Synthetic"}],"hasMore":true,"missingInputs":["synthetic"]}}`)
			fail := true
			calls := 0
			s := New(fullReadInventory{agentcapability.Definition{Name: "synthetic_capability", Effect: effect}}, assistant.PageSelection{}, true).WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) {
				calls++
				if fail {
					return nil, assistant.ErrUnavailable
				}
				return output, nil
			})
			initialTargets := len(s.targets)
			if _, err := s.Call(t.Context(), "synthetic_capability", []byte(`{}`)); err == nil {
				t.Fatal("failure swallowed")
			}
			if len(s.Evidence()) != 0 || len(s.targets) != initialTargets {
				t.Fatal("failed read added evidence/navigation")
			}
			fail = false
			if _, err := s.Call(t.Context(), "synthetic_capability", []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
			isRead := effect == agentcapability.Read || effect == agentcapability.Analysis
			if !isRead {
				if len(s.Evidence()) != 0 || len(s.targets) != initialTargets {
					t.Fatal("preview/write receipt treated as read")
				}
				return
			}
			if len(s.Evidence()) != 1 || len(s.targets) != initialTargets+1 {
				t.Fatal("read effect not recorded")
			}
			for range 15 {
				if _, err := s.Call(t.Context(), "synthetic_capability", []byte(`{}`)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Call(t.Context(), "synthetic_capability", []byte(`{}`)); assistant.NormalizeBudgetFailure(err) == nil {
				t.Fatalf("read evidence budget lost: %v", err)
			}
			if calls != 17 {
				t.Fatalf("over-budget read executed: calls=%d", calls)
			}
			if len(s.Evidence()) != 16 {
				t.Fatal("evidence exceeded bound")
			}
		})
	}
}

func TestFullReadReservesCapacityForConcurrentAnalysis(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	calls := 0
	s := New(fullReadInventory{agentcapability.Definition{Name: "analyze_period", Effect: agentcapability.Analysis}}, assistant.PageSelection{}).WithFullAccess(func(context.Context, json.RawMessage) (json.RawMessage, error) {
		calls++
		if calls == 16 {
			close(entered)
			<-finish
		}
		return []byte(`{"data":{}}`), nil
	})
	for range 15 {
		if _, err := s.Call(t.Context(), "analyze_period", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() { _, err := s.Call(t.Context(), "analyze_period", []byte(`{}`)); done <- err }()
	<-entered
	_, err := s.Call(t.Context(), "analyze_period", []byte(`{}`))
	close(finish)
	if assistant.NormalizeBudgetFailure(err) == nil {
		t.Fatalf("concurrent capacity not reserved: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if calls != 16 || len(s.Evidence()) != 16 {
		t.Fatalf("unexpected executions/evidence: %d/%d", calls, len(s.Evidence()))
	}
}

type countedReadInventory struct {
	fullReadInventory
	calls int
}

func (s *countedReadInventory) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.calls++
	return []byte(`{"data":{}}`), nil
}
func TestReadOnlyAndPageReadBudgetRejectBeforeDispatch(t *testing.T) {
	for _, page := range []bool{false, true} {
		shared := &countedReadInventory{fullReadInventory: fullReadInventory{definition: agentcapability.Definition{Name: "list_accounts", Effect: agentcapability.Read}}}
		s := New(shared, assistant.PageSelection{})
		name := "list_accounts"
		if page {
			name = "inspect_page"
			s.WithPageReader(func(context.Context, json.RawMessage) (json.RawMessage, error) {
				shared.calls++
				return []byte(`{"complete":true}`), nil
			})
		}
		for range 16 {
			if _, err := s.Call(t.Context(), name, []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Call(t.Context(), name, []byte(`{}`)); assistant.NormalizeBudgetFailure(err) == nil {
			t.Fatal("budget not enforced", err)
		}
		if shared.calls != 16 {
			t.Fatalf("over-budget execution: %d", shared.calls)
		}
	}
}
