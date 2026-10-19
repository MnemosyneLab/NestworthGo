package domain

import "time"

// ReviewAlertFilter is applied to complete instrument groups, so one held
// instrument remains one row even when several rules have open episodes.
type ReviewAlertFilter struct {
	Status         string `json:"status"` // all, unignored, pending, acknowledged, ignored, unknown, paused
	InstrumentType string `json:"instrumentType,omitempty"`
	Query          string `json:"query,omitempty"`
}

type ReviewAlertRule struct {
	RuleID          ReviewRuleID        `json:"ruleId"`
	Name            string              `json:"name"`
	Severity        string              `json:"severity"`
	Enabled         bool                `json:"enabled"`
	EpisodeID       ReviewEpisodeID     `json:"episodeId"`
	EpisodeRevision int                 `json:"episodeRevision"`
	TriggeredOn     string              `json:"triggeredOn"`
	FirstHitOn      string              `json:"firstHitOn"`
	FirstDetectedAt time.Time           `json:"firstDetectedAt"`
	AcknowledgedAt  *time.Time          `json:"acknowledgedAt,omitempty"`
	LatestClosedOn  string              `json:"latestClosedOn"`
	Window          ReviewPolicyWindow  `json:"window"`
	LatestResult    *ReviewDailyResult  `json:"latestResult,omitempty"`
	Coverage        []string            `json:"coverage"`
	Status          string              `json:"status"`
	Actionable      bool                `json:"actionable"`
	SuppressionID   string              `json:"suppressionId,omitempty"`
	SuppressedUntil string              `json:"suppressedUntil,omitempty"`
	AmountExact     *string             `json:"amountExact,omitempty"`
	CostExact       *string             `json:"costExact,omitempty"`
	TriggerPolicy   ReviewTriggerPolicy `json:"triggerPolicy"`
}

type ReviewAlertGroup struct {
	ProjectionStatus string            `json:"projectionStatus,omitempty"`
	InstrumentID     InstrumentID      `json:"instrumentId"`
	DisplayName      string            `json:"displayName"`
	InstrumentType   InstrumentType    `json:"instrumentType"`
	Currency         CurrencyCode      `json:"currency"`
	Status           string            `json:"status"`
	Rules            []ReviewAlertRule `json:"rules"`
}

type ReviewAlertPage struct {
	Items            []ReviewAlertGroup `json:"items"`
	NextCursor       string             `json:"nextCursor,omitempty"`
	Summary          ReviewAlertSummary `json:"summary"`
	ReportRevision   string             `json:"reportRevision"`
	ProjectionStatus string             `json:"projectionStatus"`
	PublishedOn      string             `json:"publishedOn,omitempty"`
}

// Counts are per complete instrument group in the same publication/read
// snapshot as Items. MatchedGroups reflects the requested filter, not the
// length of one page; ActionableRules counts open rule episodes.
type ReviewAlertSummary struct {
	TotalGroups        int `json:"totalGroups"`
	MatchedGroups      int `json:"matchedGroups"`
	PendingGroups      int `json:"pendingGroups"`
	AcknowledgedGroups int `json:"acknowledgedGroups"`
	IgnoredGroups      int `json:"ignoredGroups"`
	UnknownGroups      int `json:"unknownGroups"`
	PausedGroups       int `json:"pausedGroups"`
	ActionableRules    int `json:"actionableRules"`
}

// ReviewDailyEvidencePage reads persisted closed-day votes for one episode.
// The cursor is bound to its publication so a corrected day cannot silently
// disappear between pages.
type ReviewDailyEvidencePage struct {
	Items          []ReviewDailyResult `json:"items"`
	NextCursor     string              `json:"nextCursor,omitempty"`
	ReportRevision string              `json:"reportRevision"`
	EpisodeID      ReviewEpisodeID     `json:"episodeId"`
}
