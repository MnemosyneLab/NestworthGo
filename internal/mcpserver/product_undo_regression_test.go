package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

func TestProductHTTPInterestAndSettlementUndoPreserveDietzAndAttribution(t *testing.T) {
	type result struct{ amount, rate, capital string }
	run := func(treated string) map[string]result {
		now := time.Date(2026, 9, 20, 11, 0, 0, 0, time.UTC)
		fx := newLedgerFixtureWithClock(t, func() time.Time { return now })
		c := ledgerSession(t, fx)
		fund := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "2000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T08:00:00Z"})
		ledgerCommit(t, c, uuid.NewString(), fund["planId"].(string))
		open := ProductOperationInput{Kind: "open", Open: &application.OpenProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "1000", EffectiveAt: "2026-09-20T09:00:00Z", Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Deposit", StartOn: "2026-09-20", MaturityOn: productString("2026-09-30"), InterestMode: "none"}, Policy: productPolicy("2026-09-30")}}
		p := productPlanHTTP(t, c, open)
		opened := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
		quotes := quote.NewService(fx.app)
		if _, err := quotes.AppendManualInstrumentQuote(t.Context(), fx.instrument, "100", "2026-09-20T10:00:00Z", false); err != nil {
			t.Fatal(err)
		}
		buy := ledgerPreview(t, c, map[string]any{"kind": "trade", "side": "buy", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "5", "gross": "500", "grossCurrency": "USD", "effectiveAt": "2026-09-20T10:01:00Z"})
		ledgerCommit(t, c, uuid.NewString(), buy["planId"].(string))
		if treated != "" {
			now = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
			input := ProductOperationInput{Kind: "receive_interest", ReceiveInterest: &application.ReceiveInterestCommand{ProductID: opened["productIds"].([]any)[0].(string), Amount: "100"}}
			if treated == "settle" {
				input = ProductOperationInput{Kind: "settle", Settle: &SettleProductInput{ProductID: opened["productIds"].([]any)[0].(string), ReturnedPrincipal: productString("1000"), Interest: productString("100"), Fee: productString("5")}}
			}
			i := productPlanHTTP(t, c, input)
			received := productCommitHTTP(t, c, i["planId"].(string), uuid.NewString())
			now = time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
			u := productPlanHTTP(t, c, ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: received["operationId"].(string)}})
			productCommitHTTP(t, c, u["planId"].(string), uuid.NewString())
		}
		now = time.Date(2026, 9, 21, 23, 0, 0, 0, time.UTC)
		if _, err := quotes.AppendManualInstrumentQuote(t.Context(), fx.instrument, "110", "2026-09-21T23:00:00Z", false); err != nil {
			t.Fatal(err)
		}
		now = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
		if batchCash(t, c, fx.brokerage) != "500" {
			t.Fatal("cash did not return to baseline")
		}
		productAssertNetWorth(t, c, "2050")
		results := map[string]result{}
		for _, scope := range []domain.AnalysisScope{{Kind: domain.ScopeHousehold}, {Kind: domain.ScopeAccount, ID: fx.brokerage}} {
			q := domain.AnalysisQuery{Scope: scope, From: "2026-09-21", To: "2026-09-21", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
			analysis, err := fx.app.Analyze(t.Context(), q)
			if err != nil {
				t.Fatal(err)
			}
			if analysis.ReturnAmount == nil || analysis.ReturnRate == nil || len(analysis.DailyReturns) != 1 || analysis.DailyReturns[0].InvestedCapital == nil {
				t.Fatal(analysis)
			}
			for _, day := range analysis.Days {
				if residual, ok := day.AssetBuckets[domain.BucketResidual]; ok && !residual.IsZero() {
					t.Fatalf("%s residual=%s", treated, residual.CanonicalAmount())
				}
				if day.Component.Cash {
					if flow, ok := day.AssetBuckets[domain.BucketExternalFlow]; ok && !flow.IsZero() {
						t.Fatal("interest inverse labeled as external asset flow", flow)
					}
				}
				if len(day.DietzCapitalFlows) != 0 || !day.DietzFlow.IsZero() {
					t.Fatalf("treated=%s scope=%s: interest undo became capital: %+v", treated, scope.Kind, day.DietzCapitalFlows)
				}
			}
			actual := result{analysis.ReturnAmount.CanonicalAmount(), analysis.ReturnRate.String(), analysis.DailyReturns[0].InvestedCapital.CanonicalAmount()}
			if actual != (result{"50", "0.025", "2000"}) {
				t.Fatalf("treated=%s scope=%s got=%+v", treated, scope.Kind, actual)
			}
			http := call(t, c, "analyze_period", map[string]any{"query": map[string]any{"from": "2026-09-21", "to": "2026-09-21", "scopeKind": string(scope.Kind), "scopeId": scope.ID, "valuation": "base", "basis": "investment", "includeCash": true}}, false)["data"].(map[string]any)["investmentReturns"].(map[string]any)
			if http["amount"].(map[string]any)["amount"] != actual.amount || http["rate"] != actual.rate {
				t.Fatal(http)
			}
			results[string(scope.Kind)] = actual
		}

		detail, err := fx.app.Product(t.Context(), domain.ProductContractID(opened["productIds"].([]any)[0].(string)))
		if err != nil {
			t.Fatal(err)
		}
		for _, scope := range []domain.AnalysisScope{{Kind: domain.ScopeInstrument, ID: fx.instrument}, {Kind: domain.ScopeInstrument, ID: detail.Contract.InstrumentID.String()}} {
			for _, includeCash := range []bool{false, true} {
				q := domain.AnalysisQuery{Scope: scope, From: "2026-09-21", To: "2026-09-21", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: includeCash}
				a, err := fx.app.Analyze(t.Context(), q)
				if err != nil {
					t.Fatal(err)
				}
				wantAmount, wantRate, wantCapital := "50", "0.1", "500"
				if scope.ID == detail.Contract.InstrumentID.String() {
					wantAmount, wantRate, wantCapital = "0", "0", "1000"
					if treated == "settle" {
						wantCapital = "750"
					}
				}
				if a.ReturnAmount == nil || a.ReturnRate == nil || len(a.DailyReturns) != 1 || a.DailyReturns[0].InvestedCapital == nil {
					t.Fatalf("%s instrument %s incomplete", treated, scope.ID)
				}
				if a.ReturnAmount.CanonicalAmount() != wantAmount || a.ReturnRate.String() != wantRate || a.DailyReturns[0].InvestedCapital.CanonicalAmount() != wantCapital {
					t.Fatalf("%s instrument cash=%v amount=%s rate=%s capital=%s", treated, includeCash, a.ReturnAmount.CanonicalAmount(), a.ReturnRate.String(), a.DailyReturns[0].InvestedCapital.CanonicalAmount())
				}
				for _, d := range a.Days {
					for _, b := range []domain.AttributionBucket{domain.BucketResidual, domain.BucketDividendInterest, domain.BucketFee, domain.BucketExternalFlow} {
						if v, ok := d.AssetBuckets[b]; ok && !v.IsZero() {
							t.Fatalf("%s instrument bucket=%s amount=%s", treated, b, v.CanonicalAmount())
						}
					}
					for _, r := range []domain.ReturnComponent{domain.ReturnDividendInterest, domain.ReturnInvestmentFee} {
						if v, ok := d.ReturnComponents[r]; ok && !v.IsZero() {
							t.Fatalf("%s instrument return=%s amount=%s", treated, r, v.CanonicalAmount())
						}
					}
				}
			}
		}
		return results
	}
	baseline := run("")
	for _, kind := range []string{"receive_interest", "settle"} {
		productJSONEqual(t, run(kind), baseline)
	}
}

func TestProductHTTPExistingUndoWithActiveReservationRequiresGUI(t *testing.T) {
	for _, kind := range []string{"open", "record_existing"} {
		t.Run(kind, func(t *testing.T) {
			fx := newLedgerFixture(t)
			c := ledgerSession(t, fx)
			input := productExistingInput(fx.brokerage)
			if kind == "open" {
				f := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-28T12:00:00Z"})
				ledgerCommit(t, c, uuid.NewString(), f["planId"].(string))
				input = ProductOperationInput{Kind: "open", Open: &application.OpenProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "1000", Terms: input.RecordExisting.Terms, Policy: input.RecordExisting.Policy}}
			}
			p := productPlanHTTP(t, c, input)
			posted := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
			product, err := fx.app.Product(t.Context(), domain.ProductContractID(posted["productIds"].([]any)[0].(string)))
			if err != nil {
				t.Fatal(err)
			}
			reserve, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(product.Contract.AccountID, product.Contract.HoldingID), Label: "Cannot silently release", Amount: "100"})
			if err != nil {
				t.Fatal(err)
			}
			count := ledgerActivityCount(t, fx.app)
			status, raw := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "preview_product_operation", "arguments": ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: posted["operationId"].(string)}}}}, "")
			if status != 200 || !containsProductReservationGUIError(raw) {
				t.Fatalf("missing GUI recovery contract: %s", raw)
			}
			current, err := fx.app.Product(t.Context(), product.Contract.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Contract.Revision != product.Contract.Revision || current.Contract.State != domain.ProductStateOpen || len(current.Reservations) != 1 || current.Reservations[0].Revision != reserve.Revision || ledgerActivityCount(t, fx.app) != count {
				t.Fatal("rejected undo changed facts", current)
			}
		})
	}
}

func containsProductReservationGUIError(raw []byte) bool {
	return strings.Contains(string(raw), "unresolved_reservation_release") && strings.Contains(string(raw), "GUI")
}
