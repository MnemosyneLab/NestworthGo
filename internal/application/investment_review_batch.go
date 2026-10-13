package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewEnrollmentItem struct {
	InstrumentID string `json:"instrumentId"`
	Enabled      bool   `json:"enabled"`
}
type ReviewSnoozeItem struct {
	InstrumentID string `json:"instrumentId"`
	Days         int    `json:"days"`
}
type ReviewAcknowledgeItem struct {
	InstrumentID string `json:"instrumentId"`
	EpisodeID    string `json:"episodeId"`
}
type ReviewEnrollmentBatchInput struct {
	ExpectedRevision int                    `json:"expectedRevision"`
	Items            []ReviewEnrollmentItem `json:"items"`
}
type ReviewSnoozeBatchInput struct {
	ExpectedRevision int                `json:"expectedRevision"`
	Items            []ReviewSnoozeItem `json:"items"`
}
type ReviewAcknowledgeBatchInput struct {
	ExpectedRevision int                     `json:"expectedRevision"`
	Items            []ReviewAcknowledgeItem `json:"items"`
}
type ReviewBatchReceiptItem struct {
	InstrumentID string `json:"instrumentId"`
	Changed      bool   `json:"changed"`
}
type ReviewBatchReceipt struct {
	PreviousRevision int                      `json:"previousRevision"`
	Revision         int                      `json:"revision"`
	RequestedCount   int                      `json:"requestedCount"`
	ChangedCount     int                      `json:"changedCount"`
	Items            []ReviewBatchReceiptItem `json:"items"`
}
type reviewBatchChange struct {
	id      string
	enabled bool
	days    int
	episode string
}

func (s *Service) SetInvestmentReviewEnrollmentBatch(ctx context.Context, in ReviewEnrollmentBatchInput) (ReviewBatchReceipt, error) {
	items := make([]reviewBatchChange, 0, len(in.Items))
	for _, v := range in.Items {
		items = append(items, reviewBatchChange{id: v.InstrumentID, enabled: v.Enabled})
	}
	return s.applyInvestmentReviewBatch(ctx, "enrollment", in.ExpectedRevision, items)
}
func (s *Service) SetInvestmentReviewSnoozeBatch(ctx context.Context, in ReviewSnoozeBatchInput) (ReviewBatchReceipt, error) {
	items := make([]reviewBatchChange, 0, len(in.Items))
	for _, v := range in.Items {
		items = append(items, reviewBatchChange{id: v.InstrumentID, days: v.Days})
	}
	return s.applyInvestmentReviewBatch(ctx, "snooze", in.ExpectedRevision, items)
}
func (s *Service) AcknowledgeInvestmentReviewsBatch(ctx context.Context, in ReviewAcknowledgeBatchInput) (ReviewBatchReceipt, error) {
	items := make([]reviewBatchChange, 0, len(in.Items))
	for _, v := range in.Items {
		items = append(items, reviewBatchChange{id: v.InstrumentID, episode: v.EpisodeID})
	}
	return s.applyInvestmentReviewBatch(ctx, "acknowledge", in.ExpectedRevision, items)
}
func (s *Service) applyInvestmentReviewBatch(ctx context.Context, kind string, expected int, items []reviewBatchChange) (ReviewBatchReceipt, error) {
	empty := ReviewBatchReceipt{}
	if len(items) < 1 || len(items) > 100 || expected < 0 {
		return empty, &domain.Error{Code: domain.ErrValidation, Message: "batch supports 1 to 100 items and a nonnegative revision"}
	}
	seen := map[string]bool{}
	for _, v := range items {
		if _, err := domain.ParseInstrumentID(v.id); err != nil {
			return empty, err
		}
		if seen[v.id] {
			return empty, &domain.Error{Code: domain.ErrValidation, Message: "duplicate review target"}
		}
		seen[v.id] = true
		if kind == "snooze" && v.days != 0 && v.days != 7 && v.days != 30 {
			return empty, &domain.Error{Code: domain.ErrValidation, Message: "days must be 0, 7 or 30"}
		}
		if kind == "acknowledge" && (v.episode == "" || len(v.episode) > 512) {
			return empty, &domain.Error{Code: domain.ErrValidation, Message: "episodeId must be a current review episode"}
		}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return empty, err
	}
	defer unlock()
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	report, state, key, err := s.investmentReviews(ctx)
	if err != nil {
		return empty, err
	}
	if key == "" || state.Revision != expected {
		return empty, &domain.Error{Code: domain.ErrConflict, Message: "review settings changed; reload before saving"}
	}
	current := map[string]domain.InvestmentReviewItem{}
	for _, v := range report.Items {
		current[v.InstrumentID] = v
	}
	today, err := time.Parse(time.DateOnly, report.LocalDate)
	if err != nil {
		return empty, err
	}
	receipt := ReviewBatchReceipt{PreviousRevision: state.Revision, Revision: state.Revision, RequestedCount: len(items), Items: []ReviewBatchReceiptItem{}}
	for _, v := range items {
		item, ok := current[v.id]
		if !ok {
			return empty, &domain.Error{Code: domain.ErrConflict, Message: "review target no longer eligible"}
		}
		changed := false
		switch kind {
		case "enrollment":
			changed = state.Enrolled[v.id] != v.enabled
			if changed {
				state.Enrolled[v.id] = v.enabled
			}
		case "snooze":
			until := ""
			if v.days != 0 {
				until = today.AddDate(0, 0, v.days).Format(time.DateOnly)
			}
			changed = state.SnoozedUntil[v.id] != until
			if until == "" {
				delete(state.SnoozedUntil, v.id)
			} else {
				state.SnoozedUntil[v.id] = until
			}
		case "acknowledge":
			if item.EpisodeID == "" || item.EpisodeID != v.episode {
				return empty, &domain.Error{Code: domain.ErrConflict, Message: "review episode is no longer current"}
			}
			changed = !state.Acknowledged[v.episode]
			state.Acknowledged[v.episode] = true
		}
		receipt.Items = append(receipt.Items, ReviewBatchReceiptItem{InstrumentID: v.id, Changed: changed})
		if changed {
			receipt.ChangedCount++
		}
	}
	if receipt.ChangedCount == 0 {
		return receipt, nil
	}
	state.Revision++
	receipt.Revision = state.Revision
	b, err := json.Marshal(state)
	if err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := s.reviewStore.SaveConfiguration(key, b); err != nil {
		return empty, err
	}
	return receipt, nil
}
