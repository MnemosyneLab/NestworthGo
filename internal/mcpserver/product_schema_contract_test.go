package mcpserver

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"strings"
	"testing"
)

func TestProductSchemaConstraintsAndDefaults(t *testing.T) {
	resolved, err := productOperationInputSchema().Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(map[string]any)
		valid bool
	}{
		{"defaults", func(p map[string]any) {
			policy := p["policy"].(map[string]any)
			policy["accessKind"] = ""
			policy["earlyKind"] = ""
			policy["unlockOn"] = nil
		}, true},
		{"empty local defaults", func(p map[string]any) {
			p["effectiveAt"] = "2026-09-01T12:00:00Z"
			p["effectiveLocalDate"] = ""
			p["effectiveLocalTime"] = ""
		}, true},
		{"local pair", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01"; p["effectiveLocalTime"] = "12:00" }, true},
		{"conflicting time", func(p map[string]any) {
			p["effectiveAt"] = "2026-09-01T12:00:00Z"
			p["effectiveLocalDate"] = "2026-09-01"
			p["effectiveLocalTime"] = "12:00"
		}, false},
		{"missing local clock", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01" }, false},
		{"empty local clock", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01"; p["effectiveLocalTime"] = "" }, false},
		{"interest enum", func(p map[string]any) { p["terms"].(map[string]any)["interestMode"] = "invented" }, false},
		{"maturity required", func(p map[string]any) { p["terms"].(map[string]any)["maturityOn"] = nil }, false},
		{"policy enum", func(p map[string]any) { p["policy"].(map[string]any)["accessKind"] = "invented" }, false},
		{"settlement maximum", func(p map[string]any) { p["policy"].(map[string]any)["settlementDays"] = 366 }, false},
		{"settlement minimum", func(p map[string]any) { p["policy"].(map[string]any)["settlementDays"] = -1 }, false},
		{"settlement basis", func(p map[string]any) { p["policy"].(map[string]any)["dayBasis"] = nil }, false},
		{"early requirements", func(p map[string]any) { p["policy"].(map[string]any)["earlyKind"] = "allowed" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := productPayload("open")
			tc.edit(p)
			input := map[string]any{"kind": "open", "open": p}
			err := resolved.Validate(&input)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestNumericLedgerMoneyDiagnostic(t *testing.T) {
	s, _, _ := fixture(t)
	r := s.toolRegistry(ReadWrite)
	err := r.ValidateArguments("preview_batch", []byte(`{"commands":[{"kind":"money_added","accountId":"a","amount":15708.56,"currency":"CNY","reason":"income"}]}`))
	var detail agentcapability.ArgumentError
	if !errors.As(err, &detail) || detail.Field != "commands[0].amount" || detail.Rule != "type" {
		t.Fatalf("wrong diagnostic: %v %+v", err, detail)
	}
}

func TestProductInterestContract(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadWrite)
	for _, tc := range []struct {
		name, mode string
		fields     map[string]any
		valid      bool
	}{
		{"none", "none", map[string]any{"maturityInterest": nil}, true},
		{"none blank decimals", "none", map[string]any{"maturityInterest": " \t", "annualRate": "\u3000"}, true},
		{"none rate forbidden", "none", map[string]any{"maturityInterest": nil, "annualRate": "0.02"}, false},
		{"none interest forbidden", "none", nil, false},
		{"none paid through forbidden", "none", map[string]any{"maturityInterest": nil, "interestPaidThroughOn": "2026-09-01"}, false},
		{"manual", "manual_maturity_amount", nil, true},
		{"manual missing amount", "manual_maturity_amount", map[string]any{"maturityInterest": nil}, false},
		{"manual blank amount", "manual_maturity_amount", map[string]any{"maturityInterest": " \t"}, false},
		{"manual numeric amount", "manual_maturity_amount", map[string]any{"maturityInterest": 50}, false},
		{"manual percent forbidden", "manual_maturity_amount", map[string]any{"annualRatePercent": "2"}, false},
		{"manual paid through forbidden", "manual_maturity_amount", map[string]any{"interestPaidThroughOn": "2026-09-01"}, false},
		{"simple ratio", "simple_act_365", map[string]any{"maturityInterest": nil, "annualRate": "0.02", "interestPaidThroughOn": "2026-09-01"}, true},
		{"simple percent", "simple_act_360", map[string]any{"maturityInterest": nil, "annualRatePercent": "2", "interestPaidThroughOn": "2026-09-01"}, true},
		{"percent precedence", "simple_act_365", map[string]any{"maturityInterest": nil, "annualRatePercent": "2", "annualRate": "ignored", "interestPaidThroughOn": "2026-09-01"}, true},
		{"simple missing rate", "simple_act_365", map[string]any{"maturityInterest": nil, "interestPaidThroughOn": "2026-09-01"}, false},
		{"simple blank paid through", "simple_act_365", map[string]any{"maturityInterest": nil, "annualRate": "0.02", "interestPaidThroughOn": ""}, false},
		{"simple missing paid through", "simple_act_365", map[string]any{"maturityInterest": nil, "annualRate": "0.02"}, false},
		{"simple interest forbidden", "simple_act_365", map[string]any{"annualRate": "0.02", "interestPaidThroughOn": "2026-09-01"}, false},
		{"simple locked product forbidden", "simple_act_365", map[string]any{"kind": "locked_product", "maturityInterest": nil, "annualRate": "0.02", "interestPaidThroughOn": "2026-09-01"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := productPayload("open")
			terms := p["terms"].(map[string]any)
			terms["interestMode"] = tc.mode
			for k, v := range tc.fields {
				terms[k] = v
			}
			for _, tool := range []string{"preview_product_operation", "preview_product_terms"} {
				input := map[string]any{"kind": "open", "open": p}
				if tool == "preview_product_terms" {
					input = map[string]any{"productId": "p", "expectedRevision": 1, "terms": terms, "policy": p["policy"]}
				}
				raw, _ := jsoncodec.Marshal(input)
				err := registry.ValidateArguments(tool, raw)
				if tc.name == "simple missing rate" {
					var detail agentcapability.ArgumentError
					want := "terms.annualRate"
					if tool == "preview_product_operation" {
						want = "open." + want
					}
					if !errors.As(err, &detail) || detail.Field != want {
						t.Fatalf("missing rate diagnostic: %#v", detail)
					}
				}
				valid := tc.valid
				if tc.name == "simple missing paid through" && tool == "preview_product_operation" {
					valid = true
				}
				if (err == nil) != valid {
					t.Fatalf("%s valid=%v: %#v", tool, valid, err)
				}
			}
		})
	}
}

func TestNumericProductMoneyDiagnostic(t *testing.T) {
	s, _, _ := fixture(t)
	r := s.toolRegistry(ReadWrite)
	for _, field := range []string{"principal", "maturityInterest", "openingFee"} {
		p := productPayload("open")
		want := "open.principal"
		if field == "principal" || field == "openingFee" {
			want = "open." + field
			p[field] = 123.45
		} else {
			p["terms"].(map[string]any)[field] = 123.45
			want = "open.terms.maturityInterest"
		}
		raw, _ := jsoncodec.Marshal(map[string]any{"kind": "open", "open": p})
		var detail agentcapability.ArgumentError
		err := r.ValidateArguments("preview_product_operation", raw)
		if !errors.As(err, &detail) || detail.Field != want || detail.Rule != "type" {
			t.Fatalf("%s: %#v", field, detail)
		}
	}
}

func TestBlankDecimalMatchesTrimmedOptionalInputs(t *testing.T) {
	r, err := blankDecimal().Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"", " ", "\t\n\v\f\r", "\u0085\u00a0\u1680\u2000\u200a\u2028\u2029\u202f\u205f\u3000", "n", "t", "u0085", "0", " 2 ", "\ufeff"} {
		var value any = text
		if (r.Validate(&value) == nil) != (strings.TrimSpace(text) == "") {
			t.Fatalf("blank decimal differs from trimming: %q", text)
		}
	}
}
