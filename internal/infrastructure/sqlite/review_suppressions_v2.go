package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const reviewSuppressionSelect = `SELECT id,household_id,instrument_id,rule_id,mode,until_on,timezone,revision,created_at,updated_at FROM investment_review_suppressions`

func scanReviewSuppression(row rowScanner) (domain.ReviewSuppression, error) {
	var item domain.ReviewSuppression
	var household, instrument, rule, until sql.NullString
	var created, updated string
	if err := row.Scan(&item.ID, &household, &instrument, &rule, &item.Mode, &until, &item.Timezone, &item.Revision, &created, &updated); err != nil {
		return item, err
	}
	var err error
	item.HouseholdID, err = domain.ParseHouseholdID(household.String)
	if err != nil {
		return item, err
	}
	item.InstrumentID, err = domain.ParseInstrumentID(instrument.String)
	if err != nil {
		return item, err
	}
	item.RuleID = rule.String
	item.UntilOn = until.String
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return item, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return item, err
}

func (r *Repository) ListReviewSuppressionsV2(ctx context.Context, householdID domain.HouseholdID, after string, limit int) ([]domain.ReviewSuppression, error) {
	if limit < 1 || limit > 100 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 suppressions per page"}
	}
	if after != "" {
		if _, err := domain.ParseReviewSuppressionID(after); err != nil {
			return nil, err
		}
	}
	rows, err := r.database.SQL.QueryContext(ctx, reviewSuppressionSelect+` WHERE household_id=? AND id>? ORDER BY id LIMIT ?`, householdID.String(), after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ReviewSuppression, 0, limit)
	for rows.Next() {
		item, err := scanReviewSuppression(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) MutateReviewSuppressionV2(ctx context.Context, m domain.ReviewSuppressionMutation) (*domain.ReviewSuppression, error) {
	if m.Operation != "set" && m.Operation != "clear" {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "operation", Message: "unsupported suppression operation"}
	}
	if _, err := domain.ParseHouseholdID(m.HouseholdID.String()); err != nil {
		return nil, err
	}
	if _, err := domain.ParseReviewSuppressionID(string(m.SuppressionID)); err != nil {
		return nil, err
	}
	if _, err := domain.ParseMutationID(string(m.MutationID)); err != nil {
		return nil, err
	}
	request, err := json.Marshal(struct {
		Operation        string
		HouseholdID      domain.HouseholdID
		SuppressionID    domain.ReviewSuppressionID
		MutationID       domain.MutationID
		InstrumentID     domain.InstrumentID
		RuleID           domain.ReviewRuleID
		Mode             string
		RelativeDays     int
		CustomUntil      string
		ExpectedRevision int
	}{m.Operation, m.HouseholdID, m.SuppressionID, m.MutationID, m.InstrumentID, m.RuleID, m.Mode, m.RelativeDays, m.CustomUntil, m.ExpectedRevision})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(request)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var previousHash, previousJSON string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,result_json FROM investment_review_mutation_receipts WHERE household_id=? AND mutation_id=?`, m.HouseholdID.String(), string(m.MutationID)).Scan(&previousHash, &previousJSON)
	if err == nil {
		if previousHash != requestHash {
			return nil, reviewRuleConflict("mutation ID was already used for a different request")
		}
		var previous *domain.ReviewSuppression
		if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) VALUES(?) ON CONFLICT(household_id) DO NOTHING`, m.HouseholdID.String()); err != nil {
		return nil, err
	}
	var result *domain.ReviewSuppression
	changed := false
	if m.Operation == "clear" {
		if m.ExpectedRevision < 1 {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid suppression revision"}
		}
		current, err := scanReviewSuppression(tx.QueryRowContext(ctx, reviewSuppressionSelect+` WHERE household_id=? AND id=?`, m.HouseholdID.String(), string(m.SuppressionID)))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, reviewRuleConflict("suppression no longer exists")
		}
		if err != nil {
			return nil, err
		}
		if current.Revision != m.ExpectedRevision {
			return nil, reviewRuleConflict("suppression changed; reload before clearing")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM investment_review_suppressions WHERE household_id=? AND id=? AND revision=?`, m.HouseholdID.String(), string(m.SuppressionID), m.ExpectedRevision); err != nil {
			return nil, err
		}
		changed = true
	} else {
		if _, err := domain.ParseInstrumentID(m.InstrumentID.String()); err != nil {
			return nil, err
		}
		if m.RuleID != "" {
			if _, err := domain.ParseReviewRuleID(string(m.RuleID)); err != nil {
				return nil, err
			}
		}
		if m.Mode != "permanent" && m.Mode != "until_date" {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "mode", Message: "invalid ignore mode"}
		}
		var member int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM instruments WHERE household_id=? AND id=?`, m.HouseholdID.String(), m.InstrumentID.String()).Scan(&member); err != nil {
			return nil, err
		}
		if member != 1 {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument does not belong to this household"}
		}
		if m.RuleID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM investment_review_rules WHERE household_id=? AND id=?`, m.HouseholdID.String(), string(m.RuleID)).Scan(&member); err != nil {
				return nil, err
			}
			if member != 1 {
				return nil, &domain.Error{Code: domain.ErrValidation, Field: "ruleId", Message: "rule does not belong to this household"}
			}
		}
		until := ""
		if m.Mode == "until_date" {
			until, err = domain.ReviewSuppressionUntil(m.At, m.Timezone, m.RelativeDays, m.CustomUntil)
			if err != nil {
				return nil, err
			}
		} else if m.RelativeDays != 0 || m.CustomUntil != "" {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "untilOn", Message: "permanent ignore cannot have a resume date"}
		}
		var rule any
		if m.RuleID != "" {
			rule = string(m.RuleID)
		}
		var resume any
		if until != "" {
			resume = until
		}
		at := m.At.UTC().Format(time.RFC3339Nano)
		if m.ExpectedRevision == 0 {
			current, lookupErr := scanReviewSuppression(tx.QueryRowContext(ctx, reviewSuppressionSelect+` WHERE household_id=? AND instrument_id=? AND COALESCE(rule_id,'')=?`, m.HouseholdID.String(), m.InstrumentID.String(), string(m.RuleID)))
			if errors.Is(lookupErr, sql.ErrNoRows) {
				_, err = tx.ExecContext(ctx, `INSERT INTO investment_review_suppressions(id,household_id,instrument_id,rule_id,mode,until_on,timezone,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,1,?,?)`, string(m.SuppressionID), m.HouseholdID.String(), m.InstrumentID.String(), rule, m.Mode, resume, m.Timezone, at, at)
				if err != nil {
					return nil, mapSQLiteError(err)
				}
				changed = true
			} else if lookupErr != nil {
				return nil, lookupErr
			} else {
				// A scope owns one durable suppression row. Reuse it when a
				// temporary ignore has expired so a fresh ignore can be set
				// without conflicting with the unique scope index. The write
				// transaction is serialized by the household control row, and
				// unchanged concurrent requests therefore converge on one result.
				if current.Mode != m.Mode || current.UntilOn != until || current.Timezone != m.Timezone {
					active, activeErr := domain.ActiveReviewSuppression([]domain.ReviewSuppression{current}, m.InstrumentID, string(m.RuleID), m.At)
					if activeErr != nil {
						return nil, activeErr
					}
					if active != nil {
						return nil, reviewRuleConflict("active suppression changed; reload before setting ignore")
					}
					update, updateErr := tx.ExecContext(ctx, `UPDATE investment_review_suppressions SET mode=?,until_on=?,timezone=?,revision=revision+1,updated_at=? WHERE household_id=? AND id=? AND revision=?`, m.Mode, resume, m.Timezone, at, m.HouseholdID.String(), current.ID, current.Revision)
					if updateErr != nil {
						return nil, updateErr
					}
					rows, rowsErr := update.RowsAffected()
					if rowsErr != nil {
						return nil, rowsErr
					}
					if rows != 1 {
						return nil, reviewRuleConflict("suppression changed; reload before setting ignore")
					}
					current.Mode = m.Mode
					current.UntilOn = until
					current.Timezone = m.Timezone
					current.Revision++
					current.UpdatedAt = m.At.UTC()
					changed = true
				}
				result = &current
			}
		} else {
			current, err := scanReviewSuppression(tx.QueryRowContext(ctx, reviewSuppressionSelect+` WHERE household_id=? AND id=?`, m.HouseholdID.String(), string(m.SuppressionID)))
			if errors.Is(err, sql.ErrNoRows) {
				return nil, reviewRuleConflict("suppression no longer exists")
			}
			if err != nil {
				return nil, err
			}
			if current.Revision != m.ExpectedRevision || current.InstrumentID != m.InstrumentID || current.RuleID != string(m.RuleID) {
				return nil, reviewRuleConflict("suppression scope or revision changed")
			}
			if current.Mode != m.Mode || current.UntilOn != until || current.Timezone != m.Timezone {
				if _, err := tx.ExecContext(ctx, `UPDATE investment_review_suppressions SET mode=?,until_on=?,timezone=?,revision=revision+1,updated_at=? WHERE household_id=? AND id=? AND revision=?`, m.Mode, resume, m.Timezone, at, m.HouseholdID.String(), string(m.SuppressionID), m.ExpectedRevision); err != nil {
					return nil, err
				}
				changed = true
			}
		}
		if result == nil {
			item, err := scanReviewSuppression(tx.QueryRowContext(ctx, reviewSuppressionSelect+` WHERE household_id=? AND id=?`, m.HouseholdID.String(), string(m.SuppressionID)))
			if err != nil {
				return nil, err
			}
			result = &item
		}
	}
	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET review_state_revision=review_state_revision+1 WHERE household_id=?`, m.HouseholdID.String()); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_mutation_receipts(household_id,mutation_id,request_hash,result_json,created_at) VALUES(?,?,?,?,?)`, m.HouseholdID.String(), string(m.MutationID), requestHash, string(encoded), m.At.UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
