package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Capture complete provenance, including unrelated instrument rows and inactive
// targets. Only append-only records independently belonging to the proved FX
// transition can change. No source text is persisted in the certificate.
func captureReviewFXProvenance(ctx context.Context, tx *sql.Tx, household domain.HouseholdID, description *domain.ReviewFXDescription) error {
	if description == nil || !description.Eligible {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,request_key,household_id,target_type,operation,quote_id,target_quote_id,source_title,source_url,created_at FROM agent_quote_records WHERE household_id=? ORDER BY id`, household.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	description.Provenance = []domain.ReviewFXProvenanceSeal{}
	values, ptrs := make([]any, 10), make([]any, 10)
	for i := range values {
		ptrs[i] = &values[i]
	}
	var rowbuf []byte
	for rows.Next() {
		if err = rows.Scan(ptrs...); err != nil {
			return err
		}
		// Reuse the complete row-seal encoder: exact bytes, storage classes and
		// SQL NULL are retained, including invalid UTF-8 in unrelated metadata.
		rowbuf, err = appendCertifiedRow(rowbuf[:0], values)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(rowbuf)
		scope := make([]string, 7)
		for i := range scope {
			if values[i] == nil && (i == 5 || i == 6) {
				continue
			}
			value, ok := values[i].(string)
			if !ok {
				description.Eligible = false
				return nil
			}
			scope[i] = value
		}
		description.Provenance = append(description.Provenance, domain.ReviewFXProvenanceSeal{ID: scope[0], TargetType: scope[3], Operation: scope[4], QuoteID: domain.FXQuoteID(scope[5]), TargetQuoteID: domain.FXQuoteID(scope[6]), Fingerprint: hex.EncodeToString(digest[:])})
	}
	if err = rows.Err(); err != nil {
		return err
	}
	description.ProvenanceVersion = domain.ReviewFXProvenanceVersion
	return nil
}
