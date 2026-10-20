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
	SourceRevision        *ReviewSourceCheckpoint
	Temporal              *ReviewTemporalCertificate
	PublishedTemporal     *ReviewTemporalCertificate
	TemporalFullRecompute bool
	Control               ReviewProjectionControl
	Rules                 []ReviewRule
	Suppressions          []ReviewSuppression
	OpenEpisodes          []ReviewStoredEpisode
	EvaluatedInstruments  map[ReviewRuleID][]InstrumentID
	ReusableDailyEvidence map[ReviewDailyEvidenceKey]ReviewReusableDailyEvidence
	CompletenessLineage   *ReviewCompletenessLineage
	Suffix                *ReviewSuffixCapture
	Certified             *ReviewCertifiedCapture
}

// ReviewDailyEvidenceKey identifies one persisted daily input/result for the
// current rule version. Reuse is permitted only when the capture's complete
// historical source fingerprint matches the last published fingerprint.
type ReviewDailyEvidenceKey struct {
	RuleID           ReviewRuleID
	ConditionVersion int
	InstrumentID     InstrumentID
	TimezoneVersion  string
	On               string
}

type ReviewReusableDailyEvidence struct {
	Input                 ReviewDailyInput
	CoverageMissing       []string
	InputFingerprint      string
	Result                ReviewDailyResult
	HasResult             bool
	PriorConsecutiveCount int
}

type ReviewProjectionControl struct {
	Enabled                   bool
	ConfigRevision            int
	InputGeneration           int
	ReviewStateRevision       int
	ProjectionStatus          string
	PublishedOn               string
	PublishedFingerprint      string
	PublishedTemporalRevision string
}

type ReviewStoredEpisode struct {
	ID                     ReviewEpisodeID
	RuleID                 ReviewRuleID
	InstrumentID           InstrumentID
	PositionEpochID        string
	OpenedConditionVersion int
	FirstHitOn             string
	TriggeredOn            string
	TriggerWindow          ReviewPolicyWindow
	TriggerResult          *ReviewDailyResult
	TriggerCoverage        []string
	FirstDetectedAt        time.Time
	AcknowledgedAt         *time.Time
	Revision               int
}

type ReviewPublication struct {
	Capture             ReviewProjectionCapture
	At                  time.Time
	Rules               []ReviewRulePublication
	FXProviderKey       string
	WindowOracle        *ReviewWindowOracle
	CertifiedFullOracle *ReviewCertifiedFullOracle
}

type ReviewRulePublication struct {
	RuleID           ReviewRuleID
	RuleRevision     int
	ConditionVersion int
	InstrumentID     InstrumentID
	TimezoneVersion  string
	LatestClosedOn   string
	Days             []ReviewPublicationDay
	LatestResult     *ReviewDailyResult
	LatestCoverage   []string
	FirstHitOn       string
	TriggerWindow    ReviewPolicyWindow
	TriggerResult    *ReviewDailyResult
	TriggerCoverage  []string
	Epochs           []ReviewPublicationEpoch
	PositionKnown    bool
	CurrentEpochID   string
	LatestWindow     ReviewPolicyWindow
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
