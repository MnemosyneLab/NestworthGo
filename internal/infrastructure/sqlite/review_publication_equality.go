package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Private to one reserved publication transaction. These are byte images for
// deciding whether an UPSERT would affect zero rows, never computation inputs.
// Eligibility and authenticated provenance are checked by the caller first.
type reviewPublicationEquality struct {
	inputs, results map[string][]byte
	keybuf, rowbuf  []byte
}

func loadReviewPublicationEquality(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) (*reviewPublicationEquality, error) {
	e := &reviewPublicationEquality{inputs: map[string][]byte{}, results: map[string][]byte{}}
	queries := []struct {
		sql    string
		images map[string][]byte
	}{
		{`SELECT household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint FROM investment_review_daily_inputs WHERE household_id=? ORDER BY instrument_id,timezone_version,on_date,input_policy`, e.inputs},
		{`SELECT d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date,d.predicate_truth,d.reason,d.evidence_json,d.input_fingerprint FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? ORDER BY d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date`, e.results},
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, query.sql, id.String())
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
			var image []byte
			image, err = appendCertifiedRow(nil, values)
			if err != nil {
				break
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

func (e *reviewPublicationEquality) omit(images map[string][]byte, values []any, size int) (bool, error) {
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
	e.rowbuf, err = appendCertifiedRow(e.rowbuf[:0], values[:len(values)-1])
	if err != nil {
		return false, err
	}
	return bytes.Equal(stored, e.rowbuf), nil
}
