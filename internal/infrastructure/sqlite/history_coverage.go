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
		       COALESCE(b.binding_revision, 0),
		       COALESCE((SELECT q.source_policy_version
		                 FROM instrument_observation_slots s
		                 JOIN instrument_quotes q ON q.id = s.quote_id
		                 WHERE s.instrument_id = i.id
		                   AND s.provider_key = COALESCE(NULLIF(b.provider_key, ''), i.provider_key, '')
		                   AND s.binding_revision = COALESCE(b.binding_revision, 0)
		                   AND s.observation_kind = 'close'
		                 ORDER BY q.fetched_at DESC LIMIT 1), '')
		FROM instruments i
		LEFT JOIN instrument_provider_bindings b ON b.instrument_id = i.id
		  AND b.provider_key = i.provider_key
		  AND b.enabled = 1
		WHERE i.household_id = ?
		  AND i.quote_source = 'provider'
		ORDER BY i.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	seeds := make([]domain.InstrumentHistoryCoverage, 0)
	for rows.Next() {
		var item domain.InstrumentHistoryCoverage
		var id, key, symbol, market, currency, policy string
		if err := rows.Scan(&id, &key, &symbol, &market, &currency, &item.BindingRevision, &policy); err != nil {
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
		item.SourcePolicyVersion = instrumentHistorySourcePolicy(item.ProviderKey, policy)
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
		closes, fetchedAt, closeErr := listCloseMarketDates(ctx, r.database.SQL, item.InstrumentID.String(), item.ProviderKey, item.BindingRevision, item.SourcePolicyVersion)
		if closeErr != nil {
			return nil, closeErr
		}
		item.CloseMarketDates = closes
		item.CloseFetchedAt = fetchedAt
		noObs, expires, checked, coverageErr := listNoObservationCoverage(ctx, r.database.SQL, item.InstrumentID.String(), householdID.String(), item.ProviderKey, item.BindingRevision, item.SourcePolicyVersion)
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

func instrumentHistorySourcePolicy(providerKey, observedPolicy string) string {
	switch strings.ToLower(strings.TrimSpace(providerKey)) {
	case domain.TiingoProviderKey:
		return domain.TiingoRawClosePriceBasis
	case domain.YahooFinanceProviderKey:
		return domain.YahooRawClosePriceBasis
	default:
		return strings.TrimSpace(observedPolicy)
	}
}

func listCloseMarketDates(ctx context.Context, query queryer, instrumentID, providerKey string, bindingRevision int, policy string) ([]string, map[string]time.Time, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT s.market_date, q.fetched_at FROM instrument_observation_slots s
		JOIN instrument_quotes q ON q.id = s.quote_id
		WHERE s.instrument_id = ? AND s.provider_key = ? AND s.binding_revision = ? AND s.source_policy_version = ? AND s.observation_kind = ?
		ORDER BY s.market_date`, instrumentID, providerKey, bindingRevision, policy, observationKindClose)
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

func (r *Repository) ListFXHistoryCoverage(ctx context.Context, householdID domain.HouseholdID) ([]domain.FXHistoryCoverage, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `
		SELECT s.base_currency, s.quote_currency, s.provider_key, s.source_policy_version, s.market_date, q.fetched_at
		FROM fx_observation_slots s
		JOIN fx_quotes q ON q.id = s.quote_id
		WHERE s.household_id = ? AND s.observation_kind = ?
		ORDER BY s.base_currency, s.quote_currency, s.provider_key, s.source_policy_version, s.market_date`, householdID.String(), observationKindDailyReference)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*domain.FXHistoryCoverage{}
	order := make([]string, 0)
	for rows.Next() {
		var base, quote, provider, policy, marketDate string
		var fetchedAt sql.NullString
		if err := rows.Scan(&base, &quote, &provider, &policy, &marketDate, &fetchedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		key := fxCoverageKey(base, quote, provider, policy)
		item, ok := byKey[key]
		if !ok {
			parsedBase, baseErr := domain.ParseSupportedCurrency(base)
			if baseErr != nil {
				_ = rows.Close()
				return nil, baseErr
			}
			parsedQuote, quoteErr := domain.ParseSupportedCurrency(quote)
			if quoteErr != nil {
				_ = rows.Close()
				return nil, quoteErr
			}
			item = &domain.FXHistoryCoverage{BaseCurrency: parsedBase, QuoteCurrency: parsedQuote, ProviderKey: strings.TrimSpace(provider), SourcePolicyVersion: strings.TrimSpace(policy), DailyReferenceFetchedAt: map[string]time.Time{}, NoObservationExpiresAt: map[string]time.Time{}, NoObservationCheckedAt: map[string]time.Time{}}
			byKey[key] = item
			order = append(order, key)
		}
		item.DailyReferenceDates = append(item.DailyReferenceDates, marketDate)
		if parsed, parseErr := parseTimePtr(fetchedAt); parseErr != nil {
			_ = rows.Close()
			return nil, parseErr
		} else if parsed != nil {
			item.DailyReferenceFetchedAt[marketDate] = parsed.UTC()
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	noObsRows, err := r.database.SQL.QueryContext(ctx, `
		SELECT target_id, provider_key, source_policy_version, effective_date, expires_at, checked_at
		FROM market_data_day_status
		WHERE household_id = ? AND target_type = 'fx' AND status = ?
		ORDER BY target_id, provider_key, source_policy_version, effective_date`, householdID.String(), coverageStatusNoObservation)
	if err != nil {
		return nil, err
	}
	defer noObsRows.Close()
	for noObsRows.Next() {
		var targetID, provider, policy, date string
		var expiresAt, checkedAt sql.NullString
		if err := noObsRows.Scan(&targetID, &provider, &policy, &date, &expiresAt, &checkedAt); err != nil {
			return nil, err
		}
		key := fxCoverageKeyFromTarget(targetID, provider, policy)
		item, ok := byKey[key]
		if !ok {
			parts := strings.Split(strings.TrimSpace(targetID), "/")
			if len(parts) != 2 {
				continue
			}
			parsedBase, baseErr := domain.ParseSupportedCurrency(parts[0])
			if baseErr != nil {
				return nil, baseErr
			}
			parsedQuote, quoteErr := domain.ParseSupportedCurrency(parts[1])
			if quoteErr != nil {
				return nil, quoteErr
			}
			item = &domain.FXHistoryCoverage{BaseCurrency: parsedBase, QuoteCurrency: parsedQuote, ProviderKey: strings.TrimSpace(provider), SourcePolicyVersion: strings.TrimSpace(policy), DailyReferenceFetchedAt: map[string]time.Time{}, NoObservationExpiresAt: map[string]time.Time{}, NoObservationCheckedAt: map[string]time.Time{}}
			byKey[key] = item
			order = append(order, key)
		}
		item.NoObservationDates = append(item.NoObservationDates, date)
		if parsed, parseErr := parseTimePtr(expiresAt); parseErr != nil {
			return nil, parseErr
		} else if parsed != nil {
			item.NoObservationExpiresAt[date] = parsed.UTC()
		}
		if parsed, parseErr := parseTimePtr(checkedAt); parseErr != nil {
			return nil, parseErr
		} else if parsed != nil {
			item.NoObservationCheckedAt[date] = parsed.UTC()
		}
	}
	if err := noObsRows.Err(); err != nil {
		return nil, err
	}
	result := make([]domain.FXHistoryCoverage, 0, len(order))
	for _, key := range order {
		result = append(result, *byKey[key])
	}
	return result, nil
}

func fxCoverageKey(base, quote, provider, policy string) string {
	return strings.TrimSpace(base) + "/" + strings.TrimSpace(quote) + ":" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(policy)
}

func fxCoverageKeyFromTarget(target, provider, policy string) string {
	return strings.TrimSpace(target) + ":" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(policy)
}

func listNoObservationCoverage(ctx context.Context, query queryer, instrumentID, householdID, providerKey string, bindingRevision int, policy string) ([]string, map[string]time.Time, map[string]time.Time, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT effective_date, expires_at, checked_at FROM market_data_day_status
		WHERE target_type = 'instrument' AND target_id = ? AND household_id = ? AND provider_key = ? AND binding_revision = ? AND source_policy_version = ? AND status = ?
		ORDER BY effective_date`, instrumentID, householdID, providerKey, bindingRevision, policy, coverageStatusNoObservation)
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
