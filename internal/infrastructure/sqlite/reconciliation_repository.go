package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func reconciliationConflict(field string) error {
	return &domain.Error{Code: domain.ErrConflict, Field: field, Message: "reconciliation changed; read current evidence and review again"}
}
func (r *Repository) ReconciliationRevisions(ctx context.Context) (map[string]int64, error) {
	rows, err := r.database.SQL.QueryContext(ctx, "SELECT account_id,revision FROM account_financial_revisions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var id string
		var rev int64
		if err := rows.Scan(&id, &rev); err != nil {
			return nil, err
		}
		result[id] = rev
	}
	return result, rows.Err()
}
func (r *Repository) ReconciliationSession(ctx context.Context, household, id string) (domain.ReconciliationSession, error) {
	var s domain.ReconciliationSession
	var completed sql.NullString
	err := r.database.SQL.QueryRowContext(ctx, "SELECT id,household_id,status,revision,started_at,completed_at,note,partial FROM reconciliation_sessions WHERE household_id=? AND id=?", household, id).Scan(&s.ID, &s.HouseholdID, &s.Status, &s.Revision, &s.StartedAt, &completed, &s.Note, &s.Partial)
	if errors.Is(err, sql.ErrNoRows) {
		return s, &domain.Error{Code: domain.ErrNotFound, Field: "sessionId", Message: "reconciliation session was not found"}
	}
	if err != nil {
		return s, err
	}
	s.CompletedAt = completed.String
	rows, err := r.database.SQL.QueryContext(ctx, "SELECT evidence FROM reconciliation_accounts WHERE session_id=? ORDER BY account_id", id)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var raw string
		var a domain.ReconciliationAccount
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal([]byte(raw), &a)
		}
		if err != nil {
			rows.Close()
			return s, err
		}
		s.Accounts = append(s.Accounts, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return s, err
	}
	byID := map[string]int{}
	for i, a := range s.Accounts {
		byID[a.AccountID] = i
	}
	rows, err = r.database.SQL.QueryContext(ctx, "SELECT account_id,evidence FROM reconciliation_items WHERE session_id=? ORDER BY account_id,id", id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var aid, raw string
		var item domain.ReconciliationItem
		if err = rows.Scan(&aid, &raw); err == nil {
			err = json.Unmarshal([]byte(raw), &item)
		}
		if err != nil {
			return s, err
		}
		i := byID[aid]
		s.Accounts[i].Items = append(s.Accounts[i].Items, item)
	}
	return s, rows.Err()
}
func (r *Repository) ReconciliationSessions(ctx context.Context, household string) ([]domain.ReconciliationSession, error) {
	rows, err := r.database.SQL.QueryContext(ctx, "SELECT id,household_id,status,revision,started_at,COALESCE(completed_at,''),note,partial, (SELECT COALESCE(SUM(json_extract(evidence,'$.counts.pending')),0) FROM reconciliation_accounts a WHERE a.session_id=reconciliation_sessions.id), (SELECT COALESCE(SUM(json_extract(evidence,'$.counts.confirmed')),0) FROM reconciliation_accounts a WHERE a.session_id=reconciliation_sessions.id), (SELECT COALESCE(SUM(json_extract(evidence,'$.counts.skipped')),0) FROM reconciliation_accounts a WHERE a.session_id=reconciliation_sessions.id), (SELECT COALESCE(SUM(json_extract(evidence,'$.counts.unresolved')),0) FROM reconciliation_accounts a WHERE a.session_id=reconciliation_sessions.id) FROM reconciliation_sessions WHERE household_id=? ORDER BY started_at DESC,id DESC", household)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReconciliationSession{}
	for rows.Next() {
		var s domain.ReconciliationSession
		if err := rows.Scan(&s.ID, &s.HouseholdID, &s.Status, &s.Revision, &s.StartedAt, &s.CompletedAt, &s.Note, &s.Partial, &s.Counts.Pending, &s.Counts.Confirmed, &s.Counts.Skipped, &s.Counts.Unresolved); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
func (r *Repository) ReconciliationReceipt(ctx context.Context, household, operation, hash string) (*domain.ReconciliationSession, error) {
	var stored, raw string
	err := r.database.SQL.QueryRowContext(ctx, "SELECT payload_hash,result FROM reconciliation_receipts WHERE household_id=? AND operation_id=?", household, operation).Scan(&stored, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stored != hash {
		return nil, reconciliationConflict("operationId")
	}
	var result domain.ReconciliationSession
	err = json.Unmarshal([]byte(raw), &result)
	return &result, err
}
func (r *Repository) SaveReconciliation(ctx context.Context, s domain.ReconciliationSession, expected int64, revisions map[string]int64, operation, hash string) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		for id, rev := range revisions {
			var current int64
			if err := tx.QueryRowContext(ctx, "SELECT revision FROM account_financial_revisions WHERE account_id=?", id).Scan(&current); err != nil {
				return err
			}
			if current != rev {
				return reconciliationConflict("financialRevision")
			}
		}
		if expected == 0 {
			_, err := tx.ExecContext(ctx, "INSERT INTO reconciliation_sessions(id,household_id,status,revision,started_at,completed_at,note,partial) VALUES(?,?,?,?,?,NULL,?,?)", s.ID, s.HouseholdID, s.Status, s.Revision, s.StartedAt, s.Note, s.Partial)
			if err != nil {
				return err
			}
		} else {
			var completed any
			if s.CompletedAt != "" {
				completed = s.CompletedAt
			}
			result, err := tx.ExecContext(ctx, "UPDATE reconciliation_sessions SET status=?,revision=?,completed_at=?,note=?,partial=? WHERE id=? AND household_id=? AND revision=? AND status!='completed'", s.Status, s.Revision, completed, s.Note, s.Partial, s.ID, s.HouseholdID, expected)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				return reconciliationConflict("revision")
			}
		}
		for _, a := range s.Accounts {
			items := a.Items
			a.Items = nil
			raw, err := json.Marshal(a)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO reconciliation_accounts(session_id,account_id,evidence) VALUES(?,?,?) ON CONFLICT(session_id,account_id) DO UPDATE SET evidence=excluded.evidence", s.ID, a.AccountID, string(raw))
			if err != nil {
				return err
			}
			for _, item := range items {
				raw, err = json.Marshal(item)
				if err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, "INSERT INTO reconciliation_items(session_id,account_id,id,evidence) VALUES(?,?,?,?) ON CONFLICT(session_id,account_id,id) DO UPDATE SET evidence=excluded.evidence", s.ID, a.AccountID, item.ID, string(raw))
				if err != nil {
					return err
				}
			}
		}
		raw, err := json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO reconciliation_receipts(operation_id,household_id,payload_hash,result) VALUES(?,?,?,?)", operation, s.HouseholdID, hash, string(raw))
		return err
	})
}
func (r *Repository) ReconciliationLinks(ctx context.Context, household, session string) ([]domain.ReconciliationLinkedChange, error) {
	rows, err := r.database.SQL.QueryContext(ctx, "SELECT l.account_id,l.item_id,l.operation_id,l.family,l.evidence FROM reconciliation_links l JOIN reconciliation_sessions s ON s.id=l.session_id WHERE s.household_id=? AND s.id=? ORDER BY l.operation_id,l.account_id", household, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReconciliationLinkedChange{}
	for rows.Next() {
		v := domain.ReconciliationLinkedChange{SessionID: session}
		var raw string
		if err := rows.Scan(&v.AccountID, &v.ItemID, &v.OperationID, &v.Family, &raw); err != nil {
			return nil, err
		}
		v.Evidence = json.RawMessage(raw)
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *Repository) ReconciliationFinancialChanges(ctx context.Context, account string, after int64) ([]domain.ReconciliationFinancialChange, error) {
	rows, err := r.database.SQL.QueryContext(ctx, "SELECT revision,family,fact_id,recorded_at,effective_at,evidence FROM reconciliation_financial_changes WHERE account_id=? AND revision>? ORDER BY revision", account, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReconciliationFinancialChange{}
	for rows.Next() {
		v := domain.ReconciliationFinancialChange{AccountID: account}
		var raw string
		if err := rows.Scan(&v.Revision, &v.Family, &v.FactID, &v.RecordedAt, &v.EffectiveAt, &raw); err != nil {
			return nil, err
		}
		v.Evidence = json.RawMessage(raw)
		result = append(result, v)
	}
	return result, rows.Err()
}
