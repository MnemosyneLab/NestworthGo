package mcpserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestDerivedCommitPlanOperationIDIsStable(t *testing.T) {
	t.Parallel()
	namespace := uuid.NewSHA1(uuid.NameSpaceURL, []byte("nestworth:mcp:commit_plan:operation"))
	if commitPlanOperationNamespace != namespace || namespace.String() != "f1f65af0-bbb8-5d57-af9c-b624de715dcb" {
		t.Fatalf("namespace = %s", commitPlanOperationNamespace)
	}
	const planID = "11111111-1111-4111-8111-111111111111"
	got, err := derivedCommitPlanOperationID(planID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(got)
	if err != nil || parsed.Version() != 5 || got != "ea140a3e-eff3-54df-b11b-7a05f1d01270" {
		t.Fatalf("derived id = %s version %d, err %v", got, parsed.Version(), err)
	}
	again, err := derivedCommitPlanOperationID(planID)
	if err != nil || again != got {
		t.Fatalf("repeat = %s, %v", again, err)
	}
	upper, err := derivedCommitPlanOperationID(strings.ToUpper(planID))
	if err != nil || upper != got {
		t.Fatalf("canonical case = %s, %v", upper, err)
	}
	other, err := derivedCommitPlanOperationID("22222222-2222-4222-8222-222222222222")
	if err != nil || other == got {
		t.Fatalf("different plan = %s, %v", other, err)
	}
	if _, err = derivedCommitPlanOperationID("not-a-uuid"); err == nil || safeError(err).Code != "validation" {
		t.Fatalf("invalid plan = %v", err)
	}
	explicit := "33333333-3333-4333-8333-333333333333"
	kept, err := commitPlanOperationID("  "+explicit+"  ", planID)
	if err != nil || kept != "  "+explicit+"  " {
		t.Fatalf("explicit id = %q, %v", kept, err)
	}
	blank, err := commitPlanOperationID(" \t", planID)
	if err != nil || blank != got {
		t.Fatalf("blank id = %s, %v", blank, err)
	}
}

func TestCommitPlanOperationIDIsOptionalOnlyThere(t *testing.T) {
	t.Parallel()
	s, _, _ := fixture(t)
	registry := s.toolRegistry(LedgerWrite)
	sawCommit := false
	for _, definition := range registry.Definitions() {
		root := schemaObject(t, definition.InputSchema)
		required := requiredSet(root)
		props, _ := root["properties"].(map[string]any)
		_, hasOperation := props["operationId"]
		if definition.Name == "commit_plan" {
			sawCommit = true
			if definition.Effect != agentcapability.Write || required["operationId"] || !required["input"] || !hasOperation {
				t.Fatalf("commit_plan schema = required %v effect %s", required, definition.Effect)
			}
			if props["operationId"].(map[string]any)["type"] != "string" {
				t.Fatalf("operationId schema = %v", props["operationId"])
			}
			continue
		}
		if definition.Effect == agentcapability.Write && hasOperation && !required["operationId"] {
			t.Errorf("%s write tool no longer requires operationId", definition.Name)
		}
	}
	if !sawCommit {
		t.Fatal("commit_plan is not registered")
	}
	if _, err := s.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	call(t, c, "create_member", map[string]any{"input": map[string]string{"name": "Synthetic"}}, true)
}

func TestCommitPlanOmittedOperationIDUsesDerivedReceipt(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := ledgerPreview(t, c, incomeRequest(fx.checking, "40"))
	planID := plan["planId"].(string)
	derived, err := derivedCommitPlanOperationID(planID)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"input": map[string]any{"planId": planID}}
	receipt := commitPlanData(t, c, args)
	if receipt["id"] != derived || receipt["tool"] != "commit_change" {
		t.Fatalf("omitted commit = %v", receipt)
	}
	activityID := ledgerActivityID(t, receipt["result"].(map[string]any))
	if retry := commitPlanData(t, c, args); retry["id"] != derived || ledgerActivityID(t, retry["result"].(map[string]any)) != activityID {
		t.Fatalf("omitted retry = %v", retry)
	}
	loaded := call(t, c, "get_operation", IDInput{ID: derived}, false)["data"].(map[string]any)
	if loaded["id"] != derived || loaded["status"] != "succeeded" || loaded["tool"] != "commit_change" {
		t.Fatalf("get_operation = %v", loaded)
	}
	// Same planId with a different explicit operationId recovers the ledger
	// mutation and does not post a second activity.
	recovered := ledgerCommit(t, c, uuid.NewString(), planID)
	if ledgerActivityID(t, recovered) != activityID || ledgerActivityCount(t, fx.app) != 1 || ledgerCash(t, c, fx.checking) != "40" {
		t.Fatalf("explicit recovery duplicated the plan: activities=%d cash=%s", ledgerActivityCount(t, fx.app), ledgerCash(t, c, fx.checking))
	}
	if err = markOperationPending(t, fx, derived); err != nil {
		t.Fatal(err)
	}
	if got := ledgerActivityID(t, commitPlanData(t, c, args)["result"].(map[string]any)); got != activityID || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("unknown-result retry duplicated the plan")
	}
	// A different spelling of the same planId derives the same id but hashes
	// as different input, so it conflicts and does not post.
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"input": map[string]any{"planId": strings.ToUpper(planID)}}); code != "conflict" {
		t.Fatalf("different planId spelling = %s", code)
	}
	if ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("conflicting spelling posted")
	}
}

func TestCommitPlanDerivedIDConflictsWithExplicitReceipt(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	first := ledgerPreview(t, c, incomeRequest(fx.checking, "15"))
	second := ledgerPreview(t, c, incomeRequest(fx.checking, "25"))
	firstID := first["planId"].(string)
	secondID := second["planId"].(string)
	stolen, err := derivedCommitPlanOperationID(secondID)
	if err != nil {
		t.Fatal(err)
	}
	// The explicit call consumes the id that omitting operationId on the
	// other plan would derive. The later omit must conflict and leave that
	// receipt in place.
	posted := ledgerCommit(t, c, stolen, firstID)
	if ledgerActivityID(t, posted) == "" || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal(posted)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"input": map[string]any{"planId": secondID}}); code != "conflict" {
		t.Fatalf("derived collision = %s", code)
	}
	if ledgerActivityCount(t, fx.app) != 1 || ledgerCash(t, c, fx.checking) != "15" {
		t.Fatalf("collision posted: activities=%d cash=%s", ledgerActivityCount(t, fx.app), ledgerCash(t, c, fx.checking))
	}
	kept := call(t, c, "get_operation", IDInput{ID: stolen}, false)["data"].(map[string]any)
	if kept["tool"] != "commit_change" || kept["status"] != "succeeded" {
		t.Fatalf("explicit receipt replaced: %v", kept)
	}
	// The matching explicit id and input still replay that receipt.
	replay := ledgerCommit(t, c, stolen, firstID)
	if ledgerActivityID(t, replay) != ledgerActivityID(t, posted) || ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("matching explicit retry did not replay")
	}
}

func TestCommitPlanDerivedIDConflictsWithAnotherTool(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := ledgerPreview(t, c, incomeRequest(fx.checking, "10"))
	planID := plan["planId"].(string)
	derived, err := derivedCommitPlanOperationID(planID)
	if err != nil {
		t.Fatal(err)
	}
	call(t, c, "create_member", map[string]any{"operationId": derived, "input": map[string]string{"name": "Synthetic member"}}, false)
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"input": map[string]any{"planId": planID}}); code != "conflict" {
		t.Fatalf("cross-tool collision = %s", code)
	}
	if ledgerActivityCount(t, fx.app) != 0 {
		t.Fatal("cross-tool collision posted the plan")
	}
	kept := call(t, c, "get_operation", IDInput{ID: derived}, false)["data"].(map[string]any)
	if kept["tool"] != "create_member" || kept["status"] != "succeeded" {
		t.Fatalf("directory receipt replaced: %v", kept)
	}
}

func TestCommitPlanExplicitOperationIDUnchanged(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := ledgerPreview(t, c, incomeRequest(fx.checking, "8"))
	planID := plan["planId"].(string)
	operationID := uuid.NewString()
	derived, err := derivedCommitPlanOperationID(planID)
	if err != nil {
		t.Fatal(err)
	}
	if operationID == derived {
		t.Fatal("fixture operation id collided with the derived id")
	}
	args := map[string]any{"operationId": operationID, "input": map[string]any{"planId": planID}}
	receipt := commitPlanData(t, c, args)
	if receipt["id"] != operationID {
		t.Fatalf("explicit receipt id = %v", receipt["id"])
	}
	if retry := commitPlanData(t, c, args); retry["id"] != operationID {
		t.Fatalf("explicit retry = %v", retry["id"])
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": "not-a-uuid", "input": map[string]any{"planId": planID}}); code != "validation" {
		t.Fatalf("bad explicit id = %s", code)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"input": map[string]any{"planId": "not-a-uuid"}}); code != "validation" {
		t.Fatalf("omitted id with bad plan = %s", code)
	}
	if _, err = fx.service.GetOperation(derived); err == nil || safeError(err).Code != "not_found" {
		t.Fatalf("invalid omit consumed an operation: %v", err)
	}
}

func TestCommitPlanOmittedLiquidityDoesNotReplayPending(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	input := liquidity.SavePolicyRequest{Source: liquidity.SourceRefDTO{Kind: "account_value", AccountID: fx.checking}, Policy: ordinaryPolicy()}
	plan := productDataPreviewHTTP(t, c, "preview_save_liquidity_policy", input)
	planID := plan["planId"].(string)
	derived, err := derivedCommitPlanOperationID(planID)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"input": map[string]any{"planId": planID}}
	first := commitPlanData(t, c, args)
	if first["id"] != derived || first["tool"] != "commit_save_liquidity_policy" {
		t.Fatalf("omitted policy commit = %v", first)
	}
	if retry := commitPlanData(t, c, args); retry["id"] != derived {
		t.Fatalf("omitted policy retry = %v", retry["id"])
	}
	before := policyOverview(t, c)
	// A different explicit operationId on the used plan is still stale and
	// must not write another revision.
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "stale_preview" {
		t.Fatalf("different explicit operationId = %s", code)
	}
	if !reflect.DeepEqual(before, policyOverview(t, c)) {
		t.Fatal("stale explicit retry changed the policy")
	}
	if err = markOperationPending(t, fx, derived); err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", args); code != "operation_outcome_unknown" {
		t.Fatalf("pending omitted retry = %s", code)
	}
	unknown := call(t, c, "get_operation", IDInput{ID: derived}, false)["data"].(map[string]any)
	if unknown["status"] != "unknown" || unknown["result"] != nil {
		t.Fatalf("pending receipt = %v", unknown)
	}
	if !reflect.DeepEqual(before, policyOverview(t, c)) {
		t.Fatal("pending omitted retry replayed the policy write")
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "stale_preview" {
		t.Fatalf("new operationId beside pending receipt = %s", code)
	}
}

func incomeRequest(accountID, amount string) map[string]any {
	return map[string]any{"kind": "money_added", "accountId": accountID, "amount": amount, "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}
}

func commitPlanData(t *testing.T, c *mcp.ClientSession, args map[string]any) map[string]any {
	t.Helper()
	data := call(t, c, "commit_plan", args, false)["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Fatalf("commit receipt = %v", data)
	}
	return data
}

func markOperationPending(t *testing.T, fx ledgerFixture, operationID string) error {
	t.Helper()
	path, err := fx.service.operationPath(operationID)
	if err != nil {
		return err
	}
	receipt, err := fx.service.GetOperation(operationID)
	if err != nil {
		return err
	}
	receipt.Status, receipt.Result = "pending", nil
	return fx.service.writePrivate(path, receipt)
}
