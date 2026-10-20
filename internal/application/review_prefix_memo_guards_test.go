package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"testing"
	"time"
)

func TestReviewPrefixMemoGuards(t *testing.T) {
	for _, scenario := range []string{"stale", "cancel", "evict", "source", "provider", "ack", "suppression", "missing-row", "clock-rollback", "reopen", "rule"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3})
			r = r.WithReviewPrefixMemo()
			f.s.repository = r
			f.day(t, "2026-10-13")
			assertSuffixOracle(t, f, r, true)
			f.day(t, "2026-10-14")
			wantSuffix := true
			switch scenario {
			case "stale":
				a, b := certifiedStage(t, f, r), certifiedStage(t, f, r)
				if err := certifiedPublish(t.Context(), f, r, a, func() time.Time { return *f.now }); err != nil {
					t.Fatal(err)
				}
				if err := certifiedPublish(t.Context(), f, r, b, func() time.Time { return *f.now }); err == nil {
					t.Fatal("stale publication accepted")
				}
				requireCertifiedNoop(t, f, r)
				f.day(t, "2026-10-15")
			case "cancel":
				a := certifiedStage(t, f, r)
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				if err := certifiedPublish(ctx, f, r, a, func() time.Time { return *f.now }); err == nil {
					t.Fatal("cancelled publication accepted")
				}
			case "evict":
				r.EvictReviewPrefixMemo()
			case "rule":
				saveSyntheticReviewRule(t, f, "Synthetic memo rule edit", syntheticHistoricalReviewSpec())
				wantSuffix = false
			case "source":
				suffixQuote(t, f, f.instrument.ID, "2026-10-05", "90")
			case "provider":
				if err := f.s.SetFXProvider("synthetic-other"); err != nil {
					t.Fatal(err)
				}
				wantSuffix = false
			case "ack":
				var id string
				var rev int
				if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL LIMIT 1`).Scan(&id, &rev); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), rev); err != nil {
					t.Fatal(err)
				}
				wantSuffix = false
			case "suppression":
				if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-16"}); err != nil {
					t.Fatal(err)
				}
				wantSuffix = false
			case "reopen":
				path := f.db.Path
				f.db.Close()
				db, err := sqlite.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				f.db = db
				r = sqlite.NewRepository(db).WithReviewPrefixMemo()
				f.s.repository = r
				wantSuffix = false
			case "clock-rollback":
				*f.now = f.now.AddDate(0, 0, -3)
				wantSuffix = false
			case "missing-row":
				if _, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_results WHERE on_date='2026-10-01'`); err != nil {
					t.Fatal(err)
				}
				wantSuffix = false
			}
			p := assertSuffixOracle(t, f, r, wantSuffix)
			o := p.Capture.Certified.SuffixMemoObservation
			if scenario == "cancel" {
				if o == nil || o.ReusedDays != 22 || o.SQLDecodedDays != 2 {
					t.Fatalf("cancel advanced cache %+v", o)
				}
			} else if o != nil && o.ReusedDays != 0 {
				t.Fatalf("invalidation/stale promotion reused memo %+v", o)
			}
			if scenario != "missing-row" && scenario != "reopen" {
				requireCertifiedNoop(t, f, r)
			}
			if scenario == "ack" || scenario == "suppression" {
				f.day(t, "2026-10-15")
				assertSuffixOracle(t, f, r, true)
				f.day(t, "2026-10-16")
				warm := assertSuffixOracle(t, f, r, true)
				if warm.Capture.Certified.SuffixMemoObservation.ReusedDays == 0 {
					t.Fatal("handling/expiry warm memo not exercised")
				}
			}

		})
	}
}
func TestReviewPrefixMemoExactFX(t *testing.T) {
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.FXMode = "convert_on_day"
	f, r := certifiedPublicationFixture(t, true, spec, "1")
	r = r.WithReviewPrefixMemo()
	f.s.repository = r
	prime := certifiedStage(t, f, r)
	if err := certifiedPublish(t.Context(), f, r, prime, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-13")
	cold := assertSuffixOracle(t, f, r, true)
	for i := range cold.Capture.Suffix.Prefix {
		for k, v := range cold.Capture.Suffix.Prefix[i].Evidence.Input.SelectedFX {
			v.Quote.Rate, _ = domain.ParseFxRate("3.123456789012")
			cold.Capture.Suffix.Prefix[i].Evidence.Input.SelectedFX[k] = v
		}
	}
	requireCertifiedNoop(t, f, r)
	f.day(t, "2026-10-14")
	warm := assertSuffixOracle(t, f, r, true)
	if warm.Capture.Certified.SuffixMemoObservation.ReusedDays == 0 {
		t.Fatal("FX memo not reused")
	}
	for _, p := range warm.Capture.Suffix.Prefix {
		for _, v := range p.Evidence.Input.SelectedFX {
			if v.Quote.Rate.String() != "2" {
				t.Fatal("private FX alias")
			}
		}
	}
}

func TestReviewPrefixMemoFutureActivation(t *testing.T) {
	f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
	r = r.WithReviewPrefixMemo()
	f.s.repository = r
	f.day(t, "2026-10-14")
	f.quantity(t, false)
	f.day(t, "2026-10-12")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-13")
	cold := assertSuffixOracle(t, f, r, true)
	f.day(t, "2026-10-15")
	active := assertSuffixOracle(t, f, r, false)
	if cold.Capture.SourceRevision.Token != active.Capture.SourceRevision.Token {
		t.Fatal("future activation unexpectedly required a journal write")
	}
	if active.Capture.Certified.SuffixMemoObservation != nil && active.Capture.Certified.SuffixMemoObservation.ReusedDays != 0 {
		t.Fatal("future membership bypassed full source invalidation")
	}
}
