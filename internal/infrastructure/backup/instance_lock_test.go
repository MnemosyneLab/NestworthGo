package backup

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestInstanceLockRejectsDuplicateAndAllowsRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.db")
	close1, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if close2, err := AcquireInstanceLock(path); !errors.Is(err, ErrAlreadyRunning) {
		if close2 != nil {
			close2()
		}
		t.Fatalf("duplicate lock: %v", err)
	}
	if err := close1(); err != nil {
		t.Fatal(err)
	}
	close3, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	close3()
}
