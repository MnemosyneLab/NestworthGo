package mcpserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestTypedDecodeRejectionHasNoReceiptAndCanBeCorrected(t *testing.T) {
	fx := newLedgerFixture(t)
	local := newLocal(t, fx, ReadWrite)
	external := ledgerSession(t, fx)
	for _, literal := range []string{"1.0", "1e0", "9223372036854775808"} {
		t.Run(literal, func(t *testing.T) {
			localID, externalID := uuid.NewString(), uuid.NewString()
			args := func(id, number string) []byte {
				return []byte(`{"operationId":"` + id + `","input":{"id":"` + fx.instrument + `","changes":{"sortOrder":` + number + `}}}`)
			}
			if err := local.ValidateFull("update_instrument", args(localID, literal)); !errors.Is(err, agentcapability.ErrInvalidArguments) {
				t.Fatalf("preflight did not check typed decoding: %v", err)
			}
			_, err := local.CallFull(t.Context(), "update_instrument", args(localID, literal))
			var marker interface{ NotDispatched() bool }
			if !errors.Is(err, agentcapability.ErrInvalidArguments) || !errors.As(err, &marker) || !marker.NotDispatched() {
				t.Fatalf("typed rejection lacks pre-dispatch proof: %v", err)
			}
			if _, err := local.InspectFullOperation(localID); err == nil {
				t.Fatal("typed rejection created a receipt")
			}
			reply, err := external.CallTool(t.Context(), &mcp.CallToolParams{Name: "update_instrument", Arguments: json.RawMessage(args(externalID, literal))})
			if err != nil || reply == nil || !reply.IsError || len(reply.Content) != 1 {
				t.Fatalf("HTTP typed rejection: %v, %v", reply, err)
			}
			text := reply.Content[0].(*mcp.TextContent).Text
			if !strings.Contains(text, `"rule":"typed_decode"`) || !strings.Contains(text, "integer notation") {
				t.Fatalf("HTTP repair diagnostic lost: %s", text)
			}
			if _, err := fx.service.GetOperation(externalID); err == nil {
				t.Fatal("HTTP typed rejection created a receipt")
			}
			instruments, err := fx.app.ListInstruments(t.Context(), false)
			if err != nil || len(instruments) != 1 || instruments[0].SortOrder != 0 {
				t.Fatalf("rejected input mutated the instrument: %+v, %v", instruments, err)
			}
			// The same fresh intent is safe to correct because neither adapter
			// entered business dispatch or wrote an ambiguous receipt.
			if _, err := local.CallFull(t.Context(), "update_instrument", args(localID, "7")); err != nil {
				t.Fatalf("corrected local write blocked: %v", err)
			}
			if receipt, err := local.InspectFullOperation(localID); err != nil || receipt.Status != "succeeded" {
				t.Fatalf("corrected local receipt: %+v, %v", receipt, err)
			}
			call(t, external, "update_instrument", json.RawMessage(args(externalID, "0")), false)
			if receipt, err := fx.service.GetOperation(externalID); err != nil || receipt.Status != "succeeded" {
				t.Fatalf("corrected HTTP receipt: %+v, %v", receipt, err)
			}
		})
	}
}

func TestBusinessDiagnosticParityBetweenLocalAndHTTP(t *testing.T) {
	fx := newLedgerFixture(t)
	local := newLocal(t, fx, ReadOnly)
	external := ledgerSession(t, fx)
	for _, tc := range []struct {
		filter  map[string]any
		message string
	}{
		{map[string]any{"fromDate": "2026/09/01"}, "observation dates must be YYYY-MM-DD"},
		{map[string]any{"fromDate": "2026-09-29", "toDate": "2026-09-01"}, "observation date range must be forward"},
	} {
		args := map[string]any{"filter": tc.filter}
		_, localErr := local.Call(t.Context(), "list_agent_market_data", mustJSON(t, args))
		reply, err := external.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_agent_market_data", Arguments: args})
		if localErr == nil || err != nil || reply == nil || !reply.IsError {
			t.Fatalf("expected matching business failures: local=%v HTTP=%v transport=%v", localErr, reply, err)
		}
		wire := safeError(localErr)
		var httpWire struct{ Code, Field, Message string }
		if err := jsoncodec.Unmarshal([]byte(reply.Content[0].(*mcp.TextContent).Text), &httpWire); err != nil || wire.Code != httpWire.Code || wire.Field != httpWire.Field || wire.Message != tc.message || httpWire.Message != tc.message {
			t.Fatalf("business diagnostic drift: local=%+v HTTP=%+v err=%v", wire, httpWire, err)
		}
	}
}

func TestTypedDecodeRejectionPreservesPriorUnknownReceipt(t *testing.T) {
	fx := newLedgerFixture(t)
	local := newLocal(t, fx, ReadWrite)
	operationID := uuid.NewString()
	path, err := local.service.operationPath(operationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.service.writePrivate(path, Operation{ID: operationID, Tool: "update_instrument", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	_, err = local.CallFull(t.Context(), "update_instrument", []byte(`{"operationId":"`+operationID+`","input":{"id":"`+fx.instrument+`","changes":{"sortOrder":1.0}}}`))
	var marker interface{ NotDispatched() bool }
	if !errors.Is(err, agentcapability.ErrInvalidArguments) || errors.As(err, &marker) {
		t.Fatalf("new decode rejection erased older uncertainty: %v", err)
	}
	if receipt, err := local.InspectFullOperation(operationID); err != nil || receipt.Status != "unknown" {
		t.Fatalf("prior unknown receipt changed: %+v, %v", receipt, err)
	}
}
