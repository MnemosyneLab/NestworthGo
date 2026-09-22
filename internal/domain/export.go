package domain

// ExportRecord contains only the explicitly documented versioned export fields.
// Decimal values are strings, optional values are null, and flags are booleans.
type ExportRecord map[string]any
type ExportDatasets map[string][]ExportRecord

type ExportFacts struct {
	Directory  ExportDatasets `json:"directory"`
	History    ExportDatasets `json:"history"`
	MarketData ExportDatasets `json:"marketData"`
	Liquidity  ExportDatasets `json:"liquidity"`
}

// ExportSnapshot is an internal read model, not a serialization of domain types.
// Every field is captured from the same database read transaction.
type ExportSnapshot struct {
	Facts         ExportFacts
	Portfolio     PortfolioSnapshot
	CostEvents    map[HoldingID][]CostBasisEvent
	StartingCosts map[HoldingID]*UnitPrice
}

// ExportConversion is the stable, credential-free provenance of a metal quote.
type ExportConversion struct {
	Policy      string  `json:"policy"`
	Symbol      string  `json:"symbol"`
	RawPrice    string  `json:"rawPrice"`
	RawCurrency string  `json:"rawCurrency"`
	RawUnit     string  `json:"rawUnit"`
	RawQuotedAt string  `json:"rawQuotedAt"`
	FXRate      string  `json:"fxRate"`
	FXSource    string  `json:"fxSource"`
	FXQuotedAt  *string `json:"fxQuotedAt"`
	Currency    string  `json:"currency"`
	Unit        string  `json:"unit"`
}
