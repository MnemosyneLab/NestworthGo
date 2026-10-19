package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestSourceJournalShadowPreservesFingerprintAndPublication(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	saveSyntheticReviewRule(t, f, "Synthetic source shadow", syntheticHistoricalReviewSpec())
	f.day(t, "2026-10-03")
	repo := sqlite.NewRepository(f.db)
	before, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	var revision, afterRevision int64
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	// No existing generation helper is called. The database trigger must see
	// this in-place scalar correction while the full fingerprint remains king.
	if _, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='100.0001' WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	corrected, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&afterRevision); err != nil || afterRevision != revision+1 {
		t.Fatalf("correction not tracked: %d -> %d %v", revision, afterRevision, err)
	}
	if before.HistoricalFingerprint == corrected.HistoricalFingerprint || before.Historical.History.InputGeneration != corrected.Historical.History.InputGeneration {
		t.Fatal("scalar correction bypassed full-source fingerprint/generation control")
	}
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&revision); err != nil || revision != afterRevision {
		t.Fatal("review publication fed back into financial source tracking")
	}
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&revision); err != nil || revision != afterRevision {
		t.Fatal("repeat publication changed financial source revision")
	}
}
func TestSourceJournalEqualityDoesNotCertifyFutureActivation(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	at := f.now.Add(time.Hour)
	*f.now = at
	// Persist through the public command at its effective instant, then inspect
	// the already-persisted future event at T-1ns and T. No DML occurs between.
	f.quantity(t, false)
	repo := sqlite.NewRepository(f.db)
	id := f.account.Account.HouseholdID
	before, err := repo.CaptureReviewProjection(t.Context(), id, at.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	var revision, afterRevision int64
	var epoch, afterEpoch string
	if err = f.db.SQL.QueryRow(`SELECT revision,tracking_epoch FROM financial_source_revision`).Scan(&revision, &epoch); err != nil {
		t.Fatal(err)
	}
	after, err := repo.CaptureReviewProjection(t.Context(), id, at)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.SQL.QueryRow(`SELECT revision,tracking_epoch FROM financial_source_revision`).Scan(&afterRevision, &afterEpoch); err != nil || revision != afterRevision || epoch != afterEpoch {
		t.Fatal("time-only read mutated tracking")
	}
	if before.HistoricalFingerprint == after.HistoricalFingerprint {
		t.Fatal("full fingerprint ignored activation")
	}
	oldPosition, err := ProjectReviewPositionTimeline(t.Context(), before.Historical.History, f.instrument.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	newPosition, err := ProjectReviewPositionTimeline(t.Context(), after.Historical.History, f.instrument.ID, at.Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if oldPosition.Quantity != "1" || newPosition.Quantity != "0" {
		t.Fatalf("activation position: before=%+v after=%+v", oldPosition, newPosition)
	}
	oldDay, _ := domain.ReviewLatestClosedDay(at.Add(-time.Nanosecond), "UTC")
	newDay, _ := domain.ReviewLatestClosedDay(at, "UTC")
	if oldDay != newDay {
		t.Fatal("fixture crossed a closed-day frontier")
	}
}

func TestSourceJournalTracksReplayLatestValuesAndAgentWithdrawal(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	repo := sqlite.NewRepository(f.db)
	household := f.account.Account.HouseholdID
	capture := func() string {
		t.Helper()
		c, err := repo.CaptureReviewProjection(t.Context(), household, *f.now)
		if err != nil {
			t.Fatal(err)
		}
		return c.HistoricalFingerprint
	}
	revision := func() int64 {
		t.Helper()
		var n int64
		if err := f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"account_values", "account_cash_values"} {
		t.Run(table, func(t *testing.T) {
			before, n := capture(), revision()
			sql := `INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,projection_kind) VALUES(?,?,'1.0001','CNY','2026-10-03T23:59:00Z','2026-10-03T23:59:00Z','replay')`
			if table == "account_values" {
				sql = `INSERT INTO account_values(id,account_id,value_kind,amount,currency,effective_at,created_at,projection_kind) VALUES(?,?,'balance','1.0001','CNY','2026-10-03T23:59:00Z','2026-10-03T23:59:00Z','replay')`
			}
			if _, err := f.db.SQL.Exec(sql, string(domain.NewAccountValueID()), f.account.Account.ID.String()); err != nil {
				t.Fatal(err)
			}
			if capture() == before || revision() != n+1 {
				t.Fatal("latest-value replay fact changed fingerprint without source revision")
			}
		})
	}
	var quoteID string
	if err := f.db.SQL.QueryRow(`SELECT id FROM instrument_quotes WHERE instrument_id=? LIMIT 1`, f.instrument.ID.String()).Scan(&quoteID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO agent_quote_batches(request_key,household_id,payload_hash,created_at) VALUES('synthetic-withdrawal',?,'synthetic','2026-10-03T00:00:00Z')`, household.String()); err != nil {
		t.Fatal(err)
	}
	before, n := capture(), revision()
	if _, err := f.db.SQL.Exec(`INSERT INTO agent_quote_records(id,request_key,household_id,target_type,operation,target_quote_id,source_title,source_url,created_at) VALUES(?,'synthetic-withdrawal',?,'instrument','retract',?,'Synthetic','','2026-10-03T00:00:00Z')`, string(domain.NewAccountValueID()), household.String(), quoteID); err != nil {
		t.Fatal(err)
	}
	if capture() == before || revision() != n+1 {
		t.Fatal("withdrawal changed fingerprint without source revision")
	}
	if _, err := f.db.SQL.Exec(`DELETE FROM agent_quote_records WHERE request_key='synthetic-withdrawal'`); err != nil {
		t.Fatal(err)
	}
	if capture() != before || revision() != n+2 {
		t.Fatal("withdrawal deletion not tracked or failed to restore facts")
	}
}
