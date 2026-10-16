package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func desktopPlanKey(household, family, id string) string {
	return "reconciliation.desktop." + household + "." + family + "." + id
}
func (r *Repository) SaveReconciliationPlan(ctx context.Context, household, family, id string, raw []byte) error {
	// Immutable registration: a reused identity must never replace its reviewed body.
	_, err := r.database.SQL.ExecContext(ctx, "INSERT INTO app_configuration(key,value) VALUES(?,?)", desktopPlanKey(household, family, id), string(raw))
	return err
}
func (r *Repository) LoadReconciliationPlan(ctx context.Context, household, family, id string) ([]byte, error) {
	var raw []byte
	err := r.database.SQL.QueryRowContext(ctx, "SELECT value FROM app_configuration WHERE key=?", desktopPlanKey(household, family, id)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "planId", Message: "reviewed plan unavailable; outcome remains unknown"}
	}
	return raw, err
}
func (r *Repository) ReconciliationReceiptByID(ctx context.Context, household, id string) (*domain.ReconciliationSession, error) {
	var raw string
	err := r.database.SQL.QueryRowContext(ctx, "SELECT result FROM reconciliation_receipts WHERE household_id=? AND operation_id=?", household, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v domain.ReconciliationSession
	err = json.Unmarshal([]byte(raw), &v)
	return &v, err
}
