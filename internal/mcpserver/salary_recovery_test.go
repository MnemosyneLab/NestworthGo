package mcpserver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestSalaryArgumentRecoveryAndExactTime(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, time.Now)
	local := newLocal(t, fx, ReadWrite)
	client := ledgerSession(t, fx)
	command := map[string]any{"accountId": fx.checking, "amount": "15708.56", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T15:14:26+08:00"}
	for _, kind := range []string{"income", "cash_in"} {
		command["kind"] = kind
		input := singleLedgerInput(command)
		raw, _ := jsoncodec.Marshal(input)
		_, err := local.CallFull(t.Context(), "preview_batch", raw)
		var detail agentcapability.ArgumentError
		if !errors.As(err, &detail) || detail.Field != "commands[0].kind" || detail.Rule != "enum" || !strings.Contains(strings.Join(detail.Expected, ","), "money_added") {
			t.Fatalf("missing repair: %#v %v", detail, err)
		}
		wire, callErr := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "preview_batch", Arguments: input})
		if callErr != nil || !wire.IsError {
			t.Fatalf("expected MCP refusal: %v %v", wire, callErr)
		}
		encoded, _ := jsoncodec.Marshal(wire)
		if !strings.Contains(string(encoded), "commands[0].kind") || !strings.Contains(string(encoded), "money_added") {
			t.Fatalf("MCP lost diagnostic: %s", encoded)
		}
	}
	command["kind"] = "money_added"
	delete(command, "effectiveAt")
	command["effectiveLocalDate"] = "2026-09-28"
	command["effectiveLocalTime"] = "15:14:26"
	input := singleLedgerInput(command)
	raw, _ := jsoncodec.Marshal(input)
	_, err := local.CallFull(t.Context(), "preview_batch", raw)
	detail := safeError(err)
	if detail.Field != "commands[0].effectiveLocalTime" || !strings.Contains(detail.Message, "RFC3339") || !strings.Contains(detail.Message, "HH:MM") {
		t.Fatalf("missing time repair: %+v", detail)
	}
	wire, callErr := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "preview_batch", Arguments: input})
	if callErr != nil || !wire.IsError {
		t.Fatalf("expected MCP refusal: %v %v", wire, callErr)
	}
	encoded, _ := jsoncodec.Marshal(wire)
	if !strings.Contains(wire.Content[0].(*mcp.TextContent).Text, detail.Message) {
		t.Fatalf("MCP time repair differs: %s", encoded)
	}
	delete(command, "effectiveLocalDate")
	delete(command, "effectiveLocalTime")
	command["effectiveAt"] = "2026-09-28T15:14:26+08:00"
	raw, _ = jsoncodec.Marshal(singleLedgerInput(command))
	if _, err := local.CallFull(t.Context(), "preview_batch", raw); err != nil {
		t.Fatalf("corrected built-in preview: %v", err)
	}
	preview := ledgerPreview(t, client, command)
	if ledgerActivityCount(t, fx.app) != 0 || ledgerCash(t, client, fx.checking) != "0" {
		t.Fatal("preview or rejection wrote ledger")
	}
	result := ledgerCommit(t, client, uuid.NewString(), preview["planId"].(string))
	at, err := time.Parse(time.RFC3339Nano, result["activity"].(map[string]any)["effectiveAt"].(string))
	if err != nil || !at.Equal(time.Date(2026, 9, 28, 7, 14, 26, 0, time.UTC)) {
		t.Fatalf("seconds or offset lost: %v %v", at, err)
	}
	ledgerCommit(t, client, uuid.NewString(), preview["planId"].(string))
	if ledgerActivityCount(t, fx.app) != 1 || ledgerCash(t, client, fx.checking) != "15708.56" {
		t.Fatal("wrong amount or duplicate posting")
	}
}
