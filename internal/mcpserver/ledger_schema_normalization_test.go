package mcpserver

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestLedgerSchemaTimeNormalizationThroughPreviewAndHTTP(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, time.Now)
	registry := fx.service.toolRegistry(ReadWrite)
	client := ledgerSession(t, fx)
	for _, tc := range []struct {
		name   string
		fields map[string]any
		valid  bool
	}{
		{"timestamp", map[string]any{"effectiveAt": "2026-09-28T12:00:00Z"}, true},
		{"timestamp empty local pair", map[string]any{"effectiveAt": "2026-09-28T12:00:00Z", "effectiveLocalDate": "", "effectiveLocalTime": ""}, true},
		{"timestamp empty local date", map[string]any{"effectiveAt": "2026-09-28T12:00:00Z", "effectiveLocalDate": ""}, true},
		{"timestamp whitespace locals", map[string]any{"effectiveAt": "2026-09-28T12:00:00Z", "effectiveLocalDate": " \t", "effectiveLocalTime": "\u3000"}, true},
		{"local pair empty timestamp", map[string]any{"effectiveAt": "", "effectiveLocalDate": "2026-09-28", "effectiveLocalTime": "12:00"}, true},
		{"local pair whitespace timestamp", map[string]any{"effectiveAt": " \u00a0", "effectiveLocalDate": " 2026-09-28 ", "effectiveLocalTime": " 12:00 "}, true},
		{"omitted time", nil, true},
		{"blank time", map[string]any{"effectiveAt": "\u3000", "effectiveLocalDate": "\t"}, true},
		{"mixed time", map[string]any{"effectiveAt": "2026-09-28T12:00:00Z", "effectiveLocalDate": "2026-09-28", "effectiveLocalTime": "12:00"}, false},
		{"date only", map[string]any{"effectiveLocalDate": "2026-09-28"}, false},
		{"clock only", map[string]any{"effectiveLocalTime": "12:00"}, false},
		{"date blank clock", map[string]any{"effectiveLocalDate": "2026-09-28", "effectiveLocalTime": "\u3000"}, false},
		{"clock blank date", map[string]any{"effectiveLocalDate": " \t", "effectiveLocalTime": "12:00"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "10", "currency": "USD", "reason": "income"}
			for k, v := range tc.fields {
				command[k] = v
			}
			input := singleLedgerInput(command)
			raw, err := jsoncodec.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.ValidateArguments("preview_batch", raw); (err == nil) != tc.valid {
				t.Fatalf("schema valid=%v: %v", tc.valid, err)
			}
			var in BatchChangeInput
			if err := jsoncodec.Unmarshal(raw, &in); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.service.previewBatch(t.Context(), in); (err == nil) != tc.valid {
				t.Fatalf("preview valid=%v: %v", tc.valid, err)
			}
			if tc.valid {
				ledgerPreviewInput(t, client, input)
			} else if code := ledgerErrorCode(t, client, "preview_batch", input); code != "validation" {
				t.Fatalf("HTTP error=%s", code)
			}
		})
	}
	if cash := ledgerCash(t, client, fx.checking); cash != "0" {
		t.Fatalf("preview mutated cash: %s", cash)
	}
	if count := ledgerActivityCount(t, fx.app); count != 0 {
		t.Fatalf("preview recorded %d activities", count)
	}
}

func TestLedgerSchemaOptionalChargePairing(t *testing.T) {
	fx := newLedgerFixture(t)
	registry := fx.service.toolRegistry(ReadWrite)
	for _, kind := range []string{"trade", "debt_payment"} {
		for _, tc := range []struct {
			name             string
			amount, currency *string
			valid            bool
		}{
			{"omitted", nil, nil, true},
			{"empty amount", stringPointer(""), nil, true},
			{"empty currency", nil, stringPointer(""), true},
			{"both empty", stringPointer(""), stringPointer(""), true},
			{"paired", stringPointer("1"), stringPointer("USD"), true},
			{"amount only", stringPointer("1"), nil, false},
			{"currency only", nil, stringPointer("USD"), false},
			{"amount empty currency", stringPointer("1"), stringPointer(""), false},
			{"currency empty amount", stringPointer(""), stringPointer("USD"), false},
			{"whitespace is not empty charge", stringPointer(" \t"), nil, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				p := map[string]any{"kind": "trade", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "side": "buy", "quantity": "1", "gross": "1", "grossCurrency": "USD"}
				amount, currency := "fee", "feeCurrency"
				if kind == "debt_payment" {
					p = map[string]any{"kind": kind, "debtAccountId": fx.debt, "cashAccountId": fx.checking, "principal": "1", "principalCurrency": "USD"}
					amount, currency = "interestOrFee", "interestOrFeeCurrency"
				}
				if tc.amount != nil {
					p[amount] = *tc.amount
				}
				if tc.currency != nil {
					p[currency] = *tc.currency
				}
				raw, err := jsoncodec.Marshal(singleLedgerInput(p))
				if err != nil {
					t.Fatal(err)
				}
				if err := registry.ValidateArguments("preview_batch", raw); (err == nil) != tc.valid {
					t.Fatalf("schema valid=%v: %v", tc.valid, err)
				}
				var in BatchChangeInput
				if err := jsoncodec.Unmarshal(raw, &in); err != nil {
					t.Fatal(err)
				}
				if _, err := normalizeChangeRequest(in.Commands[0], time.Now()); (err == nil) != tc.valid {
					t.Fatalf("normalizer valid=%v: %v", tc.valid, err)
				}
				if kind == "trade" {
					p["holdingId"] = "synthetic"
				}
				correction, err := jsoncodec.Marshal(map[string]any{"action": "fix", "activityId": "synthetic", "replacement": p})
				if err != nil {
					t.Fatal(err)
				}
				if err := registry.ValidateArguments("preview_correction", correction); (err == nil) != tc.valid {
					t.Fatalf("correction schema valid=%v: %v", tc.valid, err)
				}
			})
		}
	}
}

func TestLedgerSchemaRequiresExplicitMoneyReason(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, time.Now)
	registry := fx.service.toolRegistry(ReadWrite)
	for _, kind := range []string{"money_added", "money_removed"} {
		for _, reason := range []any{nil, "", "other"} {
			p := map[string]any{"kind": kind, "accountId": fx.checking, "amount": "1", "currency": "USD", "effectiveAt": "2026-09-28T12:00:00Z"}
			if reason != nil {
				p["reason"] = reason
			}
			raw, err := jsoncodec.Marshal(singleLedgerInput(p))
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.ValidateArguments("preview_batch", raw); (err == nil) != (reason == "other") {
				t.Fatalf("%s reason=%v: %v", kind, reason, err)
			}
			var in BatchChangeInput
			if err := jsoncodec.Unmarshal(raw, &in); err != nil {
				t.Fatal(err)
			}
			// The shared adapter retains its UI default; the agent contract deliberately
			// asks callers to classify money movements instead of inferring a reason.
			origin, err := fx.app.HistoryOrigin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledgerCommand(in.Commands[0], origin); err != nil {
				t.Fatalf("shared adapter default: %v", err)
			}
		}
	}
}

func stringPointer(value string) *string { return &value }

func TestProductOperationPayloadNullRejectedBySchemaAndAdapter(t *testing.T) {
	fx := newLedgerFixture(t)
	registry := fx.service.toolRegistry(ReadWrite)
	for _, tc := range []struct{ kind, field string }{{"open", "open"}, {"record_existing", "recordExisting"}, {"receive_interest", "receiveInterest"}, {"settle", "settle"}, {"renew", "renew"}, {"undo", "undo"}} {
		t.Run(tc.kind, func(t *testing.T) {
			raw, err := jsoncodec.Marshal(map[string]any{"kind": tc.kind, tc.field: nil})
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.ValidateArguments("preview_product_operation", raw); err == nil {
				t.Fatal("schema accepted null payload")
			}
			var in ProductOperationInput
			if err := jsoncodec.Unmarshal(raw, &in); err != nil {
				t.Fatal(err)
			}
			if _, err := in.command(); err == nil {
				t.Fatal("adapter accepted null payload")
			}
		})
	}
}
