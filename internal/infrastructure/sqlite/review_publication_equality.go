package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Private to one reserved publication transaction. These are SQL value images for
// deciding whether an UPSERT would affect zero rows, never computation inputs.
// Eligibility and authenticated provenance are checked by the caller first.
type reviewPublicationEquality struct {
	inputs, results map[string][]any
	keybuf          []byte
}

// Each request range is disjoint within its full SQL namespace. Date gaps
// remain gaps; repeated publication keys request only one original image.
type reviewEqualityScope struct {
	rule, instrument, timezone string
	version                    int
}
type reviewEqualityRange struct {
	scope         reviewEqualityScope
	from, through string
}

func reviewPublicationEqualityRanges(rules []domain.ReviewRulePublication) ([]reviewEqualityRange, error) {
	dates := map[reviewEqualityScope]map[string]bool{}
	order := []reviewEqualityScope{}
	for _, rule := range rules {
		key := reviewEqualityScope{string(rule.RuleID), rule.InstrumentID.String(), rule.TimezoneVersion, rule.ConditionVersion}
		for _, day := range rule.Days {
			if dates[key] == nil {
				dates[key] = map[string]bool{}
				order = append(order, key)
			}
			dates[key][day.On] = true
		}
	}
	var ranges []reviewEqualityRange
	for _, key := range order {
		days := make([]string, 0, len(dates[key]))
		for day := range dates[key] {
			days = append(days, day)
		}
		sort.Strings(days)
		var next string
		for _, day := range days {
			parsed, err := time.Parse(time.DateOnly, day)
			if err != nil {
				return nil, err
			}
			if len(ranges) > 0 && next == day {
				ranges[len(ranges)-1].through = day
			} else {
				ranges = append(ranges, reviewEqualityRange{key, day, day})
			}
			next = parsed.AddDate(0, 0, 1).Format(time.DateOnly)
		}
	}
	return ranges, nil
}

// At most 901 parameters, below SQLite's conservative 999-variable limit.
const reviewEqualityChunk = 150

// CROSS JOIN keeps the small request relation outside the indexed row lookup;
// otherwise SQLite can choose a household-wide scan and repeatedly probe q.
func reviewEqualityQuery(id domain.HouseholdID, ranges []reviewEqualityRange, result bool) (string, []any) {
	values := make([]string, 0, len(ranges))
	args := make([]any, 0, 6*len(ranges)+1)
	for _, request := range ranges {
		k := request.scope
		if result {
			values = append(values, "(?,?,?,?,?,?)")
			args = append(args, k.rule, k.version, k.instrument, k.timezone, request.from, request.through)
		} else {
			values = append(values, "(?,?,?,?,?)")
			args = append(args, k.instrument, k.timezone, "rule:"+k.rule+"/v:"+strconv.Itoa(k.version), request.from, request.through)
		}
	}
	args = append(args, id.String())
	if result {
		return `WITH requested(rule_id,condition_version,instrument_id,timezone_version,from_day,through_day) AS (VALUES ` + strings.Join(values, ",") + `)
SELECT d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date,d.predicate_truth,d.reason,d.evidence_json,d.input_fingerprint
FROM requested q CROSS JOIN investment_review_daily_results d
JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? AND d.rule_id=q.rule_id AND d.condition_version=q.condition_version AND d.instrument_id=q.instrument_id AND d.timezone_version=q.timezone_version AND d.on_date BETWEEN q.from_day AND q.through_day
ORDER BY d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date`, args
	}
	return `WITH requested(instrument_id,timezone_version,input_policy,from_day,through_day) AS (VALUES ` + strings.Join(values, ",") + `)
SELECT d.household_id,d.instrument_id,d.timezone_version,d.on_date,d.input_policy,d.quantity,d.native_currency,d.instrument_type,d.amount_exact,d.cost_exact,d.evidence_json,d.coverage_json,d.input_fingerprint
FROM requested q CROSS JOIN investment_review_daily_inputs d WHERE d.household_id=? AND d.instrument_id=q.instrument_id AND d.timezone_version=q.timezone_version AND d.input_policy=q.input_policy AND d.on_date BETWEEN q.from_day AND q.through_day
ORDER BY d.instrument_id,d.timezone_version,d.on_date,d.input_policy`, args
}

func loadReviewPublicationEquality(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, rules []domain.ReviewRulePublication) (*reviewPublicationEquality, error) {
	e := &reviewPublicationEquality{inputs: map[string][]any{}, results: map[string][]any{}}
	ranges, err := reviewPublicationEqualityRanges(rules)
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(ranges); start += reviewEqualityChunk {
		chunk := ranges[start:min(start+reviewEqualityChunk, len(ranges))]
		for _, result := range []bool{false, true} {
			statement, args := reviewEqualityQuery(id, chunk, result)
			images := e.inputs
			if result {
				images = e.results
			}
			rows, err := tx.QueryContext(ctx, statement, args...)
			if err != nil {
				return nil, err
			}

			columns, err := rows.Columns()
			if err != nil {
				rows.Close()
				return nil, err
			}
			values, pointers := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(pointers...); err != nil {
					break
				}
				e.keybuf, err = appendCertifiedRow(e.keybuf[:0], values[:5])
				if err != nil {
					break
				}
				// Scan into *any owns byte slices; copy the tuple because the scan
				// destinations are reused. Strings (including JSON) are immutable,
				// so retaining them needs no second payload encoding/copy.
				image := append([]any(nil), values...)
				for i, value := range image {
					if blob, ok := value.([]byte); ok {
						image[i] = bytes.Clone(blob)
					}
				}
				images[string(e.keybuf)] = image
			}
			if err = errors.Join(err, rows.Err(), rows.Close()); err != nil {
				return nil, err
			}
		}
	}
	return e, nil
}

func (e *reviewPublicationEquality) omitInput(values []any) (bool, error) {
	if e == nil {
		return false, nil
	}
	return e.omit(e.inputs, values, 14)
}

func (e *reviewPublicationEquality) omitResult(values []any) (bool, error) {
	if e == nil {
		return false, nil
	}
	return e.omit(e.results, values, 10)
}

func (e *reviewPublicationEquality) omit(images map[string][]any, values []any, size int) (bool, error) {
	if len(values) != size {
		return false, fmt.Errorf("incomplete daily publication parameter image")
	}
	var err error
	e.keybuf, err = appendCertifiedRow(e.keybuf[:0], values[:5])
	if err != nil {
		return false, err
	}
	key := string(e.keybuf)
	stored, exists := images[key]
	// An earlier occurrence can change a repeated key before this one. Never
	// use an original snapshot image twice, including after a mismatch/write.
	delete(images, key)
	if !exists {
		return false, nil
	}
	// The insertion timestamp is the final argument. Equal payloads make the
	// existing UPSERT WHERE false, retaining computed/evaluated/revised times.
	// The verified columns and constructed parameters are strings, int64 or
	// NULL. Deep equality retains types and compares complete string/BLOB bytes;
	// it never substitutes a fingerprint or JSON-semantic comparison.
	return reflect.DeepEqual(stored, values[:len(values)-1]), nil
}
