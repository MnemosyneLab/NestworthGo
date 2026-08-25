// Package wailstest provides shared test fixtures for internal/wailsapi's
// own tests. It is not imported by any production internal/wailsapi
// service file; it exists only so each service's _test.go file can start
// from the same real, temp-file-backed application.Service that
// internal/application's own tests already use (service_test.go), instead
// of a hand-rolled fake for the ~50-method application.Repository
// interface.
package wailstest

import (
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// NewService opens a fresh temp-file SQLite database (removed automatically
// with the test's t.TempDir()) and returns an application.Service backed by
// the real repository implementation.
func NewService(t *testing.T, registries ...application.MarketDataRegistryPort) *application.Service {
	t.Helper()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := sqlite.NewRepository(database)
	return application.NewService(repository, registries...)
}
