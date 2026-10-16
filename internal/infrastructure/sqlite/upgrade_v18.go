package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// upgradeV18 is a single additive upgrade, not a migration chain. The verified,
// flushed recovery artifact is published read-only before the immediate transaction.
func upgradeV18(ctx context.Context, database *sql.DB, path string) error {
	generation, err := prepareV18Recovery(ctx, database, path, (*os.File).Sync, syncV18UpgradeDir)
	if err != nil {
		return err
	}
	return commitV18Upgrade(ctx, database, generation)
}

// Recovery copies are inspection artifacts, never writable live databases.
func isV18RecoveryCopy(path string) bool {
	return strings.EqualFold(filepath.Base(path), "original-v18.sqlite") && strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(path))), ".nestworth-v18-recovery-")
}

// Narrow IO seams allow failure tests without changing filesystem behavior globally.
func prepareV18Recovery(ctx context.Context, database *sql.DB, path string, syncFile func(*os.File) error, syncDir func(string) error) (int, error) {
	if err := verifySchemaVersion(ctx, database, 18); err != nil {
		return 0, err
	}
	var generation int
	if err := database.QueryRowContext(ctx, "PRAGMA data_version").Scan(&generation); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), ".nestworth-v18-recovery-")
	if err != nil {
		return 0, err
	}
	pending := filepath.Join(dir, ".snapshot-v18.partial")
	defer os.Remove(pending)
	original := &DB{SQL: database, Path: path}
	if err := original.SnapshotTo(ctx, pending); err != nil {
		return 0, fmt.Errorf("v18 recovery snapshot failed: %w", err)
	}
	verified, err := OpenReadOnlyForVerifyContext(ctx, pending)
	if err != nil {
		return 0, fmt.Errorf("v18 recovery verification failed: %w", err)
	}
	if err := verified.Close(); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(pending, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	// Flush read-only permissions together with the complete, verified file data.
	freezeErr := file.Chmod(0400)
	var flushErr error
	if freezeErr == nil {
		flushErr = syncFile(file)
	}
	closeErr := file.Close()
	if err := errors.Join(freezeErr, flushErr, closeErr); err != nil {
		return 0, err
	}
	recoveryPath := filepath.Join(dir, "original-v18.sqlite")
	if err := os.Rename(pending, recoveryPath); err != nil {
		return 0, err
	}
	for _, directory := range []string{dir, filepath.Dir(path)} {
		if err := syncDir(directory); err != nil {
			return 0, err
		}
	}
	return generation, nil
}

func syncV18UpgradeDir(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	flushErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(flushErr, closeErr)
}

func commitV18Upgrade(ctx context.Context, database *sql.DB, generation int) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRowContext(ctx, "PRAGMA data_version").Scan(&current); err != nil {
		return err
	}
	if current != generation {
		return errors.New("database changed while preparing v18 recovery snapshot; retry upgrade")
	}
	if err := verifySchemaVersion(ctx, tx, 18); err != nil {
		return err
	}
	if err := applyV18Upgrade(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The caller owns the transaction and rolls back failed verification.
func applyV18Upgrade(ctx context.Context, tx *sql.Tx) error {
	schema, err := schemaFS.ReadFile("review_schema_v19.sql")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	if err := captureLegacyReviewState(ctx, tx); err != nil {
		return err
	}
	return verifySchemaVersion(ctx, tx, 19)
}

// The old JSON remains an intact, read-only migration source. A malformed
// review value is isolated in needs_repair instead of making the finance DB
// unusable or silently enabling default rules. No daily alert is generated in
// this structural upgrade transaction.
func captureLegacyReviewState(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) SELECT id FROM households`); err != nil {
		return err
	}
	// app_configuration has unrestricted TEXT keys in v18. LIKE treats '_' as
	// a wildcard, so match the literal prefix before attempting to parse IDs.
	rows, err := tx.QueryContext(ctx, `SELECT key,value FROM app_configuration WHERE substr(key,1,length('investment_review/'))='investment_review/' ORDER BY key`)
	if err != nil {
		return err
	}
	type oldValue struct {
		PriorPolicies []domain.InvestmentReviewPolicy `json:"priorPolicies"`
		SnoozedUntil  map[string]string               `json:"snoozedUntil"`
		Enrolled      map[string]bool                 `json:"enrolled"`
		Revision      int                             `json:"revision"`
		Policy        domain.InvestmentReviewPolicy   `json:"policy"`
		Acknowledged  map[string]bool                 `json:"acknowledged"`
	}
	type entry struct{ key, raw, status, reason string }
	entries := []entry{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return err
		}
		value := oldValue{Policy: domain.DefaultInvestmentReviewPolicy(), SnoozedUntil: map[string]string{}, Enrolled: map[string]bool{}, Acknowledged: map[string]bool{}}
		status, reason := "compatible", ""
		if !strings.HasPrefix(strings.TrimSpace(raw), "{") || json.Unmarshal([]byte(raw), &value) != nil || value.Policy.Validate() != nil || value.Revision < 0 || value.Acknowledged == nil || value.Enrolled == nil {
			status, reason = "needs_repair", "invalid legacy review configuration"
		} else {
			for _, prior := range value.PriorPolicies {
				if prior.Validate() != nil {
					status, reason = "needs_repair", "invalid prior review policy"
				}
			}
			for id, until := range value.SnoozedUntil {
				if _, err := domain.ParseInstrumentID(id); err != nil {
					status, reason = "needs_repair", "invalid legacy snooze instrument"
				}
				if _, err := time.Parse(time.DateOnly, until); err != nil {
					status, reason = "needs_repair", "invalid legacy snooze date"
				}
			}
		}
		entries = append(entries, entry{key: key, raw: raw, status: status, reason: reason})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range entries {
		suffix := strings.TrimPrefix(item.key, "investment_review/")
		householdID, err := domain.ParseHouseholdID(suffix)
		if err == nil && suffix != householdID.String() {
			err = fmt.Errorf("household key is not canonical")
		}
		hash := sha256.Sum256([]byte(item.raw))
		if err == nil {
			var exists int
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM households WHERE id=?`, householdID.String()).Scan(&exists)
			if errors.Is(err, sql.ErrNoRows) {
				err = fmt.Errorf("household does not exist")
			} else if err != nil {
				return err
			}
		}
		if err != nil {
			// Quarantine is keyed by the original TEXT key, without a household
			// foreign key: malformed/orphaned source data must not abort startup.
			if _, insertErr := tx.ExecContext(ctx, `INSERT INTO investment_review_legacy_quarantine(source_key,raw_json,raw_sha256,migration_version,reason,imported_at) VALUES(?,?,?,1,?,?)`, item.key, item.raw, hex.EncodeToString(hash[:]), err.Error(), time.Now().UTC().Format(time.RFC3339Nano)); insertErr != nil {
				return insertErr
			}
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO investment_review_legacy_state(household_id,source_key,raw_json,raw_sha256,migration_version,status,error,imported_at) VALUES(?,?,?,?,1,?,?,?)`, householdID.String(), item.key, item.raw, hex.EncodeToString(hash[:]), item.status, item.reason, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		if item.status == "needs_repair" {
			if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET projection_status='needs_repair' WHERE household_id=?`, householdID.String()); err != nil {
				return err
			}
		}
	}
	return nil
}
