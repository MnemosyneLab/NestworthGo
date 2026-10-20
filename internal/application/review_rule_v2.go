package application

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type ReviewRuleV2Repository interface {
	ListReviewRulesV2(context.Context, domain.HouseholdID, string, int) ([]domain.ReviewRule, error)
	MutateReviewRuleV2(context.Context, domain.ReviewRuleMutation) (domain.ReviewRule, error)
}

type SaveReviewRuleV2Input struct {
	RuleID           string                `json:"ruleId"`
	MutationID       string                `json:"mutationId"`
	ExpectedRevision int                   `json:"expectedRevision"`
	Name             string                `json:"name"`
	Enabled          bool                  `json:"enabled"`
	Severity         string                `json:"severity"`
	Spec             domain.ReviewRuleSpec `json:"spec"`
}

// ListReviewRulesV2 is a pure bounded read. A cursor is the last rule UUID
// returned by the preceding page; no evaluation or projection write occurs.
func (s *Service) ListReviewRulesV2(ctx context.Context, cursor string, pageSize int) ([]domain.ReviewRule, error) {
	port, ok := s.repository.(ReviewRuleV2Repository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review rules are unavailable"}
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 rules per page"}
	}
	if cursor != "" {
		if _, err := domain.ParseReviewRuleID(cursor); err != nil {
			return nil, err
		}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return []domain.ReviewRule{}, nil
	}
	return port.ListReviewRulesV2(ctx, household.ID, cursor, pageSize)
}

func (s *Service) CreateReviewRuleV2(ctx context.Context, input SaveReviewRuleV2Input) (domain.ReviewRule, error) {
	return s.saveReviewRuleV2(ctx, "create", input)
}
func (s *Service) UpdateReviewRuleV2(ctx context.Context, input SaveReviewRuleV2Input) (domain.ReviewRule, error) {
	return s.saveReviewRuleV2(ctx, "update", input)
}
func (s *Service) ArchiveReviewRuleV2(ctx context.Context, ruleID, mutationID string, expectedRevision int) (domain.ReviewRule, error) {
	return s.saveReviewRuleV2(ctx, "archive", SaveReviewRuleV2Input{RuleID: ruleID, MutationID: mutationID, ExpectedRevision: expectedRevision})
}

func (s *Service) saveReviewRuleV2(ctx context.Context, operation string, input SaveReviewRuleV2Input) (domain.ReviewRule, error) {
	var empty domain.ReviewRule
	port, ok := s.repository.(ReviewRuleV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review rules are unavailable"}
	}
	ruleID, err := domain.ParseReviewRuleID(input.RuleID)
	if err != nil {
		return empty, err
	}
	mutationID, err := domain.ParseMutationID(input.MutationID)
	if err != nil {
		return empty, err
	}
	if (operation == "create" && input.ExpectedRevision != 0) || (operation != "create" && input.ExpectedRevision < 1) {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid rule revision"}
	}
	name := strings.TrimSpace(input.Name)
	if operation != "archive" {
		if len([]rune(name)) < 1 || len([]rune(name)) > 120 {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "name", Message: "rule name must contain 1–120 characters"}
		}
		if input.Severity != "notice" && input.Severity != "warning" {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "severity", Message: "use notice or warning"}
		}
		input.Spec, err = input.Spec.Normalize()
		if err != nil {
			return empty, err
		}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return empty, err
	}
	defer unlock()
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	return port.MutateReviewRuleV2(ctx, domain.ReviewRuleMutation{Operation: operation, HouseholdID: household.ID, RuleID: ruleID, MutationID: mutationID, ExpectedRevision: input.ExpectedRevision, Name: name, Enabled: input.Enabled, Severity: input.Severity, Spec: input.Spec, At: s.reviewNow().UTC()})
}

// DecodeReviewRuleV2Input is available to Wails/MCP adapters that accept raw
// JSON; unknown fields and trailing values are rejected at the rule boundary.
func DecodeReviewRuleV2Input(raw []byte) (SaveReviewRuleV2Input, error) {
	var input SaveReviewRuleV2Input
	if len(raw) > 128*1024 {
		return input, &domain.Error{Code: domain.ErrValidation, Field: "rule", Message: "request is too large"}
	}
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return input, &domain.Error{Code: domain.ErrValidation, Field: "rule", Message: "request must contain one JSON object"}
	}
	return input, nil
}
