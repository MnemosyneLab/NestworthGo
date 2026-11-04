package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Resolve retraction identity through the immediate target, never the root of a
// correction chain. Invalidators are NOT constrained by date or target filters.
func (r *Repository) ReadAgentAudit(ctx context.Context, h domain.HouseholdID, f domain.AgentAuditFilter) (domain.AgentAuditSelection, error) {
	out := domain.AgentAuditSelection{}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out, err = readAgentAuditQuery(ctx, tx, h, f)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func readAgentAuditQuery(ctx context.Context, tx queryer, h domain.HouseholdID, f domain.AgentAuditFilter) (domain.AgentAuditSelection, error) {
	out := domain.AgentAuditSelection{Inactive: map[string]bool{}}

	where := []string{"r.household_id=?"}
	args := []any{h.String()}
	identity := ` FROM agent_quote_records r
 LEFT JOIN instrument_quotes iq ON r.target_type='instrument' AND iq.id=COALESCE(r.quote_id,r.target_quote_id)
 LEFT JOIN agent_instrument_closures c ON r.target_type='instrument' AND c.id=COALESCE(r.quote_id,r.target_quote_id)
 LEFT JOIN fx_quotes fx ON r.target_type='fx' AND fx.id=COALESCE(r.quote_id,r.target_quote_id)`
	if len(f.InstrumentIDs) > 0 {
		marks, vals := sqlInArgs(f.InstrumentIDs)
		where = append(where, "COALESCE(iq.instrument_id,c.instrument_id) IN ("+marks+")")
		args = append(args, vals...)
	}
	if len(f.InstrumentIDs) > 0 {
		marks, vals := sqlInArgs(f.InstrumentIDs)
		matched := "SELECT id FROM instrument_quotes WHERE instrument_id IN (" + marks + ") UNION ALL SELECT id FROM agent_instrument_closures WHERE instrument_id IN (" + marks + ")"
		// Drive both observation and withdrawal branches from target quote IDs.
		where = append(where, `r.rowid IN (WITH matched AS MATERIALIZED (`+matched+`) SELECT a.rowid FROM matched m JOIN agent_quote_records a INDEXED BY ux_agent_quote_records_quote ON a.target_type='instrument' AND a.quote_id=m.id WHERE a.quote_id IS NOT NULL UNION ALL SELECT a.rowid FROM matched m JOIN agent_quote_records a INDEXED BY ux_agent_quote_records_target ON a.target_type='instrument' AND a.target_quote_id=m.id WHERE a.target_quote_id IS NOT NULL)`)
		args = append(args, vals...)
		args = append(args, vals...)
	}

	if len(f.FXPairs) > 0 {
		parts := []string{}
		for _, p := range f.FXPairs {
			parts = append(parts, "(fx.base_currency=? AND fx.quote_currency=?)")
			args = append(args, p.BaseCurrency, p.QuoteCurrency)
		}
		where = append(where, "("+strings.Join(parts, " OR ")+")")
	}
	if len(f.FXPairs) > 0 {
		parts := []string{}
		values := []any{h.String()}
		for _, p := range f.FXPairs {
			parts = append(parts, "(base_currency=? AND quote_currency=?)")
			values = append(values, p.BaseCurrency, p.QuoteCurrency)
		}
		matched := "SELECT id FROM fx_quotes WHERE household_id=? AND (" + strings.Join(parts, " OR ") + ")"
		where = append(where, `r.rowid IN (WITH matched AS MATERIALIZED (`+matched+`) SELECT a.rowid FROM matched m JOIN agent_quote_records a INDEXED BY ux_agent_quote_records_quote ON a.target_type='fx' AND a.quote_id=m.id WHERE a.quote_id IS NOT NULL UNION ALL SELECT a.rowid FROM matched m JOIN agent_quote_records a INDEXED BY ux_agent_quote_records_target ON a.target_type='fx' AND a.target_quote_id=m.id WHERE a.target_quote_id IS NOT NULL)`)
		args = append(args, values...)
	}

	for i, date := range []string{f.FromDate, f.ToDate} {
		if date != "" {
			op := ">="
			if i == 1 {
				op = "<="
			}
			where = append(where, "NULLIF(COALESCE(iq.effective_date,c.effective_date,fx.effective_date),'')"+op+"?")
			args = append(args, date)
		}
	}
	// Persisted timestamps are fixed-width UTC (formatTimestamp); canonicalize
	// offset inputs before comparing so equivalent instants have identical bounds.
	for i, at := range []*time.Time{f.CreatedFrom, f.CreatedTo} {
		if at != nil {
			op := ">="
			if i == 1 {
				op = "<="
			}
			where = append(where, "r.created_at"+op+"?")
			bound := at.UTC()
			if i == 0 && !bound.Equal(bound.Truncate(time.Millisecond)) {
				bound = bound.Truncate(time.Millisecond).Add(time.Millisecond)
			}
			args = append(args, formatTimestamp(bound))
		}
	}
	statement := `SELECT r.id,r.target_type,r.operation,r.quote_id,r.target_quote_id,r.source_title,r.source_url,r.created_at,
 EXISTS(SELECT 1 FROM agent_quote_records x WHERE x.household_id=r.household_id AND x.target_type=r.target_type AND x.target_quote_id=r.quote_id),
 512+length(CAST(r.source_title AS BLOB))+COALESCE(length(CAST(r.source_url AS BLOB)),0) + COALESCE(length(CAST(iq.conversion_json AS BLOB)),0) AS audit_bytes` + identity + " WHERE " + strings.Join(where, " AND ") + " ORDER BY r.rowid"
	rows, err := checkedReadQuery(ctx, tx, statement, "audit_bytes+2048", args...)
	if err != nil {
		return out, err
	}
	type selected struct {
		record      domain.AgentQuoteRecord
		kind, quote string
		inactive    bool
	}
	var selectedRows []selected
	for rows.Next() {
		var v selected
		var quote, target, url sql.NullString
		var op, created string
		if err = rows.Scan(&v.record.RecordID, &v.kind, &op, &quote, &target, &v.record.SourceTitle, &url, &created, &v.inactive, new(int)); err != nil {
			rows.Close()
			return out, err
		}
		v.quote = quote.String
		v.record.TargetQuoteID = target.String
		v.record.SourceURL = url.String
		v.record.Operation = domain.AgentQuoteOperation(op)
		v.record.CreatedAt, err = parsePortfolioTime(created)
		if err != nil {
			rows.Close()
			return out, err
		}
		selectedRows = append(selectedRows, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, v := range selectedRows {
		if v.quote != "" {
			if err = hydrateAgentRecord(ctx, tx, &v.record, v.kind, v.quote); err != nil {
				return out, err
			}
			out.Inactive[v.quote] = v.inactive
		} else if v.record.TargetQuoteID != "" {
			target := domain.AgentQuoteRecord{}
			if err = hydrateAgentRecord(ctx, tx, &target, v.kind, v.record.TargetQuoteID); err != nil {
				return out, err
			}
			out.Identities = append(out.Identities, target)
		}
		out.Records = append(out.Records, v.record)
	}
	return out, nil
}
