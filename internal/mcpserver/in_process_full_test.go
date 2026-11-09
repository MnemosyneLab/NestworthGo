package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestFullRegistryParityAndDurableDirectoryReceipt(t *testing.T) {
	fx := newLedgerFixture(t)
	dir, id := t.TempDir(), uuid.NewString()
	s, err := NewInProcessSession(t.Context(), fx.app, dir, id, ReadWrite, nil)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := s.FullDefinitions()
	if err != nil || !reflect.DeepEqual(defs, s.service.buildToolSet(ReadWrite, true).Definitions()) {
		t.Fatal("Full differs from shared MCP registry", err)
	}
	rawGolden, err := os.ReadFile(discoveryContractGolden)
	if err != nil {
		t.Fatal(err)
	}
	var golden discoveryContract
	if err := jsoncodec.Unmarshal(rawGolden, &golden); err != nil {
		t.Fatal(err)
	}
	if len(defs) != len(golden[ReadWrite]) {
		t.Fatal("Full inventory count drift")
	}
	for _, d := range defs {
		if hash, exists := golden[ReadWrite][d.Name]; !exists || hash != contractHash(t, sdkTool(d)) {
			t.Fatalf("Full contract drift: %s", d.Name)
		}
	}
	for _, name := range investmentReviewManagementTools {
		d, err := s.FullDefinition(name)
		if err != nil || d.Effect != agentcapability.Write {
			t.Fatalf("missing Full review management writer: %s", name)
		}
		if _, err := s.Call(t.Context(), name, nil); err == nil {
			t.Fatal("ordinary in-process dispatch accepted Full write")
		}
	}
	counts := map[agentcapability.Effect]int{}
	size := 0
	for _, d := range defs {
		counts[d.Effect]++
		raw, _ := jsoncodec.Marshal(d)
		size += len(raw)
	}
	t.Logf("Full MCP tools=%d effects=%v definition bytes=%d", len(defs), counts, size)
	operationID := uuid.NewString()
	args := []byte(`{"operationId":"` + operationID + `","input":{"name":"Synthetic Full member"}}`)
	if _, err := s.Call(t.Context(), "create_member", args); err == nil {
		t.Fatal("read surface acquired writes")
	}
	first, err := s.CallFull(t.Context(), "create_member", args)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CallFull(t.Context(), "create_member", args)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("replay did not return original receipt", err)
	}
	op, err := s.InspectFullOperation(operationID)
	if err != nil || op.Status != "succeeded" {
		t.Fatal("missing receipt", err)
	}
	s.Close()
	s, err = NewInProcessSession(t.Context(), fx.app, dir, id, ReadWrite, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	third, err := s.CallFull(t.Context(), "create_member", args)
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatal("restart replay changed receipt", err)
	}
	members, err := fx.app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range members {
		if m.Name == "Synthetic Full member" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("created %d members", n)
	}
}

func TestFullOperationIdentityMatchesCommitPlan(t *testing.T) {
	plan := uuid.NewString()
	want, _ := derivedCommitPlanOperationID(plan)
	got, err := FullOperationID("commit_plan", []byte(`{"input":{"planId":"`+plan+`"}}`))
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	if _, err := FullOperationID("create_member", []byte(`{"input":{"name":"Synthetic"}}`)); err == nil {
		t.Fatal("missing operation ID accepted")
	}
}

func TestFullCancellationBeforeDispatchIsExplicitlyUnsubmitted(t *testing.T) {
	s := newLocal(t, newLedgerFixture(t), ReadWrite)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.CallFull(ctx, "create_member", []byte(`{"operationId":"`+uuid.NewString()+`","input":{"name":"Synthetic cancelled"}}`))
	var marker interface{ NotDispatched() bool }
	if !errors.As(err, &marker) || !marker.NotDispatched() {
		t.Fatalf("missing pre-dispatch evidence: %v", err)
	}
}

func TestFullCommitRouteRejectsOnlyProvenUnsubmittedPlans(t *testing.T) {
	s := newLocal(t, newLedgerFixture(t), ReadWrite)
	unsupported := uuid.NewString()
	if err := s.service.writePrivate(filepath.Join(s.service.dir, "plans", unsupported+".json"), map[string]string{"type": "not_a_commit_plan"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		plan        string
		unsubmitted bool
		code        string
	}{
		{"invalid-plan", true, "validation"}, {unsupported, true, "validation"}, {uuid.NewString(), false, "operation_outcome_unknown"},
	} {
		operation := uuid.NewString()
		args := []byte(`{"operationId":"` + operation + `","input":{"planId":"` + tc.plan + `"}}`)
		if err := s.ValidateFull("commit_plan", args); err != nil {
			t.Fatal("fixture must pass schema validation", err)
		}
		_, err := s.CallFull(t.Context(), "commit_plan", args)
		requireLocalError(t, err, tc.code)
		var marker interface{ NotDispatched() bool }
		proven := errors.As(err, &marker) && marker.NotDispatched()
		if proven != tc.unsubmitted {
			t.Fatalf("plan %s: incorrect dispatch inference: %v", tc.plan, err)
		}
		if _, err := s.InspectFullOperation(operation); err == nil {
			t.Fatal("route rejection wrote an operation receipt")
		}
	}
}

func TestFullCancellationWhileWaitingForReceiptGate(t *testing.T) {
	for _, prior := range []bool{false, true} {
		t.Run(fmt.Sprint("prior=", prior), func(t *testing.T) {
			s := newLocal(t, newLedgerFixture(t), ReadWrite)
			operation := uuid.NewString()
			if prior {
				path, _ := s.service.operationPath(operation)
				if err := s.service.writePrivate(path, Operation{ID: operation, Tool: "synthetic_write", Status: "pending"}); err != nil {
					t.Fatal(err)
				}
			}
			entered := make(chan struct{})
			calls := 0
			set := newToolSet("synthetic", false)
			registerTool(set, &agentcapability.Definition{Name: "synthetic_write", Effect: agentcapability.Write}, func(ctx context.Context, in Mutation[Empty]) (Response, error) {
				close(entered)
				value, err := s.service.execute(ctx, in.OperationID, "synthetic_write", in.Input, func(context.Context) (any, error) { calls++; return map[string]bool{"posted": true}, nil })
				if err != nil {
					return Response{}, safeError(err)
				} // production-style wrapping must not lose the observation
				return Response{Data: value}, nil
			})
			s.registry = set.Registry
			s.service.operationMu.Lock()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := s.CallFull(ctx, "synthetic_write", []byte(`{"operationId":"`+operation+`","input":{}}`))
				done <- err
			}()
			<-entered
			cancel()
			s.service.operationMu.Unlock()
			err := <-done
			var marker interface{ NotDispatched() bool }
			proof := errors.As(err, &marker) && marker.NotDispatched()
			if proof == prior || calls != 0 {
				t.Fatalf("wrong queued cancellation proof=%v prior=%v calls=%d error=%v", proof, prior, calls, err)
			}
			receipt, lookup := s.InspectFullOperation(operation)
			if prior && (lookup != nil || receipt.Status != "unknown") {
				t.Fatal("old pending receipt lost", lookup)
			}
			if !prior && lookup == nil {
				t.Fatal("cancelled new call created receipt")
			}
		})
	}
}

func TestFullPreRegistryCancellationKeepsPriorUnknownReceipt(t *testing.T) {
	s := newLocal(t, newLedgerFixture(t), ReadWrite)
	operation := uuid.NewString()
	path, _ := s.service.operationPath(operation)
	if err := s.service.writePrivate(path, Operation{ID: operation, Tool: "create_member", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.CallFull(ctx, "create_member", []byte(`{"operationId":"`+operation+`","input":{"name":"Synthetic"}}`))
	var marker interface{ NotDispatched() bool }
	if err == nil || errors.As(err, &marker) {
		t.Fatalf("prior unknown receipt was marked unsubmitted: %v", err)
	}
	receipt, err := s.InspectFullOperation(operation)
	if err != nil || receipt.Status != "unknown" {
		t.Fatal("prior pending receipt lost", err)
	}
}

type cancelLoadedPlanRepository struct {
	settings.ConfigurationRepository
	planID                     string
	cancel                     context.CancelFunc
	planReads, operationWrites int
}

func (r *cancelLoadedPlanRepository) LoadConfiguration(key string) ([]byte, bool, error) {
	raw, found, err := r.ConfigurationRepository.LoadConfiguration(key)
	if err == nil && found && r.cancel != nil && strings.HasSuffix(key, ".plan."+r.planID) {
		r.planReads++
		r.cancel()
	}
	return raw, found, err
}
func (r *cancelLoadedPlanRepository) SaveConfiguration(key string, value []byte) error {
	if strings.Contains(key, ".operation.") {
		r.operationWrites++
	}
	return r.ConfigurationRepository.SaveConfiguration(key, value)
}
func TestFullCommitCancelledWhileLoadingValidPreview(t *testing.T) {
	for _, prior := range []bool{false, true} {
		t.Run(fmt.Sprint("prior=", prior), func(t *testing.T) {
			directory := t.TempDir()
			app, db := openLocalApp(t, filepath.Join(directory, "synthetic.db"))
			fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
			repository := &cancelLoadedPlanRepository{ConfigurationRepository: sqlite.NewConfigurationRepository(db)}
			s, err := NewInProcessSession(t.Context(), app, directory, uuid.NewString(), ReadWrite, nil, repository)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			preview := localData(t, s, "preview_batch", singleLedgerInput(map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}), false)
			planID := preview["planId"].(string)
			operation := uuid.NewString()
			if prior {
				path, _ := s.service.operationPath(operation)
				if err := s.service.writePrivate(path, Operation{ID: operation, Tool: "commit_batch", Status: "pending"}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := app.ListActivities(t.Context(), 100)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repository.planID, repository.cancel = planID, cancel
			repository.operationWrites = 0
			_, err = s.CallFull(ctx, "commit_plan", mustJSON(t, map[string]any{"operationId": operation, "input": map[string]any{"planId": planID}}))
			var marker interface{ NotDispatched() bool }
			proof := errors.As(err, &marker) && marker.NotDispatched()
			if err == nil || proof == prior || repository.planReads != 1 || repository.operationWrites != 0 {
				t.Fatalf("valid-plan cancellation proof=%v prior=%v reads=%d writes=%d err=%v", proof, prior, repository.planReads, repository.operationWrites, err)
			}
			after, err := app.ListActivities(t.Context(), 100)
			if err != nil || len(after) != len(before) {
				t.Fatal("cancelled valid plan posted activities", err)
			}
			receipt, lookup := s.InspectFullOperation(operation)
			if prior && (lookup != nil || receipt.Status != "unknown") {
				t.Fatal("old pending commit lost", lookup)
			}
			if !prior && lookup == nil {
				t.Fatal("new pending receipt unexpectedly saved")
			}
		})
	}
}
