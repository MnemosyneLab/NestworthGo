// Package preferences adapts the shared Nestworth settings store to Gio.
package preferences

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/diagnostics"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type Values struct {
	Appearance        settings.Appearance
	Accent            settings.Accent
	Language          settings.Language
	Timezone          string
	DateFormat        string
	TimeFormat        string
	DecimalSeparator  string
	GroupingSeparator string
	FXProvider        string
	QuoteCacheTTL     string
	LogLevel          string

	// Read-only context. The household currency and history timezone are
	// independent of the display currency and presentation timezone.
	HouseholdCurrency   string
	HistoryTimezone     string
	WeekStart           string
	LogPath             string
	CoinGeckoConfigured bool
}

func fromSettings(s settings.Settings) Values {
	if s.LogLevel == "" {
		s.LogLevel = "off"
	}
	if s.QuoteCacheTTL == "" {
		s.QuoteCacheTTL = settings.QuoteCacheTTL12h
	}
	return Values{
		Appearance: s.Appearance, Accent: s.Accent, Language: s.Language,
		Timezone: s.Timezone, DateFormat: s.DateFormat, TimeFormat: s.TimeFormat,
		DecimalSeparator: s.DecimalSeparator, GroupingSeparator: s.GroupingSeparator,
		FXProvider: s.FXProvider, QuoteCacheTTL: s.QuoteCacheTTL,
		LogLevel: s.LogLevel, WeekStart: s.WeekStart,
		CoinGeckoConfigured: strings.TrimSpace(s.CoinGeckoAPIKey) != "",
	}
}

func Default() Values { return fromSettings(settings.Default()) }

type Service struct {
	store *settings.Store
	app   *application.Service
}

// Open attaches the same SQLite configuration used by the Wails desktop.
// Callers supply the open database and the corresponding settings path.
func Open(path string, db *sqlite.DB, app *application.Service) (*Service, error) {
	store := settings.NewStore(path)
	if err := store.Attach(sqlite.NewConfigurationRepository(db)); err != nil {
		return nil, err
	}
	return &Service{store: store, app: app}, nil
}

// ConfigureValuation applies the saved source selection and quote freshness
// policy before ledger reads. Overview still uses only persisted observations.
func (s *Service) ConfigureValuation() error {
	current, err := s.store.Load()
	if err != nil {
		return err
	}
	if err := s.setFXProvider(current.FXProvider); err != nil {
		slog.Warn("configured FX provider is not available; falling back to the default for this session")
		if err := s.setFXProvider(settings.DefaultFXProvider); err != nil {
			slog.Error("default FX provider rejected at startup")
		}
	}
	s.app.SetQuoteCacheTTL(current.QuoteCacheTTLDuration())
	return nil
}

func (s *Service) setFXProvider(key string) error {
	if s.app.MarketDataRegistry() == nil {
		return s.app.SetStoredFXProvider(key)
	}
	return s.app.SetFXProvider(key)
}

// OpenLog starts the shared bounded file logger in this profile. Later
// settings writes apply the level as part of the store's save and rollback.
func (s *Service) OpenLog() (*diagnostics.Log, error) {
	current, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	log := diagnostics.New(s.store.LogPath())
	s.store.ConfigureLogging = log.Configure
	return log, log.Configure(current.LogLevel)
}

// CoinGeckoKey is the provider's runtime credential source. The settings
// view only receives the configured flag.
func (s *Service) CoinGeckoKey() (string, error) {
	current, err := s.store.Load()
	return current.CoinGeckoAPIKey, err
}

func (s *Service) withLedgerContext(ctx context.Context, value settings.Settings) (Values, error) {
	bootstrap, err := s.app.Bootstrap(ctx)
	if err != nil {
		return Values{}, err
	}
	result := fromSettings(value)
	result.LogPath = s.store.LogPath()
	if bootstrap.Household == nil {
		return result, nil
	}
	origin, err := s.app.HistoryOrigin(ctx)
	if err != nil {
		return Values{}, err
	}
	result.HouseholdCurrency = bootstrap.Household.BaseCurrency.String()
	if origin != nil {
		result.HistoryTimezone = origin.Timezone
	}
	return result, nil
}

func (s *Service) Load(ctx context.Context) (Values, error) {
	current, err := s.store.Load()
	if err != nil {
		return Values{}, err
	}
	return s.withLedgerContext(ctx, current)
}

func applyChange[T comparable](field *T, before, after T) {
	if before != after {
		*field = after
	}
}

func (s *Service) Save(ctx context.Context, before, after Values) (Values, error) {
	return s.save(ctx, func(current settings.Settings) settings.Settings {
		return merge(current, before, after)
	})
}

func (s *Service) Reset(ctx context.Context) (Values, error) {
	return s.save(ctx, func(current settings.Settings) settings.Settings {
		defaults := settings.Default()
		defaults.CoinGeckoAPIKey = current.CoinGeckoAPIKey
		return defaults
	})
}

// SetCoinGeckoKey updates a credential independently of the preference draft.
// An empty value explicitly revokes it.
func (s *Service) SetCoinGeckoKey(ctx context.Context, value string) (Values, error) {
	key := strings.TrimSpace(value)
	if value != "" && key == "" {
		return Values{}, errors.New("CoinGecko key is required")
	}
	return s.save(ctx, func(current settings.Settings) settings.Settings {
		current.CoinGeckoAPIKey = key
		return current
	})
}

// Merge a draft delta into the latest authoritative settings. A stale page
// draft cannot overwrite an unrelated choice made through the sidebar.
func merge(current settings.Settings, before, after Values) settings.Settings {
	applyChange(&current.Appearance, before.Appearance, after.Appearance)
	applyChange(&current.Accent, before.Accent, after.Accent)
	applyChange(&current.Language, before.Language, after.Language)
	applyChange(&current.Timezone, before.Timezone, after.Timezone)
	applyChange(&current.DateFormat, before.DateFormat, after.DateFormat)
	applyChange(&current.TimeFormat, before.TimeFormat, after.TimeFormat)
	applyChange(&current.DecimalSeparator, before.DecimalSeparator, after.DecimalSeparator)
	applyChange(&current.GroupingSeparator, before.GroupingSeparator, after.GroupingSeparator)
	applyChange(&current.FXProvider, before.FXProvider, after.FXProvider)
	applyChange(&current.QuoteCacheTTL, before.QuoteCacheTTL, after.QuoteCacheTTL)
	applyChange(&current.LogLevel, before.LogLevel, after.LogLevel)
	return current
}

func (s *Service) save(ctx context.Context, edit func(settings.Settings) settings.Settings) (Values, error) {
	var result Values
	err := s.app.WithWrite(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := s.store.Load()
		if err != nil {
			return err
		}
		next := edit(current)
		if err := next.Validate(); err != nil {
			return err
		}
		result, err = s.withLedgerContext(ctx, next)
		if err != nil {
			return err
		}
		fxChanged := current.FXProvider != next.FXProvider
		if fxChanged {
			if err := s.setFXProvider(next.FXProvider); err != nil {
				return err
			}
		}
		if err := s.persist(current, next); err != nil {
			if fxChanged {
				err = errors.Join(err, s.setFXProvider(current.FXProvider))
			}
			return err
		}
		if current.QuoteCacheTTL != next.QuoteCacheTTL {
			s.app.SetQuoteCacheTTL(next.QuoteCacheTTLDuration())
		}
		if current.LogLevel != next.LogLevel {
			slog.Info("application file logging configured", "level", next.LogLevel)
		}
		return nil
	})
	if err != nil {
		return Values{}, err
	}
	return result, nil
}

func (s *Service) persist(current, next settings.Settings) error {
	if current == next {
		return nil
	}
	err := s.store.Save(next)
	if err != nil && (current.Timezone != next.Timezone || current.Currency != next.Currency ||
		current.FXProvider != next.FXProvider || current.QuoteCacheTTL != next.QuoteCacheTTL || current.CoinGeckoAPIKey != next.CoinGeckoAPIKey) {
		// The store writes SQLite before replacing the presentation file.
		// Restore the prior durable values under the same write permit.
		return errors.Join(err, s.store.Save(current))
	}
	return err
}
