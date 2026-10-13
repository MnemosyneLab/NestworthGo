package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Scope only the existing current-candidate SQL, retaining its daily/realtime/
// converted grouping and correction predicates. All predicates are generated
// from validated typed targets, with values bound separately.
type marketScopedQuery struct {
	queryer
	instrumentIDs []string
	fxPredicate   string
	fxArgs        []any
}

func (q marketScopedQuery) scope(statement string, args []any) (string, []any) {
	if strings.Contains(statement, "i.household_id = ?") {
		marks, values := sqlInArgs(q.instrumentIDs)
		if len(q.instrumentIDs) == 0 {
			statement = strings.Replace(statement, "i.household_id = ?", "i.household_id = ? AND 0", 1)
		} else {
			statement = strings.Replace(statement, "i.household_id = ?", "i.household_id = ? AND i.id IN ("+marks+")", 1)
			args = append(args, values...)
		}
	} else if strings.Contains(statement, "FROM fx_preferences") {
		pred := strings.ReplaceAll(strings.ReplaceAll(q.fxPredicate, "q.base_currency", "currency_a"), "q.quote_currency", "currency_b")
		statement = strings.Replace(statement, "household_id = ?", "household_id = ? AND ("+pred+")", 1)
		args = append(args, q.fxArgs...)
	} else if strings.Contains(statement, "q.household_id = ?") {
		statement = strings.Replace(statement, "q.household_id = ?", "q.household_id = ? AND ("+q.fxPredicate+")", 1)
		args = append(args, q.fxArgs...)
	}
	return statement, args
}
func (q marketScopedQuery) QueryRowContext(ctx context.Context, statement string, args ...any) *sql.Row {
	statement, args = q.scope(statement, args)
	return q.queryer.QueryRowContext(ctx, statement, args...)
}
func (q marketScopedQuery) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	statement, args = q.scope(statement, args)
	statement += " LIMIT 20001"
	if strings.Contains(statement, "WITH candidates") {
		return q.queryer.QueryContext(ctx, statement, args...)
	}
	expression := "512"
	if strings.Contains(statement, "WITH candidates") {
		expression = "1024+length(CAST(source_key AS BLOB))"
		if strings.Contains(statement, "instrument_quotes") {
			expression += "+COALESCE(length(CAST(conversion_json AS BLOB)),0)"
		}
	}
	if strings.Contains(statement, "FROM instruments i") {
		expression = "512+length(CAST(name AS BLOB))+COALESCE(length(CAST(note AS BLOB)),0)"
	}
	var count, size int
	if err := q.queryer.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM("+expression+"),0) FROM ("+statement+")", args...).Scan(&count, &size); err != nil {
		return nil, err
	}
	if err := domain.ReserveReadBudget(ctx, fmt.Sprint(statement, args), count, size); err != nil {
		return nil, err
	}
	return q.queryer.QueryContext(ctx, statement, args...)
}
func (r *Repository) ReadMarketDataSnapshot(ctx context.Context, in domain.MarketReadRequest, now time.Time) (domain.MarketReadSnapshot, error) {
	out := domain.MarketReadSnapshot{InstrumentHistory: map[string][]domain.InstrumentQuote{}, FXHistory: map[string][]domain.FXQuote{}, OutsideRange: map[string]bool{}}
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
		return out, nil
	}
	h := out.Household.ID
	out.Origin, err = historyOriginQuery(ctx, tx, h)
	if err != nil {
		return out, err
	}
	ids := []string{}
	pairs := []domain.MarketReadTarget{}
	for _, t := range in.Targets {
		if t.InstrumentID != "" {
			ids = append(ids, t.InstrumentID)
		} else {
			pairs = append(pairs, t)
		}
	}
	scope := marketScopedQuery{queryer: tx, instrumentIDs: ids}
	out.Instruments, err = listInstrumentsQuery(ctx, scope, h, true)
	if err != nil {
		return out, err
	}
	for _, i := range out.Instruments {
		if i.MetalTemplate != "" && i.QuoteCurrency != "USD" {
			pairs = append(pairs, domain.MarketReadTarget{BaseCurrency: "USD", QuoteCurrency: i.QuoteCurrency.String()})
		}
	}
	predicates := []string{"0"}
	for _, p := range pairs {
		predicates = append(predicates, "(q.base_currency=? AND q.quote_currency=?) OR (q.base_currency=? AND q.quote_currency=?)")
		scope.fxArgs = append(scope.fxArgs, p.BaseCurrency, p.QuoteCurrency, p.QuoteCurrency, p.BaseCurrency)
	}
	scope.fxPredicate = strings.Join(predicates, " OR ")
	out.CurrentInstrumentQuotes, err = listInstrumentQuotesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	if len(out.CurrentInstrumentQuotes) > 20000 {
		return out, &domain.Error{Code: "too_large", Message: "capture exceeds current quote row budget"}
	}
	out.CurrentFXQuotes, err = listLatestFXQuotesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	if len(out.CurrentFXQuotes) > 20000 {
		return out, &domain.Error{Code: "too_large", Message: "capture exceeds current FX row budget"}
	}
	out.FXPreferences, err = listFXPreferencesQuery(ctx, scope, h)
	if err != nil {
		return out, err
	}
	location := time.UTC
	if out.Origin != nil {
		l, err := time.LoadLocation(out.Origin.Timezone)
		if err != nil {
			return out, err
		}
		location = l
	}
	trend, _ := domain.ParseTrendRange(in.Range)
	filter, _ := domain.ParseQuoteSourceFilter(in.SourceFilter)
	query := domain.NewQuoteReadWindow(trend, filter, now, location, "", "")
	if in.View != "current" {
		for _, t := range in.Targets {
			where := ""
			args := []any{}
			table := "instrument_quotes"
			if t.InstrumentID != "" {
				where = "q.instrument_id=? AND EXISTS(SELECT 1 FROM instruments i WHERE i.id=q.instrument_id AND i.household_id=?)"
				args = []any{t.InstrumentID, h.String()}
			} else {
				table = "fx_quotes"
				where = "q.household_id=? AND ((q.base_currency=? AND q.quote_currency=?) OR (q.base_currency=? AND q.quote_currency=?))"
				args = []any{h.String(), t.BaseCurrency, t.QuoteCurrency, t.QuoteCurrency, t.BaseCurrency}
			}
			kind := "instrument"
			if table == "fx_quotes" {
				kind = "fx"
			}
			where += " AND NOT EXISTS(SELECT 1 FROM agent_quote_records withdrawn WHERE withdrawn.target_type='" + kind + "' AND withdrawn.target_quote_id=q.id)"
			if query.SourceKind != nil {
				where += " AND q.source_kind=?"
				args = append(args, string(*query.SourceKind))
			}
			baseWhere, baseArgs := where, append([]any{}, args...)
			if query.From != nil {
				where += " AND ((COALESCE(q.effective_date,'')<>'' AND q.effective_date>=?) OR (COALESCE(q.effective_date,'')='' AND q.quoted_at>=?))"
				args = append(args, query.FromDate, formatTimestamp(*query.From))
			}
			if query.To != nil {
				where += " AND ((COALESCE(q.effective_date,'')<>'' AND q.effective_date<=?) OR (COALESCE(q.effective_date,'')='' AND q.quoted_at<=?))"
				args = append(args, query.ToDate, formatTimestamp(*query.To))
			}
			if err := checkReadTableBudget(ctx, tx, table+" q", where, args...); err != nil {
				return out, err
			}
			columns := marketInstrumentQuoteColumns
			if table == "fx_quotes" {
				columns = marketFXQuoteColumns
			}
			rows, err := tx.QueryContext(ctx, "SELECT "+columns+" FROM "+table+" q WHERE "+where+" ORDER BY q.quoted_at DESC,q.created_at DESC,q.id DESC LIMIT 20001", args...)
			if err != nil {
				return out, err
			}
			n := 0
			if table == "instrument_quotes" {
				values, e := scanInstrumentQuotes(rows)
				err = e
				out.InstrumentHistory[t.Key()] = values
				n = len(values)
			} else {
				values, e := scanFXQuotes(rows)
				err = e
				out.FXHistory[t.Key()] = values
				n = len(values)
			}
			rows.Close()
			if err != nil {
				return out, err
			}
			if n > 20000 {
				return out, &domain.Error{Code: "too_large", Message: "capture exceeds market history row budget"}
			}
			if n == 0 && in.Range != "all" {
				var exists bool
				if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" q WHERE "+baseWhere+")", baseArgs...).Scan(&exists); err != nil {
					return out, err
				}
				out.OutsideRange[t.Key()] = exists
			}
		}
	}
	return out, tx.Commit()
}

const marketInstrumentQuoteColumns = `q.id,q.instrument_id,q.unit_price,q.currency,q.source_kind,q.source_key,q.quoted_at,q.created_at,q.delayed,q.observation_kind,q.effective_date,q.provider_timestamp,q.fetched_at,q.value_effective_at,q.binding_revision,q.source_policy_version,q.price_basis,q.timestamp_basis,q.revision,q.supersedes_quote_id,q.split_factor,q.dividend_cash,q.conversion_json`
const marketFXQuoteColumns = `q.id,q.household_id,q.base_currency,q.quote_currency,q.rate,q.source_kind,q.source_key,q.quoted_at,q.created_at,q.delayed,q.observation_kind,q.effective_date,q.fetched_at,q.value_effective_at,q.source_policy_version,q.timestamp_basis,q.revision,q.supersedes_quote_id`
