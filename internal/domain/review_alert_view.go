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
	InstrumentID   InstrumentID      `json:"instrumentId"`
	DisplayName    string            `json:"displayName"`
	InstrumentType InstrumentType    `json:"instrumentType"`
	Currency       CurrencyCode      `json:"currency"`
	Status         string            `json:"status"`
	Rules          []ReviewAlertRule `json:"rules"`
}

type ReviewAlertPage struct {
	Items            []ReviewAlertGroup `json:"items"`
	NextCursor       string             `json:"nextCursor,omitempty"`
	ReportRevision   string             `json:"reportRevision"`
	ProjectionStatus string             `json:"projectionStatus"`
	PublishedOn      string             `json:"publishedOn,omitempty"`
}
