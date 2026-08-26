// Package settings adapts internal/settings.Store for the Wails IPC
// boundary. Unlike every other internal/wailsapi service, it is not a
// wrapper of internal/application.Service; it depends on
// internal/application only to delegate FX-provider changes (technical
// design Sec6).
package settings

import (
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
// order Controller.updatePreference already uses.
func (s *Service) Save(value settings.Settings) error {
	if err := value.Validate(); err != nil {
		return apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Message: err.Error()})
	}
	current, err := s.store.Load()
	if err != nil {
		return apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be loaded"})
	}
	if s.app != nil && value.FXProvider != current.FXProvider {
		if err := s.app.SetFXProvider(value.FXProvider); err != nil {
			return apierror.Wrap(err)
		}
	}
	if err := s.store.Save(value); err != nil {
		return apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "settings could not be saved"})
	}
	return nil
}

// Reset restores settings.Default(), applying the same FX-provider
// delegation Save does, mirroring Controller.resetPreference.
func (s *Service) Reset() (settings.Settings, error) {
	defaults := settings.Default()
	if err := s.Save(defaults); err != nil {
		return settings.Settings{}, err
	}
	return defaults, nil
}

// SupportedCurrencies returns the fixed list of display currencies the
// Settings page's currency selector may offer.
func (s *Service) SupportedCurrencies() []string {
	return settings.SupportedCurrencies()
}
