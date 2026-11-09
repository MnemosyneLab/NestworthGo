package assistant

import "testing"

func TestDeletedFinancialPageToolsAreNotProgressSources(t *testing.T) {
	for _, name := range []string{"get_financial_context_page", "get_financial_comparison_page"} {
		if progressSource(name) {
			t.Fatalf("%s is not a registered tool", name)
		}
	}
	for _, name := range []string{"get_financial_context", "get_read_page", "get_account_valuations", "open_return_trend", "operate_page_control", "create_member", "commit_plan", "refresh_market_data"} {
		if !progressSource(name) {
			t.Fatalf("live read %s dropped from progress", name)
		}
	}
}
