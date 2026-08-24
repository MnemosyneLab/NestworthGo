package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ListCostBasisEvents returns one bounded, Holding-scoped read. Transfer-in
// events retain the source Holding ID; GainService resolves that source's
// historical average before calling the pure replay function.
func (r *Repository) ListCostBasisEvents(ctx context.Context, holdingID domain.HoldingID) ([]domain.CostBasisEvent, error) {
	return listCostBasisEventsQuery(ctx, r.database.SQL, holdingID)
}

func listCostBasisEventsQuery(ctx context.Context, query queryer, holdingID domain.HoldingID) ([]domain.CostBasisEvent, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT e.activity_id, a.effective_at, a.created_at, a.kind, a.reason,
		       e.direction, e.role, e.classification, e.quantity, e.cost_unit_price,
		       t.side, t.unit_price, t.gross_currency, t.fee_amount, t.fee_currency,
		       source_effect.holding_id
		FROM activity_effects e
		JOIN activities a ON a.id = e.activity_id
		JOIN holdings h ON h.id = e.holding_id AND h.archived_at IS NULL
		JOIN accounts owner_account ON owner_account.id = h.account_id
		LEFT JOIN activity_trade_details t ON t.activity_id = a.id
		LEFT JOIN activity_effects source_effect
		  ON source_effect.activity_id = e.activity_id
		 AND source_effect.target = 'holding_quantity'
		 AND source_effect.direction = 'removed'
		 AND source_effect.role = 'transfer_from'
		WHERE e.holding_id = ?
		  AND e.target = 'holding_quantity'
		  AND a.household_id = owner_account.household_id
		  AND a.reverses_activity_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM activities reversal WHERE reversal.reverses_activity_id = a.id)
		ORDER BY a.effective_at ASC, a.created_at ASC, a.id ASC, e.sequence ASC`, holdingID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.CostBasisEvent, 0)
	for rows.Next() {
		event, err := scanCostBasisEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanCostBasisEvent(scanner interface{ Scan(...any) error }) (domain.CostBasisEvent, error) {
	var activityID, effectiveAt, createdAt, activityKind, reason string
	var direction, role, classification, quantity, costUnitPrice sql.NullString
	var side, unitPrice, grossCurrency, feeAmount, feeCurrency, sourceHoldingID sql.NullString
	if err := scanner.Scan(&activityID, &effectiveAt, &createdAt, &activityKind, &reason, &direction, &role, &classification, &quantity, &costUnitPrice, &side, &unitPrice, &grossCurrency, &feeAmount, &feeCurrency, &sourceHoldingID); err != nil {
		return domain.CostBasisEvent{}, err
	}
	parsedActivityID, err := domain.ParseActivityID(activityID)
	if err != nil {
		return domain.CostBasisEvent{}, err
	}
	effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
	if err != nil {
		return domain.CostBasisEvent{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.CostBasisEvent{}, err
	}
	parsedQuantity, err := domain.ParseQuantity(quantity.String)
	if err != nil {
		return domain.CostBasisEvent{}, err
	}
	event := domain.CostBasisEvent{ActivityID: parsedActivityID, EffectiveAt: effective.UTC(), Quantity: parsedQuantity}
	if costUnitPrice.Valid {
		parsed, parseErr := domain.ParseUnitPrice(costUnitPrice.String)
		if parseErr != nil {
			return domain.CostBasisEvent{}, parseErr
		}
		event.UnitCost = &parsed
	}
	if sourceHoldingID.Valid {
		parsed, parseErr := domain.ParseHoldingID(sourceHoldingID.String)
		if parseErr != nil {
			return domain.CostBasisEvent{}, parseErr
		}
		event.SourceHoldingID = &parsed
	}

	switch activityKind {
	case string(domain.ActivityBuy), string(domain.ActivitySell):
		if !side.Valid || !unitPrice.Valid || !grossCurrency.Valid {
			return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "trade cost-basis event is missing its Trade detail"}
		}
		parsedPrice, parseErr := domain.ParseUnitPrice(unitPrice.String)
		if parseErr != nil {
			return domain.CostBasisEvent{}, parseErr
		}
		currency, parseErr := domain.ParseCurrency(grossCurrency.String)
		if parseErr != nil {
			return domain.CostBasisEvent{}, parseErr
		}
		event.UnitPrice = &parsedPrice
		event.Currency = currency
		if feeAmount.Valid != feeCurrency.Valid {
			return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "trade fee amount and currency must appear together"}
		}
		if feeAmount.Valid {
			fee, feeErr := domain.ParseMoney(feeAmount.String, domain.CurrencyCode(feeCurrency.String))
			if feeErr != nil {
				return domain.CostBasisEvent{}, feeErr
			}
			event.Fee = &fee
		}
		if side.String == "buy" && activityKind == string(domain.ActivityBuy) {
			event.Kind = domain.CostBasisBuy
		} else if side.String == "sell" && activityKind == string(domain.ActivitySell) {
			event.Kind = domain.CostBasisSell
		} else {
			return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "Activity kind and Trade side do not match"}
		}
	case string(domain.ActivityPositionTransfer):
		if reason == string(domain.ReasonReconciliation) || classification.String == string(domain.ClassificationRemeasurement) {
			if direction.String == string(domain.EffectAdded) {
				event.Kind = domain.CostBasisAdjustmentIn
			} else if direction.String == string(domain.EffectRemoved) {
				event.Kind = domain.CostBasisAdjustmentOut
			} else {
				return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "position adjustment direction is invalid"}
			}
		} else if role.String == string(domain.EffectRoleTransferTo) && direction.String == string(domain.EffectAdded) {
			event.Kind = domain.CostBasisTransferIn
			if event.SourceHoldingID == nil {
				return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "position transfer is missing its source Holding"}
			}
		} else if role.String == string(domain.EffectRoleTransferFrom) && direction.String == string(domain.EffectRemoved) {
			event.Kind = domain.CostBasisTransferOut
		} else {
			return domain.CostBasisEvent{}, &domain.Error{Code: domain.ErrIntegrity, Message: "position transfer effect is invalid"}
		}
	default:
		return domain.CostBasisEvent{}, fmt.Errorf("unsupported Holding activity kind %q", activityKind)
	}
	_ = created // created_at participates in SQL ordering; EffectiveAt is the domain event time.
	return event, nil
}
func (r *Repository) StartingPointCost(ctx context.Context, holdingID domain.HoldingID) (*domain.UnitPrice, error) {
	var value sql.NullString
	err := r.database.SQL.QueryRowContext(ctx, `
		SELECT c.unit_cost
		FROM history_origin_components c
		JOIN history_origins o ON o.id = c.origin_id
		JOIN holdings h ON h.id = c.holding_id AND h.archived_at IS NULL
		JOIN accounts owner_account ON owner_account.id = h.account_id AND owner_account.household_id = o.household_id
		WHERE c.holding_id = ?
		  AND c.instrument_id = h.instrument_id
		  AND c.component_kind = 'holding_quantity'
		  AND CAST(c.quantity AS REAL) > 0
		ORDER BY c.created_at ASC, c.id ASC
		LIMIT 1`, holdingID.String()).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !value.Valid {
		return nil, nil
	}
	parsed, err := domain.ParseUnitPrice(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
