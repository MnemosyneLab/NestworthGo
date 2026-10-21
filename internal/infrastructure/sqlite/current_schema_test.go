package sqlite

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestOpenRejectsUnsupportedVersionsWithoutWriting(t *testing.T) {
	// Representative old boundary, unversioned file and future versions.
	cases := []struct {
		version  int
		readOnly bool
	}{
		{version: 20, readOnly: false},
		{version: 20, readOnly: true},
		{version: CurrentSchemaVersion + 1, readOnly: false},
		{version: CurrentSchemaVersion + 1, readOnly: true},
		{version: 0, readOnly: false},
	}
	for _, testCase := range cases {
		version, readOnly := testCase.version, testCase.readOnly
		name := fmt.Sprint(version)
		if readOnly {
			name += "-readonly"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unsupported.db")
			seed, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := seed.Exec(fmt.Sprintf("CREATE TABLE marker(value TEXT); INSERT INTO marker VALUES('preserve'); PRAGMA user_version = %d", version)); err != nil {
				t.Fatal(err)
			}
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			var openErr error
			if readOnly {
				_, openErr = OpenReadOnlyForVerify(path)
			} else {
				_, openErr = Open(path)
			}
			if readOnly {
				var appErr *domain.Error
				if !errors.As(openErr, &appErr) || appErr.Code != domain.ErrBackupSchemaUnsupported {
					t.Fatalf("read-only error = %v", openErr)
				}
			} else {
				status := StatusLegacyDatabase
				if version > CurrentSchemaVersion {
					status = StatusUnsupportedFuture
				}
				var bootErr *BootstrapError
				if !errors.As(openErr, &bootErr) || bootErr.Status != status || bootErr.Found != version {
					t.Fatalf("open error = %v", openErr)
				}
				message := openErr.Error()
				if !strings.Contains(message, "new database") || !strings.Contains(message, fmt.Sprintf("found version %d", version)) || !strings.Contains(message, fmt.Sprintf("supported version %d", CurrentSchemaVersion)) {
					t.Fatalf("open error = %v, want found/supported versions and new-database guidance", openErr)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || info.Mode() != afterInfo.Mode() || !info.ModTime().Equal(afterInfo.ModTime()) {
				t.Fatal("rejected database was modified")
			}
			for _, suffix := range []string{"-wal", "-shm", "-journal"} {
				if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected sidecar %s: %v", suffix, err)
				}
			}
		})
	}
}

func TestCurrentSchemaDoesNotRepairMissingTablesOrOldConstraints(t *testing.T) {
	for _, defect := range []string{"activity_mutation_keys", "change_batch_mutation_keys", "agent_quote_records", "old_cash_check"} {
		t.Run(defect, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "incomplete.db")
			seed, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := seed.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := createCurrentSchema(t.Context(), tx); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if defect == "old_cash_check" {
				// Mutate only this synthetic definition after using the current bootstrap.
				// Reopen below so SQLite reparses the altered CHECK constraint.
				if _, err := seed.Exec(`PRAGMA writable_schema=ON;
 UPDATE sqlite_master SET sql=replace(sql,
 "account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode IN ('balance','holdings')",
 "account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode = 'balance'") WHERE name='accounts';
 PRAGMA writable_schema=OFF;`); err != nil {
					t.Fatal(err)
				}
			}
			if defect != "old_cash_check" {
				if _, err := seed.Exec("DROP TABLE " + defect); err != nil {
					t.Fatal(err)
				}
			}
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			_, openErr := Open(path)
			var bootErr *BootstrapError
			if !errors.As(openErr, &bootErr) || bootErr.Status != StatusIntegrityFailed {
				t.Fatalf("Open = %v", openErr)
			}
			_, openErr = OpenReadOnlyForVerify(path)
			var appErr *domain.Error
			if !errors.As(openErr, &appErr) || appErr.Code != domain.ErrBackupIntegrityFailed {
				t.Fatalf("read-only = %v", openErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("invalid schema was repaired or changed")
			}
		})
	}
}

func TestOpenRejectsExistingEmptyFileWithoutInitializingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path)
	var bootErr *BootstrapError
	if !errors.As(err, &bootErr) || bootErr.Status != StatusLegacyDatabase || bootErr.Found != 0 {
		t.Fatalf("Open = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatal("existing file was initialized")
	}
}

func TestOpenInitializesCurrentInMemoryDatabase(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("version = %d", version)
	}
}
