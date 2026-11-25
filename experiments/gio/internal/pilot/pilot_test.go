package pilot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/testledger"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func testPaths(t *testing.T) (string, Paths) {
	t.Helper()
	root := t.TempDir()
	return root, dataPaths(filepath.Join(root, "data"))
}

func seedTestLedger(ctx context.Context, paths Paths) error {
	return testledger.Seed(ctx, paths.database, paths.settings)
}

func TestProfileInstitutionsPersistOnReopen(t *testing.T) {
	_, paths := testPaths(t)
	ctx := context.Background()
	if err := seedTestLedger(ctx, paths); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	service := application.NewService(sqlite.NewRepository(session.DB))
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
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err = application.NewService(sqlite.NewRepository(reopened.DB)).ListInstitutions(ctx, false)
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
