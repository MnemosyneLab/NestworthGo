package agentcapability

// Features describes the currently discovered business surface, never a grant.
// Keep one projection for MCP and the narrower built-in adapter so host-owned
// ReadWrite storage cannot advertise writes to a Read-only model.
func Features(definitions []Definition) []string {
	present := make(map[string]bool, len(definitions))
	for _, d := range definitions {
		present[d.Name] = true
	}
	out := []string{}
	for _, feature := range []struct {
		name  string
		tools []string
	}{
		{"directory", []string{"list_members", "list_institutions", "list_groups"}},
		{"accounts", []string{"list_accounts"}},
		{"instruments", []string{"list_instruments"}},
		{"current_valuation", []string{"get_overview", "get_account_valuations"}},
		{"financial_context", []string{"get_financial_context", "get_financial_context_item"}},
		{"financial_comparison", []string{"compare_financial_context"}},
		{"activity_history", []string{"list_activities", "get_activity"}},
		{"period_analysis", []string{"analyze_period"}},
		{"return_attribution", []string{"compare_financial_attribution"}},
		{"data_health", []string{"scan_data_health"}},
		{"data_repair_preview", []string{"preview_data_repair"}},
		{"data_repair_status", []string{"get_data_repair_job"}},
		{"market_data", []string{"get_market_data"}},
		{"agent_market_data_history", []string{"list_agent_market_data"}},
		{"managed_product_read", []string{"get_products", "list_product_operations"}},
		{"liquidity_read", []string{"get_liquidity_overview"}},
		{"bounded_read", []string{"get_read_page", "get_read_item"}},
		{"instrument_paging", []string{"list_instruments", "get_read_page"}},
		{"market_data_batch", []string{"get_market_data"}},
		{"investment_review_v2_read", []string{"list_investment_review_rules", "list_investment_review_alerts", "get_investment_review_coverage"}},
		{"upcoming_action_read", []string{"get_upcoming_actions"}},
		{"historical_overview_read", []string{"get_historical_overview"}},
		{"reconciliation_session_read", []string{"list_reconciliation_sessions", "get_reconciliation_session", "get_reconciliation_session_page", "get_reconciliation_item"}},
		{"instrument_metadata_batch", []string{"update_instruments_batch"}},
		{"reconciliation_session_write", []string{"preview_reconciliation_session", "commit_plan"}},
		{"investment_review_v2_write", []string{"create_investment_review_rule", "update_investment_review_rule", "refresh_investment_reviews"}},
		{"liquidity_policy_write", []string{"preview_save_liquidity_policy", "preview_reset_liquidity_policy", "commit_plan"}},
		{"managed_product_lifecycle", []string{"preview_product_operation", "commit_plan"}},
		{"managed_product_terms", []string{"preview_product_terms", "commit_plan"}},
		{"managed_product_valuation", []string{"preview_product_valuation", "commit_plan"}},
		{"ledger_preview_commit", []string{"preview_batch", "commit_plan"}},
		{"ledger_batch", []string{"preview_batch", "commit_plan"}},
		{"position_import", []string{"preview_batch", "commit_plan"}},
		{"reconciliation", []string{"preview_reconciliation", "commit_plan"}},
		{"cost_reconciliation", []string{"preview_reconciliation", "commit_plan"}},
		{"position_transfer", []string{"preview_batch", "commit_plan"}},
		{"activity_correction", []string{"preview_correction", "commit_plan"}},
		{"data_repair", []string{"start_data_repair"}},
		{"agent_market_data_import", []string{"import_market_data"}},
	} {
		available := true
		for _, name := range feature.tools {
			available = available && present[name]
		}
		if available {
			out = append(out, feature.name)
		}
	}
	return out
}
