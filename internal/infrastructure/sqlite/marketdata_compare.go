package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadMarketComparisonSnapshot includes inactive facts. It intentionally does
// not use the active-only chart/history API as absence evidence.
func (r *Repository) ReadMarketComparisonSnapshot(ctx context.Context, scopes []domain.MarketComparisonScope, now time.Time) (domain.MarketComparisonSnapshot, error) {
	out := domain.MarketComparisonSnapshot{Managed: map[string]bool{}}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.Household, err = scanHousehold(tx.QueryRowContext(ctx, `SELECT id,name,base_currency,created_at,updated_at FROM households WHERE singleton_key=1`))
	if err != nil {
		return out, err
	}
	if out.Household == nil {
		return out, &domain.Error{Code: domain.ErrUnavailable, Message: "household unavailable"}
	}
	h := out.Household.ID
	out.Origin, err = historyOriginQuery(ctx, tx, h)
	if err != nil {
		return out, err
	}
	if out.Origin != nil {
		if _, err = time.LoadLocation(out.Origin.Timezone); err != nil {
			return out, err
		}
	}
	scope := marketScopedQuery{queryer: tx, budgetCandidates: true, fxPredicate: "0"}
	ip, fp := []string{"0"}, []string{"0"}
	ia, fa := []any{}, []any{}
	pairs := []string{"0"}
	for _, s := range scopes {
		datePredicate := []string{"0"}
		args := []any{}
		// Undated legacy quotes only need a conservative three-UTC-day
		// window around each requested civil date. This covers all supported
		// timezone offsets without expanding the target's entire legacy history.
		for _, d := range s.Dates {
			date, e := time.Parse(time.DateOnly, d)
			if e != nil {
				return out, e
			}
			datePredicate = append(datePredicate, "(COALESCE(q.effective_date,'')='' AND substr(q.quoted_at,1,10) BETWEEN ? AND ?)")
			args = append(args, date.AddDate(0, 0, -1).Format(time.DateOnly), date.AddDate(0, 0, 1).Format(time.DateOnly))
		}
		if len(s.Dates) > 0 {
			marks := make([]string, len(s.Dates))
			for i, d := range s.Dates {
				marks[i] = "?"
				args = append(args, d)
			}
			datePredicate = append(datePredicate, "q.effective_date IN ("+strings.Join(marks, ",")+")")
		}
		for _, at := range s.Instants {
			// Scan the whole UTC second, then compare parsed instants exactly.
			// This includes supported legacy RFC3339 forms without a fraction.
			second := at.UTC().Truncate(time.Second)
			datePredicate = append(datePredicate, "(q.quoted_at>=? AND q.quoted_at<?)")
			args = append(args, second.Format("2006-01-02T15:04:05"), second.Add(time.Second).Format("2006-01-02T15:04:05"))
		}
		dates := "(" + comparisonSQLOr(datePredicate) + ")"
		if s.Target.InstrumentID != "" {
			scope.instrumentIDs = append(scope.instrumentIDs, s.Target.InstrumentID)
			ip = append(ip, "(q.instrument_id=? AND "+dates+")")
			ia = append(ia, s.Target.InstrumentID)
			ia = append(ia, args...)
		} else {
			pair := "((q.base_currency=? AND q.quote_currency=?) OR (q.base_currency=? AND q.quote_currency=?))"
			values := []any{s.Target.BaseCurrency, s.Target.QuoteCurrency, s.Target.QuoteCurrency, s.Target.BaseCurrency}
			pairs = append(pairs, pair)
			scope.fxArgs = append(scope.fxArgs, values...)
			fp = append(fp, "("+pair+" AND "+dates+")")
			fa = append(fa, values...)
			fa = append(fa, args...)
		}
	}
	scope.fxPredicate = comparisonSQLOr(pairs)
	out.Instruments, err = listInstrumentsQuery(ctx, scope, h, true)
	if err != nil {
		return out, err
	}
	for _, i := range out.Instruments {
		var managed bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_contracts WHERE instrument_id=? AND household_id=?)`, i.ID.String(), h.String()).Scan(&managed)
		if err != nil {
			return out, err
		}
		out.Managed[i.ID.String()] = managed
	}
	out.CurrentInstrumentQuotes, err = listInstrumentQuotesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	out.CurrentFXQuotes, err = listLatestFXQuotesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	if len(out.CurrentInstrumentQuotes) > 20000 || len(out.CurrentFXQuotes) > 20000 {
		return out, &domain.Error{Code: "too_large", Message: "current evidence exceeds row budget"}
	}
	out.FXPreferences, err = listFXPreferencesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	iw := "EXISTS(SELECT 1 FROM instruments i WHERE i.id=q.instrument_id AND i.household_id=?) AND (" + comparisonSQLOr(ip) + ")"
	fw := "q.household_id=? AND (" + comparisonSQLOr(fp) + ")"
	ia = append([]any{h.String()}, ia...)
	fa = append([]any{h.String()}, fa...)
	for _, part := range []struct {
		table, where, columns string
		args                  []any
	}{{"instrument_quotes", iw, marketInstrumentQuoteColumns, ia}, {"fx_quotes", fw, marketFXQuoteColumns, fa}} {
		if err = checkReadTableBudget(ctx, tx, part.table+" q", part.where, part.args...); err != nil {
			return out, err
		}
		rows, e := tx.QueryContext(ctx, "SELECT "+part.columns+" FROM "+part.table+" q WHERE "+part.where+" ORDER BY q.id LIMIT 20001", part.args...)
		if e != nil {
			return out, e
		}
		if part.table == "instrument_quotes" {
			out.InstrumentQuotes, e = scanInstrumentQuotes(rows)
		} else {
			out.FXQuotes, e = scanFXQuotes(rows)
		}
		rows.Close()
		if e != nil {
			return out, e
		}
	}
	// Include every operation originating or targeting a fact in scope, even if
	// a correction moves the replacement to a different market day. Links outside
	// the evidence scope are identifiers, not a claim that all history was read.
	auditWhere := `a.household_id=? AND ((a.target_type='instrument' AND EXISTS(SELECT 1 FROM instrument_quotes q WHERE (` + iw + `) AND (q.id=a.quote_id OR q.id=a.target_quote_id))) OR (a.target_type='fx' AND EXISTS(SELECT 1 FROM fx_quotes q WHERE (` + fw + `) AND (q.id=a.quote_id OR q.id=a.target_quote_id))))`
	aa := append([]any{h.String()}, ia...)
	aa = append(aa, fa...)
	if err = checkReadTableBudget(ctx, tx, "agent_quote_records a", auditWhere, aa...); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.operation,COALESCE(a.quote_id,''),COALESCE(a.target_quote_id,''),a.source_title,a.source_url,a.created_at FROM agent_quote_records a WHERE `+auditWhere+` ORDER BY a.id LIMIT 20001`, aa...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var a domain.MarketComparisonAudit
		var at string
		if err = rows.Scan(&a.RecordID, &a.Operation, &a.QuoteID, &a.TargetQuoteID, &a.SourceTitle, &a.SourceURL, &at); err != nil {
			rows.Close()
			return out, err
		}
		a.CreatedAt, err = parsePortfolioTime(at)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Audits = append(out.Audits, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// A left-deep OR chain can exceed SQLite's expression depth before the 500-row
// request limit. A balanced tree preserves parameter order and every predicate.
func comparisonSQLOr(parts []string) string {
	if len(parts) == 0 {
		return "0"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	middle := len(parts) / 2
	return "(" + comparisonSQLOr(parts[:middle]) + " OR " + comparisonSQLOr(parts[middle:]) + ")"
}
