package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) ListInstrumentHistoryCoverage(ctx context.Context, householdID domain.HouseholdID) ([]domain.InstrumentHistoryCoverage, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `
		SELECT i.id,
		       COALESCE(NULLIF(b.provider_key, ''), i.provider_key, ''),
		       COALESCE(NULLIF(b.provider_symbol, ''), i.provider_symbol, ''),
		       COALESCE(NULLIF(b.market, ''), i.market_code, ''),
		       i.quote_currency,
		       COALESCE(b.binding_revision, 0)
		FROM instruments i
		LEFT JOIN instrument_provider_bindings b ON b.instrument_id = i.id
		WHERE i.household_id = ?
		  AND i.quote_source = 'provider'
		ORDER BY i.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	seeds := make([]domain.InstrumentHistoryCoverage, 0)
	for rows.Next() {
		var item domain.InstrumentHistoryCoverage
		var id, key, symbol, market, currency string
		if err := rows.Scan(&id, &key, &symbol, &market, &currency, &item.BindingRevision); err != nil {
			_ = rows.Close()
			return nil, err
		}
		parsed, parseErr := domain.ParseInstrumentID(id)
		if parseErr != nil {
			_ = rows.Close()
			return nil, parseErr
		}
		item.InstrumentID = parsed
		item.ProviderKey = strings.TrimSpace(key)
		item.ProviderSymbol = strings.TrimSpace(symbol)
		item.Market = strings.TrimSpace(market)
		item.QuoteCurrency = domain.CurrencyCode(currency)
		seeds = append(seeds, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]domain.InstrumentHistoryCoverage, 0, len(seeds))
	for _, item := range seeds {
		closes, fetchedAt, closeErr := listCloseMarketDates(ctx, r.database.SQL, item.InstrumentID.String())
		if closeErr != nil {
			return nil, closeErr
		}
		item.CloseMarketDates = closes
		item.CloseFetchedAt = fetchedAt
		noObs, expires, checked, coverageErr := listNoObservationCoverage(ctx, r.database.SQL, item.InstrumentID.String())
		if coverageErr != nil {
			return nil, coverageErr
		}
		item.NoObservationDates = noObs
		item.NoObservationExpiresAt = expires
		item.NoObservationCheckedAt = checked
		result = append(result, item)
	}
	return result, nil
}

func listCloseMarketDates(ctx context.Context, query queryer, instrumentID string) ([]string, map[string]time.Time, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT s.market_date, q.fetched_at FROM instrument_observation_slots s
		JOIN instrument_quotes q ON q.id = s.quote_id
		WHERE s.instrument_id = ? AND s.observation_kind = ?
		ORDER BY s.market_date`, instrumentID, observationKindClose)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	dates := make([]string, 0)
	fetched := map[string]time.Time{}
	for rows.Next() {
		var date string
		var fetchedAt sql.NullString
		if err := rows.Scan(&date, &fetchedAt); err != nil {
			return nil, nil, err
		}
		dates = append(dates, date)
		if parsed, parseErr := parseTimePtr(fetchedAt); parseErr != nil {
			return nil, nil, parseErr
		} else if parsed != nil {
			fetched[date] = parsed.UTC()
		}
	}
	return dates, fetched, rows.Err()
}

func listNoObservationCoverage(ctx context.Context, query queryer, instrumentID string) ([]string, map[string]time.Time, map[string]time.Time, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT effective_date, expires_at, checked_at FROM market_data_day_status
		WHERE target_type = 'instrument' AND target_id = ? AND status = ?
		ORDER BY effective_date`, instrumentID, coverageStatusNoObservation)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	dates := make([]string, 0)
	expires := map[string]time.Time{}
	checked := map[string]time.Time{}
	for rows.Next() {
		var date string
		var expiresAt, checkedAt sql.NullString
		if err := rows.Scan(&date, &expiresAt, &checkedAt); err != nil {
			return nil, nil, nil, err
		}
		dates = append(dates, date)
		if parsed, parseErr := parseTimePtr(expiresAt); parseErr != nil {
			return nil, nil, nil, parseErr
		} else if parsed != nil {
			expires[date] = parsed.UTC()
		}
		if parsed, parseErr := parseTimePtr(checkedAt); parseErr != nil {
			return nil, nil, nil, parseErr
		} else if parsed != nil {
			checked[date] = parsed.UTC()
		}
	}
	return dates, expires, checked, rows.Err()
}
