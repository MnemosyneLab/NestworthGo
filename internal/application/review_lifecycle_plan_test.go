package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewLifecycleKeepsAcknowledgementThroughUnknownAndReturn(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	household, instrument, ruleID := domain.NewHouseholdID(), domain.NewInstrumentID(), domain.NewReviewRuleID()
	acknowledged := now.Add(-time.Hour)
	rule := domain.ReviewRule{ID: ruleID, HouseholdID: household, Enabled: true, Evaluator: domain.ReviewEvaluator, CurrentConditionVersion: 1, Spec: syntheticHistoricalReviewSpec()}
	capture := domain.ReviewProjectionCapture{HouseholdID: household, Control: domain.ReviewProjectionControl{Enabled: true}, OpenEpisodes: []domain.ReviewStoredEpisode{{ID: domain.NewReviewEpisodeID(), RuleID: ruleID, InstrumentID: instrument, PositionEpochID: "epoch-1", OpenedConditionVersion: 1, TriggeredOn: "2026-10-10", FirstDetectedAt: now.AddDate(0, 0, -3), AcknowledgedAt: &acknowledged}}}
	plan := ReviewRuleInstrumentPlan{RuleID: ruleID, ConditionVersion: 1, InstrumentID: instrument, LatestClosedOn: "2026-10-12", LatestWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}, Position: ReviewPositionTimeline{Known: true, Quantity: "1", Epochs: []ReviewPositionEpoch{{StartUnknown: true}}}, EpochIDs: []string{"epoch-1"}, Days: []ReviewPlannedDay{
		{On: "2026-10-10", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
		{On: "2026-10-11", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewUnknown}},
		{On: "2026-10-12", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
	}}
	decision, err := PlanReviewLifecycle(capture, rule, plan, now)
	if err != nil || len(decision.Closed) != 0 || decision.Opened || decision.Current == nil || decision.Current.ID != string(capture.OpenEpisodes[0].ID) || decision.Current.AcknowledgedAt == nil || decision.Actionable {
		t.Fatalf("T-U-T lost acknowledged identity: %+v %v", decision, err)
	}
}

func TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	household, instrument, ruleID := domain.NewHouseholdID(), domain.NewInstrumentID(), domain.NewReviewRuleID()
	acknowledged := now.Add(-time.Hour)
	rule := domain.ReviewRule{ID: ruleID, HouseholdID: household, Enabled: true, Evaluator: domain.ReviewEvaluator, CurrentConditionVersion: 1, Spec: syntheticHistoricalReviewSpec()}
	capture := domain.ReviewProjectionCapture{HouseholdID: household, Control: domain.ReviewProjectionControl{Enabled: true}, OpenEpisodes: []domain.ReviewStoredEpisode{{ID: domain.NewReviewEpisodeID(), RuleID: ruleID, InstrumentID: instrument, PositionEpochID: "old", OpenedConditionVersion: 1, TriggeredOn: "2026-10-10", FirstDetectedAt: now.AddDate(0, 0, -5), AcknowledgedAt: &acknowledged}}}
	closed := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	reopened := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	ignore := domain.ReviewSuppression{ID: "permanent", InstrumentID: instrument, Mode: "permanent", Timezone: "UTC"}
	plan := ReviewRuleInstrumentPlan{RuleID: ruleID, ConditionVersion: 1, InstrumentID: instrument, LatestClosedOn: "2026-10-14", LatestWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue, HitCount: 2}, Position: ReviewPositionTimeline{Known: true, Quantity: "1", Epochs: []ReviewPositionEpoch{{StartUnknown: true, ClosedAt: &closed}, {OpenedAt: &reopened}}}, EpochIDs: []string{"old", "new"}, ActiveSuppression: &ignore, Days: []ReviewPlannedDay{
		{On: "2026-10-10", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
		{On: "2026-10-11", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
		{On: "2026-10-12", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
		{On: "2026-10-13", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}}, // rolling stays true on flat day
		{On: "2026-10-14", PolicyWindow: domain.ReviewPolicyWindow{Truth: domain.ReviewTrue}},
	}}
	decision, err := PlanReviewLifecycle(capture, rule, plan, now)
	if err != nil || len(decision.Closed) != 1 || decision.Closed[0].ResolvedReason != "position_closed" || !decision.Opened || decision.Current == nil || decision.Current.ID == string(capture.OpenEpisodes[0].ID) || decision.Current.TriggeredOn != "2026-10-14" || decision.Current.AcknowledgedAt != nil || decision.Actionable {
		t.Fatalf("re-buy inherited old identity/ack or discarded rolling hits/ignore: %+v %v", decision, err)
	}
}
