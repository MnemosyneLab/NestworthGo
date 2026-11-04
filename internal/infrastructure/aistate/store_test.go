package aistate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
)

func TestVersionedPrivateStoreAndRollback(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, "ledger-a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "ledger-a"); e == nil {
		t.Fatal("concurrent store owner admitted")
	}
	state, e := s.Load()
	if e != nil || state.Version != 1 {
		t.Fatal(e)
	}
	state.Revision++
	if e := s.Save(state); e != nil {
		t.Fatal(e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "ledger-b"); e == nil {
		t.Fatal("different ledger admitted")
	}
	s, e = Open(dir, "ledger-a")
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.Load()
	if e != nil || got.Revision != state.Revision {
		t.Fatal("durable state lost")
	}
	if _, e := s.db.Exec("PRAGMA user_version=99"); e != nil {
		t.Fatal(e)
	}
	_ = s.Close()
	before, _ := os.ReadFile(filepath.Join(dir, "state.db"))
	if _, e := Open(dir, "ledger-a"); e == nil {
		t.Fatal("future schema opened")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "state.db"))
	if !bytes.Equal(before, after) {
		t.Fatal("future schema was modified before rejection")
	}
	// Rolling back this component never migrates or writes a financial database.
	// A previous app ignores the separate assistant directory; removing only this
	// fixture sidecar starts a fresh disabled namespace with no operation replay.
	if e := os.Remove(filepath.Join(dir, "state.db")); e != nil {
		t.Fatal(e)
	}
	s, e = Open(dir, "ledger-a")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got, e = s.Load()
	if e != nil || got.Config.Enabled || got.Version != assistant.StateVersion || len(got.Conversations) != 0 {
		t.Fatal("unsafe rollback initialization")
	}
}
func TestPrivateStoreRefusesSymlinkAndBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "ledger"); e == nil {
		t.Fatal("broad directory admitted")
	}
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "target")
	if e := os.WriteFile(outside, []byte("do not modify"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(dir, "state.db")); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "ledger"); e == nil {
		t.Fatal("symlink store admitted")
	}
	raw, _ := os.ReadFile(outside)
	if string(raw) != "do not modify" {
		t.Fatal("symlink target changed")
	}
}

func TestCorruptPrivateSchemaAndFuturePayloadFailClosed(t *testing.T) {
	for _, mode := range []string{"schema", "payload"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			s, e := Open(dir, "ledger")
			if e != nil {
				t.Fatal(e)
			}
			if mode == "schema" {
				_, e = s.db.Exec("ALTER TABLE assistant_state ADD COLUMN unknown TEXT")
			} else {
				_, e = s.db.Exec(`UPDATE assistant_state SET payload='{"version":99,"ledger":"ledger","revision":1,"config":{"provider":"openai-responses","model":"mock","enabled":false},"conversations":[]}'`)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			if _, e := Open(dir, "ledger"); e == nil {
				t.Fatal("corrupt/future state accepted")
			}
		})
	}
}
