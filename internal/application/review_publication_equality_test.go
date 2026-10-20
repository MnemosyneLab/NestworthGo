package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func publicationDailyTimes(t *testing.T, db *sqlite.DB) []string {
	t.Helper()
	times := []string{}
	for _, query := range []string{
		`SELECT json_array(household_id,instrument_id,timezone_version,on_date,input_policy,computed_at) FROM investment_review_daily_inputs ORDER BY household_id,instrument_id,timezone_version,on_date,input_policy`,
		`SELECT json_array(rule_id,condition_version,instrument_id,timezone_version,on_date,evaluated_at,revised_at) FROM investment_review_daily_results ORDER BY rule_id,condition_version,instrument_id,timezone_version,on_date`,
	} {
		rows, err := db.SQL.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			times = append(times, value)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return times
}

func TestReviewCertifiedPublicationEqualityFullDurableOracle(t *testing.T) {
	for _, scenario := range []string{"same-day-full", "clock", "source-correction", "missing-day", "changed-result"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), "1")
			*f.now = f.now.Add(time.Hour)
			switch scenario {
			case "clock":
				f.day(t, "2026-10-13")
			case "source-correction":
				if _, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='101'`); err != nil {
					t.Fatal(err)
				}
			case "missing-day":
				if _, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_inputs WHERE on_date='2026-10-01'`); err != nil {
					t.Fatal(err)
				}
			case "changed-result":
				if _, err := f.db.SQL.Exec(`UPDATE investment_review_daily_results SET evidence_json='{}' WHERE on_date='2026-10-01'`); err != nil {
					t.Fatal(err)
				}
			}
			reference := windowOracleReference(t, f) // independent forced full before candidate writes
			p := certifiedStage(t, f, r)
			assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, p)
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, reference)) || !reflect.DeepEqual(publicationDailyTimes(t, f.db), publicationDailyTimes(t, reference)) {
				t.Fatal("daily publication differs from independent full durable oracle/timestamps")
			}
			if scenario == "missing-day" || scenario == "changed-result" {
				assertNoCertifiedMAC(t, f)
			} else {
				requireCertifiedNoop(t, f, r)
			}
		})
	}
}

// The injected clock runs after comparison rows are read. The comparison must
// still use the actual SQL arguments, rather than an earlier equality stamp.
func TestReviewCertifiedPublicationEqualityRechecksActualArguments(t *testing.T) {
	f, r := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), "1")
	*f.now = f.now.Add(time.Hour)
	p := certifiedStage(t, f, r)
	clock := func() time.Time {
		p.Rules[0].Days[0].Result.Truth = domain.ReviewFalse
		return *f.now
	}
	if err := certifiedPublish(t.Context(), f, r, p, clock); err != nil {
		t.Fatal(err)
	}
	var truth string
	if err := f.db.SQL.QueryRow(`SELECT predicate_truth FROM investment_review_daily_results WHERE on_date=?`, p.Rules[0].Days[0].On).Scan(&truth); err != nil || truth != "false" {
		t.Fatal("earlier equality suppressed the actual changed result", truth, err)
	}
}

func TestReviewCertifiedPublicationEqualityRepeatedKeyRestoresOriginal(t *testing.T) {
	f, r := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), "1")
	*f.now = f.now.Add(time.Hour)
	p := certifiedStage(t, f, r)
	original := p.Rules[0].Days[0]
	changed := original
	changed.Result.Truth = domain.ReviewFalse
	p.Rules[0].Days = append([]domain.ReviewPublicationDay{changed, original}, p.Rules[0].Days[1:]...)
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	var truth, revised string
	if err := f.db.SQL.QueryRow(`SELECT predicate_truth,revised_at FROM investment_review_daily_results WHERE on_date=?`, original.On).Scan(&truth, &revised); err != nil || truth != string(original.Result.Truth) || revised != f.now.UTC().Format(time.RFC3339Nano) {
		t.Fatal("a consumed original image suppressed the later restoration", truth, revised, err)
	}
}
