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
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/testfixture"
)

// NewService opens a fresh temp-file SQLite database (removed automatically
// with the test's t.TempDir()) and returns an application.Service backed by
// the real repository implementation.
func NewService(t *testing.T, registries ...application.MarketDataRegistryPort) *application.Service {
	return newService(t, false, registries...)
}

// NewBaselineService stages synthetic current facts for tests of existing
// immutable history origins; use NewService for actual onboarding tests.
func NewBaselineService(t *testing.T, registries ...application.MarketDataRegistryPort) *application.Service {
	return newService(t, true, registries...)
}

func newService(t *testing.T, baseline bool, registries ...application.MarketDataRegistryPort) *application.Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nestworth.db")
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := testfixture.Repository(database)
	var port application.Repository = repository
	if baseline {
		port = &testfixture.BaselineRepository{Repository: repository}
	}
	service := testfixture.NewService(port, registries...)
	appports.Wire(service)
	appports.AttachSQLiteHistory(service, repository)
	service.SetLiveDatabasePath(path)
	return service
}
