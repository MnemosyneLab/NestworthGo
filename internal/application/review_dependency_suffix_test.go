package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func suffixQuote(t *testing.T, f seriesZeroFixture, id domain.InstrumentID, on, price string) {
	t.Helper()
	at := on + "T17:00:00.000Z"
	_, err := f.db.SQL.Exec(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,'CNY','agent','agent',?,?,'close',?,?,0,'agent_raw_close_v1','date_label',1)`, domain.NewInstrumentQuoteID().String(), id.String(), price, at, at, on, at)
	if err != nil {
		t.Fatal(err)
	}
}
func suffixFixture(t *testing.T, policy domain.ReviewTriggerPolicy, zones ...string) (seriesZeroFixture, *sqlite.Repository, domain.InstrumentID) {
	t.Helper()
	zone := "UTC"
	if len(zones) > 0 {
		zone = zones[0]
	}
	f := newSeriesZeroFixture(t, false, zone, "2026-10-01", "1")
	second, err := f.s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic independent stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CreateHolding(t.Context(), HoldingInput{AccountID: f.account.Account.ID.String(), InstrumentID: second.ID.String(), Quantity: "1", UnitCost: "100"}); err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 11; day++ {
		on := time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		suffixQuote(t, f, f.instrument.ID, on, "120")
		suffixQuote(t, f, second.ID, on, "120")
	}
	f.day(t, "2026-10-12")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "110", Currency: "CNY", FXMode: "native"}}}}
	spec.TriggerPolicy = policy
	spec.Schedule.Timezone = zone
	saveSyntheticReviewRule(t, f, "Synthetic dependency suffix", spec)
	r := sqlite.NewRepository(f.db).WithReviewDependencySuffix()
	fake := newRefreshFakeProvider()
	fake.key = "synthetic-pinned"
	f.s = NewService(r, NewMarketDataRegistry(fake, &refreshAliasProvider{key: "synthetic-other", target: fake}))
	if err = f.s.SetFXProvider(fake.key); err != nil {
		t.Fatal(err)
	}
	f.s.setClock(func() time.Time { return *f.now })
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f, r, second.ID
}
func assertSuffixOracle(t *testing.T, f seriesZeroFixture, r *sqlite.Repository, wantReuse bool) domain.ReviewPublication {
	t.Helper()
	preserved := suffixExistingIdentities(t, f.db)
	reference := suffixFullReference(t, f)
	stage := certifiedStage(t, f, r)
	if (stage.Capture.Suffix != nil) != wantReuse {
		t.Fatalf("suffix=%v want %v", stage.Capture.Suffix != nil, wantReuse)
	}
	assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, stage)
	if err := certifiedPublish(t.Context(), f, r, stage, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	got, want := suffixDurableRows(t, f.db, preserved), suffixDurableRows(t, reference, preserved)
	if !reflect.DeepEqual(got, want) {
		for table := range got {
			if !reflect.DeepEqual(got[table], want[table]) {
				t.Errorf("durable oracle mismatch %s\ngot=%v\nwant=%v", table, got[table], want[table])
			}
		}
		t.FailNow()
	}
	if !reflect.DeepEqual(publicationDailyTimes(t, f.db), publicationDailyTimes(t, reference)) {
		t.Fatal("daily timestamps differ")
	}

	return stage
}
func TestReviewDependencySuffixFullDurableOracle(t *testing.T) {
	testReviewDependencySuffixFullDurableOracle(t, false)
}

// Normal check keeps representative policies and change/fallback boundaries.
// The complete cross-product is local opt-in, not another normal CI workload.
func testReviewDependencySuffixFullDurableOracle(t *testing.T, extended bool) {
	for _, policy := range []domain.ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 3}, {Kind: "rolling", WindowDays: 5, MinHits: 3}} {
		for _, scenario := range []string{"clock", "append", "correction", "backfill", "delete", "move", "quantity", "quote-and-quantity", "missing-row", "restart"} {
			representative := scenario == "correction" || policy.Kind == "single" && (scenario == "clock" || scenario == "delete" || scenario == "move" || scenario == "restart")
			if !extended && !representative {
				continue
			}
			t.Run(policy.Kind+"/"+scenario, func(t *testing.T) {
				f, r, second := suffixFixture(t, policy)
				*f.now = f.now.Add(time.Hour)
				want := true
				switch scenario {
				case "clock":
					f.day(t, "2026-10-13")
				case "append":
					f.day(t, "2026-10-13")
					suffixQuote(t, f, f.instrument.ID, "2026-10-12", "90")
				case "correction":
					_, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE instrument_id=? AND effective_date='2026-10-09'`, f.instrument.ID.String())
					if err != nil {
						t.Fatal(err)
					}
				case "backfill":
					suffixQuote(t, f, f.instrument.ID, "2026-10-08", "90")
				case "delete":
					_, err := f.db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=? AND effective_date='2026-10-08'`, f.instrument.ID.String())
					if err != nil {
						t.Fatal(err)
					}
				case "move":
					_, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET instrument_id=?,effective_date='2026-10-07' WHERE instrument_id=? AND effective_date='2026-10-09'`, second.String(), f.instrument.ID.String())
					if err != nil {
						t.Fatal(err)
					}
				case "quantity", "quote-and-quantity":
					f.quantity(t, false)
					want = false
					if scenario == "quote-and-quantity" {
						suffixQuote(t, f, f.instrument.ID, "2026-10-08", "90")
					}
				case "missing-row":
					_, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_inputs WHERE on_date='2026-10-02'`)
					if err != nil {
						t.Fatal(err)
					}
					want = false
				case "restart":
					want = false // Same file, different DB handle has no private signing key.
					db, err := sqlite.Open(f.db.Path)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { db.Close() })
					r = sqlite.NewRepository(db).WithReviewDependencySuffix()
				}
				stage := assertSuffixOracle(t, f, r, want)
				if scenario == "restart" || scenario == "missing-row" {
					assertNoCertifiedMAC(t, f)
				}
				if want {
					for _, rule := range stage.Capture.Rules {
						for _, id := range []domain.InstrumentID{f.instrument.ID, second} {
							plan, err := PlanReviewRuleInstrument(t.Context(), stage.Capture, rule, id, *f.now, f.s.FXProviderKey())
							if err != nil {
								t.Fatal(err)
							}
							if scenario == "correction" && id == f.instrument.ID && plan.RecomputedDays != 3 {
								t.Fatalf("correction recomputed %d, want 3", plan.RecomputedDays)
							}
							if scenario == "correction" && id == second && plan.RecomputedDays != 0 {
								t.Fatal("unrelated instrument recomputed")
							}
							if (scenario == "clock" || scenario == "append") && plan.RecomputedDays != 1 {
								t.Fatalf("new day recomputed %d, want 1", plan.RecomputedDays)
							}
						}
					}
				}
			})
		}
	}
}

func TestReviewDependencySuffixAuthorityAndRollback(t *testing.T) {
	for _, scenario := range []string{"source", "handling", "tamper-prefix", "cancel", "legacy-publisher", "disabled-publisher", "clock", "provider-aba", "projection", "floor", "checkpoint-abort", "external-source", "final-cancel", "final-prefix-change", "final-digest-change", "final-source-proof-change"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			if p.Capture.Suffix == nil {
				t.Fatal("no suffix")
			}
			switch scenario {
			case "external-source":
				db, err := sqlite.Open(f.db.Path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE instrument_id=? AND effective_date='2026-10-09'`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "source":
				suffixQuote(t, f, f.instrument.ID, "2026-10-05", "90")
			case "handling":
				if _, err := f.db.SQL.Exec(`UPDATE investment_review_control SET review_state_revision=review_state_revision+1`); err != nil {
					t.Fatal(err)
				}
			case "provider-aba":
				for _, key := range []string{"synthetic-other", "synthetic-pinned"} {
					if err := f.s.SetFXProvider(key); err != nil {
						t.Fatal(err)
					}
				}
			case "projection":
				if _, err := f.db.SQL.Exec(`UPDATE investment_review_daily_results SET evidence_json='{}' WHERE on_date='2026-10-01'`); err != nil {
					t.Fatal(err)
				}
			case "floor":
				if err := f.db.PruneFinancialSourceChanges(t.Context(), f.account.Account.HouseholdID.String(), p.Capture.SourceRevision.Token.Revision); err != nil {
					t.Fatal(err)
				}
			case "checkpoint-abort":
				if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_suffix_abort BEFORE UPDATE ON review_temporal_checkpoints BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
					t.Fatal(err)
				}
			case "tamper-prefix":
				p.Capture.Suffix.Prefix[0].Evidence.Result.Truth = domain.ReviewFalse
			}
			before := noChangeProjectionRows(t, f.db)
			ctx := t.Context()
			if scenario == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			clock := func() time.Time { return *f.now }
			if scenario == "final-cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				clock = func() time.Time { cancel(); return *f.now }
			}
			if scenario == "final-source-proof-change" {
				clock = func() time.Time {
					p.Capture.Certified.SuffixSourceProof.QuoteSeals[0].Fingerprint = "synthetic-drift"
					return *f.now
				}
			}
			if scenario == "final-prefix-change" {
				clock = func() time.Time { p.Capture.Suffix.Prefix[0].Evidence.Result.Truth = domain.ReviewFalse; return *f.now }
			}
			if scenario == "final-digest-change" {
				clock = func() time.Time {
					p.Capture.Suffix.Prefix[0].Evidence.Result.Truth = domain.ReviewFalse
					p.Capture.Certified.SuffixDigest, _ = domain.ReviewSuffixFingerprint(p.Capture.Suffix)
					return *f.now
				}
			}
			if scenario == "clock" {
				clock = func() time.Time { return f.now.AddDate(0, 0, 1) }
			}
			var err error
			switch scenario {
			case "legacy-publisher":
				err = r.PublishReviewProjection(ctx, p, *f.now)
			case "disabled-publisher":
				err = sqlite.NewRepository(f.db).PublishReviewProjection(ctx, p, *f.now)
			default:
				err = certifiedPublish(ctx, f, r, p, clock)
			}
			if err == nil {
				t.Fatal("lost suffix authority committed")
			}
			if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("failed suffix half-wrote projection")
			}
		})
	}
}

// Preserve every preexisting identity. Only UUIDs independently allocated by
// the two current publications map through their complete semantic key; every
// financial, lifecycle, handling, frozen payload and timestamp field stays exact.
func suffixExistingIdentities(t *testing.T, db *sqlite.DB) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	rows, err := db.SQL.Query(`SELECT id FROM investment_review_episodes UNION ALL SELECT id FROM investment_review_episode_events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[id] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
func suffixDurableRows(t *testing.T, db *sqlite.DB, preserved map[string]bool) map[string][]string {
	t.Helper()
	replacements := map[string]string{}
	for _, q := range []string{`SELECT id,json_array(rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at) FROM investment_review_episodes`, `SELECT id,event_key FROM investment_review_episode_events`} {
		rows, err := db.SQL.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, key string
			if err = rows.Scan(&id, &key); err != nil {
				t.Fatal(err)
			}
			if preserved[id] {
				continue
			}
			for old, mapped := range replacements {
				key = strings.ReplaceAll(key, old, mapped)
			}
			replacements[id] = fmt.Sprintf("new-identity:%x", sha256.Sum256([]byte(key)))
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	result := noChangeProjectionRows(t, db)
	for table, rows := range result {
		for i, row := range rows {
			for old, mapped := range replacements {
				row = strings.ReplaceAll(row, old, mapped)
			}
			rows[i] = row
		}
		sort.Strings(rows)
		result[table] = rows
	}
	return result
}

func TestReviewDependencySuffixHandlingCalendarsAndFallback(t *testing.T) {
	for _, scenario := range []string{"acknowledged-correction", "suppression-expiry", "clock-rollback", "future-position-activation", "source-rollback", "journal-scope-damage", "FX", "rule-edit", "DST"} {
		t.Run(scenario, func(t *testing.T) {
			zone := "UTC"
			if scenario == "DST" {
				zone = "America/New_York"
			}
			f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, zone)
			want := true
			switch scenario {
			case "acknowledged-correction":
				var id string
				var rev int
				if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL LIMIT 1`).Scan(&id, &rev); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), rev); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE instrument_id=? AND effective_date='2026-10-09'`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
			case "suppression-expiry":
				if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-14"}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				f.day(t, "2026-10-15")
			case "clock-rollback":
				*f.now = f.now.Add(-time.Minute)
				want = false
			case "future-position-activation":
				f.day(t, "2026-10-14")
				f.quantity(t, false)
				f.day(t, "2026-10-12")
				// The clock rollback forces a complete new authenticated publication.
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				f.day(t, "2026-10-15")
				want = false
			case "source-rollback":
				tx, err := f.db.SQL.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE effective_date='2026-10-09'`); err != nil {
					t.Fatal(err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			case "journal-scope-damage":
				if _, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE instrument_id=? AND effective_date='2026-10-09'`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.SQL.Exec(`UPDATE financial_source_changes SET old_scope_json=json_set(old_scope_json,'$.effective_date','2026-10-11'),new_scope_json=json_set(new_scope_json,'$.effective_date','2026-10-11') WHERE revision=(SELECT MAX(revision) FROM financial_source_changes)`); err != nil {
					t.Fatal(err)
				}
				want = false
			case "FX":
				if _, err := f.s.AppendManualFXQuote(t.Context(), "USD", "CNY", "2", f.now.Format(time.RFC3339)); err != nil {
					t.Fatal(err)
				}
				want = false
			case "rule-edit":
				if _, err := f.db.SQL.Exec(`UPDATE investment_review_rules SET row_revision=row_revision+1`); err != nil {
					t.Fatal(err)
				}
				want = false
			case "DST":
				f.day(t, "2026-10-31")
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				f.day(t, "2026-11-02")
			}
			assertSuffixOracle(t, f, r, want)
		})
	}
}

func TestReviewDependencySuffixExactFXPrefix(t *testing.T) {
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.FXMode = "convert_on_day"
	f, r := certifiedPublicationFixture(t, true, spec, "1", false)
	r = r.WithReviewDependencySuffix()
	f.s.repository = r
	// A prior C3 certificate cannot invent D2 source descriptions. Establish
	// them through one full publication on the existing authenticated chain.
	prime := certifiedStage(t, f, r)
	if prime.Capture.Suffix != nil {
		t.Fatal("old C3 certificate acquired D2 reuse")
	}
	if err := certifiedPublish(t.Context(), f, r, prime, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-13")
	p := assertSuffixOracle(t, f, r, true)
	found := false
	for _, day := range p.Capture.Suffix.Prefix {
		for _, selected := range day.Evidence.Input.SelectedFX {
			if !day.OutputAttested {
				t.Fatal("exact FX clock prefix not output-attested")
			}
			found = true
			if selected.Quote.Rate.String() != "2" {
				t.Fatal("exact FX lost")
			}
		}
	}
	if !found {
		t.Fatal("fixture did not select FX")
	}
}

func TestReviewDependencySuffixEnabledServiceAndForcedFull(t *testing.T) {
	f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
	f.day(t, "2026-10-13")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	requireCertifiedNoop(t, f, r)
	r = r.WithFullReviewRecompute()
	if r.CertifiedReviewEnabled() {
		t.Fatal("rollback retained certified dispatcher")
	}
	cap, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Suffix != nil || len(cap.ReusableDailyEvidence) != 0 || cap.Certified != nil {
		t.Fatal("forced full retained reuse authority")
	}
	f.s.repository = r
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	noChangeFullOracle(t, f)
}

func suffixFullReference(t *testing.T, f seriesZeroFixture) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-suffix-full.db")
	if _, err := f.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := sqlite.NewRepository(db).WithFullReviewRecompute()
	c, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Suffix != nil || len(c.ReusableDailyEvidence) != 0 {
		t.Fatal("oracle reused projection")
	}
	p, err := PrepareReviewPublication(t.Context(), c, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	// Select the identical additive exact-FX serializer, without source MAC,
	// certified dispatch, prefix facts, checkpoints or incremental computation.
	p.Capture.Certified = &domain.ReviewCertifiedCapture{}
	if err = r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReviewDependencySuffixLookbackBaseline(t *testing.T) {
	f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 3, MinHits: 2})
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}}
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 3, MinHits: 2}
	saveSyntheticReviewRule(t, f, "Synthetic lookback suffix", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='90' WHERE instrument_id=? AND effective_date='2026-10-02'`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	p := assertSuffixOracle(t, f, r, true)
	found := false
	for _, rule := range p.Rules {
		for _, day := range rule.Days {
			for _, leaf := range day.Result.Leaves {
				if day.On == "2026-10-09" && leaf.Metric == "price_change" && leaf.Truth == domain.ReviewTrue && leaf.BaselineOn == "2026-10-02" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("correction did not propagate beyond the rolling window to its lookback endpoint")
	}
}

func TestReviewDependencySourceDescriptionMatchesFullDerivation(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			f.day(t, "2026-10-13")
			if changed {
				suffixQuote(t, f, f.instrument.ID, "2026-10-09", "90")
			}
			p := certifiedStage(t, f, r)
			if p.Capture.Suffix == nil || p.Capture.Certified.SuffixSourceProof == nil {
				t.Fatal("no authenticated source description")
			}
			quotes, err := domain.ReviewQuoteDaySeals(p.Capture.Historical)
			if err != nil {
				t.Fatal(err)
			}
			nonquote, err := domain.ReviewNonQuoteFingerprint(p.Capture.Historical)
			if err != nil {
				t.Fatal(err)
			}
			actual := p.Capture.Certified.SuffixSourceProof
			if !reflect.DeepEqual(quotes, actual.QuoteSeals) || nonquote != actual.NonQuoteFingerprint {
				t.Fatal("description differs from independent complete derivation")
			}
			if err = certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err = f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var cert domain.ReviewTemporalCertificate
			if err = json.Unmarshal([]byte(raw), &cert); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cert.SuffixQuoteSeals, quotes) || cert.SuffixNonQuoteFingerprint != nonquote {
				t.Fatal("published source description differs")
			}
		})
	}
}

func TestReviewDependencyProviderChangePreservesCertifiedChain(t *testing.T) {
	f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
	*f.now = f.now.Add(time.Minute)
	if err := f.s.SetFXProvider("synthetic-other"); err != nil {
		t.Fatal(err)
	}
	// The supported setter notifies SQLite and changes its clock epoch before
	// capture. Financial reuse is rejected while integrity authority survives.
	capture, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Suffix != nil || capture.Certified == nil || !capture.Certified.MayCertify {
		t.Fatal("provider transition did not select an authenticated full capture")
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var cert domain.ReviewTemporalCertificate
	if err := json.Unmarshal([]byte(raw), &cert); err != nil {
		t.Fatal(err)
	}
	if cert.CertifiedMAC == "" || cert.CertifiedProvider != f.s.reviewProviderSnapshot() {
		t.Fatal("successful full provider fallback lost the existing certified chain")
	}
	requireCertifiedNoop(t, f, r)
	f.day(t, "2026-10-13")
	assertSuffixOracle(t, f, r, true)
}

func TestReviewDependencyClockOutputPartitionAndAtomicGuards(t *testing.T) {
	for _, scenario := range []string{"complete", "gap", "overlap", "duplicate", "missing-pair", "excluded-pair", "late-attestation"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 5, MinHits: 3})
			f.day(t, "2026-10-13")
			if scenario == "complete" {
				stage := assertSuffixOracle(t, f, r, true)
				n := 0
				for _, item := range stage.Rules {
					n += len(item.Days)
				}
				if n != 2 {
					t.Fatalf("emitted %d days, want 2", n)
				}
				return
			}
			stage := certifiedStage(t, f, r)
			before := suffixDurableRows(t, f.db, suffixExistingIdentities(t, f.db))
			times := publicationDailyTimes(t, f.db)
			clock := func() time.Time { return *f.now }
			switch scenario {
			case "gap":
				stage.Rules[0].Days = nil
			case "overlap":
				p := stage.Capture.Suffix.Prefix[0]
				for i := range stage.Rules {
					if stage.Rules[i].RuleID == p.Key.RuleID && stage.Rules[i].InstrumentID == p.Key.InstrumentID {
						stage.Rules[i].Days = append(stage.Rules[i].Days, domain.ReviewPublicationDay{On: p.Key.On, Input: p.Evidence.Input, Result: p.Evidence.Result, CoverageMissing: p.Evidence.CoverageMissing, InputFingerprint: p.Evidence.InputFingerprint})
						break
					}
				}
			case "duplicate":
				stage.Rules[0].Days = append(stage.Rules[0].Days, stage.Rules[0].Days[0])
			case "missing-pair":
				stage.Rules = stage.Rules[1:]
			case "excluded-pair":
				stage.Rules[0].TargetExcluded = true
				stage.Rules[0].Days = nil
			case "late-attestation":
				clock = func() time.Time { stage.Capture.Suffix.Prefix[0].OutputAttested = false; return *f.now }
			}
			if err := certifiedPublish(t.Context(), f, r, stage, clock); err == nil {
				t.Fatal("invalid partition published")
			}
			after := suffixDurableRows(t, f.db, suffixExistingIdentities(t, f.db))
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(times, publicationDailyTimes(t, f.db)) {
				t.Fatal("failed publication changed durable output/timestamps")
			}
		})
	}
}
