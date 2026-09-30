package backup

import (
	"archive/zip"
	"bytes"
	"context"
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
	for _, version := range []int{0, 9, 10, 11, 12, 13, 14, sqlite.CurrentSchemaVersion + 1, 99} {
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
