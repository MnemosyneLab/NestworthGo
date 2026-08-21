// Package settings contains the user-facing preferences for the v0.1.0 UI
// MVP. These preferences are deliberately kept outside the future financial
// database so the presentation layer can evolve without defining business
// persistence prematurely.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// Settings contains presentation preferences only. Currency is the primary
// display currency in this MVP; it does not change a future Household base
// currency or perform an FX conversion.
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
}

// Minimum and maximum window dimensions accepted from a persisted settings
// file. These bound the v0.1.1 window-state restoration (see
// docs/development/code-review-2026-08-21.md BUG-5/GAP-2) so a corrupted or
// hand-edited settings file can never restore a degenerate or absurd window.
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

func validCurrency(value string) bool {
	switch value {
	case "AUD", "CNY", "EUR", "GBP", "HKD", "JPY", "SGD", "TWD", "USD":
		return true
	default:
		return false
	}
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
	if err := loaded.Validate(); err != nil {
		return defaults, err
	}
	return loaded, nil
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
