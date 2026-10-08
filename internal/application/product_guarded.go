package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func productReviewedTimeError() error {
	return &domain.Error{Code: domain.ErrValidation, Field: "reviewedAt", Message: "reviewed time is owned by the stored-plan service"}
}

func productReservationGUIError() error {
	return &domain.Error{Code: domain.ErrUnresolvedReservationRelease, Message: "this operation changes reservations; resolve it in the GUI"}
}

// PreviewProductOperationGuarded serializes all dependencies with writes and
// restore. The exact frozen command is returned as NormalizedJSON for storage.
func (s *Service) PreviewProductOperationGuarded(ctx context.Context, command ProductCommand) (ProductOperationPreview, string, error) {
	if command.ReviewedAt != "" {
		return ProductOperationPreview{}, "", productReviewedTimeError()
	}
	if err := validateGuardedProductCommand(command); err != nil {
		return ProductOperationPreview{}, "", err
	}
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return ProductOperationPreview{}, "", err
	}
	defer unlock()
	preview, err := s.previewProductOperation(ctx, command, true)
	return preview, s.writes.previewToken(), err
}

// RecordProductOperationGuarded checks a durable business receipt before the
// process/restore token. A stale uncommitted plan can never become a new write.
func (s *Service) RecordProductOperationGuarded(ctx context.Context, command ProductCommand, mutationID, stateHash, token string) (ProductOperationReceipt, error) {
	if command.ReviewedAt == "" {
		return ProductOperationReceipt{}, productReviewedTimeError()
	}
	if _, err := time.Parse(time.RFC3339Nano, command.ReviewedAt); err != nil {
		return ProductOperationReceipt{}, productReviewedTimeError()
	}
	if err := validateGuardedProductCommand(command); err != nil {
		return ProductOperationReceipt{}, err
	}
	return s.recordProductOperation(ctx, command, mutationID, stateHash, &token)
}

func validateGuardedProductCommand(command ProductCommand) error {
	switch command.Kind {
	case domain.ProductOpOpen, domain.ProductOpRecordExisting, domain.ProductOpReceiveInterest, domain.ProductOpSettle, domain.ProductOpUndo:
	default:
		return &domain.Error{Code: domain.ErrValidation, Field: "kind", Message: "unsupported stored product operation"}
	}
	if command.Renew != nil || command.Settle != nil && len(command.Settle.ReleaseReservationIDs) != 0 {
		return productReservationGUIError()
	}
	_, _, err := normalizeProductCommand(command)
	return err
}

func freezeProductCommand(command ProductCommand, origin *domain.HistoryOrigin, now time.Time) (ProductCommand, error) {
	// Clone before changing pointer payloads; do not mutate the caller's command.
	raw, err := json.Marshal(command)
	if err != nil {
		return command, err
	}
	var cloned ProductCommand
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return command, err
	}
	command = cloned
	command.ReviewedAt = now.UTC().Format(time.RFC3339Nano)
	resolve := func(at, date, clock *string) error {
		value, err := resolveProductEffectiveTime(*at, *date, *clock, origin, now)
		if err != nil {
			return err
		}
		if value == "" {
			value = command.ReviewedAt
		}
		*at, *date, *clock = value, "", ""
		return nil
	}
	switch command.Kind {
	case domain.ProductOpOpen:
		if command.Open != nil {
			err = resolve(&command.Open.EffectiveAt, &command.Open.EffectiveLocalDate, &command.Open.EffectiveLocalTime)
		}
	case domain.ProductOpReceiveInterest:
		if command.ReceiveInterest != nil {
			err = resolve(&command.ReceiveInterest.EffectiveAt, &command.ReceiveInterest.EffectiveLocalDate, &command.ReceiveInterest.EffectiveLocalTime)
		}
	case domain.ProductOpSettle:
		if command.Settle != nil {
			err = resolve(&command.Settle.EffectiveAt, &command.Settle.EffectiveLocalDate, &command.Settle.EffectiveLocalTime)
		}
	case domain.ProductOpRecordExisting:
		if command.RecordExisting != nil {
			if command.RecordExisting.EffectiveAt != "" {
				return command, productReviewedTimeError()
			}
			command.RecordExisting.EffectiveAt = command.ReviewedAt
		}
	}
	return command, err
}
