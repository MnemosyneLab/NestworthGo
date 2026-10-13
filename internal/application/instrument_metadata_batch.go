package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type InstrumentMetadataRepository interface {
	ReadInstrumentMetadataFiltered(context.Context, domain.InstrumentMetadataReadQuery) (domain.InstrumentMetadataReadSnapshot, error)
	UpdateInstrumentMetadataBatch(context.Context, domain.HouseholdID, []domain.InstrumentMetadataUpdate, time.Time) (domain.InstrumentMetadataReceipt, error)
}
type InstrumentMetadataBatchInput struct {
	Items []domain.InstrumentMetadataUpdate `json:"items"`
}

func (s *Service) FilteredInstrumentMetadata(ctx context.Context, q domain.InstrumentMetadataReadQuery) (domain.InstrumentMetadataReadSnapshot, error) {
	r, ok := s.repository.(InstrumentMetadataRepository)
	if !ok {
		return domain.InstrumentMetadataReadSnapshot{}, &domain.Error{Code: domain.ErrUnavailable, Message: "instrument metadata storage unavailable"}
	}
	return r.ReadInstrumentMetadataFiltered(ctx, q)
}
func (s *Service) UpdateInstrumentsBatch(ctx context.Context, in InstrumentMetadataBatchInput) (domain.InstrumentMetadataReceipt, error) {
	if len(in.Items) < 1 || len(in.Items) > 100 {
		return domain.InstrumentMetadataReceipt{}, &domain.Error{Code: domain.ErrValidation, Message: "batch supports 1 to 100 items"}
	}
	in.Items = append([]domain.InstrumentMetadataUpdate{}, in.Items...)
	seen := map[string]bool{}
	for n, v := range in.Items {
		parsed, parseErr := domain.ParseInstrumentID(v.ID)
		if parseErr != nil {
			return domain.InstrumentMetadataReceipt{}, parseErr
		}
		v.ID = parsed.String()
		in.Items[n].ID = v.ID
		if seen[v.ID] || len(v.ExpectedMetadataVersion) != 64 {
			return domain.InstrumentMetadataReceipt{}, &domain.Error{Code: domain.ErrValidation, Message: "duplicate target or invalid metadataVersion"}
		}
		seen[v.ID] = true
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.InstrumentMetadataReceipt{}, err
	}
	defer unlock()
	r, ok := s.repository.(InstrumentMetadataRepository)
	if !ok {
		return domain.InstrumentMetadataReceipt{}, &domain.Error{Code: domain.ErrUnavailable, Message: "instrument metadata storage unavailable"}
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.InstrumentMetadataReceipt{}, err
	}
	receipt, err := r.UpdateInstrumentMetadataBatch(ctx, h.ID, in.Items, s.clock())
	if err == nil && receipt.ChangedCount > 0 {
		s.invalidateAnalysis()
	}
	return receipt, err
}
