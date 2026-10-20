package domain

import "time"

// AgentQuoteOperation records an append-only change to an Agent quote.
type AgentQuoteOperation string

const (
	AgentQuoteAppend  AgentQuoteOperation = "append"
	AgentQuoteCorrect AgentQuoteOperation = "correct"
	AgentQuoteRetract AgentQuoteOperation = "retract"
)

// AgentQuoteRecord keeps the submitted evidence alongside its quote or the
// quote it withdraws. A correction has both a new quote and TargetQuoteID.
type AgentQuoteRecord struct {
	RecordID        string
	Operation       AgentQuoteOperation
	Closure         *InstrumentClosure
	InstrumentQuote *InstrumentQuote
	FXQuote         *FXQuote
	TargetQuoteID   string
	SourceTitle     string
	SourceURL       string
	CreatedAt       time.Time
}

type AgentQuoteBatch struct {
	HouseholdID HouseholdID
	RequestKey  string
	RequestHash string
	Records     []AgentQuoteRecord
	CreatedAt   time.Time
}

type AgentQuoteBatchResult struct {
	RequestKey string
	Replayed   bool
	Inserted   int
	RecordIDs  []string
	QuoteIDs   []string
}

// InstrumentClosure is a dated non-price fact. Provenance lives in AgentQuoteRecord.
type InstrumentClosure struct {
	ID           string
	InstrumentID InstrumentID
	Date         string
	EffectiveAt  time.Time
	Timezone     string
	CreatedAt    time.Time
}
