package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func temporalCapture(t *testing.T, f seriesZeroFixture) domain.ReviewProjectionCapture {
	t.Helper()
	c, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func temporalPublicationOracle(t *testing.T, f seriesZeroFixture, capture domain.ReviewProjectionCapture) domain.ReviewPublication {
	t.Helper()
	candidate, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	full, err := fullReviewOracle(sqlite.NewRepository(f.db)).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.ReusableDailyEvidence) != 0 {
		t.Fatal("oracle reused evidence")
	}
	oracle, err := PrepareReviewPublication(t.Context(), full, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := compareReviewFullOracle(candidate, oracle, f.s.FXProviderKey()); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func temporalConflict(t *testing.T, err error) {
	t.Helper()
	var conflict *domain.Error
	if !errors.As(err, &conflict) || conflict.Code != domain.ErrConflict {
		t.Fatalf("wanted whole-stage conflict: %v", err)
	}
}

func TestReviewTemporalQuantityCapturePreservesStoredInstantPrecision(t *testing.T) {
	for _, raw := range []string{"2026-10-03T13:00:00Z", "2026-10-03T13:00:00.123456789Z", "2026-10-03T15:30:00+02:30"} {
		t.Run(raw, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
			at, _ := time.Parse(time.RFC3339Nano, raw)
			*f.now = at
			f.quantity(t, false)
			result, err := f.db.SQL.Exec(`UPDATE activities SET effective_at=? WHERE id IN (SELECT activity_id FROM activity_effects WHERE holding_id=? AND target='holding_quantity')`, raw, f.holding.ID.String())
			if err != nil {
				t.Fatal(err)
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				t.Fatal("precision fixture omitted quantity event")
			}
			var token domain.SourceRevisionToken
			for i, now := range []time.Time{at.Add(-time.Nanosecond), at, at.Add(time.Nanosecond)} {
				*f.now = now
				capture := temporalCapture(t, f)
				if i == 0 {
					token = capture.SourceRevision.Token
				} else if token != capture.SourceRevision.Token {
					t.Fatal("clock precision changed source token")
				}
				position, err := ProjectReviewPositionTimeline(t.Context(), capture.Historical.History, f.instrument.ID, now.Add(time.Nanosecond))
				want := "0"
				if i == 0 {
					want = "1"
				}
				if err != nil || !position.Known || position.Quantity != want {
					t.Fatalf("stored %s cutoff %s: %+v %v", raw, now, position, err)
				}
			}
		})
	}
}

func TestReviewTemporalMixedPrecisionRetainsEconomicReplayOrder(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "0")
	first := time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC)
	*f.now = first
	f.quantity(t, true)
	*f.now = first.Add(time.Millisecond)
	f.quantity(t, false)
	// SQL text order places the fractional timestamp before the whole second.
	// Replay must instead apply the positive crossing before its clear event.
	if _, err := f.db.SQL.Exec(`UPDATE activities SET effective_at=CASE WHEN id IN (SELECT activity_id FROM activity_effects WHERE direction='added' AND target='holding_quantity') THEN '2026-10-03T13:00:00Z' ELSE '2026-10-03T13:00:00.000000001Z' END WHERE id IN (SELECT activity_id FROM activity_effects WHERE target='holding_quantity')`); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-04")
	saveSyntheticReviewRule(t, f, "Synthetic precise replay", syntheticHistoricalReviewSpec())
	capture := temporalCapture(t, f)
	if len(capture.Historical.History.Activities) != 2 || !capture.Historical.History.Activities[0].EffectiveAt.Before(capture.Historical.History.Activities[1].EffectiveAt) {
		t.Fatal("quantity replay retained timestamp text order")
	}
	temporalPublicationOracle(t, f, capture)
}

func TestReviewTemporalPreexistingFutureQuantityLifecycleOracle(t *testing.T) {
	for _, scenario := range []string{"exit", "entry", "clear-rebuy", "transfer"} {
		t.Run(scenario, func(t *testing.T) {
			quantity := "1"
			if scenario == "entry" {
				quantity = "0"
			}
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", quantity)
			f.day(t, "2026-10-03")
			at := f.now.Add(time.Hour)
			saveSyntheticReviewRule(t, f, "Synthetic temporal lifecycle", syntheticHistoricalReviewSpec())
			var destination domain.Holding
			if scenario == "transfer" {
				var owner domain.MemberID
				if err := f.db.SQL.QueryRow(`SELECT member_id FROM account_ownership WHERE account_id=?`, f.account.Account.ID.String()).Scan(&owner); err != nil {
					t.Fatal(err)
				}
				other := overviewAccount(t, f.s, owner, "Synthetic temporal receiving account", "brokerage", "asset", "holdings", "CNY", "")
				var err error
				destination, err = f.s.CreateHolding(t.Context(), HoldingInput{AccountID: other.Account.ID.String(), InstrumentID: f.instrument.ID.String(), Quantity: "0"})
				if err != nil {
					t.Fatal(err)
				}
			}
			*f.now = at
			switch scenario {
			case "entry":
				f.quantity(t, true)
			case "exit", "clear-rebuy":
				f.quantity(t, false)
			case "transfer":
				if _, err := f.s.RecordChange(t.Context(), domain.PositionTransferInput{HouseholdID: f.account.Account.HouseholdID, FromHoldingID: f.holding.ID, ToHoldingID: destination.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: at}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "clear-rebuy" {
				*f.now = at.Add(time.Hour)
				f.quantity(t, true)
			}
			// All financial DML is complete before the first capture. From here
			// only review publication/handling may write; source revision is fixed.
			*f.now = at.Add(-time.Nanosecond)
			before := temporalCapture(t, f)
			if before.Temporal == nil || !before.Temporal.NextTransitionAt.Equal(at) {
				t.Fatal("future event omitted from uncut frontier")
			}
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			var oldEpisode string
			_ = f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&oldEpisode)
			if oldEpisode != "" {
				if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), oldEpisode, string(domain.NewMutationID()), 1); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			stable := temporalCapture(t, f)
			if !stable.PublishedTemporal.Eligible(*f.now, stable.Temporal) {
				t.Fatal("boundary-before diagnostic certificate rejected")
			}
			closed, _ := domain.ReviewLatestClosedDay(*f.now, "UTC")
			points := []time.Time{at, at.Add(time.Nanosecond)}
			if scenario == "clear-rebuy" {
				points = append(points, at.Add(time.Hour).Add(-time.Nanosecond), at.Add(time.Hour), at.Add(time.Hour+time.Nanosecond))
			}
			for _, now := range points {
				*f.now = now
				capture := temporalCapture(t, f)
				if capture.SourceRevision.Token != before.SourceRevision.Token {
					t.Fatal("time-only activation changed source token")
				}
				day, _ := domain.ReviewLatestClosedDay(now, "UTC")
				if day != closed {
					t.Fatal("test crossed the closed-day frontier")
				}
				if !now.Before(at) && stable.PublishedTemporal.Eligible(now, capture.Temporal) {
					t.Fatal("expired future-event certificate reused")
				}
				stage := temporalPublicationOracle(t, f, capture)
				position, err := ProjectReviewPositionTimeline(t.Context(), capture.Historical.History, f.instrument.ID, now.Add(time.Nanosecond))
				if err != nil || !position.Known {
					t.Fatalf("unknown activation position: %+v %v", position, err)
				}
				want := "0"
				if scenario == "entry" || scenario == "transfer" || scenario == "clear-rebuy" && !now.Before(at.Add(time.Hour)) {
					want = "1"
				}
				if position.Quantity != want {
					t.Fatalf("position=%s want=%s at=%s", position.Quantity, want, now)
				}
				if err := sqlite.NewRepository(f.db).PublishReviewProjection(t.Context(), stage, now); err != nil {
					t.Fatal(err)
				}
			}
			// Reopen after activation without DML, then roll the original session
			// backwards across T and publish the full oracle with handling intact.
			path := filepath.Join(t.TempDir(), "synthetic-after-activation.db")
			if err := f.db.SnapshotTo(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			reopened, err := sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			run := f
			run.db = reopened
			run.s = NewService(sqlite.NewRepository(reopened))
			run.s.setClock(func() time.Time { return *run.now })
			fresh := temporalCapture(t, run)
			if !fresh.TemporalFullRecompute || fresh.PublishedTemporal.Eligible(*run.now, fresh.Temporal) || fresh.SourceRevision.Token != before.SourceRevision.Token {
				t.Fatal("post-activation restart reused old certificate")
			}
			temporalPublicationOracle(t, run, fresh)
			reopened.Close()
			*f.now = at.Add(-time.Nanosecond)
			back := temporalCapture(t, f)
			if !back.TemporalFullRecompute || len(back.ReusableDailyEvidence) != 0 || back.SourceRevision.Token != before.SourceRevision.Token {
				t.Fatal("rollback across activation reused old epoch evidence")
			}
			rollback := temporalPublicationOracle(t, f, back)
			if err := sqlite.NewRepository(f.db).PublishReviewProjection(t.Context(), rollback, *f.now); err != nil {
				t.Fatal(err)
			}
			if oldEpisode != "" {
				var ack string
				if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, oldEpisode).Scan(&ack); err != nil || ack == "" {
					t.Fatal("temporal replay lost acknowledgement history")
				}
			}
		})
	}
}

func TestReviewTemporalFinalClockRejectsActivationDuringFullRecheck(t *testing.T) {
	db, faults := openReviewFaultDatabase(t)
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1", db)
	f.day(t, "2026-10-03")
	at := f.now.Add(time.Hour)
	*f.now = at
	f.quantity(t, false)
	*f.now = at.Add(-time.Nanosecond)
	saveSyntheticReviewRule(t, f, "Synthetic publication boundary", syntheticHistoricalReviewSpec())
	comparisons := 0
	repo := &temporalRecheckFaultRepository{Repository: sqlite.NewRepository(f.db), faults: faults, afterRecheck: func() { comparisons++; *f.now = at }}
	capture := temporalCapture(t, f)
	stage := temporalPublicationOracle(t, f, capture)
	err := repo.PublishReviewProjectionWithClock(t.Context(), stage, func() time.Time {
		if comparisons == 0 {
			t.Fatal("clock sampled before full source comparison")
		}
		return *f.now
	})
	temporalConflict(t, err)
	for _, table := range []string{"investment_review_daily_results", "investment_review_episodes", "investment_review_episode_events", "review_temporal_checkpoints"} {
		var count int
		if err := f.db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rejected stage leaked %s: %d %v", table, count, err)
		}
	}
	// The service recaptures at the new clock; no source DML is needed.
	*f.now = at.Add(-time.Nanosecond)
	f.s = NewService(repo)
	f.s.setClock(func() time.Time { return *f.now })
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if comparisons != 3 {
		t.Fatalf("expected a rejected old stage followed by recapture: comparisons=%d", comparisons)
	}
	current := temporalCapture(t, f)
	temporalPublicationOracle(t, f, current)
	if current.SourceRevision.Token != capture.SourceRevision.Token {
		t.Fatal("recapture required a source mutation")
	}
}

func TestReviewTemporalFutureQuotesOriginAndHoldingFrontiers(t *testing.T) {
	for _, scenario := range []string{"instrument-quote", "fx-quote", "origin", "holding"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSeriesZeroFixture(t, scenario == "fx-quote", "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			at := f.now.Add(time.Hour)
			saveSyntheticReviewRule(t, f, "Synthetic future source", syntheticHistoricalReviewSpec())
			statement := map[string]string{"instrument-quote": `UPDATE instrument_quotes SET quoted_at=?`, "fx-quote": `UPDATE fx_quotes SET quoted_at=?`, "origin": `UPDATE history_origins SET started_at=?`, "holding": `UPDATE holdings SET created_at=?`}[scenario]
			if _, err := f.db.SQL.Exec(statement, at.Format("2006-01-02T15:04:05.000Z")); err != nil {
				t.Fatal(err)
			}
			*f.now = at.Add(-time.Nanosecond)
			before := temporalCapture(t, f)
			if !before.Temporal.NextTransitionAt.Equal(at) {
				t.Fatal("uncut future source omitted")
			}
			stage := temporalPublicationOracle(t, f, before)
			temporalConflict(t, sqlite.NewRepository(f.db).PublishReviewProjection(t.Context(), stage, at))
			for _, now := range []time.Time{at, at.Add(time.Nanosecond)} {
				*f.now = now
				current := temporalCapture(t, f)
				if current.SourceRevision.Token != before.SourceRevision.Token {
					t.Fatal("read/clock changed source revision")
				}
				temporalPublicationOracle(t, f, current)
			}
		})
	}
}

func TestReviewTemporalFixedDayDependencyClosureDoesNotFollowCurrentClock(t *testing.T) {
	// Every row is present before either capture. These effective instants are
	// selected against fixed closed-day cutoffs, not the moving current instant.
	for _, kind := range []string{"account-state-ownership", "instrument-state", "holding-state", "instrument-preference", "fx-preference", "provider-binding", "quote-provider-times", "coverage-expiry"} {
		t.Run(kind, func(t *testing.T) {
			f := newSeriesZeroFixture(t, true, "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			at := f.now.Add(time.Hour)
			stamp := at.Format("2006-01-02T15:04:05.000Z")
			saveSyntheticReviewRule(t, f, "Synthetic fixed-day dependency", syntheticHistoricalReviewSpec())
			id := domain.NewActivityID().String()
			var err error
			switch kind {
			case "account-state-ownership":
				_, err = f.db.SQL.Exec(`INSERT INTO account_state_observations(id,account_id,effective_at,include_in_net_worth,include_in_portfolio,include_in_liquid_assets,created_at) VALUES(?,?,?,0,0,0,?)`, id, f.account.Account.ID.String(), stamp, stamp)
				if err == nil {
					_, err = f.db.SQL.Exec(`INSERT INTO account_state_ownership(observation_id,member_id,share_bps) SELECT ?,member_id,share_bps FROM account_ownership WHERE account_id=?`, id, f.account.Account.ID.String())
				}
			case "instrument-state":
				_, err = f.db.SQL.Exec(`INSERT INTO instrument_state_observations(id,instrument_id,effective_at,archived_at,created_at) VALUES(?,?,?,?,?)`, id, f.instrument.ID.String(), stamp, stamp, stamp)
			case "holding-state":
				_, err = f.db.SQL.Exec(`INSERT INTO holding_state_observations(id,holding_id,effective_at,archived_at,created_at) VALUES(?,?,?,?,?)`, id, f.holding.ID.String(), stamp, stamp, stamp)
			case "instrument-preference":
				_, err = f.db.SQL.Exec(`INSERT INTO instrument_preference_observations(id,instrument_id,source_kind,effective_at,created_at) VALUES(?,?,'agent',?,?)`, id, f.instrument.ID.String(), stamp, stamp)
			case "fx-preference":
				_, err = f.db.SQL.Exec(`INSERT INTO fx_preference_observations(id,household_id,currency_a,currency_b,source_kind,effective_at,created_at) VALUES(?,?,'CNY','USD','agent',?,?)`, id, f.account.Account.HouseholdID.String(), stamp, stamp)
			case "provider-binding":
				_, err = f.db.SQL.Exec(`INSERT INTO instrument_provider_binding_revisions(instrument_id,provider_key,binding_revision,provider_symbol,enabled,effective_from,created_at) VALUES(?,'synthetic',1,'SYNTHETIC',1,?,?)`, f.instrument.ID.String(), stamp, stamp)
			case "quote-provider-times":
				_, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET provider_timestamp=?,fetched_at=?,value_effective_at=?`, stamp, stamp, stamp)
			case "coverage-expiry":
				_, err = f.db.SQL.Exec(`INSERT INTO market_data_day_status(target_type,target_id,provider_key,household_id,binding_revision,source_policy_version,effective_date,status,reason,checked_at,next_check_at,expires_at) VALUES('fx','USD/CNY','manual',?,0,?,'2026-10-02','no_observation','synthetic',?,?,?)`, f.account.Account.HouseholdID.String(), domain.MarketDataResolverPolicy, f.now.AddDate(0, 0, -1).Format(time.RFC3339Nano), stamp, stamp)
			}
			if err != nil {
				t.Fatal(err)
			}
			*f.now = at.Add(-time.Nanosecond)
			before := temporalCapture(t, f)
			if kind == "coverage-expiry" {
				found := false
				for _, coverage := range before.Historical.History.FXHistoryCoverage {
					if expiry, ok := coverage.NoObservationExpiresAt["2026-10-02"]; ok && expiry.Equal(at) {
						found = true
					}
				}
				if !found {
					t.Fatal("expiry fixture was not in the captured resolver closure")
				}
			}
			if !before.Temporal.Covers(at.Add(time.Nanosecond)) {
				t.Fatal("fixed-day-only fact treated as a current-instant dependency")
			}
			first := temporalPublicationOracle(t, f, before)
			*f.now = at
			after := temporalCapture(t, f)
			if before.HistoricalFingerprint != after.HistoricalFingerprint || before.SourceRevision.Token != after.SourceRevision.Token {
				t.Fatal("fixed-day selection followed current clock")
			}
			last := temporalPublicationOracle(t, f, after)
			// Rebuild both at one logical instant for an exact comparison of all
			// financial/coverage/lifecycle fields; detection timestamps are the only
			// reason independently planned first publications use their own now.
			first, err = PrepareReviewPublication(t.Context(), before, at, f.s.FXProviderKey())
			if err != nil {
				t.Fatal(err)
			}
			if err := compareReviewFullOracle(first, last, f.s.FXProviderKey()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReviewTemporalPerRuleAndSuppressionCalendarsAndStaleReads(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic UTC", syntheticHistoricalReviewSpec())
	spec := syntheticHistoricalReviewSpec()
	spec.Schedule.Timezone = "Asia/Tokyo"
	saveSyntheticReviewRule(t, f, "Synthetic Tokyo", spec)
	if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-04"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE investment_review_suppressions SET timezone='Asia/Kathmandu'`); err != nil {
		t.Fatal(err)
	}
	before := temporalCapture(t, f)
	want := time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC) // Tokyo midnight precedes household midnight.
	if !before.Temporal.NextTransitionAt.Equal(want) {
		t.Fatalf("wrong per-rule frontier %s", before.Temporal.NextTransitionAt)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	*f.now = want
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
	if err != nil || page.ProjectionStatus != "stale" {
		t.Fatalf("expired publication claimed current: %+v %v", page, err)
	}
	coverage, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{}, "", 10)
	if err != nil || coverage.ProjectionStatus != "stale" {
		t.Fatalf("coverage did not share temporal status: %+v %v", coverage, err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	current := temporalCapture(t, f)
	want = time.Date(2026, 10, 3, 18, 15, 0, 0, time.UTC)
	if !current.Temporal.NextTransitionAt.Equal(want) {
		t.Fatal("suppression's independent calendar omitted")
	}
	*f.now = want
	temporalPublicationOracle(t, f, temporalCapture(t, f))
}

func TestReviewTemporalRollbackRestartFloorAndRestoreForceFullOracle(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic rollback", syntheticHistoricalReviewSpec())
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	published := *f.now
	var episode string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&episode); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, now := range []time.Time{published.Add(time.Hour), published.Add(30 * time.Minute), published.Add(-time.Nanosecond), published.Add(-24 * time.Hour)} {
		*f.now = now
		capture := temporalCapture(t, f)
		if now != published.Add(time.Hour) && (!capture.TemporalFullRecompute || len(capture.ReusableDailyEvidence) != 0) {
			t.Fatal("observed rollback reused old financial evidence")
		}
		temporalPublicationOracle(t, f, capture)
	}
	// A new session invalidates even a forward certificate; an earlier clock
	// is additionally checked against its persisted logical as-of.
	path := filepath.Join(t.TempDir(), "synthetic-temporal-restart.db")
	if err := f.db.SnapshotTo(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	run := f
	run.db = reopened
	run.s = NewService(sqlite.NewRepository(reopened))
	run.s.setClock(func() time.Time { return *run.now })
	for _, now := range []time.Time{published.Add(time.Second), published.Add(-time.Nanosecond)} {
		*run.now = now
		c := temporalCapture(t, run)
		if c.PublishedTemporal == nil || !c.TemporalFullRecompute || len(c.ReusableDailyEvidence) != 0 || c.PublishedTemporal.ClockEpoch == c.Temporal.ClockEpoch {
			t.Fatal("restart certified old calendar/clock session")
		}
		temporalPublicationOracle(t, run, c)
	}
	*run.now = published
	if _, err := run.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if _, err := reopened.SQL.Exec(`UPDATE instrument_quotes SET unit_price='100.0001'`); err != nil {
		t.Fatal(err)
	}
	if err := reopened.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := reopened.PruneFinancialSourceChanges(t.Context(), run.account.Account.HouseholdID.String(), revision); err != nil {
		t.Fatal(err)
	}
	c := temporalCapture(t, run)
	if !c.TemporalFullRecompute || c.PublishedTemporal.Eligible(*run.now, c.Temporal) {
		t.Fatal("below-floor certificate reused")
	}
	temporalPublicationOracle(t, run, c)
	if err := reopened.ResetFinancialSourceTracking(t.Context()); err != nil {
		t.Fatal(err)
	}
	c = temporalCapture(t, run)
	if !c.TemporalFullRecompute || c.PublishedTemporal.Eligible(*run.now, c.Temporal) {
		t.Fatal("restore epoch reused old certificate")
	}
	temporalPublicationOracle(t, run, c)
	if _, err := run.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var ack string
	if err := reopened.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, episode).Scan(&ack); err != nil || ack == "" {
		t.Fatal("rollback/restart/restore lost acknowledgement")
	}
}

// Install a test-owned driver callback only during publication, after its full
// source checkpoint read and before the final injected clock. Capture itself
// is deliberately excluded so retry-count assertions retain their meaning.
type temporalRecheckFaultRepository struct {
	*sqlite.Repository
	faults       *reviewDriverFaults
	afterRecheck func()
}

func (r *temporalRecheckFaultRepository) PublishReviewProjectionWithClock(ctx context.Context, p domain.ReviewPublication, clock func() time.Time) error {
	r.faults.mu.Lock()
	old := r.faults.afterQuery
	r.faults.afterQuery = func(query string) {
		if strings.HasPrefix(query, "SELECT revision,journal_floor,tracking_epoch,tracking_version") {
			r.afterRecheck()
		}
	}
	r.faults.mu.Unlock()
	defer func() { r.faults.mu.Lock(); r.faults.afterQuery = old; r.faults.mu.Unlock() }()
	return r.Repository.PublishReviewProjectionWithClock(ctx, p, clock)
}
