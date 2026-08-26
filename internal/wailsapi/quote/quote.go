// Package quote adapts internal/application.Service's manual/current
// Instrument-quote and FX-quote/preference surface for the Wails IPC
// boundary. Provider-driven refresh lives in the marketdata service;
// this service only reads and manually appends quotes.
package quote

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

func (s *Service) CurrentInstrumentQuote(ctx context.Context, instrumentID string) (*wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	quote, err := s.app.CurrentInstrumentQuote(ctx, id)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuotePtr(quote), nil
}

func (s *Service) InstrumentQuoteHistory(ctx context.Context, instrumentID string) ([]wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	quotes, err := s.app.InstrumentQuoteHistory(ctx, id)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuotes(quotes), nil
}

func (s *Service) SaveManualInstrumentQuote(ctx context.Context, instrumentID, unitPrice, quotedAt string) (wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	quote, err := s.app.SaveManualInstrumentQuote(ctx, id, unitPrice, quotedAt)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuote(quote), nil
}

func (s *Service) AppendManualInstrumentQuote(ctx context.Context, instrumentID, unitPrice, quotedAt string, delayed bool) (wire.InstrumentQuoteDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	quote, err := s.app.AppendManualInstrumentQuote(ctx, id, unitPrice, quotedAt, delayed)
	if err != nil {
		return wire.InstrumentQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstrumentQuote(quote), nil
}

func (s *Service) CurrentFXQuote(ctx context.Context, currencyA, currencyB string) (*wire.FXQuoteDTO, error) {
	a, err := domain.ParseCurrency(currencyA)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	b, err := domain.ParseCurrency(currencyB)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	quote, err := s.app.CurrentFXQuote(ctx, a, b)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromFXQuotePtr(quote), nil
}

func (s *Service) FXQuoteHistory(ctx context.Context) ([]wire.FXQuoteDTO, error) {
	quotes, err := s.app.FXQuoteHistory(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromFXQuotes(quotes), nil
}

func (s *Service) SaveManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (wire.FXQuoteDTO, error) {
	quote, err := s.app.SaveManualFXQuote(ctx, baseCurrency, quoteCurrency, rate, quotedAt)
	if err != nil {
		return wire.FXQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromFXQuote(quote), nil
}

func (s *Service) AppendManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (wire.FXQuoteDTO, error) {
	quote, err := s.app.AppendManualFXQuote(ctx, baseCurrency, quoteCurrency, rate, quotedAt)
	if err != nil {
		return wire.FXQuoteDTO{}, apierror.Wrap(err)
	}
	return wire.FromFXQuote(quote), nil
}

func (s *Service) SetFXPreference(ctx context.Context, currencyA, currencyB, source string) (wire.FXPreferenceDTO, error) {
	preference, err := s.app.SetFXPreference(ctx, currencyA, currencyB, source)
	if err != nil {
		return wire.FXPreferenceDTO{}, apierror.Wrap(err)
	}
	return wire.FromFXPreference(preference), nil
}

func (s *Service) ListFXPreferences(ctx context.Context) ([]wire.FXPreferenceDTO, error) {
	preferences, err := s.app.ListFXPreferences(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromFXPreferences(preferences), nil
}
