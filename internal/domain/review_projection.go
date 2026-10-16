package domain

import "time"

// ReviewProjectionCapture is a single financial and review-state snapshot.
// The publisher must recapture HistoricalFingerprint inside its write
// transaction and compare the control and rule revisions before committing.
type ReviewProjectionCapture struct {
	HouseholdID           HouseholdID
	SourceCutoff          time.Time
	Historical            ReviewHistoricalBatch
	HistoricalFingerprint string
	Control               ReviewProjectionControl
	Rules                 []ReviewRule
	Suppressions          []ReviewSuppression
	OpenEpisodes          []ReviewStoredEpisode
	EvaluatedInstruments  map[ReviewRuleID][]InstrumentID
}

type ReviewProjectionControl struct {
	Enabled              bool
	ConfigRevision       int
	InputGeneration      int
	ReviewStateRevision  int
	ProjectionStatus     string
	PublishedOn          string
	PublishedFingerprint string
}

type ReviewStoredEpisode struct {
	ID                     ReviewEpisodeID
	RuleID                 ReviewRuleID
	InstrumentID           InstrumentID
	PositionEpochID        string
	OpenedConditionVersion int
	FirstHitOn             string
	TriggeredOn            string
	FirstDetectedAt        time.Time
	AcknowledgedAt         *time.Time
	Revision               int
}

type ReviewPublication struct {
	Capture ReviewProjectionCapture
	At      time.Time
	Rules   []ReviewRulePublication
}

type ReviewRulePublication struct {
	RuleID           ReviewRuleID
	RuleRevision     int
	ConditionVersion int
	InstrumentID     InstrumentID
	TimezoneVersion  string
	LatestClosedOn   string
	Days             []ReviewPublicationDay
	Epochs           []ReviewPublicationEpoch
	PositionKnown    bool
	CurrentEpochID   string
	LatestWindow     ReviewPolicyWindow
	LatestCoverage   []string
	CurrentEpisode   *ReviewEpisodeIdentity
	ClosedEpisodes   []ReviewEpisodeIdentity
	OpenedEpisode    bool
	TargetExcluded   bool
}

type ReviewPublicationDay struct {
	On               string
	Input            ReviewDailyInput
	CoverageMissing  []string
	Result           ReviewDailyResult
	PolicyWindow     ReviewPolicyWindow
	InputFingerprint string
}

type ReviewPublicationEpoch struct {
	ID           string
	InstrumentID InstrumentID
	OpenedAt     *time.Time
	OpeningID    *ActivityID
	ClosedAt     *time.Time
	ClosingID    *ActivityID
	StartUnknown bool
}
