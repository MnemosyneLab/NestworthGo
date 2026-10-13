package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
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

func ordinaryPolicy() application.ProductPolicyInput {
	days, earlyDays := 3, 2
	return application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: productString("2026-10-02"), SettlementDays: &days, DayBasis: productString("weekdays"), ReceiptOnOverride: productString("2026-10-08"), AccessibleAmountCap: productString("123.1234"), NormalExitFee: productString("1.0001"), EarlyKind: "allowed", EarlySettlementDays: &earlyDays, EarlyDayBasis: productString("calendar"), EarlyFee: productString("2.0002"), EarlyAmountMode: productString("fixed_gross"), EarlyGrossAmount: productString("100.0003"), Note: productString("Synthetic withdrawal rules")}
}

func policyOverview(t *testing.T, c *mcp.ClientSession) map[string]any {
	t.Helper()
	return call(t, c, "get_liquidity_overview", LiquidityOverviewInput{CustomHorizonOn: productString("2026-10-09"), IncludeEarlyWithdrawal: true}, false)["data"].(map[string]any)
}

func assertPolicyGUIParity(t *testing.T, fx ledgerFixture, c *mcp.ClientSession) {
	t.Helper()
	gui, err := liquidity.NewService(fx.app).Overview(t.Context(), liquidity.OverviewRequest{CustomHorizonOn: productString("2026-10-09"), IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(gui)
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	got := policyOverview(t, c)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MCP liquidity differs from GUI: %v vs %v", got, want)
	}
}

func TestLiquidityPolicyHTTPPermissionsAndSchema(t *testing.T) {
	t.Parallel()
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
		for _, name := range []string{"preview_save_liquidity_policy", "commit_save_liquidity_policy", "preview_reset_liquidity_policy", "commit_reset_liquidity_policy"} {
			if (found[name] != nil) != (mode == LedgerWrite) {
				t.Fatalf("%s %s", mode, name)
			}
			if mode != LedgerWrite {
				r, e := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if e == nil && !r.IsError {
					t.Fatal(name)
				}
				continue
			}
			raw, _ := json.Marshal(found[name].InputSchema)
			if !strings.Contains(string(raw), `"additionalProperties":false`) {
				t.Fatal(string(raw))
			}
			if found[name].Annotations.ReadOnlyHint != strings.HasPrefix(name, "preview_") {
				t.Fatal(found[name])
			}
			if name == "preview_save_liquidity_policy" {
				for _, field := range []string{"sourceRef", "expectedRevision", "accessibleAmountCap", "unlockOn", "receiptOnOverride", "normalExitFee", "earlySettlementDays", "earlyDayBasis", "earlyFee", "earlyAmountMode", "earlyGrossAmount", "note"} {
					if !strings.Contains(string(raw), `"`+field+`"`) {
						t.Fatal(field)
					}
				}
			}
		}
		caps, _ := json.Marshal(call(t, c, "get_context", Empty{}, false)["data"].(map[string]any)["capabilities"])
		if strings.Contains(string(caps), "liquidity_policy_write") != (mode == LedgerWrite) {
			t.Fatal(string(caps))
		}
	}
}

func TestLiquidityPolicyHTTPAllSourcesRoundTrip(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	for _, account := range []string{fx.checking, fx.brokerage} {
		p := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": account, "amount": "1000", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-29T12:00:00Z"})
		ledgerCommit(t, c, uuid.NewString(), p["planId"].(string))
	}
	// Create an ordinary holding through the real financial workflow.
	p := ledgerPreview(t, c, map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "unitCost": "20", "currency": "USD", "effectiveAt": "2026-09-29T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), p["planId"].(string))
	holdings := batchHoldings(t, c, fx.brokerage)
	holding := holdings[0].(map[string]any)["id"].(string)
	call(t, c, "import_market_data", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"instrumentId": fx.instrument, "currency": "USD", "kind": "latest", "value": "20", "quotedAt": "2026-09-29T12:00:00Z", "sourceTitle": "Synthetic price"}}}}, false)
	factsBefore := call(t, c, "get_overview", Empty{}, false)["data"]
	holdingsBefore := batchHoldings(t, c, fx.brokerage)

	refs := []liquidity.SourceRefDTO{{Kind: "account_value", AccountID: fx.checking}, {Kind: "account_cash", AccountID: fx.brokerage, Currency: productString("USD")}, {Kind: "holding", AccountID: fx.brokerage, HoldingID: &holding}}
	for _, ref := range refs {
		t.Run(ref.Kind, func(t *testing.T) {
			domainRef, err := ref.ToDomain()
			if err != nil {
				t.Fatal(err)
			}
			reservation, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domainRef, Label: "Synthetic reserve", Amount: "10"})
			if err != nil {
				t.Fatal(err)
			}
			before := policyOverview(t, c)
			input := liquidity.SavePolicyRequest{Source: ref, ExpectedRevision: 0, Policy: ordinaryPolicy()}
			p := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
			if !reflect.DeepEqual(before, policyOverview(t, c)) {
				t.Fatal("preview changed facts")
			}
			if p["beforeOrigin"] != "assumed" || p["afterOrigin"] != "explicit" {
				t.Fatal(p)
			}
			id, op := p["planId"].(string), uuid.NewString()
			result := productDataCommitHTTP(t, c, "commit_save_liquidity_policy", id, op)
			if result["revision"] != float64(1) || result["accessibleAmountCap"].(map[string]any)["amount"] != "123.1234" || result["earlyGrossAmount"].(map[string]any)["amount"] != "100.0003" {
				t.Fatal(result)
			}
			if retry := productDataCommitHTTP(t, c, "commit_save_liquidity_policy", id, op); !reflect.DeepEqual(result, retry) {
				t.Fatal("retry differs")
			}
			if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", id); code != "stale_preview" {
				t.Fatal(code)
			}
			assertPolicyGUIParity(t, fx, c)
			for _, rawSource := range policyOverview(t, c)["sources"].([]any) {
				source := rawSource.(map[string]any)
				if !reflect.DeepEqual(source["sourceRef"], result["sourceRef"]) {
					continue
				}
				normal := source["normalRoute"].(map[string]any)
				early := source["earlyRoute"].(map[string]any)
				if normal["netNative"].(map[string]any)["amount"] != "122.1233" || early["netNative"].(map[string]any)["amount"] != "98.0001" {
					t.Fatal(normal, early)
				}
			}
			// Full replacement clears cap/date/overrides/note/early fields and keeps known zero fee.
			input.ExpectedRevision = 1
			input.Policy = application.ProductPolicyInput{AccessKind: "on_request", EarlyKind: "not_allowed", NormalExitFee: productString("0")}
			p = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
			result = productDataCommitHTTP(t, c, "commit_save_liquidity_policy", p["planId"].(string), uuid.NewString())
			for _, field := range []string{"accessibleAmountCap", "unlockOn", "receiptOnOverride", "note", "earlyFee", "earlyGrossAmount", "settlementDays"} {
				if result[field] != nil {
					t.Fatal(field, result)
				}
			}
			if result["revision"] != float64(2) {
				t.Fatal(result)
			}
			assertPolicyGUIParity(t, fx, c)
			p = productDataPreviewHTTP(t, c, "preview_reset_liquidity_policy", liquidity.ResetPolicyRequest{Source: ref, ExpectedRevision: 2})
			if p["afterOrigin"] != "assumed" || p["after"].(map[string]any)["revision"] != float64(0) {
				t.Fatal(p)
			}
			result = productDataCommitHTTP(t, c, "commit_reset_liquidity_policy", p["planId"].(string), uuid.NewString())
			if result["revision"] != float64(0) {
				t.Fatal(result)
			}
			assertPolicyGUIParity(t, fx, c)
			reserves, err := fx.app.ListLiquidityReservations(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range reserves {
				if r.ID == reservation.ID {
					found = reflect.DeepEqual(r, reservation)
				}
			}
			if !found {
				t.Fatal("policy write changed reservation")
			}
		})
	}
	if !reflect.DeepEqual(factsBefore, call(t, c, "get_overview", Empty{}, false)["data"]) || !reflect.DeepEqual(holdingsBefore, batchHoldings(t, c, fx.brokerage)) {
		t.Fatal("policy workflow changed ledger/valuation facts")
	}
}

func TestLiquidityPolicyHTTPValidationCASAndRecovery(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	input := liquidity.SavePolicyRequest{Source: liquidity.SourceRefDTO{Kind: "account_value", AccountID: fx.checking}, Policy: ordinaryPolicy()}
	for _, change := range []func(*application.ProductPolicyInput){
		func(p *application.ProductPolicyInput) { p.UnlockOn = productString("2026-02-30") },
		func(p *application.ProductPolicyInput) {
			p.AccessKind = "on_request"
			p.UnlockOn = productString("invalid")
		},
		func(p *application.ProductPolicyInput) { p.ReceiptOnOverride = productString("2026-13-01") },
		func(p *application.ProductPolicyInput) { v := -1; p.SettlementDays = &v },
		func(p *application.ProductPolicyInput) { v := 366; p.EarlySettlementDays = &v },
		func(p *application.ProductPolicyInput) { p.NormalExitFee = productString("-1") },
		func(p *application.ProductPolicyInput) { p.EarlyFee = productString("-1") },
		func(p *application.ProductPolicyInput) { p.AccessibleAmountCap = productString("bad") },
		func(p *application.ProductPolicyInput) { p.EarlyGrossAmount = nil },
		func(p *application.ProductPolicyInput) { p.EarlyAmountMode = productString("unsupported") },
		func(p *application.ProductPolicyInput) { p.DayBasis = productString("business") },
		func(p *application.ProductPolicyInput) { p.EarlyFee = nil },
	} {
		bad := input
		bad.Policy = ordinaryPolicy()
		change(&bad.Policy)
		before := policyOverview(t, c)
		if code := ledgerErrorCode(t, c, "preview_save_liquidity_policy", bad); code != "validation" {
			t.Fatal(code, bad)
		}
		if !reflect.DeepEqual(before, policyOverview(t, c)) {
			t.Fatal("invalid input wrote facts")
		}
	}
	p := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	// Wrong family must fail without invalidating the reviewed plan.
	if code := productDataCommitCode(t, c, "commit_reset_liquidity_policy", p["planId"].(string)); code != "validation" {
		t.Fatal(code)
	}
	productDataCommitHTTP(t, c, "commit_save_liquidity_policy", p["planId"].(string), uuid.NewString())
	if code := ledgerErrorCode(t, c, "preview_save_liquidity_policy", input); code != "revision_conflict" {
		t.Fatal(code)
	}
	if code := ledgerErrorCode(t, c, "preview_reset_liquidity_policy", liquidity.ResetPolicyRequest{Source: input.Source, ExpectedRevision: 0}); code != "revision_conflict" {
		t.Fatal(code)
	}
	input.ExpectedRevision = 1
	p = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	// A GUI edit fences the plan even if a later reset/recreate returns revision 1.
	if _, err := liquidity.NewService(fx.app).SavePolicy(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	before := policyOverview(t, c)
	if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", p["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	if !reflect.DeepEqual(before, policyOverview(t, c)) {
		t.Fatal("stale commit wrote")
	}
	input.ExpectedRevision = 2
	p = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	productEditPlan(t, fx.service, p["planId"].(string), func(p map[string]any) { p["expiresAt"] = time.Now().Add(-time.Minute).Format(time.RFC3339Nano) })
	if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", p["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	p = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	op := uuid.NewString()
	id := p["planId"].(string)
	productDataCommitHTTP(t, c, "commit_save_liquidity_policy", id, op)
	// Simulate interrupted receipt persistence after the business write.
	path := filepath.Join(fx.service.dir, "operations", op+".json")
	receipt, err := fx.service.GetOperation(op)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Status = "pending"
	receipt.Result = nil
	if err := fx.service.writePrivate(path, receipt); err != nil {
		t.Fatal(err)
	}
	newer := input
	newer.ExpectedRevision = 3
	newer.Policy.Note = productString("Concurrent GUI rule edit")
	if _, err := liquidity.NewService(fx.app).SavePolicy(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
	current := policyOverview(t, c)
	args := map[string]any{"operationId": op, "input": map[string]any{"planId": id}}
	if code := ledgerErrorCode(t, c, "commit_save_liquidity_policy", args); code != "operation_outcome_unknown" {
		t.Fatal(code)
	}
	if !reflect.DeepEqual(current, policyOverview(t, c)) {
		t.Fatal("ambiguous retry overwrote newer rules")
	}
	if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", id); code != "stale_preview" {
		t.Fatal(code)
	}
	args["input"] = map[string]any{"planId": uuid.NewString()}
	if code := ledgerErrorCode(t, c, "commit_save_liquidity_policy", args); code != "conflict" {
		t.Fatal(code)
	}
	input.Source.AccountID = fx.debt
	input.ExpectedRevision = 0
	if code := ledgerErrorCode(t, c, "preview_save_liquidity_policy", input); code == "" {
		t.Fatal("liability accepted")
	}
}

func TestLiquidityPolicyHTTPManagedRejection(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	for _, kind := range []string{"term_deposit", "locked_product"} {
		id, _ := productDataSeed(t, fx, c, kind)
		product := call(t, c, "get_product", IDInput{ID: id}, false)["data"].(map[string]any)["product"].(map[string]any)
		holding := product["holdingId"].(string)
		ref := liquidity.SourceRefDTO{Kind: "holding", AccountID: fx.brokerage, HoldingID: &holding}
		for _, tool := range []string{"preview_save_liquidity_policy", "preview_reset_liquidity_policy"} {
			var input any = liquidity.ResetPolicyRequest{Source: ref, ExpectedRevision: 1}
			if tool == "preview_save_liquidity_policy" {
				input = liquidity.SavePolicyRequest{Source: ref, ExpectedRevision: 1, Policy: ordinaryPolicy()}
			}
			if code := ledgerErrorCode(t, c, tool, input); code != string(domain.ErrManagedPosition) {
				t.Fatal(code)
			}
		}
		p := productDataPreviewHTTP(t, c, "preview_product_terms", productTermsInput(id, 1, kind))
		productDataCommitHTTP(t, c, "commit_product_terms", p["planId"].(string), uuid.NewString())
		assertPolicyGUIParity(t, fx, c)
	}
}

func TestLiquidityPolicyHTTPExamplesNullableModesAndGuards(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	args := skillExample(t, "products", "liquidity-policy-save", map[string]string{"accountId": fx.checking})
	p := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", args)
	commit := skillExample(t, "products", "liquidity-policy-commit", map[string]string{"operationId": uuid.NewString(), "planId": p["planId"].(string)})
	call(t, c, "commit_save_liquidity_policy", commit, false)
	p = productDataPreviewHTTP(t, c, "preview_reset_liquidity_policy", skillExample(t, "products", "liquidity-policy-reset", map[string]string{"accountId": fx.checking}))
	commit = skillExample(t, "products", "liquidity-policy-commit", map[string]string{"operationId": uuid.NewString(), "planId": p["planId"].(string)})
	call(t, c, "commit_reset_liquidity_policy", commit, false)
	ref := liquidity.SourceRefDTO{Kind: "account_value", AccountID: fx.checking}
	revision := 0
	for _, kind := range []string{"unknown", "excluded", "on_request", "on_date"} {
		policy := ordinaryPolicy()
		policy.AccessKind = kind
		policy.NormalExitFee = nil
		policy.SettlementDays = nil
		policy.EarlyAmountMode = productString("current_value")
		policy.EarlyGrossAmount = nil
		policy.Note = nil
		input := liquidity.SavePolicyRequest{Source: ref, ExpectedRevision: revision, Policy: policy}
		p = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
		result := productDataCommitHTTP(t, c, "commit_save_liquidity_policy", p["planId"].(string), uuid.NewString())
		if result["normalExitFee"] != nil || result["settlementDays"] != nil || result["earlyAmountMode"] != "current_value" || result["note"] != nil {
			t.Fatal(result)
		}
		revision++
		assertPolicyGUIParity(t, fx, c)
	}
	// Schema distinguishes full replacement from partial edits and rejects numeric money.
	for _, bad := range []any{
		map[string]any{"sourceRef": ref, "expectedRevision": revision, "policy": map[string]any{"accessKind": "unknown", "earlyKind": "unknown"}},
		map[string]any{"sourceRef": ref, "expectedRevision": revision, "policy": map[string]any{"normalExitFee": 12}},
		map[string]any{"sourceRef": ref, "expectedRevision": revision, "policy": ordinaryPolicy(), "unexpected": "private"},
		map[string]any{"sourceRef": map[string]any{"kind": "account_value", "accountId": fx.checking, "holdingId": uuid.NewString()}, "expectedRevision": revision, "policy": ordinaryPolicy()},
	} {
		if code := ledgerErrorCode(t, c, "preview_save_liquidity_policy", bad); code != "validation" {
			t.Fatal(code, bad)
		}
	}
	p = productDataPreviewHTTP(t, c, "preview_reset_liquidity_policy", liquidity.ResetPolicyRequest{Source: ref, ExpectedRevision: revision})
	if err := fx.app.ArchiveAccount(t.Context(), domain.AccountID(fx.checking), true); err != nil {
		t.Fatal(err)
	}
	if code := productDataCommitCode(t, c, "commit_reset_liquidity_policy", p["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	if code := ledgerErrorCode(t, c, "preview_reset_liquidity_policy", liquidity.ResetPolicyRequest{Source: ref, ExpectedRevision: revision}); code == "" {
		t.Fatal("archived source accepted")
	}
}

func TestLiquidityPolicyHTTPPersistentRestartRestoreAndABA(t *testing.T) {
	t.Parallel()
	clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	fx, db, repo := newPersistentProductFixture(t, clock)
	c := ledgerSession(t, fx)
	input := liquidity.SavePolicyRequest{Source: liquidity.SourceRefDTO{Kind: "account_value", AccountID: fx.checking}, Policy: ordinaryPolicy()}
	p := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	op := uuid.NewString()
	id := p["planId"].(string)
	result := productDataCommitHTTP(t, c, "commit_save_liquidity_policy", id, op)
	input.ExpectedRevision = 1
	pending := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	// Reset and recreate preserve revision 1 but must never admit this old plan.
	gui := liquidity.NewService(fx.app)
	if _, err := gui.ResetPolicy(t.Context(), liquidity.ResetPolicyRequest{Source: input.Source, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	recreate := input
	recreate.ExpectedRevision = 0
	recreate.Policy.Note = productString("New generation")
	if _, err := gui.SavePolicy(t.Context(), recreate); err != nil {
		t.Fatal(err)
	}
	if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", pending["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	pending = productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	fx.service.Close()
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo)
	app.SetClock(clock)
	fx.app = app
	fx.service = New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(db))
	t.Cleanup(fx.service.Close)
	if err := fx.service.Resume(); err != nil {
		t.Fatal(err)
	}
	c = connect(t, fx.service)
	if retry := productDataCommitHTTP(t, c, "commit_save_liquidity_policy", id, op); !reflect.DeepEqual(result, retry) {
		t.Fatal("restart changed original receipt")
	}
	if code := productDataCommitCode(t, c, "commit_save_liquidity_policy", pending["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	pending = productDataPreviewHTTP(t, c, "preview_reset_liquidity_policy", liquidity.ResetPolicyRequest{Source: input.Source, ExpectedRevision: 1})
	if err := app.WithExclusive(t.Context(), application.ExclusiveRestore, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if code := productDataCommitCode(t, c, "commit_reset_liquidity_policy", pending["planId"].(string)); code != "stale_preview" {
		t.Fatal(code)
	}
	assertPolicyGUIParity(t, fx, c)
}
