package aicredentials

import (
	"context"
	"testing"
)

func TestDefaultBuildNeverFallsBackToPlaintext(t *testing.T) {
	store := New("synthetic-only")
	// Ordinary tests/dev/ad-hoc binaries have no audited release identity. This
	// must pass on macOS too, without touching or prompting a user's Keychain.
	if store.Available() {
		t.Fatal("unsigned test runtime has secure credential authority")
	}
	called := false
	if store.Put(context.Background(), []byte("synthetic-canary")) == nil || store.Delete(context.Background()) == nil || store.With(context.Background(), func([]byte) error { called = true; return nil }) == nil || called {
		t.Fatal("closed secure store accepted a credential")
	}
}
