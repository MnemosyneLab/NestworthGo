package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReplaceDerivedData publishes one fully staged rebuild in a single database
// transaction. Source facts and baseline rows are never deleted.
func (r *Repository) ReplaceDerivedData(ctx context.Context, rebuild domain.DerivedDataRebuild) error {
	if rebuild.HouseholdID == "" || rebuild.AsOf.IsZero() {
		return &domain.Error{Code: domain.ErrValidation, Field: "rebuild", Message: "rebuild household and time are required"}
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var generation int
		if err := tx.QueryRowContext(ctx, `SELECT input_generation FROM history_snapshot_state WHERE household_id = ?`, rebuild.HouseholdID.String()).Scan(&generation); err != nil {
			return err
		}
		if generation != rebuild.InputGeneration {
			return snapshotGenerationChanged()
		}
		if err := validateRebuildDatesTx(ctx, tx, rebuild); err != nil {
			return err
		}
		for _, projection := range rebuild.Projections {
			if projection.Activity.HouseholdID != rebuild.HouseholdID {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "rebuild activity belongs to another household"}
			}
			if err := replaceEventProjectionTx(ctx, tx, projection); err != nil {
				return err
			}
		}
		if err := replaceCurrentProjectionsTx(ctx, tx, rebuild); err != nil {
			return err
		}
		if err := replaceDailySnapshotsTx(ctx, tx, rebuild); err != nil {
			return err
		}
		var closedThrough any
		if rebuild.ClosedThrough != "" {
			closedThrough = rebuild.ClosedThrough
		}
		result, err := tx.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from = NULL, dirty_to = NULL, last_completed_closed_on = ?, resolver_policy_version = ?, updated_at = ? WHERE household_id = ? AND input_generation = ?`, closedThrough, domain.MarketDataResolverPolicy, formatTimestamp(rebuild.AsOf), rebuild.HouseholdID.String(), rebuild.InputGeneration)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			if err != nil {
				return err
			}
			return snapshotGenerationChanged()
		}
		return nil
	})
}

func validateRebuildDatesTx(ctx context.Context, tx *sql.Tx, rebuild domain.DerivedDataRebuild) error {
	var startedAt, timezone string
	if err := tx.QueryRowContext(ctx, `SELECT started_at, timezone FROM history_origins WHERE household_id = ?`, rebuild.HouseholdID.String()).Scan(&startedAt, &timezone); err != nil {
		return err
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return &domain.Error{Code: domain.ErrIntegrity, Message: "stored history Starting point is invalid"}
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	startDate := started.In(location).Format("2006-01-02")
	today := rebuild.AsOf.In(location).Format("2006-01-02")
	if rebuild.ClosedThrough == "" {
		if startDate < today || len(rebuild.Snapshots) != 0 {
			return &domain.Error{Code: domain.ErrValidation, Field: "closedThrough", Message: "closed snapshot range is incomplete"}
		}
		return nil
	}
	if rebuild.ClosedThrough >= today || rebuild.ClosedThrough < startDate {
		return &domain.Error{Code: domain.ErrValidation, Field: "closedThrough", Message: "closed snapshot range is invalid"}
	}
	date, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return err
	}
	for index, snapshot := range rebuild.Snapshots {
		if snapshot.HouseholdID != rebuild.HouseholdID || snapshot.LocalDate != date.Format("2006-01-02") || snapshot.InputGeneration != 0 && snapshot.InputGeneration != rebuild.InputGeneration {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged snapshots do not cover every closed day in order"}
		}
		if snapshot.LocalDate > rebuild.ClosedThrough || index > 0 && snapshot.LocalDate <= rebuild.Snapshots[index-1].LocalDate {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged snapshot range is invalid"}
		}
		date = date.AddDate(0, 0, 1)
	}
	if date.Format("2006-01-02") != nextDateUnchecked(rebuild.ClosedThrough) {
		return &domain.Error{Code: domain.ErrIntegrity, Message: "staged snapshots do not reach the last closed day"}
	}
	return nil
}

func nextDateUnchecked(localDate string) string {
	date, err := time.Parse("2006-01-02", localDate)
	if err != nil || date.Format("2006-01-02") != localDate {
		return ""
	}
	return date.AddDate(0, 0, 1).Format("2006-01-02")
}

func replaceEventProjectionTx(ctx context.Context, tx *sql.Tx, projection domain.ActivityProjection) error {
	effects := projection.Activity.Effects
	if projection.Activity.Kind == domain.ActivityValueUpdate && projection.Activity.RecordedEffects != nil {
		// Replay may drop a now-zero delta, but the event projection still
		// contains the user's absolute target and must be present.
		effects = projection.Activity.RecordedEffects
	}
	for _, effect := range effects {
		if effect.Target == domain.EffectTargetHoldingCost {
			continue
		}
		var sourceCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_effects e JOIN activities a ON a.id = e.activity_id WHERE e.id = ? AND a.id = ? AND a.household_id = ?`, effect.ID.String(), projection.Activity.ID.String(), projection.Activity.HouseholdID.String()).Scan(&sourceCount); err != nil {
			return err
		}
		if sourceCount != 1 {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged event projection has no matching source effect"}
		}
		view, err := endpointForEffect(effect, projection.Resulting)
		if err != nil {
			return err
		}
		var table, field, value string
		switch effect.Target {
		case domain.EffectTargetAccountValue:
			table, field, value = "account_values", "amount", view.Amount
		case domain.EffectTargetAccountCash:
			table, field, value = "account_cash_values", "amount", view.Amount
		case domain.EffectTargetHoldingQuantity:
			table, field, value = "holding_quantity_values", "quantity", view.Quantity
		default:
			return &domain.Error{Code: domain.ErrIntegrity, Message: "rebuild effect target is unsupported"}
		}
		var rowID sql.NullString
		var rowCount int
		err = tx.QueryRowContext(ctx, `SELECT MIN(id), COUNT(*) FROM `+table+` WHERE activity_effect_id = ? AND projection_kind = 'event'`, effect.ID.String()).Scan(&rowID, &rowCount)
		if err != nil {
			return err
		}
		if rowCount > 1 {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "activity effect has duplicate event projections"}
		}
		if rowCount == 0 {
			// A value update's event row is its only saved absolute target.
			if projection.Activity.Kind == domain.ActivityValueUpdate {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "value update absolute target is missing"}
			}
			if err := insertEventProjectionTx(ctx, tx, projection.Activity, effect, view); err != nil {
				return err
			}
			continue
		}
		// Preserve the absolute target even when a correction changes the
		// preceding balance. The application replay reads this saved target.
		if projection.Activity.Kind == domain.ActivityValueUpdate {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET `+field+` = ? WHERE id = ?`, value, rowID.String); err != nil {
			return err
		}
	}
	return nil
}

func insertEventProjectionTx(ctx context.Context, tx *sql.Tx, activity domain.Activity, effect domain.ActivityEffect, view domain.EndpointView) error {
	switch effect.Target {
	case domain.EffectTargetAccountValue:
		_, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) SELECT ?, a.id, a.tracking_mode, ?, ?, ?, ?, ?, 'event' FROM accounts a WHERE a.id = ? AND a.household_id = ?`, domain.NewAccountValueID().String(), view.Amount, view.Currency.String(), formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String(), effect.AccountID.String(), activity.HouseholdID.String())
		return err
	case domain.EffectTargetAccountCash:
		_, err := tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, 'event')`, domain.NewAccountCashValueID().String(), effect.AccountID.String(), view.Amount, view.Currency.String(), formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String())
		return err
	case domain.EffectTargetHoldingQuantity:
		_, err := tx.ExecContext(ctx, `INSERT INTO holding_quantity_values(id, holding_id, quantity, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, 'event')`, domain.NewHoldingQuantityValueID().String(), effect.HoldingID.String(), view.Quantity, formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String())
		return err
	}
	return &domain.Error{Code: domain.ErrIntegrity, Message: "rebuild effect target is unsupported"}
}

func replaceCurrentProjectionsTx(ctx context.Context, tx *sql.Tx, rebuild domain.DerivedDataRebuild) error {
	for _, table := range []string{"account_values", "account_cash_values", "holding_quantity_values"} {
		var scope string
		if table == "holding_quantity_values" {
			scope = `holding_id IN (SELECT h.id FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE a.household_id = ?)`
		} else {
			scope = `account_id IN (SELECT id FROM accounts WHERE household_id = ?)`
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE projection_kind = 'replay' AND `+scope, rebuild.HouseholdID.String()); err != nil {
			return err
		}
	}
	stamp := formatTimestamp(rebuild.AsOf)
	seen := map[string]bool{}
	holdingCount := 0
	for _, view := range rebuild.Current {
		var key string
		switch view.Target {
		case domain.EffectTargetAccountValue:
			if view.AccountID == nil || view.Amount == "" || view.Currency == "" {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "staged account value is incomplete"}
			}
			key = "value|" + view.AccountID.String()
			if _, err := domain.ParseMoney(view.Amount, view.Currency); err != nil {
				return err
			}
		case domain.EffectTargetAccountCash:
			if view.AccountID == nil || view.Amount == "" || view.Currency == "" {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "staged cash value is incomplete"}
			}
			key = "cash|" + view.AccountID.String() + "|" + view.Currency.String()
			if _, err := domain.ParseMoney(view.Amount, view.Currency); err != nil {
				return err
			}
		case domain.EffectTargetHoldingQuantity:
			if view.HoldingID == nil || view.Quantity == "" {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "staged holding quantity is incomplete"}
			}
			key = "holding|" + view.HoldingID.String()
			if _, err := domain.ParseQuantity(view.Quantity); err != nil {
				return err
			}
			holdingCount++
		default:
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged current target is unsupported"}
		}
		if seen[key] {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged current target is duplicated"}
		}
		seen[key] = true
		var result sql.Result
		var err error
		switch view.Target {
		case domain.EffectTargetAccountValue:
			result, err = tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) SELECT ?, a.id, a.tracking_mode, ?, ?, ?, ?, NULL, 'replay' FROM accounts a WHERE a.id = ? AND a.household_id = ? AND a.default_currency = ? AND a.tracking_mode IN ('balance','manual_value')`, domain.NewAccountValueID().String(), view.Amount, view.Currency.String(), stamp, stamp, view.AccountID.String(), rebuild.HouseholdID.String(), view.Currency.String())
		case domain.EffectTargetAccountCash:
			result, err = tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) SELECT ?, a.id, ?, ?, ?, ?, NULL, 'replay' FROM accounts a WHERE a.id = ? AND a.household_id = ?`, domain.NewAccountCashValueID().String(), view.Amount, view.Currency.String(), stamp, stamp, view.AccountID.String(), rebuild.HouseholdID.String())
		case domain.EffectTargetHoldingQuantity:
			result, err = tx.ExecContext(ctx, `UPDATE holdings SET quantity = ? WHERE id = ? AND account_id IN (SELECT id FROM accounts WHERE household_id = ?)`, view.Quantity, view.HoldingID.String(), rebuild.HouseholdID.String())
			if err == nil {
				var replayResult sql.Result
				replayResult, err = tx.ExecContext(ctx, `INSERT INTO holding_quantity_values(id, holding_id, quantity, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, NULL, 'replay')`, domain.NewHoldingQuantityValueID().String(), view.HoldingID.String(), view.Quantity, stamp, stamp)
				_ = replayResult
			}
		}
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "staged current target belongs to another household or is missing"}
		}
	}
	var actualHoldings int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE a.household_id = ?`, rebuild.HouseholdID.String()).Scan(&actualHoldings); err != nil {
		return err
	}
	if holdingCount != actualHoldings {
		return &domain.Error{Code: domain.ErrIntegrity, Message: "staged current quantities do not cover every holding"}
	}
	return nil
}

func replaceDailySnapshotsTx(ctx context.Context, tx *sql.Tx, rebuild domain.DerivedDataRebuild) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM daily_valuation_snapshot_items WHERE snapshot_id IN (SELECT id FROM daily_valuation_snapshots WHERE household_id = ?)`, rebuild.HouseholdID.String()); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM daily_valuation_snapshots WHERE household_id = ? ORDER BY local_date DESC, revision DESC`, rebuild.HouseholdID.String())
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM daily_valuation_snapshots WHERE id = ?`, id); err != nil {
			return err
		}
	}
	for _, snapshot := range rebuild.Snapshots {
		snapshot.Revision = 0
		snapshot.SupersedesID = nil
		snapshot.InputGeneration = rebuild.InputGeneration
		snapshot.ResolverPolicyVersion = domain.MarketDataResolverPolicy
		if _, err := saveDailyValuationSnapshotTx(ctx, tx, snapshot); err != nil {
			return err
		}
	}
	return nil
}
