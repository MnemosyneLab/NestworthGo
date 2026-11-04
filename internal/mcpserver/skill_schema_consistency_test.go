package mcpserver

import (
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Independent routing: shipped examples must validate against their actual
// registered tool, not a duplicated/schema-inferred approximation. State/ID/
// permission/cursor validity is tested separately by real workflow regressions.
var skillSchemaExamples = map[string]map[string][]string{
	"accounts-and-instruments": {"create_account": {"create-brokerage"}, "create_instrument": {"create-fund"}},
	"positions-and-trades":     {"preview_change": {"import-position", "buy", "sell"}, "commit_change": {"commit"}},
	"cash-and-debt":            {"preview_change": {"income", "expense", "fx-conversion", "debt-payment"}},
	"market-data":              {"import_market_data": {"fund-nav", "fx-reference", "withdraw-quote"}, "set_fx_source": {"fx-source"}},
	"data-health":              {"get_data_repair_job": {"repair-progress", "repair-evidence"}},
	"products": {
		"list_products": {"product-list"}, "get_product": {"product-detail"}, "get_upcoming_actions": {"upcoming-actions"}, "list_product_operations": {"product-history"}, "get_liquidity_overview": {"product-liquidity"},
		"preview_product_operation": {"product-open", "product-existing", "product-interest", "product-settle", "product-undo"}, "commit_product_operation": {"product-commit"},
		"preview_product_terms": {"product-terms"}, "commit_product_terms": {"product-terms-commit"}, "preview_product_valuation": {"product-valuation"}, "commit_product_valuation": {"product-valuation-commit"},
		"preview_save_liquidity_policy": {"liquidity-policy-save"}, "commit_save_liquidity_policy": {"liquidity-policy-commit"}, "preview_reset_liquidity_policy": {"liquidity-policy-reset"},
	},
	"reconciliation-and-corrections": {
		"preview_reconciliation": {"balance-target", "cost-target", "session-linked-target"}, "preview_change": {"session-linked-income"}, "preview_reconciliation_session": {"session-start", "session-explicit-check", "session-partial-finish"},
		"commit_reconciliation_session": {"session-commit"}, "get_reconciliation_session_page": {"session-items"},
	},
	"analysis": {
		"get_historical_overview": {"historical-overview"}, "get_historical_overview_series": {"historical-overview-series"}, "analyze_period": {"analyze-period"},
		"get_financial_context": {"financial-context", "financial-context-named"}, "get_read_page": {"financial-context-positions-page", "financial-context-gaps-page", "financial-context-evidence-page", "financial-context-item-page", "financial-comparison-positions-page", "financial-comparison-gaps-page", "financial-comparison-evidence-page"}, "get_financial_context_item": {"financial-context-item"},
		"compare_financial_context": {"financial-comparison"}, "compare_financial_attribution": {"financial-attribution", "financial-attribution-account"},
	},
	"paging-and-batches": {
		"get_liquidity_overview": {"scale-liquidity-funds", "scale-liquidity-unconfigured", "scale-empty-match"}, "list_products": {"scale-products"}, "get_read_item": {"scale-product-item", "compare-import-item"}, "get_read_page": {"scale-page"},
		"get_market_data_batch": {"scale-market-current", "scale-market-coverage", "scale-market-history", "scale-market-records"}, "list_agent_market_data": {"scale-agent-retractions", "scale-agent-closures"}, "list_instruments": {"scale-instruments"}, "update_instruments_batch": {"scale-metadata"}, "compare_market_data_import": {"compare-import-summary"},
	},
}

func TestSkillAllToolExamplesMatchRegisteredSchemas(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(LedgerWrite)
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
	checked, configs := 0, 0
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		reference := strings.TrimSuffix(filepath.Base(file), ".md")
		for _, match := range examples.FindAllStringSubmatch(string(raw), -1) {
			name := match[1]
			if reference == "connection" && (name == "inspector-http-config" || name == "task-tool-discovery") {
				configs++
				continue
			}
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
	if configs != 2 || checked == 0 {
		t.Fatal("configuration/tool example classification failed", configs, checked)
	}
	t.Logf("registered-schema validation: %d tool examples, %d separately classified client/protocol examples", checked, configs)
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
	for _, tool := range s.toolRegistry(LedgerWrite).Definitions() {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(tool.Name) + `\b`).MatchString(text) {
			t.Errorf("actual tool absent from skill references: %s", tool.Name)
		}
	}
}

func TestSkillSharedReadPageDefaultsAndShape(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	guide, err := os.ReadFile(filepath.Join("..", "..", "skills", "nestworth", "references", "paging-and-batches.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"Ordinary get_read_page | 25 / 100", "Import comparison initial capture and pages | 100 / 100", "section/items/page", "5 minutes", "4 MiB", "16 MiB / 8", "2 builders", "10-second", "65,536", "read_expired/read_revoked", "context_expired/context_revoked"} {
		if !strings.Contains(string(guide), phrase) {
			t.Errorf("contract guidance missing: %s", phrase)
		}
	}
	if readTTL != 5*time.Minute || contextTTL != readTTL || readCaptureLimit != 4<<20 || contextPackageLimit != readCaptureLimit || readCacheLimit != 16<<20 || contextCacheLimit != readCacheLimit || readCacheCount != 8 || contextCacheCount != readCacheCount || readBuildConcurrency != 2 || readBuildTimeout != 10*time.Second || contextWireLimit != 65536 {
		t.Fatal("runtime budgets changed; review skill contract table")
	}
	rows, err := readRows(make([]string, 151))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind  string
		count int
	}{{"synthetic_contract", 25}, {"market_import_comparison", 100}} {
		captured, err := s.reads.store(s.contextGeneration, readCapture{Kind: c.kind, Query: map[string]any{"view": "full"}, Summary: map[string]any{"source": "synthetic"}, Sections: map[string][]json.RawMessage{"rows": rows}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		page := call(t, client, "get_read_page", ReadPageInput{CaptureID: captured.CaptureID, Section: "rows", Cursor: *captured.Sections["rows"].Page.NextCursor}, false)["data"].(map[string]any)
		if len(page["items"].([]any)) != c.count || page["section"] != "rows" || page["contentHash"] != captured.ContentHash {
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
	if total != len(s.toolRegistry(LedgerWrite).Definitions()) || pages < 2 {
		t.Fatal("incomplete tool discovery", total, pages)
	}
}
