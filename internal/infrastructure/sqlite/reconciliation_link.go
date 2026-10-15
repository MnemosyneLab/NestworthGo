package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/domain"
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
	err := tx.QueryRowContext(ctx, `SELECT s.status,s.revision,r.revision FROM reconciliation_sessions s JOIN reconciliation_accounts a ON a.session_id=s.id JOIN account_financial_revisions r ON r.account_id=a.account_id JOIN reconciliation_items i ON i.session_id=a.session_id AND i.account_id=a.account_id WHERE s.id=? AND a.account_id=? AND i.id=?`, l.SessionID, l.AccountID, l.ItemID).Scan(&status, &revision, &financial)
	if err != nil {
		return reconciliationConflict("reconciliationLink")
	}
	if status != "in_progress" || revision != l.ExpectedRevision || financial != l.FinancialRevision {
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
		if account == l.AccountID {
			affected = true
		}
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
	if !affected {
		return reconciliationConflict("reconciliationLink.accountId")
	}
	return saveReconciliationLinkTx(ctx, tx, commit.Activity.ID.String(), "activity", map[string]any{"activityId": commit.Activity.ID.String(), "kind": commit.Activity.Kind, "reason": commit.Activity.Reason, "effectiveAt": commit.Activity.EffectiveAt, "recordedAt": commit.Activity.CreatedAt, "note": commit.Activity.Note, "effects": effects, "resulting": commit.Resulting})
}
func saveReconciliationLinkTx(ctx context.Context, tx *sql.Tx, operation, family string, after any) error {
	l := domain.ReconciliationLinkFrom(ctx)
	if l == nil {
		return nil
	}
	var before string
	if err := tx.QueryRowContext(ctx, "SELECT evidence FROM reconciliation_items WHERE session_id=? AND account_id=? AND id=?", l.SessionID, l.AccountID, l.ItemID).Scan(&before); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{"reviewedBefore": json.RawMessage(before), "committed": after})
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
