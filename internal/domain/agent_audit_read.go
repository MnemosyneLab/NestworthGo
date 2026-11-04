package domain

import "time"

// AgentAuditFilter intersects target categories, matching the MCP read contract.
// Dates are source labels; created bounds are inclusive instants.
type AgentAuditFilter struct {
	InstrumentIDs          []string
	FXPairs                []MarketReadTarget
	FromDate, ToDate       string
	CreatedFrom, CreatedTo *time.Time
}

// Selected records and their immediate withdrawal identities come from one
// read transaction. Inactive is checked against full household history.
type AgentAuditSelection struct {
	Records    []AgentQuoteRecord
	Identities []AgentQuoteRecord
	Inactive   map[string]bool
}
