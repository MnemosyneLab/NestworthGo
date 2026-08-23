package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) CommitActivity(ctx context.Context, activity domain.Activity, effects []domain.ActivityEffect, resulting []domain.EndpointView, asOf time.Time) error {
	return r.CommitActivityBatch(ctx, []domain.ActivityCommit{{Activity: activity, Effects: effects, Resulting: resulting}}, asOf)
}

func (r *Repository) CommitActivityBatch(ctx context.Context, commits []domain.ActivityCommit, asOf time.Time) error {
	if len(commits) == 0 {
		return &domain.Error{Code: domain.ErrInvalidChange, Field: "activities", Message: "at least one activity is required"}
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var originTimezone string
		if err := tx.QueryRowContext(ctx, `SELECT timezone FROM history_origins WHERE household_id = ?`, commits[0].Activity.HouseholdID.String()).Scan(&originTimezone); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a change"}
			}
			return err
		}
		for _, commit := range commits {
			if commit.Activity.HouseholdID != commits[0].Activity.HouseholdID {
				return &domain.Error{Code: domain.ErrInvalidChange, Field: "householdId", Message: "an activity batch must belong to one Household"}
			}
			if err := commitActivityTx(ctx, tx, commit, asOf); err != nil {
				return err
			}
			if err := markHistoryDirtyTx(ctx, tx, commit.Activity.HouseholdID, commit.Activity.EffectiveLocalDate, originTimezone, asOf); err != nil {
				return err
			}
		}
		for _, inverse := range commits {
			if inverse.Activity.CorrectionGroupID == nil || inverse.Activity.ReversesActivityID == nil {
				continue
			}
			for _, replacement := range commits {
				if replacement.Activity.CorrectionGroupID == nil || *replacement.Activity.CorrectionGroupID != *inverse.Activity.CorrectionGroupID || replacement.Activity.ReversesActivityID != nil {
					continue
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO activity_correction_groups(id, household_id, original_activity_id, replacement_activity_id, created_at) VALUES(?, ?, ?, ?, ?)`, inverse.Activity.CorrectionGroupID.String(), inverse.Activity.HouseholdID.String(), inverse.Activity.ReversesActivityID.String(), replacement.Activity.ID.String(), formatTimestamp(asOf)); err != nil {
					return err
				}
				break
			}
		}
		return nil
	})
}

func commitActivityTx(ctx context.Context, tx *sql.Tx, commit domain.ActivityCommit, asOf time.Time) error {
	activity, effects, resulting := commit.Activity, commit.Effects, commit.Resulting
	if len(effects) == 0 {
		return &domain.Error{Code: domain.ErrInvalidChange, Field: "effects", Message: "a saved change requires at least one effect"}
	}
	for _, effect := range effects {
		if err := effect.Validate(); err != nil {
			return err
		}
		if err := validateActiveEffectTargetTx(ctx, tx, activity.HouseholdID, effect); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, activity.ID.String(), activity.HouseholdID.String(), activity.Kind.String(), string(activity.Reason), formatTimestamp(activity.EffectiveAt), activity.EffectiveLocalDate, formatTimestamp(activity.CreatedAt), nullableString(activity.Note), nullableActivityID(activity.ReversesActivityID), nullableCorrectionGroupID(activity.CorrectionGroupID), nullableRate(activity.TransactionFXRate)); err != nil {
		return err
	}
	for _, effect := range effects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity_effects(id, activity_id, sequence, role, direction, target, classification, account_id, holding_id, instrument_id, amount, currency, quantity) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, effect.ID.String(), activity.ID.String(), effect.Sequence, string(effect.Role), string(effect.Direction), string(effect.Target), string(effect.Classification), nullableID(effect.AccountID), nullableID(effect.HoldingID), nullableID(effect.InstrumentID), nullableMoney(effect.Money), nullableMoneyCurrency(effect.Money), nullableQuantity(effect.Quantity)); err != nil {
			return err
		}
		view, err := endpointForEffect(effect, resulting)
		if err != nil {
			return err
		}
		switch effect.Target {
		case domain.EffectTargetAccountValue:
			if effect.AccountID == nil || effect.Money == nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "Account value effect is incomplete"}
			}
			var valueKind string
			if err := tx.QueryRowContext(ctx, `SELECT tracking_mode FROM accounts WHERE id = ? AND household_id = ?`, effect.AccountID.String(), activity.HouseholdID.String()).Scan(&valueKind); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'event')`, domain.NewAccountValueID().String(), effect.AccountID.String(), valueKind, view.Amount, view.Currency.String(), formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String()); err != nil {
				return err
			}
			if activity.EffectiveAt.Before(asOf) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'replay')`, domain.NewAccountValueID().String(), effect.AccountID.String(), valueKind, view.Amount, view.Currency.String(), formatTimestamp(asOf), formatTimestamp(asOf), effect.ID.String()); err != nil {
					return err
				}
			}
		case domain.EffectTargetAccountCash:
			if effect.AccountID == nil || effect.Money == nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "Account cash effect is incomplete"}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, 'event')`, domain.NewAccountCashValueID().String(), effect.AccountID.String(), view.Amount, view.Currency.String(), formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String()); err != nil {
				return err
			}
			if activity.EffectiveAt.Before(asOf) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, 'replay')`, domain.NewAccountCashValueID().String(), effect.AccountID.String(), view.Amount, view.Currency.String(), formatTimestamp(asOf), formatTimestamp(asOf), effect.ID.String()); err != nil {
					return err
				}
			}
		case domain.EffectTargetHoldingQuantity:
			if effect.HoldingID == nil || effect.Quantity == nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "Holding quantity effect is incomplete"}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO holding_quantity_values(id, holding_id, quantity, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, 'event')`, domain.NewHoldingQuantityValueID().String(), effect.HoldingID.String(), view.Quantity, formatTimestamp(activity.EffectiveAt), formatTimestamp(activity.CreatedAt), effect.ID.String()); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE holdings SET quantity = ?, updated_at = ? WHERE id = ?`, view.Quantity, formatTimestamp(activity.CreatedAt), effect.HoldingID.String())
			if err != nil {
				return err
			}
			if err := requireAffected(result, "holding"); err != nil {
				return err
			}
			if activity.EffectiveAt.Before(asOf) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO holding_quantity_values(id, holding_id, quantity, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, 'replay')`, domain.NewHoldingQuantityValueID().String(), effect.HoldingID.String(), view.Quantity, formatTimestamp(asOf), formatTimestamp(asOf), effect.ID.String()); err != nil {
					return err
				}
			}
		default:
			return &domain.Error{Code: domain.ErrIntegrity, Message: "unsupported effect target"}
		}
	}
	if activity.TradeDetail != nil {
		trade := activity.TradeDetail
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity_trade_details(activity_id, side, instrument_id, holding_id, quantity, gross_amount, gross_currency, unit_price, fee_amount, fee_currency) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, activity.ID.String(), string(trade.Side), trade.InstrumentID.String(), trade.HoldingID.String(), trade.Quantity.Canonical(), trade.Gross.CanonicalAmount(), trade.Gross.Currency().String(), trade.UnitPrice.Canonical(), nullableMoneyAmount(trade.Fee), nullableMoneyCurrency(trade.Fee)); err != nil {
			return err
		}
	}
	return nil
}

func validateActiveEffectTargetTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, effect domain.ActivityEffect) error {
	switch effect.Target {
	case domain.EffectTargetAccountValue, domain.EffectTargetAccountCash:
		if effect.AccountID == nil {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "Account effect is missing its Account"}
		}
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT archived_at FROM accounts WHERE id = ? AND household_id = ?`, effect.AccountID.String(), householdID.String()).Scan(&archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
			}
			return err
		}
		if archived.Valid {
			return &domain.Error{Code: domain.ErrConflict, Field: "accountId", Message: "Account is archived"}
		}
	case domain.EffectTargetHoldingQuantity:
		if effect.HoldingID == nil {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "Holding effect is missing its Holding"}
		}
		var holdingArchived, accountArchived, instrumentArchived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT h.archived_at, a.archived_at, i.archived_at FROM holdings h JOIN accounts a ON a.id = h.account_id JOIN instruments i ON i.id = h.instrument_id WHERE h.id = ? AND a.household_id = ?`, effect.HoldingID.String(), householdID.String()).Scan(&holdingArchived, &accountArchived, &instrumentArchived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Field: "holdingId", Message: "Holding was not found"}
			}
			return err
		}
		if holdingArchived.Valid || accountArchived.Valid || instrumentArchived.Valid {
			return &domain.Error{Code: domain.ErrConflict, Field: "holdingId", Message: "Holding, its Account, or Instrument is archived"}
		}
	}
	return nil
}

func endpointForEffect(effect domain.ActivityEffect, resulting []domain.EndpointView) (domain.EndpointView, error) {
	for _, view := range resulting {
		if view.Target != effect.Target {
			continue
		}
		if effect.AccountID != nil && view.AccountID != nil && *effect.AccountID == *view.AccountID {
			if effect.Money == nil || view.Currency == effect.Money.Currency() {
				return view, nil
			}
		}
		if effect.HoldingID != nil && view.HoldingID != nil && *effect.HoldingID == *view.HoldingID {
			return view, nil
		}
	}
	return domain.EndpointView{}, &domain.Error{Code: domain.ErrIntegrity, Message: "change result does not contain an affected endpoint"}
}

func markHistoryDirtyTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, effectiveLocalDate, timezone string, asOf time.Time) error {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	var startedAt string
	if err := tx.QueryRowContext(ctx, `SELECT started_at FROM history_origins WHERE household_id = ?`, householdID.String()).Scan(&startedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	origin, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return &domain.Error{Code: domain.ErrIntegrity, Message: "stored history Starting point is invalid"}
	}
	originDate := origin.In(location).Format("2006-01-02")
	if effectiveLocalDate < originDate {
		effectiveLocalDate = originDate
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if effectiveLocalDate >= asOf.In(location).Format("2006-01-02") {
		return nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from IS NULL OR dirty_from > ? THEN ? ELSE dirty_from END, updated_at = ? WHERE household_id = ?`, effectiveLocalDate, effectiveLocalDate, formatTimestamp(asOf), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "history snapshot state")
}

func nullableActivityID(value *domain.ActivityID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableCorrectionGroupID(value *domain.ActivityCorrectionGroupID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableRate(value *domain.FxRate) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}

func nullableMoneyAmount(value *domain.Money) any {
	if value == nil {
		return nil
	}
	return value.CanonicalAmount()
}
