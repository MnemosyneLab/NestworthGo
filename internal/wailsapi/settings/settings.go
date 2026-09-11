// Package settings adapts internal/settings.Store for the Wails IPC
// boundary. Unlike every other internal/wailsapi service, it is not a
// wrapper of internal/application.Service; it depends on
// internal/application only to delegate FX-provider changes.
package settings

import (
	"context"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Service struct {
	store *settings.Store
	app   *application.Service
}

// SettingsDTO is the Wails wire contract. The persisted settings.Settings
// struct intentionally keeps its snake_case JSON tags for the on-disk format;
// it must not leak into IPC or force the frontend to maintain two key shapes.
type SettingsDTO struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	Appearance        settings.Appearance `json:"appearance"`
	Accent            settings.Accent     `json:"accent"`
	Language          settings.Language   `json:"language"`
	Timezone          string              `json:"timezone"`
	WeekStart         string              `json:"weekStart"`
	DateFormat        string              `json:"dateFormat"`
	TimeFormat        string              `json:"timeFormat"`
	Currency          string              `json:"currency"`
	DecimalSeparator  string              `json:"decimalSeparator"`
	GroupingSeparator string              `json:"groupingSeparator"`
	DecimalPlaces     int                 `json:"decimalPlaces"`
	WindowWidth       float32             `json:"windowWidth"`
	WindowHeight      float32             `json:"windowHeight"`
	FXProvider        string              `json:"fxProvider"`
	QuoteCacheTTL     string              `json:"quoteCacheTTL"`
}

// TiingoKeyStatusDTO exposes only whether a locally configured key exists. The
// key itself never crosses the Wails boundary.
type TiingoKeyStatusDTO struct {
	Configured bool `json:"configured"`
}

func fromSettings(value settings.Settings) SettingsDTO {
	return SettingsDTO{
		SchemaVersion: value.SchemaVersion, Appearance: value.Appearance, Accent: value.Accent,
		Language: value.Language, Timezone: value.Timezone, WeekStart: value.WeekStart,
		DateFormat: value.DateFormat, TimeFormat: value.TimeFormat, Currency: value.Currency,
		DecimalSeparator: value.DecimalSeparator, GroupingSeparator: value.GroupingSeparator,
		DecimalPlaces: value.DecimalPlaces, WindowWidth: value.WindowWidth, WindowHeight: value.WindowHeight,
		FXProvider: value.FXProvider, QuoteCacheTTL: value.QuoteCacheTTL,
	}
}

func (value SettingsDTO) toSettings() settings.Settings {
	return settings.Settings{
		SchemaVersion: value.SchemaVersion, Appearance: value.Appearance, Accent: value.Accent,
		Language: value.Language, Timezone: value.Timezone, WeekStart: value.WeekStart,
		DateFormat: value.DateFormat, TimeFormat: value.TimeFormat, Currency: value.Currency,
		DecimalSeparator: value.DecimalSeparator, GroupingSeparator: value.GroupingSeparator,
		DecimalPlaces: value.DecimalPlaces, WindowWidth: value.WindowWidth, WindowHeight: value.WindowHeight,
		FXProvider: value.FXProvider, QuoteCacheTTL: value.QuoteCacheTTL,
	}
}

func NewService(store *settings.Store, app *application.Service) *Service {
	return &Service{store: store, app: app}
}

func (s *Service) TiingoKeyStatus() (TiingoKeyStatusDTO, error) {
	value, err := s.store.Load()
	if err != nil {
		return TiingoKeyStatusDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "Tiingo key status could not be read"})
	}
	return tiingoKeyStatus(value.TiingoAPIKey), nil
}

func (s *Service) SaveTiingoAPIKey(value string) (TiingoKeyStatusDTO, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TiingoKeyStatusDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Field: "tiingo_api_key", Message: "Tiingo API key is required"})
	}
	return s.updateTiingoAPIKey(value)
}

func (s *Service) DeleteTiingoAPIKey() (TiingoKeyStatusDTO, error) {
	return s.updateTiingoAPIKey("")
}

func tiingoKeyStatus(value string) TiingoKeyStatusDTO {
	return TiingoKeyStatusDTO{Configured: strings.TrimSpace(value) != ""}
}

func (s *Service) updateTiingoAPIKey(value string) (TiingoKeyStatusDTO, error) {
	persist := func() error {
		current, err := s.store.Load()
		if err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"}
		}
		current.TiingoAPIKey = value
		if err := s.store.Save(current); err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be saved"}
		}
		return nil
	}
	var err error
	if s.app == nil {
		err = persist()
	} else {
		err = s.app.WithWrite(context.Background(), func(context.Context) error { return persist() })
	}
	if err != nil {
		return TiingoKeyStatusDTO{}, apierror.Wrap(err)
	}
	return tiingoKeyStatus(value), nil
}

// Load returns the persisted preferences, or internal/settings' documented
// defaults if none are saved yet or the saved file could not be fully
// trusted (internal/settings.Store.Load already salvages individually
// invalid fields rather than discarding the whole file).
func (s *Service) Load() (SettingsDTO, error) {
	value, err := s.store.Load()
	if err != nil {
		return fromSettings(settings.Default()), apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"})
	}
	return fromSettings(value), nil
}

// Save validates the submitted preferences, applies an FX provider change
// through application.Service.SetFXProvider first (so a rejected provider
// choice never gets persisted), and only then writes the file — the same
// ordering used by the application service.

func (s *Service) Save(value SettingsDTO) error {
	persisted := value.toSettings()
	if err := persisted.Validate(); err != nil {
		return apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Message: err.Error()})
	}
	persist := func() error {
		current, err := s.store.Load()
		if err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"}
		}
		if s.app != nil && persisted.FXProvider != current.FXProvider {
			if err := s.app.SetFXProvider(persisted.FXProvider); err != nil {
				return err
			}
		}
		if s.app != nil {
			s.app.SetQuoteCacheTTL(persisted.QuoteCacheTTLDuration())
			s.app.SetUILanguage(string(persisted.Language))
		}
		// SettingsDTO deliberately omits the API key. General preference saves
		// must preserve the separately managed key instead of clearing it.
		persisted.TiingoAPIKey = current.TiingoAPIKey
		if err := s.store.Save(persisted); err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be saved"}
		}
		return nil
	}
	if s.app == nil {
		return apierror.Wrap(persist())
	}
	return apierror.Wrap(s.app.WithWrite(context.Background(), func(context.Context) error {
		return persist()
	}))
}

// Reset restores settings.Default(), applying the same FX-provider
// delegation performed by Save.
func (s *Service) Reset() (SettingsDTO, error) {
	defaults := settings.Default()
	if err := s.Save(fromSettings(defaults)); err != nil {
		return SettingsDTO{}, err
	}
	return fromSettings(defaults), nil
}

// SupportedCurrencies returns the closed currency catalog from domain.
func (s *Service) SupportedCurrencies() []string {
	return settings.SupportedCurrencies()
}

// FXProviders returns only registered providers that advertise latest-FX
// support. Settings can therefore render the same closed list the
// application routing layer can actually resolve.
func (s *Service) FXProviders() []string {
	if s.app == nil || s.app.MarketDataRegistry() == nil {
		return []string{settings.DefaultFXProvider}
	}
	providers := s.app.MarketDataRegistry().Providers()
	keys := make([]string, 0, len(providers))
	for _, provider := range providers {
		if provider == nil || !provider.Capabilities().LatestFX {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Key()))
		if key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return []string{settings.DefaultFXProvider}
	}
	sort.Strings(keys)
	return keys
}
