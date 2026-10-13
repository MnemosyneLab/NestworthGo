package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const productOperationColumns = `o.id, o.household_id, o.kind, o.payload_sha256, o.request_version, o.request_json, o.result_json, o.effective_at, o.created_at, o.reverses_operation_id`

func lookupProductOperationQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, id domain.ProductOperationID) (*domain.ProductOperation, error) {
	operation, err := scanProductOperation(query.QueryRowContext(ctx, `SELECT `+productOperationColumns+` FROM product_operations o WHERE o.household_id = ? AND o.id = ?`, householdID.String(), id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

func (r *Repository) ReadProductOperationHistorySnapshot(ctx context.Context, productID domain.ProductContractID, limit int, cursor string) (domain.ProductOperationHistorySnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readProductOperationHistoryQuery(ctx, tx, productID, limit, cursor)
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	return snapshot, nil
}

// Every fact used by availability is read through this queryer, in one SQLite snapshot.
func readProductOperationHistoryQuery(ctx context.Context, query queryer, productID domain.ProductContractID, limit int, cursor string) (domain.ProductOperationHistorySnapshot, error) {
	household, err := scanHousehold(query.QueryRowContext(ctx, `SELECT id, name, base_currency, created_at, updated_at FROM households WHERE singleton_key = 1`))
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if household == nil {
		return domain.ProductOperationHistorySnapshot{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	_, err = scanProductContract(query.QueryRowContext(ctx, productContractSelect+` WHERE household_id = ? AND id = ?`, household.ID.String(), productID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProductOperationHistorySnapshot{}, &domain.Error{Code: domain.ErrNotFound, Message: "product was not found"}
	}
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if limit <= 0 {
		limit = 25
	}
	if limit > 101 {
		limit = 101
	}
	statement := `SELECT ` + productOperationColumns + `, reverse.id FROM product_operations o JOIN product_operation_products p ON p.operation_id = o.id LEFT JOIN product_operations reverse ON reverse.reverses_operation_id = o.id AND reverse.household_id = o.household_id WHERE o.household_id = ? AND p.product_id = ?`
	args := []any{household.ID.String(), productID.String()}
	if strings.TrimSpace(cursor) != "" {
		created, id, err := parseProductOperationCursor(cursor)
		if err != nil {
			return domain.ProductOperationHistorySnapshot{}, err
		}
		statement += ` AND (o.created_at < ? OR (o.created_at = ? AND o.id < ?))`
		args = append(args, formatTimestamp(created), formatTimestamp(created), id.String())
	}
	statement += ` ORDER BY o.created_at DESC, o.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	result := domain.ProductOperationHistorySnapshot{Rows: []domain.ProductOperationHistoryRow{}}
	for rows.Next() {
		var reverse sql.NullString
		operation, err := scanProductOperation(operationReverseScanner{rows, &reverse})
		if err != nil {
			rows.Close()
			return domain.ProductOperationHistorySnapshot{}, err
		}
		row := domain.ProductOperationHistoryRow{ProductOperation: operation}
		if reverse.Valid {
			id, err := domain.ParseProductOperationID(reverse.String)
			if err != nil {
				rows.Close()
				return domain.ProductOperationHistorySnapshot{}, err
			}
			row.ReversedByOperationID = &id
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	base := `SELECT ` + productOperationColumns + ` FROM product_operations o JOIN product_operation_products p ON p.operation_id = o.id WHERE o.household_id = ? AND p.product_id = ?`
	head, err := scanProductOperation(query.QueryRowContext(ctx, base+` ORDER BY o.created_at DESC, o.id DESC LIMIT 1`, household.ID.String(), productID.String()))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if err == nil {
		result.LatestRecordedOperationID = &head.ID
	}
	candidate, err := scanProductOperation(query.QueryRowContext(ctx, base+` AND o.reverses_operation_id IS NULL AND NOT EXISTS (SELECT 1 FROM product_operations reverse WHERE reverse.reverses_operation_id = o.id AND reverse.household_id = o.household_id) ORDER BY o.created_at DESC, o.id DESC LIMIT 1`, household.ID.String(), productID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	result.Candidate = &candidate
	if candidate.Kind == domain.ProductOpValueObservation {
		return result, nil
	}
	evidence, err := readProductUndoEvidenceQuery(ctx, query, household.ID, candidate.ID)
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	result.UndoEvidence = &evidence
	result.Portfolio, err = readPortfolioSnapshotQuery(ctx, query, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.ProductOperationHistorySnapshot{}, err
	}
	if result.Portfolio.Household == nil || result.Portfolio.Household.ID != household.ID {
		return domain.ProductOperationHistorySnapshot{}, &domain.Error{Code: domain.ErrIntegrity, Message: "history household changed"}
	}
	return result, nil
}

type operationReverseScanner struct {
	scanner
	reverse *sql.NullString
}

func (s operationReverseScanner) Scan(dest ...any) error {
	return s.scanner.Scan(append(dest, s.reverse)...)
}

func (r *Repository) ReadProductUndoEvidence(ctx context.Context, householdID domain.HouseholdID, operationID domain.ProductOperationID) (domain.ProductUndoEvidence, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ProductUndoEvidence{}, err
	}
	defer tx.Rollback()
	evidence, err := readProductUndoEvidenceQuery(ctx, tx, householdID, operationID)
	if err != nil {
		return domain.ProductUndoEvidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ProductUndoEvidence{}, err
	}
	return evidence, nil
}

func readProductUndoEvidenceQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, operationID domain.ProductOperationID) (domain.ProductUndoEvidence, error) {
	operation, err := productOperationEvidenceQuery(ctx, query, householdID, operationID)
	if err != nil {
		return domain.ProductUndoEvidence{}, err
	}
	result := domain.ProductUndoEvidence{Operation: operation, Contracts: map[domain.ProductContractID]domain.ProductContract{}, Histories: map[domain.ProductContractID][]domain.ProductOperation{}, Activities: map[domain.ActivityID]domain.Activity{}}
	// A single candidate can link both sides of renewal. Load only those contracts.
	rows, err := query.QueryContext(ctx, productContractSelect+` WHERE household_id = ? AND id IN (SELECT product_id FROM product_operation_products WHERE operation_id = ?)`, householdID.String(), operationID.String())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		contract, err := scanProductContract(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Contracts[contract.ID] = contract
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for _, link := range operation.Products {
		if _, ok := result.Contracts[link.ProductID]; !ok {
			return result, &domain.Error{Code: domain.ErrIntegrity, Message: "operation product is missing from household"}
		}
	}
	rows, err = query.QueryContext(ctx, `SELECT p.product_id, `+productOperationColumns+` FROM product_operations o JOIN product_operation_products p ON p.operation_id = o.id JOIN product_contracts c ON c.id = p.product_id AND c.household_id = o.household_id WHERE o.household_id = ? AND p.product_id IN (SELECT product_id FROM product_operation_products WHERE operation_id = ?) ORDER BY o.created_at DESC, o.id DESC`, householdID.String(), operationID.String())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var product string
		op, err := scanProductOperation(operationProductScanner{rows, &product})
		if err != nil {
			rows.Close()
			return result, err
		}
		id, err := domain.ParseProductContractID(product)
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Histories[id] = append(result.Histories[id], op)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	rows, err = query.QueryContext(ctx, `SELECT a.id, a.household_id, a.kind, a.reason, a.effective_at, a.effective_local_date, a.created_at, a.note, a.reverses_activity_id, a.correction_group_id, a.transaction_fx_rate FROM activities a JOIN product_operation_activities link ON link.activity_id = a.id WHERE a.household_id = ? AND link.operation_id = ? ORDER BY link.sequence, a.id`, householdID.String(), operationID.String())
	if err != nil {
		return result, err
	}
	activities := []domain.Activity{}
	for rows.Next() {
		activity, err := activityFromScanner(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	if err := hydrateActivities(ctx, query, activities); err != nil {
		return result, err
	}
	for _, activity := range activities {
		result.Activities[activity.ID] = activity
	}
	for _, link := range operation.Activities {
		if _, ok := result.Activities[link.ActivityID]; !ok {
			return result, &domain.Error{Code: domain.ErrIntegrity, Message: "operation activity is missing from household"}
		}
	}
	result.Reservations, err = listLiquidityReservationsQuery(ctx, query, householdID, true)
	return result, err
}

type operationProductScanner struct {
	scanner
	product *string
}

func (s operationProductScanner) Scan(dest ...any) error {
	return s.scanner.Scan(append([]any{s.product}, dest...)...)
}
