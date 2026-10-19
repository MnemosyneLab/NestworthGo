package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const financialTrackingVersion = "c1-household-all-v1"

// A row bound avoids wall-clock/expiry ambiguity. At a pruning boundary keep
// 4096 entries; at most 255 newer entries accrue before the next atomic prune.
const financialJournalRetain = 4096
const financialJournalPruneEvery = 256
const financialJournalMaxRetained = financialJournalRetain + financialJournalPruneEvery - 1

// This explicit roster is a conservative superset of the financial read closure.
// C1 deliberately invalidates every household, including OLD/NEW owners and
// consumers of shared instruments. Scope metadata is diagnostic, never a proof
// that a narrower invalidation or temporal reuse is safe.
var financialSourceTables = strings.Fields(`households members institutions account_groups accounts account_ownership account_values instruments holdings account_cash_values instrument_quotes fx_quotes fx_preferences history_origins history_origin_components history_origin_account_states history_origin_ownership history_origin_instrument_preferences history_origin_fx_preferences activities activity_effects activity_trade_details activity_dividend_details activity_correction_groups holding_quantity_values account_state_observations account_state_ownership instrument_preference_observations fx_preference_observations history_snapshot_state instrument_state_observations holding_state_observations instrument_provider_bindings instrument_provider_binding_revisions instrument_observation_slots fx_observation_slots market_data_day_status product_operations product_contracts liquidity_policies liquidity_reservations product_operation_products product_operation_activities product_operation_reservations agent_quote_records app_configuration`)

// These are outputs, handling/idempotency or provenance receipts, not new
// financial authorities. Schema additions require an explicit classification.
var financialSourceExcluded = strings.Fields(`daily_valuation_snapshots daily_valuation_snapshot_items activity_mutation_keys change_batch_mutation_keys agent_quote_batches reconciliation_sessions reconciliation_accounts reconciliation_items reconciliation_receipts reconciliation_links account_financial_revisions reconciliation_financial_changes investment_review_control investment_review_rules investment_review_rule_versions investment_review_rule_targets investment_review_suppressions investment_review_position_epochs investment_review_daily_inputs investment_review_daily_results investment_review_episodes investment_review_evaluation_state investment_review_episode_events investment_review_legacy_state investment_review_legacy_quarantine investment_review_mutation_receipts financial_source_revision financial_source_changes review_temporal_checkpoints`)

var financialColumnManifest = func() map[string][]schemaColumn {
	tables := expectedSchemaTables()
	for name, cols := range historySchemaColumns() {
		tables[name] = cols
	}
	return tables
}()

func financialSourceColumns(table string) []schemaColumn { return financialColumnManifest[table] }

func financialScope(table, side string) string {
	// The shared durable settings blob contains credentials. Never copy its
	// value, key, JSON paths or hashes into the invalidation journal.
	if table == "app_configuration" {
		return "json_object('configuration',1)"
	}
	fields := []string{}
	for _, c := range financialSourceColumns(table) {
		n := c.name
		keep := c.primaryKey > 0 || strings.HasSuffix(n, "_id")
		switch n {
		case "currency", "base_currency", "quote_currency", "currency_a", "currency_b", "provider_key", "provider_symbol", "binding_revision", "source_policy_version", "projection_kind", "observation_kind", "price_basis", "timestamp_basis", "source_kind", "target_type", "kind", "timezone", "effective_at", "effective_local_date", "quoted_at", "value_effective_at", "provider_timestamp", "market_date", "effective_date", "started_at", "effective_from", "opened_on", "closed_on", "archived_at", "expires_at", "next_check_at", "created_at":
			keep = true
		}
		if keep {
			fields = append(fields, "'"+n+"'", side+"."+n)
		}
	}
	return "json_object(" + strings.Join(fields, ",") + ")"
}
func financialEffective(table, side string) string {
	cols := map[string]bool{}
	for _, c := range financialSourceColumns(table) {
		cols[c.name] = true
	}
	terms := []string{}
	for _, n := range []string{"effective_at", "value_effective_at", "effective_date", "market_date", "effective_from", "started_at", "quoted_at"} {
		if cols[n] {
			terms = append(terms, side+"."+n)
		}
	}
	if len(terms) == 0 {
		// Parent dates are available for direct child edits. During FK cascades
		// the parent may already be gone; NULL explicitly requires full fallback.
		switch table {
		case "activity_effects", "activity_trade_details", "activity_dividend_details":
			return "(SELECT effective_at FROM activities WHERE id=" + side + ".activity_id)"
		case "account_state_ownership":
			return "(SELECT effective_at FROM account_state_observations WHERE id=" + side + ".observation_id)"
		case "history_origin_components", "history_origin_account_states", "history_origin_ownership", "history_origin_instrument_preferences", "history_origin_fx_preferences":
			return "(SELECT started_at FROM history_origins WHERE id=" + side + ".origin_id)"
		case "product_operation_products", "product_operation_activities", "product_operation_reservations":
			return "(SELECT effective_at FROM product_operations WHERE id=" + side + ".operation_id)"
		}
		return "NULL"
	}
	if len(terms) == 1 {
		return terms[0]
	}
	return "COALESCE(" + strings.Join(terms, ",") + ")"
}
func financialRowID(table, side string) string {
	if table == "app_configuration" {
		return "'configuration'"
	}
	keys := []string{}
	for _, c := range financialSourceColumns(table) {
		if c.primaryKey > 0 {
			keys = append(keys, side+"."+c.name)
		}
	}
	if len(keys) == 1 {
		return "CAST(" + keys[0] + " AS TEXT)"
	}
	return "json_array(" + strings.Join(keys, ",") + ")"
}
func financialChangeCondition(table string) string {
	terms := []string{}
	for _, c := range financialSourceColumns(table) {
		if table == "history_snapshot_state" && c.name != "input_generation" && c.name != "resolver_policy_version" && c.name != "household_id" {
			continue
		}
		terms = append(terms, "OLD."+c.name+" IS NOT NEW."+c.name)
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

var financialSourceTriggerDDL = buildFinancialTriggerStatements()

func financialTriggerStatements() map[string]string { return financialSourceTriggerDDL }

func buildFinancialTriggerStatements() map[string]string {
	result := map[string]string{}
	for _, table := range financialSourceTables {
		for _, op := range []string{"insert", "update", "delete"} {
			timing := "BEFORE"
			// BEFORE INSERT also runs for ignored inserts and the UPDATE arm of
			// UPSERT. Only AFTER INSERT proves that a source row was inserted.
			// UPDATE/DELETE remain BEFORE to retain OLD/parent dependencies.
			if op == "insert" {
				timing = "AFTER"
			}
			name := "financial_source_" + table + "_" + op
			condition := ""
			if op == "update" {
				condition = " WHEN " + financialChangeCondition(table)
			}
			// Quantity replay rows are absent from the review read closure. Account
			// and cash replay rows ARE read by its current portfolio latest-value
			// helpers, so conservatively track those until a separate proof permits
			// their exclusion. Review publication itself writes none of these.
			if table == "holding_quantity_values" {
				relevant := "OLD.projection_kind<>'replay'"
				if op == "insert" {
					relevant = "NEW.projection_kind<>'replay'"
				}
				if op == "update" {
					relevant = "(OLD.projection_kind<>'replay' OR NEW.projection_kind<>'replay') AND " + financialChangeCondition(table)
				}
				condition = " WHEN " + relevant
			}
			oldScope, newScope, oldAt, newAt := "NULL", "NULL", "NULL", "NULL"
			side := "NEW"
			if op != "insert" {
				oldScope = financialScope(table, "OLD")
				oldAt = financialEffective(table, "OLD")
			}
			if op != "delete" {
				newScope = financialScope(table, "NEW")
				newAt = financialEffective(table, "NEW")
			} else {
				side = "OLD"
			}
			body := ""
			if table == "households" && op == "insert" {
				body = `INSERT INTO financial_source_revision VALUES(NEW.id,0,0,'` + financialTrackingVersion + `',lower(hex(randomblob(16))));`
			}
			// Missing state cannot silently make a mutation invisible. New households
			// initialize above; ordinary writes never repair or reset a missing token.
			body += `SELECT CASE WHEN (SELECT COUNT(*) FROM households)<>(SELECT COUNT(*) FROM financial_source_revision) THEN RAISE(ABORT,'financial source tracking incomplete') END;`
			body += `UPDATE financial_source_revision SET revision=revision+1;`
			body += `SELECT CASE WHEN changes()<>(SELECT COUNT(*) FROM households) THEN RAISE(ABORT,'financial source revision not advanced') END;`
			body += `INSERT INTO financial_source_changes SELECT household_id,revision,0,'` + table + `',` + financialRowID(table, side) + `,'` + op + `',` + oldScope + `,` + newScope + `,` + oldAt + `,` + newAt + `,'household_all' FROM financial_source_revision;`
			// An outer OR IGNORE policy must not turn failed bookkeeping into a
			// successful source mutation. SQLite restores changes() after nested
			// trigger execution, including the retention trigger below.
			body += `SELECT CASE WHEN changes()<>(SELECT COUNT(*) FROM households) THEN RAISE(ABORT,'financial source journal not published') END;`
			result[name] = "CREATE TRIGGER " + name + " " + timing + " " + strings.ToUpper(op) + " ON " + table + condition + " BEGIN " + body + " END"
		}
	}
	// The PK bounds the prefix delete. No source-history or journal count scan
	// runs per mutation; this trigger executes only once per 256 revisions.
	result["financial_source_journal_retention"] = fmt.Sprintf(`CREATE TRIGGER financial_source_journal_retention AFTER INSERT ON financial_source_changes WHEN NEW.revision>%d AND NEW.revision%%%d=0 BEGIN
UPDATE financial_source_revision SET journal_floor=MAX(journal_floor,NEW.revision-%d) WHERE household_id=NEW.household_id;
SELECT CASE WHEN (SELECT journal_floor FROM financial_source_revision WHERE household_id=NEW.household_id)<NEW.revision-%d THEN RAISE(ABORT,'financial source journal floor not advanced') END;
DELETE FROM financial_source_changes WHERE household_id=NEW.household_id AND revision<=(SELECT journal_floor FROM financial_source_revision WHERE household_id=NEW.household_id);
SELECT CASE WHEN EXISTS(SELECT 1 FROM financial_source_changes WHERE household_id=NEW.household_id AND revision<=(SELECT journal_floor FROM financial_source_revision WHERE household_id=NEW.household_id)) THEN RAISE(ABORT,'financial source journal not pruned') END;
END`, financialJournalRetain, financialJournalPruneEvery, financialJournalRetain, financialJournalRetain)
	return result
}
func installFinancialSourceTracking(ctx context.Context, tx *sql.Tx) error {
	raw, err := schemaFS.ReadFile("source_changes_schema_v20.sql")
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO financial_source_revision SELECT id,0,0,?,lower(hex(randomblob(16))) FROM households`, financialTrackingVersion); err != nil {
		return err
	}
	statements := financialTriggerStatements()
	names := []string{}
	for n := range statements {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if _, err = tx.ExecContext(ctx, statements[n]); err != nil {
			return fmt.Errorf("install %s: %w", n, err)
		}
	}
	// An empty journal is an unknown pre-migration baseline, never evidence of
	// no financial change. Initial capture/publish still uses the full oracle.
	_, err = tx.ExecContext(ctx, `UPDATE investment_review_control SET projection_status=CASE WHEN projection_status='needs_repair' THEN 'needs_repair' ELSE 'pending' END,published_input_fingerprint=NULL,published_on=NULL`)
	return err
}

// ResetFinancialSourceTracking is called only at restore/import boundaries,
// before resuming readers. Normal opens/restarts preserve the epoch. Complete
// backup snapshots retain the original metadata; only the restored live copy
// gets a new epoch and a floor invalidating every prior journal consumer.
func (db *DB) ResetFinancialSourceTracking(ctx context.Context) error {
	// Restore/import cannot create proof that prior review history never existed.
	db.reviewInitialPublicationAllowed.Store(false)
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE financial_source_revision SET tracking_epoch=lower(hex(randomblob(16))),journal_floor=revision`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM financial_source_changes`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET projection_status=CASE WHEN projection_status='needs_repair' THEN 'needs_repair' ELSE 'pending' END,published_input_fingerprint=NULL,published_on=NULL`)
		return err
	})
}

// PruneFinancialSourceChanges is an explicit transactional maintenance boundary,
// not an automatic retention policy. A consumer below the persisted floor must
// fully recapture. C1 has no consumer and never interprets an empty log as no-op.
func (db *DB) PruneFinancialSourceChanges(ctx context.Context, householdID string, through int64) error {
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		var revision, floor int64
		if err := tx.QueryRowContext(ctx, `SELECT revision,journal_floor FROM financial_source_revision WHERE household_id=?`, householdID).Scan(&revision, &floor); err != nil {
			return err
		}
		if through < floor || through > revision {
			return fmt.Errorf("source journal prune outside retained range")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE financial_source_revision SET journal_floor=? WHERE household_id=?`, through, householdID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM financial_source_changes WHERE household_id=? AND revision<=?`, householdID, through)
		return err
	})
}
