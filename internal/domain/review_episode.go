package domain

import "time"

// ReviewEpisodeIdentity is durable across daily window rotation and evidence
// gaps. The application supplies a new UUID only if a distinct episode opens.
// Acknowledgement belongs to this identity, not a rule, instrument or date.
type ReviewEpisodeIdentity struct {
	ID              string
	TriggeredOn     string
	FirstDetectedAt time.Time
	AcknowledgedAt  *time.Time
	ResolvedAt      *time.Time
	ResolvedReason  string
}

type ReviewEpisodeDecision struct {
	Current *ReviewEpisodeIdentity
	Closed  *ReviewEpisodeIdentity
	Opened  bool
	State   ReviewTruth
}

// AdvanceReviewEpisode consumes already-evaluated date-keyed policy states.
// The caller must supply complete calendar coverage from the existing open
// episode's trigger date through L; omitted dates are unknown. A proven F
// splits an episode, while U cannot prove recovery. This does not invent
// historic detections: FirstDetectedAt is the actual reconciliation time.
func AdvanceReviewEpisode(firstOn, latestClosedOn string, policyByDay map[string]ReviewTruth, currentQuantityKnown, currentHeld, archived bool, existing *ReviewEpisodeIdentity, nextID string, detectedAt time.Time) (ReviewEpisodeDecision, error) {
	first, err := time.Parse(time.DateOnly, firstOn)
	if err != nil {
		return ReviewEpisodeDecision{}, validation("firstOn", "must be a valid ISO civil date")
	}
	last, err := time.Parse(time.DateOnly, latestClosedOn)
	if err != nil || last.Before(first) {
		return ReviewEpisodeDecision{}, validation("latestClosedOn", "must be on or after firstOn")
	}
	if detectedAt.IsZero() {
		return ReviewEpisodeDecision{}, validation("detectedAt", "must be an actual reconciliation time")
	}
	if existing != nil {
		triggered, err := time.Parse(time.DateOnly, existing.TriggeredOn)
		if existing.ID == "" || existing.ResolvedAt != nil || err != nil || triggered.Before(first) || triggered.After(last) {
			return ReviewEpisodeDecision{}, validation("existingEpisode", "needs an open identity and coverage from its trigger date")
		}
	}
	result := ReviewEpisodeDecision{State: policyByDay[latestClosedOn]}
	if !result.State.valid() {
		return ReviewEpisodeDecision{}, validation("policyByDay", "unsupported policy truth value")
	}
	lastProvenFalse := ""
	firstTrueSinceFalse := ""
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		on := day.Format(time.DateOnly)
		state := policyByDay[on]
		if !state.valid() {
			return ReviewEpisodeDecision{}, validation("policyByDay", "unsupported policy truth value")
		}
		if state == ReviewFalse {
			lastProvenFalse = on
			firstTrueSinceFalse = ""
		} else if state == ReviewTrue && firstTrueSinceFalse == "" {
			firstTrueSinceFalse = on
		}
	}
	closeReason := ""
	if archived {
		closeReason = "rule_archived"
	} else if currentQuantityKnown && !currentHeld {
		closeReason = "position_closed"
	} else if existing != nil && lastProvenFalse >= existing.TriggeredOn {
		closeReason = "recovered"
	}
	if existing != nil && closeReason != "" {
		closed := *existing
		closed.ResolvedReason = closeReason
		closedAt := detectedAt.UTC()
		closed.ResolvedAt = &closedAt
		result.Closed = &closed
	}
	if archived || !currentQuantityKnown || !currentHeld || result.State != ReviewTrue {
		if existing != nil && result.Closed == nil {
			retained := *existing
			result.Current = &retained // U keeps both identity and acknowledgement.
		}
		return result, nil
	}
	if existing != nil && result.Closed == nil {
		retained := *existing
		result.Current = &retained
		return result, nil
	}
	if nextID == "" || firstTrueSinceFalse == "" {
		return ReviewEpisodeDecision{}, validation("nextEpisodeID", "a new episode needs a stable ID and a proven trigger day")
	}
	result.Current = &ReviewEpisodeIdentity{ID: nextID, TriggeredOn: firstTrueSinceFalse, FirstDetectedAt: detectedAt.UTC()}
	result.Opened = true
	return result, nil
}
