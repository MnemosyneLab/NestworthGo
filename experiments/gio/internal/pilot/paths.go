package pilot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppID       = "com.nestworth.gio"
	DisplayName = "Nestworth Gio"
)

// Paths identifies the Gio ledger and its presentation settings.
type Paths struct {
	data, database, settings string
}

func (p Paths) RootPath() string {
	if filepath.Base(p.data) == "data" {
		return filepath.Dir(p.data)
	}
	return p.data
}

func (p Paths) SettingsPath() string { return p.settings }

func dataPaths(dir string) Paths {
	return Paths{data: dir, database: filepath.Join(dir, "nestworth.db"), settings: filepath.Join(dir, "settings.json")}
}

// PathsFromEnv uses the Wails database and settings overrides. Without an
// override, the corresponding file lives in Gio's own data directory.
func PathsFromEnv() (Paths, error) {
	database := os.Getenv("NESTWORTH_DATABASE_PATH")
	settings := strings.TrimSpace(os.Getenv("NESTWORTH_SETTINGS_PATH"))
	var paths Paths
	if database == "" || settings == "" {
		base, err := os.UserConfigDir()
		if err != nil && database == "" {
			return Paths{}, fmt.Errorf("resolve Gio application directory: %w", err)
		}
		if base == "" {
			if database == "" {
				return Paths{}, errors.New("the operating system returned an empty config directory")
			}
			base = os.TempDir()
		}
		paths = dataPaths(filepath.Join(base, AppID, "data"))
	}
	if database != "" {
		paths.data, paths.database = filepath.Dir(database), database
	}
	if settings != "" {
		paths.settings = settings
	}
	return paths, nil
}
