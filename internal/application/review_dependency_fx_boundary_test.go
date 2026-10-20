package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// FX mutations deliberately retain full preparation until complete candidate
// coverage has an authenticated OLD/NEW description independent of the journal.
func TestReviewDependencyFXMutationFullOracle(t *testing.T) {
	for _, scenario := range []string{"insert", "correction", "delete", "date-move", "inverse", "cross", "mixed", "unselected-candidate"} {
		t.Run(scenario, func(t *testing.T) {
			spec := syntheticHistoricalReviewSpec()
			spec.Predicate.All[1].Leaf.FXMode = "convert_on_day"
			f, r := certifiedPublicationFixture(t, true, spec, "1", false)
			r = r.WithReviewDependencySuffix()
			f.s.repository = r
			q, err := f.s.AppendManualFXQuote(t.Context(), "USD", "CNY", "2", "2026-10-08")
			if err != nil {
				t.Fatal(err)
			}
			// Explicit civil-date candidates keep the economic-date policy visible.
			if _, err = f.db.SQL.Exec(`UPDATE fx_quotes SET source_kind='agent',source_key='agent',observation_kind='daily_reference',effective_date=substr(quoted_at,1,10),timestamp_basis='date_label',source_policy_version='agent_daily_reference_v1',value_effective_at=quoted_at`); err != nil {
				t.Fatal(err)
			}
			if scenario == "unselected-candidate" {
				loser := domain.NewFXQuoteID()
				if _, err = f.db.SQL.Exec(`INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,timestamp_basis,source_policy_version,value_effective_at) SELECT ?,household_id,base_currency,quote_currency,'999','provider','synthetic-pinned',quoted_at,created_at,observation_kind,effective_date,timestamp_basis,source_policy_version,value_effective_at FROM fx_quotes WHERE id=?`, loser.String(), q.ID.String()); err != nil {
					t.Fatal(err)
				}
				q.ID = loser
			}
			if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			var episode string
			var revision int
			if err = f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL LIMIT 1`).Scan(&episode, &revision); err != nil {
				t.Fatal("fixture must exercise acknowledged lifecycle", err)
			}
			if _, err = f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), revision); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			*f.now = f.now.Add(time.Hour)
			// Positive control: this chain really can reuse authenticated FX input.
			control := assertSuffixOracle(t, f, r, true)
			selected := false
			directSelected := false
			for _, day := range control.Capture.Suffix.Prefix {
				selected = selected || len(day.Evidence.Input.SelectedFX) > 0
				for _, fx := range day.Evidence.Input.SelectedFX {
					directSelected = directSelected || fx.Quote.ID == q.ID
				}
				if scenario == "unselected-candidate" {
					for _, fx := range day.Evidence.Input.SelectedFX {
						if fx.Quote.ID == q.ID {
							t.Fatal("losing candidate was selected")
						}
					}
				}
			}
			if !selected || (scenario != "unselected-candidate" && !directSelected) {
				t.Fatal("positive control did not reuse selected FX")
			}
			before, err := domain.ReviewNonQuoteFingerprint(control.Capture.Historical)
			if err != nil {
				t.Fatal(err)
			}
			var statement string
			switch scenario {
			case "correction":
				statement = `UPDATE fx_quotes SET rate='3.123456789123',revision=revision+1 WHERE id=?`
			case "delete":
				statement = `DELETE FROM fx_quotes WHERE id=?`
			case "date-move":
				statement = `UPDATE fx_quotes SET effective_date='2026-10-06' WHERE id=?`
			case "inverse":
				statement = `UPDATE fx_quotes SET base_currency='CNY',quote_currency='USD',rate='0.5' WHERE id=?`
			case "cross":
				statement = `UPDATE fx_quotes SET base_currency='EUR' WHERE id=?`
			case "insert", "mixed", "unselected-candidate":
				// A different ID at an older effective date is still a complete
				// candidate change even when a later selected quote wins.
				statement = `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,timestamp_basis,source_policy_version,value_effective_at) SELECT '` + domain.NewFXQuoteID().String() + `',household_id,base_currency,quote_currency,'3.123456789123',source_kind,source_key,'2026-10-06T12:00:00Z',created_at,'daily_reference','2026-10-06','date_label',source_policy_version,'2026-10-06T12:00:00Z' FROM fx_quotes WHERE id=?`
				if scenario == "unselected-candidate" {
					statement = `UPDATE fx_quotes SET rate='3.123456789123' WHERE id=?`
				}
			}
			result, err := f.db.SQL.Exec(statement, q.ID.String())
			if err != nil {
				t.Fatal(err)
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				t.Fatal("mutation must change exactly one candidate", n, err)
			}
			if scenario == "mixed" {
				if _, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='99' WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			*f.now = f.now.Add(time.Hour)
			stage := assertSuffixOracle(t, f, r, false)
			after, err := domain.ReviewNonQuoteFingerprint(stage.Capture.Historical)
			if err != nil || before == after {
				t.Fatal("complete non-quote authentication lost FX mutation", err)
			}
		})
	}
}
