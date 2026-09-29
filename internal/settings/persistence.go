package settings

import (
	"encoding/json"
	"errors"
	"log/slog"
)

// ConfigurationRepository stores private application state in the household DB.
// A missing key is distinct from a saved empty credential (explicit revocation).
type ConfigurationRepository interface {
	LoadConfiguration(key string) ([]byte, bool, error)
	SaveConfiguration(key string, value []byte) error
	ListConfiguration(prefix string) (map[string][]byte, error)
	CoreSettings() (currency, timezone string, err error)
}

const durableSettingsKey = "settings"

type durableSettings struct {
	Currency        string `json:"currency"`
	Timezone        string `json:"timezone"`
	FXProvider      string `json:"fx_provider"`
	QuoteCacheTTL   string `json:"quote_cache_ttl"`
	CoinGeckoAPIKey string `json:"coingecko_api_key"`
	TiingoAPIKey    string `json:"tiingo_api_key"`
}

func durable(value Settings) durableSettings {
	return durableSettings{value.Currency, value.Timezone, value.FXProvider, value.QuoteCacheTTL, value.CoinGeckoAPIKey, value.TiingoAPIKey}
}
func (d durableSettings) apply(value *Settings) {
	value.Currency = d.Currency
	value.Timezone = d.Timezone
	value.FXProvider = d.FXProvider
	value.QuoteCacheTTL = d.QuoteCacheTTL
	value.CoinGeckoAPIKey = d.CoinGeckoAPIKey
	value.TiingoAPIKey = d.TiingoAPIKey
}

// PresentationJSON is deliberately an allowlist. Adding a new secret to Settings
// cannot accidentally include it in the disposable preferences file.
func PresentationJSON(value Settings) ([]byte, error) {
	return json.MarshalIndent(struct {
		SchemaVersion     int        `json:"schema_version"`
		LogLevel          string     `json:"log_level,omitempty"`
		Appearance        Appearance `json:"appearance"`
		Accent            Accent     `json:"accent"`
		Language          Language   `json:"language"`
		WeekStart         string     `json:"week_start"`
		DateFormat        string     `json:"date_format"`
		TimeFormat        string     `json:"time_format"`
		DecimalSeparator  string     `json:"decimal_separator"`
		GroupingSeparator string     `json:"grouping_separator"`
		DecimalPlaces     int        `json:"decimal_places"`
		WindowWidth       float32    `json:"window_width"`
		WindowHeight      float32    `json:"window_height"`
	}{value.SchemaVersion, value.LogLevel, value.Appearance, value.Accent, value.Language, value.WeekStart, value.DateFormat, value.TimeFormat, value.DecimalSeparator, value.GroupingSeparator, value.DecimalPlaces, value.WindowWidth, value.WindowHeight}, "", "  ")
}

// Attach imports legacy JSON once. Commit the durable copy before removing any
// fields from JSON; subsequent launches always prefer SQLite, even after a crash.
func (s *Store) Attach(repository ConfigurationRepository) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	value, status, fileErr := s.loadFile()
	raw, found, err := repository.LoadConfiguration(durableSettingsKey)
	if err != nil {
		return err
	}
	if found {
		var d durableSettings
		if json.Unmarshal(raw, &d) != nil {
			return errors.New("stored application settings are invalid")
		}
		d.apply(&value)
	} else {
		if fileErr != nil {
			return fileErr
		}
		if status == LoadStatusRecovered {
			return errors.New("legacy settings require recovery before migration")
		}
		if status == LoadStatusMissing {
			currency, timezone, err := repository.CoreSettings()
			if err != nil {
				return err
			}
			if currency != "" {
				value.Currency = currency
			}
			if timezone != "" {
				value.Timezone = timezone
			}
		}
		if err = value.Validate(); err != nil {
			return errors.New("application settings are invalid")
		}
		raw, err = json.Marshal(durable(value))
		if err != nil {
			return err
		}
		if err = repository.SaveConfiguration(durableSettingsKey, raw); err != nil {
			return err
		}
	}
	if err = value.Validate(); err != nil {
		return errors.New("stored application settings are invalid")
	}
	s.repository = repository
	// Losing optional presentation preferences must not make durable data unusable.
	if err = s.writeFile(value); err != nil {
		slog.Warn("could not rebuild presentation preferences")
	}
	return nil
}
