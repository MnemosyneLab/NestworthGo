package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func productExistingInput(account string) ProductOperationInput {
	return ProductOperationInput{Kind: "record_existing", RecordExisting: &RecordExistingProductInput{AccountID: account, Currency: "USD", Principal: "1000", TotalCostBasis: "950", CurrentValue: "1000", CashExcludesProduct: true,
		Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic existing", StartOn: "2026-09-01", MaturityOn: productString("2026-09-30"), InterestMode: "manual_maturity_amount", MaturityInterest: productString("50")}, Policy: productPolicy("2026-09-30")}}
}

func productPlanHTTP(t *testing.T, c *mcp.ClientSession, in ProductOperationInput) map[string]any {
	t.Helper()
	return call(t, c, "preview_product_operation", in, false)["data"].(map[string]any)
}

func productSkillPlan(t *testing.T, c *mcp.ClientSession, name string, values map[string]string) map[string]any {
	t.Helper()
	return call(t, c, "preview_product_operation", skillExample(t, "products", name, values), false)["data"].(map[string]any)
}

func productAssertNetWorth(t *testing.T, c *mcp.ClientSession, want string) {
	t.Helper()
	actual := call(t, c, "get_overview", map[string]any{}, false)["data"].(map[string]any)
	if actual["netWorth"] != want {
		t.Fatal(actual)
	}
}

func productCommitHTTP(t *testing.T, c *mcp.ClientSession, planID, operationID string) map[string]any {
	t.Helper()
	op := call(t, c, "commit_product_operation", skillExample(t, "products", "product-commit", map[string]string{"operationId": operationID, "planId": planID}), false)["data"].(map[string]any)
	if op["status"] != "succeeded" {
		t.Fatal(op)
	}
	return op["result"].(map[string]any)
}

func productCommitCode(t *testing.T, c *mcp.ClientSession, planID string) string {
	t.Helper()
	return ledgerErrorCode(t, c, "commit_product_operation", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}})
}

func TestProductHTTPLifecyclePermissionsSchema(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		c := connect(t, fx.service)
		list, err := c.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]*mcp.Tool{}
		for _, tool := range list.Tools {
			found[tool.Name] = tool
		}
		for _, name := range []string{"preview_product_operation", "commit_product_operation"} {
			if (found[name] != nil) != (mode == LedgerWrite) {
				t.Fatalf("%s: %s", mode, name)
			}
			if mode != LedgerWrite {
				result, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err == nil && (result == nil || !result.IsError) {
					t.Fatal(err)
				}
				continue
			}
			raw, _ := json.Marshal(found[name].InputSchema)
			if !strings.Contains(string(raw), `"additionalProperties":false`) || strings.Contains(string(raw), "releaseReservationIds") || strings.Contains(string(raw), `"renew"`) || strings.Contains(string(raw), `"reviewedAt"`) {
				t.Fatal(string(raw))
			}
			if found[name].Annotations.ReadOnlyHint != (name == "preview_product_operation") {
				t.Fatal(found[name])
			}
		}
	}
	c := connect(t, fx.service)
	for _, args := range []any{map[string]any{"kind": "renew", "renew": map[string]any{}}, map[string]any{"kind": "undo", "undo": map[string]any{"operationId": uuid.NewString(), "effectiveAt": "private"}}, map[string]any{"kind": "undo", "reviewedAt": "private", "undo": map[string]any{"operationId": uuid.NewString()}}, map[string]any{"kind": "settle", "settle": map[string]any{"releaseReservationIds": []string{uuid.NewString()}}}} {
		if code := ledgerErrorCode(t, c, "preview_product_operation", args); code != "validation" {
			t.Fatal(code)
		}
	}
	bad := productExistingInput(fx.brokerage)
	bad.Undo = &application.UndoProductCommand{OperationID: uuid.NewString()}
	if code := ledgerErrorCode(t, c, "preview_product_operation", bad); code != "validation" {
		t.Fatal(code)
	}
	bad = productExistingInput(fx.brokerage)
	bad.RecordExisting.CashExcludesProduct = false
	if code := ledgerErrorCode(t, c, "preview_product_operation", bad); code != "validation" {
		t.Fatal(code)
	}
}

func TestProductHTTPLifecycleFrozenTimesAndAccounting(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fx := newLedgerFixtureWithClock(t, func() time.Time { return now })
	c := ledgerSession(t, fx)
	// Funding is an actual ledger fact, not a synthetic product cash leg.
	fund := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1500", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), fund["planId"].(string))
	plan := productSkillPlan(t, c, "product-open", map[string]string{"accountId": fx.brokerage})
	at := plan["preview"].(map[string]any)["effectiveAt"]
	now = now.Add(time.Minute)
	opened := productCommitHTTP(t, c, plan["planId"].(string), uuid.NewString())
	id := opened["productIds"].([]any)[0].(string)
	if opened["operationId"] != plan["planId"] || batchCash(t, c, fx.brokerage) != "500" {
		t.Fatal(opened)
	}
	productAssertNetWorth(t, c, "1500")
	first := call(t, c, "get_activity", map[string]any{"id": opened["activityIds"].([]any)[0]}, false)["data"].(map[string]any)
	if first["effectiveAt"] != at || first["createdAt"] == at {
		t.Fatal("effective time not frozen or creation time falsified", first)
	}
	interest := productSkillPlan(t, c, "product-interest", map[string]string{"productId": id})
	now = now.Add(time.Minute)
	productCommitHTTP(t, c, interest["planId"].(string), uuid.NewString())
	detail := call(t, c, "get_product", IDInput{ID: id}, false)["data"].(map[string]any)["product"].(map[string]any)
	if batchCash(t, c, fx.brokerage) != "510" || detail["currentValue"].(map[string]any)["amount"] != "1000" || detail["maturityInterest"].(map[string]any)["amount"] != "40" {
		t.Fatal(detail)
	}
	productAssertNetWorth(t, c, "1510")
	// Whole settlement before the predicted unlock records an actual receipt.
	settle := productSkillPlan(t, c, "product-settle", map[string]string{"productId": id})
	now = now.Add(time.Minute)
	settled := productCommitHTTP(t, c, settle["planId"].(string), uuid.NewString())
	if batchCash(t, c, fx.brokerage) != "1548" {
		t.Fatal(settled)
	}
	productAssertNetWorth(t, c, "1548")
	undo := productSkillPlan(t, c, "product-undo", map[string]string{"productOperationId": settled["operationId"].(string)})
	undoAt := undo["preview"].(map[string]any)["effectiveAt"]
	now = now.Add(2 * time.Minute)
	reversed := productCommitHTTP(t, c, undo["planId"].(string), uuid.NewString())
	if batchCash(t, c, fx.brokerage) != "510" {
		t.Fatal(reversed)
	}
	productAssertNetWorth(t, c, "1510")
	activity := call(t, c, "get_activity", IDInput{ID: reversed["activityIds"].([]any)[0].(string)}, false)["data"].(map[string]any)
	if activity["effectiveAt"] != undoAt || activity["createdAt"] == undoAt {
		t.Fatal(activity)
	}
	// Existing import freezes preview-time, keeps original cost and never moves cash.
	p := productSkillPlan(t, c, "product-existing", map[string]string{"accountId": fx.brokerage})
	existingAt := p["preview"].(map[string]any)["effectiveAt"]
	now = now.Add(time.Minute)
	imported := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
	importActivity := call(t, c, "get_activity", IDInput{ID: imported["activityIds"].([]any)[0].(string)}, false)["data"].(map[string]any)
	if importActivity["effectiveAt"] != existingAt || batchCash(t, c, fx.brokerage) != "510" {
		t.Fatal(importActivity)
	}
	product, err := fx.app.Product(t.Context(), domain.ProductContractID(imported["productIds"].([]any)[0].(string)))
	if err != nil {
		t.Fatal(err)
	}
	gain, err := fx.app.HoldingGain(t.Context(), product.Contract.HoldingID)
	if err != nil {
		t.Fatal(err)
	}
	if gain.TotalCost.Amount != "950" {
		t.Fatal(gain)
	}
	count := ledgerActivityCount(t, fx.app)
	replayed := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
	if replayed["operationId"] != imported["operationId"] || replayed["replayed"] != true || ledgerActivityCount(t, fx.app) != count {
		t.Fatal(replayed)
	}
	productAssertNetWorth(t, c, "2510")
	// The closed-day instrument return counts actual received interest once,
	// including the settlement's compensating reversal, never its forecast.
	now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	original, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	analysis := call(t, c, "analyze_period", map[string]any{"query": map[string]any{"from": "2026-09-29", "to": "2026-09-29", "scopeKind": "instrument", "scopeId": original.Contract.InstrumentID.String(), "valuation": "base", "basis": "investment", "includeCash": true}}, false)["data"].(map[string]any)
	returns := analysis["investmentReturns"].(map[string]any)
	if returns["amount"].(map[string]any)["amount"] != "10" {
		t.Fatal(returns)
	}
	productAssertNetWorth(t, c, "2510")
}

func TestProductHTTPPlanSafetyAndReservations(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	id := plan["planId"].(string)
	// Wrong family never acquires a financial write permit or consumes the plan.
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": id}}); code != "validation" {
		t.Fatal(code)
	}
	result := productCommitHTTP(t, c, id, uuid.NewString())
	productID := result["productIds"].([]any)[0].(string)
	interest := ProductOperationInput{Kind: "receive_interest", ReceiveInterest: &application.ReceiveInterestCommand{ProductID: productID, Amount: "1", RemainingInterest: productString("49")}}
	stale := productPlanHTTP(t, c, interest)
	guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "receive_interest", ReceiveInterest: interest.ReceiveInterest})
	if code := productCommitCode(t, c, stale["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	expired := productPlanHTTP(t, c, interest)
	path := filepath.Join(fx.service.dir, "plans", expired["planId"].(string)+".json")
	raw, err := fx.service.readPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored productOperationPlan
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	stored.ExpiresAt = time.Now().Add(-time.Second)
	if err := fx.service.writePrivate(path, stored); err != nil {
		t.Fatal(err)
	}
	if code := productCommitCode(t, c, stored.ID); code != "stale_preview" {
		t.Fatal(code)
	}
	undoOld := ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: result["operationId"].(string)}}
	if code := ledgerErrorCode(t, c, "preview_product_operation", undoOld); code != "unsafe_undo" {
		t.Fatal(code)
	}
	product, err := fx.app.Product(t.Context(), domain.ProductContractID(productID))
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(product.Contract.AccountID, product.Contract.HoldingID), Label: "Reserved", Amount: "100"})
	if err != nil {
		t.Fatal(err)
	}
	settle := ProductOperationInput{Kind: "settle", Settle: &SettleProductInput{ProductID: productID, ReturnedPrincipal: productString("1000"), Interest: productString("49")}}
	if code := ledgerErrorCode(t, c, "preview_product_operation", settle); code != "unresolved_reservation_release" {
		t.Fatal(code)
	}
	current, err := fx.app.Product(t.Context(), product.Contract.ID)
	if err != nil || len(current.Reservations) != 1 || current.Reservations[0].Revision != reserve.Revision {
		t.Fatal(current, err)
	}
	// A GUI settlement can release explicitly. MCP undo must not restore it implicitly.
	guiSettle := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "settle", Settle: &application.SettleProductCommand{ProductID: productID, ReturnedPrincipal: productString("1000"), Interest: productString("49"), ReleaseReservationIDs: []string{reserve.ID.String()}}})
	if code := ledgerErrorCode(t, c, "preview_product_operation", ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: guiSettle.OperationID}}); code != "unresolved_reservation_release" {
		t.Fatal(code)
	}
}

func newPersistentProductFixture(t *testing.T, clock func() time.Time) (ledgerFixture, *sqlite.DB, *sqlite.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "products.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo)
	app.SetLiveDatabasePath(path)
	fx := newLedgerFixtureWithApp(t, app, clock)
	fx.service.Close()
	fx.service = New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(db))
	t.Cleanup(fx.service.Close)
	return fx, db, repo
}

func TestProductHTTPRestartUnknownAndRestore(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	fx, db, repo := newPersistentProductFixture(t, clock)
	c := ledgerSession(t, fx)
	plan := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	id := plan["planId"].(string)
	opID := uuid.NewString()
	posted := productCommitHTTP(t, c, id, opID)
	count := ledgerActivityCount(t, fx.app)
	op, err := fx.service.GetOperation(opID)
	if err != nil {
		t.Fatal(err)
	}
	op.Status = "pending"
	op.Result = nil
	path, _ := fx.service.operationPath(opID)
	if err := fx.service.writePrivate(path, op); err != nil {
		t.Fatal(err)
	}
	uncommitted := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	fx.service.Close()
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo)
	app.SetClock(clock)
	s := New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(db))
	t.Cleanup(s.Close)
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	fx.service = s
	fx.app = app
	c = connect(t, s)
	recovered := productCommitHTTP(t, c, id, opID)
	if recovered["operationId"] != posted["operationId"] || recovered["replayed"] != true || ledgerActivityCount(t, app) != count {
		t.Fatal(recovered)
	}
	if code := productCommitCode(t, c, uncommitted["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	fresh := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	if err := app.WithExclusive(t.Context(), application.ExclusiveRestore, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if code := productCommitCode(t, c, fresh["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	productCommitHTTP(t, c, id, uuid.NewString()) // retained business receipt still replays after restore fencing
	if ledgerActivityCount(t, app) != count {
		t.Fatal("duplicated during recovery")
	}
}

func TestProductHTTPConcurrentCommitAndAtomicFailure(t *testing.T) {
	fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	plan := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	id := plan["planId"].(string)
	var wg sync.WaitGroup
	results := make(chan *mcp.CallToolResult, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "commit_product_operation", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": id}}})
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for r := range results {
		if r.IsError {
			t.Fatal(r)
		}
	}
	if ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("concurrent duplicate")
	}
	for _, stage := range []string{"operation", "instruments", "observations", "holdings", "quotes", "activity:0", "contracts", "policies", "links", "complete"} {
		p := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
		before := ledgerActivityCount(t, fx.app)
		counts := func() []int {
			var values []int
			for _, table := range []string{"product_operations", "instruments", "holdings", "instrument_quotes", "product_contracts", "liquidity_policies", "product_operation_products", "product_operation_activities"} {
				var count int
				if err := db.SQL.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				values = append(values, count)
			}
			return values
		}
		beforeCounts := counts()
		opID := uuid.NewString()
		sqlite.SetProductCommitFailAfter(stage)
		code := ledgerErrorCode(t, c, "commit_product_operation", map[string]any{"operationId": opID, "input": map[string]any{"planId": p["planId"]}})
		sqlite.SetProductCommitFailAfter("")
		if code != "unavailable" || ledgerActivityCount(t, fx.app) != before {
			t.Fatalf("%s %s: partial write", stage, code)
		}
		productJSONEqual(t, counts(), beforeCounts)
		// Failed write invalidates the uncommitted plan; unknown receipt cannot blindly reapply.
		if code := productCommitCode(t, c, p["planId"].(string)); code != "stale_preview" {
			t.Fatal(code)
		}
	}
}

type failProductSuccessReceipt struct {
	settings.ConfigurationRepository
	fail bool
}

func (r *failProductSuccessReceipt) SaveConfiguration(key string, value []byte) error {
	var op Operation
	if r.fail && strings.HasPrefix(key, "mcp.operation.") && json.Unmarshal(value, &op) == nil && op.Status == "succeeded" {
		r.fail = false
		return errors.New("synthetic final receipt persistence failure")
	}
	return r.ConfigurationRepository.SaveConfiguration(key, value)
}

func TestProductHTTPPostCommitReceiptFailureAndOlderSnapshot(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	fx, _, _ := newPersistentProductFixture(t, clock)
	c := ledgerSession(t, fx)
	snapshot := filepath.Join(t.TempDir(), "before.db")
	if err := fx.app.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	p := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	fx.service.repository = &failProductSuccessReceipt{ConfigurationRepository: fx.service.repository, fail: true}
	opID := uuid.NewString()
	if code := ledgerErrorCode(t, c, "commit_product_operation", map[string]any{"operationId": opID, "input": map[string]any{"planId": p["planId"]}}); code != "operation_outcome_unknown" {
		t.Fatal(code)
	}
	if ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("lost committed fact")
	}
	op, err := fx.service.GetOperation(opID)
	if err != nil || op.Status != "unknown" {
		t.Fatal(op, err)
	}
	posted := productCommitHTTP(t, c, p["planId"].(string), opID)
	if posted["replayed"] != true || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal(posted)
	}
	second := productPlanHTTP(t, c, productExistingInput(fx.brokerage))
	if code := ledgerErrorCode(t, c, "commit_product_operation", map[string]any{"operationId": opID, "input": map[string]any{"planId": second["planId"]}}); code != "conflict" {
		t.Fatal(code)
	}
	fx.service.Close()
	restored, err := sqlite.Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repo := sqlite.NewRepository(restored)
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo)
	app.SetClock(clock)
	s := New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(restored))
	t.Cleanup(s.Close)
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	c = connect(t, s)
	if _, err := s.GetOperation(opID); safeError(err).Code != "not_found" {
		t.Fatal(err)
	}
	if code := productCommitCode(t, c, p["planId"].(string)); code != "not_found" {
		t.Fatal(code)
	}
	if ledgerActivityCount(t, app) != 0 {
		t.Fatal("older restore reused external success or plan")
	}
}

func TestProductHTTPLocalLifecycleTimesRemainFrozen(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fx := newLedgerFixtureWithClock(t, func() time.Time { return now })
	c := ledgerSession(t, fx)
	fund := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1500", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-28T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), fund["planId"].(string))
	var productID string
	for i, kind := range []string{"open", "receive_interest", "settle"} {
		localTime := []string{"11:30", "11:31", "11:32"}[i]
		var input ProductOperationInput
		switch kind {
		case "open":
			input = ProductOperationInput{Kind: kind, Open: &application.OpenProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "1000", EffectiveLocalDate: "2026-09-29", EffectiveLocalTime: localTime, Terms: productExistingInput("").RecordExisting.Terms, Policy: productPolicy("2026-09-30")}}
		case "receive_interest":
			input = ProductOperationInput{Kind: kind, ReceiveInterest: &application.ReceiveInterestCommand{ProductID: productID, Amount: "10", RemainingInterest: productString("40"), EffectiveLocalDate: "2026-09-29", EffectiveLocalTime: localTime}}
		case "settle":
			input = ProductOperationInput{Kind: kind, Settle: &SettleProductInput{ProductID: productID, ReturnedPrincipal: productString("1000"), Interest: productString("40"), EffectiveLocalDate: "2026-09-29", EffectiveLocalTime: localTime}}
		}
		p := productPlanHTTP(t, c, input)
		at := p["preview"].(map[string]any)["effectiveAt"]
		want := "2026-09-29T" + localTime + ":00.000Z"
		if at != want {
			t.Fatal(kind, at, want)
		}
		now = now.Add(time.Minute)
		r := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
		productID = r["productIds"].([]any)[0].(string)
		for _, activityID := range r["activityIds"].([]any) {
			a := call(t, c, "get_activity", IDInput{ID: activityID.(string)}, false)["data"].(map[string]any)
			if a["effectiveAt"] != want || a["createdAt"] != now.Format("2006-01-02T15:04:05.000Z") {
				t.Fatal(kind, a)
			}
		}
		// Same reviewed plan with a new outer ID recovers its original receipt.
		replay := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
		if replay["replayed"] != true || replay["operationId"] != r["operationId"] {
			t.Fatal(kind, replay)
		}
	}
	if batchCash(t, c, fx.brokerage) != "1550" || ledgerActivityCount(t, fx.app) != 5 {
		t.Fatal("local-time lifecycle accounting mismatch")
	}
	productAssertNetWorth(t, c, "1550")
}
