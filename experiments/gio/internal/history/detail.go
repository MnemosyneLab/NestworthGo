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
	ReferenceReader
	Activity(context.Context, domain.ActivityID) (domain.Activity, error)
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
	Activity    domain.Activity
	Accounts    map[domain.AccountID]string
	Instruments map[domain.InstrumentID]string
	Location    *time.Location
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
	var ids []domain.AccountID
	addAccount := func(id *domain.AccountID) {
		if id != nil {
			if _, seen := detail.Accounts[*id]; !seen {
				ids = append(ids, *id)
				detail.Accounts[*id] = ""
			}
		}
	}
	hasInstruments := record.TradeDetail != nil || record.DividendDetail != nil
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
		}
	}
	if hasInstruments {
		instruments, err := service.ListInstruments(ctx, true)
		if err != nil {
			return Detail{}, err
		}
		detail.Instruments = make(map[domain.InstrumentID]string, len(instruments))
		for _, instrument := range instruments {
			detail.Instruments[instrument.ID] = instrument.DisplayLabel()
		}
	}
	return detail, nil
}
