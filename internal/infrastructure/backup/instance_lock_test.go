package backup

import (
	"errors"
	"os"
	"os/exec"
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

func TestInstanceLockFencesAnotherProcessAndDirectoryAlias(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "business.db")
	release, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstanceLockChild$")
	cmd.Env = append(os.Environ(), "NESTWORTH_TEST_LOCK="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("process fencing: %v %s", err, out)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip("symlink unavailable")
	}
	if other, err := AcquireInstanceLock(filepath.Join(alias, "business.db")); !errors.Is(err, ErrAlreadyRunning) {
		if other != nil {
			other()
		}
		t.Fatalf("directory alias bypassed instance lock: %v", err)
	}
}
func TestInstanceLockChild(t *testing.T) {
	path := os.Getenv("NESTWORTH_TEST_LOCK")
	if path == "" {
		t.Skip("subprocess helper")
	}
	if release, err := AcquireInstanceLock(path); !errors.Is(err, ErrAlreadyRunning) {
		if release != nil {
			release()
		}
		t.Fatalf("parallel process acquired business database: %v", err)
	}
}
