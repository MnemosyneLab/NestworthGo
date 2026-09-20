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

// AppendAccountCashValue is the cash baseline/import entry point before
// history starts. After history starts, application.Service translates the
// requested resulting balance into a reconciliation cash_in/cash_out and
// commits it through the same PreviewChange/RecordChange engine; it does not
// create a second persistence path.
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
