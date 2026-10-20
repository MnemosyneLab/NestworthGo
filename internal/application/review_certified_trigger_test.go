package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewCertifiedPreopenUnknownTriggerCannotBootstrap(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-12")
	saveSyntheticReviewRule(t, f, "Synthetic trigger boundary", syntheticHistoricalReviewSpec())
	if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_hidden_writer AFTER UPDATE OF input_generation ON investment_review_control WHEN OLD.input_generation=NEW.input_generation BEGIN UPDATE investment_review_daily_inputs SET evidence_json=json_set(evidence_json,'$.Quantity','999') WHERE on_date='2026-10-01'; END`); err != nil {
		t.Fatal(err)
	}
	path := f.db.Path
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f.db = db
	r := sqlite.NewRepository(db).WithCertifiedReviewCandidate()
	f.s = NewService(r)
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A confirmation must decline before its control self-update can run the
	// trigger. Repeated confirmations may never ratchet the damage into authority.
	before := noChangeProjectionRows(t, db)
	for range 2 {
		_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Error("unknown persistent writer authorized no-op")
		}
	}
	if !reflect.DeepEqual(before, noChangeProjectionRows(t, db)) {
		t.Error("confirmation fired unknown writer and changed historical evidence")
	}
	assertNoCertifiedMAC(t, f)
}

func TestReviewCertifiedHandlingCannotResealUnknownWriters(t *testing.T) {
	for _, storage := range []string{"TEMP", "permanent"} {
		for _, operation := range []string{"ack", "rule", "suppression"} {
			t.Run(storage+"/"+operation, func(t *testing.T) {
				f, r := certifiedFixture(t)
				// Retain a resolved episode/event that full preparation does not rewrite.
				f.day(t, "2026-10-13")
				f.quantity(t, false)
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				var closed string
				if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NOT NULL LIMIT 1`).Scan(&closed); err != nil {
					t.Fatal(err)
				}
				f.day(t, "2026-10-14")
				f.quantity(t, true)
				f.day(t, "2026-10-15")
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				var before string
				if err := f.db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, closed).Scan(&before); err != nil {
					t.Fatal(err)
				}
				prefix := ""
				if storage == "TEMP" {
					prefix = "TEMP "
				}
				if _, err := f.db.SQL.Exec(`CREATE ` + prefix + `TRIGGER synthetic_handling_writer AFTER UPDATE ON investment_review_control BEGIN UPDATE investment_review_episode_events SET payload_json=json_set(payload_json,'$.TriggerResult.truth','false') WHERE episode_id='` + closed + `' AND kind='opened'; END`); err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "ack":
					var id string
					var revision int
					if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&id, &revision); err != nil {
						t.Fatal(err)
					}
					if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), revision); err != nil {
						t.Fatal(err)
					}
				case "rule":
					rules, err := f.s.ListReviewRulesV2(t.Context(), "", 10)
					if err != nil {
						t.Fatal(err)
					}
					rule := rules[0]
					if _, err := f.s.UpdateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(rule.ID), MutationID: string(domain.NewMutationID()), ExpectedRevision: rule.RowRevision, Name: rule.Name + " edited", Enabled: rule.Enabled, Severity: rule.Severity, Spec: rule.Spec}); err != nil {
						t.Fatal(err)
					}
				case "suppression":
					if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-16"}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := f.db.SQL.Exec(`DROP TRIGGER synthetic_handling_writer`); err != nil {
					t.Fatal(err)
				}
				var after string
				if err := f.db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, closed).Scan(&after); err != nil {
					t.Fatal(err)
				}
				if before == after {
					t.Fatal("fault trigger did not alter retained frozen history")
				}
				p := certifiedStage(t, f, r)
				if p.Capture.Certified.MayCertify {
					t.Fatal("handling laundered an unrecognized writer into integrity authority")
				}
				if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
					t.Fatal(err)
				}
				assertNoCertifiedMAC(t, f)
				var retained string
				if err := f.db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, closed).Scan(&retained); err != nil {
					t.Fatal(err)
				}
				if retained != after {
					t.Fatal("fixture no longer demonstrates unrepaired retained history")
				}
			})
		}
	}
}
