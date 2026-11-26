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
	holdings, err := ledger.HoldingsByAccounts(ctx, []domain.AccountID{id})
	if err != nil {
		return OpeningPositionSources{}, err
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
		return product || i.ArchivedAt != nil ||
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

type StartingPointHolding struct {
	domain.StartingPointHoldingView
	AccountName string
}

// LoadStartingPointHoldings adds the account context to Wails' cost draft.
// Each opening cost uses the instrument's currency, including foreign positions.
func LoadStartingPointHoldings(ctx context.Context, ledger OpeningHoldingReader) ([]StartingPointHolding, error) {
	draft, err := ledger.StartingPointDraft(ctx)
	if err != nil || len(draft) == 0 {
		return nil, err
	}
	unsupported := &domain.Error{Code: domain.ErrConflict, Field: "openingHoldings"}
	products, err := ledger.InstrumentProductOwnership(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := ledger.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	ids := make([]domain.AccountID, 0, len(accounts))
	accountNames := make(map[domain.AccountID]string, len(accounts))
	for _, account := range accounts {
		if account.Account.TrackingMode == domain.TrackingHoldings {
			ids = append(ids, account.Account.ID)
			accountNames[account.Account.ID] = account.Account.Name
		}
	}
	holdings, err := ledger.HoldingsByAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	byHolding := make(map[domain.HoldingID]domain.AccountID)
	for account, positions := range holdings {
		for _, holding := range positions {
			if holding.ArchivedAt == nil {
				byHolding[holding.ID] = account
			}
		}
	}
	rows := make([]StartingPointHolding, 0, len(draft))
	for _, row := range draft {
		if _, managed := products[row.InstrumentID.String()]; managed || byHolding[row.HoldingID] == "" {
			return nil, unsupported
		}
		rows = append(rows, StartingPointHolding{StartingPointHoldingView: row, AccountName: accountNames[byHolding[row.HoldingID]]})
	}
	return rows, nil
}
