package mcpserver

import (
	"context"
	"encoding/json"
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
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func productDataPreviewHTTP(t *testing.T, c *mcp.ClientSession, tool string, input any) map[string]any {
	t.Helper()
	return call(t, c, tool, input, false)["data"].(map[string]any)
}

func productDataCommitHTTP(t *testing.T, c *mcp.ClientSession, tool, id, operation string) map[string]any {
	t.Helper()
	op := call(t, c, tool, map[string]any{"operationId": operation, "input": map[string]any{"planId": id}}, false)["data"].(map[string]any)
	if op["status"] != "succeeded" {
		t.Fatal(op)
	}
	return op["result"].(map[string]any)
}

func productDataCommitCode(t *testing.T, c *mcp.ClientSession, tool, id string) string {
	t.Helper()
	return ledgerErrorCode(t, c, tool, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": id}})
}

func productDataSeed(t *testing.T, fx ledgerFixture, c *mcp.ClientSession, kind string) (string, map[string]any) {
	t.Helper()
	input := productExistingInput(fx.brokerage)
	input.RecordExisting.Terms.Kind = kind
	if kind == "locked_product" {
		input.RecordExisting.Terms.InterestMode = "none"
		input.RecordExisting.Terms.MaturityInterest = nil
	}
	p := productPlanHTTP(t, c, input)
	r := productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
	return r["productIds"].([]any)[0].(string), r
}

func productTermsInput(id string, revision int, kind string) liquidity.UpdateProductTermsRequest {
	terms := productExistingInput("").RecordExisting.Terms
	terms.Kind = kind
	terms.Name = "Updated terms"
	terms.MaturityOn = productString("2026-12-01")
	if kind == "locked_product" {
		terms.InterestMode = "none"
		terms.MaturityInterest = nil
	} else {
		terms.InterestMode = "simple_act_360"
		terms.MaturityInterest = nil
		terms.AnnualRate = productString("0.05")
		terms.InterestPaidThroughOn = productString("2026-09-01")
	}
	return liquidity.UpdateProductTermsRequest{ProductID: id, ExpectedRevision: revision, Terms: terms, Policy: productPolicy("2026-12-01")}
}

func productEditPlan(t *testing.T, s *Service, id string, change func(map[string]any)) {
	t.Helper()
	path := filepath.Join(s.dir, "plans", id+".json")
	raw, err := s.readPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	change(plan)
	if err := s.writePrivate(path, plan); err != nil {
		t.Fatal(err)
	}
}

func TestProductHTTPDataPermissionsSchemaAndValidation(t *testing.T) {
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
		for _, name := range []string{"preview_product_terms", "commit_product_terms", "preview_product_valuation", "commit_product_valuation"} {
			if (found[name] != nil) != (mode == LedgerWrite) {
				t.Fatalf("%s %s", mode, name)
			}
			if mode != LedgerWrite {
				r, e := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if e == nil && (r == nil || !r.IsError) {
					t.Fatal(name)
				}
				continue
			}
			raw, _ := json.Marshal(found[name].InputSchema)
			if !strings.Contains(string(raw), `"additionalProperties":false`) || strings.Contains(string(raw), "releaseReservationIds") || strings.Contains(string(raw), "mutationId") {
				t.Fatal(string(raw))
			}
			if found[name].Annotations.ReadOnlyHint != strings.HasPrefix(name, "preview_") {
				t.Fatal(found[name])
			}
		}
		info := call(t, c, "get_context", Empty{}, false)["data"].(map[string]any)
		caps, _ := json.Marshal(info["capabilities"])
		for _, cap := range []string{"managed_product_terms", "managed_product_valuation"} {
			if strings.Contains(string(caps), cap) != (mode == LedgerWrite) {
				t.Fatal(info)
			}
		}
	}
	c := connect(t, fx.service)
	deposit, _ := productDataSeed(t, fx, c, "term_deposit")
	if code := ledgerErrorCode(t, c, "preview_product_valuation", ProductValuationInput{ProductID: deposit, Amount: "999", ObservedAt: ""}); code != string(domain.ErrManagedPosition) {
		t.Fatal(code)
	}
	locked, _ := productDataSeed(t, fx, c, "locked_product")
	for _, in := range []any{map[string]any{"productId": locked, "amount": "1000", "observedAt": "", "mutationId": "private"}, map[string]any{"productId": locked, "amount": 123, "observedAt": ""}, map[string]any{"productId": locked, "amount": "1000", "observedAt": "", "releaseReservationIds": []string{uuid.NewString()}}} {
		if code := ledgerErrorCode(t, c, "preview_product_valuation", in); code != "validation" {
			t.Fatal(code)
		}
	}
	for _, in := range []ProductValuationInput{{locked, "-1", ""}, {locked, "0", ""}, {locked, "1", "2026-09-30T12:00:00Z"}, {locked, "1", "2026-08-31T12:00:00Z"}} {
		if code := ledgerErrorCode(t, c, "preview_product_valuation", in); code == "" {
			t.Fatal(in)
		}
	}
	bad := productTermsInput(deposit, 999, "term_deposit")
	if code := ledgerErrorCode(t, c, "preview_product_terms", bad); code != "revision_conflict" {
		t.Fatal(code)
	}
	bad = productTermsInput(deposit, 1, "locked_product")
	if code := ledgerErrorCode(t, c, "preview_product_terms", bad); code != "conflict" {
		t.Fatal(code)
	}
	bad = productTermsInput(deposit, 1, "term_deposit")
	bad.Terms.StartOn = "2026-09-02"
	if code := ledgerErrorCode(t, c, "preview_product_terms", bad); code != "conflict" {
		t.Fatal(code)
	}
	bad = productTermsInput(deposit, 1, "term_deposit")
	bad.Policy.AccessibleAmountCap = productString("100")
	if code := ledgerErrorCode(t, c, "preview_product_terms", bad); code == "" {
		t.Fatal("managed cap accepted")
	}
}

func TestProductHTTPTermsPreviewCommitReceiptAndReservations(t *testing.T) {
	fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	id, opened := productDataSeed(t, fx, c, "term_deposit")
	before, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(before.Contract.AccountID, before.Contract.HoldingID), Label: "Held for goal", Amount: "100"})
	if err != nil {
		t.Fatal(err)
	}
	input := productTermsInput(id, 1, "term_deposit")
	p := productDataPreviewHTTP(t, c, "preview_product_terms", input)
	plan := p["planId"].(string)
	if p["before"].(map[string]any)["revision"] != float64(1) || p["after"].(map[string]any)["revision"] != float64(2) || p["netWorthDelta"].(map[string]any)["amount"] != "0" {
		t.Fatal(p)
	}
	if code := productDataCommitCode(t, c, "commit_product_valuation", plan); code != "validation" {
		t.Fatal(code)
	}
	if code := productCommitCode(t, c, plan); code != "validation" {
		t.Fatal(code)
	}
	result := productDataCommitHTTP(t, c, "commit_product_terms", plan, uuid.NewString())
	if result["mutationId"] != plan {
		t.Fatal(result)
	}
	current, err := fx.app.Product(t.Context(), before.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	productJSONEqual(t, result["product"], liquidity.FromProductDetail(current).Product)
	if current.Contract.Revision != 2 || current.Policy.Revision != 2 || len(current.Reservations) != 1 || current.Reservations[0].Revision != reserve.Revision {
		t.Fatal(current)
	}
	if batchCash(t, c, fx.brokerage) != "0" || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("terms posted financial effects")
	}
	productAssertNetWorth(t, c, "1000")
	assertReconciliationCost(t, fx, current.Contract.HoldingID.String(), "1", "950", "950")
	// A later edit blocks unsafe lifecycle undo and must not rewrite an earlier receipt.
	if code := ledgerErrorCode(t, c, "preview_product_operation", ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: opened["operationId"].(string)}}); code != "unsafe_undo" {
		t.Fatal(code)
	}
	laterInput := productTermsInput(id, 2, "term_deposit")
	laterInput.Terms.Name = "Later actual terms"
	laterInput.Terms.MaturityOn = productString("2027-01-05")
	laterInput.Terms.AnnualRate = productString("0.08")
	laterInput.Policy = productPolicy("2027-01-05")
	next := productDataPreviewHTTP(t, c, "preview_product_terms", laterInput)
	productDataCommitHTTP(t, c, "commit_product_terms", next["planId"].(string), uuid.NewString())
	replay := productDataCommitHTTP(t, c, "commit_product_terms", plan, uuid.NewString())
	if replay["replayed"] != true || replay["product"].(map[string]any)["revision"] != float64(2) || replay["product"].(map[string]any)["maturityOn"] != "2026-12-01" || replay["product"].(map[string]any)["annualRate"] != "0.05" {
		t.Fatal(replay)
	}
	snapshot := filepath.Join(t.TempDir(), "typed.db")
	if err := db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	verified.Close()
	// Private receipts are excluded from JSON export; contract/policy facts remain included.
	raw, err := fx.app.ExportJSONBytes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "product.terms-mutation.") || strings.Contains(string(raw), "mcp.plan.") {
		t.Fatal("private recovery metadata exported")
	}
}

func TestProductHTTPValuationFrozenObservationImmutableReceiptAndHistory(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fx := newLedgerFixtureWithClock(t, func() time.Time { return now })
	c := ledgerSession(t, fx)
	id, opened := productDataSeed(t, fx, c, "locked_product")
	p := productDataPreviewHTTP(t, c, "preview_product_valuation", ProductValuationInput{id, "1100", ""})
	plan := p["planId"].(string)
	preview := p["preview"].(map[string]any)
	command := preview["command"].(map[string]any)
	if preview["netWorthDelta"].(map[string]any)["amount"] != "100" || preview["currentValueBefore"].(map[string]any)["amount"] != "1000" || preview["currentValueAfter"].(map[string]any)["amount"] != "1100" {
		t.Fatal(p)
	}
	now = now.Add(time.Minute)
	result := productDataCommitHTTP(t, c, "commit_product_valuation", plan, uuid.NewString())
	observed, err := time.Parse(time.RFC3339Nano, result["observedAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := time.Parse(time.RFC3339Nano, command["observedAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Equal(frozen) || result["observedAt"] == result["recordedAt"] || result["operationId"] != plan || result["amount"].(map[string]any)["amount"] != "1100" {
		t.Fatal(result)
	}
	productAssertNetWorth(t, c, "1100")
	if batchCash(t, c, fx.brokerage) != "0" || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("valuation wrote cash or activity")
	}
	current, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	assertReconciliationCost(t, fx, current.Contract.HoldingID.String(), "1", "950", "950")
	if code := ledgerErrorCode(t, c, "preview_product_operation", ProductOperationInput{Kind: "undo", Undo: &application.UndoProductCommand{OperationID: opened["operationId"].(string)}}); code != "unsafe_undo" {
		t.Fatal(code)
	}
	later := productDataPreviewHTTP(t, c, "preview_product_valuation", ProductValuationInput{id, "1200", ""})
	productDataCommitHTTP(t, c, "commit_product_valuation", later["planId"].(string), uuid.NewString())
	replay := productDataCommitHTTP(t, c, "commit_product_valuation", plan, uuid.NewString())
	if replay["quoteId"] != result["quoteId"] || replay["amount"].(map[string]any)["amount"] != "1100" || replay["replayed"] != true {
		t.Fatal(replay)
	}
	historical := productDataPreviewHTTP(t, c, "preview_product_valuation", ProductValuationInput{id, "900", "2026-09-28T14:00:00+02:00"})
	h := historical["preview"].(map[string]any)
	if h["currentValueBefore"].(map[string]any)["amount"] != "1200" || h["currentValueAfter"].(map[string]any)["amount"] != "1200" || h["netWorthDelta"].(map[string]any)["amount"] != "0" {
		t.Fatal(h)
	}
	productDataCommitHTTP(t, c, "commit_product_valuation", historical["planId"].(string), uuid.NewString())
	productAssertNetWorth(t, c, "1200")
	page := call(t, c, "list_product_operations", liquidity.ListOperationsRequest{ProductID: id, Limit: 100}, false)["data"].(map[string]any)
	if len(page["operations"].([]any)) != 4 {
		t.Fatal(page)
	}
}

func TestProductHTTPDataStaleExpiredHashAndConcurrency(t *testing.T) {
	for _, kind := range []string{"terms", "valuation"} {
		t.Run(kind, func(t *testing.T) {
			fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			previewTool, commitTool := "preview_product_"+kind, "commit_product_"+kind
			input := any(productTermsInput(id, 1, "locked_product"))
			if kind == "valuation" {
				input = ProductValuationInput{id, "1100", ""}
			}
			for _, change := range []func(map[string]any){func(p map[string]any) { p["expiresAt"] = time.Now().Add(-time.Second) }, func(p map[string]any) { p["reviewedStateHash"] = "private-wrong-hash" }} {
				p := productDataPreviewHTTP(t, c, previewTool, input)
				productEditPlan(t, fx.service, p["planId"].(string), change)
				if code := productDataCommitCode(t, c, commitTool, p["planId"].(string)); code != "stale_preview" {
					t.Fatal(code)
				}
			}
			stale := productDataPreviewHTTP(t, c, previewTool, input)
			gui := liquidity.NewService(fx.app)
			if _, err := gui.UpdateProductTerms(t.Context(), productTermsInput(id, 1, "locked_product")); err != nil {
				t.Fatal(err)
			}
			if code := productDataCommitCode(t, c, commitTool, stale["planId"].(string)); code != "stale_preview" {
				t.Fatal(code)
			}
			if kind == "terms" {
				input = productTermsInput(id, 2, "locked_product")
			}
			p := productDataPreviewHTTP(t, c, previewTool, input)
			plan := p["planId"].(string)
			var wg sync.WaitGroup
			errs := make(chan error, 6)
			for range 6 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r, e := c.CallTool(t.Context(), &mcp.CallToolParams{Name: commitTool, Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": plan}}})
					if e == nil && r.IsError {
						e = &domain.Error{Code: domain.ErrUnavailable, Message: "concurrent commit failed"}
					}
					errs <- e
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			current, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "terms" && current.Contract.Revision != 3 {
				t.Fatal("duplicate terms", current)
			}
			if kind == "valuation" {
				var count int
				if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM product_operations WHERE kind='value_observation'`).Scan(&count); err != nil || count != 1 {
					t.Fatal(count, err)
				}
			}
		})
	}
}

func TestProductHTTPDataAtomicFailuresAndUnknownRecovery(t *testing.T) {
	for _, kind := range []string{"terms", "valuation"} {
		t.Run(kind, func(t *testing.T) {
			clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
			fx, db, repo := newPersistentProductFixture(t, clock)
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			previewTool, commitTool := "preview_product_"+kind, "commit_product_"+kind
			input := any(productTermsInput(id, 1, "locked_product"))
			stages := []string{"terms:contract", "terms:policy", "terms:receipt"}
			if kind == "valuation" {
				input = ProductValuationInput{id, "1100", ""}
				stages = []string{"operation", "quotes", "links", "complete"}
			}
			for _, stage := range stages {
				before, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
				if err != nil {
					t.Fatal(err)
				}
				p := productDataPreviewHTTP(t, c, previewTool, input)
				var beforeQuotes int
				db.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes`).Scan(&beforeQuotes)
				sqlite.SetProductCommitFailAfter(stage)
				code := productDataCommitCode(t, c, commitTool, p["planId"].(string))
				sqlite.SetProductCommitFailAfter("")
				if code != "unavailable" {
					t.Fatal(stage, code)
				}
				after, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
				if err != nil {
					t.Fatal(err)
				}
				productJSONEqual(t, after, before)
				var quotes, receipts int
				db.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes`).Scan(&quotes)
				db.SQL.QueryRow(`SELECT COUNT(*) FROM app_configuration WHERE key LIKE 'product.terms-mutation.%'`).Scan(&receipts)
				if quotes != beforeQuotes || receipts != 0 {
					t.Fatal("partial atomic write")
				}
			}
			p := productDataPreviewHTTP(t, c, previewTool, input)
			plan := p["planId"].(string)
			op := uuid.NewString()
			fx.service.repository = &failProductSuccessReceipt{ConfigurationRepository: fx.service.repository, fail: true}
			if code := ledgerErrorCode(t, c, commitTool, map[string]any{"operationId": op, "input": map[string]any{"planId": plan}}); code != "operation_outcome_unknown" {
				t.Fatal(code)
			}
			productEditPlan(t, fx.service, plan, func(p map[string]any) { p["expiresAt"] = time.Now().Add(-time.Second) })
			// Restart both application and MCP; recover business evidence before token/expiry.
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
			recovered := productDataCommitHTTP(t, c, commitTool, plan, op)
			if recovered["replayed"] != true {
				t.Fatal(recovered)
			}
			freshInput := input
			if kind == "terms" {
				freshInput = productTermsInput(id, 2, "locked_product")
			}
			uncommitted := productDataPreviewHTTP(t, c, previewTool, freshInput)
			if err := app.WithExclusive(t.Context(), application.ExclusiveRestore, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if code := productDataCommitCode(t, c, commitTool, uncommitted["planId"].(string)); code != "stale_preview" {
				t.Fatal(code)
			}
			if code := ledgerErrorCode(t, c, commitTool, map[string]any{"operationId": op, "input": map[string]any{"planId": uncommitted["planId"]}}); code != "conflict" {
				t.Fatal(code)
			}
			// Same plan with a new outer UUID still recovers after restore fencing.
			replay := productDataCommitHTTP(t, c, commitTool, plan, uuid.NewString())
			productJSONEqual(t, replay, recovered)
		})
	}
}

func TestProductHTTPDataRestartAndOlderSnapshot(t *testing.T) {
	for _, kind := range []string{"terms", "valuation"} {
		t.Run(kind, func(t *testing.T) {
			clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
			fx, db, repo := newPersistentProductFixture(t, clock)
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			input := any(productTermsInput(id, 1, "locked_product"))
			if kind == "valuation" {
				input = ProductValuationInput{id, "1100", ""}
			}
			previewTool, commitTool := "preview_product_"+kind, "commit_product_"+kind
			p := productDataPreviewHTTP(t, c, previewTool, input)
			// Snapshot contains this uncommitted plan, but no mutation/outer success.
			snapshot := filepath.Join(t.TempDir(), "before.db")
			if err := db.SnapshotTo(t.Context(), snapshot); err != nil {
				t.Fatal(err)
			}
			op := uuid.NewString()
			productDataCommitHTTP(t, c, commitTool, p["planId"].(string), op)
			pendingInput := input
			if kind == "terms" {
				pendingInput = productTermsInput(id, 2, "locked_product")
			}
			pending := productDataPreviewHTTP(t, c, previewTool, pendingInput)
			restart := func(database *sqlite.DB, repository *sqlite.Repository) *mcp.ClientSession {
				fx.service.Close()
				app := application.NewService(repository)
				appports.Wire(app)
				appports.AttachSQLiteHistory(app, repository)
				app.SetClock(clock)
				s := New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(database))
				t.Cleanup(s.Close)
				if err := s.Resume(); err != nil {
					t.Fatal(err)
				}
				fx.service = s
				fx.app = app
				return connect(t, s)
			}
			c = restart(db, repo)
			if code := productDataCommitCode(t, c, commitTool, pending["planId"].(string)); code != "stale_preview" {
				t.Fatal(code)
			}
			restored, err := sqlite.Open(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			c = restart(restored, sqlite.NewRepository(restored))
			if _, err := fx.service.GetOperation(op); safeError(err).Code != "not_found" {
				t.Fatal(err)
			}
			if code := productDataCommitCode(t, c, commitTool, p["planId"].(string)); code != "stale_preview" {
				t.Fatal(code)
			}
			current, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
			if err != nil {
				t.Fatal(err)
			}
			if current.Contract.Revision != 1 || current.CurrentValue.CanonicalAmount() != "1000" {
				t.Fatal("restored missing fact was reposted")
			}
		})
	}
}

func TestProductHTTPDataReceiptIntegrity(t *testing.T) {
	for _, kind := range []string{"terms", "valuation"} {
		t.Run(kind, func(t *testing.T) {
			fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			input := any(productTermsInput(id, 1, "locked_product"))
			if kind == "valuation" {
				input = ProductValuationInput{id, "1100", ""}
			}
			p := productDataPreviewHTTP(t, c, "preview_product_"+kind, input)
			productDataCommitHTTP(t, c, "commit_product_"+kind, p["planId"].(string), uuid.NewString())
			snapshot := filepath.Join(t.TempDir(), "valid.db")
			if err := db.SnapshotTo(t.Context(), snapshot); err != nil {
				t.Fatal(err)
			}
			verified, err := sqlite.OpenReadOnlyForVerify(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			verified.Close()
			if kind == "terms" {
				_, err = db.SQL.Exec(`UPDATE app_configuration SET value=json_set(value,'$.receipt.currentValue.currency','EUR') WHERE key LIKE 'product.terms-mutation.%'`)
			} else {
				_, err = db.SQL.Exec(`UPDATE product_operations SET result_json=json_set(result_json,'$.receipt.amount.amount','999') WHERE kind='value_observation'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			broken := filepath.Join(t.TempDir(), "broken.db")
			if err := db.SnapshotTo(t.Context(), broken); err != nil {
				t.Fatal(err)
			}
			if verified, err := sqlite.OpenReadOnlyForVerify(broken); err == nil {
				verified.Close()
				t.Fatal("corrupt receipt accepted by backup verifier")
			}
			if opened, err := sqlite.Open(broken); err == nil {
				opened.Close()
				t.Fatal("corrupt receipt accepted by live startup")
			}
		})
	}
}

func TestProductHTTPDataClosedRules(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	id, _ := productDataSeed(t, fx, c, "locked_product")
	p := productPlanHTTP(t, c, ProductOperationInput{Kind: "settle", Settle: &SettleProductInput{ProductID: id, GrossProceeds: productString("1000"), EffectiveAt: ""}})
	productCommitHTTP(t, c, p["planId"].(string), uuid.NewString())
	if code := ledgerErrorCode(t, c, "preview_product_valuation", ProductValuationInput{id, "1000", ""}); code != "conflict" {
		t.Fatal(code)
	}
	current, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	in := productTermsInput(id, current.Contract.Revision, "locked_product")
	in.Policy = productPolicy("2026-09-30")
	if code := ledgerErrorCode(t, c, "preview_product_terms", in); code != "conflict" {
		t.Fatal(code)
	}
	// Use the unchanged financial terms/policy; only recorded name/note change.
	in.Terms.MaturityOn = productString("2026-09-30")
	in.Policy = productPolicy("2026-09-30")
	in.Terms.Note = productString("Statement checked")
	p = productDataPreviewHTTP(t, c, "preview_product_terms", in)
	r := productDataCommitHTTP(t, c, "commit_product_terms", p["planId"].(string), uuid.NewString())
	after, err := fx.app.Product(t.Context(), current.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Policy.Revision != current.Policy.Revision || after.Contract.State != domain.ProductStateSettled || after.Contract.Name != in.Terms.Name || batchCash(t, c, fx.brokerage) != "1000" {
		t.Fatal(after, r)
	}
	productAssertNetWorth(t, c, "1000")
}

func TestProductHTTPDataSkillExamples(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	deposit, _ := productDataSeed(t, fx, c, "term_deposit")
	p := productDataPreviewHTTP(t, c, "preview_product_terms", skillExample(t, "products", "product-terms", map[string]string{"productId": deposit}))
	call(t, c, "commit_product_terms", skillExample(t, "products", "product-terms-commit", map[string]string{"operationId": uuid.NewString(), "planId": p["planId"].(string)}), false)
	locked, _ := productDataSeed(t, fx, c, "locked_product")
	p = productDataPreviewHTTP(t, c, "preview_product_valuation", skillExample(t, "products", "product-valuation", map[string]string{"productId": locked}))
	call(t, c, "commit_product_valuation", skillExample(t, "products", "product-valuation-commit", map[string]string{"operationId": uuid.NewString(), "planId": p["planId"].(string)}), false)
	productAssertNetWorth(t, c, "2100")
}

func TestProductHTTPValuationUnknownBeforeRemainsUnknown(t *testing.T) {
	fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	id, _ := productDataSeed(t, fx, c, "locked_product")
	product, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic missing observation fixture; no real ledger or generic bypass tool.
	if _, err := db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=?`, product.Contract.InstrumentID.String()); err != nil {
		t.Fatal(err)
	}
	p := productDataPreviewHTTP(t, c, "preview_product_valuation", ProductValuationInput{id, "1100", ""})
	preview := p["preview"].(map[string]any)
	if preview["currentValueBefore"] != nil || preview["netWorthKnown"] != false || preview["netWorthDelta"] != nil || preview["currentValueAfter"].(map[string]any)["amount"] != "1100" {
		t.Fatal(p)
	}
	productDataCommitHTTP(t, c, "commit_product_valuation", p["planId"].(string), uuid.NewString())
	productAssertNetWorth(t, c, "1100")
}

// Legal-shaped receipt corruption must fail both before and after later edits.
func TestProductHTTPTermsLegalReceiptTamperRejected(t *testing.T) {
	for _, later := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_revision", true: "later_revision"}[later], func(t *testing.T) {
			fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "term_deposit")
			p := productDataPreviewHTTP(t, c, "preview_product_terms", productTermsInput(id, 1, "term_deposit"))
			plan := p["planId"].(string)
			original := productDataCommitHTTP(t, c, "commit_product_terms", plan, uuid.NewString())
			if later {
				next := productDataPreviewHTTP(t, c, "preview_product_terms", productTermsInput(id, 2, "term_deposit"))
				productDataCommitHTTP(t, c, "commit_product_terms", next["planId"].(string), uuid.NewString())
			}
			_, err := db.SQL.Exec(`UPDATE app_configuration SET value=json_set(value,'$.receipt.contract.maturityOn','2026-12-02','$.receipt.contract.annualRate','0.07','$.receipt.policy.UnlockOn','2026-12-02') WHERE json_extract(value,'$.id')=? AND key LIKE 'product.terms-mutation.%'`, plan)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "tampered.db")
			if err := db.SnapshotTo(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			if checked, err := sqlite.OpenReadOnlyForVerify(path); err == nil {
				checked.Close()
				t.Fatal("legal-shaped corrupt receipt accepted by backup verification")
			}
			if opened, err := sqlite.Open(path); err == nil {
				opened.Close()
				t.Fatal("legal-shaped corrupt receipt accepted at live startup")
			}
			if code := productDataCommitCode(t, c, "commit_product_terms", plan); code != string(domain.ErrIntegrity) {
				t.Fatal("corrupt receipt was recovered", code)
			}
			if original["product"].(map[string]any)["maturityOn"] != "2026-12-01" {
				t.Fatal(original)
			}
		})
	}
}

func TestProductHTTPTermsReceiptFeesRoundTripAndCommandBinding(t *testing.T) {
	fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	id, _ := productDataSeed(t, fx, c, "locked_product")
	in := productTermsInput(id, 1, "locked_product")
	in.Policy.NormalExitFee = productString("1.25")
	in.Policy.EarlyKind = "allowed"
	in.Policy.EarlyFee = productString("2.5")
	in.Policy.EarlyGrossAmount = productString("980")
	in.Policy.EarlyAmountMode = productString("fixed_gross")
	zero := 0
	in.Policy.EarlySettlementDays = &zero
	in.Policy.EarlyDayBasis = productString("calendar")
	p := productDataPreviewHTTP(t, c, "preview_product_terms", in)
	plan := p["planId"].(string)
	first := productDataCommitHTTP(t, c, "commit_product_terms", plan, uuid.NewString())
	valuePlan := productDataPreviewHTTP(t, c, "preview_product_valuation", ProductValuationInput{id, "1100", ""})
	productDataCommitHTTP(t, c, "commit_product_valuation", valuePlan["planId"].(string), uuid.NewString())
	replay := productDataCommitHTTP(t, c, "commit_product_terms", plan, uuid.NewString())
	replay["replayed"] = false
	productJSONEqual(t, replay, first)
	policy := replay["product"].(map[string]any)["policy"].(map[string]any)
	for field, want := range map[string]string{"normalExitFee": "1.25", "earlyFee": "2.5", "earlyGrossAmount": "980"} {
		m := policy[field].(map[string]any)
		if m["amount"] != want || m["currency"] != "USD" {
			t.Fatal(field, m)
		}
	}
	path := filepath.Join(t.TempDir(), "money.db")
	if err := db.SnapshotTo(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	opened, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
	if _, err := db.SQL.Exec(`UPDATE app_configuration SET value=json_set(value,'$.command.terms.name','Unreviewed command') WHERE key LIKE 'product.terms-mutation.%'`); err != nil {
		t.Fatal(err)
	}
	if code := productDataCommitCode(t, c, "commit_product_terms", plan); code != string(domain.ErrIntegrity) {
		t.Fatal(code)
	}
}

func TestProductHTTPTermsSameRevisionFactsIntegrity(t *testing.T) {
	for _, field := range []string{"contract", "policy"} {
		t.Run(field, func(t *testing.T) {
			fx, db, _ := newPersistentProductFixture(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			p := productDataPreviewHTTP(t, c, "preview_product_terms", productTermsInput(id, 1, "locked_product"))
			productDataCommitHTTP(t, c, "commit_product_terms", p["planId"].(string), uuid.NewString())
			var err error
			if field == "contract" {
				_, err = db.SQL.Exec(`UPDATE product_contracts SET name='Different valid financial row' WHERE id=?`, id)
			} else {
				_, err = db.SQL.Exec(`UPDATE liquidity_policies SET note='Different valid policy row' WHERE source_kind='holding'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "mismatch.db")
			if err := db.SnapshotTo(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			if opened, err := sqlite.OpenReadOnlyForVerify(path); err == nil {
				opened.Close()
				t.Fatal("same-revision facts disagreed with receipt")
			}
		})
	}
}
