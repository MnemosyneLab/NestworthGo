package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Independent routing: shipped examples must validate against their actual
// registered tool, not a duplicated/schema-inferred approximation. State/ID/
// permission/cursor validity is tested separately by real workflow regressions.
var skillSchemaExamples = map[string]map[string][]string{
	"accounts-and-instruments": {"get_account_valuations": {"account-summary", "account-holdings"}, "get_read_item": {"account-evidence"}, "create_account": {"create-brokerage"}, "create_instrument": {"create-fund"}, "update_institution": {"institution-icon"}, "update_account": {"account-clear-note"}},
	"positions-and-trades":     {"preview_batch": {"import-position", "buy", "sell"}, "commit_plan": {"commit"}},
	"cash-and-debt":            {"preview_batch": {"income", "expense", "fx-conversion", "debt-payment"}},
	"market-data":              {"import_market_data": {"fund-nav", "fx-reference", "withdraw-quote"}, "set_fx_source": {"fx-source"}},
	"data-health":              {"get_data_repair_job": {"repair-progress", "repair-evidence"}},
	"products": {
		"get_products": {"product-list", "product-detail"}, "get_upcoming_actions": {"upcoming-actions"}, "list_product_operations": {"product-history"}, "get_liquidity_overview": {"product-liquidity"},
	},
	"product-lifecycle": {
		"preview_product_operation": {"product-open", "product-existing", "product-interest", "product-settle", "product-undo"}, "commit_plan": {"product-commit"},
	},
	"product-terms":     {"preview_product_terms": {"product-terms"}, "commit_plan": {"product-terms-commit"}},
	"product-valuation": {"preview_product_valuation": {"product-valuation"}, "commit_plan": {"product-valuation-commit"}},
	"liquidity-policy": {
		"preview_save_liquidity_policy": {"liquidity-policy-save"}, "preview_reset_liquidity_policy": {"liquidity-policy-reset"}, "commit_plan": {"liquidity-policy-commit"},
	},
	"product-pages": {
		"get_liquidity_overview": {"scale-liquidity-funds", "scale-liquidity-unconfigured", "scale-empty-match"}, "get_products": {"scale-products"}, "get_read_item": {"scale-product-item"},
	},
	"reconciliation-and-corrections": {
		"preview_reconciliation": {"balance-target", "cost-target", "session-linked-target"}, "preview_batch": {"session-linked-income"}, "preview_reconciliation_session": {"session-start", "session-explicit-check", "session-partial-finish"},
		"commit_plan": {"session-commit"}, "get_reconciliation_session_page": {"session-items"},
	},
	"analysis": {
		"get_historical_overview": {"historical-overview", "historical-overview-series", "historical-pending"}, "analyze_period": {"analyze-period"}, "preview_investment_review_rule": {"review-rule-preview"},
	},
	"financial-context": {
		"get_financial_context": {"financial-context", "financial-context-named"}, "get_read_page": {"financial-context-positions-page", "financial-context-gaps-page", "financial-context-evidence-page"},
	},
	"financial-context-item": {"get_financial_context_item": {"financial-context-item"}, "get_read_page": {"financial-context-item-page"}},
	"financial-comparison": {
		"compare_financial_context": {"financial-comparison"}, "get_read_page": {"financial-comparison-positions-page", "financial-comparison-gaps-page", "financial-comparison-evidence-page"},
	},
	"financial-attribution": {"compare_financial_attribution": {"financial-attribution", "financial-attribution-account"}},
	"paging-and-batches":    {"get_read_page": {"scale-page"}},
	"directory-batches":     {"list_instruments": {"scale-instruments"}, "update_instruments_batch": {"scale-metadata"}},
	"market-read-pages": {
		"get_market_data": {"scale-market-current", "scale-market-coverage", "scale-market-history", "scale-market-records"}, "list_agent_market_data": {"scale-agent-retractions", "scale-agent-closures"}, "get_read_item": {"compare-import-item"}, "compare_market_data_import": {"compare-import-summary"},
	},
}

func TestSkillAllToolExamplesMatchRegisteredSchemas(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadWrite)
	root := filepath.Join("..", "..", "skills", "nestworth", "references")
	files, err := filepath.Glob(filepath.Join(root, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	examples := regexp.MustCompile("(?s)<!-- example: ([a-z0-9-]+) -->\\s*```json\\n(.*?)\\n```")
	placeholder := regexp.MustCompile(`\$\{([^}]+)\}`)
	values := map[string]string{
		"effectiveAt": "2026-09-28T12:00:00Z", "actualRFC3339Time": "2026-09-28T12:00:00Z",
		"leftDate": "2026-09-01", "rightDate": "2026-09-28", "actualSourceCivilDate": "2026-09-28",
		"section": "positions", "ref": "account-1", "cursor": "synthetic-cursor", "metadataVersion": strings.Repeat("0", 64), "fundCode": "000001", "userProvidedValue": "1250",
	}
	routed := map[string]bool{}
	for reference, tools := range skillSchemaExamples {
		for _, names := range tools {
			for _, name := range names {
				key := reference + "/" + name
				if _, exists := routed[key]; exists {
					t.Fatal("duplicate routing", key)
				}
				routed[key] = false
			}
		}
	}
	checked := 0
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		reference := strings.TrimSuffix(filepath.Base(file), ".md")
		for _, match := range examples.FindAllStringSubmatch(string(raw), -1) {
			name := match[1]
			key := reference + "/" + name
			tool := ""
			for candidate, names := range skillSchemaExamples[reference] {
				for _, n := range names {
					if n == name {
						tool = candidate
					}
				}
			}
			if tool == "" {
				t.Errorf("unrouted example: %s", key)
				continue
			}
			routed[key] = true
			t.Run(key, func(t *testing.T) {
				text := placeholder.ReplaceAllStringFunc(match[2], func(p string) string {
					k := placeholder.FindStringSubmatch(p)[1]
					if v, ok := values[k]; ok {
						return v
					}
					return "00000000-0000-4000-8000-000000000001"
				})
				var input map[string]any
				if err := jsoncodec.Unmarshal([]byte(text), &input); err != nil {
					t.Fatal(err)
				}
				definition, err := registry.Definition(tool)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := jsoncodec.Marshal(definition.InputSchema)
				var schema jsonschema.Schema
				if err := jsoncodec.Unmarshal(encoded, &schema); err != nil {
					t.Fatal(err)
				}
				resolved, err := schema.Resolve(nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := resolved.Validate(&input); err != nil {
					t.Fatalf("%s schema rejects shipped example: %v", tool, err)
				}
			})
			checked++
		}
	}
	for key, seen := range routed {
		if !seen {
			t.Errorf("mapped example disappeared: %s", key)
		}
	}
	if checked == 0 {
		t.Fatal("no tool examples were validated")
	}
	t.Logf("registered-schema validation: %d tool examples", checked)
}

func TestSkillToolInventoryCoversRegisteredTools(t *testing.T) {
	s, _, _ := fixture(t)
	files, err := filepath.Glob(filepath.Join("..", "..", "skills", "nestworth", "references", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text += "\n" + string(raw)
	}
	for _, tool := range s.toolRegistry(ReadWrite).Definitions() {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(tool.Name) + `\b`).MatchString(text) {
			t.Errorf("actual tool absent from skill references: %s", tool.Name)
		}
	}
}

func TestSkillSharedReadPageDefaultsAndShape(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadWrite); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	guide, err := os.ReadFile(filepath.Join("..", "..", "skills", "nestworth", "references", "paging-and-batches.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"Ordinary initial business reads and every continuation | 25 / 100", "get_read_page({cursor, limit?})", "items/page", "5 minutes", "4 MiB", "8 MiB per capture", "32,768", "16 MiB / 8", "2 builders", "10-second", "65,536", "read_expired/read_revoked", "context_expired/context_revoked"} {
		if !strings.Contains(string(guide), phrase) {
			t.Errorf("contract guidance missing: %s", phrase)
		}
	}
	marketGuide, err := os.ReadFile(filepath.Join("..", "..", "skills", "nestworth", "references", "market-data.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(marketGuide), "Comparison pages default to 25 rows") || !strings.Contains(string(marketGuide), "get_read_page({cursor, limit?})") {
		t.Error("market comparison guide differs from the shared default/continuation contract")
	}
	marketDefinition, err := s.toolRegistry(ReadWrite).Definition("compare_market_data_import")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(marketDefinition.Description, "Comparison pages default to 25 rows, maximum 100") || !strings.Contains(marketDefinition.Description, "get_read_page({cursor,limit?})") {
		t.Error("market comparison discovery differs from its schema/runtime default and continuation")
	}
	definition, err := s.toolRegistry(ReadWrite).Definition("compare_financial_attribution")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition.Description, "get_read_page({cursor,limit?})") || strings.Contains(definition.Description, "get_financial_comparison_page") {
		t.Error("financial attribution discovery must direct continuation through get_read_page")
	}
	if readRetainedCaptureLimit != 8<<20 || readDrillEntryLimit != 32768 || readTTL != 5*time.Minute || contextTTL != readTTL || readCaptureLimit != 4<<20 || contextPackageLimit != readCaptureLimit || readCacheLimit != 16<<20 || contextCacheLimit != readCacheLimit || readCacheCount != 8 || contextCacheCount != readCacheCount || readBuildConcurrency != 2 || readBuildTimeout != 10*time.Second || contextWireLimit != 65536 {
		t.Fatal("runtime budgets changed; review skill contract table")
	}
	rows, err := readRows(make([]string, 151))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind  string
		count int
	}{{"synthetic_contract", 25}, {"market_import_comparison", 25}} {
		captured, err := s.reads.store(s.contextGeneration, readCapture{Kind: c.kind, Query: map[string]any{"view": "full"}, Summary: map[string]any{"source": "synthetic"}, Sections: map[string][]json.RawMessage{"rows": rows}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		page := call(t, client, "get_read_page", ReadPageInput{CaptureID: captured.CaptureID, Section: "rows", Cursor: *captured.Sections["rows"].Page.NextCursor}, false)["data"].(map[string]any)
		if len(page["items"].([]any)) != c.count || page["section"] != "rows" || page["captureId"] != captured.CaptureID {
			t.Fatalf("%s default/identity differs: %v", c.kind, page)
		}
		for _, field := range []string{"summary", "sections", "query"} {
			if _, ok := page[field]; ok {
				t.Fatal("continuation repeats initial field", field)
			}
		}
	}
	params := &mcp.ListToolsParams{}
	total, pages := 0, 0
	for {
		page, err := client.ListTools(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		total += len(page.Tools)
		if len(page.Tools) > 16 || (page.NextCursor != "" && len(page.Tools) != 16) {
			t.Fatal("discovery differs from skill page size", len(page.Tools))
		}
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	if total != len(s.toolRegistry(ReadWrite).Definitions()) || pages < 2 {
		t.Fatal("incomplete tool discovery", total, pages)
	}
}

func TestSkillOptionalBooleanFiltersRejectNull(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadOnly)
	for _, tc := range []struct{ tool, field string }{
		{"get_liquidity_overview", "hasRule"}, {"get_liquidity_overview", "excluded"}, {"list_agent_market_data", "active"},
	} {
		t.Run(tc.tool+"/"+tc.field, func(t *testing.T) {
			definition, err := registry.Definition(tc.tool)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := jsoncodec.Marshal(definition.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			var schema jsonschema.Schema
			if err := jsoncodec.Unmarshal(encoded, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []any{true, false, nil} {
				input := map[string]any{"filter": map[string]any{tc.field: value}}
				err := resolved.Validate(&input)
				if (err == nil) != (value != nil) {
					t.Errorf("%v validation: %v", value, err)
				}
			}
			input := map[string]any{"filter": map[string]any{}}
			if err := resolved.Validate(&input); err != nil {
				t.Fatalf("omitted boolean: %v", err)
			}
		})
	}
}

func TestSkillOmittedOperationRecoveryMatchesDerivation(t *testing.T) {
	raw, err := os.ReadFile("../../skills/nestworth/references/recovery.md")
	if err != nil {
		t.Fatal(err)
	}
	const plan = "11111111-1111-4111-8111-111111111111"
	derived, err := derivedCommitPlanOperationID(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{commitPlanOperationNamespace.String(), plan, derived, "uuid.uuid5(namespace, str(uuid.UUID(plan_id)))", "without submitting", "Missing/pending/unknown receipts do not prove non-execution"} {
		if !strings.Contains(string(raw), text) {
			t.Errorf("recovery recipe missing %q", text)
		}
	}
}
