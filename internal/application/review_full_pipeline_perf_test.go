//go:build perf

package application

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// TestReviewFullPipelinePerformanceEvidence is an opt-in end-to-end budget
// measurement. It uses a temporary SQLite database populated only with
// synthetic accounts, positions, rules, quote history, FX references, and
// activity rows. The perf build tag is the gate.
func TestReviewFullPipelinePerformanceEvidence(t *testing.T) {
	defer logReviewPerfProcessMemory(t)
	samples := reviewPerfSampleCount(t, 10)
	mode := os.Getenv("NW_REVIEW_PERF_MODE")
	if err := validateReviewPerfMode(mode); err != nil {
		t.Fatal(err)
	}
	historyDays := 365
	if mode == "warm" || mode == "stages" || mode == "warm-marketdata" || mode == "warm-correction" || mode == "stages-marketdata" || mode == "stages-correction" || mode == "profile" {
		historyDays = 30
	}
	fixture := newReviewFullPipelineFixture(t, historyDays)
	ctx := context.Background()

	started := time.Now()
	result, err := fixture.service.ReconcileInvestmentReviewsV2(ctx)
	cold := time.Since(started)
	if err != nil {
		t.Fatalf("cold reconcile: %v", err)
	}
	if result.RuleInstrumentCount != 2_000 {
		t.Fatalf("cold publication covered %d rule/instrument pairs, want 2000", result.RuleInstrumentCount)
	}
	t.Logf("fixture cold publish complete: historyDays=%d elapsed=%s", historyDays, cold)
	if mode == "cold" {
		logReviewFullPipelineSourceCounts(t, fixture, historyDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}
	metrics := enableReviewSQLMetrics(t, fixture)
	if mode == "profile" || mode == "profile365" {
		profileReviewReusableEvidence(t, fixture, metrics)
		return
	}
	if mode == "stages" || mode == "stages365" {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		repository := reviewPerfRepository(t, fixture.db)
		metrics.reset()
		started = time.Now()
		capture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		captureElapsed := time.Since(started)
		captureSQL := metrics.snapshot(8)
		if err != nil {
			t.Fatal(err)
		}
		started = time.Now()
		capturedFingerprint, err := domain.ReviewHistoricalFingerprint(capture.Historical)
		fingerprintElapsed := time.Since(started)
		if err != nil || capturedFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("fingerprint recheck in memory differs: %v", err)
		}
		started = time.Now()
		publication, err := PrepareReviewPublication(ctx, capture, *fixture.now, fixture.service.FXProviderKey())
		prepareElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		plannedDays := 0
		for _, item := range publication.Rules {
			plannedDays += len(item.Days)
		}
		started = time.Now()
		var inputBytes, resultBytes int64
		for _, item := range publication.Rules {
			for _, day := range item.Days {
				inputJSON, err := json.Marshal(day.Input)
				if err != nil {
					t.Fatal(err)
				}
				coverageJSON, err := json.Marshal(day.CoverageMissing)
				if err != nil {
					t.Fatal(err)
				}
				resultJSON, err := json.Marshal(day.Result)
				if err != nil {
					t.Fatal(err)
				}
				inputBytes += int64(len(inputJSON) + len(coverageJSON))
				resultBytes += int64(len(resultJSON))
			}
		}
		serializationElapsed := time.Since(started)
		metrics.reset()
		started = time.Now()
		recheck, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		recheckElapsed := time.Since(started)
		if err != nil || recheck.HistoricalFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("read-only publication recheck differs: %v", err)
		}
		recheckSQL := metrics.snapshot(8)
		metrics.reset()
		started = time.Now()
		if err := repository.PublishReviewProjection(ctx, publication, *fixture.now); err != nil {
			t.Fatalf("publish staged warm increment: %v", err)
		}
		publishElapsed := time.Since(started)
		publishSQL := metrics.snapshot(8)
		stagedBytes := inputBytes + resultBytes
		t.Logf("one warm increment phases: capture+source fingerprint=%s (%s; top SQL: %s; positionActivitiesCaptured=%d); in-memory source fingerprint=%s; planning+daily fingerprints=%s; staged JSON serialization=%s (input+coverage=%d bytes result=%d bytes total=%d); separate read-only recheck=%s (%s; top SQL: %s); atomic publish recheck+serialization+writes=%s (%s; top SQL: %s); staged ruleInstrumentPairs=%d newDays=%d", captureElapsed, captureSQL.summary(), captureSQL.slowest(), len(capture.Historical.History.Activities), fingerprintElapsed, prepareElapsed, serializationElapsed, inputBytes, resultBytes, stagedBytes, recheckElapsed, recheckSQL.summary(), recheckSQL.slowest(), publishElapsed, publishSQL.summary(), publishSQL.slowest(), len(publication.Rules), plannedDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}
	if mode == "stages-marketdata" || mode == "stages-correction" {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		if mode == "stages-marketdata" {
			if err := appendReviewPerfDailyMarketData(ctx, fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
				t.Fatalf("append synthetic daily quote and FX refresh: %v", err)
			}
		} else if err := correctReviewPerfHistoricalQuote(ctx, fixture, 0); err != nil {
			t.Fatalf("correct synthetic historical quote: %v", err)
		}
		repository := reviewPerfRepository(t, fixture.db)
		metrics.reset()
		started = time.Now()
		capture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		captureElapsed := time.Since(started)
		captureSQL := metrics.snapshot(8)
		if err != nil {
			t.Fatal(err)
		}
		if capture.HistoricalFingerprint == capture.Control.PublishedFingerprint {
			t.Fatalf("source change did not invalidate the published fingerprint: %s", mode)
		}
		if len(capture.ReusableDailyEvidence) != 0 {
			t.Fatalf("source change unexpectedly reused %d daily evidence rows: %s", len(capture.ReusableDailyEvidence), mode)
		}
		started = time.Now()
		publication, err := PrepareReviewPublication(ctx, capture, *fixture.now, fixture.service.FXProviderKey())
		prepareElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		metrics.reset()
		started = time.Now()
		if err := repository.PublishReviewProjection(ctx, publication, *fixture.now); err != nil {
			t.Fatal(err)
		}
		publishElapsed := time.Since(started)
		publishSQL := metrics.snapshot(8)
		var quoteBytes, resultBytes int64
		for _, item := range publication.Rules {
			for _, day := range item.Days {
				inputJSON, _ := json.Marshal(day.Input)
				coverageJSON, _ := json.Marshal(day.CoverageMissing)
				resultJSON, _ := json.Marshal(day.Result)
				quoteBytes += int64(len(inputJSON) + len(coverageJSON))
				resultBytes += int64(len(resultJSON))
			}
		}
		scenario := "historical quote correction (one existing instrument/day fact updated)"
		if mode == "stages-marketdata" {
			scenario = "daily market refresh (100 new instrument quotes + 4 FX references)"
		}
		t.Logf("one warm %s: capture+source fingerprint=%s (%s; top SQL: %s); planning+daily fingerprints=%s; staged JSON bytes input+coverage=%d result=%d total=%d; atomic publish recheck+serialization+writes=%s (%s; top SQL: %s); ruleInstrumentPairs=%d newDays=%d", scenario, captureElapsed, captureSQL.summary(), captureSQL.slowest(), prepareElapsed, quoteBytes, resultBytes, quoteBytes+resultBytes, publishElapsed, publishSQL.summary(), publishSQL.slowest(), len(publication.Rules), countReviewPublicationDays(publication))
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}

	incremental := make([]time.Duration, samples)
	for index := range incremental {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		if mode == "warm-marketdata" {
			if err := appendReviewPerfDailyMarketData(ctx, fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
				t.Fatalf("append synthetic daily quote and FX refresh %d: %v", index+1, err)
			}
		} else if mode == "warm-correction" {
			if err := correctReviewPerfHistoricalQuote(ctx, fixture, index); err != nil {
				t.Fatalf("correct synthetic historical quote %d: %v", index+1, err)
			}
		}
		if index == 0 {
			metrics.reset()
		}
		started = time.Now()
		result, err = fixture.service.ReconcileInvestmentReviewsV2(ctx)
		incremental[index] = time.Since(started)
		if err != nil {
			t.Fatalf("incremental reconcile %d: %v", index+1, err)
		}
		if result.RuleInstrumentCount != 2_000 {
			t.Fatalf("incremental publication %d covered %d rule/instrument pairs, want 2000", index+1, result.RuleInstrumentCount)
		}
		if index == 0 {
			sqlSnapshot := metrics.snapshot(8)
			t.Logf("first warm full reconcile SQL: %s; top SQL: %s", sqlSnapshot.summary(), sqlSnapshot.slowest())
		}
	}

	pageReads := make([]time.Duration, samples)
	for index := range pageReads {
		if index == 0 {
			metrics.reset()
		}
		started = time.Now()
		page, err := fixture.service.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "pending"}, "", 50)
		pageReads[index] = time.Since(started)
		if err != nil {
			t.Fatalf("first page read %d: %v", index+1, err)
		}
		if page.ProjectionStatus != "complete" || len(page.Items) == 0 {
			t.Fatalf("first page read returned status=%q items=%d", page.ProjectionStatus, len(page.Items))
		}
		if index == 0 {
			sqlSnapshot := metrics.snapshot(8)
			t.Logf("first alert page SQL: %s; top SQL: %s", sqlSnapshot.summary(), sqlSnapshot.slowest())
		}
	}

	quoteCount, activityCount, fxCount := 0, 0, 0
	for _, query := range []struct {
		sql  string
		into *int
	}{{`SELECT count(*) FROM instrument_quotes`, &quoteCount}, {`SELECT count(*) FROM activities`, &activityCount}, {`SELECT count(*) FROM fx_quotes`, &fxCount}} {
		if err := fixture.db.SQL.QueryRowContext(ctx, query.sql).Scan(query.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("fixture: holdings=100 rules=20 ruleInstrumentPairs=2000 instrumentQuotes=%d activities=%d fxReferences=%d historyDays=%d", quoteCount, activityCount, fxCount, historyDays)
	t.Logf("cold full reconcile n=1: %s (budget <=3s)", cold)
	if mode == "warm-marketdata" {
		t.Logf("warm incremental full reconcile with daily quote+FX additions n=%d: median=%s p95=%s max=%s (market data adds 100 quote + 4 FX rows per sample)", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	} else if mode == "warm-correction" {
		t.Logf("warm incremental full reconcile with one historical quote correction per sample n=%d: median=%s p95=%s max=%s", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	} else {
		t.Logf("warm incremental full reconcile n=%d: median=%s p95=%s max=%s (budget p95 <=500ms)", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	}
	t.Logf("published alert first page n=%d: median=%s p95=%s max=%s (budget p95 <=150ms)", len(pageReads), durationPercentile(pageReads, 0.50), durationPercentile(pageReads, 0.95), durationPercentile(pageReads, 1))
	logReviewFullPipelineStorage(t, fixture, historyDays)
}
