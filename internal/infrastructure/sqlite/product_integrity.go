package sqlite

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Shared by live open, migrations and read-only backup verification.
func verifyProductIntegrity(ctx context.Context, query schemaQuery) error {
	checks := []struct{ sql, message string }{
		{`SELECT COUNT(*) FROM product_operations WHERE NOT json_valid(result_json) OR NOT json_valid(request_json)`, "product operation evidence is invalid JSON"},
		{`SELECT COUNT(*) FROM product_contracts c WHERE NOT EXISTS(SELECT 1 FROM liquidity_policies p WHERE p.household_id=c.household_id AND p.source_kind='holding' AND p.account_id=c.account_id AND p.holding_id=c.holding_id)`, "managed product policy is missing"},
		{`SELECT COUNT(*) FROM product_operations o WHERE
 (SELECT COUNT(*) FROM product_operation_products p WHERE p.operation_id=o.id) != CASE WHEN o.kind='renew' THEN 2 WHEN o.kind='undo' THEN (SELECT COUNT(*) FROM product_operation_products p WHERE p.operation_id=o.reverses_operation_id) ELSE 1 END
 OR (o.kind='undo' AND (o.reverses_operation_id IS NULL OR NOT EXISTS(SELECT 1 FROM product_operations target WHERE target.id=o.reverses_operation_id AND target.kind NOT IN ('undo','value_observation') AND target.household_id=o.household_id)))
 OR (o.kind!='undo' AND o.reverses_operation_id IS NOT NULL)
 OR (o.kind!='value_observation' AND NOT EXISTS(SELECT 1 FROM product_operation_activities a WHERE a.operation_id=o.id))`, "required product operation members are missing"},
		{`SELECT COUNT(*) FROM product_operations o WHERE o.kind!='value_observation' AND (
 json_extract(o.result_json,'$.receipt.OperationID') IS NOT o.id
 OR json_extract(o.result_json,'$.receipt.Kind') IS NOT o.kind
 OR json_array_length(o.result_json,'$.receipt.ProductIDs') IS NOT (SELECT COUNT(*) FROM product_operation_products p WHERE p.operation_id=o.id)
 OR json_array_length(o.result_json,'$.receipt.ActivityIDs') IS NOT (SELECT COUNT(*) FROM product_operation_activities a WHERE a.operation_id=o.id)
 OR EXISTS(SELECT 1 FROM json_each(o.result_json,'$.receipt.ProductIDs') j WHERE NOT EXISTS(SELECT 1 FROM product_operation_products p WHERE p.operation_id=o.id AND p.product_id=j.value))
 OR EXISTS(SELECT 1 FROM json_each(o.result_json,'$.receipt.ActivityIDs') j WHERE NOT EXISTS(SELECT 1 FROM product_operation_activities a WHERE a.operation_id=o.id AND a.activity_id=j.value AND a.sequence=j.key+1))
 )`, "product receipt and financial member evidence disagree"},
		{`SELECT COUNT(*) FROM product_operation_products p JOIN product_operations o ON o.id=p.operation_id WHERE
 (o.kind IN ('open','record_existing') AND (p.role!='opened' OR NOT EXISTS(SELECT 1 FROM product_operation_activities a WHERE a.operation_id=o.id AND a.product_id=p.product_id AND a.purpose=CASE o.kind WHEN 'open' THEN 'acquisition' ELSE 'existing_position' END)))
 OR (o.kind='receive_interest' AND (p.role!='income' OR NOT EXISTS(SELECT 1 FROM product_operation_activities a WHERE a.operation_id=o.id AND a.product_id=p.product_id AND a.purpose='interest')))
 OR (o.kind IN ('settle','renew') AND ((p.role NOT IN ('opened','settled')) OR NOT EXISTS(SELECT 1 FROM product_operation_activities a WHERE a.operation_id=o.id AND a.product_id=p.product_id AND a.purpose=CASE p.role WHEN 'opened' THEN 'acquisition' ELSE 'redemption' END)))
 OR (o.kind='undo' AND NOT EXISTS(SELECT 1 FROM product_operation_products original WHERE original.operation_id=o.reverses_operation_id AND original.product_id=p.product_id AND p.role=CASE original.role WHEN 'opened' THEN 'cancelled' WHEN 'settled' THEN 'reopened' ELSE original.role END))`, "product operation roles or financial purposes are inconsistent"},
		{`SELECT COUNT(*) FROM product_operation_activities a JOIN product_operations o ON o.id=a.operation_id WHERE
 NOT EXISTS(SELECT 1 FROM product_operation_products p WHERE p.operation_id=a.operation_id AND p.product_id=a.product_id)
 OR (o.kind='undo' AND a.purpose!='reversal')
 OR (o.kind='value_observation')`, "product activity is not a valid operation member"},
		{`SELECT COUNT(*) FROM product_contracts c
 LEFT JOIN accounts a ON a.id=c.account_id
 LEFT JOIN holdings h ON h.id=c.holding_id
 LEFT JOIN instruments i ON i.id=c.instrument_id
 LEFT JOIN product_operations o ON o.id=c.opened_operation_id
 LEFT JOIN product_operations x ON x.id=c.closed_operation_id
 WHERE a.id IS NULL OR h.id IS NULL OR i.id IS NULL OR o.id IS NULL
 OR a.household_id!=c.household_id OR i.household_id!=c.household_id OR o.household_id!=c.household_id
 OR h.account_id!=c.account_id OR h.instrument_id!=c.instrument_id OR i.quote_currency!=c.currency
 OR i.instrument_type!='bank_investment_product' OR a.tracking_mode!='holdings' OR a.balance_sheet_role!='asset'
 OR (c.state='open' AND (h.quantity!='1' OR c.closed_operation_id IS NOT NULL))
 OR (c.state!='open' AND (h.quantity!='0' OR x.id IS NULL OR x.household_id!=c.household_id))
 OR EXISTS(SELECT 1 FROM holdings other WHERE other.instrument_id=c.instrument_id AND other.id!=c.holding_id)`, "product ownership, currency, private holding or lifecycle is inconsistent"},
		{`SELECT COUNT(*) FROM product_contracts c LEFT JOIN product_contracts p ON p.id=c.renewed_from_id
 WHERE c.renewed_from_id IS NOT NULL AND (p.id IS NULL OR p.id=c.id OR p.household_id!=c.household_id OR p.account_id!=c.account_id OR p.currency!=c.currency)`, "product renewal lineage is inconsistent"},
		{`SELECT COUNT(*) FROM liquidity_policies p LEFT JOIN accounts a ON a.id=p.account_id LEFT JOIN holdings h ON h.id=p.holding_id
 WHERE a.id IS NULL OR a.household_id!=p.household_id OR (p.source_kind='holding' AND (h.id IS NULL OR h.account_id!=p.account_id OR p.currency IS NOT NULL))
 OR EXISTS(SELECT 1 FROM product_contracts c WHERE c.holding_id=p.holding_id AND p.accessible_amount_cap IS NOT NULL)`, "policy source ownership or managed cap is inconsistent"},
		{`SELECT COUNT(*) FROM liquidity_reservations r LEFT JOIN accounts a ON a.id=r.account_id LEFT JOIN holdings h ON h.id=r.holding_id LEFT JOIN instruments i ON i.id=h.instrument_id
 WHERE a.id IS NULL OR a.household_id!=r.household_id OR (r.source_kind='holding' AND (h.id IS NULL OR h.account_id!=r.account_id OR i.quote_currency!=r.currency))
 OR (r.source_kind='account_value' AND r.currency!=a.default_currency)`, "reservation source ownership or currency is inconsistent"},
		{`SELECT COUNT(*) FROM product_operation_products l JOIN product_operations o ON o.id=l.operation_id JOIN product_contracts c ON c.id=l.product_id WHERE o.household_id!=c.household_id`, "product operation ownership is inconsistent"},
		{`SELECT COUNT(*) FROM product_operation_activities l JOIN product_operations o ON o.id=l.operation_id JOIN product_contracts c ON c.id=l.product_id JOIN activities a ON a.id=l.activity_id WHERE o.household_id!=c.household_id OR o.household_id!=a.household_id OR EXISTS(SELECT 1 FROM activity_effects e WHERE e.activity_id=a.id AND e.currency IS NOT NULL AND e.currency!='' AND e.currency!=c.currency)`, "product activity currency or ownership is inconsistent"},
		{`SELECT COUNT(*) FROM product_operation_reservations l JOIN product_operations o ON o.id=l.operation_id JOIN liquidity_reservations r ON r.id=l.reservation_id WHERE o.household_id!=r.household_id`, "product reservation operation ownership is inconsistent"},
	}
	for _, check := range checks {
		var count int
		if err := query.QueryRowContext(ctx, check.sql).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return storedIntegrity("product", check.message)
		}
	}
	rows, err := query.QueryContext(ctx, `SELECT id FROM households`)
	if err != nil {
		return err
	}
	ids := []domain.HouseholdID{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		id, err := domain.ParseHouseholdID(raw)
		if err != nil {
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
		contracts, err := listProductContractsQuery(ctx, query, id)
		if err != nil {
			return asStoredIntegrity("product", err)
		}
		managed := map[string]bool{}
		contractsBySource := map[string]domain.ProductContract{}
		for _, c := range contracts {
			if err := c.Validate(); err != nil {
				return asStoredIntegrity("product", err)
			}
			managed[domain.HoldingSourceRef(c.AccountID, c.HoldingID).Key()] = true
			contractsBySource[domain.HoldingSourceRef(c.AccountID, c.HoldingID).Key()] = c
		}
		policies, err := listLiquidityPoliciesQuery(ctx, query, id)
		if err != nil {
			return asStoredIntegrity("policy", err)
		}
		for _, p := range policies {
			if c, ok := contractsBySource[p.Source.Key()]; ok {
				if err := p.ValidateProductDates(c.Kind, c.MaturityOn); err != nil {
					return asStoredIntegrity("policy", err)
				}
			}
			if err := p.Validate(managed[p.Source.Key()]); err != nil {
				return asStoredIntegrity("policy", err)
			}
		}
		reservations, err := listLiquidityReservationsQuery(ctx, query, id, true)
		if err != nil {
			return asStoredIntegrity("reservation", err)
		}
		for _, r := range reservations {
			if err := r.Validate(); err != nil {
				return asStoredIntegrity("reservation", err)
			}
		}
	}
	return nil
}
