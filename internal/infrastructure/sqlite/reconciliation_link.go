package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Invoked before ANY financial side effect in WithTx. All owning financial
// repositories use this transaction boundary, so linkage fails closed.
func validateReconciliationLinkTx(ctx context.Context, tx *sql.Tx) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	var status string
	var revision, financial int64
	err := tx.QueryRowContext(ctx, `SELECT s.status,s.revision,r.revision FROM reconciliation_sessions s JOIN reconciliation_accounts a ON a.session_id=s.id JOIN account_financial_revisions r ON r.account_id=a.account_id WHERE s.id=? AND a.account_id=?`, l.SessionID, l.AccountID).Scan(&status, &revision, &financial)
	if err != nil {
		return reconciliationConflict("reconciliationLink")
	}
	if status != "in_progress" || revision != l.ExpectedRevision || financial != l.FinancialRevision {
		return reconciliationConflict("reconciliationLink")
	}
	if item := domain.ReconciliationWorkingItemFrom(ctx); item != nil {
		if item.ID != l.ItemID || item.AccountID != l.AccountID || item.FinancialRevision != l.FinancialRevision {
			return reconciliationConflict("reconciliationLink")
		}
		raw, err := jsoncodec.Marshal(item)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO reconciliation_items(session_id,account_id,id,evidence) VALUES(?,?,?,?) ON CONFLICT(session_id,account_id,id) DO UPDATE SET evidence=excluded.evidence", l.SessionID, l.AccountID, l.ItemID, string(raw)); err != nil {
			return err
		}
	}
	var present int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM reconciliation_items WHERE session_id=? AND account_id=? AND id=?", l.SessionID, l.AccountID, l.ItemID).Scan(&present); err != nil {
		return err
	}
	if present != 1 {
		return reconciliationConflict("reconciliationLink")
	}
	return nil
}
func linkReconciliationActivityTx(ctx context.Context, tx *sql.Tx, commit domain.ActivityCommit) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	var household string
	if err := tx.QueryRowContext(ctx, "SELECT household_id FROM reconciliation_sessions WHERE id=?", l.SessionID).Scan(&household); err != nil {
		return err
	}
	if household != commit.Activity.HouseholdID.String() {
		return reconciliationConflict("reconciliationLink")
	}
	item := domain.ReconciliationWorkingItemFrom(ctx)
	if item == nil {
		return reconciliationConflict("reconciliationLink")
	}
	affected := false
	effects := []map[string]any{}
	for _, e := range commit.Effects {
		account := ""
		if e.AccountID != nil {
			account = e.AccountID.String()
		}
		if account == "" && e.HoldingID != nil {
			if err := tx.QueryRowContext(ctx, "SELECT account_id FROM holdings WHERE id=?", e.HoldingID.String()).Scan(&account); err != nil {
				return err
			}
		}
		affected = affected || domain.ReconciliationEffectMatches(*item, account, e)
		effect := map[string]any{"accountId": account, "target": e.Target, "direction": e.Direction}
		if e.Money != nil {
			effect["amount"] = e.Money.CanonicalAmount()
			effect["currency"] = e.Money.Currency().String()
		}
		if e.Quantity != nil {
			effect["quantity"] = e.Quantity.Canonical()
		}
		if e.CostUnitPrice != nil {
			effect["unitCost"] = e.CostUnitPrice.Canonical()
		}
		effects = append(effects, effect)
	}
	return saveReconciliationLinkTx(ctx, tx, commit.Activity.ID.String(), "activity", map[string]any{"activityId": commit.Activity.ID.String(), "kind": commit.Activity.Kind, "reason": commit.Activity.Reason, "effectiveAt": commit.Activity.EffectiveAt, "recordedAt": commit.Activity.CreatedAt, "note": commit.Activity.Note, "effects": effects, "resulting": commit.Resulting}, affected)
}
func saveReconciliationLinkTx(ctx context.Context, tx *sql.Tx, operation, family string, after any, matches ...bool) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	var before string
	if err := tx.QueryRowContext(ctx, "SELECT evidence FROM reconciliation_items WHERE session_id=? AND account_id=? AND id=?", l.SessionID, l.AccountID, l.ItemID).Scan(&before); err != nil {
		return err
	}
	var reviewed domain.ReconciliationItem
	if err := jsoncodec.Unmarshal([]byte(before), &reviewed); err != nil {
		return err
	}
	reviewed.Attempts = nil
	matched := len(matches) == 0 || matches[0]
	raw, err := jsoncodec.Marshal(map[string]any{"reviewedBefore": reviewed, "committed": after, "targetMatched": matched, "sessionRevision": l.ExpectedRevision})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO reconciliation_links(session_id,account_id,item_id,operation_id,family,evidence) VALUES(?,?,?,?,?,?)", l.SessionID, l.AccountID, l.ItemID, operation, family, string(raw))
	return err
}
func reconciliationLinkCountTx(ctx context.Context, tx *sql.Tx) (int, error) {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return 0, nil
	}
	var count int
	err := tx.QueryRowContext(ctx, "SELECT count(*) FROM reconciliation_links WHERE session_id=? AND account_id=?", l.SessionID, l.AccountID).Scan(&count)
	return count, err
}
func finishReconciliationLinkTx(ctx context.Context, tx *sql.Tx, priorCount int) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	// Any unsupported write path must roll back rather than create an unlinked correction.
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM reconciliation_links WHERE session_id=? AND account_id=?", l.SessionID, l.AccountID).Scan(&count); err != nil {
		return err
	}
	if count <= priorCount {
		return reconciliationConflict("reconciliationLink")
	}
	var matched int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM reconciliation_links WHERE session_id=? AND account_id=? AND json_extract(evidence,'$.sessionRevision')=? AND json_extract(evidence,'$.targetMatched')=1", l.SessionID, l.AccountID, l.ExpectedRevision).Scan(&matched); err != nil {
		return err
	}
	if matched == 0 {
		return reconciliationConflict("reconciliationLink")
	}
	result, err := tx.ExecContext(ctx, "UPDATE reconciliation_sessions SET revision=revision+1 WHERE id=? AND revision=? AND status='in_progress'", l.SessionID, l.ExpectedRevision)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return reconciliationConflict("reconciliationLink")
	}
	return nil
}

func linkReconciliationProductTx(ctx context.Context, tx *sql.Tx, bundle domain.ProductBundle) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	var household string
	if err := tx.QueryRowContext(ctx, "SELECT household_id FROM reconciliation_sessions WHERE id=?", l.SessionID).Scan(&household); err != nil {
		return err
	}
	if household != bundle.Operation.HouseholdID.String() {
		return reconciliationConflict("reconciliationLink")
	}
	item := domain.ReconciliationWorkingItemFrom(ctx)
	if item == nil {
		return reconciliationConflict("reconciliationLink")
	}
	matchesProduct := func(account, id string) bool {
		return account == l.AccountID && (item.Kind == "scope" || item.Kind == "product" && item.ProductID == id)
	}
	affected := false
	for _, p := range bundle.Contracts {
		if matchesProduct(p.AccountID.String(), p.ID.String()) {
			affected = true
		}
	}
	for _, p := range bundle.ProductLinks {
		var account string
		err := tx.QueryRowContext(ctx, "SELECT account_id FROM product_contracts WHERE id=?", p.ProductID.String()).Scan(&account)
		if err == nil && matchesProduct(account, p.ProductID.String()) {
			affected = true
		}
	}
	// A product receipt can also correct the selected cash currency. Other
	// operation legs remain companions, never substitute for the selected target.
	for _, activity := range bundle.Activities {
		for _, effect := range activity.Effects {
			account := ""
			if effect.AccountID != nil {
				account = effect.AccountID.String()
			}
			if account == "" && effect.HoldingID != nil {
				for _, holding := range bundle.Holdings {
					if holding.ID == *effect.HoldingID {
						account = holding.AccountID.String()
					}
				}
				if account == "" {
					if err := tx.QueryRowContext(ctx, "SELECT account_id FROM holdings WHERE id=?", effect.HoldingID.String()).Scan(&account); err != nil {
						return err
					}
				}
			}
			affected = affected || domain.ReconciliationEffectMatches(*item, account, effect)
		}
	}
	if !affected {
		return reconciliationConflict("reconciliationLink")
	}
	return saveReconciliationLinkTx(ctx, tx, bundle.Operation.ID.String(), "product_operation", map[string]any{"kind": bundle.Operation.Kind, "request": json.RawMessage(bundle.Operation.RequestJSON), "result": json.RawMessage(bundle.Operation.ResultJSON), "contracts": bundle.Contracts, "effectiveAt": bundle.Operation.EffectiveAt, "recordedAt": bundle.Operation.CreatedAt})
}
