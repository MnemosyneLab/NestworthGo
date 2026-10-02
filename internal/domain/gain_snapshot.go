package domain

// GainSnapshot owns all database inputs needed for a gain read. Cost replay,
// transfer recursion and FX decomposition run after the read transaction closes.
type GainSnapshot struct {
	Portfolio          PortfolioSnapshot
	CostEvents         map[HoldingID][]CostBasisEvent
	StartingCosts      map[HoldingID]*UnitPrice
	HistoricalFXQuotes []FXQuote
	Dividends          []Activity
}
