package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadGainSnapshot uses query helpers on the same transaction, never public
// repository methods (which would acquire another connection). No replay or
// valuation calculation runs while SQLite holds the read transaction.
func (r *Repository) ReadGainSnapshot(ctx context.Context) (domain.GainSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.GainSnapshot{}, err
	}
	defer tx.Rollback()
	result, err := readGainSnapshotQuery(ctx, tx)
	if err != nil {
		return domain.GainSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.GainSnapshot{}, err
	}
	return result, nil
}

func readGainSnapshotQuery(ctx context.Context, query queryer) (domain.GainSnapshot, error) {
	result := domain.GainSnapshot{CostEvents: make(map[domain.HoldingID][]domain.CostBasisEvent), StartingCosts: make(map[domain.HoldingID]*domain.UnitPrice)}
	var err error
	result.Portfolio, err = readPortfolioSnapshotQuery(ctx, query, domain.AccountFilter{IncludeArchived: true})
	if err != nil || result.Portfolio.Household == nil {
		return result, err
	}
	householdID := result.Portfolio.Household.ID
	// Bulk reads keep query count independent of the number of holdings. Include
	// archived sources: recursive transfer replay must never query the live repo.
	for _, holding := range result.Portfolio.Holdings {
		result.CostEvents[holding.ID] = nil
		result.StartingCosts[holding.ID] = nil
	}
	if err = readGainCosts(ctx, query, householdID, &result); err != nil {
		return domain.GainSnapshot{}, err
	}
	result.HistoricalFXQuotes, err = listFXQuotesQuery(ctx, query, householdID)
	if err != nil {
		return domain.GainSnapshot{}, err
	}
	result.Dividends, err = readGainDividends(ctx, query, householdID)
	if err != nil {
		return domain.GainSnapshot{}, err
	}

	return result, nil
}

// prependScanner reuses the canonical scanners while a bulk query also selects
// the owning holding or dividend columns.
type prependScanner struct {
	row    rowScanner
	prefix []any
}

func (s prependScanner) Scan(dest ...any) error { return s.row.Scan(append(s.prefix, dest...)...) }

func readGainCosts(ctx context.Context, query queryer, householdID domain.HouseholdID, result *domain.GainSnapshot) error {
	statement := strings.Replace(costBasisEventsSelect, "SELECT e.activity_id", "SELECT e.holding_id, e.activity_id", 1)
	rows, err := query.QueryContext(ctx, correctedActivityCTE+statement+` WHERE a.household_id = ?`+validCostBasisEvents+costBasisEventOrder, householdID.String())
	if err != nil {
		return err
	}
	for rows.Next() {
		var holdingID domain.HoldingID
		event, err := scanCostBasisEvent(prependScanner{row: rows, prefix: []any{&holdingID}})
		if err != nil {
			rows.Close()
			return err
		}
		result.CostEvents[holdingID] = append(result.CostEvents[holdingID], event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = query.QueryContext(ctx, `SELECT c.holding_id,c.quantity,c.unit_cost FROM history_origin_components c
 JOIN history_origins o ON o.id=c.origin_id
 JOIN holdings h ON h.id=c.holding_id AND c.instrument_id=h.instrument_id
 JOIN accounts owner_account ON owner_account.id=h.account_id AND owner_account.household_id=o.household_id
 WHERE o.household_id=? AND c.component_kind='holding_quantity' ORDER BY c.created_at,c.id`, householdID.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.HoldingID
		var quantity string
		var cost sql.NullString
		if err := rows.Scan(&id, &quantity, &cost); err != nil {
			return err
		}
		q, err := domain.ParseQuantity(quantity)
		if err != nil {
			return asStoredIntegrity("quantity", err)
		}
		if q.IsZero() || !cost.Valid || cost.String == "" || result.StartingCosts[id] != nil {
			continue
		}
		price, err := domain.ParseUnitPrice(cost.String)
		if err != nil {
			return asStoredIntegrity("unitPrice", err)
		}
		result.StartingCosts[id] = &price
	}
	return rows.Err()
}

func readGainDividends(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.Activity, error) {
	// Gain calculations need the corrected economic date and dividend detail,
	// not audit effects, product contexts or resulting snapshots for every page.
	rows, err := query.QueryContext(ctx, correctedActivityCTE+`SELECT d.holding_id,d.instrument_id,d.amount,d.currency,
 a.id,a.household_id,a.kind,a.reason,a.effective_at,a.effective_local_date,a.created_at,a.note,a.reverses_activity_id,a.correction_group_id,a.transaction_fx_rate
 FROM economic_activities a JOIN activity_dividend_details d ON d.activity_id=a.id
 WHERE a.household_id=? AND a.kind='cash_dividend' AND a.reverses_activity_id IS NULL
 AND NOT EXISTS (SELECT 1 FROM activities reversal WHERE reversal.reverses_activity_id=a.id)
 ORDER BY a.effective_at,a.created_at,a.ordering_id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Activity
	for rows.Next() {
		var holding, instrument, amount, currency string
		activity, err := activityFromScanner(prependScanner{row: rows, prefix: []any{&holding, &instrument, &amount, &currency}})
		if err != nil {
			return nil, err
		}
		activity.DividendDetail, err = parseDividendDetail(holding, instrument, amount, currency)
		if err != nil {
			return nil, err
		}
		result = append(result, activity)
	}
	return result, rows.Err()
}
