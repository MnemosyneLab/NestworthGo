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

func NewService(store *settings.Store, app *application.Service) *Service {
	return &Service{store: store, app: app}
}

// Load returns the persisted preferences, or internal/settings' documented
// defaults if none are saved yet or the saved file could not be fully
// trusted (internal/settings.Store.Load already salvages individually
// invalid fields rather than discarding the whole file).
func (s *Service) Load() (settings.Settings, error) {
	value, err := s.store.Load()
	if err != nil {
		return settings.Default(), apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"})
	}
	return value, nil
}

// Save validates the submitted preferences, applies an FX provider change
// through application.Service.SetFXProvider first (so a rejected provider
// choice never gets persisted), and only then writes the file — the same
// same ordering used by the application service.
func (s *Service) Save(value settings.Settings) error {
	if err := value.Validate(); err != nil {
		return apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Message: err.Error()})
	}
	persist := func() error {
		current, err := s.store.Load()
		if err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"}
		}
		if s.app != nil && value.FXProvider != current.FXProvider {
			if err := s.app.SetFXProvider(value.FXProvider); err != nil {
				return err
			}
		}
		if s.app != nil {
			s.app.SetQuoteCacheTTL(value.QuoteCacheTTLDuration())
			s.app.SetUILanguage(string(value.Language))
		}
		if err := s.store.Save(value); err != nil {
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
func (s *Service) Reset() (settings.Settings, error) {
	defaults := settings.Default()
	if err := s.Save(defaults); err != nil {
		return settings.Settings{}, err
	}
	return defaults, nil
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
