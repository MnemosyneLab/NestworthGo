package agentcapability

// ToolGroup describes discovery only. It must not be used to grant permission.
type ToolGroup string

const (
	CoreTools           ToolGroup = ""
	AccountsTools       ToolGroup = "accounts"
	HistoryTools        ToolGroup = "history"
	InvestmentsTools    ToolGroup = "investments"
	MarketDataTools     ToolGroup = "market_data"
	ReconciliationTools ToolGroup = "reconciliation"
	MaintenanceTools    ToolGroup = "maintenance"
	ChangesTools        ToolGroup = "changes"
)

// ToolGroup derives a category from registered tool identity and effect, never
// from model or user text. Unclassified tools stay discoverable in maintenance.
func (d Definition) ToolGroup() ToolGroup {
	switch d.Name {
	case "get_current_page", "read_page_content", "operate_page_control", "get_navigation_targets", "open_page", "open_return_trend",
		"get_overview", "list_accounts", "get_account_valuations", "get_financial_context", "get_financial_context_item",
		"compare_financial_context", "compare_financial_attribution", "get_read_page", "get_read_item", "get_operation":
		return CoreTools
	case "get_context", "get_catalog", "list_members", "list_institutions", "list_groups", "list_holdings", "get_liquidity_overview", "get_upcoming_actions":
		return AccountsTools
	case "get_activity", "list_activities", "get_historical_overview", "analyze_period", "list_contributions":
		return HistoryTools
	case "list_instruments", "get_products", "list_product_operations",
		"get_investment_review_alert", "get_investment_review_coverage", "list_investment_review_alerts",
		"list_investment_review_daily_evidence", "list_investment_review_episode_events", "list_investment_review_episode_history",
		"list_investment_review_rules", "list_investment_review_suppressions":
		return InvestmentsTools
	case "get_market_data", "list_agent_market_data", "compare_market_data_import", "search_market_instruments":
		return MarketDataTools
	case "get_reconciliation_item", "get_reconciliation_session", "get_reconciliation_session_page", "list_reconciliation_sessions":
		return ReconciliationTools
	default:
		if d.Effect == Write || d.Effect == Preview {
			return ChangesTools
		}
		return MaintenanceTools
	}
}
