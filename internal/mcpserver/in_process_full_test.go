package mcpserver

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestFullRegistryParityAndDurableDirectoryReceipt(t *testing.T) {
	fx := newLedgerFixture(t)
	dir, id := t.TempDir(), uuid.NewString()
	s, err := NewInProcessSession(t.Context(), fx.app, dir, id, LedgerWrite, nil)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := s.FullDefinitions()
	if err != nil || !reflect.DeepEqual(defs, s.service.buildToolSetWithPolicy(LedgerWrite, true, true).Definitions()) {
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
	if len(defs) != len(golden[LedgerWrite])+len(investmentReviewManagementTools) {
		t.Fatal("Full inventory count drift")
	}
	for _, d := range defs {
		if hash, exists := golden[LedgerWrite][d.Name]; exists && hash != contractHash(t, sdkTool(d)) {
			t.Fatalf("Full contract drift: %s", d.Name)
		}
	}
	for _, name := range investmentReviewManagementTools {
		d, err := s.FullDefinition(name)
		if err != nil || d.Effect != agentcapability.Write {
			t.Fatalf("missing Full review management writer: %s", name)
		}
		if _, err := s.registry.Definition(name); err == nil {
			t.Fatal("Full changed ordinary in-process inventory")
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
	s, err = NewInProcessSession(t.Context(), fx.app, dir, id, LedgerWrite, nil)
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
	s := newLocal(t, newLedgerFixture(t), LedgerWrite)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.CallFull(ctx, "create_member", []byte(`{"operationId":"`+uuid.NewString()+`","input":{"name":"Synthetic cancelled"}}`))
	var marker interface{ NotDispatched() bool }
	if !errors.As(err, &marker) || !marker.NotDispatched() {
		t.Fatalf("missing pre-dispatch evidence: %v", err)
	}
}
