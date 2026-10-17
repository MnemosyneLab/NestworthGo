package domain

// ReviewEpisodeHistoryItem keeps the original trigger identity and handling
// outcome for both open and resolved daily-rule episodes. It does not
// reinterpret old evidence with the rule's current condition version.
type ReviewEpisodeHistoryItem struct {
	EpisodeID              ReviewEpisodeID `json:"episodeId"`
	RuleID                 ReviewRuleID    `json:"ruleId"`
	RuleName               string          `json:"ruleName"`
	InstrumentID           InstrumentID    `json:"instrumentId"`
	InstrumentName         string          `json:"instrumentName"`
	PositionEpochID        string          `json:"positionEpochId"`
	OpenedConditionVersion int             `json:"openedConditionVersion"`
	FirstHitOn             string          `json:"firstHitOn"`
	TriggeredOn            string          `json:"triggeredOn"`
	FirstDetectedAt        string          `json:"firstDetectedAt"`
	LatestClosedOn         string          `json:"latestClosedOn"`
	WindowStart            string          `json:"windowStart"`
	WindowEnd              string          `json:"windowEnd"`
	HitCount               int             `json:"hitCount"`
	FalseCount             int             `json:"falseCount"`
	UnknownCount           int             `json:"unknownCount"`
	AcknowledgedAt         string          `json:"acknowledgedAt,omitempty"`
	ResolvedAt             string          `json:"resolvedAt,omitempty"`
	ResolvedReason         string          `json:"resolvedReason,omitempty"`
	Revision               int             `json:"revision"`
}

type ReviewEpisodeHistoryPage struct {
	Items          []ReviewEpisodeHistoryItem `json:"items"`
	NextCursor     string                     `json:"nextCursor,omitempty"`
	ReportRevision string                     `json:"reportRevision"`
	RuleID         ReviewRuleID               `json:"ruleId"`
}

type ReviewEpisodeEvent struct {
	ID          string          `json:"id"`
	EpisodeID   ReviewEpisodeID `json:"episodeId"`
	Kind        string          `json:"kind"`
	PayloadJSON string          `json:"payloadJson"`
	CreatedAt   string          `json:"createdAt"`
}

type ReviewEpisodeEventPage struct {
	Items          []ReviewEpisodeEvent `json:"items"`
	NextCursor     string               `json:"nextCursor,omitempty"`
	ReportRevision string               `json:"reportRevision"`
	EpisodeID      ReviewEpisodeID      `json:"episodeId"`
}
