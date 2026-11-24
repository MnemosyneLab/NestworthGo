package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func testPilotPaths(t *testing.T) (string, pilotPaths) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(root, "data", "nestworth.db")
	settings := filepath.Join(root, "data", "settings.json")
	paths, err := resolvePilotPaths(root, database, settings)
	if err != nil {
		t.Fatal(err)
	}
	return root, paths
}

func TestPilotRejectsImplicitPathsAndLinkedFiles(t *testing.T) {
	root, paths := testPilotPaths(t)
	for _, input := range [][3]string{
		{},
		{root, "", paths.settings},
		{root, paths.database, filepath.Join(root, "settings.json")},
		{root, filepath.Join(root, "nestworth.db"), paths.settings},
	} {
		if _, err := resolvePilotPaths(input[0], input[1], input[2]); err == nil {
			t.Fatalf("accepted unsafe path configuration %v", input)
		}
	}

	ctx := context.Background()
	if err := preparePilotFixture(ctx, paths); err != nil {
		t.Fatal(err)
	}
	if err := preparePilotFixture(ctx, paths); err == nil {
		t.Fatal("the seeder accepted an existing data directory")
	}
	outside := filepath.Join(root, "saved-settings.json")
	if err := os.Rename(paths.settings, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, paths.settings); err != nil {
		t.Fatal(err)
	}
	if db, err := openPilot(ctx, paths); err == nil {
		_ = db.Close()
		t.Fatal("accepted a linked settings file")
	}
	if err := os.Remove(paths.settings); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(outside, paths.settings); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(paths.database, filepath.Join(root, "second-database-link")); err != nil {
		t.Fatal(err)
	}
	if db, err := openPilot(ctx, paths); err == nil {
		_ = db.Close()
		t.Fatal("accepted a hard-linked database")
	}
}

func TestPilotRequiresTheHouseholdMarker(t *testing.T) {
	_, paths := testPilotPaths(t)
	ctx := context.Background()
	if err := preparePilotFixture(ctx, paths); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	err = sqlite.NewConfigurationRepository(db).SaveConfiguration(pilotMarkerKey, []byte("a different household"))
	closeErr := db.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("could not corrupt the test marker: %v, %v", err, closeErr)
	}
	if db, err := openPilot(ctx, paths); err == nil {
		_ = db.Close()
		t.Fatal("accepted a database with a mismatched marker")
	} else if !strings.Contains(err.Error(), "fixture marker does not match") {
		t.Fatalf("unexpected startup failure: %v", err)
	}
}

func TestPilotInstitutionsPersistOnReopen(t *testing.T) {
	_, paths := testPilotPaths(t)
	ctx := context.Background()
	if err := preparePilotFixture(ctx, paths); err != nil {
		t.Fatal(err)
	}
	db, err := openPilot(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := application.NewService(sqlite.NewRepository(db))
	rows, err := service.ListInstitutions(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var bank domain.Institution
	for _, row := range rows {
		if row.Name == "Harbor Sample Bank" {
			bank = row
			break
		}
	}
	if bank.ID == "" {
		t.Fatal("the synthetic bank was not listed")
	}
	if _, err := service.UpdateInstitution(ctx, bank.ID, "  Harbor Pilot Bank  "); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckpointWAL(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openPilot(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err = application.NewService(sqlite.NewRepository(reopened)).ListInstitutions(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == bank.ID {
			if row.Name != "Harbor Pilot Bank" || row.InstitutionType != bank.InstitutionType {
				t.Fatalf("unexpected authoritative institution after reopening: %q, %q", row.Name, row.InstitutionType)
			}
			return
		}
	}
	t.Fatal("the edited institution was missing after reopening")
}
