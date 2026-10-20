package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Private to one reserved publication transaction. These are SQL value images for
// deciding whether an UPSERT would affect zero rows, never computation inputs.
// Eligibility and authenticated provenance are checked by the caller first.
type reviewPublicationEquality struct {
	inputs, results map[string][]any
	keybuf          []byte
}

func loadReviewPublicationEquality(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, minimumDay string) (*reviewPublicationEquality, error) {
	e := &reviewPublicationEquality{inputs: map[string][]any{}, results: map[string][]any{}}
	queries := []struct {
		sql    string
		images map[string][]any
	}{
		{`SELECT household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint FROM investment_review_daily_inputs WHERE household_id=? AND on_date>=? ORDER BY instrument_id,timezone_version,on_date,input_policy`, e.inputs},
		{`SELECT d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date,d.predicate_truth,d.reason,d.evidence_json,d.input_fingerprint FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? AND d.on_date>=? ORDER BY d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date`, e.results},
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, query.sql, id.String(), minimumDay)
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
			query.images[string(e.keybuf)] = image
		}
		if err = errors.Join(err, rows.Err(), rows.Close()); err != nil {
			return nil, err
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
