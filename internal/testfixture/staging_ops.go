package testfixture

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture/staging"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

func stage(s *application.Service, ctx context.Context) (*staging.Stager, error) {
	entry, ok := registered.Load(s)
	if !ok {
		return nil, nil
	}
	reg := entry.(registration)
	if !storage.PendingPath(reg.repo.Path()) {
		return nil, nil
	}
	return staging.New(reg.repo, reg.now), nil
}

func CreateAccount(s *application.Service, ctx context.Context, input application.AccountInput) (domain.AccountRecord, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if fixture == nil {
		return s.CreateAccount(ctx, input)
	}
	return fixture.CreateAccount(ctx, staging.AccountInput(input))
}

func UpdateAccount(s *application.Service, ctx context.Context, id domain.AccountID, input application.AccountInput) (domain.AccountRecord, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if fixture == nil {
		return s.UpdateAccount(ctx, id, input)
	}
	return fixture.UpdateAccount(ctx, id, staging.AccountInput(input))
}

func ArchiveAccount(s *application.Service, ctx context.Context, id domain.AccountID, archived bool) error {
	fixture, err := stage(s, ctx)
	if err != nil {
		return err
	}
	if fixture == nil {
		return s.ArchiveAccount(ctx, id, archived)
	}
	return fixture.ArchiveAccount(ctx, id, archived)
}

func AppendAccountValue(s *application.Service, ctx context.Context, id domain.AccountID, amount, date string) (domain.AccountValue, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.AccountValue{}, err
	}
	if fixture == nil {
		return s.AppendAccountValue(ctx, id, amount, date)
	}
	return fixture.AppendAccountValue(ctx, id, amount, date)
}

func CreateInstrument(s *application.Service, ctx context.Context, input application.InstrumentInput) (domain.Instrument, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if fixture == nil {
		return s.CreateInstrument(ctx, input)
	}
	return fixture.CreateInstrument(ctx, staging.InstrumentInput(input))
}

func UpdateInstrument(s *application.Service, ctx context.Context, id domain.InstrumentID, input application.InstrumentInput) (domain.Instrument, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if fixture == nil {
		return s.UpdateInstrument(ctx, id, input)
	}
	return fixture.UpdateInstrument(ctx, id, staging.InstrumentInput(input))
}

func SetInstrumentQuoteSource(s *application.Service, ctx context.Context, id domain.InstrumentID, source string) error {
	fixture, err := stage(s, ctx)
	if err != nil {
		return err
	}
	if fixture == nil {
		return s.SetInstrumentQuoteSource(ctx, id, source)
	}
	return fixture.SetInstrumentQuoteSource(ctx, id, source)
}

func CreateHolding(s *application.Service, ctx context.Context, input application.HoldingInput) (domain.Holding, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	if fixture == nil {
		return s.CreateHolding(ctx, input)
	}
	return fixture.CreateHolding(ctx, staging.HoldingInput(input))
}

func UpdateHolding(s *application.Service, ctx context.Context, id domain.HoldingID, input application.HoldingUpdateInput) (domain.Holding, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	if fixture == nil {
		return s.UpdateHolding(ctx, id, input)
	}
	return fixture.UpdateHolding(ctx, id, staging.HoldingUpdateInput(input))
}

func AppendAccountCashValue(s *application.Service, ctx context.Context, id domain.AccountID, amount, currency, date string) (domain.AccountCashValue, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	if fixture == nil {
		return s.AppendAccountCashValue(ctx, id, amount, currency, date)
	}
	return fixture.AppendAccountCashValue(ctx, id, amount, currency, date)
}

func SetFXPreference(s *application.Service, ctx context.Context, a, b, source string) (domain.FXPreference, error) {
	fixture, err := stage(s, ctx)
	if err != nil {
		return domain.FXPreference{}, err
	}
	if fixture == nil {
		return s.SetFXPreference(ctx, a, b, source)
	}
	return fixture.SetFXPreference(ctx, a, b, source)
}
