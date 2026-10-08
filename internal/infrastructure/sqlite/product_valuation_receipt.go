package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Legacy GUI observations have only quoteId. Guarded observations additionally
// carry the normalized command and immutable receipt; verify both against facts.
func verifyProductValuationReceipts(ctx context.Context, query schemaQuery) error {
	rows, err := query.QueryContext(ctx, `SELECT o.id,o.payload_sha256,o.request_json,o.result_json,o.effective_at,o.created_at,c.id,c.currency,q.id,q.instrument_id,c.instrument_id,q.unit_price,q.currency,q.quoted_at,q.created_at
 FROM product_operations o JOIN product_operation_products l ON l.operation_id=o.id
 JOIN product_contracts c ON c.id=l.product_id
 LEFT JOIN instrument_quotes q ON q.id=json_extract(o.result_json,'$.quoteId')
 WHERE o.kind='value_observation' AND (json_type(o.request_json,'$.command') IS NOT NULL OR json_type(o.result_json,'$.receipt') IS NOT NULL)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, digest, request, result, effective, created, product, currency string
		var quoteID, instrument, contractInstrument, amount, quoteCurrency, quoted, quoteCreated *string
		if err := rows.Scan(&id, &digest, &request, &result, &effective, &created, &product, &currency, &quoteID, &instrument, &contractInstrument, &amount, &quoteCurrency, &quoted, &quoteCreated); err != nil {
			return err
		}
		var req struct {
			Kind    string `json:"kind"`
			Command struct {
				ProductID  string `json:"productId"`
				Amount     string `json:"amount"`
				ObservedAt string `json:"observedAt"`
			} `json:"command"`
		}
		var res struct {
			QuoteID string                         `json:"quoteId"`
			Receipt domain.ProductValuationReceipt `json:"receipt"`
		}
		invalid := func() error { return storedIntegrity("product", "valuation receipt disagrees with recorded facts") }
		if json.Unmarshal([]byte(request), &req) != nil || json.Unmarshal([]byte(result), &res) != nil || res.Receipt.Validate() != nil {
			return invalid()
		}
		raw, err := json.Marshal(req.Command)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(raw)
		r := res.Receipt
		if req.Kind != "value_observation" || hex.EncodeToString(hash[:]) != digest || req.Command.ProductID != product || r.ProductID.String() != product || r.OperationID.String() != id || r.Amount.Currency().String() != currency || r.Replayed || quoteID == nil || instrument == nil || contractInstrument == nil || amount == nil || quoteCurrency == nil || quoted == nil || quoteCreated == nil || *instrument != *contractInstrument || *quoteCurrency != currency || res.QuoteID != r.QuoteID.String() || *quoteID != res.QuoteID {
			return invalid()
		}
		observed, err := time.Parse(time.RFC3339Nano, req.Command.ObservedAt)
		if err != nil || !observed.Equal(r.ObservedAt) {
			return invalid()
		}
		money, err := domain.ParseMoney(req.Command.Amount, domain.CurrencyCode(currency))
		if err != nil || !money.Amount().Equal(r.Amount.Amount()) {
			return invalid()
		}
		qMoney, err := domain.ParseMoney(*amount, domain.CurrencyCode(currency))
		if err != nil || !qMoney.Amount().Equal(r.Amount.Amount()) {
			return invalid()
		}
		for _, pair := range []struct {
			raw  string
			want time.Time
		}{{effective, r.ObservedAt}, {created, r.RecordedAt}, {*quoted, r.ObservedAt}, {*quoteCreated, r.RecordedAt}} {
			v, err := time.Parse(time.RFC3339Nano, pair.raw)
			// Preserve original command/receipt bytes for older observations;
			// SQL facts retain only UTC milliseconds. All other evidence and
			// the original command-to-receipt instant remain checked above.
			if err != nil || !v.Equal(productFactTime(pair.want)) {
				return invalid()
			}
		}
	}
	return rows.Err()
}
