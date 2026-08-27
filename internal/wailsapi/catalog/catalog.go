// Package catalog exposes the closed domain and settings vocabularies the
// frontend uses to populate selectors. Go remains the only list; this
// service copies those slices onto a Wails DTO.
package catalog

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// CatalogDTO is the complete closed-vocabulary payload. Maps are keyed by
// the parent enum's wire value so the frontend can drive dependent selects
// without duplicating combination rules.
type CatalogDTO struct {
	Currencies                   []string            `json:"currencies"`
	InstrumentTypes              []string            `json:"instrumentTypes"`
	QuoteSources                 []string            `json:"quoteSources"`
	PrimaryCategories            []string            `json:"primaryCategories"`
	SecondaryCategoriesByPrimary map[string][]string `json:"secondaryCategoriesByPrimary"`
	TrackingModesByPrimary       map[string][]string `json:"trackingModesByPrimary"`
	TrendRanges                  []string            `json:"trendRanges"`
	Appearances                  []string            `json:"appearances"`
	Languages                    []string            `json:"languages"`
	Accents                      []string            `json:"accents"`
	MoneyInReasons               []string            `json:"moneyInReasons"`
	MoneyOutReasons              []string            `json:"moneyOutReasons"`
	ValueUpdateReasons           []string            `json:"valueUpdateReasons"`
	TradeSides                   []string            `json:"tradeSides"`
}

func (s *Service) Catalog() CatalogDTO {
	return CatalogDTO{
		Currencies:                   settings.SupportedCurrencies(),
		InstrumentTypes:              stringSlice(domain.AllInstrumentTypes()),
		QuoteSources:                 stringSlice(domain.AllQuoteSourceKinds()),
		PrimaryCategories:            stringSlice(domain.AllPrimaryCategories()),
		SecondaryCategoriesByPrimary: stringMapSlice(domain.SecondaryCategoriesByPrimary()),
		TrackingModesByPrimary:       stringMapSlice(domain.TrackingModesByPrimary()),
		TrendRanges:                  stringSlice(domain.AllTrendRanges()),
		Appearances:                  stringSlice(settings.AllAppearances()),
		Languages:                    stringSlice(settings.AllLanguages()),
		Accents:                      stringSlice(settings.AllAccents()),
		MoneyInReasons:               stringSlice(domain.MoneyInReasons()),
		MoneyOutReasons:              stringSlice(domain.MoneyOutReasons()),
		ValueUpdateReasons:           stringSlice(domain.ValueUpdateReasons()),
		TradeSides:                   stringSlice(domain.AllTradeSides()),
	}
}

func stringSlice[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}

func stringMapSlice[K ~string, V ~string](values map[K][]V) map[string][]string {
	out := make(map[string][]string, len(values))
	for key, list := range values {
		out[string(key)] = stringSlice(list)
	}
	return out
}
