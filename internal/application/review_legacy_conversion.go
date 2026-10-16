package application

import (
	"context"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Conversion always creates new daily-rule identities. The old v1 rule and
// its irreversible acknowledgement aliases remain in history, while the new
// rules start without copied acknowledgements.
type ReviewLegacyConversionDraft struct {
	LegacyRuleID string                     `json:"legacyRuleId"`
	TargetKind   string                     `json:"targetKind"`
	Timezone     string                     `json:"timezone"`
	Trigger      domain.ReviewTriggerPolicy `json:"triggerPolicy"`
}

type ReviewLegacyConversionPreview struct {
	LegacyRuleID      domain.ReviewRuleID        `json:"legacyRuleId"`
	LegacyRevision    int                        `json:"legacyRevision"`
	ConfigRevision    int                        `json:"configRevision"`
	StateRevision     int                        `json:"stateRevision"`
	SourceToken       string                     `json:"sourceToken"`
	RecommendedPolicy domain.ReviewTriggerPolicy `json:"recommendedPolicy"`
	LowYieldSpec      domain.ReviewRuleSpec      `json:"lowYieldSpec"`
	LossSpec          domain.ReviewRuleSpec      `json:"lossSpec"`
	LowYieldAlerts    int                        `json:"lowYieldAlerts"`
	LossAlerts        int                        `json:"lossAlerts"`
	UnknownOutcomes   int                        `json:"unknownOutcomes"`
	LegacySustainDays int                        `json:"legacySustainDays"`
	LegacyWindowDays  int                        `json:"legacyWindowDays"`
}

type ConvertReviewLegacyRuleV2Input struct {
	ReviewLegacyConversionDraft
	ExpectedRevision       int    `json:"expectedRevision"`
	ExpectedConfigRevision int    `json:"expectedConfigRevision"`
	ExpectedStateRevision  int    `json:"expectedStateRevision"`
	ExpectedSourceToken    string `json:"expectedSourceToken"`
	LowYieldRuleID         string `json:"lowYieldRuleId"`
	LossRuleID             string `json:"lossRuleId"`
	MutationID             string `json:"mutationId"`
}

type ReviewLegacyConversionPort interface {
	ConvertReviewLegacyRuleV2(context.Context, domain.ReviewLegacyConversionMutation) ([]domain.ReviewRule, error)
}

func reviewLegacyRuleInCapture(capture domain.ReviewProjectionCapture, id domain.ReviewRuleID) (domain.ReviewRule, error) {
	for _, rule := range capture.Rules {
		if rule.ID == id && rule.Evaluator == domain.ReviewLegacyEvaluator && rule.LegacySpec != nil && rule.ArchivedAt == nil {
			return rule, nil
		}
	}
	return domain.ReviewRule{}, &domain.Error{Code: domain.ErrNotFound, Field: "legacyRuleId", Message: "current legacy review rule not found"}
}

func buildReviewLegacyConversion(rule domain.ReviewRule, draft ReviewLegacyConversionDraft) (domain.ReviewRuleSpec, domain.ReviewRuleSpec, error) {
	if rule.LegacySpec == nil {
		return domain.ReviewRuleSpec{}, domain.ReviewRuleSpec{}, &domain.Error{Code: domain.ErrValidation, Field: "legacyRuleId", Message: "a legacy policy is required"}
	}
	return domain.BuildReviewLegacyConversion(*rule.LegacySpec, draft.TargetKind, draft.Timezone, draft.Trigger)
}

func (s *Service) PreviewReviewLegacyConversionV2(ctx context.Context, draft ReviewLegacyConversionDraft) (ReviewLegacyConversionPreview, error) {
	var empty ReviewLegacyConversionPreview
	port, ok := s.repository.(ReviewProjectionRepository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "legacy conversion preview is unavailable"}
	}
	id, err := domain.ParseReviewRuleID(draft.LegacyRuleID)
	if err != nil {
		return empty, err
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	now := s.clock()
	capture, err := port.CaptureReviewProjection(ctx, household.ID, now)
	if err != nil {
		return empty, err
	}
	rule, err := reviewLegacyRuleInCapture(capture, id)
	if err != nil {
		return empty, err
	}
	lowSpec, lossSpec, err := buildReviewLegacyConversion(rule, draft)
	if err != nil {
		return empty, err
	}
	result := ReviewLegacyConversionPreview{LegacyRuleID: id, LegacyRevision: rule.RowRevision, ConfigRevision: capture.Control.ConfigRevision, StateRevision: capture.Control.ReviewStateRevision, SourceToken: capture.HistoricalFingerprint,
		RecommendedPolicy: domain.ReviewTriggerPolicy{Kind: "consecutive", Count: rule.LegacySpec.Configuration.Policy.SustainDays},
		LowYieldSpec:      lowSpec, LossSpec: lossSpec,
		LegacySustainDays: rule.LegacySpec.Configuration.Policy.SustainDays, LegacyWindowDays: rule.LegacySpec.Configuration.Policy.WindowDays}
	if !rule.Enabled || !capture.Control.Enabled {
		return result, nil // original master pause remains paused after conversion
	}
	ids := lowSpec.Target.InstrumentIDs
	if lowSpec.Target.Kind == "all_current" {
		seen := map[domain.InstrumentID]bool{}
		for _, holding := range capture.Historical.History.Portfolio.Holdings {
			seen[holding.InstrumentID] = true
		}
		ids = ids[:0]
		for id := range seen {
			ids = append(ids, id)
		}
		slices.Sort(ids)
	}
	for _, instrumentID := range ids {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		for index, spec := range []domain.ReviewRuleSpec{lowSpec, lossSpec} {
			hypothetical := domain.ReviewRule{ID: domain.NewReviewRuleID(), HouseholdID: household.ID, Evaluator: domain.ReviewEvaluator, CurrentConditionVersion: 1, Spec: spec}
			plan, err := PlanReviewRuleInstrument(ctx, capture, hypothetical, instrumentID, now, s.FXProviderKey())
			if err != nil {
				return empty, err
			}
			quantity, err := domain.ParseQuantity(plan.Position.Quantity)
			if !plan.Position.Known || err != nil || !quantity.Decimal().IsPositive() {
				continue
			}
			if plan.LatestWindow.Truth == domain.ReviewUnknown {
				result.UnknownOutcomes++
			}
			if plan.LatestWindow.Truth != domain.ReviewTrue {
				continue
			}
			globalIgnore, err := domain.ActiveReviewSuppression(capture.Suppressions, instrumentID, "", now)
			if err != nil {
				return empty, err
			}
			oldZone := capture.Historical.History.Origin.Timezone
			if oldZone == "" {
				oldZone = draft.Timezone
			}
			location, _ := time.LoadLocation(oldZone)
			if globalIgnore != nil || rule.LegacySpec.Configuration.SnoozedUntil[instrumentID.String()] > now.In(location).Format(time.DateOnly) {
				continue
			}
			if index == 0 {
				result.LowYieldAlerts++
			} else {
				result.LossAlerts++
			}
		}
	}
	return result, nil
}

func (s *Service) ConvertReviewLegacyRuleV2(ctx context.Context, input ConvertReviewLegacyRuleV2Input) ([]domain.ReviewRule, error) {
	port, ok := s.repository.(ReviewLegacyConversionPort)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "legacy conversion is unavailable"}
	}
	legacyID, err := domain.ParseReviewRuleID(input.LegacyRuleID)
	if err != nil {
		return nil, err
	}
	lowID, err := domain.ParseReviewRuleID(input.LowYieldRuleID)
	if err != nil {
		return nil, err
	}
	lossID, err := domain.ParseReviewRuleID(input.LossRuleID)
	if err != nil {
		return nil, err
	}
	mutationID, err := domain.ParseMutationID(input.MutationID)
	if err != nil {
		return nil, err
	}
	if lowID == lossID || lowID == legacyID || lossID == legacyID || input.ExpectedRevision < 1 || input.ExpectedConfigRevision < 1 || input.ExpectedStateRevision < 0 || len(input.ExpectedSourceToken) != 64 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "conversion", Message: "distinct rule IDs, legacy revision and source token are required"}
	}
	writeCtx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	household, err := s.repository.Household(writeCtx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	return port.ConvertReviewLegacyRuleV2(writeCtx, domain.ReviewLegacyConversionMutation{HouseholdID: household.ID, LegacyRuleID: legacyID, ExpectedRevision: input.ExpectedRevision, ExpectedConfigRevision: input.ExpectedConfigRevision, ExpectedStateRevision: input.ExpectedStateRevision, ExpectedSourceToken: input.ExpectedSourceToken, LowYieldRuleID: lowID, LossRuleID: lossID, MutationID: mutationID, TargetKind: input.TargetKind, Timezone: input.Timezone, Trigger: input.Trigger, At: s.clock().UTC()})
}
