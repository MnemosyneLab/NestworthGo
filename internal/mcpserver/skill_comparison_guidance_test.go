package mcpserver

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var skillSections = []string{"positions", "gaps", "evidence"}

func skillPageState(t *testing.T, data any) (map[string]FinancialContextPageInfo, map[string][]json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	states := map[string]FinancialContextPageInfo{}
	rows := map[string][]json.RawMessage{}
	for _, section := range skillSections {
		var info FinancialContextPageInfo
		if err = json.Unmarshal(fields[section+"Page"], &info); err != nil {
			t.Fatal(err)
		}
		states[section] = info
	}
	if nested, ok := fields["content"]; ok {
		if err = json.Unmarshal(nested, &fields); err != nil {
			t.Fatal(err)
		}
	}
	for _, section := range skillSections {
		var sectionRows []json.RawMessage
		if err = json.Unmarshal(fields[section], &sectionRows); err != nil {
			t.Fatal(err)
		}
		rows[section] = sectionRows
	}
	return states, rows
}

// Round-robin pages intentionally finish sections at different times. Other
// returned descriptors must not reactivate a saved completed section.
func skillIndependentPages(t *testing.T, client *mcp.ClientSession, kind, id, ref string, initial any) {
	t.Helper()
	states, got := skillPageState(t, initial)
	totals := map[string]int{}
	budget := 0
	for _, section := range skillSections {
		totals[section] = states[section].Total
		budget += totals[section]
		if !states[section].HasMore {
			t.Fatal("fixture must page every section", kind, section)
		}
	}
	sawReset := false
	for rounds := 0; ; rounds++ {
		pending := false
		if rounds > budget {
			t.Fatal("pagination restarted or looped")
		}
		for _, section := range skillSections {
			before := states[section]
			if !before.HasMore {
				continue
			}
			pending = true
			tool, example := "get_financial_context_page", "financial-context-"+section+"-page"
			values := map[string]string{"contextId": id, "comparisonId": id, "ref": ref, "section": section, "cursor": before.NextCursor}
			if kind == "item" {
				tool, example = "get_financial_context_item", "financial-context-item-page"
			}
			if kind == "comparison" {
				tool, example = "get_financial_comparison_page", "financial-comparison-"+section+"-page"
			}
			args := skillExample(t, "analysis", example, values)
			args["limit"] = 1
			response := call(t, client, tool, args, false)
			next, rows := skillPageState(t, response["data"])
			for _, other := range skillSections {
				if other != section && !states[other].HasMore && next[other].HasMore {
					sawReset = true
				}
			}
			// The shipped recipe: update ONLY this section; ignore all other descriptors.
			states[section] = next[section]
			got[section] = append(got[section], rows[section]...)
			if next[section].Returned != len(rows[section]) || len(rows[section]) == 0 || next[section].NextCursor == before.NextCursor {
				t.Fatal("nonadvancing section", section)
			}
		}
		if !pending {
			break
		}
	}
	if !sawReset {
		t.Fatal("fixture did not exercise nonrequested offset-zero descriptors")
	}
	for _, section := range skillSections {
		if len(got[section]) != totals[section] || states[section].HasMore {
			t.Fatal("missing rows", section)
		}
		seen := map[string]bool{}
		for _, row := range got[section] {
			var value any
			if err := json.Unmarshal(row, &value); err != nil {
				t.Fatal(err)
			}
			canonical, _ := json.Marshal(value)
			key := string(canonical)
			if seen[key] {
				t.Fatal("duplicated row", kind, section)
			}
			seen[key] = true
		}
	}
}

func TestSkillIndependentSectionStateAcrossAllPagingTools(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	comparisonFixture(t, s, app)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	// Use an already permitted synthetic projection so the context and item
	// each have multiple rows in all three sections, including shared evidence.
	sample, _ := itemFixture(t)
	entry := sample.contexts.entries["package-a"]
	s.contexts.mu.Lock()
	s.contexts.entries["package-a"] = entry
	s.contexts.bytes += entry.size
	initial, err := s.contexts.response("package-a", entry, s.contextGeneration, "", 0, 1)
	s.contexts.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("context", func(t *testing.T) { skillIndependentPages(t, client, "context", "package-a", "", initial) })
	item := decodeItem(t, call(t, client, "get_financial_context_item", skillExample(t, "analysis", "financial-context-item", map[string]string{"contextId": "package-a", "ref": "account-1"}), false))
	t.Run("item", func(t *testing.T) { skillIndependentPages(t, client, "item", "package-a", "account-1", item) })
	comparison := decodeComparison(t, call(t, client, "compare_financial_context", skillExample(t, "analysis", "financial-comparison", nil), false))
	t.Run("comparison", func(t *testing.T) {
		skillIndependentPages(t, client, "comparison", comparison.ComparisonID, "", comparison)
	})
}

func TestSkillComparisonUsesAuthoritativeChangesNotRowSums(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	members, err := app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, kind, role, mode, value string) domain.AccountID {
		t.Helper()
		a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: "USD", InitialAmount: value, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return a.Account.ID
	}
	cash := create("Cash", "bank_account", "asset", "balance", "100")
	investment := create("Investment", "investment_account", "asset", "manual_value", "50")
	property := create("Property", "property", "asset", "manual_value", "300")
	debt := create("Debt", "loan", "liability", "balance", "30")
	if _, err = app.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	for id, value := range map[domain.AccountID]string{cash: "120", investment: "60", property: "330", debt: "35"} {
		if _, err = app.AppendAccountValue(t.Context(), id, value, ""); err != nil {
			t.Fatal(err)
		}
	}
	now = now.AddDate(0, 0, 1)
	if _, err = s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	read := func(left, right string) application.FinancialComparisonContent {
		t.Helper()
		args := skillExample(t, "analysis", "financial-comparison", nil)
		args["leftAsOf"], args["rightAsOf"] = left, right
		r := decodeComparison(t, call(t, client, "compare_financial_context", args, false))
		content := r.Content
		info := r.PositionsPage
		for info.HasMore {
			page := decodeComparison(t, call(t, client, "get_financial_comparison_page", skillExample(t, "analysis", "financial-comparison-positions-page", map[string]string{"comparisonId": r.ComparisonID, "cursor": info.NextCursor}), false))
			content.Positions = append(content.Positions, page.Content.Positions...)
			info = page.PositionsPage
		}
		return content
	}
	want := func(amount *string, expected string) {
		t.Helper()
		if amount == nil || *amount != expected {
			t.Fatalf("amount=%v want %s", amount, expected)
		}
	}
	result := read("2026-08-01", "2026-08-02")
	want(result.Change.Cash.Value, "20")
	want(result.Change.Investments.Value, "10")
	want(result.Change.OtherAssets.Value, "30")
	want(result.Change.Liabilities.Value, "5")
	want(result.Change.NetWorth.Value, "55")
	cashRows, debtRows := 0, 0
	for _, row := range result.Positions {
		if row.Left == nil || row.Right == nil || !row.Left.Included || !row.Right.Included {
			t.Fatal("fixture must have stable inclusion")
		}
		if row.Left.Role == "liability" {
			debtRows++
			want(row.BaseChange, "5")
		}
		if row.BaseChange != nil && *row.BaseChange == "20" {
			cashRows++
		}
	}
	if cashRows != 2 || debtRows != 2 {
		t.Fatal("parent/component duplication missing", cashRows, debtRows)
	}
	// Unchanged money leaving scope changes totals without being a balance flow.
	if _, err = app.UpdateAccount(t.Context(), cash, application.AccountInput{Name: "Cash", IncludeInNetWorthSet: true, IncludeInNetWorth: false}); err != nil {
		t.Fatal(err)
	}
	removed := read("2026-08-02", "current")
	want(removed.Change.Cash.Value, "-120")
	want(removed.Change.NetWorth.Value, "-120")
	for _, row := range removed.Positions {
		want(row.BaseChange, "0")
	}
	// A right-only included account contributes to change.* despite null row deltas.
	create("New cash", "bank_account", "asset", "balance", "7")
	added := read("2026-08-02", "current")
	want(added.Change.Cash.Value, "-113")
	want(added.Change.NetWorth.Value, "-113")
	absent := 0
	for _, row := range added.Positions {
		if row.Left == nil {
			absent++
			if row.Right == nil || row.BaseChange != nil {
				t.Fatal(row)
			}
			want(row.Right.BaseAmount, "7")
		}
	}
	if absent != 2 {
		t.Fatal("expected absent parent and component", absent)
	}
	// Category answers above are read verbatim, never reconstructed from row signs.
	if reflect.DeepEqual(result.Change.Cash, result.Change.NetWorth) {
		t.Fatal("fixture did not distinguish category and total")
	}
}
