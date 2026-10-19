package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	modernsqlite "modernc.org/sqlite"
)

type noChangeTestRepository struct {
	*sqlite.Repository
	captures, publications, confirmations int
}

func (r *noChangeTestRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, now time.Time) (domain.ReviewProjectionCapture, error) {
	r.captures++
	return r.Repository.CaptureReviewProjection(ctx, id, now)
}

func (r *noChangeTestRepository) PublishReviewProjectionWithClock(ctx context.Context, p domain.ReviewPublication, clock func() time.Time) error {
	r.publications++
	return r.Repository.PublishReviewProjectionWithClock(ctx, p, clock)
}

func (r *noChangeTestRepository) ConfirmReviewNoChange(ctx context.Context, id domain.HouseholdID, now time.Time, clock func() time.Time, provider func() string) (domain.ReviewNoChangeResult, bool, error) {
	r.confirmations++
	return r.Repository.ConfirmReviewNoChange(ctx, id, now, clock, provider)
}

func noChangeService(f seriesZeroFixture) (*Service, *noChangeTestRepository) {
	r := &noChangeTestRepository{Repository: sqlite.NewRepository(f.db).WithReviewNoChangeCandidate()}
	s := NewService(r)
	s.setClock(func() time.Time { return *f.now })
	return s, r
}

func noChangeFixture(t *testing.T) (seriesZeroFixture, *noChangeTestRepository) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic no-change", syntheticHistoricalReviewSpec())
	f.s, _ = noChangeService(f)
	r := f.s.repository.(*noChangeTestRepository)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f, r
}

// Independent cloned DB, new session and forced full recomputation. Retain all
// financial/lifecycle columns; normalize only three computation timestamps.
func noChangeProjectionRows(t *testing.T, db *sqlite.DB) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	for _, table := range []string{"investment_review_control", "investment_review_daily_inputs", "investment_review_daily_results", "investment_review_position_epochs", "investment_review_evaluation_state", "investment_review_episodes", "investment_review_episode_events"} {
		rows, err := db.SQL.Query(`SELECT * FROM ` + table + ` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values, dest := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			for i, column := range columns {
				if column == "computed_at" || column == "evaluated_at" || column == "revised_at" {
					values[i] = nil
				}
			}
			raw, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], string(raw))
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func noChangeFullOracle(t *testing.T, f seriesZeroFixture) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-full-oracle.db")
	if _, err := f.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(sqlite.NewRepository(db).WithReviewNoChangeCandidate().WithFullReviewRecompute())
	s.fxProviderKey = f.s.FXProviderKey()
	s.setClock(func() time.Time { return *f.now })
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, want := noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, db); !reflect.DeepEqual(got, want) {
		for table := range got {
			if !reflect.DeepEqual(got[table], want[table]) {
				t.Errorf("full oracle mismatch %s\ngot=%v\nwant=%v", table, got[table], want[table])
			}
		}
	}
}

func TestReviewNoChangeOptInAndFullOracle(t *testing.T) {
	f, r := noChangeFixture(t)
	if sqlite.NewRepository(f.db).ReviewNoChangeEnabled() || r.WithFullReviewRecompute().ReviewNoChangeEnabled() {
		t.Fatal("default/full oracle enabled shortcut")
	}
	before := noChangeProjectionRows(t, f.db)
	*f.now = f.now.Add(time.Nanosecond)
	got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 0 || got.RuleInstrumentCount != 1 || r.captures != 1 || r.publications != 1 || r.confirmations != 2 {
		t.Fatalf("no-change did not bypass full work: %+v %v %+v", got, err, r)
	}
	if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
		t.Fatal("no-change rewrote projection/lifecycle rows")
	}
	var evaluated string
	if err := f.db.SQL.QueryRow(`SELECT evaluated_at FROM review_temporal_checkpoints`).Scan(&evaluated); err != nil || evaluated != f.now.Format(time.RFC3339Nano) {
		t.Fatal("final logical as-of not committed")
	}
	noChangeFullOracle(t, f)
}

func TestReviewNoChangeConservativeFallbacks(t *testing.T) {
	for _, scenario := range []string{"source", "config", "state", "rule-without-revision", "episode-without-state", "pending", "missing-proof", "unknown-policy", "journal-gap", "trigger-drop-recreate", "foreign-keys", "recursive-triggers", "rollback", "restart", "epoch-reset", "provider"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := noChangeFixture(t)
			statement := ""
			switch scenario {
			case "provider":
				f.s.fxProviderKey = "synthetic-new-provider"
			case "source":
				statement = `UPDATE instrument_quotes SET unit_price='101'`
			case "config":
				statement = `UPDATE investment_review_control SET config_revision=config_revision+1`
			case "state":
				statement = `UPDATE investment_review_control SET review_state_revision=review_state_revision+1`
			case "rule-without-revision":
				statement = `UPDATE investment_review_rules SET name='Synthetic changed name'`
			case "episode-without-state":
				statement = `UPDATE investment_review_episodes SET acknowledged_at='2026-10-03T12:00:00Z',revision=revision+1 WHERE resolved_at IS NULL`
			case "pending":
				statement = `UPDATE investment_review_control SET projection_status='pending'`
			case "missing-proof":
				statement = `UPDATE review_temporal_checkpoints SET certificate_json=json_remove(certificate_json,'$.NoChangeReviewFingerprint')`
			case "unknown-policy":
				statement = `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.PolicyVersion','future-policy')`
			case "journal-gap":
				statement = `DELETE FROM financial_source_changes WHERE revision=(SELECT max(revision) FROM financial_source_changes)`
			case "trigger-drop-recreate":
				var ddl string
				if err := f.db.SQL.QueryRow(`SELECT sql FROM sqlite_master WHERE name='financial_source_instrument_quotes_update'`).Scan(&ddl); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.SQL.Exec(`DROP TRIGGER financial_source_instrument_quotes_update; UPDATE instrument_quotes SET unit_price='101'; ` + ddl); err != nil {
					t.Fatal(err)
				}
			case "foreign-keys":
				statement = `PRAGMA foreign_keys=0`
			case "recursive-triggers":
				statement = `PRAGMA recursive_triggers=0`
			case "rollback":
				r.ObserveReviewClock(f.now.Add(time.Hour))
				*f.now = f.now.Add(30 * time.Minute)
			case "restart":
				db, err := sqlite.Open(f.db.Path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				r.Repository = sqlite.NewRepository(db).WithReviewNoChangeCandidate()
			case "epoch-reset":
				if err := f.db.ResetFinancialSourceTracking(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if statement != "" {
				if _, err := f.db.SQL.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			_, ok, err := r.ConfirmReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.FXProviderKey)
			if err != nil || ok {
				t.Fatalf("unsafe reuse %s: %v %v", scenario, ok, err)
			}
			switch scenario {
			case "source", "config", "state", "rule-without-revision", "episode-without-state", "pending", "epoch-reset", "provider":
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				noChangeFullOracle(t, f)
			}
		})
	}
}

func TestReviewNoChangeCrashChild(t *testing.T) {
	if os.Getenv("NW_C3_CRASH_CHILD") != "1" {
		t.Skip("subprocess crash helper")
	}
	path := os.Getenv("NW_C3_CRASH_DB")
	if filepath.Base(path) != "synthetic-c3-crash.db" {
		t.Fatal("synthetic isolated path required")
	}
	if err := modernsqlite.RegisterScalarFunction("synthetic_c3_crash", 0, func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		os.Exit(93)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now, err := time.Parse(time.RFC3339Nano, os.Getenv("NW_C3_CRASH_AT"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(sqlite.NewRepository(db).WithReviewNoChangeCandidate())
	s.setClock(func() time.Time { return now })
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Nanosecond)
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Fatal("no-change update crash was not reached")
}

func TestReviewNoChangeCrashBeforeCommitAndRecovery(t *testing.T) {
	f, _ := noChangeFixture(t)
	path := filepath.Join(t.TempDir(), "synthetic-c3-crash.db")
	if _, err := f.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`CREATE TRIGGER synthetic_c3_crash AFTER UPDATE ON review_temporal_checkpoints WHEN NEW.evaluated_at<>OLD.evaluated_at BEGIN SELECT synthetic_c3_crash(); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestReviewNoChangeCrashChild$", "-test.v")
	child.Env = append(os.Environ(), "NW_C3_CRASH_CHILD=1", "NW_C3_CRASH_DB="+path, "NW_C3_CRASH_AT="+f.now.Format(time.RFC3339Nano))
	output, err := child.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 93 {
		t.Fatalf("expected crash93: %v\n%s", err, output)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var evaluated string
	if err := db.SQL.QueryRow(`SELECT evaluated_at FROM review_temporal_checkpoints`).Scan(&evaluated); err != nil || evaluated != f.now.Format(time.RFC3339Nano) || !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, db)) {
		t.Fatal("crashed confirmation partially committed", err)
	}
	if _, err := db.SQL.Exec(`DROP TRIGGER synthetic_c3_crash`); err != nil {
		t.Fatal(err)
	}
	f.db = db
	f.s, _ = noChangeService(f)
	*f.now = f.now.Add(time.Nanosecond)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	noChangeFullOracle(t, f)
}

func TestReviewNoChangeActivationLifecycleFullOracle(t *testing.T) {
	for _, scenario := range []string{"exit", "entry", "clear-rebuy", "transfer"} {
		t.Run(scenario, func(t *testing.T) {
			quantity := "1"
			if scenario == "entry" {
				quantity = "0"
			}
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", quantity)
			f.day(t, "2026-10-03")
			saveSyntheticReviewRule(t, f, "Synthetic no-change activation", syntheticHistoricalReviewSpec())
			at := f.now.Add(time.Hour)
			var destination domain.Holding
			if scenario == "transfer" {
				var owner domain.MemberID
				if err := f.db.SQL.QueryRow(`SELECT member_id FROM account_ownership WHERE account_id=?`, f.account.Account.ID.String()).Scan(&owner); err != nil {
					t.Fatal(err)
				}
				account := overviewAccount(t, f.s, owner, "Synthetic no-change receiver", "brokerage", "asset", "holdings", "CNY", "")
				var err error
				destination, err = f.s.CreateHolding(t.Context(), HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: f.instrument.ID.String(), Quantity: "0"})
				if err != nil {
					t.Fatal(err)
				}
			}
			*f.now = at
			if scenario == "transfer" {
				if _, err := f.s.RecordChange(t.Context(), domain.PositionTransferInput{HouseholdID: f.account.Account.HouseholdID, FromHoldingID: f.holding.ID, ToHoldingID: destination.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: at}); err != nil {
					t.Fatal(err)
				}
			} else {
				f.quantity(t, scenario == "entry")
			}
			if scenario == "clear-rebuy" {
				*f.now = at.Add(time.Hour)
				f.quantity(t, true)
			}
			*f.now = at.Add(-time.Nanosecond)
			f.s, _ = noChangeService(f)
			r := f.s.repository.(*noChangeTestRepository)
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			var episode string
			_ = f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&episode)
			if episode != "" {
				if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 1); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			before := temporalCapture(t, f)
			captures := r.captures
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil || r.captures != captures {
				t.Fatal("T-1ns did not confirm no-change", err)
			}
			noChangeFullOracle(t, f)
			points := []time.Time{at, at.Add(time.Nanosecond)}
			if scenario == "clear-rebuy" {
				points = append(points, at.Add(time.Hour), at.Add(time.Hour+time.Nanosecond))
			}
			for _, now := range points {
				*f.now = now
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				current := temporalCapture(t, f)
				if current.SourceRevision.Token != before.SourceRevision.Token || current.Control.PublishedOn != before.Control.PublishedOn {
					t.Fatal("activation test changed financial source/closed vector")
				}
				noChangeFullOracle(t, f)
			}
			if r.captures <= captures {
				t.Fatal("activation never took full fallback")
			}
		})
	}
}

func TestReviewNoChangeCASFailureAndBoundedRetry(t *testing.T) {
	for _, scenario := range []string{"control-ignore", "checkpoint-ignore", "checkpoint-abort"} {
		t.Run(scenario, func(t *testing.T) {
			f, _ := noChangeFixture(t)
			ddl := `CREATE TRIGGER synthetic_no_change_failure BEFORE UPDATE ON investment_review_control BEGIN SELECT RAISE(IGNORE); END`
			if scenario != "control-ignore" {
				action := "IGNORE"
				if scenario == "checkpoint-abort" {
					action = "ABORT,'synthetic checkpoint failure'"
				}
				ddl = `CREATE TRIGGER synthetic_no_change_failure BEFORE UPDATE ON review_temporal_checkpoints WHEN NEW.evaluated_at<>OLD.evaluated_at BEGIN SELECT RAISE(` + action + `); END`
			}
			if _, err := f.db.SQL.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			// Install before verified open. The first full fallback refreshes
			// session/proof at the same timestamp and does not hit the fault.
			db, err := sqlite.Open(f.db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			f.db = db
			f.s, _ = noChangeService(f)
			r := f.s.repository.(*noChangeTestRepository)
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			before := noChangeProjectionRows(t, f.db)
			var certificate string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&certificate); err != nil {
				t.Fatal(err)
			}
			*f.now = f.now.Add(time.Nanosecond)
			_, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err == nil || r.captures != 1 || r.publications != 1 {
				t.Fatal("failed CAS was accepted or recomputed", err, r)
			}
			if scenario != "checkpoint-abort" && r.confirmations != 4 {
				t.Fatal("confirmation conflict retry was not bounded at three", r.confirmations)
			}
			var after string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil || certificate != after || !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("failed CAS partially committed", err)
			}
		})
	}
}

func TestReviewNoChangeFinalBoundaryCancellationAndWriterReservation(t *testing.T) {
	for _, scenario := range []string{"boundary", "cancel", "writer"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := noChangeFixture(t)
			var checkpoint string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&checkpoint); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writer, err := sql.Open("sqlite", f.db.Path+"?_txlock=immediate&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1&_pragma=busy_timeout%3d0")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			_, ok, err := r.ConfirmReviewNoChange(ctx, f.account.Account.HouseholdID, *f.now, func() time.Time {
				switch scenario {
				case "boundary":
					return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
				case "cancel":
					cancel()
				case "writer":
					if _, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='101'`); err == nil || !strings.Contains(err.Error(), "locked") {
						t.Fatalf("second writer was not excluded by BEGIN IMMEDIATE: %v", err)
					}
				}
				return f.now.Add(time.Nanosecond)
			}, f.s.FXProviderKey)
			if scenario == "writer" {
				if err != nil || !ok {
					t.Fatal(ok, err)
				}
				if _, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='101'`); err != nil {
					t.Fatal("writer failed after confirmation commit", err)
				}
				*f.now = f.now.Add(time.Nanosecond)
				if _, ok, err := r.ConfirmReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.FXProviderKey); err != nil || ok {
					t.Fatal("post-confirmation source edit reused proof", ok, err)
				}
				return
			}
			if ok || err == nil {
				t.Fatal("failed confirmation reported success", ok, err)
			}
			var after string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil || after != checkpoint {
				t.Fatal("rejected confirmation partially updated checkpoint", err)
			}
		})
	}
}
