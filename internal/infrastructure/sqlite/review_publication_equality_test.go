package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/domain"
	"reflect"
	"testing"
)

func publicationEqualityFixture(t *testing.T, values []any, result bool) *reviewPublicationEquality {
	t.Helper()
	key, err := appendCertifiedRow(nil, values[:5])
	if err != nil {
		t.Fatal(err)
	}
	image := append([]any(nil), values[:len(values)-1]...)
	e := &reviewPublicationEquality{inputs: map[string][]any{}, results: map[string][]any{}}
	if result {
		e.results[string(key)] = image
	} else {
		e.inputs[string(key)] = image
	}
	return e
}

func TestReviewPublicationEqualityRequiresEveryParameter(t *testing.T) {
	for _, result := range []bool{false, true} {
		values := []any{"household", "instrument", "UTC/v1", "2026-10-01", "rule:r/v:1", "1", "CNY", "stock", "100", "50", `{"exactFX":"2"}`, `[]`, "fingerprint", "old-time"}
		if result {
			values = []any{"rule", int64(1), "instrument", "UTC/v1", "2026-10-01", "true", nil, `{"truth":"true"}`, "fingerprint", "old-time"}
		}
		for column := range values {
			t.Run(fmt.Sprintf("result=%v/column=%d", result, column), func(t *testing.T) {
				e := publicationEqualityFixture(t, values, result)
				changed := append([]any(nil), values...)
				switch value := changed[column].(type) {
				case int64:
					changed[column] = value + 1
				default:
					changed[column] = fmt.Sprint(value) + " changed"
				}
				var omit bool
				var err error
				if result {
					omit, err = e.omitResult(changed)
				} else {
					omit, err = e.omitInput(changed)
				}
				if err != nil || omit != (column == len(values)-1) {
					t.Fatal("non-timestamp parameter mismatch was omitted", omit, err)
				}
			})
		}
	}
}

func TestReviewPublicationEqualityConsumesImagesAfterMismatchAndEquality(t *testing.T) {
	values := []any{"rule", int64(1), "instrument", "UTC/v1", "2026-10-01", "true", nil, `{"truth":"true"}`, "fp", "at"}
	for _, changedFirst := range []bool{false, true} {
		e := publicationEqualityFixture(t, values, true)
		first := append([]any(nil), values...)
		if changedFirst {
			first[7] = `{"truth":"false"}`
		}
		if omit, err := e.omitResult(first); err != nil || omit == changedFirst {
			t.Fatal("incorrect first comparison", omit, err)
		}
		if omit, err := e.omitResult(values); err != nil || omit {
			t.Fatal("repeated key reused an obsolete image", omit, err)
		}
	}
	for _, replacement := range []any{"", []byte{}, "null"} {
		e := publicationEqualityFixture(t, values, true)
		changed := append([]any(nil), values...)
		changed[6] = replacement
		if omit, err := e.omitResult(changed); err != nil || omit {
			t.Fatal("SQL NULL was equated with a non-NULL value", omit, err)
		}
	}
	if omit, err := (*reviewPublicationEquality)(nil).omitResult(values); err != nil || omit {
		t.Fatal("unknown provenance enabled omission", omit, err)
	}
}

// Exercise ordinary SQL reads across chunks, gaps and every key dimension.
// No certificate/authority is created by this standalone reader fixture.
func TestReviewPublicationEqualityRequestedScopes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE TABLE investment_review_rules(id TEXT PRIMARY KEY,household_id TEXT)`,
		`CREATE TABLE investment_review_daily_inputs(household_id TEXT,instrument_id TEXT,timezone_version TEXT,on_date TEXT,input_policy TEXT,quantity TEXT,native_currency TEXT,instrument_type TEXT,amount_exact TEXT,cost_exact TEXT,evidence_json TEXT,coverage_json TEXT,input_fingerprint TEXT,PRIMARY KEY(household_id,instrument_id,timezone_version,on_date,input_policy))`,
		`CREATE TABLE investment_review_daily_results(rule_id TEXT,condition_version INTEGER,instrument_id TEXT,timezone_version TEXT,on_date TEXT,predicate_truth TEXT,reason TEXT,evidence_json TEXT,input_fingerprint TEXT,PRIMARY KEY(rule_id,condition_version,instrument_id,timezone_version,on_date))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	household := domain.NewHouseholdID()
	instrument := domain.NewInstrumentID()
	var requested []domain.ReviewRulePublication
	expectedInputs, expectedResults := map[string][]any{}, map[string][]any{}
	insert := func(rule domain.ReviewRuleID, version int, inst domain.InstrumentID, tz, on string, wanted bool) {
		t.Helper()
		input := []any{household.String(), inst.String(), tz, on, fmt.Sprintf("rule:%s/v:%d", rule, version), nil, "CNY", "stock", "1.00000000000000001", nil, `{"fx":"0.123456789123456789"}`, `[]`, "fp"}
		result := []any{string(rule), int64(version), inst.String(), tz, on, "unknown", nil, `{ "exactFX": "1.00000000001" }`, "fp"}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO investment_review_rules VALUES(?,?)`, string(rule), household.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO investment_review_daily_inputs VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, input...); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO investment_review_daily_results VALUES(?,?,?,?,?,?,?,?,?)`, result...); err != nil {
			t.Fatal(err)
		}
		if wanted {
			ik, _ := appendCertifiedRow(nil, input[:5])
			rk, _ := appendCertifiedRow(nil, result[:5])
			expectedInputs[string(ik)] = input
			expectedResults[string(rk)] = result
		}
	}
	rule := domain.NewReviewRuleID()
	request := domain.ReviewRulePublication{RuleID: rule, ConditionVersion: 1, InstrumentID: instrument, TimezoneVersion: "UTC/v1"}
	for _, on := range []string{"2026-10-03", "2026-10-01", "2026-10-02", "2026-10-01", "2026-10-06"} {
		request.Days = append(request.Days, domain.ReviewPublicationDay{On: on})
	}
	requested = append(requested, request, request) // duplicate scopes/dates are deduplicated
	for _, on := range []string{"2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-07"} {
		insert(rule, 1, instrument, "UTC/v1", on, on <= "2026-10-03")
	}
	insert(rule, 2, instrument, "UTC/v1", "2026-10-01", false)
	insert(rule, 1, instrument, "UTC/v2", "2026-10-01", false)
	insert(rule, 1, domain.NewInstrumentID(), "UTC/v1", "2026-10-01", false)
	insert(domain.NewReviewRuleID(), 1, instrument, "UTC/v1", "2026-10-01", false)
	// More than one chunk; no omission of the last chunk or duplicate row images.
	for i := 0; i < 151; i++ {
		r := domain.NewReviewRuleID()
		insert(r, 1, instrument, "UTC/v1", "2026-10-02", true)
		requested = append(requested, domain.ReviewRulePublication{RuleID: r, ConditionVersion: 1, InstrumentID: instrument, TimezoneVersion: "UTC/v1", Days: []domain.ReviewPublicationDay{{On: "2026-10-02"}}})
	}
	// Same input key in another household must not enter this image set.
	if _, err := tx.Exec(`INSERT INTO investment_review_daily_inputs SELECT ?,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint FROM investment_review_daily_inputs WHERE household_id=?`, domain.NewHouseholdID().String(), household.String()); err != nil {
		t.Fatal(err)
	}
	ranges, err := reviewPublicationEqualityRanges(requested)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []bool{false, true} {
		query, args := reviewEqualityQuery(household, ranges[:min(reviewEqualityChunk, len(ranges))], result)
		if len(args) > 999 {
			t.Fatal("SQLite parameter limit exceeded", len(args))
		}
		rows, err := tx.Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var node, parent, unused int
			var detail string
			if err := rows.Scan(&node, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			t.Logf("result=%v plan: %s", result, detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	equality, err := loadReviewPublicationEquality(ctx, tx, household, requested)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(equality.inputs, expectedInputs) || !reflect.DeepEqual(equality.results, expectedResults) {
		t.Fatalf("scope images differ: input=%d/%d result=%d/%d", len(equality.inputs), len(expectedInputs), len(equality.results), len(expectedResults))
	}
	var oldInputs, oldResults int
	if err := tx.QueryRow(`SELECT count(*) FROM investment_review_daily_inputs WHERE household_id=? AND on_date>=?`, household.String(), "2026-10-01").Scan(&oldInputs); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? AND d.on_date>=?`, household.String(), "2026-10-01").Scan(&oldResults); err != nil {
		t.Fatal(err)
	}
	t.Logf("global-minimum reader images inputs/results=%d/%d; exact scopes=%d/%d (empty/missing/duplicate/gapped/multichunk requests checked)", oldInputs, oldResults, len(equality.inputs), len(equality.results))
	missing := []any{string(rule), int64(1), instrument.String(), "UTC/v1", "2026-10-06", "unknown", nil, `{}`, "fp", "timestamp"}
	if omit, err := equality.omitResult(missing); err != nil || omit {
		t.Fatal("absent row must UPSERT", omit, err)
	}
	empty, err := loadReviewPublicationEquality(ctx, tx, household, nil)
	if err != nil || len(empty.inputs)+len(empty.results) != 0 {
		t.Fatal("empty request", empty, err)
	}
}
