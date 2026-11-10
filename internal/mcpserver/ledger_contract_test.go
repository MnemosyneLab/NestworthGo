package mcpserver

import (
	"errors"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestLedgerContractDiagnostics(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadWrite)
	base := map[string]any{"kind": "money_added", "accountId": "synthetic", "amount": "10", "currency": "CNY", "reason": "income"}
	cases := []struct {
		name        string
		patch       map[string]any
		field, rule string
	}{
		{"salary category", map[string]any{"reason": "PRIVATE 工资"}, "commands[0].reason", "enum"},
		{"mixed times", map[string]any{"effectiveAt": "2026-09-01T12:00:00Z", "effectiveLocalDate": "2026-09-01", "effectiveLocalTime": "12:00"}, "commands[0]", "forbidden_combination"},
		{"irrelevant fee", map[string]any{"fee": "1", "feeCurrency": "CNY"}, "commands[0]", "unexpected_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{}
			for k, v := range base {
				input[k] = v
			}
			for k, v := range tc.patch {
				input[k] = v
			}
			raw, _ := jsoncodec.Marshal(singleLedgerInput(input))
			err := registry.ValidateArguments("preview_batch", raw)
			var detail agentcapability.ArgumentError
			if !errors.As(err, &detail) || detail.Field != tc.field || detail.Rule != tc.rule {
				t.Fatalf("diagnostic=%+v error=%v", detail, err)
			}
			safe, _ := jsoncodec.Marshal(detail)
			if strings.Contains(string(safe), "PRIVATE") {
				t.Fatal("echoed submitted value")
			}
		})
	}
	valid, _ := jsoncodec.Marshal(singleLedgerInput(base))
	if err := registry.ValidateArguments("preview_batch", valid); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"commands":[{"kind":"trade","settlementAccountId":"a","instrumentId":"i","quantity":"1","gross":"1","grossCurrency":"USD","side":"purchase"}]}`,
		`{"commands":[{"kind":"cash_transfer","fromAccountId":"a","toAccountId":"b","sent":"1","sentCurrency":"USD","received":"1","receivedCurrency":"USD","feeCurrency":"USD"}]}`,
		`{"commands":[{"kind":"debt_payment","debtAccountId":"a","cashAccountId":"b","principal":"1","principalCurrency":"USD","interestOrFeeCurrency":"USD"}]}`,
	} {
		if err := registry.ValidateArguments("preview_batch", []byte(input)); err == nil {
			t.Fatal("invalid contract accepted", input)
		}
	}
	for _, input := range []string{
		`{"action":"fix","activityId":"a"}`,
		`{"action":"fix","activityId":"a","replacement":null}`,
		`{"action":"undo","activityId":"a","replacement":{"kind":"money_added"}}`,
		`{"action":"unknown","activityId":"a"}`,
		`{"action":"fix","activityId":"a","replacement":{"kind":"money_added","accountId":"a","amount":"1","currency":"USD","reason":"income","effectiveAt":"2026-09-01T12:00:00Z"}}`,
	} {
		if err := registry.ValidateArguments("preview_correction", []byte(input)); err == nil {
			t.Fatal("invalid correction accepted", input)
		}
	}
}
