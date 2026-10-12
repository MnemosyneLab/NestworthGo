package domain

// The initial series budget bounds retained inputs, not just output points.
const ErrHistoricalSeriesBudget ErrorCode = "historical_series_budget"

type HistoricalSeriesLimits struct {
	MaxLeaves int
	MaxFacts  int
	MaxBytes  int64
}

type HistoricalSeriesWork struct {
	CandidateFacts int   `json:"candidateFacts"`
	CandidateBytes int64 `json:"candidateBytes"`
	Leaves         int   `json:"leaves"`
	Queries        int   `json:"queries"`
}
