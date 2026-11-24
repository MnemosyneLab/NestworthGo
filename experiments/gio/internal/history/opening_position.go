package history

import (
	"context"
	"slices"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type OpeningPositionLedger interface {
	WithRead(context.Context, func(context.Context) error) error
	WithWrite(context.Context, func(context.Context) error) error
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	CreateHolding(context.Context, application.HoldingInput) (domain.Holding, error)
	MobileDataVersion() string
}

type OpeningPositionSources struct {
	Account     domain.Account
	Instruments []domain.Instrument
}

func LoadOpeningPositionSources(ctx context.Context, ledger OpeningPositionLedger, id domain.AccountID) (out OpeningPositionSources, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		out, err = openingPositionSources(ctx, ledger, id)
		return err
	})
	return out, err
}

func openingPositionSources(ctx context.Context, ledger OpeningPositionLedger, id domain.AccountID) (OpeningPositionSources, error) {
	origin, err := ledger.HistoryOrigin(ctx)
	if err != nil {
		return OpeningPositionSources{}, err
	}
	if origin != nil {
		return OpeningPositionSources{}, &domain.Error{Code: domain.ErrConflict, Field: "historyOrigin"}
	}
	accounts, err := ledger.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return OpeningPositionSources{}, err
	}
	at := slices.IndexFunc(accounts, func(row domain.AccountRecord) bool { return row.Account.ID == id })
	if at < 0 || !TradeAccount(accounts[at].Account) {
		return OpeningPositionSources{}, &domain.Error{Code: domain.ErrValidation, Field: "accountId"}
	}
	account := accounts[at].Account
	var accountIDs []domain.AccountID
	for _, row := range accounts {
		if row.Account.TrackingMode == domain.TrackingHoldings {
			accountIDs = append(accountIDs, row.Account.ID)
		}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, accountIDs)
	if err != nil {
		return OpeningPositionSources{}, err
	}
	// The Starting Point slice can capture one positive opening position.
	// Keep this entry from creating a second one it could not capture.
	for _, positions := range holdings {
		if slices.ContainsFunc(positions, func(h domain.Holding) bool { return !h.Quantity.IsZero() }) {
			return OpeningPositionSources{}, &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
		}
	}
	instruments, err := ledger.ListInstruments(ctx, false)
	if err != nil {
		return OpeningPositionSources{}, err
	}
	managed, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return OpeningPositionSources{}, err
	}
	instruments = slices.DeleteFunc(instruments, func(i domain.Instrument) bool {
		_, product := managed[i.ID.String()]
		return product || i.ArchivedAt != nil || i.QuoteCurrency != account.DefaultCurrency ||
			slices.ContainsFunc(holdings[id], func(h domain.Holding) bool {
				return h.ArchivedAt == nil && h.InstrumentID == i.ID
			})
	})
	return OpeningPositionSources{Account: account, Instruments: instruments}, nil
}

type OpeningPositionReview struct {
	Account    domain.Account
	Instrument domain.Instrument
	Quantity   domain.Quantity
	token      string
}

func PreviewOpeningPosition(ctx context.Context, ledger OpeningPositionLedger, account domain.AccountID, id domain.InstrumentID, value string) (review OpeningPositionReview, err error) {
	quantity, err := domain.ParseQuantity(strings.TrimSpace(value))
	if err != nil || quantity.IsZero() {
		return review, &domain.Error{Code: domain.ErrValidation, Field: "quantity"}
	}
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		sources, err := openingPositionSources(ctx, ledger, account)
		if err != nil {
			return err
		}
		at := slices.IndexFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == id })
		if at < 0 {
			return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId"}
		}
		review = OpeningPositionReview{Account: sources.Account, Instrument: sources.Instruments[at], Quantity: quantity, token: ledger.MobileDataVersion()}
		return nil
	})
	return review, err
}

func (r OpeningPositionReview) Confirm(ctx context.Context, ledger OpeningPositionLedger) (holding domain.Holding, err error) {
	err = ledger.WithWrite(ctx, func(ctx context.Context) error {
		if r.token == "" || r.token != ledger.MobileDataVersion() {
			return &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken"}
		}
		// CreateHolding can record a position adjustment after History. Check
		// the origin under the same write permit to stay on the baseline path.
		origin, err := ledger.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin != nil {
			return &domain.Error{Code: domain.ErrConflict, Field: "historyOrigin"}
		}
		holding, err = ledger.CreateHolding(ctx, application.HoldingInput{
			AccountID: r.Account.ID.String(), InstrumentID: r.Instrument.ID.String(), Quantity: r.Quantity.Canonical(),
		})
		return err
	})
	return holding, err
}

// Verify only reads the unique active account/instrument pair. A known
// response additionally requires the exact ID returned by CreateHolding.
func (r OpeningPositionReview) Verify(ctx context.Context, ledger OpeningPositionLedger, saved domain.HoldingID) (found bool, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{r.Account.ID})
		if err != nil {
			return err
		}
		for _, h := range holdings[r.Account.ID] {
			if h.ArchivedAt == nil && h.InstrumentID == r.Instrument.ID {
				if (saved != "" && saved == h.ID) || (saved == "" && h.Quantity.Canonical() == r.Quantity.Canonical()) {
					found = true
				} else {
					return &domain.Error{Code: domain.ErrConflict, Field: "holdingId"}
				}
				break
			}
		}
		return nil
	})
	return found, err
}

type OpeningHoldingReader interface {
	StartingPointDraft(context.Context) ([]domain.StartingPointHoldingView, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
}

// EligibleStartingPointHolding narrows the Wails draft to the single ordinary,
// same-currency holding that the Gio review form can describe.
func EligibleStartingPointHolding(ctx context.Context, ledger OpeningHoldingReader) (*domain.StartingPointHoldingView, error) {
	draft, err := ledger.StartingPointDraft(ctx)
	if err != nil || len(draft) == 0 {
		return nil, err
	}
	unsupported := &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
	if len(draft) != 1 {
		return nil, unsupported
	}
	row := draft[0]
	products, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return nil, err
	}
	if _, managed := products[row.InstrumentID.String()]; managed {
		return nil, unsupported
	}
	accounts, err := ledger.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return nil, err
	}
	var ids []domain.AccountID
	for _, account := range accounts {
		if TradeAccount(account.Account) && account.Account.DefaultCurrency == row.Currency {
			ids = append(ids, account.Account.ID)
		}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, positions := range holdings {
		for _, holding := range positions {
			if holding.ID == row.HoldingID {
				return &row, nil
			}
		}
	}
	return nil, unsupported
}
