package secrets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
)

func TestMemoryStoreRoundTripAndDerivedConfiguredFlag(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	ref := application.TiingoSecretRef()
	status, err := store.Status(ctx, ref)
	if err != nil || status != application.SecretStatusMissing || application.TiingoKeyConfigured(status) {
		t.Fatalf("empty status = %s err=%v", status, err)
	}
	status, err = store.Put(ctx, ref, []byte("test-tiingo-key"))
	if err != nil || status != application.SecretStatusAvailable {
		t.Fatalf("put = %s err=%v", status, err)
	}
	if !application.TiingoKeyConfigured(status) {
		t.Fatal("available key is not configured")
	}
	value, status, err := store.Get(ctx, ref)
	if err != nil || status != application.SecretStatusAvailable || string(value) != "test-tiingo-key" {
		t.Fatalf("get = %q %s err=%v", value, status, err)
	}
	status, err = store.Delete(ctx, ref)
	if err != nil || status != application.SecretStatusMissing || application.TiingoKeyConfigured(status) {
		t.Fatalf("delete = %s err=%v", status, err)
	}
}

func TestUnavailableStoreUsesSessionOnlyMemory(t *testing.T) {
	ctx := context.Background()
	store := NewUnavailableStore()
	ref := application.TiingoSecretRef()
	status, err := store.Status(ctx, ref)
	if err != nil || status != application.SecretStatusUnavailable {
		t.Fatalf("unavailable status = %s err=%v", status, err)
	}
	status, err = store.Put(ctx, ref, []byte("session-key"))
	if err != nil || status != application.SecretStatusSessionOnly {
		t.Fatalf("session put = %s err=%v", status, err)
	}
	if !application.TiingoKeyConfigured(status) {
		t.Fatal("session-only key should count as configured for this process")
	}
	value, status, err := store.Get(ctx, ref)
	if err != nil || string(value) != "session-key" || status != application.SecretStatusSessionOnly {
		t.Fatalf("session get = %q %s err=%v", value, status, err)
	}
}

func TestLockedStoreDoesNotStoreSecrets(t *testing.T) {
	ctx := context.Background()
	store := NewLockedStore()
	ref := application.TiingoSecretRef()
	status, err := store.Put(ctx, ref, []byte("locked-key"))
	if err != nil || status != application.SecretStatusLocked {
		t.Fatalf("locked put = %s err=%v", status, err)
	}
	value, status, err := store.Get(ctx, ref)
	if err != nil || value != nil || status != application.SecretStatusLocked {
		t.Fatalf("locked get = %q %s err=%v", value, status, err)
	}
	if application.TiingoKeyConfigured(status) {
		t.Fatal("locked store must not report a configured key")
	}
}

func TestSecretStoreDoesNotWritePlaintextFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store := NewMemoryStore()
	if _, err := store.Put(ctx, application.TiingoSecretRef(), []byte("must-not-hit-disk")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("secret store wrote files under %s: %v", dir, names(entries))
	}
	rootEntries, err := os.ReadDir(filepath.Join(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range rootEntries {
		if strings.Contains(strings.ToLower(entry.Name()), "tiingo") || strings.Contains(strings.ToLower(entry.Name()), "secret") {
			t.Fatalf("secret-looking file %s", entry.Name())
		}
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}
