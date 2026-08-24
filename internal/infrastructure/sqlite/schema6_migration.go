package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type schema6Holding struct {
	originID        string
	originCreatedAt string
	holdingID       string
	accountID       string
	instrumentID    string
	quantity        string
	components      []schema6Component
}

type schema6Component struct {
	id       string
	quantity string
	unitCost sql.NullString
}

// backfillSchema6CostBasis completes development fixtures while the schema
// 5 -> 6 migration transaction is still open. It mirrors
// selectInstrumentQuote: the selected source and currency must match the
// Instrument, then the newest quote wins by quoted_at, created_at, and id.
// A non-reversed trade is the deterministic fallback when no selected quote
// exists. No Activity or projection row is created by this repair.
func backfillSchema6CostBasis(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT o.id, o.created_at, h.id, h.account_id, h.instrument_id, h.quantity,
		       c.id, c.quantity, c.unit_cost
		FROM history_origins o
		JOIN accounts a ON a.household_id = o.household_id
		JOIN holdings h ON h.account_id = a.id
		LEFT JOIN history_origin_components c
		  ON c.origin_id = o.id
		 AND c.holding_id = h.id
		 AND c.instrument_id = h.instrument_id
		 AND c.component_kind = 'holding_quantity'
		WHERE h.quantity <> '0'
		ORDER BY o.id, h.id, c.id`)
	if err != nil {
		return err
	}
	var loaded []schema6Holding
	byKey := make(map[string]int)
	for rows.Next() {
		var holding schema6Holding
		var componentID, componentQuantity, unitCost sql.NullString
		if err := rows.Scan(&holding.originID, &holding.originCreatedAt, &holding.holdingID, &holding.accountID, &holding.instrumentID, &holding.quantity, &componentID, &componentQuantity, &unitCost); err != nil {
			_ = rows.Close()
			return err
		}
		key := holding.originID + "\x00" + holding.holdingID
		index, ok := byKey[key]
		if !ok {
			byKey[key] = len(loaded)
			loaded = append(loaded, holding)
			index = len(loaded) - 1
		}
		if componentID.Valid {
			loaded[index].components = append(loaded[index].components, schema6Component{id: componentID.String, quantity: componentQuantity.String, unitCost: unitCost})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, holding := range loaded {
		quantity, err := domain.ParseQuantity(holding.quantity)
		if err != nil {
			return fmt.Errorf("schema 6 cost-basis backfill: holding %s quantity: %w", holding.holdingID, err)
		}
		if quantity.IsZero() {
			continue
		}
		hasPositiveCost, err := schema6HasPositiveCost(holding.components)
		if err != nil {
			return fmt.Errorf("schema 6 cost-basis backfill: holding %s component: %w", holding.holdingID, err)
		}
		if hasPositiveCost {
			continue
		}
		cost, err := schema6ResolveCost(ctx, tx, holding)
		if err != nil {
			return err
		}
		updated := false
		for _, component := range holding.components {
			componentQuantity, parseErr := domain.ParseQuantity(component.quantity)
			if parseErr != nil {
				return fmt.Errorf("schema 6 cost-basis backfill: component %s quantity: %w", component.id, parseErr)
			}
			if componentQuantity.IsZero() {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE history_origin_components SET unit_cost = ? WHERE id = ?`, cost, component.id); err != nil {
				return err
			}
			updated = true
			break
		}
		if updated {
			continue
		}
		componentID := domain.NewHistoryOriginComponentID().String()
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_components(id, origin_id, component_kind, account_id, holding_id, instrument_id, quantity, created_at, unit_cost) VALUES(?, ?, 'holding_quantity', ?, ?, ?, ?, ?, ?)`, componentID, holding.originID, holding.accountID, holding.holdingID, holding.instrumentID, holding.quantity, holding.originCreatedAt, cost); err != nil {
			return err
		}
	}
	return nil
}

func schema6HasPositiveCost(components []schema6Component) (bool, error) {
	for _, component := range components {
		quantity, err := domain.ParseQuantity(component.quantity)
		if err != nil {
			return false, err
		}
		if quantity.IsZero() {
			continue
		}
		if !component.unitCost.Valid {
			continue
		}
		if _, err := domain.ParseUnitPrice(component.unitCost.String); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func schema6ResolveCost(ctx context.Context, tx *sql.Tx, holding schema6Holding) (string, error) {
	var price string
	err := tx.QueryRowContext(ctx, `
		SELECT q.unit_price
		FROM instrument_quotes q
		JOIN instruments i ON i.id = q.instrument_id
		WHERE q.instrument_id = ?
		  AND q.source_kind = i.quote_source
		  AND q.currency = i.quote_currency
		ORDER BY q.quoted_at DESC, q.created_at DESC, q.id DESC
		LIMIT 1`, holding.instrumentID).Scan(&price)
	if err == nil {
		parsed, parseErr := domain.ParseUnitPrice(price)
		if parseErr != nil {
			return "", fmt.Errorf("schema 6 cost-basis backfill: selected quote for holding %s: %w", holding.holdingID, parseErr)
		}
		return parsed.Canonical(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT td.unit_price
		FROM activity_trade_details td
		JOIN activities a ON a.id = td.activity_id
		WHERE td.holding_id = ?
		  AND a.reverses_activity_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM activities reversal WHERE reversal.reverses_activity_id = a.id)
		ORDER BY a.effective_at DESC, a.created_at DESC, a.id DESC
		LIMIT 1`, holding.holdingID).Scan(&price)
	if err == nil {
		parsed, parseErr := domain.ParseUnitPrice(price)
		if parseErr != nil {
			return "", fmt.Errorf("schema 6 cost-basis backfill: trade price for holding %s: %w", holding.holdingID, parseErr)
		}
		return parsed.Canonical(), nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("schema 6 cost-basis backfill: holding %s has no selected Instrument quote or non-reversed trade", holding.holdingID)
	}
	return "", err
}
