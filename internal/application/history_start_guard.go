package application

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoryStartReview holds the opening facts for display and the inputs for
// confirmation. The shared planner runs again at the actual capture instant.
type HistoryStartReview struct {
	Origin         domain.HistoryOrigin
	Components     []domain.HistoryOriginComponent
	CaptureCurrent bool
	zone, day      string
	costs          map[domain.HoldingID]string
	token          string
}

func (s *Service) PreviewHistoryStart(ctx context.Context, zone, day string, costs map[domain.HoldingID]string) (HistoryStartReview, error) {
	var review HistoryStartReview
	err := s.WithRead(ctx, func(ctx context.Context) error {
		origin, err := s.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin != nil {
			return &domain.Error{Code: domain.ErrConflict, Field: "historyOrigin"}
		}
		// Collect the canonical values into a new map. Later edits to the
		// caller's form must never change the reviewed inputs.
		input := HistoryStartReview{zone: zone, day: day, costs: make(map[domain.HoldingID]string, len(costs))}
		for holding, raw := range costs {
			price, err := domain.ParseUnitPrice(strings.TrimSpace(raw))
			if err != nil {
				return err
			}
			input.costs[holding] = price.Canonical()
		}
		data, err := s.prepareReviewedHistoryStart(ctx, input)
		if err != nil {
			return err
		}
		review = input
		review.Origin, review.Components, review.token = data.Origin, data.Components, s.writes.previewToken()
		for _, part := range data.Components {
			if (part.Amount != nil && !part.Amount.IsZero()) || (part.Quantity != nil && !part.Quantity.IsZero()) {
				review.CaptureCurrent = true
				break
			}
		}
		return nil
	})
	return review, err
}

func (s *Service) ConfirmHistoryStart(ctx context.Context, review HistoryStartReview) (domain.HistoryOrigin, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	defer unlock()
	if review.token == "" {
		return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrValidation, Field: "previewToken"}
	}
	// The persisted singleton is the durable result, including after a process
	// restart. An existing origin and its components are always retained.
	if existing, err := s.HistoryOrigin(ctx); err != nil {
		return domain.HistoryOrigin{}, err
	} else if existing != nil {
		return *existing, nil
	}
	if s.writes.previewToken() != review.token {
		return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken"}
	}
	data, err := s.prepareReviewedHistoryStart(ctx, review)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	return s.saveHistoryStart(ctx, data)
}

func (s *Service) prepareReviewedHistoryStart(ctx context.Context, review HistoryStartReview) (domain.HistoryOriginData, error) {
	if strings.TrimSpace(review.day) == "" {
		return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate"}
	}
	data, err := s.prepareHistoryStart(ctx, review.zone, review.day, review.costs)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	count := 0
	for _, part := range data.Components {
		if part.Quantity == nil || part.Quantity.IsZero() {
			continue
		}
		// The planner supports quote fallback for other entry points. An
		// interactive review must name every positive holding explicitly.
		if part.HoldingID == nil || review.costs[*part.HoldingID] == "" {
			return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
		}
		count++
	}
	if count != len(review.costs) {
		return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
	}
	return data, nil
}
