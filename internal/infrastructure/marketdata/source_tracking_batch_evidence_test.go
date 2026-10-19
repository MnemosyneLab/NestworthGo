package marketdata

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Opt-in complete repository persistence calls, including validation, binding,
// quote/slot/coverage UPSERTs, dirty generation and Commit. No live provider.
func TestSourceTrackingProviderImportBatchEvidence(t *testing.T) {
	if os.Getenv("NW_SOURCE_BATCH_PERF") != "1" {
		t.Skip("synthetic end-to-end batch evidence; run alone")
	}
	db, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	fetched := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	request := instrumentCommit(household, instrument, outcome, fetched)
	template := request.Observations[0]
	request.PendingRanges = nil
	request.VerifiedRanges = nil
	request.NextCheckAt = nil
	start := time.Date(2007, 1, 1, 17, 0, 0, 0, time.UTC)
	for page := 0; page < 3; page++ {
		request.Observations = nil
		for i := 0; i < 2304; i++ {
			observation := template
			at := start.AddDate(0, 0, page*2304+i)
			observation.MarketDate = at.Format(time.DateOnly)
			observation.ValueEffectiveAt = at
			observation.ProviderTimestamp = at
			observation.Value = "100.0001"
			request.Observations = append(request.Observations, observation)
		}
		for repeat := 0; repeat < 2; repeat++ {
			label := fmt.Sprintf("history-import-page-%d-repeat-%d", page, repeat)
			if err = db.CheckpointWAL(t.Context()); err != nil {
				t.Fatal(err)
			}
			var beforeChanges, beforeRevision int64
			if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&beforeChanges); err != nil {
				t.Fatal(err)
			}
			if err = db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&beforeRevision); err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			beforeBatch, err := repo.LoadReviewHistoricalBatch(t.Context(), household.ID, fetched)
			if err != nil {
				t.Fatal(err)
			}
			beforeFingerprint, err := domain.ReviewHistoricalFingerprint(beforeBatch)
			if err != nil {
				t.Fatal(err)
			}
			began := time.Now()
			result, err := repo.CommitInstrumentHistory(t.Context(), request)
			elapsed := time.Since(began)
			if err != nil {
				t.Fatal(err)
			}
			var changes, revision, floor, rows, textBytes, quotes, slots int64
			if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changes); err != nil {
				t.Fatal(err)
			}
			if err = db.SQL.QueryRow(`SELECT revision,journal_floor,(SELECT count(*) FROM financial_source_changes),(SELECT coalesce(sum(length(source_table)+length(source_id)+length(operation)+coalesce(length(old_scope_json),0)+coalesce(length(new_scope_json),0)+coalesce(length(old_effective_at),0)+coalesce(length(new_effective_at),0)),0) FROM financial_source_changes) FROM financial_source_revision`).Scan(&revision, &floor, &rows, &textBytes); err != nil {
				t.Fatal(err)
			}
			if err = db.SQL.QueryRow(`SELECT count(*) FROM instrument_quotes`).Scan(&quotes); err != nil {
				t.Fatal(err)
			}
			if err = db.SQL.QueryRow(`SELECT count(*) FROM instrument_observation_slots`).Scan(&slots); err != nil {
				t.Fatal(err)
			}
			wal := int64(0)
			if info, e := os.Stat(db.Path + "-wal"); e == nil {
				wal = info.Size()
			}
			if err = db.CheckpointWAL(t.Context()); err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			batch, err := repo.LoadReviewHistoricalBatch(t.Context(), household.ID, fetched)
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := domain.ReviewHistoricalFingerprint(batch)
			if err != nil {
				t.Fatal(err)
			}
			if rows != revision-floor || rows > 4351 || quotes != int64((page+1)*2304) || slots != quotes {
				t.Fatalf("unbounded/inconsistent batch: revision=%d floor=%d rows=%d quotes=%d slots=%d", revision, floor, rows, quotes, slots)
			}
			if repeat == 0 && (result.PersistedObservations != 2304 || revision == beforeRevision) {
				t.Fatalf("import not persisted: %+v", result)
			}
			if repeat == 1 && (!result.Unchanged || result.NewRevisions != 0 || revision != beforeRevision || fingerprint != beforeFingerprint) {
				t.Fatalf("unchanged real UPSERTs mutated authoritative facts/tracking: %+v revision %d->%d", result, beforeRevision, revision)
			}
			if err = db.Verify(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Logf("batch=%s sample=single elapsed_ns=%d observations=2304 totalChangedRows=%d revisionDelta=%d revision=%d floor=%d retainedRows=%d retainedTextBytes=%d quotes=%d slots=%d WALBeforeCheckpoint=%d DBBefore=%d DBAfter=%d DBGrowth=%d result=%+v", label, elapsed.Nanoseconds(), changes-beforeChanges, revision-beforeRevision, revision, floor, rows, textBytes, quotes, slots, wal, beforeInfo.Size(), afterInfo.Size(), afterInfo.Size()-beforeInfo.Size(), result)
		}
	}
}
