package domain

import "time"

type ReviewRuleID string
type ReviewEpisodeID string
type ReviewSuppressionID string

func NewReviewRuleID() ReviewRuleID               { return ReviewRuleID(newID()) }
func NewReviewEpisodeID() ReviewEpisodeID         { return ReviewEpisodeID(newID()) }
func NewReviewSuppressionID() ReviewSuppressionID { return ReviewSuppressionID(newID()) }

func ParseReviewRuleID(value string) (ReviewRuleID, error) {
	return parseID[ReviewRuleID](value, "reviewRuleId")
}
func ParseReviewEpisodeID(value string) (ReviewEpisodeID, error) {
	return parseID[ReviewEpisodeID](value, "reviewEpisodeId")
}
func ParseReviewSuppressionID(value string) (ReviewSuppressionID, error) {
	return parseID[ReviewSuppressionID](value, "reviewSuppressionId")
}

type ReviewRule struct {
	ID                      ReviewRuleID      `json:"id"`
	HouseholdID             HouseholdID       `json:"householdId"`
	Name                    string            `json:"name"`
	Enabled                 bool              `json:"enabled"`
	Severity                string            `json:"severity"`
	Evaluator               string            `json:"evaluator"`
	RowRevision             int               `json:"rowRevision"`
	CurrentConditionVersion int               `json:"currentConditionVersion"`
	Spec                    ReviewRuleSpec    `json:"spec"`
	LegacySpec              *ReviewLegacySpec `json:"legacySpec,omitempty"`
	SemanticHash            string            `json:"semanticHash"`
	CreatedAt               time.Time         `json:"createdAt"`
	UpdatedAt               time.Time         `json:"updatedAt"`
	ArchivedAt              *time.Time        `json:"archivedAt,omitempty"`
}

type ReviewRuleMutation struct {
	Operation        string
	HouseholdID      HouseholdID
	RuleID           ReviewRuleID
	MutationID       MutationID
	ExpectedRevision int
	Name             string
	Enabled          bool
	Severity         string
	Spec             ReviewRuleSpec
	At               time.Time
}
