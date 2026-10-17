package domain

type ReviewCoverageFilter struct {
	Status         string `json:"status"`
	InstrumentType string `json:"instrumentType,omitempty"`
	Query          string `json:"query,omitempty"`
}

// ReviewCoverage includes current positive holdings even when no alert is
// open. A missing evaluation state is shown as uncomputed, never as healthy.
type ReviewCoverageRule struct {
	RuleID         ReviewRuleID       `json:"ruleId"`
	Name           string             `json:"name"`
	State          string             `json:"state"`
	LatestClosedOn string             `json:"latestClosedOn"`
	Window         ReviewPolicyWindow `json:"window"`
	Missing        []string           `json:"missing"`
}

type ReviewCoverageGroup struct {
	InstrumentID   InstrumentID         `json:"instrumentId"`
	DisplayName    string               `json:"displayName"`
	InstrumentType InstrumentType       `json:"instrumentType"`
	Status         string               `json:"status"`
	Rules          []ReviewCoverageRule `json:"rules"`
}

type ReviewCoveragePage struct {
	Items            []ReviewCoverageGroup `json:"items"`
	NextCursor       string                `json:"nextCursor,omitempty"`
	ReportRevision   string                `json:"reportRevision"`
	ProjectionStatus string                `json:"projectionStatus"`
	PublishedOn      string                `json:"publishedOn,omitempty"`
}
