package assistant

import (
	"crypto/sha256"
	"encoding/hex"
)

// ValidateState rejects malformed/future private state before exposing messages
// or treating its dedup records/history as recoverable. This is corruption
// detection, not authentication against the owner of the private database.
func ValidateState(s State) error {
	if s.Version != StateVersion || s.Ledger == "" || s.Revision == 0 || !s.Config.Valid() || len(s.Conversations) > MaxConversations {
		return ErrStorage
	}
	conversations := map[string]bool{}
	runs := map[string]bool{}
	for _, c := range s.Conversations {
		if !canonicalID(c.ID) || conversations[c.ID] || c.Updated.IsZero() || len(c.History) > MaxHistoryBytes || len(c.Runs) > MaxRuns || len(c.Events) > MaxEvents || c.HistoryDigest != historyDigest(s.Ledger, c.ID, c.History) {
			return ErrStorage
		}
		conversations[c.ID] = true
		requests := map[string]bool{}
		localRuns := map[string]bool{}
		for _, saved := range c.Runs {
			r := saved.Run
			sum := promptDigest(r.Prompt, r.Page, r.PageLeaseID)
			if (r.PageLeaseID != "" && !canonicalID(r.PageLeaseID)) || !validEvidence(r.Evidence) || !r.Page.Valid() || !canonicalID(r.ID) || runs[r.ID] || !canonicalID(r.RequestID) || requests[r.RequestID] || r.ConversationID != c.ID || r.Created.IsZero() || len(r.Prompt) == 0 || len(r.Prompt) > MaxPromptBytes || len(r.Text) > MaxTextBytes || saved.Digest != hex.EncodeToString(sum[:]) || !knownState(r.State) || !knownCode(r.Code) {
				return ErrStorage
			}
			if !((ModelConfig{Provider: r.Provider, Model: r.Model}).Valid()) || r.ConfigRevision == 0 {
				return ErrStorage
			}
			if terminal(r.State) != (r.Finished != nil) || (r.State != "succeeded" && (r.Text != "" || len(r.Evidence) != 0)) {
				return ErrStorage
			}
			runs[r.ID] = true
			localRuns[r.ID] = true
			requests[r.RequestID] = true
		}
		previous := uint64(0)
		for _, e := range c.Events {
			if e.Sequence == 0 || (previous != 0 && e.Sequence != previous+1) || !localRuns[e.RunID] || !knownState(e.State) || !knownCode(e.Code) {
				return ErrStorage
			}
			previous = e.Sequence
		}
		if previous != c.Sequence {
			return ErrStorage
		}
	}
	return nil
}
func knownState(s string) bool { return s == "accepted" || s == "running" || terminal(s) }
func knownCode(s string) bool {
	if IsResponseFailureCode(s) {
		return true
	}
	switch s {
	case "provider_authentication", "provider_rate_limited", "provider_request_rejected", "provider_unavailable", "provider_connection", "provider_response_rejected", "model_history_rejected", "tool_result_rejected", "assistant_budget_exceeded":
		return true
	}
	return s == "" || s == "restart_requires_new_request" || s == "authority_revoked" || s == "provider_or_storage_failure" || s == "cancelled" || s == "timeout"
}

func historyDigest(ledger, id string, history []byte) string {
	if len(history) == 0 {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(ledger))
	h.Write([]byte{0})
	h.Write([]byte(id))
	h.Write([]byte{0})
	h.Write(history)
	return hex.EncodeToString(h.Sum(nil))
}
