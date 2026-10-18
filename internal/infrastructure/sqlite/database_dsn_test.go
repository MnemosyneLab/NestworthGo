package sqlite

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSQLiteFileURLEncodesAbsoluteFilesystemPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger space 東京 ? #.db")
	fileURL, err := sqliteFileURL(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(fileURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(fileURL, " ") || strings.Contains(fileURL, "東京") || strings.Contains(fileURL, "?") || strings.Contains(fileURL, "#") {
		t.Fatalf("file URL did not escape filesystem characters: %q (%+v)", fileURL, parsed)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("test path is not absolute: %q", path)
	}
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(path)
		if len(volume) == 2 && volume[1] == ':' {
			wantPrefix := "file:///" + volume + "/"
			if !strings.HasPrefix(strings.ToLower(fileURL), strings.ToLower(wantPrefix)) {
				t.Fatalf("Windows drive path URL = %q, want prefix %q", fileURL, wantPrefix)
			}
		} else if parsed.Host == "" {
			t.Fatalf("Windows UNC path URL has no host: %q", fileURL)
		}
	} else if !strings.HasPrefix(fileURL, "file:///") || parsed.Path != filepath.ToSlash(path) {
		t.Fatalf("absolute file URL = %q, parsed path %q, want %q", fileURL, parsed.Path, filepath.ToSlash(path))
	}

	dsn, err := sqliteDSN(path, url.Values{"mode": {"ro"}})
	if err != nil {
		t.Fatal(err)
	}
	parsedDSN, err := url.Parse(dsn)
	if err != nil || parsedDSN.Query().Get("mode") != "ro" || parsedDSN.Fragment != "" {
		t.Fatalf("DSN options were not separated from the filename: %q (%v)", dsn, err)
	}
}

func TestSQLiteOpenReopensPathWithSpaceUnicodeQueryAndFragmentCharacters(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger space 東京 ? #.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, "CREATE TABLE path_probe (value TEXT NOT NULL)"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, "INSERT INTO path_probe(value) VALUES (?)", "original file"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var value string
	if err := reopened.SQL.QueryRowContext(ctx, "SELECT value FROM path_probe").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "original file" {
		t.Fatalf("reopened file marker = %q", value)
	}
}

func TestSQLiteOpenRejectsUnwritableDirectoryWithoutRedirecting(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "ordinary-file")
	if err := os.WriteFile(parent, []byte("synthetic marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "ledger.db")
	if _, err := Open(path); err == nil {
		t.Fatal("database under a regular file unexpectedly opened")
	}
	if contents, err := os.ReadFile(parent); err != nil || string(contents) != "synthetic marker" {
		t.Fatalf("failed open changed the blocking path: contents=%q err=%v", contents, err)
	}
}

func TestSQLiteOpenFollowsSymlinkToExistingDatabase(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	target := filepath.Join(directory, "target.db")
	link := filepath.Join(directory, "ledger link.db")
	database, err := Open(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, "CREATE TABLE path_probe (value TEXT NOT NULL)"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, "INSERT INTO path_probe(value) VALUES (?)", "symlink target"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable on this host: %v", err)
	}

	throughLink, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	defer throughLink.Close()
	var value string
	if err := throughLink.SQL.QueryRowContext(ctx, "SELECT value FROM path_probe").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "symlink target" {
		t.Fatalf("symlink open returned marker %q", value)
	}
}

func TestSQLiteMemoryDatabaseOverrideStillWorks(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.SQL.Exec("CREATE TABLE isolated_override (value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
}
