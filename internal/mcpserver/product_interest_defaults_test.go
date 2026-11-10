package mcpserver

import (
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestProductHTTPInterestDateDefaults(t *testing.T) {
	for _, kind := range []string{"open", "record_existing", "renew"} {
		for _, mode := range []string{"simple_act_365", "simple_act_360"} {
			for _, omit := range []bool{false, true} {
				name := kind + "/" + mode + "/null"
				if omit {
					name = kind + "/" + mode + "/omitted"
				}
				t.Run(name, func(t *testing.T) {
					fx := newLedgerFixture(t)
					c := ledgerSession(t, fx)
					base := productExistingInput(fx.brokerage)
					terms := base.RecordExisting.Terms
					terms.InterestMode = mode
					terms.MaturityInterest = nil
					terms.AnnualRate = productString("0.02")
					terms.InterestPaidThroughOn = nil
					in := ProductOperationInput{Kind: kind}
					key := kind
					switch kind {
					case "record_existing":
						in = base
						in.RecordExisting.Terms = terms
						key = "recordExisting"
					case "open":
						fund := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1500", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-28T12:00:00Z"})
						ledgerCommit(t, c, uuid.NewString(), fund["planId"].(string))
						in.Open = &application.OpenProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "1000", Terms: terms, Policy: base.RecordExisting.Policy}
					case "renew":
						oldPlan := productPlanHTTP(t, c, base)
						old := productCommitHTTP(t, c, oldPlan["planId"].(string), uuid.NewString())
						oldID := old["productIds"].([]any)[0].(string)
						in.Renew = &RenewProductInput{Principal: "1000", Terms: terms, Policy: base.RecordExisting.Policy, Settle: SettleProductInput{ProductID: oldID, ReturnedPrincipal: productString("1000")}}
					}
					raw, _ := jsoncodec.Marshal(in)
					var input map[string]any
					if err := jsoncodec.Unmarshal(raw, &input); err != nil {
						t.Fatal(err)
					}
					if omit {
						delete(input[key].(map[string]any)["terms"].(map[string]any), "interestPaidThroughOn")
					}
					preview := call(t, c, "preview_product_operation", input, false)["data"].(map[string]any)
					result := productCommitHTTP(t, c, preview["planId"].(string), uuid.NewString())
					ids := result["productIds"].([]any)
					detail, err := fx.app.Product(t.Context(), domain.ProductContractID(ids[len(ids)-1].(string)))
					if err != nil {
						t.Fatal(err)
					}
					paid := detail.Contract.InterestPaidThroughOn
					if paid == nil || *paid != terms.StartOn {
						t.Fatalf("paid-through date did not default to start: %v", paid)
					}
				})
			}
		}
	}
}

func TestTermsReplacementDoesNotDefaultInterestDate(t *testing.T) {
	fx := newLedgerFixture(t)
	registry := fx.service.toolRegistry(ReadWrite)
	for _, omit := range []bool{false, true} {
		input := productTermsInput("synthetic-product", 1, "term_deposit")
		input.Terms.InterestPaidThroughOn = nil
		raw, _ := jsoncodec.Marshal(input)
		var value map[string]any
		_ = jsoncodec.Unmarshal(raw, &value)
		if omit {
			delete(value["terms"].(map[string]any), "interestPaidThroughOn")
		}
		raw, _ = jsoncodec.Marshal(value)
		if err := registry.ValidateArguments("preview_product_terms", raw); err == nil {
			t.Fatalf("terms replacement accepted missing date (omitted=%v)", omit)
		}
	}
}
