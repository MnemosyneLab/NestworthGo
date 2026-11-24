package application

import (
	"context"
	"slices"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// BalanceHistoryStartReview freezes the chosen civil day and timezone. The
// read-only snapshot is for review. Confirmation always rebuilds it from the
// shared planner under the ledger permit and uses the actual capture clock.
type BalanceHistoryStartReview struct {
	Origin         domain.HistoryOrigin
	Components     []domain.HistoryOriginComponent
	CaptureCurrent bool
	zone, day      string
	token          string
}

// PreviewBalanceHistoryStart supports origins without positive investment
// positions. The full Starting Point workflow can also review opening costs.
func (s *Service) PreviewBalanceHistoryStart(ctx context.Context, zone, day string) (BalanceHistoryStartReview, error) {
	var review BalanceHistoryStartReview
	err := s.WithRead(ctx, func(ctx context.Context) error {
		origin, err := s.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin != nil {
			return &domain.Error{Code: domain.ErrConflict, Field: "historyOrigin"}
		}
		data, err := s.prepareBalanceHistoryStart(ctx, zone, day)
		if err != nil {
			return err
		}
		review = BalanceHistoryStartReview{
			Origin: data.Origin, Components: data.Components, zone: zone, day: day, token: s.writes.previewToken(),
			CaptureCurrent: slices.ContainsFunc(data.Components, func(c domain.HistoryOriginComponent) bool {
				return c.Amount != nil && !c.Amount.IsZero()
			}),
		}
		return nil
	})
	return review, err
}

func (s *Service) ConfirmBalanceHistoryStart(ctx context.Context, review BalanceHistoryStartReview) (domain.HistoryOrigin, error) {
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
	data, err := s.prepareBalanceHistoryStart(ctx, review.zone, review.day)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	return s.saveHistoryStart(ctx, data)
}

func (s *Service) prepareBalanceHistoryStart(ctx context.Context, zone, day string) (domain.HistoryOriginData, error) {
	if strings.TrimSpace(day) == "" {
		return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate"}
	}
	data, err := s.prepareHistoryStart(ctx, zone, day, nil)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	for _, component := range data.Components {
		if component.Quantity != nil && !component.Quantity.IsZero() {
			return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost"}
		}
	}
	return data, nil
}
