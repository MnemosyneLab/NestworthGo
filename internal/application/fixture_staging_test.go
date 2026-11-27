package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture/staging"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

// fixtureStage is restricted to staging synthetic data before capturing the
// fixture origin. It never participates in production code. Reads after
// fixture capture and all ordinary mutations go through normal Service.
func (s *Service) fixtureStage(ctx context.Context) (*staging.Stager, func(), error) {
	if !storage.PendingPath(s.repository.Path()) {
		return nil, nil, nil
	}
	_, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !storage.PendingPath(s.repository.Path()) {
		unlock()
		return nil, nil, nil
	}
	return staging.New(s.repository, s.clock), unlock, nil
}

func (s *Service) fixtureCreateAccount(ctx context.Context, input AccountInput) (domain.AccountRecord, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if stage == nil {
		return s.CreateAccount(ctx, input)
	}
	defer unlock()
	v, err := stage.CreateAccount(ctx, staging.AccountInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureUpdateAccount(ctx context.Context, id domain.AccountID, input AccountInput) (domain.AccountRecord, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if stage == nil {
		return s.UpdateAccount(ctx, id, input)
	}
	defer unlock()
	v, err := stage.UpdateAccount(ctx, id, staging.AccountInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return err
	}
	if stage == nil {
		return s.ArchiveAccount(ctx, id, archived)
	}
	defer unlock()
	err = stage.ArchiveAccount(ctx, id, archived)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) fixtureAppendAccountValue(ctx context.Context, id domain.AccountID, amount, date string) (domain.AccountValue, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.AccountValue{}, err
	}
	if stage == nil {
		return s.AppendAccountValue(ctx, id, amount, date)
	}
	defer unlock()
	v, err := stage.AppendAccountValue(ctx, id, amount, date)
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureCreateInstrument(ctx context.Context, input InstrumentInput) (domain.Instrument, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if stage == nil {
		return s.CreateInstrument(ctx, input)
	}
	defer unlock()
	v, err := stage.CreateInstrument(ctx, staging.InstrumentInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureUpdateInstrument(ctx context.Context, id domain.InstrumentID, input InstrumentInput) (domain.Instrument, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if stage == nil {
		return s.UpdateInstrument(ctx, id, input)
	}
	defer unlock()
	v, err := stage.UpdateInstrument(ctx, id, staging.InstrumentInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureSetInstrumentQuoteSource(ctx context.Context, id domain.InstrumentID, source string) error {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return err
	}
	if stage == nil {
		return s.SetInstrumentQuoteSource(ctx, id, source)
	}
	defer unlock()
	err = stage.SetInstrumentQuoteSource(ctx, id, source)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) fixtureCreateHolding(ctx context.Context, input HoldingInput) (domain.Holding, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	if stage == nil {
		return s.CreateHolding(ctx, input)
	}
	defer unlock()
	v, err := stage.CreateHolding(ctx, staging.HoldingInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureUpdateHolding(ctx context.Context, id domain.HoldingID, input HoldingUpdateInput) (domain.Holding, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	if stage == nil {
		return s.UpdateHolding(ctx, id, input)
	}
	defer unlock()
	v, err := stage.UpdateHolding(ctx, id, staging.HoldingUpdateInput(input))
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureAppendAccountCashValue(ctx context.Context, id domain.AccountID, amount, currency, date string) (domain.AccountCashValue, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	if stage == nil {
		return s.AppendAccountCashValue(ctx, id, amount, currency, date)
	}
	defer unlock()
	v, err := stage.AppendAccountCashValue(ctx, id, amount, currency, date)
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}

func (s *Service) fixtureSetFXPreference(ctx context.Context, a, b, source string) (domain.FXPreference, error) {
	stage, unlock, err := s.fixtureStage(ctx)
	if err != nil {
		return domain.FXPreference{}, err
	}
	if stage == nil {
		return s.SetFXPreference(ctx, a, b, source)
	}
	defer unlock()
	v, err := stage.SetFXPreference(ctx, a, b, source)
	if err == nil {
		s.invalidateAnalysis()
	}
	return v, err
}
