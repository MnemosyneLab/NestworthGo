package sqlite

import (
	"context"
	"database/sql"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func verifyCanonicalNumericText(ctx context.Context, query schemaQuery) error {
	if err := verifyParsedTextColumn(ctx, query, `SELECT quantity FROM holdings`, "quantity", parseStoredQuantity); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT quantity FROM holding_quantity_values`, "quantity", parseStoredQuantity); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT quantity FROM activity_effects WHERE quantity IS NOT NULL AND quantity != ''`, "quantity", parseStoredQuantity); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT quantity FROM history_origin_components WHERE quantity IS NOT NULL AND quantity != ''`, "quantity", parseStoredQuantity); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT quantity FROM activity_trade_details`, "quantity", parseStoredQuantity); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT unit_price FROM instrument_quotes`, "unitPrice", parseStoredUnitPrice); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT unit_price FROM activity_trade_details`, "unitPrice", parseStoredUnitPrice); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT cost_unit_price FROM activity_effects WHERE cost_unit_price IS NOT NULL AND cost_unit_price != ''`, "unitPrice", parseStoredUnitPrice); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT unit_cost FROM history_origin_components WHERE unit_cost IS NOT NULL AND unit_cost != ''`, "unitPrice", parseStoredUnitPrice); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT rate FROM fx_quotes`, "fxRate", parseStoredFxRate); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT transaction_fx_rate FROM activities WHERE transaction_fx_rate IS NOT NULL AND transaction_fx_rate != ''`, "fxRate", parseStoredFxRate); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT amount, currency FROM account_values`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT amount, currency FROM account_cash_values`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT amount, currency FROM activity_effects WHERE amount IS NOT NULL AND amount != ''`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT gross_amount, gross_currency FROM activity_trade_details`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT fee_amount, fee_currency FROM activity_trade_details WHERE fee_amount IS NOT NULL AND fee_amount != ''`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT amount, currency FROM activity_dividend_details`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT amount, currency FROM history_origin_components WHERE amount IS NOT NULL AND amount != ''`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT assets_amount, currency FROM daily_valuation_snapshots WHERE assets_amount IS NOT NULL AND assets_amount != ''`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT liabilities_amount, currency FROM daily_valuation_snapshots WHERE liabilities_amount IS NOT NULL AND liabilities_amount != ''`, "amount", false); err != nil {
		return err
	}
	if err := verifyMoneyPairs(ctx, query, `SELECT net_worth_amount, currency FROM daily_valuation_snapshots WHERE net_worth_amount IS NOT NULL AND net_worth_amount != ''`, "amount", true); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT native_amount FROM daily_valuation_snapshot_items WHERE native_amount IS NOT NULL AND native_amount != ''`, "nativeAmount", parseStoredNativeAmount); err != nil {
		return err
	}
	if err := verifyParsedTextColumn(ctx, query, `SELECT base_amount FROM daily_valuation_snapshot_items WHERE base_amount IS NOT NULL AND base_amount != ''`, "baseAmount", parseStoredNativeAmount); err != nil {
		return err
	}
	return nil
}

func verifySnapshotProvenance(ctx context.Context, query schemaQuery) error {
	checks := []struct {
		statement string
		field     string
		message   string
	}{
		{
			statement: `SELECT COUNT(*) FROM daily_valuation_snapshot_items i
				JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
				LEFT JOIN instrument_quotes q ON q.id = i.quote_id
				LEFT JOIN instruments inst ON inst.id = q.instrument_id AND inst.household_id = s.household_id
				WHERE i.quote_id IS NOT NULL AND i.quote_id != '' AND inst.id IS NULL`,
			field:   "quoteId",
			message: "snapshot quote provenance does not exist in this household",
		},
		{
			statement: `SELECT COUNT(*) FROM daily_valuation_snapshot_items i
				JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
				LEFT JOIN fx_quotes q ON q.id = i.fx_quote_id AND q.household_id = s.household_id
				WHERE i.fx_quote_id IS NOT NULL AND i.fx_quote_id != '' AND q.id IS NULL`,
			field:   "fxQuoteId",
			message: "snapshot FX quote provenance does not exist in this household",
		},
		{
			statement: `SELECT COUNT(*) FROM daily_valuation_snapshot_items i
				JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
				LEFT JOIN account_state_observations o ON o.id = i.state_observation_id
				LEFT JOIN accounts a ON a.id = o.account_id AND a.household_id = s.household_id
				WHERE i.state_observation_id IS NOT NULL AND i.state_observation_id != '' AND a.id IS NULL`,
			field:   "stateObservationId",
			message: "snapshot account-state provenance does not exist in this household",
		},
		{
			statement: `SELECT COUNT(*) FROM daily_valuation_snapshot_items i
				JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
				LEFT JOIN instrument_preference_observations o ON o.id = i.preference_observation_id
				LEFT JOIN instruments inst ON inst.id = o.instrument_id AND inst.household_id = s.household_id
				WHERE i.preference_observation_id IS NOT NULL AND i.preference_observation_id != '' AND inst.id IS NULL`,
			field:   "preferenceObservationId",
			message: "snapshot instrument-preference provenance does not exist in this household",
		},
		{
			statement: `SELECT COUNT(*) FROM daily_valuation_snapshot_items i
				JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
				LEFT JOIN fx_preference_observations o ON o.id = i.fx_preference_observation_id AND o.household_id = s.household_id
				WHERE i.fx_preference_observation_id IS NOT NULL AND i.fx_preference_observation_id != '' AND o.id IS NULL`,
			field:   "fxPreferenceObservationId",
			message: "snapshot FX-preference provenance does not exist in this household",
		},
	}
	for _, check := range checks {
		var count int
		if err := query.QueryRowContext(ctx, check.statement).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return storedIntegrity(check.field, check.message)
		}
	}
	return nil
}

func verifyParsedTextColumn(ctx context.Context, query schemaQuery, statement, field string, parse func(string) error) error {
	rows, err := query.QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		if err := parse(value); err != nil {
			return asStoredIntegrity(field, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

func verifyMoneyPairs(ctx context.Context, query schemaQuery, statement, field string, signed bool) error {
	rows, err := query.QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var amount, currency string
		if err := rows.Scan(&amount, &currency); err != nil {
			return err
		}
		var parseErr error
		if signed {
			_, parseErr = domain.ParseSignedMoney(amount, domain.CurrencyCode(currency))
		} else {
			_, parseErr = domain.ParseMoney(amount, domain.CurrencyCode(currency))
		}
		if parseErr != nil {
			return asStoredIntegrity(field, parseErr)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

func parseStoredQuantity(value string) error {
	_, err := domain.ParseQuantity(value)
	return err
}

func parseStoredUnitPrice(value string) error {
	_, err := domain.ParseUnitPrice(value)
	return err
}

func parseStoredFxRate(value string) error {
	_, err := domain.ParseFxRate(value)
	return err
}

func parseStoredNativeAmount(value string) error {
	_, err := domain.ParseNativeAmount(value)
	return err
}

func requireHouseholdRow(ctx context.Context, tx *sql.Tx, statement, id, householdID, field, message string) error {
	var found int
	if err := tx.QueryRowContext(ctx, statement, id, householdID).Scan(&found); err != nil {
		return err
	}
	if found != 1 {
		return storedIntegrity(field, message)
	}
	return nil
}
