package mcpserver

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	modernc "modernc.org/sqlite"
)

// The function runs inside the archive SQL, after the old implementation had
// already committed name/icon. It is test-only and only opens synthetic data.
var directorySQLFaults sync.Map

func init() {
	modernc.MustRegisterScalarFunction("test_directory_fault", 1, func(_ *modernc.FunctionContext, args []driver.Value) (driver.Value, error) {
		fn, ok := directorySQLFaults.Load(args[0].(string))
		if !ok {
			return nil, errors.New("missing synthetic fault")
		}
		return nil, fn.(func() error)()
	})
}

type directoryFaultRepository struct {
	application.Repository
	cancelKey   string
	afterCommit func()
	cancelled   atomic.Bool
}

func (r *directoryFaultRepository) run(ctx context.Context, write func(context.Context) error) error {
	if r.cancelKey != "" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		directorySQLFaults.Store(r.cancelKey, func() error { cancel(); r.cancelled.Store(true); return context.Canceled })
		defer directorySQLFaults.Delete(r.cancelKey)
	}
	err := write(ctx)
	if err == nil && r.afterCommit != nil {
		r.afterCommit()
	}
	return err
}
func (r *directoryFaultRepository) UpdateMember(ctx context.Context, v domain.Member) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.UpdateMember(ctx, v) })
}
func (r *directoryFaultRepository) UpdateInstitution(ctx context.Context, v domain.Institution) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.UpdateInstitution(ctx, v) })
}
func (r *directoryFaultRepository) UpdateGroup(ctx context.Context, v domain.Group) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.UpdateGroup(ctx, v) })
}
func (r *directoryFaultRepository) SetMemberArchive(ctx context.Context, h domain.HouseholdID, id domain.MemberID, archived bool, now time.Time) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.SetMemberArchive(ctx, h, id, archived, now) })
}
func (r *directoryFaultRepository) SetInstitutionArchive(ctx context.Context, h domain.HouseholdID, id domain.InstitutionID, archived bool, now time.Time) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.SetInstitutionArchive(ctx, h, id, archived, now) })
}
func (r *directoryFaultRepository) SetGroupArchive(ctx context.Context, h domain.HouseholdID, id domain.GroupID, archived bool, now time.Time) error {
	return r.run(ctx, func(ctx context.Context) error { return r.Repository.SetGroupArchive(ctx, h, id, archived, now) })
}

var directoryEntities = []struct{ kind, table string }{{"member", "members"}, {"institution", "institutions"}, {"group", "account_groups"}}

func directoryAtomicFixture(t *testing.T, dir string) (*Service, *sqlite.DB, *directoryFaultRepository, *atomic.Int32) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := &directoryFaultRepository{Repository: sqlite.NewRepository(db)}
	app := application.NewService(repo)
	var ticks atomic.Int64
	app.SetClock(func() time.Time {
		return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC).Add(time.Duration(ticks.Add(1)) * time.Second)
	})
	h, err := repo.Household(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic atomicity", BaseCurrency: "USD", MemberNames: []string{"Alice"}}); err != nil {
			t.Fatal(err)
		}
	}
	changes := &atomic.Int32{}
	s := New(app, dir, func() { changes.Add(1) })
	t.Cleanup(s.Close)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	return s, db, repo, changes
}

// Capture every column, including fields outside this PATCH and both timestamps.
func directoryRow(t *testing.T, db *sqlite.DB, table, id string) map[string]any {
	t.Helper()
	rows, err := db.SQL.QueryContext(t.Context(), "SELECT * FROM "+table+" WHERE id = ?", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("missing directory row")
	}
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for i, k := range columns {
		out[k] = values[i]
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatal("unexpected directory rows", rows.Err())
	}
	return out
}
func directoryExec(t *testing.T, db *sqlite.DB, query string) {
	t.Helper()
	if _, err := db.SQL.ExecContext(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryCompositeSQLiteFaults(t *testing.T) {
	t.Parallel()
	for _, entity := range directoryEntities {
		for _, archived := range []bool{true, false} {
			for _, fault := range []string{"constraint", "io_error", "cancel"} {
				t.Run(fmt.Sprintf("%s/archive=%t/%s", entity.kind, archived, fault), func(t *testing.T) {
					s, db, repo, changes := directoryAtomicFixture(t, t.TempDir())
					c := connect(t, s)
					input := map[string]any{"name": "Original"}
					if entity.kind == "institution" {
						input["institutionType"] = "bank"
					}
					id := mutate(t, c, "create_"+entity.kind, input)["id"].(string)
					if !archived {
						mutate(t, c, "update_"+entity.kind, map[string]any{"id": id, "archived": true})
					}
					before := directoryRow(t, db, entity.table, id)
					count := changes.Load()
					faultSQL := "SELECT RAISE(ABORT, 'synthetic archive constraint');"
					key := uuid.NewString()
					if fault != "constraint" {
						faultSQL = fmt.Sprintf("SELECT test_directory_fault('%s');", key)
						if fault == "cancel" {
							repo.cancelKey = key
						} else {
							directorySQLFaults.Store(key, func() error { return syscall.EIO })
							defer directorySQLFaults.Delete(key)
						}
					}
					// UPDATE OF catches the former second SQL, in either direction. The
					// AFTER trigger also proves a modified row is rolled back on failure.
					timing := "BEFORE"
					if fault == "constraint" {
						timing = "AFTER"
					}
					directoryExec(t, db, fmt.Sprintf("CREATE TRIGGER directory_fault %s UPDATE OF archived_at ON %s BEGIN %s END", timing, entity.table, faultSQL))
					args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"id": id, "name": "Changed", "iconKey": "fund", "archived": archived}}
					call(t, c, "update_"+entity.kind, args, true)
					if fault == "cancel" && !repo.cancelled.Load() {
						t.Fatal("SQL did not reach cancellation hook")
					}
					if got := directoryRow(t, db, entity.table, id); !reflect.DeepEqual(got, before) {
						t.Fatalf("partial persistence: before=%v after=%v", before, got)
					}
					op, err := s.GetOperation(args["operationId"].(string))
					wantStatus := "unknown"
					if fault == "constraint" && entity.kind == "member" {
						wantStatus = "failed"
					}
					if err != nil || op.Status != wantStatus {
						t.Fatalf("receipt=%+v error=%v want=%s", op, err, wantStatus)
					}
					if changes.Load() != count {
						t.Fatal("failed write notified UI")
					}
					directoryExec(t, db, "DROP TRIGGER directory_fault")
					repo.cancelKey = ""
					// Removing the fault never authorizes replay of failed/unknown IDs.
					call(t, c, "update_"+entity.kind, args, true)
					if fault == "cancel" && !repo.cancelled.Load() {
						t.Fatal("SQL did not reach cancellation hook")
					}
					if got := directoryRow(t, db, entity.table, id); !reflect.DeepEqual(got, before) || changes.Load() != count {
						t.Fatal("same operationId replayed", got)
					}
					args["operationId"] = uuid.NewString()
					result := call(t, c, "update_"+entity.kind, args, false)
					after := directoryRow(t, db, entity.table, id)
					want := map[string]any{}
					for k, v := range before {
						want[k] = v
					}
					want["name"], want["icon_key"], want["updated_at"] = "Changed", "fund", after["updated_at"]
					want["archived_at"] = nil
					if archived {
						want["archived_at"] = after["updated_at"]
					}
					if reflect.DeepEqual(before["updated_at"], after["updated_at"]) || !reflect.DeepEqual(want, after) || changes.Load() != count+1 {
						t.Fatalf("composite success: want=%v after=%v notifications=%d", want, after, changes.Load()-count)
					}
					retry := call(t, c, "update_"+entity.kind, args, false)
					if !reflect.DeepEqual(result, retry) || !reflect.DeepEqual(after, directoryRow(t, db, entity.table, id)) || changes.Load() != count+1 {
						t.Fatal("success retry wrote or notified twice")
					}
				})
			}
		}
	}
}

// Abrupt exit inside the archive statement versus after the repository commit
// bounds the promise: directory fields are atomic, but an outer MCP receipt can
// remain unknown. Neither restart nor retry may dispatch that operation again.
func TestDirectoryCompositeCrashBoundary(t *testing.T) {
	if os.Getenv("NESTWORTH_DIRECTORY_CRASH_CHILD") == "1" {
		dir, kind, id, phase := os.Getenv("NESTWORTH_DIRECTORY_CRASH_DIR"), os.Getenv("NESTWORTH_DIRECTORY_CRASH_KIND"), os.Getenv("NESTWORTH_DIRECTORY_CRASH_ID"), os.Getenv("NESTWORTH_DIRECTORY_CRASH_PHASE")
		s, db, repo, _ := directoryAtomicFixture(t, dir)
		c := connect(t, s)
		if phase == "in_sql" {
			key := uuid.NewString()
			directorySQLFaults.Store(key, func() error { os.Exit(73); return nil })
			table := ""
			for _, entity := range directoryEntities {
				if entity.kind == kind {
					table = entity.table
				}
			}
			directoryExec(t, db, fmt.Sprintf("CREATE TRIGGER directory_crash AFTER UPDATE OF archived_at ON %s BEGIN SELECT test_directory_fault('%s'); END", table, key))
		} else {
			repo.afterCommit = func() { os.Exit(73) }
		}
		archived := os.Getenv("NESTWORTH_DIRECTORY_CRASH_ARCHIVED") == "true"
		call(t, c, "update_"+kind, map[string]any{"operationId": os.Getenv("NESTWORTH_DIRECTORY_CRASH_OP"), "input": map[string]any{"id": id, "name": "Changed", "iconKey": "fund", "archived": archived}}, false)
		t.Fatal("child did not exit at fault")
	}
	t.Parallel()
	for _, entity := range directoryEntities {
		for _, archived := range []bool{true, false} {
			for _, phase := range []string{"in_sql", "after_commit"} {
				t.Run(fmt.Sprintf("%s/archive=%t/%s", entity.kind, archived, phase), func(t *testing.T) {
					dir := t.TempDir()
					s, db, _, _ := directoryAtomicFixture(t, dir)
					c := connect(t, s)
					input := map[string]any{"name": "Original"}
					if entity.kind == "institution" {
						input["institutionType"] = "bank"
					}
					id := mutate(t, c, "create_"+entity.kind, input)["id"].(string)
					if !archived {
						mutate(t, c, "update_"+entity.kind, map[string]any{"id": id, "archived": true})
					}
					before := directoryRow(t, db, entity.table, id)
					s.Close()
					_ = db.Close()
					opID := uuid.NewString()
					crashCtx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
					defer cancel()
					cmd := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestDirectoryCompositeCrashBoundary$")
					cmd.Env = append(os.Environ(), "NESTWORTH_DIRECTORY_CRASH_CHILD=1", "NESTWORTH_DIRECTORY_CRASH_DIR="+dir, "NESTWORTH_DIRECTORY_CRASH_KIND="+entity.kind, "NESTWORTH_DIRECTORY_CRASH_ID="+id, "NESTWORTH_DIRECTORY_CRASH_PHASE="+phase, "NESTWORTH_DIRECTORY_CRASH_OP="+opID, fmt.Sprintf("NESTWORTH_DIRECTORY_CRASH_ARCHIVED=%t", archived))
					output, err := cmd.CombinedOutput()
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 73 {
						t.Fatalf("child: %v %s", err, output)
					}
					resumed, reopened, _, changes := directoryAtomicFixture(t, dir)
					after := directoryRow(t, reopened, entity.table, id)
					want := map[string]any{}
					for k, v := range before {
						want[k] = v
					}
					if phase == "after_commit" {
						want["name"], want["icon_key"], want["updated_at"] = "Changed", "fund", after["updated_at"]
						want["archived_at"] = nil
						if archived {
							want["archived_at"] = after["updated_at"]
						}
					}
					if !reflect.DeepEqual(want, after) {
						t.Fatalf("crash left partial state: want=%v after=%v", want, after)
					}
					op, err := resumed.GetOperation(opID)
					if err != nil || op.Status != "unknown" {
						t.Fatalf("restart receipt=%+v %v", op, err)
					}
					if phase == "in_sql" {
						directoryExec(t, reopened, "DROP TRIGGER directory_crash")
					}
					retry := connect(t, resumed)
					call(t, retry, "update_"+entity.kind, map[string]any{"operationId": opID, "input": map[string]any{"id": id, "name": "Changed", "iconKey": "fund", "archived": archived}}, true)
					if !reflect.DeepEqual(after, directoryRow(t, reopened, entity.table, id)) || changes.Load() != 0 {
						t.Fatal("restart replayed unknown operation")
					}
				})
			}
		}
	}
}
