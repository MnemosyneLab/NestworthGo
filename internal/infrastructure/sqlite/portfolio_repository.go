package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadPortfolioSnapshot loads every current portfolio input through one
// read-only transaction. The number of queries is fixed by entity type and
// never grows with the number of holdings or quotes.
func (r *Repository) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	fail := func(cause error) (domain.PortfolioSnapshot, error) {
		_ = tx.Rollback()
		return domain.PortfolioSnapshot{}, cause
	}
	household, err := scanHousehold(tx.QueryRowContext(ctx, `SELECT id, name, base_currency, created_at, updated_at FROM households WHERE singleton_key = 1`))
	if err != nil {
		return fail(err)
	}
	if household == nil {
		if err := tx.Commit(); err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		return domain.PortfolioSnapshot{}, nil
	}
	members, err := listMembersQuery(ctx, tx, true)
	if err != nil {
		return fail(err)
	}
	institutions, err := listInstitutionsQuery(ctx, tx, true)
	if err != nil {
		return fail(err)
	}
	groups, err := listGroupsQuery(ctx, tx, true)
	if err != nil {
		return fail(err)
	}
	accounts, err := listAccountRecords(ctx, tx, household.ID, filter)
	if err != nil {
		return fail(err)
	}
	instruments, err := listInstrumentsQuery(ctx, tx, household.ID, true)
	if err != nil {
		return fail(err)
	}
	holdings, err := listHoldingsQuery(ctx, tx, household.ID, true)
	if err != nil {
		return fail(err)
	}
	cashValues, err := listCashValuesQuery(ctx, tx, household.ID)
	if err != nil {
		return fail(err)
	}
	instrumentQuotes, err := listInstrumentQuotesQuery(ctx, tx, household.ID)
	if err != nil {
		return fail(err)
	}
	fxQuotes, err := listFXQuotesQuery(ctx, tx, household.ID)
	if err != nil {
		return fail(err)
	}
	fxPreferences, err := listFXPreferencesQuery(ctx, tx, household.ID)
	if err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	return domain.PortfolioSnapshot{
		Household: household, Members: members, Institutions: institutions, Groups: groups,
		Accounts: accounts, Instruments: instruments, Holdings: holdings, CashValues: cashValues,
		InstrumentQuotes: instrumentQuotes, FXQuotes: fxQuotes, FXPreferences: fxPreferences,
	}, nil
}

func (r *Repository) CreateInstrument(ctx context.Context, instrument domain.Instrument) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, instrument.HouseholdID); err != nil {
			return err
		}
		if err := validateInstrumentBinding(instrument); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO instruments(id, household_id, name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, logo_asset_id, sort_order, quote_source, provider_key, provider_symbol, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, instrument.ID.String(), instrument.HouseholdID.String(), instrument.Name, string(instrument.Type), instrument.QuoteCurrency.String(), nullableString(instrument.Symbol), nullableString(instrument.MarketCode), nullableString(instrument.CountryCode), nullableString(instrument.ISIN), nullableString(instrument.Note), nullableID(instrument.LogoAssetID), instrument.SortOrder, string(instrument.QuoteSource), nullableString(instrument.ProviderKey), nullableString(instrument.ProviderSymbol), formatTimestamp(instrument.CreatedAt), formatTimestamp(instrument.UpdatedAt), nullableTime(instrument.ArchivedAt))
		return mapPortfolioWriteError(err, "instrument")
	})
}

func (r *Repository) UpdateInstrument(ctx context.Context, instrument domain.Instrument) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateInstrumentBinding(instrument); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET name = ?, instrument_type = ?, quote_currency = ?, symbol = ?, market_code = ?, country_code = ?, isin = ?, note = ?, logo_asset_id = ?, sort_order = ?, quote_source = ?, provider_key = ?, provider_symbol = ?, updated_at = ? WHERE id = ? AND household_id = ?`, instrument.Name, string(instrument.Type), instrument.QuoteCurrency.String(), nullableString(instrument.Symbol), nullableString(instrument.MarketCode), nullableString(instrument.CountryCode), nullableString(instrument.ISIN), nullableString(instrument.Note), nullableID(instrument.LogoAssetID), instrument.SortOrder, string(instrument.QuoteSource), nullableString(instrument.ProviderKey), nullableString(instrument.ProviderSymbol), formatTimestamp(instrument.UpdatedAt), instrument.ID.String(), instrument.HouseholdID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		return requireAffected(result, "instrument")
	})
}

func (r *Repository) Instrument(ctx context.Context, householdID domain.HouseholdID, id domain.InstrumentID) (domain.Instrument, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT id, household_id, name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, logo_asset_id, sort_order, quote_source, provider_key, provider_symbol, created_at, updated_at, archived_at FROM instruments WHERE id = ? AND household_id = ?`, id.String(), householdID.String())
	return scanInstrument(row)
}

func (r *Repository) ListInstruments(ctx context.Context, householdID domain.HouseholdID, includeArchived bool) ([]domain.Instrument, error) {
	return listInstrumentsQuery(ctx, r.database.SQL, householdID, includeArchived)
}

func (r *Repository) SetInstrumentArchive(ctx context.Context, householdID domain.HouseholdID, id domain.InstrumentID, archived bool, now time.Time) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, householdID); err != nil {
			return err
		}
		if !archived {
			var providerKey, providerSymbol sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT provider_key, provider_symbol FROM instruments WHERE id = ? AND household_id = ?`, id.String(), householdID.String()).Scan(&providerKey, &providerSymbol); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
				}
				return err
			}
			if providerKey.Valid && providerSymbol.Valid {
				var conflictID string
				if err := tx.QueryRowContext(ctx, `SELECT id FROM instruments WHERE household_id = ? AND provider_key = ? AND provider_symbol = ? AND archived_at IS NULL AND id <> ? LIMIT 1`, householdID.String(), providerKey.String, providerSymbol.String, id.String()).Scan(&conflictID); err == nil {
					return &domain.Error{Code: domain.ErrConflict, Message: "an active instrument already uses this provider binding"}
				} else if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET archived_at = ?, updated_at = ? WHERE id = ? AND household_id = ?`, archiveValue(archived, now), formatTimestamp(now), id.String(), householdID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		return requireAffected(result, "instrument")
	})
}

func (r *Repository) SetInstrumentLogo(ctx context.Context, householdID domain.HouseholdID, id domain.InstrumentID, assetID domain.MediaAssetID) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE instruments SET logo_asset_id = ?, updated_at = ? WHERE id = ? AND household_id = ?`, assetID.String(), formatTimestamp(time.Now()), id.String(), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "instrument")
}

func (r *Repository) SetInstrumentQuoteSource(ctx context.Context, householdID domain.HouseholdID, id domain.InstrumentID, source domain.QuoteSourceKind) error {
	parsedSource, err := domain.ParseQuoteSourceKind(string(source))
	if err != nil {
		return err
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var providerKey, providerSymbol sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT provider_key, provider_symbol FROM instruments WHERE id = ? AND household_id = ? AND archived_at IS NULL`, id.String(), householdID.String()).Scan(&providerKey, &providerSymbol); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
			}
			return err
		}
		if parsedSource == domain.QuoteSourceProvider && (!providerKey.Valid || strings.TrimSpace(providerKey.String) == "" || !providerSymbol.Valid || strings.TrimSpace(providerSymbol.String) == "") {
			return &domain.Error{Code: domain.ErrValidation, Field: "provider", Message: "provider key and symbol are required when Provider is selected"}
		}
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET quote_source = ?, updated_at = ? WHERE id = ? AND household_id = ? AND archived_at IS NULL`, string(parsedSource), formatTimestamp(time.Now()), id.String(), householdID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		return requireAffected(result, "instrument")
	})
}

func (r *Repository) CreateHolding(ctx context.Context, holding domain.Holding) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateHoldingReferences(ctx, tx, holding.AccountID, holding.InstrumentID, nil); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO holdings(id, account_id, instrument_id, quantity, note, sort_order, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, holding.ID.String(), holding.AccountID.String(), holding.InstrumentID.String(), holding.Quantity.Canonical(), nullableString(holding.Note), holding.SortOrder, formatTimestamp(holding.CreatedAt), formatTimestamp(holding.UpdatedAt), nullableTime(holding.ArchivedAt))
		return mapPortfolioWriteError(err, "holding")
	})
}

func (r *Repository) UpdateHolding(ctx context.Context, holding domain.Holding) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE holdings SET quantity = ?, note = ?, sort_order = ?, updated_at = ? WHERE id = ?`, holding.Quantity.Canonical(), nullableString(holding.Note), holding.SortOrder, formatTimestamp(holding.UpdatedAt), holding.ID.String())
	if err != nil {
		return mapPortfolioWriteError(err, "holding")
	}
	return requireAffected(result, "holding")
}

func (r *Repository) Holding(ctx context.Context, id domain.HoldingID) (domain.Holding, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT id, account_id, instrument_id, quantity, note, sort_order, created_at, updated_at, archived_at FROM holdings WHERE id = ?`, id.String())
	return scanHolding(row)
}

func (r *Repository) ListHoldings(ctx context.Context, accountID domain.AccountID, includeArchived bool) ([]domain.Holding, error) {
	query := `SELECT id, account_id, instrument_id, quantity, note, sort_order, created_at, updated_at, archived_at FROM holdings WHERE account_id = ?`
	if !includeArchived {
		query += ` AND archived_at IS NULL`
	}
	query += ` ORDER BY sort_order ASC, instrument_id ASC, id ASC`
	rows, err := r.database.SQL.QueryContext(ctx, query, accountID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHoldings(rows)
}

func (r *Repository) SetHoldingArchive(ctx context.Context, householdID domain.HouseholdID, id domain.HoldingID, archived bool, now time.Time) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var accountID, instrumentID string
		if err := tx.QueryRowContext(ctx, `SELECT h.account_id, h.instrument_id FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE h.id = ? AND a.household_id = ?`, id.String(), householdID.String()).Scan(&accountID, &instrumentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
			}
			return err
		}
		if !archived {
			var conflictID string
			if err := tx.QueryRowContext(ctx, `SELECT id FROM holdings WHERE account_id = ? AND instrument_id = ? AND archived_at IS NULL AND id <> ? LIMIT 1`, accountID, instrumentID, id.String()).Scan(&conflictID); err == nil {
				return &domain.Error{Code: domain.ErrConflict, Message: "an active holding already uses this instrument in the account"}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE holdings SET archived_at = ?, updated_at = ? WHERE id = ?`, archiveValue(archived, now), formatTimestamp(now), id.String())
		if err != nil {
			return mapPortfolioWriteError(err, "holding")
		}
		return requireAffected(result, "holding")
	})
}

func (r *Repository) AppendAccountCashValue(ctx context.Context, value domain.AccountCashValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var mode string
		if err := tx.QueryRowContext(ctx, `SELECT tracking_mode FROM accounts WHERE id = ?`, value.AccountID.String()).Scan(&mode); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
			}
			return err
		}
		if mode != string(domain.TrackingHoldings) {
			return &domain.Error{Code: domain.ErrValidation, Field: "trackingMode", Message: "cash observations require a Holdings account"}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at) VALUES(?, ?, ?, ?, ?, ?)`, value.ID.String(), value.AccountID.String(), value.Amount.CanonicalAmount(), value.Amount.Currency().String(), formatTimestamp(value.EffectiveAt), formatTimestamp(value.CreatedAt))
		return mapPortfolioWriteError(err, "cash value")
	})
}

func (r *Repository) ListAccountCashValues(ctx context.Context, accountID domain.AccountID) ([]domain.AccountCashValue, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `SELECT id, account_id, amount, currency, effective_at, created_at FROM account_cash_values WHERE account_id = ? ORDER BY currency ASC, effective_at DESC, created_at DESC, id DESC`, accountID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCashValues(rows)
}

func (r *Repository) AppendInstrumentQuote(ctx context.Context, quote domain.InstrumentQuote) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return appendInstrumentQuoteTx(ctx, tx, quote)
	})
}

func (r *Repository) AppendProviderInstrumentQuoteIfChanged(ctx context.Context, quote domain.InstrumentQuote) (bool, error) {
	inserted := false
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var instrumentCurrency string
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT quote_currency, archived_at FROM instruments WHERE id = ?`, quote.InstrumentID.String()).Scan(&instrumentCurrency, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
			}
			return err
		}
		if archived.Valid {
			return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
		}
		if instrumentCurrency != quote.Currency.String() {
			return &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "quote currency does not match instrument"}
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM instrument_quotes
				WHERE instrument_id = ? AND unit_price = ? AND currency = ? AND source_kind = ? AND source_key = ? AND quoted_at = ? AND delayed = ?
			)`,
			quote.ID.String(), quote.InstrumentID.String(), quote.UnitPrice.Canonical(), quote.Currency.String(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed),
			quote.InstrumentID.String(), quote.UnitPrice.Canonical(), quote.Currency.String(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), boolValue(quote.Delayed))
		if err != nil {
			return mapPortfolioWriteError(err, "instrument quote")
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		inserted = count == 1
		return nil
	})
	return inserted, err
}

func (r *Repository) AppendInstrumentQuoteAndSelectManual(ctx context.Context, quote domain.InstrumentQuote) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := appendInstrumentQuoteTx(ctx, tx, quote); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET quote_source = 'manual', updated_at = ? WHERE id = ? AND archived_at IS NULL`, formatTimestamp(quote.CreatedAt), quote.InstrumentID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		return requireAffected(result, "instrument")
	})
}

func (r *Repository) ListInstrumentQuotes(ctx context.Context, instrumentID domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `SELECT id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed FROM instrument_quotes WHERE instrument_id = ? ORDER BY quoted_at DESC, created_at DESC, id DESC`, instrumentID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstrumentQuotes(rows)
}

func (r *Repository) AppendFXQuote(ctx context.Context, quote domain.FXQuote) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return appendFXQuoteTx(ctx, tx, quote)
	})
}

func (r *Repository) AppendProviderFXQuoteIfChanged(ctx context.Context, quote domain.FXQuote) (bool, error) {
	inserted := false
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, quote.HouseholdID); err != nil {
			return err
		}
		if quote.BaseCurrency == quote.QuoteCurrency {
			return &domain.Error{Code: domain.ErrValidation, Field: "currencyPair", Message: "currencies must differ"}
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM fx_quotes
				WHERE household_id = ? AND base_currency = ? AND quote_currency = ? AND rate = ? AND source_kind = ? AND source_key = ? AND quoted_at = ? AND delayed = ?
			)`,
			quote.ID.String(), quote.HouseholdID.String(), quote.BaseCurrency.String(), quote.QuoteCurrency.String(), quote.Rate.Canonical(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed),
			quote.HouseholdID.String(), quote.BaseCurrency.String(), quote.QuoteCurrency.String(), quote.Rate.Canonical(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), boolValue(quote.Delayed))
		if err != nil {
			return mapPortfolioWriteError(err, "FX quote")
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		inserted = count == 1
		return nil
	})
	return inserted, err
}

func (r *Repository) AppendFXQuoteAndSelectManual(ctx context.Context, quote domain.FXQuote) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := appendFXQuoteTx(ctx, tx, quote); err != nil {
			return err
		}
		a, b, err := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fx_preferences(household_id, currency_a, currency_b, source_kind, created_at, updated_at) VALUES(?, ?, ?, 'manual', ?, ?) ON CONFLICT(household_id, currency_a, currency_b) DO UPDATE SET source_kind = 'manual', updated_at = excluded.updated_at`, quote.HouseholdID.String(), a.String(), b.String(), formatTimestamp(quote.CreatedAt), formatTimestamp(quote.CreatedAt))
		return mapPortfolioWriteError(err, "FX preference")
	})
}

func (r *Repository) ListFXQuotes(ctx context.Context, householdID domain.HouseholdID) ([]domain.FXQuote, error) {
	return listFXQuotesQuery(ctx, r.database.SQL, householdID)
}

func (r *Repository) SetFXPreference(ctx context.Context, preference domain.FXPreference) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, preference.HouseholdID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO fx_preferences(household_id, currency_a, currency_b, source_kind, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(household_id, currency_a, currency_b) DO UPDATE SET source_kind = excluded.source_kind, updated_at = excluded.updated_at`, preference.HouseholdID.String(), preference.CurrencyA.String(), preference.CurrencyB.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt), formatTimestamp(preference.UpdatedAt))
		return mapPortfolioWriteError(err, "FX preference")
	})
}

func (r *Repository) FXPreference(ctx context.Context, householdID domain.HouseholdID, currencyA, currencyB domain.CurrencyCode) (domain.FXPreference, error) {
	a, b, err := domain.NormalizeFXPair(currencyA, currencyB)
	if err != nil {
		return domain.FXPreference{}, err
	}
	row := r.database.SQL.QueryRowContext(ctx, `SELECT household_id, currency_a, currency_b, source_kind, created_at, updated_at FROM fx_preferences WHERE household_id = ? AND currency_a = ? AND currency_b = ?`, householdID.String(), a.String(), b.String())
	return scanFXPreference(row)
}

func (r *Repository) ListFXPreferences(ctx context.Context, householdID domain.HouseholdID) ([]domain.FXPreference, error) {
	return listFXPreferencesQuery(ctx, r.database.SQL, householdID)
}

func ensureHousehold(ctx context.Context, query queryer, id domain.HouseholdID) error {
	var found string
	if err := query.QueryRowContext(ctx, `SELECT id FROM households WHERE id = ?`, id.String()).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
		}
		return err
	}
	return nil
}

func validateInstrumentBinding(instrument domain.Instrument) error {
	if instrument.QuoteSource == domain.QuoteSourceProvider && (instrument.ProviderKey == nil || instrument.ProviderSymbol == nil || strings.TrimSpace(*instrument.ProviderKey) == "" || strings.TrimSpace(*instrument.ProviderSymbol) == "") {
		return &domain.Error{Code: domain.ErrValidation, Field: "provider", Message: "provider key and symbol are required when Provider is selected"}
	}
	return nil
}

func validateHoldingReferences(ctx context.Context, tx *sql.Tx, accountID domain.AccountID, instrumentID domain.InstrumentID, retainedHolding *domain.HoldingID) error {
	var accountHousehold, tracking string
	var accountArchived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT household_id, tracking_mode, archived_at FROM accounts WHERE id = ?`, accountID.String()).Scan(&accountHousehold, &tracking, &accountArchived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
		}
		return err
	}
	if accountArchived.Valid {
		return &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "account is archived"}
	}
	if tracking != string(domain.TrackingHoldings) {
		return &domain.Error{Code: domain.ErrValidation, Field: "trackingMode", Message: "holding requires a Holdings account"}
	}
	var instrumentHousehold string
	var instrumentArchived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT household_id, archived_at FROM instruments WHERE id = ?`, instrumentID.String()).Scan(&instrumentHousehold, &instrumentArchived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
		}
		return err
	}
	if instrumentHousehold != accountHousehold {
		return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "account and instrument belong to different households"}
	}
	if instrumentArchived.Valid {
		return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
	}
	var duplicate string
	statement := `SELECT id FROM holdings WHERE account_id = ? AND instrument_id = ? AND archived_at IS NULL`
	args := []any{accountID.String(), instrumentID.String()}
	if retainedHolding != nil {
		statement += ` AND id <> ?`
		args = append(args, retainedHolding.String())
	}
	if err := tx.QueryRowContext(ctx, statement+` LIMIT 1`, args...).Scan(&duplicate); err == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "an active holding already uses this instrument in the account"}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func mapPortfolioWriteError(err error, entity string) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") {
		return &domain.Error{Code: domain.ErrConflict, Message: entity + " conflicts with an existing active record"}
	}
	if strings.Contains(message, "foreign key constraint") {
		return &domain.Error{Code: domain.ErrValidation, Field: entity, Message: "referenced object is invalid"}
	}
	return err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTimestamp(*value)
}

func listInstrumentsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, includeArchived bool) ([]domain.Instrument, error) {
	statement := `SELECT id, household_id, name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, logo_asset_id, sort_order, quote_source, provider_key, provider_symbol, created_at, updated_at, archived_at FROM instruments WHERE household_id = ?`
	if !includeArchived {
		statement += ` AND archived_at IS NULL`
	}
	statement += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := query.QueryContext(ctx, statement, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Instrument
	for rows.Next() {
		instrument, err := scanInstrument(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, instrument)
	}
	return result, rows.Err()
}

func listHoldingsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, includeArchived bool) ([]domain.Holding, error) {
	statement := `SELECT h.id, h.account_id, h.instrument_id, h.quantity, h.note, h.sort_order, h.created_at, h.updated_at, h.archived_at FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE a.household_id = ?`
	if !includeArchived {
		statement += ` AND h.archived_at IS NULL`
	}
	statement += ` ORDER BY h.account_id ASC, h.sort_order ASC, h.instrument_id ASC, h.id ASC`
	rows, err := query.QueryContext(ctx, statement, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHoldings(rows)
}

func listCashValuesQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.AccountCashValue, error) {
	rows, err := query.QueryContext(ctx, `SELECT c.id, c.account_id, c.amount, c.currency, c.effective_at, c.created_at FROM account_cash_values c JOIN accounts a ON a.id = c.account_id WHERE a.household_id = ? ORDER BY c.account_id ASC, c.currency ASC, c.effective_at DESC, c.created_at DESC, c.id DESC`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCashValues(rows)
}

func listInstrumentQuotesQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.InstrumentQuote, error) {
	rows, err := query.QueryContext(ctx, `SELECT q.id, q.instrument_id, q.unit_price, q.currency, q.source_kind, q.source_key, q.quoted_at, q.created_at, q.delayed FROM instrument_quotes q JOIN instruments i ON i.id = q.instrument_id WHERE i.household_id = ? ORDER BY q.instrument_id ASC, q.quoted_at DESC, q.created_at DESC, q.id DESC`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstrumentQuotes(rows)
}

func listFXQuotesQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.FXQuote, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed FROM fx_quotes WHERE household_id = ? ORDER BY base_currency ASC, quote_currency ASC, quoted_at DESC, created_at DESC, id DESC`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFXQuotes(rows)
}

func listFXPreferencesQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.FXPreference, error) {
	rows, err := query.QueryContext(ctx, `SELECT household_id, currency_a, currency_b, source_kind, created_at, updated_at FROM fx_preferences WHERE household_id = ? ORDER BY currency_a ASC, currency_b ASC`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFXPreferences(rows)
}

func scanInstrument(row interface{ Scan(...any) error }) (domain.Instrument, error) {
	var id, householdID, name, instrumentType, quoteCurrency, createdAt, updatedAt string
	var symbol, marketCode, countryCode, isin, note, logo, quoteSource, providerKey, providerSymbol, archived sql.NullString
	var sortOrder int
	if err := row.Scan(&id, &householdID, &name, &instrumentType, &quoteCurrency, &symbol, &marketCode, &countryCode, &isin, &note, &logo, &sortOrder, &quoteSource, &providerKey, &providerSymbol, &createdAt, &updatedAt, &archived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instrument{}, &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
		}
		return domain.Instrument{}, err
	}
	instrumentID, err := domain.ParseInstrumentID(id)
	if err != nil {
		return domain.Instrument{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.Instrument{}, err
	}
	parsedType, err := domain.ParseInstrumentType(instrumentType)
	if err != nil {
		return domain.Instrument{}, err
	}
	currency, err := domain.ParseCurrency(quoteCurrency)
	if err != nil {
		return domain.Instrument{}, err
	}
	source, err := domain.ParseQuoteSourceKind(quoteSource.String)
	if err != nil {
		return domain.Instrument{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.Instrument{}, err
	}
	updated, err := parsePortfolioTime(updatedAt)
	if err != nil {
		return domain.Instrument{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.Instrument{}, err
	}
	return domain.Instrument{ID: instrumentID, HouseholdID: hID, Name: name, Type: parsedType, QuoteCurrency: currency, Symbol: parseNullable(nullString(symbol)), MarketCode: parseNullable(nullString(marketCode)), CountryCode: parseNullable(nullString(countryCode)), ISIN: parseNullable(nullString(isin)), Note: parseNullable(nullString(note)), LogoAssetID: parseMediaID(nullString(logo)), SortOrder: sortOrder, QuoteSource: source, ProviderKey: parseNullable(nullString(providerKey)), ProviderSymbol: parseNullable(nullString(providerSymbol)), CreatedAt: created, UpdatedAt: updated, ArchivedAt: archivedAt}, nil
}

func scanHolding(row interface{ Scan(...any) error }) (domain.Holding, error) {
	var id, accountID, instrumentID, quantity, createdAt, updatedAt string
	var note, archived sql.NullString
	var sortOrder int
	if err := row.Scan(&id, &accountID, &instrumentID, &quantity, &note, &sortOrder, &createdAt, &updatedAt, &archived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Holding{}, &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
		}
		return domain.Holding{}, err
	}
	holdingID, err := domain.ParseHoldingID(id)
	if err != nil {
		return domain.Holding{}, err
	}
	account, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.Holding{}, err
	}
	instrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.Holding{}, err
	}
	parsedQuantity, err := domain.ParseQuantity(quantity)
	if err != nil {
		return domain.Holding{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.Holding{}, err
	}
	updated, err := parsePortfolioTime(updatedAt)
	if err != nil {
		return domain.Holding{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.Holding{}, err
	}
	return domain.Holding{ID: holdingID, AccountID: account, InstrumentID: instrument, Quantity: parsedQuantity, Note: parseNullable(nullString(note)), SortOrder: sortOrder, CreatedAt: created, UpdatedAt: updated, ArchivedAt: archivedAt}, nil
}

func scanHoldings(rows *sql.Rows) ([]domain.Holding, error) {
	var result []domain.Holding
	for rows.Next() {
		holding, err := scanHolding(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, holding)
	}
	return result, rows.Err()
}

func scanCashValue(row interface{ Scan(...any) error }) (domain.AccountCashValue, error) {
	var id, accountID, amount, currency, effectiveAt, createdAt string
	if err := row.Scan(&id, &accountID, &amount, &currency, &effectiveAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrNotFound, Message: "cash value was not found"}
		}
		return domain.AccountCashValue{}, err
	}
	cashID, err := domain.ParseAccountCashValueID(id)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	parsedAccount, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	parsedCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	money, err := domain.ParseMoney(amount, parsedCurrency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	effective, err := parsePortfolioTime(effectiveAt)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	return domain.AccountCashValue{ID: cashID, AccountID: parsedAccount, Amount: money, EffectiveAt: effective, CreatedAt: created}, nil
}

func scanCashValues(rows *sql.Rows) ([]domain.AccountCashValue, error) {
	var result []domain.AccountCashValue
	for rows.Next() {
		value, err := scanCashValue(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanInstrumentQuote(row interface{ Scan(...any) error }) (domain.InstrumentQuote, error) {
	var id, instrumentID, unitPrice, currency, sourceKind, sourceKey, quotedAt, createdAt string
	var delayed int
	if err := row.Scan(&id, &instrumentID, &unitPrice, &currency, &sourceKind, &sourceKey, &quotedAt, &createdAt, &delayed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.InstrumentQuote{}, &domain.Error{Code: domain.ErrNotFound, Message: "instrument quote was not found"}
		}
		return domain.InstrumentQuote{}, err
	}
	quoteID, err := domain.ParseInstrumentQuoteID(id)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	price, err := domain.ParseUnitPrice(unitPrice)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	parsedCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	quoted, err := parsePortfolioTime(quotedAt)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	return domain.InstrumentQuote{ID: quoteID, InstrumentID: parsedInstrument, UnitPrice: price, Currency: parsedCurrency, SourceKind: parsedSource, SourceKey: sourceKey, QuotedAt: quoted, CreatedAt: created, Delayed: delayed != 0}, nil
}

func scanInstrumentQuotes(rows *sql.Rows) ([]domain.InstrumentQuote, error) {
	var result []domain.InstrumentQuote
	for rows.Next() {
		quote, err := scanInstrumentQuote(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, quote)
	}
	return result, rows.Err()
}

func scanFXQuote(row interface{ Scan(...any) error }) (domain.FXQuote, error) {
	var id, householdID, baseCurrency, quoteCurrency, rate, sourceKind, sourceKey, quotedAt, createdAt string
	var delayed int
	if err := row.Scan(&id, &householdID, &baseCurrency, &quoteCurrency, &rate, &sourceKind, &sourceKey, &quotedAt, &createdAt, &delayed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.FXQuote{}, &domain.Error{Code: domain.ErrNotFound, Message: "FX quote was not found"}
		}
		return domain.FXQuote{}, err
	}
	quoteID, err := domain.ParseFXQuoteID(id)
	if err != nil {
		return domain.FXQuote{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.FXQuote{}, err
	}
	base, err := domain.ParseCurrency(baseCurrency)
	if err != nil {
		return domain.FXQuote{}, err
	}
	quote, err := domain.ParseCurrency(quoteCurrency)
	if err != nil {
		return domain.FXQuote{}, err
	}
	parsedRate, err := domain.ParseFxRate(rate)
	if err != nil {
		return domain.FXQuote{}, err
	}
	parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
	if err != nil {
		return domain.FXQuote{}, err
	}
	quoted, err := parsePortfolioTime(quotedAt)
	if err != nil {
		return domain.FXQuote{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.FXQuote{}, err
	}
	if base == quote {
		return domain.FXQuote{}, fmt.Errorf("invalid identical currencies in FX quote")
	}
	return domain.FXQuote{ID: quoteID, HouseholdID: hID, BaseCurrency: base, QuoteCurrency: quote, Rate: parsedRate, SourceKind: parsedSource, SourceKey: sourceKey, QuotedAt: quoted, CreatedAt: created, Delayed: delayed != 0}, nil
}

func scanFXQuotes(rows *sql.Rows) ([]domain.FXQuote, error) {
	var result []domain.FXQuote
	for rows.Next() {
		quote, err := scanFXQuote(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, quote)
	}
	return result, rows.Err()
}

func scanFXPreference(row interface{ Scan(...any) error }) (domain.FXPreference, error) {
	var householdID, currencyA, currencyB, sourceKind, createdAt, updatedAt string
	if err := row.Scan(&householdID, &currencyA, &currencyB, &sourceKind, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.FXPreference{}, &domain.Error{Code: domain.ErrNotFound, Message: "FX preference was not found"}
		}
		return domain.FXPreference{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.FXPreference{}, err
	}
	a, err := domain.ParseCurrency(currencyA)
	if err != nil {
		return domain.FXPreference{}, err
	}
	b, err := domain.ParseCurrency(currencyB)
	if err != nil {
		return domain.FXPreference{}, err
	}
	source, err := domain.ParseQuoteSourceKind(sourceKind)
	if err != nil {
		return domain.FXPreference{}, err
	}
	created, err := parsePortfolioTime(createdAt)
	if err != nil {
		return domain.FXPreference{}, err
	}
	updated, err := parsePortfolioTime(updatedAt)
	if err != nil {
		return domain.FXPreference{}, err
	}
	return domain.FXPreference{HouseholdID: hID, CurrencyA: a, CurrencyB: b, SourceKind: source, CreatedAt: created, UpdatedAt: updated}, nil
}

func scanFXPreferences(rows *sql.Rows) ([]domain.FXPreference, error) {
	var result []domain.FXPreference
	for rows.Next() {
		preference, err := scanFXPreference(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, preference)
	}
	return result, rows.Err()
}

func parsePortfolioTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrIntegrity, Field: "timestamp", Message: "stored timestamp is invalid"}
	}
	return parsed.UTC(), nil
}

func appendInstrumentQuoteTx(ctx context.Context, tx *sql.Tx, quote domain.InstrumentQuote) error {
	var instrumentCurrency string
	var archived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT quote_currency, archived_at FROM instruments WHERE id = ?`, quote.InstrumentID.String()).Scan(&instrumentCurrency, &archived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
		}
		return err
	}
	if archived.Valid {
		return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
	}
	if instrumentCurrency != quote.Currency.String() {
		return &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "quote currency does not match instrument"}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, quote.ID.String(), quote.InstrumentID.String(), quote.UnitPrice.Canonical(), quote.Currency.String(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed))
	return mapPortfolioWriteError(err, "instrument quote")
}

func appendFXQuoteTx(ctx context.Context, tx *sql.Tx, quote domain.FXQuote) error {
	if err := ensureHousehold(ctx, tx, quote.HouseholdID); err != nil {
		return err
	}
	if quote.BaseCurrency == quote.QuoteCurrency {
		return &domain.Error{Code: domain.ErrValidation, Field: "currencyPair", Message: "currencies must differ"}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, quote.ID.String(), quote.HouseholdID.String(), quote.BaseCurrency.String(), quote.QuoteCurrency.String(), quote.Rate.Canonical(), string(quote.SourceKind), quote.SourceKey, formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed))
	return mapPortfolioWriteError(err, "FX quote")
}
