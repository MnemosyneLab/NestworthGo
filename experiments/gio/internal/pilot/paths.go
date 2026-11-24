package pilot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	AppID       = "com.nestworth.gio"
	DisplayName = "Nestworth Gio"
)

// Paths identifies the private synthetic ledger and its presentation settings.
type Paths struct {
	data, database, settings string
}

func (p Paths) RootPath() string     { return filepath.Dir(p.data) }
func (p Paths) SettingsPath() string { return p.settings }

func dataPaths(dir string) Paths {
	return Paths{dir, filepath.Join(dir, "nestworth.db"), filepath.Join(dir, "settings.json")}
}

// PathsFromEnv resolves the persistent Gio profile or an explicitly isolated run.
// Resolving paths performs no writes, including on first launch.
func PathsFromEnv() (Paths, error) {
	root := os.Getenv("NESTWORTH_GIO_RUN_DIR")
	database := os.Getenv("NESTWORTH_DATABASE_PATH")
	settings := os.Getenv("NESTWORTH_SETTINGS_PATH")
	if root == "" && database == "" && settings == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve Gio application directory: %w", err)
		}
		root = filepath.Join(base, AppID)
	}
	return resolvePaths(root, database, settings)
}

func resolvePaths(root, database, settings string) (Paths, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return Paths{}, errors.New("NESTWORTH_GIO_RUN_DIR must be the absolute path of the isolated preview run")
	}
	paths := dataPaths(filepath.Join(root, "data"))
	if (database != "" || settings != "") && (database != paths.database || settings != paths.settings) {
		return Paths{}, errors.New("explicit database and settings paths must match the isolated preview run")
	}
	if err := checkRoot(root); err != nil {
		return Paths{}, err
	}
	return paths, nil
}

// Check the nearest existing ancestor even when the profile has yet to be created.
func checkRoot(root string) error {
	for path := root; ; path = filepath.Dir(path) {
		_, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && path != filepath.Dir(path) {
			continue
		}
		if err != nil {
			return err
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || canonical != path {
			return errors.New("the preview directory must be at its canonical path")
		}
		if path == root {
			return requirePrivateDirectory(root)
		}
		return nil
	}
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || !isPrivateOwned(info) {
		return fmt.Errorf("the preview requires a private directory owned by the current user: %s", path)
	}
	return nil
}

func requireDisposableRun(paths Paths) error {
	if os.Getenv("NESTWORTH_GIO_RUN_DIR") != paths.RootPath() {
		return errors.New("use an explicit NESTWORTH_GIO_RUN_DIR for this disposable fixture")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if filepath.Join(configDir, AppID) == paths.RootPath() {
		return errors.New("use a disposable run directory outside the default Gio profile")
	}
	return nil
}

func isPrivateOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0o077 == 0
}

func verifyFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !isPrivateOwned(info) || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, errors.New("expected a private regular file without symbolic or hard links")
	}
	return info, nil
}
