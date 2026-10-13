package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) ReadInstrumentMetadata(ctx context.Context, archived bool) ([]domain.InstrumentMetadataState, error) {
	reread, err := r.ReadInstrumentMetadataFiltered(ctx, domain.InstrumentMetadataReadQuery{IncludeArchived: archived})
	return reread.Items, err
}

type metadataFilteredQuery struct {
	queryer
	ids, types []string
}

func (q metadataFilteredQuery) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	suffix := ""
	if len(q.ids) > 0 {
		marks, values := sqlInArgs(q.ids)
		suffix += " AND i.id IN (" + marks + ")"
		args = append(args, values...)
	}
	if len(q.types) > 0 {
		marks, values := sqlInArgs(q.types)
		suffix += " AND i.instrument_type IN (" + marks + ")"
		args = append(args, values...)
	}
	statement = strings.Replace(statement, "i.household_id = ?", "i.household_id = ?"+suffix, 1)
	return q.queryer.QueryContext(ctx, statement, args...)
}
func (r *Repository) ReadInstrumentMetadataFiltered(ctx context.Context, q domain.InstrumentMetadataReadQuery) (domain.InstrumentMetadataReadSnapshot, error) {
	empty := domain.InstrumentMetadataReadSnapshot{Items: []domain.InstrumentMetadataState{}}
	archived := q.IncludeArchived
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	h, err := scanHousehold(tx.QueryRowContext(ctx, `SELECT id,name,base_currency,created_at,updated_at FROM households WHERE singleton_key=1`))
	if err != nil {
		return empty, err
	}
	out := []domain.InstrumentMetadataState{}
	if h == nil {
		return empty, nil
	}
	predicate := "household_id = ?"
	args := []any{h.ID.String()}
	if !archived {
		predicate += " AND archived_at IS NULL"
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM instruments WHERE "+predicate, args...).Scan(&empty.TotalCount); err != nil {
		return empty, err
	}
	if len(q.IDs) > 0 {
		marks, values := sqlInArgs(q.IDs)
		predicate += " AND id IN (" + marks + ")"
		args = append(args, values...)
	}
	if len(q.Types) > 0 {
		marks, values := sqlInArgs(q.Types)
		predicate += " AND instrument_type IN (" + marks + ")"
		args = append(args, values...)
	}
	if err := checkReadTableBudget(ctx, tx, "instruments", predicate, args...); err != nil {
		return empty, err
	}
	items, err := listInstrumentsQuery(ctx, metadataFilteredQuery{queryer: tx, ids: q.IDs, types: q.Types}, h.ID, archived)
	if err != nil {
		return empty, err
	}
	managed := map[string]bool{}
	managedPredicate := "household_id=?"
	managedArgs := []any{h.ID.String()}
	if len(items) == 0 {
		return empty, tx.Commit()
	}
	selectedIDs := make([]string, 0, len(items))
	for _, item := range items {
		selectedIDs = append(selectedIDs, item.ID.String())
	}
	marks, values := sqlInArgs(selectedIDs)
	managedPredicate += " AND instrument_id IN (" + marks + ")"
	managedArgs = append(managedArgs, values...)
	rows, err := tx.QueryContext(ctx, `SELECT instrument_id FROM product_contracts WHERE `+managedPredicate, managedArgs...)
	if err != nil {
		return empty, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return empty, err
		}
		managed[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, err
	}
	for _, i := range items {
		out = append(out, domain.InstrumentMetadataState{Instrument: i, Managed: managed[i.ID.String()]})
	}
	empty.Items = out
	return empty, tx.Commit()
}
func (r *Repository) UpdateInstrumentMetadataBatch(ctx context.Context, household domain.HouseholdID, items []domain.InstrumentMetadataUpdate, now time.Time) (domain.InstrumentMetadataReceipt, error) {
	receipt := domain.InstrumentMetadataReceipt{RequestedCount: len(items), Items: []domain.InstrumentMetadataReceiptItem{}}
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		updated := []domain.Instrument{}
		for _, item := range items {
			row := tx.QueryRowContext(ctx, `SELECT i.id,i.household_id,i.name,i.short_name,i.instrument_type,i.quote_currency,i.symbol,i.market_code,i.country_code,i.isin,i.note,i.icon_key,i.sort_order,i.quote_source,i.provider_key,i.provider_symbol,i.metal_template,i.quantity_unit,i.created_at,i.updated_at,i.archived_at,0 FROM instruments i WHERE i.id=? AND i.household_id=?`, item.ID, household.String())
			current, err := scanInstrumentColumns(row)
			if err != nil {
				return err
			}
			var managed bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_contracts WHERE instrument_id=? AND household_id=?)`, item.ID, household.String()).Scan(&managed); err != nil {
				return err
			}
			if managed {
				return domain.ManagedPositionConflict("instrumentId", "managed instrument requires product terms")
			}
			if current.ArchivedAt != nil {
				return &domain.Error{Code: domain.ErrConflict, Message: "archived instrument cannot be updated"}
			}
			if item.ExpectedMetadataVersion != domain.InstrumentMetadataVersion(current, managed) {
				return &domain.Error{Code: domain.ErrConflict, Message: "instrument metadata changed; reload before saving"}
			}
			next, changed, err := domain.ApplyInstrumentMetadata(current, item.Changes, now)
			if err != nil {
				return err
			}
			receipt.Items = append(receipt.Items, domain.InstrumentMetadataReceiptItem{ID: item.ID, Changed: changed, MetadataVersion: domain.InstrumentMetadataVersion(next, false)})
			if changed {
				updated = append(updated, next)
				receipt.ChangedCount++
			}
		}
		for _, i := range updated {
			result, err := tx.ExecContext(ctx, `UPDATE instruments SET name=?,short_name=?,note=?,updated_at=? WHERE id=? AND household_id=?`, i.Name, i.ShortName, nullableString(i.Note), formatTimestamp(i.UpdatedAt), i.ID.String(), household.String())
			if err != nil {
				return mapPortfolioWriteError(err, "instrument")
			}
			if err := requireAffected(result, "instrument"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.InstrumentMetadataReceipt{}, err
	}
	return receipt, nil
}
