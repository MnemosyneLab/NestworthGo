package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func productString(value string) *string { return &value }

func productPolicy(maturity string) application.ProductPolicyInput {
	zero, calendar := 0, "calendar"
	return application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, SettlementDays: &zero, DayBasis: &calendar, EarlyKind: "not_allowed"}
}

// Seed only synthetic facts through the same GUI adapter used by the frontend.
// This PR exposes reads, so its HTTP tests must not claim MCP write coverage.
func guiProductPost(t *testing.T, fx ledgerFixture, command liquidity.ProductCommandRequest) liquidity.ProductOperationReceiptDTO {
	t.Helper()
	gui := liquidity.NewService(fx.app)
	preview, err := gui.PreviewProductOperation(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := gui.RecordProductOperation(t.Context(), liquidity.RecordProductOperationRequest{Command: command, MutationID: domain.NewProductOperationID().String(), ReviewedStateHash: preview.ReviewedStateHash})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func productJSONEqual(t *testing.T, actual, expected any) {
	t.Helper()
	encode := func(value any) any {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	if !reflect.DeepEqual(encode(actual), encode(expected)) {
		t.Fatalf("MCP/GUI mismatch:\n%v\n%v", actual, expected)
	}
}

func TestProductHTTPReadDiscoveryPermissionsAndSchemas(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			if _, err := fx.service.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, fx.service)
			listed, err := client.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, tool := range listed.Tools {
				if strings.Contains(tool.Name, "reservation") || strings.Contains(tool.Name, "product") && !isProductTool(tool.Name) {
					t.Fatalf("unexpected product/reservation write: %s", tool.Name)
				}
				if isProductTool(tool.Name) {
					found[tool.Name] = true
					if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
						t.Fatalf("incorrect read annotations: %+v", tool)
					}
					schema, _ := json.Marshal(tool.InputSchema)
					if !strings.Contains(string(schema), `"additionalProperties":false`) || strings.Contains(string(schema), "releaseReservationIds") {
						t.Fatalf("unsafe schema: %s", schema)
					}
				}
			}
			if len(found) != 4 {
				t.Fatal(found)
			}
			info := call(t, client, "get_context", Empty{}, false)["data"].(map[string]any)
			caps, _ := json.Marshal(info["capabilities"])
			if !strings.Contains(string(caps), "managed_product_read") || !strings.Contains(string(caps), "liquidity_read") || strings.Contains(string(caps), "product_preview") {
				t.Fatal(info)
			}
			instructions := client.InitializeResult().Instructions
			if !strings.Contains(instructions, "GUI permittedActions do not grant MCP permission") || !strings.Contains(instructions, "due_unconfirmed does not automatically record cash") {
				t.Fatal("missing product routing/boundary instructions")
			}
			call(t, client, "list_products", map[string]any{}, false)
			call(t, client, "get_liquidity_overview", map[string]any{}, false)
			for _, name := range []string{"preview_product_operation", "commit_product_operation", "update_product_terms", "append_product_valuation", "save_reservation", "release_reservation"} {
				_, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err == nil || !strings.Contains(err.Error(), "unknown tool") {
					t.Fatalf("write unexpectedly exposed in %s: %s: %v", mode, name, err)
				}
			}
		})
	}
}

func TestProductHTTPGUIReadLifecycleAndAccounting(t *testing.T) {
	fx := newLedgerFixture(t)
	gui := liquidity.NewService(fx.app)
	// All mutations below are the established GUI path, never an MCP fallback.
	if _, err := history.NewService(fx.app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.brokerage, Amount: "1500", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-28T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	opened := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{
		AccountID: fx.brokerage, Currency: "USD", Principal: "1000", EffectiveAt: "2026-09-28T12:00:00Z",
		Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-09-28", MaturityOn: productString("2026-09-30"), InterestMode: "manual_maturity_amount", MaturityInterest: productString("50")}, Policy: productPolicy("2026-09-30"),
	}})
	id := opened.ProductIDs[0]
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, fx.service)
	read := func(wantCash, wantValue, wantState string) map[string]any {
		t.Helper()
		actual := call(t, client, "get_product", map[string]any{"id": id}, false)["data"].(map[string]any)
		expected, err := gui.Product(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		productJSONEqual(t, actual, expected)
		product := actual["product"].(map[string]any)
		if product["state"] != wantState || product["currentValue"].(map[string]any)["amount"] != wantValue || batchCash(t, client, fx.brokerage) != wantCash {
			t.Fatalf("unexpected cash/value/state: %v", actual)
		}
		return product
	}
	read("500", "1000", "open")
	interest := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "receive_interest", ReceiveInterest: &application.ReceiveInterestCommand{ProductID: id, Amount: "10", EffectiveAt: "2026-09-29T12:00:00Z", RemainingInterest: productString("40")}})
	if product := read("510", "1000", "open"); product["maturityInterest"].(map[string]any)["amount"] != "40" {
		t.Fatal("received interest was not removed from forecast")
	}
	settled := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "settle", Settle: &application.SettleProductCommand{ProductID: id, ReturnedPrincipal: productString("1000"), Interest: productString("40"), Fee: productString("2"), EffectiveAt: "2026-09-29T12:00:00Z"}})
	read("1548", "0", "settled")
	openOnly := call(t, client, "list_products", map[string]any{}, false)["data"].([]any)
	if len(openOnly) != 0 {
		t.Fatal("closed product in default list")
	}
	all := call(t, client, "list_products", skillExample(t, "products", "product-list", map[string]string{"accountId": fx.brokerage}), false)["data"]
	expected, err := gui.ListProducts(t.Context(), liquidity.ListProductsRequest{AccountID: &fx.brokerage, IncludeClosed: true})
	if err != nil {
		t.Fatal(err)
	}
	productJSONEqual(t, all, expected)
	undo := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: settled.OperationID}})
	read("510", "1000", "open")
	// Check newest-first pagination and reversal links across equal creation times.
	seen := map[string]bool{}
	cursor := ""
	for {
		args := skillExample(t, "products", "product-history", map[string]string{"productId": id})
		args["limit"], args["cursor"] = 1, cursor
		page := call(t, client, "list_product_operations", args, false)["data"].(map[string]any)
		expected, err := gui.ListOperations(t.Context(), liquidity.ListOperationsRequest{ProductID: id, Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		productJSONEqual(t, page, expected)
		for _, raw := range page["operations"].([]any) {
			op := raw.(map[string]any)
			opID := op["id"].(string)
			if seen[opID] || op["requestJson"] != nil || op["resultJson"] != nil {
				t.Fatal("duplicate or private operation data", op)
			}
			seen[opID] = true
			if opID == undo.OperationID && op["reversesOperationId"] != settled.OperationID {
				t.Fatal("missing reversal link", op)
			}
		}
		if page["next"] == nil {
			break
		}
		cursor = page["next"].(string)
	}
	for _, op := range []string{opened.OperationID, interest.OperationID, settled.OperationID, undo.OperationID} {
		if !seen[op] {
			t.Fatal("lost operation", op)
		}
	}
	if len(seen) != 4 {
		t.Fatal(seen)
	}
	// Advancing beyond maturity changes only the display/forecast, never cash.
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) })
	if product := read("510", "1000", "open"); product["displayState"] != "due_unconfirmed" {
		t.Fatal("maturity was mistaken for actual settlement", product)
	}
	args := skillExample(t, "products", "product-liquidity", nil)
	overview := call(t, client, "get_liquidity_overview", args, false)["data"]
	expectedOverview, err := gui.Overview(t.Context(), liquidity.OverviewRequest{CustomHorizonOn: productString("2026-10-08"), IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	productJSONEqual(t, overview, expectedOverview)
	read("510", "1000", "open")
}

func TestProductHTTPReservesUnknownFXAndReadPurity(t *testing.T) {
	fx := newLedgerFixture(t)
	gui := liquidity.NewService(fx.app)
	if _, err := history.NewService(fx.app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.brokerage, Amount: "500", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-29T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"simple_act_365", "simple_act_360"} {
		guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{
			AccountID: fx.brokerage, Currency: "USD", Principal: "1000", TotalCostBasis: "1000", CurrentValue: "1000", CashExcludesProduct: true,
			Terms: application.ProductTermsInput{Kind: "term_deposit", Name: mode, StartOn: "2026-09-01", MaturityOn: productString("2026-10-01"), InterestMode: mode, AnnualRatePercent: productString("3.65")}, Policy: productPolicy("2026-10-01"),
		}})
	}
	locked := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{
		AccountID: fx.brokerage, Currency: "EUR", Principal: "100", TotalCostBasis: "90", CurrentValue: "110", CashExcludesProduct: true,
		Terms: application.ProductTermsInput{Kind: "locked_product", Name: "Missing FX", StartOn: "2026-09-01", InterestMode: "none"}, Policy: application.ProductPolicyInput{AccessKind: "unknown", EarlyKind: "unknown"},
	}})
	zero, calendar := 0, "calendar"
	if _, err := fx.app.SaveLiquidityPolicy(t.Context(), application.SavePolicyInput{
		Source: domain.AccountCashSourceRef(domain.AccountID(fx.brokerage), domain.CurrencyCode("USD")),
		Policy: application.ProductPolicyInput{AccessKind: "on_request", SettlementDays: &zero, DayBasis: &calendar, NormalExitFee: productString("0"), EarlyKind: "not_allowed"},
	}); err != nil {
		t.Fatal(err)
	}
	reservation, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.AccountCashSourceRef(domain.AccountID(fx.brokerage), domain.CurrencyCode("USD")), Label: "Synthetic reserve", Amount: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	client := connect(t, fx.service)
	// These reads must not invalidate an already reviewed unrelated ledger plan.
	plan := ledgerPreview(t, client, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-29T12:00:00Z"})
	count := ledgerActivityCount(t, fx.app)
	list := call(t, client, "list_products", map[string]any{}, false)["data"]
	expected, err := gui.ListProducts(t.Context(), liquidity.ListProductsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	productJSONEqual(t, list, expected)
	call(t, client, "get_product", skillExample(t, "products", "product-detail", map[string]string{"productId": locked.ProductIDs[0]}), false)
	call(t, client, "list_product_operations", map[string]any{"productId": locked.ProductIDs[0]}, false)
	actual := call(t, client, "get_liquidity_overview", map[string]any{}, false)["data"]
	overview, err := gui.Overview(t.Context(), liquidity.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	productJSONEqual(t, actual, overview)
	if overview.Buckets[0].FullAvailable != nil || overview.Buckets[0].UnknownSourceCount == 0 {
		t.Fatal("unknown source/FX was converted to a complete value", overview)
	}
	var cashFound, missingFXFound bool
	for _, source := range overview.Sources {
		if source.SourceRef.Kind == string(domain.SourceAccountCash) && source.NativeCurrency == "USD" {
			cashFound = true
			bucket := source.BucketResults[0]
			if source.ReservationRequested.Amount != "100" || bucket.NetNative == nil || bucket.NetNative.Amount != "500" || bucket.AppliedReserveNative == nil || bucket.AppliedReserveNative.Amount != "100" || bucket.UnreservedNative == nil || bucket.UnreservedNative.Amount != "400" {
				t.Fatal("gross/after-reserve distinction lost", source)
			}
		}
		if source.ProductID != nil && *source.ProductID == locked.ProductIDs[0] {
			missingFXFound = true
			if source.NativeCurrency != "EUR" || source.CurrentNativeValue.Amount != "110" || source.BucketResults[0].NetBase != nil || source.NormalRoute.Status != "unavailable" {
				t.Fatal("missing FX/unknown route replaced with zero", source)
			}
		}
	}
	if !cashFound || !missingFXFound || ledgerActivityCount(t, fx.app) != count || batchCash(t, client, fx.brokerage) != "500" {
		t.Fatal("missing source or read changed facts")
	}
	reserves, err := fx.app.ListLiquidityReservations(t.Context())
	if err != nil || len(reserves) != 1 || reserves[0].ID != reservation.ID || reserves[0].ReleasedAt != nil || reserves[0].Revision != reservation.Revision {
		t.Fatal("read released/edited reservation", reserves, err)
	}
	ledgerCommit(t, client, uuid.NewString(), plan["planId"].(string))
}

func TestProductHTTPStrictInputsAndPrivateErrors(t *testing.T) {
	fx := newLedgerFixture(t)
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, fx.service)
	for _, tc := range []struct {
		tool string
		args any
	}{
		{"list_products", map[string]any{"accountId": ""}},
		{"list_products", map[string]any{"accountId": "private-invalid-id"}},
		{"get_product", map[string]any{"id": "private-invalid-id"}},
		{"list_product_operations", map[string]any{"productId": uuid.NewString(), "limit": -1}},
		{"list_product_operations", map[string]any{"productId": uuid.NewString(), "limit": 101}},
		{"list_product_operations", map[string]any{"productId": uuid.NewString(), "cursor": strings.Repeat("private", 100)}},
		{"get_liquidity_overview", map[string]any{"customHorizonOn": "private-date"}},
	} {
		if code := ledgerErrorCode(t, client, tc.tool, tc.args); code != "validation" {
			t.Fatalf("%s: %s", tc.tool, code)
		}
	}
	for _, name := range []string{"get_product", "list_product_operations"} {
		args := map[string]any{"id": uuid.NewString()}
		if name == "list_product_operations" {
			args = map[string]any{"productId": uuid.NewString()}
		}
		if code := ledgerErrorCode(t, client, name, args); code != "not_found" {
			t.Fatal("missing product must not look like valid empty history", code)
		}
	}
	for _, tool := range []string{"list_products", "get_product", "list_product_operations", "get_liquidity_overview"} {
		for _, args := range []any{map[string]any{"private-sensitive-property": "private-sensitive-value"}, map[string]any{"includeClosed": "private-sensitive-value"}} {
			status, raw := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}, "")
			if status != http.StatusOK || strings.Contains(string(raw), "private-sensitive") || !strings.Contains(string(raw), `\"code\":\"validation\"`) {
				t.Fatalf("schema diagnostics echoed private input: %d %s", status, raw)
			}
		}
	}
}

func productHTTP(t *testing.T, service *Service, payload any, protocol string) (int, []byte) {
	t.Helper()
	cfg, err := service.Connection()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.Endpoint, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	if protocol != "" {
		req.Header.Set("MCP-Protocol-Version", protocol)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, body
}

func TestProductHTTPBudgetAndBatchGuard(t *testing.T) {
	fx := newLedgerFixture(t)
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"list_products", "get_product", "list_product_operations", "get_liquidity_overview"} {
		payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{strings.Repeat("private", 12000): true}}}
		status, raw := productHTTP(t, fx.service, payload, "")
		if status != http.StatusOK || len(raw) > contextWireLimit || strings.Contains(string(raw), "privateprivate") || !strings.Contains(string(raw), "too_large") || !strings.Contains(string(raw), "use the GUI to view the complete result") || strings.Contains(string(raw), "account filter") {
			t.Fatalf("unbounded schema error: %d %d %s", status, len(raw), raw)
		}
		payload["id"] = strings.Repeat("private", 100)
		if status, _ := productHTTP(t, fx.service, payload, ""); status != http.StatusBadRequest {
			t.Fatal("unbounded RPC ID accepted")
		}
		payload["id"] = 1
		for _, protocol := range []string{"", "2025-03-26", "2025-06-18"} {
			if status, _ := productHTTP(t, fx.service, []any{payload}, protocol); status != http.StatusBadRequest {
				t.Fatalf("batch bypass in %s", protocol)
			}
		}
	}
	// Valid stored notes can exceed the final wire budget after SDK duplication
	// into both text and structuredContent. Fail the whole result, never truncate.
	var productID string
	for range 12 {
		policy := productPolicy("2026-12-01")
		policy.Note = productString(strings.Repeat("stored-private-note", 90))
		receipt := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{
			AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100", CashExcludesProduct: true,
			Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Large note", Note: productString(strings.Repeat("stored-private-note", 90)), StartOn: "2026-09-01", MaturityOn: productString("2026-12-01"), InterestMode: "none"}, Policy: policy,
		}})
		productID = receipt.ProductIDs[0]
	}
	for _, tc := range []struct {
		name string
		args any
	}{{"list_products", map[string]any{}}, {"get_liquidity_overview", map[string]any{}}} {
		status, raw := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tc.name, "arguments": tc.args}}, "")
		if status != http.StatusOK || len(raw) > contextWireLimit || strings.Contains(string(raw), "stored-private-note") || !strings.Contains(string(raw), "too_large") || !strings.Contains(string(raw), "use the GUI to view the complete result") || strings.Contains(string(raw), "account filter") {
			t.Fatalf("oversize product was not rejected privately: %d %d", status, len(raw))
		}
	}
	client := connect(t, fx.service)
	if list := call(t, client, "list_products", map[string]any{"accountId": fx.savings}, false)["data"].([]any); len(list) != 0 {
		t.Fatal("account filter did not narrow oversized list")
	}
	detail, err := fx.app.Product(t.Context(), domain.ProductContractID(productID))
	if err != nil {
		t.Fatal(err)
	}
	// Reservation evidence must not be removed to make a large detail fit.
	for range 90 {
		if _, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: strings.Repeat("stored-private-reserve", 5), Amount: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	status, raw := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "get_product", "arguments": map[string]any{"id": productID}}}, "")
	if status != http.StatusOK || len(raw) > contextWireLimit || !strings.Contains(string(raw), "too_large") || strings.Contains(string(raw), "stored-private-reserve") || !strings.Contains(string(raw), "use the GUI to view the complete result") || strings.Contains(string(raw), "account filter") {
		t.Fatalf("oversized detail leaked/truncated reservations: %d %d", status, len(raw))
	}
}

func TestProductHTTPLongHistoryDefaultAndMaximumPages(t *testing.T) {
	fx := newLedgerFixture(t)
	gui := liquidity.NewService(fx.app)
	receipt := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{
		AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "90", CurrentValue: "110", CashExcludesProduct: true,
		Terms: application.ProductTermsInput{Kind: "locked_product", Name: "Long history", StartOn: "2026-09-01", InterestMode: "none"}, Policy: application.ProductPolicyInput{AccessKind: "unknown", EarlyKind: "unknown"},
	}})
	id := receipt.ProductIDs[0]
	for range 26 {
		if _, err := gui.AppendProductValuation(t.Context(), liquidity.AppendProductValuationRequest{ProductID: id, Amount: "110", ObservedAt: "2026-09-29T12:00:00Z", MutationID: domain.NewProductOperationID().String()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, fx.service)
	page := call(t, client, "list_product_operations", map[string]any{"productId": id}, false)["data"].(map[string]any)
	if len(page["operations"].([]any)) != 25 || page["next"] == nil {
		t.Fatal("default page size/cursor", page)
	}
	last := call(t, client, "list_product_operations", map[string]any{"productId": id, "cursor": page["next"]}, false)["data"].(map[string]any)
	if len(last["operations"].([]any)) != 2 || last["next"] != nil {
		t.Fatal("last page/cursor", last)
	}
	all := call(t, client, "list_product_operations", map[string]any{"productId": id, "limit": 100}, false)["data"].(map[string]any)
	if len(all["operations"].([]any)) != 27 || all["next"] != nil {
		t.Fatal("maximum page size", all)
	}
	if code := ledgerErrorCode(t, client, "list_product_operations", map[string]any{"productId": id, "cursor": "private-invalid-cursor"}); code != "validation" {
		t.Fatal("invalid cursor accepted", code)
	}
	detail := call(t, client, "get_product", map[string]any{"id": id}, false)["data"].(map[string]any)["product"].(map[string]any)
	if detail["currency"] != "USD" || detail["currentValue"].(map[string]any)["amount"] != "110" || detail["currentCostBasis"] != nil || batchCash(t, client, fx.brokerage) != "0" {
		t.Fatal("valuation/import facts did not survive read model", detail)
	}
	// The shared GUI product detail currently leaves cost basis unavailable.
	// Preserve its null rather than inventing cost from principal or value;
	// separately verify the authoritative holding still carries the import cost.
	assertReconciliationCost(t, fx, detail["holdingId"].(string), "1", "90", "90")
}
