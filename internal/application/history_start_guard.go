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
	holding        domain.HoldingID
	cost           string
	token          string
}

func (s *Service) PreviewHistoryStart(ctx context.Context, zone, day string, holding domain.HoldingID, cost string) (HistoryStartReview, error) {
	var review HistoryStartReview
	err := s.WithRead(ctx, func(ctx context.Context) error {
		origin, err := s.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin != nil {
			return &domain.Error{Code: domain.ErrConflict, Field: "historyOrigin"}
		}
		input := HistoryStartReview{zone: zone, day: day, holding: holding, cost: cost}
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
	costs := make(map[domain.HoldingID]string, 1)
	if review.holding != "" {
		price, err := domain.ParseUnitPrice(strings.TrimSpace(review.cost))
		if err != nil {
			return domain.HistoryOriginData{}, err
		}
		costs[review.holding] = price.Canonical()
	}
	data, err := s.prepareHistoryStart(ctx, review.zone, review.day, costs)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	found := false
	for _, part := range data.Components {
		if part.Quantity == nil || part.Quantity.IsZero() {
			continue
		}
		// Require an explicit reviewed price for every opening holding. Gio's
		// current form can review one position; the planner's quote fallback
		// must never silently set the cost of an omitted position.
		if part.HoldingID == nil || *part.HoldingID != review.holding {
			return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
		}
		found = true
	}
	if found != (review.holding != "") {
		return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
	}
	return data, nil
}
