package application

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Install before the first certified connection registers its commit hook.
// No user trigger is involved: SQLite's FK action is the hidden writer.
func TestReviewCertifiedPreCaptureForeignKeyCascadeCannotCertify(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-12")
	saveSyntheticReviewRule(t, f, "Synthetic FK boundary", syntheticHistoricalReviewSpec())
	var cookie int
	var definition string
	if err := f.db.SQL.QueryRow(`PRAGMA schema_version`).Scan(&cookie); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT sql FROM sqlite_master WHERE name='investment_review_episodes'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	rows, err := f.db.SQL.Query(`SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='investment_review_episodes' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	indexes := []string{}
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, statement)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	end := strings.LastIndex(definition, ")")
	definition = definition[:end] + ", FOREIGN KEY(first_detected_at) REFERENCES review_temporal_checkpoints(evaluated_at) ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED" + definition[end:]
	statements := append([]string{
		`DROP TABLE investment_review_episodes`, definition,
		`CREATE UNIQUE INDEX synthetic_checkpoint_time ON review_temporal_checkpoints(evaluated_at)`,
	}, indexes...)
	statements = append(statements, fmt.Sprintf(`PRAGMA schema_version=%d`, cookie))
	for _, statement := range statements {
		if _, err := f.db.SQL.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	r := sqlite.NewRepository(f.db).WithDefaultReviewOptimizations()
	f.s = NewService(r)
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var detected string
	if err := f.db.SQL.QueryRow(`SELECT first_detected_at FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&detected); err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	before := noChangeProjectionRows(t, f.db)
	_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	var after string
	if err := f.db.SQL.QueryRow(`SELECT first_detected_at FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if detected != after {
		t.Errorf("checkpoint FK action rewrote frozen detection time: %s -> %s (confirmation=%v)", detected, after, ok)
	}
	if ok {
		t.Error("modified FK contract authorized no-op")
	}
	if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
		t.Error("rejected confirmation changed projection rows")
	}
	assertNoCertifiedMAC(t, f)
}
