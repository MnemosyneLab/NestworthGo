package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) LookupProductOperation(ctx context.Context, householdID domain.HouseholdID, id domain.ProductOperationID) (*domain.ProductOperation, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT id, household_id, kind, payload_sha256, request_version, request_json, result_json, effective_at, created_at, reverses_operation_id FROM product_operations WHERE household_id = ? AND id = ?`, householdID.String(), id.String())
	operation, err := scanProductOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

func (r *Repository) ProductOperationEvidence(ctx context.Context, householdID domain.HouseholdID, id domain.ProductOperationID) (domain.ProductOperationEvidence, error) {
	operation, err := r.LookupProductOperation(ctx, householdID, id)
	if err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	if operation == nil {
		return domain.ProductOperationEvidence{}, &domain.Error{Code: domain.ErrNotFound, Message: "product operation was not found"}
	}
	evidence := domain.ProductOperationEvidence{Operation: *operation}
	productRows, err := r.database.SQL.QueryContext(ctx, `SELECT operation_id, product_id, role FROM product_operation_products WHERE operation_id = ? ORDER BY product_id, role`, id.String())
	if err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	defer productRows.Close()
	for productRows.Next() {
		var operationID, productID, role string
		if err := productRows.Scan(&operationID, &productID, &role); err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedOperation, err := domain.ParseProductOperationID(operationID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedProduct, err := domain.ParseProductContractID(productID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedRole, err := domain.ParseProductOperationRole(role)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		evidence.Products = append(evidence.Products, domain.ProductOperationProduct{OperationID: parsedOperation, ProductID: parsedProduct, Role: parsedRole})
	}
	if err := productRows.Err(); err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	activityRows, err := r.database.SQL.QueryContext(ctx, `SELECT operation_id, activity_id, sequence, purpose, product_id FROM product_operation_activities WHERE operation_id = ? ORDER BY sequence, activity_id`, id.String())
	if err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	defer activityRows.Close()
	for activityRows.Next() {
		var operationID, activityID, purpose, productID string
		var sequence int
		if err := activityRows.Scan(&operationID, &activityID, &sequence, &purpose, &productID); err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedOperation, err := domain.ParseProductOperationID(operationID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedActivity, err := domain.ParseActivityID(activityID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedPurpose, err := domain.ParseProductActivityPurpose(purpose)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedProduct, err := domain.ParseProductContractID(productID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		evidence.Activities = append(evidence.Activities, domain.ProductOperationActivity{OperationID: parsedOperation, ActivityID: parsedActivity, Sequence: sequence, Purpose: parsedPurpose, ProductID: parsedProduct})
	}
	if err := activityRows.Err(); err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	reservationRows, err := r.database.SQL.QueryContext(ctx, `SELECT operation_id, reservation_id, previous_released_at, resulting_released_at, resulting_revision FROM product_operation_reservations WHERE operation_id = ? ORDER BY reservation_id`, id.String())
	if err != nil {
		return domain.ProductOperationEvidence{}, err
	}
	defer reservationRows.Close()
	for reservationRows.Next() {
		var operationID, reservationID string
		var previousReleased, resultingReleased sql.NullString
		var revision int
		if err := reservationRows.Scan(&operationID, &reservationID, &previousReleased, &resultingReleased, &revision); err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedOperation, err := domain.ParseProductOperationID(operationID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		parsedReservation, err := domain.ParseLiquidityReservationID(reservationID)
		if err != nil {
			return domain.ProductOperationEvidence{}, err
		}
		link := domain.ProductOperationReservation{OperationID: parsedOperation, ReservationID: parsedReservation, ResultingRevision: revision}
		if previousReleased.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, previousReleased.String)
			if err != nil {
				return domain.ProductOperationEvidence{}, err
			}
			link.PreviousReleasedAt = &parsed
		}
		if resultingReleased.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, resultingReleased.String)
			if err != nil {
				return domain.ProductOperationEvidence{}, err
			}
			link.ResultingReleasedAt = &parsed
		}
		evidence.Reservations = append(evidence.Reservations, link)
	}
	return evidence, reservationRows.Err()
}

func (r *Repository) SaveProductContract(ctx context.Context, contract domain.ProductContract) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return upsertProductContractTx(ctx, tx, contract)
	})
}

func (r *Repository) Product(ctx context.Context, householdID domain.HouseholdID, id domain.ProductContractID) (domain.ProductContract, error) {
	row := r.database.SQL.QueryRowContext(ctx, productContractSelect+` WHERE household_id = ? AND id = ?`, householdID.String(), id.String())
	contract, err := scanProductContract(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProductContract{}, &domain.Error{Code: domain.ErrNotFound, Message: "product was not found"}
	}
	return contract, err
}

func (r *Repository) ListProducts(ctx context.Context, householdID domain.HouseholdID, accountID *domain.AccountID, includeClosed bool) ([]domain.ProductContract, error) {
	statement := productContractSelect + ` WHERE household_id = ?`
	args := []any{householdID.String()}
	if accountID != nil {
		statement += ` AND account_id = ?`
		args = append(args, accountID.String())
	}
	if !includeClosed {
		statement += ` AND state = 'open'`
	}
	statement += ` ORDER BY start_on DESC, created_at DESC, id DESC`
	rows, err := r.database.SQL.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ProductContract{}
	for rows.Next() {
		contract, err := scanProductContract(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, contract)
	}
	return result, rows.Err()
}

func (r *Repository) ListProductOperations(ctx context.Context, householdID domain.HouseholdID, productID domain.ProductContractID, limit int, cursor string) ([]domain.ProductOperation, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	query := `SELECT o.id, o.household_id, o.kind, o.payload_sha256, o.request_version, o.request_json, o.result_json, o.effective_at, o.created_at, o.reverses_operation_id FROM product_operations o JOIN product_operation_products p ON p.operation_id = o.id WHERE o.household_id = ? AND p.product_id = ?`
	args := []any{householdID.String(), productID.String()}
	if strings.TrimSpace(cursor) != "" {
		createdAt, id, err := parseProductOperationCursor(cursor)
		if err != nil {
			return nil, err
		}
		query += ` AND (o.created_at < ? OR (o.created_at = ? AND o.id < ?))`
		stamp := formatTimestamp(createdAt)
		args = append(args, stamp, stamp, id.String())
	}
	query += ` ORDER BY o.created_at DESC, o.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.database.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ProductOperation{}
	for rows.Next() {
		operation, err := scanProductOperation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, operation)
	}
	return result, rows.Err()
}

func parseProductOperationCursor(cursor string) (time.Time, domain.ProductOperationID, error) {
	createdAtRaw, idRaw, ok := strings.Cut(strings.TrimSpace(cursor), "|")
	if !ok || createdAtRaw == "" || idRaw == "" {
		return time.Time{}, "", &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "is not a valid operations cursor"}
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return time.Time{}, "", &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "is not a valid operations cursor"}
	}
	id, err := domain.ParseProductOperationID(idRaw)
	if err != nil {
		return time.Time{}, "", &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "is not a valid operations cursor"}
	}
	return createdAt, id, nil
}

func (r *Repository) ProductByHolding(ctx context.Context, holdingID domain.HoldingID) (*domain.ProductContract, error) {
	row := r.database.SQL.QueryRowContext(ctx, productContractSelect+` WHERE holding_id = ?`, holdingID.String())
	contract, err := scanProductContract(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

func (r *Repository) ProductByInstrument(ctx context.Context, instrumentID domain.InstrumentID) (*domain.ProductContract, error) {
	row := r.database.SQL.QueryRowContext(ctx, productContractSelect+` WHERE instrument_id = ?`, instrumentID.String())
	contract, err := scanProductContract(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

func (r *Repository) ProductActivityContext(ctx context.Context, activityID domain.ActivityID) (*domain.ProductActivityContext, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT a.operation_id, a.product_id, a.purpose, c.holding_id, c.instrument_id, c.kind FROM product_operation_activities a JOIN product_contracts c ON c.id = a.product_id WHERE a.activity_id = ?`, activityID.String())
	context, err := scanProductActivityContext(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &context, nil
}

func (r *Repository) ListProductActivityContexts(ctx context.Context, householdID domain.HouseholdID) (map[domain.ActivityID]domain.ProductActivityContext, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `SELECT a.activity_id, a.operation_id, a.product_id, a.purpose, c.holding_id, c.instrument_id, c.kind FROM product_operation_activities a JOIN product_contracts c ON c.id = a.product_id JOIN product_operations o ON o.id = a.operation_id WHERE o.household_id = ?`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[domain.ActivityID]domain.ProductActivityContext{}
	for rows.Next() {
		var activityID string
		var operationID, productID, purpose, holdingID, instrumentID, kind string
		if err := rows.Scan(&activityID, &operationID, &productID, &purpose, &holdingID, &instrumentID, &kind); err != nil {
			return nil, err
		}
		parsedActivity, err := domain.ParseActivityID(activityID)
		if err != nil {
			return nil, err
		}
		context, err := productActivityContextFromParts(operationID, productID, purpose, holdingID, instrumentID, kind)
		if err != nil {
			return nil, err
		}
		result[parsedActivity] = context
	}
	return result, rows.Err()
}

var productCommitFailAfter string

func SetProductCommitFailAfter(stage string) {
	productCommitFailAfter = stage
}

func (r *Repository) CommitProductBundle(ctx context.Context, bundle domain.ProductBundle) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var originTimezone string
		if err := tx.QueryRowContext(ctx, `SELECT timezone FROM history_origins WHERE household_id = ?`, bundle.Operation.HouseholdID.String()).Scan(&originTimezone); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a product operation"}
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_operations(id, household_id, kind, payload_sha256, request_version, request_json, result_json, effective_at, created_at, reverses_operation_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, bundle.Operation.ID.String(), bundle.Operation.HouseholdID.String(), string(bundle.Operation.Kind), bundle.Operation.PayloadSHA256, bundle.Operation.RequestVersion, bundle.Operation.RequestJSON, bundle.Operation.ResultJSON, formatTimestamp(bundle.Operation.EffectiveAt), formatTimestamp(bundle.Operation.CreatedAt), nullableID(bundle.Operation.ReversesOperationID)); err != nil {
			return err
		}
		if err := failProductCommit("operation"); err != nil {
			return err
		}
		for _, instrument := range bundle.Instruments {
			if err := insertInstrumentTx(ctx, tx, instrument); err != nil {
				return err
			}
			if err := syncInstrumentBindingFromInstrumentTx(ctx, tx, instrument); err != nil {
				return err
			}
		}
		for _, observation := range bundle.InstrumentObservations {
			if err := appendInstrumentPreferenceObservationTx(ctx, tx, observation); err != nil {
				return err
			}
		}
		for _, holding := range bundle.Holdings {
			if err := insertHolding(ctx, tx, holding); err != nil {
				return err
			}
		}
		for _, quote := range bundle.Quotes {
			if err := appendInstrumentQuoteTx(ctx, tx, quote); err != nil {
				return err
			}
			if err := markQuoteHistoryDirtyTx(ctx, tx, quote.InstrumentID, quote.QuotedAt, quote.CreatedAt); err != nil {
				return err
			}
		}
		for index, commit := range bundle.Activities {
			if err := commitActivityTx(ctx, tx, commit, bundle.AsOf); err != nil {
				return err
			}
			if err := markHistoryDirtyTx(ctx, tx, commit.Activity.HouseholdID, commit.Activity.EffectiveLocalDate, originTimezone, bundle.AsOf); err != nil {
				return err
			}
			if err := failProductCommit("activity:" + strconv.Itoa(index)); err != nil {
				return err
			}
		}
		for _, contract := range bundle.Contracts {
			if err := upsertProductContractTx(ctx, tx, contract); err != nil {
				return err
			}
		}
		for _, policy := range bundle.Policies {
			if err := upsertLiquidityPolicyTx(ctx, tx, policy); err != nil {
				return err
			}
		}
		for _, link := range bundle.ProductLinks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_operation_products(operation_id, product_id, role) VALUES(?, ?, ?)`, link.OperationID.String(), link.ProductID.String(), string(link.Role)); err != nil {
				return err
			}
		}
		for _, link := range bundle.ActivityLinks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_operation_activities(operation_id, activity_id, sequence, purpose, product_id) VALUES(?, ?, ?, ?, ?)`, link.OperationID.String(), link.ActivityID.String(), link.Sequence, string(link.Purpose), link.ProductID.String()); err != nil {
				return err
			}
		}
		for _, link := range bundle.ReservationLinks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_operation_reservations(operation_id, reservation_id, previous_released_at, resulting_released_at, resulting_revision) VALUES(?, ?, ?, ?, ?)`, link.OperationID.String(), link.ReservationID.String(), nullableTime(link.PreviousReleasedAt), nullableTime(link.ResultingReleasedAt), link.ResultingRevision); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE liquidity_reservations SET released_at = ?, revision = ?, updated_at = ? WHERE id = ?`, nullableTime(link.ResultingReleasedAt), link.ResultingRevision, formatTimestamp(bundle.AsOf), link.ReservationID.String()); err != nil {
				return err
			}
		}
		return failProductCommit("complete")
	})
}

func failProductCommit(stage string) error {
	if productCommitFailAfter != "" && productCommitFailAfter == stage {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "injected product commit failure"}
	}
	return nil
}

const productContractSelect = `SELECT id, household_id, account_id, holding_id, instrument_id, kind, name, note, currency, principal, start_on, maturity_on, interest_mode, annual_rate, maturity_interest, interest_paid_through_on, renewed_from_id, state, opened_operation_id, closed_operation_id, revision, created_at, updated_at FROM product_contracts`

type scanner interface {
	Scan(dest ...any) error
}

func scanProductOperation(row scanner) (domain.ProductOperation, error) {
	var id, householdID, kind, hash, requestJSON, resultJSON, effectiveAt, createdAt string
	var version int
	var reverses sql.NullString
	if err := row.Scan(&id, &householdID, &kind, &hash, &version, &requestJSON, &resultJSON, &effectiveAt, &createdAt, &reverses); err != nil {
		return domain.ProductOperation{}, err
	}
	parsedID, err := domain.ParseProductOperationID(id)
	if err != nil {
		return domain.ProductOperation{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.ProductOperation{}, err
	}
	parsedKind, err := domain.ParseProductOperationKind(kind)
	if err != nil {
		return domain.ProductOperation{}, err
	}
	effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
	if err != nil {
		return domain.ProductOperation{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.ProductOperation{}, err
	}
	operation := domain.ProductOperation{ID: parsedID, HouseholdID: parsedHousehold, Kind: parsedKind, PayloadSHA256: hash, RequestVersion: version, RequestJSON: requestJSON, ResultJSON: resultJSON, EffectiveAt: effective, CreatedAt: created}
	if reverses.Valid {
		parsed, err := domain.ParseProductOperationID(reverses.String)
		if err != nil {
			return domain.ProductOperation{}, err
		}
		operation.ReversesOperationID = &parsed
	}
	return operation, nil
}

func scanProductContract(row scanner) (domain.ProductContract, error) {
	var id, householdID, accountID, holdingID, instrumentID, kind, name, currency, principal, startOn, interestMode, state, openedID, createdAt, updatedAt string
	var note, maturityOn, annualRate, maturityInterest, paidThrough, renewedFrom, closedID sql.NullString
	var revision int
	if err := row.Scan(&id, &householdID, &accountID, &holdingID, &instrumentID, &kind, &name, &note, &currency, &principal, &startOn, &maturityOn, &interestMode, &annualRate, &maturityInterest, &paidThrough, &renewedFrom, &state, &openedID, &closedID, &revision, &createdAt, &updatedAt); err != nil {
		return domain.ProductContract{}, err
	}
	parsedID, err := domain.ParseProductContractID(id)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedAccount, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedHolding, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedKind, err := domain.ParseProductKind(kind)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedPrincipal, err := domain.ParseMoney(principal, parsedCurrency)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedMode, err := domain.ParseInterestMode(interestMode)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedState, err := domain.ParseProductContractState(state)
	if err != nil {
		return domain.ProductContract{}, err
	}
	parsedOpened, err := domain.ParseProductOperationID(openedID)
	if err != nil {
		return domain.ProductContract{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.ProductContract{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.ProductContract{}, err
	}
	contract := domain.ProductContract{
		ID: parsedID, HouseholdID: parsedHousehold, AccountID: parsedAccount, HoldingID: parsedHolding, InstrumentID: parsedInstrument,
		Kind: parsedKind, Name: name, Currency: parsedCurrency, Principal: parsedPrincipal, StartOn: startOn, InterestMode: parsedMode,
		State: parsedState, OpenedOperationID: parsedOpened, Revision: revision, CreatedAt: created, UpdatedAt: updated,
	}
	contract.Note = nullableStringValue(note)
	contract.MaturityOn = nullableStringValue(maturityOn)
	contract.InterestPaidThroughOn = nullableStringValue(paidThrough)
	if annualRate.Valid {
		rate, err := domain.ParseAnnualRateRatio(annualRate.String)
		if err != nil {
			return domain.ProductContract{}, err
		}
		contract.AnnualRate = &rate
	}
	if maturityInterest.Valid {
		money, err := domain.ParseMoney(maturityInterest.String, parsedCurrency)
		if err != nil {
			return domain.ProductContract{}, err
		}
		contract.MaturityInterest = &money
	}
	if renewedFrom.Valid {
		parsed, err := domain.ParseProductContractID(renewedFrom.String)
		if err != nil {
			return domain.ProductContract{}, err
		}
		contract.RenewedFromID = &parsed
	}
	if closedID.Valid {
		parsed, err := domain.ParseProductOperationID(closedID.String)
		if err != nil {
			return domain.ProductContract{}, err
		}
		contract.ClosedOperationID = &parsed
	}
	return contract, nil
}

func scanProductActivityContext(row scanner) (domain.ProductActivityContext, error) {
	var operationID, productID, purpose, holdingID, instrumentID, kind string
	if err := row.Scan(&operationID, &productID, &purpose, &holdingID, &instrumentID, &kind); err != nil {
		return domain.ProductActivityContext{}, err
	}
	return productActivityContextFromParts(operationID, productID, purpose, holdingID, instrumentID, kind)
}

func productActivityContextFromParts(operationID, productID, purpose, holdingID, instrumentID, kind string) (domain.ProductActivityContext, error) {
	parsedOperation, err := domain.ParseProductOperationID(operationID)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	parsedProduct, err := domain.ParseProductContractID(productID)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	parsedPurpose, err := domain.ParseProductActivityPurpose(purpose)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	parsedHolding, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	parsedKind, err := domain.ParseProductKind(kind)
	if err != nil {
		return domain.ProductActivityContext{}, err
	}
	return domain.ProductActivityContext{OperationID: parsedOperation, ProductID: parsedProduct, Purpose: parsedPurpose, HoldingID: parsedHolding, InstrumentID: parsedInstrument, ProductKind: parsedKind}, nil
}

func upsertProductContractTx(ctx context.Context, tx *sql.Tx, contract domain.ProductContract) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO product_contracts(id, household_id, account_id, holding_id, instrument_id, kind, name, note, currency, principal, start_on, maturity_on, interest_mode, annual_rate, maturity_interest, interest_paid_through_on, renewed_from_id, state, opened_operation_id, closed_operation_id, revision, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name = excluded.name, note = excluded.note, maturity_on = excluded.maturity_on, interest_mode = excluded.interest_mode, annual_rate = excluded.annual_rate, maturity_interest = excluded.maturity_interest, interest_paid_through_on = excluded.interest_paid_through_on, state = excluded.state, closed_operation_id = excluded.closed_operation_id, revision = excluded.revision, updated_at = excluded.updated_at`,
		contract.ID.String(), contract.HouseholdID.String(), contract.AccountID.String(), contract.HoldingID.String(), contract.InstrumentID.String(), string(contract.Kind), contract.Name, nullableString(contract.Note), contract.Currency.String(), contract.Principal.CanonicalAmount(), contract.StartOn, nullableString(contract.MaturityOn), string(contract.InterestMode), nullableAnnualRate(contract.AnnualRate), nullableMoneyAmount(contract.MaturityInterest), nullableString(contract.InterestPaidThroughOn), nullableID(contract.RenewedFromID), string(contract.State), contract.OpenedOperationID.String(), nullableID(contract.ClosedOperationID), contract.Revision, formatTimestamp(contract.CreatedAt), formatTimestamp(contract.UpdatedAt))
	return err
}

func nullableAnnualRate(value *domain.AnnualRate) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}
