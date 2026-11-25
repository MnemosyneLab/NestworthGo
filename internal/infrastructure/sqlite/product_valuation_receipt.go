package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Legacy GUI observations have only quoteId. Guarded observations additionally
// carry the normalized command and immutable receipt; verify both against facts.
func verifyProductValuationReceipts(ctx context.Context, query schemaQuery) error {
	rows, err := query.QueryContext(ctx, `SELECT o.id,o.payload_sha256,o.request_json,o.result_json,o.effective_at,o.created_at,c.id,c.currency,q.id,q.instrument_id,c.instrument_id,q.unit_price,q.currency,q.quoted_at,q.created_at,
 c.account_id,o.household_id,rl.session_id,rl.account_id,rl.item_id,s.household_id,rl.evidence
 FROM product_operations o JOIN product_operation_products l ON l.operation_id=o.id
 JOIN product_contracts c ON c.id=l.product_id
 LEFT JOIN instrument_quotes q ON q.id=json_extract(o.result_json,'$.quoteId')
 LEFT JOIN reconciliation_links rl ON rl.operation_id=o.id AND rl.family='product_operation'
 LEFT JOIN reconciliation_sessions s ON s.id=rl.session_id
 WHERE o.kind='value_observation' AND (json_type(o.request_json,'$.command') IS NOT NULL OR json_type(o.result_json,'$.receipt') IS NOT NULL OR rl.operation_id IS NOT NULL)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	linkedOperations := make(map[string]struct{})
	for rows.Next() {
		var id, digest, request, result, effective, created, product, currency string
		var quoteID, instrument, contractInstrument, amount, quoteCurrency, quoted, quoteCreated *string
		var productAccount, household string
		var link persistedValuationLink
		if err := rows.Scan(&id, &digest, &request, &result, &effective, &created, &product, &currency, &quoteID, &instrument, &contractInstrument, &amount, &quoteCurrency, &quoted, &quoteCreated,
			&productAccount, &household, &link.session, &link.account, &link.item, &link.household, &link.evidence); err != nil {
			return err
		}
		var req struct {
			Kind    string          `json:"kind"`
			Command json.RawMessage `json:"command"`
		}
		var command struct {
			ReconciliationLink *domain.ReconciliationLink `json:"reconciliationLink,omitempty"`
			ProductID          string                     `json:"productId"`
			Amount             string                     `json:"amount"`
			ObservedAt         string                     `json:"observedAt"`
		}
		var res struct {
			QuoteID string                         `json:"quoteId"`
			Receipt domain.ProductValuationReceipt `json:"receipt"`
		}
		invalid := func() error { return storedIntegrity("product", "valuation receipt disagrees with recorded facts") }
		if jsoncodec.Unmarshal([]byte(request), &req) != nil || jsoncodec.Unmarshal(req.Command, &command) != nil ||
			jsoncodec.Unmarshal([]byte(result), &res) != nil || res.Receipt.Validate() != nil {
			return invalid()
		}
		raw, err := jsoncodec.Marshal(command)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(raw)
		r := res.Receipt
		if req.Kind != "value_observation" || hex.EncodeToString(hash[:]) != digest || command.ProductID != product || r.ProductID.String() != product || r.OperationID.String() != id || r.Amount.Currency().String() != currency || r.Replayed || quoteID == nil || instrument == nil || contractInstrument == nil || amount == nil || quoteCurrency == nil || quoted == nil || quoteCreated == nil || *instrument != *contractInstrument || *quoteCurrency != currency || res.QuoteID != r.QuoteID.String() || *quoteID != res.QuoteID {
			return invalid()
		}
		observed, err := time.Parse(time.RFC3339Nano, command.ObservedAt)
		if err != nil || !observed.Equal(r.ObservedAt) {
			return invalid()
		}
		commandAmount := command.Amount
		if command.ReconciliationLink != nil {
			// The writer trims the amount for valuation but seals its literal
			// text. Check that digest before applying the same parsing rule.
			commandAmount = strings.TrimSpace(commandAmount)
		}
		money, err := domain.ParseMoney(commandAmount, domain.CurrencyCode(currency))
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
		if !link.matches(command.ReconciliationLink, household, productAccount, request, result, r) {
			return invalid()
		}
		if command.ReconciliationLink != nil {
			// A linked observation has one normalized command and one joined
			// product link. Reject ambiguous encodings and extra join rows.
			if _, duplicate := linkedOperations[id]; duplicate || !bytes.Equal(req.Command, raw) {
				return invalid()
			}
			linkedOperations[id] = struct{}{}
		}
	}
	return rows.Err()
}

type persistedValuationLink struct {
	session, account, item, household, evidence *string
}

func (l persistedValuationLink) matches(command *domain.ReconciliationLink, household, productAccount, request, result string, receipt domain.ProductValuationReceipt) bool {
	if command == nil {
		return l.session == nil
	}
	if l.session == nil || l.account == nil || l.item == nil || l.household == nil || l.evidence == nil ||
		*l.session != command.SessionID || *l.account != command.AccountID || *l.item != command.ItemID ||
		*l.household != household || *l.account != productAccount || command.ExpectedRevision < 1 || command.FinancialRevision < 1 {
		return false
	}
	// The session can advance after the write. Compare the saved checkpoint
	// in the atomic link record with the original command and receipt.
	var evidence struct {
		Before          domain.ReconciliationItem `json:"reviewedBefore"`
		TargetMatched   bool                      `json:"targetMatched"`
		SessionRevision int64                     `json:"sessionRevision"`
		Committed       struct {
			Kind        domain.ProductOperationKind `json:"kind"`
			Request     json.RawMessage             `json:"request"`
			Result      json.RawMessage             `json:"result"`
			EffectiveAt time.Time                   `json:"effectiveAt"`
			RecordedAt  time.Time                   `json:"recordedAt"`
		} `json:"committed"`
	}
	if jsoncodec.Unmarshal([]byte(*l.evidence), &evidence) != nil {
		return false
	}
	before, committed := evidence.Before, evidence.Committed
	return evidence.TargetMatched && evidence.SessionRevision == command.ExpectedRevision &&
		before.ID == command.ItemID && before.AccountID == command.AccountID && before.FinancialRevision == command.FinancialRevision &&
		(before.Kind == "scope" || before.Kind == "product" && before.ProductID == receipt.ProductID.String()) &&
		committed.Kind == domain.ProductOpValueObservation && bytes.Equal(committed.Request, []byte(request)) && bytes.Equal(committed.Result, []byte(result)) &&
		committed.EffectiveAt.Equal(receipt.ObservedAt) && committed.RecordedAt.Equal(receipt.RecordedAt)
}
