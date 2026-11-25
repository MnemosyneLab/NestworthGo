package history

import (
	"context"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReferenceReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
}

type DetailReader interface {
	Reader
	ReferenceReader
	Activity(context.Context, domain.ActivityID) (domain.Activity, error)
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	RepeatEligibleActivities(context.Context, []domain.Activity) (map[domain.ActivityID]bool, error)
}

type References struct {
	Origin      *domain.HistoryOrigin
	Location    *time.Location
	Accounts    []domain.AccountRecord
	Instruments []domain.Instrument
}

func LoadReferences(ctx context.Context, service ReferenceReader) (References, error) {
	origin, zone, err := historyClock(ctx, service)
	if err != nil || origin == nil {
		return References{}, err
	}
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return References{}, err
	}
	instruments, err := service.ListInstruments(ctx, true)
	if err != nil {
		return References{}, err
	}
	return References{Origin: origin, Location: zone, Accounts: accounts, Instruments: instruments}, nil
}

func historyClock(ctx context.Context, service ReferenceReader) (*domain.HistoryOrigin, *time.Location, error) {
	origin, err := service.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		return origin, nil, err
	}
	zone, err := time.LoadLocation(origin.Timezone)
	return origin, zone, err
}

type Detail struct {
	Activity             domain.Activity
	Accounts             map[domain.AccountID]string
	HoldingAccounts      map[domain.HoldingID]domain.AccountID
	Instruments          map[domain.InstrumentID]string
	InstrumentCurrencies map[domain.InstrumentID]domain.CurrencyCode
	Location             *time.Location
	Links                AmendmentLinks
	Amendable            bool
	Reversible           bool
	Repeatable           bool
	LiabilityAccount     domain.AccountID
}

var ErrActivityUnavailable = errors.New("activity unavailable")

func LoadDetail(ctx context.Context, service DetailReader, id domain.ActivityID) (Detail, error) {
	record, err := service.Activity(ctx, id)
	if err != nil {
		var issue *domain.Error
		if errors.As(err, &issue) && issue.Code == domain.ErrNotFound {
			return Detail{}, ErrActivityUnavailable
		}
		return Detail{}, err
	}
	if record.ID != id {
		return Detail{}, ErrActivityUnavailable
	}
	_, zone, err := historyClock(ctx, service)
	if err != nil {
		return Detail{}, err
	}
	detail := Detail{Activity: record, Location: zone, Accounts: make(map[domain.AccountID]string)}
	eligible, err := service.RepeatEligibleActivities(ctx, []domain.Activity{record})
	if err != nil {
		return Detail{}, err
	}
	detail.Repeatable = eligible[id]
	detail.Links, err = loadAmendmentLinks(ctx, service, record)
	if err != nil {
		return Detail{}, err
	}
	var ids []domain.AccountID
	addAccount := func(id *domain.AccountID) {
		if id != nil {
			if _, seen := detail.Accounts[*id]; !seen {
				ids = append(ids, *id)
				detail.Accounts[*id] = ""
			}
		}
	}
	hasInstruments := record.TradeDetail != nil || record.DividendDetail != nil || record.ProductContext != nil
	for _, effect := range record.Effects {
		addAccount(effect.AccountID)
		hasInstruments = hasInstruments || effect.InstrumentID != nil
	}
	for _, result := range record.Resulting {
		addAccount(result.AccountID)
	}
	if len(ids) > 0 {
		accounts, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: ids, IncludeArchived: true})
		if err != nil {
			return Detail{}, err
		}
		for _, account := range accounts {
			detail.Accounts[account.Account.ID] = account.Account.Name
			if len(ids) == 1 && account.Account.IsLiability() {
				detail.LiabilityAccount = account.Account.ID
			}
		}
		if movement, eligible := CashAmendment(record); eligible && zone != nil && detail.Links.Reversal == "" {
			endpoints, ok := movement.accounts(accounts)
			detail.Amendable = ok
			detail.Reversible = ok && endpoints.active()
		}
	}
	if hasInstruments {
		instruments, err := service.ListInstruments(ctx, true)
		if err != nil {
			return Detail{}, err
		}
		detail.Instruments = make(map[domain.InstrumentID]string, len(instruments))
		detail.InstrumentCurrencies = make(map[domain.InstrumentID]domain.CurrencyCode, len(instruments))
		for _, instrument := range instruments {
			detail.Instruments[instrument.ID] = instrument.DisplayLabel()
			detail.InstrumentCurrencies[instrument.ID] = instrument.QuoteCurrency
		}
	}
	if record.Kind == domain.ActivityPositionTransfer && record.Reason == domain.ReasonOther && record.ProductContext == nil {
		if err := readCustodyAccounts(ctx, service, &detail); err != nil {
			return Detail{}, err
		}
	}
	return detail, nil
}

func readCustodyAccounts(ctx context.Context, service DetailReader, detail *Detail) error {
	detail.HoldingAccounts = make(map[domain.HoldingID]domain.AccountID)
	for _, effect := range detail.Activity.Effects {
		if effect.HoldingID != nil && (effect.Role == domain.EffectRoleTransferFrom || effect.Role == domain.EffectRoleTransferTo) {
			detail.HoldingAccounts[*effect.HoldingID] = ""
		}
	}
	if len(detail.HoldingAccounts) == 0 {
		return nil
	}
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return err
	}
	ids := make([]domain.AccountID, 0, len(accounts))
	for _, row := range accounts {
		if row.Account.TrackingMode == domain.TrackingHoldings {
			ids = append(ids, row.Account.ID)
		}
	}
	holdings, err := service.HoldingsByAccounts(ctx, ids)
	if err != nil {
		return err
	}
	for _, row := range accounts {
		for _, holding := range holdings[row.Account.ID] {
			if _, needed := detail.HoldingAccounts[holding.ID]; needed {
				detail.HoldingAccounts[holding.ID], detail.Accounts[row.Account.ID] = row.Account.ID, row.Account.Name
			}
		}
	}
	return nil
}
