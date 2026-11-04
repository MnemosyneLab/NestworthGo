package aicredentials_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestDatabaseCredentialLifecycleAndBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewConfigurationRepository(db)
	store := aicredentials.New(repo)
	if !store.Available() || store.With(ctx, func([]byte) error { t.Fatal("missing key callback"); return nil }) == nil {
		t.Fatal("missing status")
	}
	for _, key := range []string{"synthetic-first-key", "synthetic-replacement-key"} {
		if err := store.Put(ctx, []byte(key)); err != nil {
			t.Fatal(err)
		}
		var lease []byte
		if err := store.With(ctx, func(value []byte) error {
			lease = value
			if string(value) != key {
				t.Fatal("wrong key")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, b := range lease {
			if b != 0 {
				t.Fatal("lease not cleared")
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "synthetic.nestworth-backup")
	pkg := backup.Package{Manifest: backup.NewManifest(time.Now(), data, []byte("{}"), sqlite.EntityCounts{}), Database: data, Settings: []byte("{}")}
	if err := backup.WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = aicredentials.New(sqlite.NewConfigurationRepository(db))
	if err := store.With(ctx, func(key []byte) error {
		if string(key) != "synthetic-replacement-key" {
			t.Fatal("restart lost key")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if store.With(ctx, func([]byte) error { t.Fatal("deleted key returned"); return nil }) == nil {
		t.Fatal("delete failed")
	}
	verified, err := backup.ReadPackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.db")
	if err := backup.ExtractDatabase(verified, restored); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := sqlite.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	if err := aicredentials.New(sqlite.NewConfigurationRepository(restoredDB)).With(ctx, func(key []byte) error {
		if string(key) != "synthetic-replacement-key" {
			t.Fatal("backup lost key")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type brokenRepository struct{}

func (brokenRepository) LoadConfiguration(string) ([]byte, bool, error) {
	return nil, false, errors.New("synthetic-secret-in-storage-error")
}
func (brokenRepository) SaveConfiguration(string, []byte) error {
	return errors.New("synthetic-secret-in-storage-error")
}
func TestStorageErrorsAreFixedAndCancellationDoesNotWrite(t *testing.T) {
	store := aicredentials.New(brokenRepository{})
	if store.Available() {
		t.Fatal("broken store available")
	}
	for _, err := range []error{store.Put(context.Background(), []byte("synthetic-key")), store.Delete(context.Background()), store.With(context.Background(), func([]byte) error { t.Fatal("callback"); return nil })} {
		if err == nil || err.Error() != "assistant_credentials_unavailable" {
			t.Fatal("unsafe error")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if store.Put(ctx, []byte("synthetic-key")) == nil {
		t.Fatal("cancelled save succeeded")
	}
}
