package application

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type storedReviewEpisodeEvidence struct {
	Window          domain.ReviewPolicyWindow
	TriggerWindow   domain.ReviewPolicyWindow
	FirstHitOn      string
	TriggerResult   *domain.ReviewDailyResult
	TriggerCoverage []string
	LatestResult    *domain.ReviewDailyResult
	Coverage        []string
}

func TestReviewEpisodeRetainsOriginalTriggerEvidenceWhileWindowAdvances(t *testing.T) {
	for _, test := range []struct {
		name, triggeredOn, firstHitOn, triggerWindowStart, currentWindowStart string
		policy                                                                domain.ReviewTriggerPolicy
		triggerHits, triggerFalse, currentHits, currentFalse                  int
	}{
		{
			name: "single", triggeredOn: "2026-10-13", firstHitOn: "2026-10-13",
			triggerWindowStart: "2026-10-13", currentWindowStart: "2026-10-16",
			policy: domain.ReviewTriggerPolicy{Kind: "single"}, triggerHits: 1, currentHits: 1,
		},
		{
			name: "consecutive", triggeredOn: "2026-10-15", firstHitOn: "2026-10-13",
			triggerWindowStart: "2026-10-13", currentWindowStart: "2026-10-14",
			policy: domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, triggerHits: 3, currentHits: 3,
		},
		{
			name: "rolling", triggeredOn: "2026-10-15", firstHitOn: "2026-10-13",
			triggerWindowStart: "2026-10-06", currentWindowStart: "2026-10-07",
			policy:      domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 3},
			triggerHits: 3, triggerFalse: 7, currentHits: 4, currentFalse: 6,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-06", "1")
			f.day(t, "2026-10-13")
			f.quantity(t, true)    // Cost becomes 200: F through 10/12, then T.
			f.day(t, "2026-10-17") // Discover the rule after the complete episode history.
			spec := syntheticHistoricalReviewSpec()
			spec.Predicate.All[1].Leaf.Value = "150"
			spec.Predicate.All[1].Leaf.Operator = ">"
			spec.TriggerPolicy = test.policy
			rule := saveSyntheticReviewRule(t, f, "Synthetic late-discovered review", spec)
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}

			var episodeID, triggered, firstHit, currentStart, currentEnd, evidenceJSON string
			var currentHits, currentFalse int
			if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,first_hit_on,window_start,window_end,hit_count,false_count,evidence_json
				FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(
				&episodeID, &triggered, &firstHit, &currentStart, &currentEnd, &currentHits, &currentFalse, &evidenceJSON); err != nil {
				t.Fatal(err)
			}
			var stored storedReviewEpisodeEvidence
			if err := json.Unmarshal([]byte(evidenceJSON), &stored); err != nil {
				t.Fatal(err)
			}
			if triggered != test.triggeredOn || firstHit != test.firstHitOn || stored.FirstHitOn != test.firstHitOn || stored.TriggerWindow.WindowStart != test.triggerWindowStart || stored.TriggerWindow.WindowEnd != test.triggeredOn || stored.TriggerWindow.HitCount != test.triggerHits || stored.TriggerWindow.FalseCount != test.triggerFalse {
				t.Fatalf("opening snapshot: triggered=%s firstHit=%s stored=%+v, want trigger=%s firstHit=%s window=%s..%s H/F=%d/%d", triggered, firstHit, stored, test.triggeredOn, test.firstHitOn, test.triggerWindowStart, test.triggeredOn, test.triggerHits, test.triggerFalse)
			}
			if currentStart != test.currentWindowStart || currentEnd != "2026-10-16" || currentHits != test.currentHits || currentFalse != test.currentFalse || stored.Window.WindowStart != currentStart {
				t.Fatalf("current rolling window was not retained separately: columns=%s..%s H/F=%d/%d evidence=%+v", currentStart, currentEnd, currentHits, currentFalse, stored.Window)
			}
			if stored.TriggerResult == nil || stored.TriggerResult.On != test.triggeredOn || stored.TriggerResult.Truth != domain.ReviewTrue {
				t.Fatalf("trigger-date daily result missing from snapshot: %+v", stored.TriggerResult)
			}

			var openedPayload string
			if err := f.db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, episodeID).Scan(&openedPayload); err != nil {
				t.Fatal(err)
			}
			var opened storedReviewEpisodeEvidence
			if err := json.Unmarshal([]byte(openedPayload), &opened); err != nil {
				t.Fatal(err)
			}
			if opened.TriggerWindow != stored.TriggerWindow || opened.Window != stored.TriggerWindow || opened.FirstHitOn != test.firstHitOn || opened.LatestResult == nil || opened.LatestResult.On != test.triggeredOn {
				t.Fatalf("opened event did not preserve initial evidence: %+v", opened)
			}

			oldest, newest := "", ""
			cursor := ""
			for {
				page, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episodeID, cursor, 1)
				if err != nil || len(page.Items) != 1 {
					t.Fatalf("evidence page after %q: %+v %v", cursor, page, err)
				}
				if newest == "" {
					newest = page.Items[0].On
				}
				oldest = page.Items[0].On
				if page.NextCursor == "" {
					break
				}
				cursor = page.NextCursor
			}
			if newest != "2026-10-16" || oldest != test.triggerWindowStart {
				t.Fatalf("evidence pagination range=%s..%s, want original trigger start %s through current %s", oldest, newest, test.triggerWindowStart, "2026-10-16")
			}

			// Ack and then damage only the pre-fix projection fields to prove one
			// full-history rebuild reconstructs an existing open episode in place.
			if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episodeID, string(domain.NewMutationID()), 1); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.SQL.Exec(`UPDATE investment_review_episodes SET first_hit_on='2026-10-16',evidence_json=? WHERE id=?`,
				`{"Window":{"truth":"true","windowStart":"2026-10-16","windowEnd":"2026-10-16","hitCount":1},"FirstHitOn":"2026-10-16","Coverage":[]}`, episodeID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.SQL.Exec(`UPDATE investment_review_episode_events SET payload_json=? WHERE episode_id=? AND kind='opened'`,
				`{"Window":{"truth":"true","windowStart":"2026-10-16","windowEnd":"2026-10-16","hitCount":1},"FirstHitOn":"2026-10-16","Coverage":[]}`, episodeID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatalf("rebuild legacy/broken episode projection: %v", err)
			}
			var repairedID, repairedFirstHit, acknowledged, repairedEvidenceJSON string
			if err := f.db.SQL.QueryRow(`SELECT id,first_hit_on,acknowledged_at,evidence_json FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&repairedID, &repairedFirstHit, &acknowledged, &repairedEvidenceJSON); err != nil {
				t.Fatal(err)
			}
			if repairedID != episodeID || repairedFirstHit != test.firstHitOn || acknowledged == "" {
				t.Fatalf("projection rebuild changed episode identity/ack or failed to repair first hit: id=%s first=%s ack=%q", repairedID, repairedFirstHit, acknowledged)
			}
			var repaired storedReviewEpisodeEvidence
			if err := json.Unmarshal([]byte(repairedEvidenceJSON), &repaired); err != nil || repaired.TriggerWindow.WindowStart != test.triggerWindowStart || repaired.FirstHitOn != test.firstHitOn {
				t.Fatalf("projection rebuild failed to restore initial snapshot: %+v %v", repaired, err)
			}

			// An ordinary continuation advances the current window and counts but
			// keeps trigger metadata and the acknowledgement stable.
			f.day(t, "2026-10-18")
			capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
			if err != nil {
				t.Fatal(err)
			}
			incremental, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
			if err != nil {
				t.Fatal(err)
			}
			fullCapture := capture
			fullCapture.Control.ProjectionStatus = "pending"
			fullCapture.Control.PublishedFingerprint = ""
			fullCapture.ReusableDailyEvidence = nil
			full, err := PrepareReviewPublication(t.Context(), fullCapture, *f.now, "")
			if err != nil {
				t.Fatal(err)
			}
			var incrementalRule, fullRule *domain.ReviewRulePublication
			for i := range incremental.Rules {
				if incremental.Rules[i].RuleID == rule.ID {
					incrementalRule = &incremental.Rules[i]
				}
			}
			for i := range full.Rules {
				if full.Rules[i].RuleID == rule.ID {
					fullRule = &full.Rules[i]
				}
			}
			if incrementalRule == nil || fullRule == nil || len(incrementalRule.Days) != 1 || incrementalRule.Days[0].On != "2026-10-17" || len(fullRule.Days) == 0 || fullRule.Days[0].On != "2026-10-06" || incrementalRule.LatestWindow != fullRule.LatestWindow || incrementalRule.TriggerWindow != fullRule.TriggerWindow || incrementalRule.FirstHitOn != fullRule.FirstHitOn || incrementalRule.CurrentEpisode == nil || fullRule.CurrentEpisode == nil || incrementalRule.CurrentEpisode.TriggeredOn != fullRule.CurrentEpisode.TriggeredOn || incrementalRule.FirstHitOn != test.firstHitOn || incrementalRule.TriggerWindow.WindowStart != test.triggerWindowStart {
				t.Fatalf("full/incremental trigger metadata differs: incremental=%+v full=%+v", compactReviewRulePublication(incrementalRule), compactReviewRulePublication(fullRule))
			}
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := f.db.SQL.QueryRow(`SELECT first_hit_on,acknowledged_at,evidence_json FROM investment_review_episodes WHERE id=?`, episodeID).Scan(&repairedFirstHit, &acknowledged, &repairedEvidenceJSON); err != nil {
				t.Fatal(err)
			}
			if repairedFirstHit != test.firstHitOn || acknowledged == "" {
				t.Fatalf("ordinary continuation changed first hit or ack: first=%s ack=%q", repairedFirstHit, acknowledged)
			}
			if err := json.Unmarshal([]byte(repairedEvidenceJSON), &repaired); err != nil || repaired.Window.WindowEnd != "2026-10-17" || repaired.TriggerWindow.WindowStart != test.triggerWindowStart {
				t.Fatalf("ordinary continuation failed to advance only current window: %+v %v", repaired, err)
			}
		})
	}
}

func TestReviewHistoricalCorrectionReconstructsTriggerAndFirstHitWithoutLosingAcknowledgement(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	importReviewDailyClose(t, f, "2026-08-01", "100")
	f.day(t, "2026-08-08")
	quoteID := importReviewDailyClose(t, f, "2026-08-08", "100")
	f.day(t, "2026-08-09")
	importReviewDailyClose(t, f, "2026-08-09", "120")
	*f.now = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}}
	rule := saveSyntheticReviewRule(t, f, "Synthetic corrected trigger", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID, triggered, firstHit string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,first_hit_on FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episodeID, &triggered, &firstHit); err != nil || triggered != "2026-08-09" || firstHit != "2026-08-09" {
		t.Fatalf("fixture did not open on Aug 9: id=%s trigger=%s first=%s %v", episodeID, triggered, firstHit, err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episodeID, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	correction := AgentMarketDataItem{Operation: "correct", TargetQuoteID: quoteID, Kind: "close", InstrumentID: f.instrument.ID.String(), Currency: "CNY", Date: "2026-08-08", Timezone: "UTC", Value: "120", SourceTitle: "Synthetic original-date trigger correction"}
	if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
		t.Fatal(err)
	}
	assertReviewSourceInvalidated(t, f)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var repairedID, repairedTrigger, repairedFirstHit, acknowledged, evidenceJSON string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,first_hit_on,acknowledged_at,evidence_json FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&repairedID, &repairedTrigger, &repairedFirstHit, &acknowledged, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var repaired storedReviewEpisodeEvidence
	if err := json.Unmarshal([]byte(evidenceJSON), &repaired); err != nil {
		t.Fatal(err)
	}
	if got := reviewTruthAt(t, f, rule, "2026-08-08"); got != "true" {
		t.Fatalf("historically corrected date remained %s, want true", got)
	}
	if repairedID != episodeID || repairedTrigger != "2026-08-08" || repairedFirstHit != "2026-08-08" || acknowledged == "" || repaired.TriggerWindow.WindowStart != "2026-08-08" || repaired.TriggerResult == nil || repaired.TriggerResult.On != "2026-08-08" {
		t.Fatalf("historical correction failed to reconstruct trigger while preserving the episode/ack: id=%s trigger=%s first=%s ack=%q evidence=%+v", repairedID, repairedTrigger, repairedFirstHit, acknowledged, repaired)
	}
}

func TestReviewRollingTriggerEvidenceIncludesFalseAndUnknownDaysAcrossPagination(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-06", "1")
	requireDailyReviewPrices(t, &f)
	importReviewDailyClose(t, f, "2026-10-06", "100") // F
	for _, day := range []string{"2026-10-10", "2026-10-11", "2026-10-12"} {
		f.day(t, day)
		importReviewDailyClose(t, f, day, "200") // three initial T votes
	}
	f.day(t, "2026-10-17") // missing closes leave U votes through the current date
	spec := syntheticHistoricalReviewSpec()
	allowDailyReviewCrypto(&spec.Predicate)
	spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "150", Currency: "CNY", FXMode: "native"}}
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 3}
	rule := saveSyntheticReviewRule(t, f, "Synthetic rolling F/U evidence", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID, triggered, firstHit, evidenceJSON string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,first_hit_on,evidence_json FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episodeID, &triggered, &firstHit, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var stored storedReviewEpisodeEvidence
	if err := json.Unmarshal([]byte(evidenceJSON), &stored); err != nil {
		t.Fatal(err)
	}
	if triggered != "2026-10-12" || firstHit != "2026-10-10" || stored.TriggerWindow.WindowStart != "2026-10-03" || stored.TriggerWindow.HitCount != 3 || stored.TriggerWindow.FalseCount != 1 || stored.TriggerWindow.UnknownCount != 6 || stored.Window.WindowStart != "2026-10-07" || stored.Window.HitCount != 3 || stored.Window.UnknownCount != 7 {
		t.Fatalf("rolling trigger snapshot/current window mismatch: trigger=%s first=%s triggerWindow=%+v current=%+v", triggered, firstHit, stored.TriggerWindow, stored.Window)
	}
	truths := make(map[string]domain.ReviewTruth)
	cursor := ""
	for {
		page, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episodeID, cursor, 1)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("evidence page after %q: %+v %v", cursor, page, err)
		}
		truths[page.Items[0].On] = page.Items[0].Truth
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if truths["2026-10-06"] != domain.ReviewFalse || truths["2026-10-07"] != domain.ReviewUnknown || truths["2026-10-08"] != domain.ReviewUnknown || truths["2026-10-09"] != domain.ReviewUnknown || truths["2026-10-10"] != domain.ReviewTrue {
		t.Fatalf("paged evidence omitted false/unknown days from the rolling trigger window: %v", truths)
	}
}

func compactReviewRulePublication(item *domain.ReviewRulePublication) any {
	if item == nil {
		return nil
	}
	var triggered string
	if item.CurrentEpisode != nil {
		triggered = item.CurrentEpisode.TriggeredOn
	}
	return map[string]any{
		"days": len(item.Days), "first_day": func() string {
			if len(item.Days) == 0 {
				return ""
			}
			return item.Days[0].On
		}(),
		"first_hit": item.FirstHitOn, "triggered": triggered, "trigger_window": item.TriggerWindow,
		"latest_window": item.LatestWindow,
	}
}
