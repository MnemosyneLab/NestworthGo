// Package preferences adapts the shared Nestworth settings store to Gio's
// General settings and sidebar. Only editable General fields can be saved.
package preferences

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/internal/application"
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

	// Read-only context. The household currency and history timezone are
	// independent of the display currency and presentation timezone.
	HouseholdCurrency string
	HistoryTimezone   string
	WeekStart         string
}

func fromSettings(s settings.Settings) Values {
	return Values{
		Appearance: s.Appearance, Accent: s.Accent, Language: s.Language,
		Timezone: s.Timezone, DateFormat: s.DateFormat, TimeFormat: s.TimeFormat,
		DecimalSeparator: s.DecimalSeparator, GroupingSeparator: s.GroupingSeparator,
		WeekStart: s.WeekStart,
	}
}

func Default() Values { return fromSettings(settings.Default()) }

type Service struct {
	store *settings.Store
	app   *application.Service
}

// Open attaches the same SQLite configuration used by the Wails desktop.
// Callers supply the already verified pilot database and its settings path.
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
	if err := s.app.SetStoredFXProvider(current.FXProvider); err != nil {
		return err
	}
	s.app.SetQuoteCacheTTL(current.QuoteCacheTTLDuration())
	return nil
}

func (s *Service) withLedgerContext(ctx context.Context, value settings.Settings) (Values, error) {
	bootstrap, err := s.app.Bootstrap(ctx)
	if err != nil {
		return Values{}, err
	}
	result := fromSettings(value)
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
		return merge(current, fromSettings(current), Default())
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
		return s.persist(current, next)
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
	if err != nil && current.Timezone != next.Timezone {
		// Timezone is the only General field stored in SQLite. The shared store
		// writes it before atomically replacing the presentation file. Restore
		// the prior settings under the same write permit if publication failed.
		return errors.Join(err, s.store.Save(current))
	}
	return err
}
