package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Version 1's explicit projection is intentionally independent of schema discovery.
// Adding a database column must never silently extend the public export contract.
var exportQueries = []struct{ section, name, query string }{
	{"directory", "households", `SELECT id AS id, name AS name, base_currency AS baseCurrency, created_at AS createdAt, updated_at AS updatedAt FROM households ORDER BY 1, 2, 3, 4, 5`},
	{"directory", "members", `SELECT id AS id, household_id AS householdId, name AS name, note AS note, sort_order AS sortOrder, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt FROM members ORDER BY 1, 2, 3, 4, 5, 6, 7, 8`},
	{"directory", "institutions", `SELECT id AS id, household_id AS householdId, name AS name, institution_type AS institutionType, country_code AS countryCode, website AS website, note AS note, sort_order AS sortOrder, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt FROM institutions ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11`},
	{"directory", "groups", `SELECT id AS id, household_id AS householdId, name AS name, color AS color, description AS description, sort_order AS sortOrder, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt FROM account_groups ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9`},
	{"directory", "accounts", `SELECT id AS id, household_id AS householdId, institution_id AS institutionId, group_id AS groupId, name AS name, account_type AS accountType, balance_sheet_role AS balanceSheetRole, tracking_mode AS trackingMode, default_currency AS defaultCurrency, note AS note, include_in_net_worth AS includeInNetWorth, include_in_portfolio AS includeInPortfolio, include_in_liquid_assets AS includeInLiquidAssets, opened_on AS openedOn, closed_on AS closedOn, sort_order AS sortOrder, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt FROM accounts ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19`},
	{"directory", "ownership", `SELECT account_id AS accountId, member_id AS memberId, share_bps AS shareBps FROM account_ownership ORDER BY 1, 2, 3`},
	{"directory", "instruments", `SELECT id AS id, household_id AS householdId, name AS name, instrument_type AS instrumentType, quote_currency AS quoteCurrency, symbol AS symbol, market_code AS marketCode, country_code AS countryCode, isin AS isin, note AS note, sort_order AS sortOrder, quote_source AS quoteSource, provider_key AS providerKey, provider_symbol AS providerSymbol, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt, metal_template AS metalTemplate, quantity_unit AS quantityUnit FROM instruments ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19`},
	{"directory", "holdings", `SELECT id AS id, account_id AS accountId, instrument_id AS instrumentId, note AS note, sort_order AS sortOrder, created_at AS createdAt, updated_at AS updatedAt, archived_at AS archivedAt FROM holdings ORDER BY 1, 2, 3, 4, 5, 6, 7, 8`},
	{"history", "origins", `SELECT id AS id, household_id AS householdId, timezone AS timezone, started_at AS startedAt, created_at AS createdAt FROM history_origins ORDER BY 1, 2, 3, 4, 5`},
	{"history", "openingPositions", `SELECT id AS id, origin_id AS originId, component_kind AS componentKind, account_id AS accountId, holding_id AS holdingId, instrument_id AS instrumentId, amount AS amount, currency AS currency, quantity AS quantity, created_at AS createdAt, unit_cost AS unitCost FROM history_origin_components ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11`},
	{"history", "openingAccountStates", `SELECT origin_id AS originId, account_id AS accountId, archived_at AS archivedAt, include_in_net_worth AS includeInNetWorth, include_in_portfolio AS includeInPortfolio, include_in_liquid_assets AS includeInLiquidAssets, created_at AS createdAt FROM history_origin_account_states ORDER BY 1, 2, 3, 4, 5, 6, 7`},
	{"history", "openingOwnership", `SELECT origin_id AS originId, account_id AS accountId, member_id AS memberId, share_bps AS shareBps FROM history_origin_ownership ORDER BY 1, 2, 3, 4`},
	{"history", "openingInstrumentPreferences", `SELECT origin_id AS originId, instrument_id AS instrumentId, source_kind AS sourceKind, created_at AS createdAt FROM history_origin_instrument_preferences ORDER BY 1, 2, 3, 4`},
	{"history", "openingFXPreferences", `SELECT origin_id AS originId, currency_a AS currencyA, currency_b AS currencyB, source_kind AS sourceKind, created_at AS createdAt FROM history_origin_fx_preferences ORDER BY 1, 2, 3, 4, 5`},
	{"history", "activities", `SELECT id AS id, household_id AS householdId, kind AS kind, reason AS reason, effective_at AS effectiveAt, effective_local_date AS effectiveLocalDate, created_at AS createdAt, note AS note, reverses_activity_id AS reversesActivityId, correction_group_id AS correctionGroupId, transaction_fx_rate AS transactionFxRate FROM activities ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11`},
	{"history", "effects", `SELECT id AS id, activity_id AS activityId, sequence AS sequence, role AS role, direction AS direction, target AS target, classification AS classification, account_id AS accountId, holding_id AS holdingId, instrument_id AS instrumentId, amount AS amount, currency AS currency, quantity AS quantity, cost_unit_price AS costUnitPrice FROM activity_effects ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14`},
	{"history", "trades", `SELECT activity_id AS activityId, side AS side, instrument_id AS instrumentId, holding_id AS holdingId, quantity AS quantity, gross_amount AS grossAmount, gross_currency AS grossCurrency, unit_price AS unitPrice, fee_amount AS feeAmount, fee_currency AS feeCurrency FROM activity_trade_details ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10`},
	{"history", "dividends", `SELECT activity_id AS activityId, holding_id AS holdingId, instrument_id AS instrumentId, amount AS amount, currency AS currency FROM activity_dividend_details ORDER BY 1, 2, 3, 4, 5`},
	{"history", "corrections", `SELECT id AS id, household_id AS householdId, original_activity_id AS originalActivityId, replacement_activity_id AS replacementActivityId, created_at AS createdAt FROM activity_correction_groups ORDER BY 1, 2, 3, 4, 5`},
	{"history", "accountStates", `SELECT id AS id, account_id AS accountId, effective_at AS effectiveAt, archived_at AS archivedAt, include_in_net_worth AS includeInNetWorth, include_in_portfolio AS includeInPortfolio, include_in_liquid_assets AS includeInLiquidAssets, activity_id AS activityId, created_at AS createdAt FROM account_state_observations ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9`},
	{"history", "accountStateOwnership", `SELECT observation_id AS observationId, member_id AS memberId, share_bps AS shareBps FROM account_state_ownership ORDER BY 1, 2, 3`},
	{"history", "instrumentStates", `SELECT id AS id, instrument_id AS instrumentId, effective_at AS effectiveAt, archived_at AS archivedAt, activity_id AS activityId, created_at AS createdAt FROM instrument_state_observations ORDER BY 1, 2, 3, 4, 5, 6`},
	{"history", "holdingStates", `SELECT id AS id, holding_id AS holdingId, effective_at AS effectiveAt, archived_at AS archivedAt, activity_id AS activityId, created_at AS createdAt FROM holding_state_observations ORDER BY 1, 2, 3, 4, 5, 6`},
	{"history", "instrumentPreferences", `SELECT id AS id, instrument_id AS instrumentId, source_kind AS sourceKind, effective_at AS effectiveAt, activity_id AS activityId, created_at AS createdAt FROM instrument_preference_observations ORDER BY 1, 2, 3, 4, 5, 6`},
	{"history", "fxPreferences", `SELECT id AS id, household_id AS householdId, currency_a AS currencyA, currency_b AS currencyB, source_kind AS sourceKind, effective_at AS effectiveAt, activity_id AS activityId, created_at AS createdAt FROM fx_preference_observations ORDER BY 1, 2, 3, 4, 5, 6, 7, 8`},
	{"history", "balanceBaselines", `SELECT id AS id, account_id AS accountId, value_kind AS valueKind, amount AS amount, currency AS currency, effective_at AS effectiveAt, created_at AS createdAt FROM account_values WHERE projection_kind = 'baseline' ORDER BY 1, 2, 3, 4, 5, 6, 7`},
	{"history", "cashBaselines", `SELECT id AS id, account_id AS accountId, amount AS amount, currency AS currency, effective_at AS effectiveAt, created_at AS createdAt FROM account_cash_values WHERE projection_kind = 'baseline' ORDER BY 1, 2, 3, 4, 5, 6`},
	{"history", "quantityBaselines", `SELECT id AS id, holding_id AS holdingId, quantity AS quantity, effective_at AS effectiveAt, created_at AS createdAt FROM holding_quantity_values WHERE projection_kind = 'baseline' ORDER BY 1, 2, 3, 4, 5`},
	{"marketData", "instrumentQuotes", `SELECT id AS id, instrument_id AS instrumentId, unit_price AS unitPrice, currency AS currency, source_kind AS sourceKind, source_key AS sourceKey, quoted_at AS quotedAt, created_at AS createdAt, delayed AS delayed, observation_kind AS observationKind, effective_date AS effectiveDate, provider_timestamp AS providerTimestamp, fetched_at AS fetchedAt, value_effective_at AS valueEffectiveAt, binding_revision AS bindingRevision, source_policy_version AS sourcePolicyVersion, price_basis AS priceBasis, timestamp_basis AS timestampBasis, revision AS revision, supersedes_quote_id AS supersedesQuoteId, split_factor AS splitFactor, dividend_cash AS dividendCash, conversion_json AS conversion FROM instrument_quotes ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22`},
	{"marketData", "fxQuotes", `SELECT id AS id, household_id AS householdId, base_currency AS baseCurrency, quote_currency AS quoteCurrency, rate AS rate, source_kind AS sourceKind, source_key AS sourceKey, quoted_at AS quotedAt, created_at AS createdAt, delayed AS delayed, observation_kind AS observationKind, effective_date AS effectiveDate, fetched_at AS fetchedAt, value_effective_at AS valueEffectiveAt, source_policy_version AS sourcePolicyVersion, timestamp_basis AS timestampBasis, revision AS revision, supersedes_quote_id AS supersedesQuoteId FROM fx_quotes ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18`},
	{"marketData", "fxPreferences", `SELECT household_id AS householdId, currency_a AS currencyA, currency_b AS currencyB, source_kind AS sourceKind, created_at AS createdAt, updated_at AS updatedAt FROM fx_preferences ORDER BY 1, 2, 3, 4, 5, 6`},
	{"marketData", "providerBindings", `SELECT instrument_id AS instrumentId, provider_key AS providerKey, provider_symbol AS providerSymbol, market AS market, currency AS currency, enabled AS enabled, created_at AS createdAt, updated_at AS updatedAt, binding_revision AS bindingRevision, effective_from AS effectiveFrom FROM instrument_provider_bindings ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10`},
	{"marketData", "providerBindingHistory", `SELECT instrument_id AS instrumentId, provider_key AS providerKey, binding_revision AS bindingRevision, provider_symbol AS providerSymbol, market AS market, currency AS currency, enabled AS enabled, effective_from AS effectiveFrom, created_at AS createdAt FROM instrument_provider_binding_revisions ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9`},
	{"marketData", "instrumentObservationSlots", `SELECT instrument_id AS instrumentId, provider_key AS providerKey, binding_revision AS bindingRevision, source_policy_version AS sourcePolicyVersion, market_date AS marketDate, observation_kind AS observationKind, quote_id AS quoteId, updated_at AS updatedAt FROM instrument_observation_slots ORDER BY 1, 2, 3, 4, 5, 6, 7, 8`},
	{"marketData", "fxObservationSlots", `SELECT household_id AS householdId, base_currency AS baseCurrency, quote_currency AS quoteCurrency, provider_key AS providerKey, source_policy_version AS sourcePolicyVersion, market_date AS marketDate, observation_kind AS observationKind, quote_id AS quoteId, updated_at AS updatedAt FROM fx_observation_slots ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9`},
	{"marketData", "coverage", `SELECT target_type AS targetType, target_id AS targetId, provider_key AS providerKey, household_id AS householdId, binding_revision AS bindingRevision, source_policy_version AS sourcePolicyVersion, effective_date AS effectiveDate, status AS status, reason AS reason, checked_at AS checkedAt FROM market_data_day_status ORDER BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10`},
}

func (r *Repository) ReadExportSnapshot(ctx context.Context) (domain.ExportSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ExportSnapshot{}, err
	}
	defer tx.Rollback()
	result, err := readExportSnapshot(ctx, tx)
	if err != nil {
		return domain.ExportSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ExportSnapshot{}, err
	}
	return result, nil
}

func readExportSnapshot(ctx context.Context, tx *sql.Tx) (domain.ExportSnapshot, error) {
	result := domain.ExportSnapshot{Facts: domain.ExportFacts{
		Directory: domain.ExportDatasets{}, History: domain.ExportDatasets{}, MarketData: domain.ExportDatasets{},
	}, CostEvents: map[domain.HoldingID][]domain.CostBasisEvent{}, StartingCosts: map[domain.HoldingID]*domain.UnitPrice{}}
	// This first SELECT establishes the snapshot shared by all subsequent reads.
	portfolio, err := readPortfolioSnapshotQuery(ctx, tx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return result, err
	}
	result.Portfolio = portfolio
	for _, spec := range exportQueries {
		records, err := readExportRecords(ctx, tx, spec.query)
		if err != nil {
			return result, err
		}
		switch spec.section {
		case "directory":
			result.Facts.Directory[spec.name] = records
		case "history":
			result.Facts.History[spec.name] = records
		case "marketData":
			result.Facts.MarketData[spec.name] = records
		}
	}
	for _, holding := range portfolio.Holdings {
		events, err := listCostBasisEventsQuery(ctx, tx, holding.ID, true)
		if err != nil {
			return result, err
		}
		result.CostEvents[holding.ID] = events
		cost, err := startingPointCostQuery(ctx, tx, holding.ID, true)
		if err != nil {
			return result, err
		}
		result.StartingCosts[holding.ID] = cost
	}
	return result, nil
}

func readExportRecords(ctx context.Context, tx *sql.Tx, query string) ([]domain.ExportRecord, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	records := make([]domain.ExportRecord, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		record := domain.ExportRecord{}
		for i, name := range columns {
			value := values[i]
			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}
			switch name {
			case "conversion":
				if value != nil && value != "" {
					var conversion domain.ExportConversion
					if err := json.Unmarshal([]byte(value.(string)), &conversion); err != nil {
						return nil, err
					}
					value = conversion
				} else {
					value = nil
				}

			case "includeInNetWorth", "includeInPortfolio", "includeInLiquidAssets", "enabled", "delayed":
				if value != nil {
					value = value == int64(1)
				}
			}
			record[name] = value
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
