package backup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestFirstRestoreStartupRenameBoundaries(t *testing.T) {
	replacement := recoveryDatabase(t, "Synthetic replacement")
	for _, original := range []string{"absent", "legacy"} {
		for _, boundary := range []struct {
			state     string
			installed bool
		}{
			{StatePrepared, false},
			{StateOriginalRenamed, false},
			{StateOriginalRenamed, true},
			{StateReplacementInstalled, true},
			{StateVerified, true},
		} {
			t.Run(fmt.Sprintf("%s/%s/installed=%t", original, boundary.state, boundary.installed), func(t *testing.T) {
				live := filepath.Join(t.TempDir(), "synthetic.db")
				journal := recoveryJournal(live, boundary.state)
				candidate := filepath.Join(filepath.Dir(live), journal.StagingName)
				if boundary.installed {
					candidate = live
				}
				writeRecoveryFile(t, candidate, replacement)
				writeRecoveryJournal(t, live, journal, original)
				err := ReconcileOnStartup(live, nil)
				if original == "legacy" && !boundary.installed {
					// Old journals cannot prove that an original was absent.
					if err == nil || fileExists(live) || !fileExists(candidate) || !fileExists(JournalPath(live)) {
						t.Fatalf("ambiguous legacy journal was discarded: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				assertRecoveryHousehold(t, live, "Synthetic replacement")
				assertRecoveryFinished(t, live, journal)
			})
		}
	}
}

func TestExistingRestoreStartupRenameBoundaries(t *testing.T) {
	// The household exists only in committed WAL frames, so losing a sidecar
	// produces the wrong ledger even when the main file is structurally valid.
	original := recoveryWALDatabase(t, "Synthetic original")
	replacement := recoveryDatabase(t, "Synthetic replacement")
	for _, boundary := range []struct {
		name      string
		state     string
		moved     int
		installed bool
	}{
		{"before-original-rename", StatePrepared, 0, false},
		{"after-original-main", StatePrepared, 1, false},
		{"after-original-wal", StatePrepared, 2, false},
		{"after-original-shm", StatePrepared, 3, false},
		{"before-replacement-rename", StateOriginalRenamed, 3, false},
		{"after-replacement-rename", StateOriginalRenamed, 3, true},
		{"replacement-installed", StateReplacementInstalled, 3, true},
		{"verified", StateVerified, 3, true},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "synthetic.db")
			journal := recoveryJournal(live, boundary.state)
			safety := filepath.Join(filepath.Dir(live), journal.SafetyName)
			staging := filepath.Join(filepath.Dir(live), journal.StagingName)
			for suffix, data := range original {
				writeRecoveryFile(t, live+suffix, data)
			}
			writeRecoveryFile(t, staging, replacement)
			for i, suffix := range []string{"", "-wal", "-shm"} {
				if i < boundary.moved {
					if err := os.Rename(live+suffix, safety+suffix); err != nil {
						t.Fatal(err)
					}
				}
			}
			if boundary.installed {
				if err := os.Rename(staging, live); err != nil {
					t.Fatal(err)
				}
			}
			writeRecoveryJournal(t, live, journal, "present")
			if err := ReconcileOnStartup(live, nil); err != nil {
				t.Fatal(err)
			}
			want := "Synthetic original"
			if boundary.installed {
				want = "Synthetic replacement"
			}
			// Inspect sidecars before opening SQLite, which may manage SHM itself.
			group := live
			if boundary.installed {
				group = safety
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				assertRecoveryBytes(t, group+suffix, original[suffix])
			}
			assertRecoveryHousehold(t, live, want)
			assertRecoveryFinished(t, live, journal)
		})
	}
}

func TestFirstRestoreStartupStagedWALRenameBoundaries(t *testing.T) {
	replacement := recoveryWALDatabase(t, "Synthetic WAL replacement")
	for _, original := range []string{"absent", "legacy"} {
		for moved := 1; moved <= 3; moved++ {
			t.Run(fmt.Sprintf("%s/after-%d-renames", original, moved), func(t *testing.T) {
				live := filepath.Join(t.TempDir(), "synthetic.db")
				journal := recoveryJournal(live, StateOriginalRenamed)
				staging := filepath.Join(filepath.Dir(live), journal.StagingName)
				for i, suffix := range []string{"", "-wal", "-shm"} {
					path := staging + suffix
					if i < moved {
						path = live + suffix
					}
					writeRecoveryFile(t, path, replacement[suffix])
				}
				writeRecoveryJournal(t, live, journal, original)
				if err := ReconcileOnStartup(live, nil); err != nil {
					t.Fatal(err)
				}
				assertRecoveryHousehold(t, live, "Synthetic WAL replacement")
				assertRecoveryFinished(t, live, journal)
			})
		}
	}
}

func TestRestoreStartupInvalidOrMissingReplacementPreservesEvidence(t *testing.T) {
	for _, state := range []string{StateOriginalRenamed, StateReplacementInstalled, StateVerified} {
		for _, candidate := range []string{"invalid-main", "orphan-sidecars", "missing"} {
			for _, original := range []string{"absent", "legacy"} {
				t.Run(state+"/"+candidate+"/"+original, func(t *testing.T) {
					live := filepath.Join(t.TempDir(), "synthetic.db")
					journal := recoveryJournal(live, state)
					if candidate == "invalid-main" {
						writeRecoveryFile(t, live, []byte("invalid synthetic SQLite"))
					}
					if candidate != "missing" {
						writeRecoveryFile(t, live+"-wal", []byte("synthetic evidence WAL"))
						writeRecoveryFile(t, live+"-shm", []byte("synthetic evidence SHM"))
					}
					writeRecoveryJournal(t, live, journal, original)
					before, err := os.ReadFile(JournalPath(live))
					if err != nil {
						t.Fatal(err)
					}
					for attempt := 0; attempt < 2; attempt++ {
						if err := ReconcileOnStartup(live, nil); err == nil {
							t.Fatal("unsafe recovery reported success")
						}
						assertRecoveryBytes(t, JournalPath(live), before)
						if candidate == "invalid-main" {
							assertRecoveryBytes(t, live, []byte("invalid synthetic SQLite"))
						} else if fileExists(live) {
							t.Fatal("recovery created an empty live database")
						}
						if candidate != "missing" {
							assertRecoveryBytes(t, live+"-wal", []byte("synthetic evidence WAL"))
							// SQLite may rebuild the SHM index during a read-only
							// verification. It must retain the financial main/WAL;
							// an orphan group never reaches SQLite at all.
							if candidate == "orphan-sidecars" {
								assertRecoveryBytes(t, live+"-shm", []byte("synthetic evidence SHM"))
							} else if !fileExists(live + "-shm") {
								t.Fatal("recovery discarded the replacement SHM")
							}
						}
					}
				})
			}
		}
	}
}

func TestFirstRestoreStartupRejectsInvalidStaging(t *testing.T) {
	for _, state := range []string{StatePrepared, StateOriginalRenamed} {
		t.Run(state, func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "synthetic.db")
			journal := recoveryJournal(live, state)
			staging := filepath.Join(filepath.Dir(live), journal.StagingName)
			writeRecoveryFile(t, staging, []byte("invalid staged database"))
			writeRecoveryJournal(t, live, journal, "absent")
			if err := ReconcileOnStartup(live, nil); err == nil {
				t.Fatal("invalid staging accepted")
			}
			if fileExists(live) || !fileExists(JournalPath(live)) {
				t.Fatal("invalid first restore lost its recovery fence")
			}
			assertRecoveryBytes(t, staging, []byte("invalid staged database"))
		})
	}
}

func TestInstallRestoreRecordsOriginalPresence(t *testing.T) {
	replacement := recoveryDatabase(t, "Synthetic replacement")
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "synthetic.db")
			if present {
				writeRecoveryFile(t, live, []byte("old unreadable database"))
			}
			pkg := Package{Manifest: NewManifest(time.Now().UTC(), replacement, []byte(`{}`), sqlite.EntityCounts{}), Database: replacement, Settings: []byte(`{}`)}
			hooks := &SessionHooks{Close: func() error {
				prepared, err := ReadJournal(live)
				if err != nil || prepared.State != StatePrepared {
					t.Fatalf("restore was not journaled before closing: %v", err)
				}
				return nil
			}}
			if err := InstallRestore(t.Context(), live, pkg, hooks, RestoreClasses{}, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(JournalPath(live))
			if err != nil {
				t.Fatal(err)
			}
			var journal struct {
				OriginalPresent *bool `json:"original_present"`
			}
			if err := jsoncodec.Unmarshal(raw, &journal); err != nil {
				t.Fatal(err)
			}
			if journal.OriginalPresent == nil || *journal.OriginalPresent != present {
				t.Fatalf("original presence was not persisted: %s", raw)
			}
			if err := ReconcileOnStartup(live, nil); err != nil {
				t.Fatal(err)
			}
			assertRecoveryHousehold(t, live, "Synthetic replacement")
		})
	}
}

func TestRestoreStartupRollbackRenameBoundaries(t *testing.T) {
	original := recoveryDatabase(t, "Synthetic original")
	for _, state := range []string{"quarantining", "rolling-back"} {
		for moved := 0; moved <= 3; moved++ {
			t.Run(fmt.Sprintf("%s/after-%d-renames", state, moved), func(t *testing.T) {
				live := filepath.Join(t.TempDir(), "synthetic.db")
				journal := recoveryJournal(live, state)
				safety := filepath.Join(filepath.Dir(live), journal.SafetyName)
				failed := live + ".failed-restore-synthetic"
				writeRecoveryFile(t, safety, original)
				writeRecoveryFile(t, safety+"-wal", []byte("original WAL"))
				writeRecoveryFile(t, safety+"-shm", []byte("original SHM"))
				writeRecoveryFile(t, live, []byte("invalid replacement"))
				writeRecoveryFile(t, live+"-wal", []byte("replacement WAL"))
				writeRecoveryFile(t, live+"-shm", []byte("replacement SHM"))
				from, to := live, failed
				if state == "rolling-back" {
					if err := MoveFileGroup(live, failed); err != nil {
						t.Fatal(err)
					}
					from, to = safety, live
				}
				for i, suffix := range []string{"", "-wal", "-shm"} {
					if i < moved {
						if err := os.Rename(from+suffix, to+suffix); err != nil {
							t.Fatal(err)
						}
					}
				}
				writeRecoveryJournal(t, live, journal, "present")
				// Add the quarantine path separately to retain baseline compilation.
				raw, err := os.ReadFile(JournalPath(live))
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err := jsoncodec.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				fields["failed_name"] = filepath.Base(failed)
				raw, err = jsoncodec.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				writeRecoveryFile(t, JournalPath(live), raw)
				if err := ReconcileOnStartup(live, nil); err != nil {
					t.Fatal(err)
				}
				assertRecoveryBytes(t, failed, []byte("invalid replacement"))
				assertRecoveryBytes(t, failed+"-wal", []byte("replacement WAL"))
				assertRecoveryBytes(t, failed+"-shm", []byte("replacement SHM"))
				assertRecoveryBytes(t, live+"-wal", []byte("original WAL"))
				assertRecoveryBytes(t, live+"-shm", []byte("original SHM"))
				assertRecoveryHousehold(t, live, "Synthetic original")
				assertRecoveryFinished(t, live, journal)
			})
		}
	}
}

func TestRestoreStartupConflictingSidecarsStayBlocked(t *testing.T) {
	original := recoveryDatabase(t, "Synthetic original")
	for _, state := range []string{StatePrepared, "rolling-back"} {
		t.Run(state, func(t *testing.T) {
			live := filepath.Join(t.TempDir(), "synthetic.db")
			journal := recoveryJournal(live, state)
			safety := filepath.Join(filepath.Dir(live), journal.SafetyName)
			writeRecoveryFile(t, safety, original)
			writeRecoveryFile(t, safety+"-wal", []byte("safety WAL"))
			writeRecoveryFile(t, live+"-wal", []byte("conflicting live WAL"))
			writeRecoveryJournal(t, live, journal, "present")
			if err := ReconcileOnStartup(live, nil); err == nil {
				t.Fatal("conflicting sidecars were accepted")
			}
			assertRecoveryBytes(t, safety, original)
			assertRecoveryBytes(t, safety+"-wal", []byte("safety WAL"))
			assertRecoveryBytes(t, live+"-wal", []byte("conflicting live WAL"))
			if fileExists(live) || !fileExists(JournalPath(live)) {
				t.Fatal("conflicted recovery lost its fence")
			}
		})
	}
}

func recoveryDatabase(t *testing.T, household string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001',?,'CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, household); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func recoveryWALDatabase(t *testing.T, household string) map[string][]byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wal-fixture.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CheckpointWAL(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001',?,'CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, household); err != nil {
		t.Fatal(err)
	}
	// Copy the idle, synthetic file group while its session is still open;
	// Close would checkpoint away the crash-persisted WAL being exercised.
	group := make(map[string][]byte)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || len(data) == 0 {
			t.Fatalf("synthetic WAL fixture %s missing: %v", suffix, err)
		}
		group[suffix] = data
	}
	return group
}

func recoveryJournal(live, state string) Journal {
	return Journal{Operation: "restore", State: state, LiveName: filepath.Base(live), SafetyName: filepath.Base(live) + ".pre-restore-synthetic", StagingName: StagingName(live), StartedAt: time.Now().UTC().Format(time.RFC3339)}
}

// Encode the optional metadata separately so these regressions also compile
// against the implementation predating original_present.
func writeRecoveryJournal(t *testing.T, live string, journal Journal, original string) {
	t.Helper()
	if err := WriteJournal(live, journal); err != nil {
		t.Fatal(err)
	}
	if original == "legacy" {
		return
	}
	raw, err := os.ReadFile(JournalPath(live))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := jsoncodec.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["original_present"] = original == "present"
	raw, err = jsoncodec.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	writeRecoveryFile(t, JournalPath(live), raw)
}

func writeRecoveryFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveryBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("recovery artifact %s changed: got %d bytes, want %d bytes, err %v", filepath.Base(path), len(got), len(want), err)
	}
}

func assertRecoveryHousehold(t *testing.T, live, want string) {
	t.Helper()
	db, err := sqlite.OpenReadOnlyForVerify(live)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	summary, err := db.HouseholdSummary(t.Context())
	if err != nil || summary.Name != want {
		t.Fatalf("household = %q, want %q: %v", summary.Name, want, err)
	}
}

func assertRecoveryFinished(t *testing.T, live string, journal Journal) {
	t.Helper()
	if fileExists(JournalPath(live)) || GroupExists(filepath.Join(filepath.Dir(live), journal.StagingName)) {
		t.Fatal("completed recovery retained its journal or staging group")
	}
	if err := ReconcileOnStartup(live, nil); err != nil {
		t.Fatalf("completed recovery was not idempotent: %v", err)
	}
}
