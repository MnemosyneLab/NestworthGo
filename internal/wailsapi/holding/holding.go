// Package holding adapts internal/application.Service's Holding position
// CRUD and Account cash-value surface for the Wails IPC boundary.
package holding

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// CreateHoldingRequest mirrors application.HoldingInput.
type CreateHoldingRequest struct {
	AccountID    string  `json:"accountId"`
	InstrumentID string  `json:"instrumentId"`
	Quantity     string  `json:"quantity"`
	UnitCost     string  `json:"unitCost,omitempty"`
	Note         *string `json:"note,omitempty"`
	SortOrder    int     `json:"sortOrder,omitempty"`
}

func (s *Service) CreateHolding(ctx context.Context, request CreateHoldingRequest) (wire.HoldingDTO, error) {
	holding, err := s.app.CreateHolding(ctx, application.HoldingInput{
		AccountID: request.AccountID, InstrumentID: request.InstrumentID, Quantity: request.Quantity,
		UnitCost: request.UnitCost, Note: request.Note, SortOrder: request.SortOrder,
	})
	if err != nil {
		return wire.HoldingDTO{}, apierror.Wrap(err)
	}
	return wire.FromHolding(holding), nil
}

// UpdateHoldingRequest mirrors application.HoldingUpdateInput's explicit
// "Set flags" shape: a field is only applied when its *Set companion is
// true, which is how the application layer already distinguishes "leave
// unchanged" from "clear to empty/zero" for Note and SortOrder.
type UpdateHoldingRequest struct {
	Quantity     string  `json:"quantity,omitempty"`
	QuantitySet  bool    `json:"quantitySet,omitempty"`
	Note         *string `json:"note,omitempty"`
	NoteSet      bool    `json:"noteSet,omitempty"`
	SortOrder    int     `json:"sortOrder,omitempty"`
	SortOrderSet bool    `json:"sortOrderSet,omitempty"`
}

func (s *Service) UpdateHolding(ctx context.Context, id string, request UpdateHoldingRequest) (wire.HoldingDTO, error) {
	holdingID, err := domain.ParseHoldingID(id)
	if err != nil {
		return wire.HoldingDTO{}, apierror.Wrap(err)
	}
	holding, err := s.app.UpdateHolding(ctx, holdingID, application.HoldingUpdateInput{
		Quantity: request.Quantity, QuantitySet: request.QuantitySet, Note: request.Note, NoteSet: request.NoteSet,
		SortOrder: request.SortOrder, SortOrderSet: request.SortOrderSet,
	})
	if err != nil {
		return wire.HoldingDTO{}, apierror.Wrap(err)
	}
	return wire.FromHolding(holding), nil
}

// UpdateHoldingQuantity is the dedicated "record a quantity change outside
// of History" path application.Service exposes for pre-history use; once
// history has started, the application layer rejects it with
// domain.ErrConflict and the frontend must use the history service's
// RecordChange instead.
func (s *Service) UpdateHoldingQuantity(ctx context.Context, id, quantity string) (wire.HoldingDTO, error) {
	holdingID, err := domain.ParseHoldingID(id)
	if err != nil {
		return wire.HoldingDTO{}, apierror.Wrap(err)
	}
	holding, err := s.app.UpdateHoldingQuantity(ctx, holdingID, quantity)
	if err != nil {
		return wire.HoldingDTO{}, apierror.Wrap(err)
	}
	return wire.FromHolding(holding), nil
}

func (s *Service) ListHoldings(ctx context.Context, accountID string, includeArchived bool) ([]wire.HoldingDTO, error) {
	parsedAccountID, err := domain.ParseAccountID(accountID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	holdings, err := s.app.ListHoldings(ctx, parsedAccountID, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromHoldings(holdings), nil
}

// HoldingsByAccounts loads holdings for several accounts in one call,
// returned as a map keyed by account ID string (JSON object keys must be
// strings; domain.AccountID already is one).
func (s *Service) HoldingsByAccounts(ctx context.Context, accountIDs []string) (map[string][]wire.HoldingDTO, error) {
	parsed := make([]domain.AccountID, 0, len(accountIDs))
	for _, id := range accountIDs {
		accountID, err := domain.ParseAccountID(id)
		if err != nil {
			return nil, apierror.Wrap(err)
		}
		parsed = append(parsed, accountID)
	}
	grouped, err := s.app.HoldingsByAccounts(ctx, parsed)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	result := make(map[string][]wire.HoldingDTO, len(grouped))
	for accountID, holdings := range grouped {
		result[accountID.String()] = wire.FromHoldings(holdings)
	}
	return result, nil
}

func (s *Service) ArchiveHolding(ctx context.Context, id string, archived bool) error {
	holdingID, err := domain.ParseHoldingID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveHolding(ctx, holdingID, archived))
}

func (s *Service) AppendAccountCashValue(ctx context.Context, accountID, amount, currency, effectiveAt string) (wire.AccountCashValueDTO, error) {
	parsedAccountID, err := domain.ParseAccountID(accountID)
	if err != nil {
		return wire.AccountCashValueDTO{}, apierror.Wrap(err)
	}
	value, err := s.app.AppendAccountCashValue(ctx, parsedAccountID, amount, currency, effectiveAt)
	if err != nil {
		return wire.AccountCashValueDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountCashValue(value), nil
}

func (s *Service) ListAccountCashValues(ctx context.Context, accountID string) ([]wire.AccountCashValueDTO, error) {
	parsedAccountID, err := domain.ParseAccountID(accountID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	values, err := s.app.ListAccountCashValues(ctx, parsedAccountID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromAccountCashValues(values), nil
}
