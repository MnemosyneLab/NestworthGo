// Package settings contains user-facing preferences. These preferences are
// deliberately kept outside the financial database so the presentation layer
// can evolve without changing business persistence.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const CurrentSchemaVersion = 1

type Appearance string

const (
	AppearanceSystem = Appearance("system")
	AppearanceLight  = Appearance("light")
	AppearanceDark   = Appearance("dark")
)

type Accent string

const (
	AccentNestworth = Accent("nestworth")
	AccentOcean     = Accent("ocean")
	AccentForest    = Accent("forest")
	AccentAmber     = Accent("amber")
	AccentRose      = Accent("rose")
)

type Language string

const (
	LanguageSystem  = Language("system")
	LanguageEnglish = Language("en")
	LanguageZhCN    = Language("zh-CN")
	LanguageZhTW    = Language("zh-TW")
)

const (
	FXProviderFrankfurter = "frankfurter"
	DefaultFXProvider     = FXProviderFrankfurter
)

const (
	TimezoneSystem = "system"

	WeekStartMonday = "monday"
	WeekStartSunday = "sunday"

	DateFormatISO        = "iso"
	DateFormatDayFirst   = "day-first"
	DateFormatMonthFirst = "month-first"
	DateFormatLocalized  = "localized"

	TimeFormat24 = "24h"
	TimeFormat12 = "12h"

	DecimalDot   = "."
	DecimalComma = ","

	GroupingComma = ","
	GroupingDot   = "."
	GroupingSpace = "space"
	GroupingApost = "'"
	GroupingNone  = "none"
)

// Settings contains presentation preferences and explicit market-data
// routing choices. Currency is the display currency; FXProvider selects the
// provider used only for user-initiated FX refresh.
type Settings struct {
	SchemaVersion     int        `json:"schema_version"`
	Appearance        Appearance `json:"appearance"`
	Accent            Accent     `json:"accent"`
	Language          Language   `json:"language"`
	Timezone          string     `json:"timezone"`
	WeekStart         string     `json:"week_start"`
	DateFormat        string     `json:"date_format"`
	TimeFormat        string     `json:"time_format"`
	Currency          string     `json:"currency"`
	DecimalSeparator  string     `json:"decimal_separator"`
	GroupingSeparator string     `json:"grouping_separator"`
	DecimalPlaces     int        `json:"decimal_places"`
	WindowWidth       float32    `json:"window_width"`
	WindowHeight      float32    `json:"window_height"`
	FXProvider        string     `json:"fx_provider"`
}

// Minimum and maximum window dimensions accepted from a persisted settings
// file. A corrupted or hand-edited settings file can never restore a
// degenerate or absurd window.
const (
	MinWindowWidth  = 640
	MinWindowHeight = 480
	MaxWindowWidth  = 10000
	MaxWindowHeight = 10000

	DefaultWindowWidth  = 1100
	DefaultWindowHeight = 720
)

func Default() Settings {
	return Settings{
		SchemaVersion:     CurrentSchemaVersion,
		Appearance:        AppearanceSystem,
		Accent:            AccentNestworth,
		Language:          LanguageSystem,
		Timezone:          TimezoneSystem,
		WeekStart:         WeekStartMonday,
		DateFormat:        DateFormatISO,
		TimeFormat:        TimeFormat24,
		Currency:          "CNY",
		DecimalSeparator:  DecimalDot,
		GroupingSeparator: GroupingComma,
		DecimalPlaces:     2,
		WindowWidth:       DefaultWindowWidth,
		WindowHeight:      DefaultWindowHeight,
		FXProvider:        DefaultFXProvider,
	}
}

func (s Settings) Validate() error {
	if s.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported settings schema version %d", s.SchemaVersion)
	}
	if !oneOf(string(s.Appearance), string(AppearanceSystem), string(AppearanceLight), string(AppearanceDark)) {
		return fmt.Errorf("unsupported appearance %q", s.Appearance)
	}
	if !oneOf(string(s.Accent), string(AccentNestworth), string(AccentOcean), string(AccentForest), string(AccentAmber), string(AccentRose)) {
		return fmt.Errorf("unsupported accent %q", s.Accent)
	}
	if !oneOf(string(s.Language), string(LanguageSystem), string(LanguageEnglish), string(LanguageZhCN), string(LanguageZhTW)) {
		return fmt.Errorf("unsupported language %q", s.Language)
	}
	if strings.TrimSpace(s.Timezone) == "" {
		return errors.New("timezone cannot be empty")
	}
	if s.Timezone != TimezoneSystem {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("unsupported timezone %q: %w", s.Timezone, err)
		}
	}
	if !oneOf(s.WeekStart, WeekStartMonday, WeekStartSunday) {
		return fmt.Errorf("unsupported week start %q", s.WeekStart)
	}
	if !oneOf(s.DateFormat, DateFormatISO, DateFormatDayFirst, DateFormatMonthFirst, DateFormatLocalized) {
		return fmt.Errorf("unsupported date format %q", s.DateFormat)
	}
	if !oneOf(s.TimeFormat, TimeFormat24, TimeFormat12) {
		return fmt.Errorf("unsupported time format %q", s.TimeFormat)
	}
	if !validCurrency(s.Currency) {
		return fmt.Errorf("unsupported currency %q", s.Currency)
	}
	if !oneOf(s.DecimalSeparator, DecimalDot, DecimalComma) {
		return fmt.Errorf("unsupported decimal separator %q", s.DecimalSeparator)
	}
	if !oneOf(s.GroupingSeparator, GroupingComma, GroupingDot, GroupingSpace, GroupingApost, GroupingNone) {
		return fmt.Errorf("unsupported grouping separator %q", s.GroupingSeparator)
	}
	if s.GroupingSeparator != GroupingNone && separatorValue(s.GroupingSeparator) == s.DecimalSeparator {
		return errors.New("decimal and grouping separators must differ")
	}
	if s.DecimalPlaces != 0 && s.DecimalPlaces != 2 && s.DecimalPlaces != 4 {
		return fmt.Errorf("decimal places must be one of 0, 2, or 4, got %d", s.DecimalPlaces)
	}
	if s.WindowWidth < MinWindowWidth || s.WindowWidth > MaxWindowWidth {
		return fmt.Errorf("window width must be between %d and %d, got %v", MinWindowWidth, MaxWindowWidth, s.WindowWidth)
	}
	if s.WindowHeight < MinWindowHeight || s.WindowHeight > MaxWindowHeight {
		return fmt.Errorf("window height must be between %d and %d, got %v", MinWindowHeight, MaxWindowHeight, s.WindowHeight)
	}
	if strings.TrimSpace(s.FXProvider) != "" && strings.TrimSpace(s.FXProvider) != s.FXProvider {
		return errors.New("FX provider cannot have leading or trailing whitespace")
	}
	if s.FXProvider != FXProviderFrankfurter {
		return fmt.Errorf("unsupported FX provider %q", s.FXProvider)
	}
	return nil
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

// SupportedCurrencies is the single source of truth for the currencies the
// app accepts as a display currency. Validation here and the settings-page
// currency selector must both derive from this list; display symbols live in
// format.CurrencySymbol.
func SupportedCurrencies() []string {
	codes := domain.SupportedCurrencies()
	values := make([]string, len(codes))
	for i, code := range codes {
		values[i] = code.String()
	}
	return values
}

func AllAppearances() []Appearance {
	return []Appearance{AppearanceSystem, AppearanceLight, AppearanceDark}
}

func AllLanguages() []Language {
	return []Language{LanguageSystem, LanguageEnglish, LanguageZhCN, LanguageZhTW}
}

func AllAccents() []Accent {
	return []Accent{AccentNestworth, AccentOcean, AccentForest, AccentAmber, AccentRose}
}

func validCurrency(value string) bool {
	return slices.Contains(SupportedCurrencies(), value)
}

func separatorValue(value string) string {
	if value == GroupingSpace {
		return " "
	}
	return value
}

type Store struct {
	Path string
}

func NewStore(path string) *Store {
	return &Store{Path: path}
}

func DefaultStore() *Store {
	if explicitPath := strings.TrimSpace(os.Getenv("NESTWORTH_SETTINGS_PATH")); explicitPath != "" {
		return NewStore(explicitPath)
	}
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = os.TempDir()
	}
	return NewStore(filepath.Join(configDir, "Nestworth", "settings.json"))
}

func (s *Store) Load() (Settings, error) {
	defaults := Default()
	if s == nil || s.Path == "" {
		return defaults, errors.New("settings path is empty")
	}

	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}

	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return defaults, fmt.Errorf("decode settings: %w", err)
	}
	if loaded.SchemaVersion != CurrentSchemaVersion {
		// A different schema version changes what the fields mean, so no
		// individual field can be trusted; fall back to full defaults.
		return defaults, fmt.Errorf("unsupported settings schema version %d", loaded.SchemaVersion)
	}
	repaired := salvage(loaded, defaults)
	if err := repaired.Validate(); err != nil {
		slog.Error("settings salvage still produced invalid settings; using defaults", "path", s.Path, "error", err)
		return defaults, nil
	}
	if repaired != loaded {
		slog.Warn("settings file contained invalid values; reset individual fields to defaults",
			"path", s.Path,
			"reset", strings.Join(changedFieldNames(loaded, repaired), ", "))
	}
	return repaired, nil
}

// salvage keeps every field that still validates and resets only the
// individually invalid ones to their defaults, so one hand-edited value can
// no longer discard the whole preference set.
func salvage(loaded, defaults Settings) Settings {
	fixed := loaded
	fixed.Appearance = salvageValue(fixed.Appearance, defaults.Appearance, func(v Appearance) bool {
		return oneOf(string(v), string(AppearanceSystem), string(AppearanceLight), string(AppearanceDark))
	})
	fixed.Accent = salvageValue(fixed.Accent, defaults.Accent, func(v Accent) bool {
		return oneOf(string(v), string(AccentNestworth), string(AccentOcean), string(AccentForest), string(AccentAmber), string(AccentRose))
	})
	fixed.Language = salvageValue(fixed.Language, defaults.Language, func(v Language) bool {
		return oneOf(string(v), string(LanguageSystem), string(LanguageEnglish), string(LanguageZhCN), string(LanguageZhTW))
	})
	fixed.Timezone = salvageValue(fixed.Timezone, defaults.Timezone, func(v string) bool {
		return strings.TrimSpace(v) != "" && (v == TimezoneSystem || func() bool { _, err := time.LoadLocation(v); return err == nil }())
	})
	fixed.WeekStart = salvageValue(fixed.WeekStart, defaults.WeekStart, func(v string) bool {
		return oneOf(v, WeekStartMonday, WeekStartSunday)
	})
	fixed.DateFormat = salvageValue(fixed.DateFormat, defaults.DateFormat, func(v string) bool {
		return oneOf(v, DateFormatISO, DateFormatDayFirst, DateFormatMonthFirst, DateFormatLocalized)
	})
	fixed.TimeFormat = salvageValue(fixed.TimeFormat, defaults.TimeFormat, func(v string) bool {
		return oneOf(v, TimeFormat24, TimeFormat12)
	})
	fixed.Currency = salvageValue(fixed.Currency, defaults.Currency, validCurrency)
	fixed.DecimalSeparator = salvageValue(fixed.DecimalSeparator, defaults.DecimalSeparator, func(v string) bool {
		return oneOf(v, DecimalDot, DecimalComma)
	})
	fixed.GroupingSeparator = salvageValue(fixed.GroupingSeparator, defaults.GroupingSeparator, func(v string) bool {
		return oneOf(v, GroupingComma, GroupingDot, GroupingSpace, GroupingApost, GroupingNone)
	})
	if fixed.GroupingSeparator != GroupingNone && separatorValue(fixed.GroupingSeparator) == fixed.DecimalSeparator {
		if fixed.DecimalSeparator == DecimalComma {
			fixed.GroupingSeparator = GroupingDot
		} else {
			fixed.GroupingSeparator = GroupingComma
		}
	}
	fixed.DecimalPlaces = salvageValue(fixed.DecimalPlaces, defaults.DecimalPlaces, func(v int) bool {
		return v == 0 || v == 2 || v == 4
	})
	fixed.WindowWidth = salvageValue(fixed.WindowWidth, defaults.WindowWidth, func(v float32) bool {
		return v >= MinWindowWidth && v <= MaxWindowWidth
	})
	fixed.WindowHeight = salvageValue(fixed.WindowHeight, defaults.WindowHeight, func(v float32) bool {
		return v >= MinWindowHeight && v <= MaxWindowHeight
	})
	fixed.FXProvider = salvageValue(strings.TrimSpace(fixed.FXProvider), defaults.FXProvider, func(v string) bool {
		return v == FXProviderFrankfurter
	})
	return fixed
}

func salvageValue[T comparable](value, fallback T, valid func(T) bool) T {
	if valid(value) {
		return value
	}
	return fallback
}

func changedFieldNames(from, to Settings) []string {
	fields := []struct {
		name     string
		from, to any
	}{
		{"appearance", from.Appearance, to.Appearance},
		{"accent", from.Accent, to.Accent},
		{"language", from.Language, to.Language},
		{"timezone", from.Timezone, to.Timezone},
		{"week_start", from.WeekStart, to.WeekStart},
		{"date_format", from.DateFormat, to.DateFormat},
		{"time_format", from.TimeFormat, to.TimeFormat},
		{"currency", from.Currency, to.Currency},
		{"decimal_separator", from.DecimalSeparator, to.DecimalSeparator},
		{"grouping_separator", from.GroupingSeparator, to.GroupingSeparator},
		{"decimal_places", from.DecimalPlaces, to.DecimalPlaces},
		{"window_width", from.WindowWidth, to.WindowWidth},
		{"window_height", from.WindowHeight, to.WindowHeight},
		{"fx_provider", from.FXProvider, to.FXProvider},
	}
	var changed []string
	for _, field := range fields {
		if field.from != field.to {
			changed = append(changed, field.name)
		}
	}
	return changed
}

func (s *Store) Save(value Settings) error {
	if s == nil || s.Path == "" {
		return errors.New("settings path is empty")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, s.Path)
}
