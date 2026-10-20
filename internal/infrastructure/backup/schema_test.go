package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestUnsupportedBackupSchemasNeverReplaceLiveDatabase(t *testing.T) {
	for _, version := range []int{14, sqlite.CurrentSchemaVersion + 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.db")
			db, err := sqlite.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			config := []byte(`{"schema_version":1}`)
			pkg := Package{Manifest: NewManifest(time.Now(), data, config, sqlite.EntityCounts{}), Database: data, Settings: config}
			pkg.Manifest.SchemaVersion = version
			archive := filepath.Join(dir, "unsupported.nestworth-backup")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			manifest, err := json.Marshal(pkg.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string][]byte{MemberManifest: manifest, MemberDatabase: data, MemberSettings: config} {
				member, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := member.Write(content); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			archiveBefore, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ReadPackage(archive)
			assertUnsupportedSchema(t, err)
			archiveAfter, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(archiveBefore, archiveAfter) {
				t.Fatal("archive changed")
			}
			live := filepath.Join(dir, "live.db")
			liveDB, err := sqlite.Open(live)
			if err != nil {
				t.Fatal(err)
			}
			if err := liveDB.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(live)
			if err != nil {
				t.Fatal(err)
			}
			hooksCalled := false
			hooks := &SessionHooks{Quiesce: func(context.Context) error { hooksCalled = true; return nil }}
			assertUnsupportedSchema(t, InstallRestore(context.Background(), live, pkg, hooks, RestoreClasses{}, time.Now()))
			// A forged current manifest must still fail the read-only database verifier.
			pkg.Manifest.SchemaVersion = sqlite.CurrentSchemaVersion
			assertUnsupportedSchema(t, InstallRestore(context.Background(), live, pkg, hooks, RestoreClasses{}, time.Now()))
			after, err := os.ReadFile(live)
			if err != nil {
				t.Fatal(err)
			}
			if hooksCalled || !bytes.Equal(before, after) {
				t.Fatal("unsupported restore touched the live database/session")
			}
			for _, name := range []string{StagingName(live), filepath.Base(JournalPath(live))} {
				if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("restore artifact %s: %v", name, err)
				}
			}
		})
	}
}
func assertUnsupportedSchema(t *testing.T, err error) {
	t.Helper()
	var appErr *domain.Error
	if !errors.As(err, &appErr) || appErr.Code != domain.ErrBackupSchemaUnsupported {
		t.Fatalf("error = %v, want backup_schema_unsupported", err)
	}
}

func TestV15PackageRestoreUpgradesAndKeepsOriginalPackage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "synthetic-v15.sqlite")
	schema, err := os.ReadFile("../sqlite/testdata/schema_v15.sql")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", source+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`INSERT INTO households VALUES('00000000-0000-4000-8000-000000000001',1,'Synthetic','USD','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,symbol,icon_key,created_at,updated_at)
 VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Full identity','mutual_fund','USD','000001','chart-line','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');`); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	database, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"schema_version":1}`)
	pkg := Package{Manifest: NewManifest(time.Now(), database, config, sqlite.EntityCounts{Households: 1, Instruments: 1}), Database: database, Settings: config}
	pkg.Manifest.SchemaVersion = 15
	archive := filepath.Join(dir, "v15.nestworth-backup")
	if err := WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadPackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "restored.sqlite")
	if err := InstallRestore(context.Background(), dest, read, nil, RestoreClasses{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileOnStartup(dest, nil); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name, symbol, alias string
	if err := restored.SQL.QueryRow(`SELECT name,symbol,short_name FROM instruments`).Scan(&name, &symbol, &alias); err != nil || name != "Full identity" || symbol != "000001" || alias != "" {
		t.Fatalf("identity %q %q %q: %v", name, symbol, alias, err)
	}
	after, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("restore changed old backup package")
	}
}

func TestSupportedManifestVersionMustMatchDatabase(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "v16.sqlite")
	db, err := sqlite.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	config := []byte(`{}`)
	pkg := Package{Manifest: NewManifest(time.Now(), data, config, sqlite.EntityCounts{}), Database: data, Settings: config}
	pkg.Manifest.SchemaVersion = 15
	dest := filepath.Join(dir, "live.sqlite")
	err = InstallRestore(context.Background(), dest, pkg, nil, RestoreClasses{}, time.Now())
	var failure *domain.Error
	if !errors.As(err, &failure) || failure.Code != domain.ErrBackupIntegrityFailed {
		t.Fatalf("manifest mismatch %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("mismatched package was installed")
	}
}
