// Package instrument adapts internal/application.Service's Instrument
// identity CRUD, icon, and quote-source surface for the Wails IPC boundary.
package instrument

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

// InstrumentRequest mirrors application.InstrumentInput. Replace=true means
// "this is the complete form state" (matches InstrumentInput.Replace); when
// false and used for an update, zero-valued fields keep the current value.
type InstrumentRequest struct {
	Replace        bool    `json:"replace,omitempty"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	QuoteCurrency  string  `json:"quoteCurrency"`
	Symbol         *string `json:"symbol,omitempty"`
	MarketCode     *string `json:"marketCode,omitempty"`
	CountryCode    *string `json:"countryCode,omitempty"`
	ISIN           *string `json:"isin,omitempty"`
	Note           *string `json:"note,omitempty"`
	IconKey        *string `json:"iconKey,omitempty"`
	SortOrder      int     `json:"sortOrder,omitempty"`
	QuoteSource    string  `json:"quoteSource,omitempty"`
	ProviderKey    *string `json:"providerKey,omitempty"`
	ProviderSymbol *string `json:"providerSymbol,omitempty"`
}

func (r InstrumentRequest) toApplicationInput() application.InstrumentInput {
	return application.InstrumentInput{
		Replace: r.Replace, Name: r.Name, Type: r.Type, QuoteCurrency: r.QuoteCurrency,
		Symbol: wire.StringFromPtr(r.Symbol), MarketCode: wire.StringFromPtr(r.MarketCode),
		CountryCode: wire.StringFromPtr(r.CountryCode), ISIN: wire.StringFromPtr(r.ISIN),
		Note: r.Note, IconKey: wire.StringFromPtr(r.IconKey), SortOrder: r.SortOrder,
		QuoteSource: r.QuoteSource, ProviderKey: wire.StringFromPtr(r.ProviderKey), ProviderSymbol: wire.StringFromPtr(r.ProviderSymbol),
	}
}

func (s *Service) CreateInstrument(ctx context.Context, request InstrumentRequest) (wire.InstrumentDTO, error) {
	instrument, err := s.app.CreateInstrument(ctx, request.toApplicationInput())
	if err != nil {
		return wire.InstrumentDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstrument(instrument), nil
}

func (s *Service) UpdateInstrument(ctx context.Context, id string, request InstrumentRequest) (wire.InstrumentDTO, error) {
	instrumentID, err := domain.ParseInstrumentID(id)
	if err != nil {
		return wire.InstrumentDTO{}, apierror.Wrap(err)
	}
	instrument, err := s.app.UpdateInstrument(ctx, instrumentID, request.toApplicationInput())
	if err != nil {
		return wire.InstrumentDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstrument(instrument), nil
}

func (s *Service) ListInstruments(ctx context.Context, includeArchived bool) ([]wire.InstrumentDTO, error) {
	instruments, err := s.app.ListInstruments(ctx, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstruments(instruments), nil
}

func (s *Service) ArchiveInstrument(ctx context.Context, id string, archived bool) error {
	instrumentID, err := domain.ParseInstrumentID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveInstrument(ctx, instrumentID, archived))
}

func (s *Service) SetInstrumentIcon(ctx context.Context, id, iconKey string) error {
	instrumentID, err := domain.ParseInstrumentID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetInstrumentIcon(ctx, instrumentID, iconKey))
}

func (s *Service) SetInstrumentQuoteSource(ctx context.Context, id, source string) error {
	instrumentID, err := domain.ParseInstrumentID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetInstrumentQuoteSource(ctx, instrumentID, source))
}
