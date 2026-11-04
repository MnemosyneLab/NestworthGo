package domain

import "time"

// MarketComparisonScope is an internal, validated bounded evidence query. Dates
// refer to stored source market dates, never the lookup or audit creation date.
type MarketComparisonScope struct {
	Target   MarketReadTarget
	Dates    []string
	Instants []time.Time
}
type MarketComparisonAudit struct {
	RecordID      string    `json:"recordId"`
	Operation     string    `json:"operation"`
	QuoteID       string    `json:"quoteId,omitempty"`
	TargetQuoteID string    `json:"targetQuoteId,omitempty"`
	SourceTitle   string    `json:"sourceTitle"`
	SourceURL     string    `json:"sourceUrl,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}
type MarketComparisonSnapshot struct {
	MarketReadSnapshot
	InstrumentQuotes []InstrumentQuote
	FXQuotes         []FXQuote
	Audits           []MarketComparisonAudit
	Managed          map[string]bool
}
