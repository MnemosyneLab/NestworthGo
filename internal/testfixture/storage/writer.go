// Package storage builds explicitly synthetic ledger fixtures. No product
// command or application repository imports this package. It uses the already
// opened temporary database connection and its normal transaction wrapper.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/domain"
	"strings"
	"sync"
	"time"
)

type Handle struct {
	SQL    *sql.DB
	WithTx func(context.Context, func(*sql.Tx) error) error
}
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Writer struct{ database Handle }

func NewWriter(h Handle) *Writer { return &Writer{database: h} }

var handles sync.Map
var stagingDatabases sync.Map

// Pending reports an explicitly constructed synthetic fixture, not a product state.
func (r *Writer) Pending() bool { _, ok := stagingDatabases.Load(r.database.SQL); return ok }

func Register(path string, h Handle) { handles.Store(path, h) }
func PendingPath(path string) bool {
	h, ok := handles.Load(path)
	return ok && NewWriter(h.(Handle)).Pending()
}

func ForPath(path string) *Writer {
	h, ok := handles.Load(path)
	if !ok {
		panic("synthetic fixture handle was not registered: " + path)
	}
	return NewWriter(h.(Handle))
}

func (r *Writer) CreateOnboarding(ctx context.Context, household domain.Household, members []domain.Member) error {
	if len(members) == 0 {
		return &domain.Error{Code: domain.ErrValidation, Field: "members", Message: "at least one member is required"}
	}
	operation := func(tx *sql.Tx) error {
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE singleton_key = 1`).Scan(&existing); err == nil {
			return &domain.Error{Code: domain.ErrConflict, Message: "a Household already exists"}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO households(id, singleton_key, name, base_currency, created_at, updated_at) VALUES(?, 1, ?, ?, ?, ?)`, household.ID.String(), household.Name, household.BaseCurrency.String(), formatTimestamp(household.CreatedAt), formatTimestamp(household.UpdatedAt)); err != nil {
			return err
		}
		for index, member := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO members(id, household_id, name, icon_key, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, member.ID.String(), household.ID.String(), member.Name, nullableString(member.IconKey), nullableString(member.Note), index, formatTimestamp(member.CreatedAt), formatTimestamp(member.UpdatedAt)); err != nil {
				return err
			}
		}
		// Install an ordinary empty origin before any application operation
		// can read or write the synthetic database.
		origin, err := domain.NewHistoryOrigin(household.ID, "UTC", household.CreatedAt.UTC().Truncate(24*time.Hour), household.CreatedAt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES(?,?,?,?,?)`, origin.ID.String(), household.ID.String(), "UTC", formatTimestamp(origin.StartedAt), formatTimestamp(origin.CreatedAt)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at, input_generation, resolver_policy_version) VALUES(?, NULL, NULL, ?, 0, ?)`, household.ID.String(), formatTimestamp(origin.CreatedAt), domain.MarketDataResolverPolicy)
		return err
	}
	err := r.database.WithTx(ctx, operation)
	if err == nil {
		stagingDatabases.Store(r.database.SQL, struct{}{})
	}
	return err
}

func (r *Writer) CaptureSyntheticOrigin(ctx context.Context, data domain.HistoryOriginData) (domain.HistoryOrigin, error) {
	if err := validateHistoryOriginData(data); err != nil {
		return domain.HistoryOrigin{}, err
	}
	var result domain.HistoryOrigin
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var existingID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM history_origins WHERE household_id = ?`, data.Origin.HouseholdID.String()).Scan(&existingID); err == nil {
			if !r.Pending() {
				_, scanErr := scanHistoryOrigin(tx.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE id = ?`, existingID), &result)
				return scanErr
			}
			// This is a synthetic database created by this writer in this
			// process. Replace setup-only facts with the requested immutable
			// baseline atomically. Quote observations remain intact.
			for _, statement := range []string{
				"DELETE FROM daily_valuation_snapshot_items",
				"DELETE FROM daily_valuation_snapshots",
				"DELETE FROM account_state_ownership",
				"DELETE FROM account_state_observations",
				"DELETE FROM instrument_preference_observations",
				"DELETE FROM fx_preference_observations",
				"DELETE FROM instrument_state_observations",
				"DELETE FROM holding_state_observations",
				"DELETE FROM activity_mutation_keys",
				"DELETE FROM activity_trade_details",
				"DELETE FROM activity_dividend_details",
				"DELETE FROM holding_quantity_values",
				"UPDATE account_values SET activity_effect_id=NULL, projection_kind='baseline'",
				"UPDATE account_cash_values SET activity_effect_id=NULL, projection_kind='baseline'",
				"DELETE FROM activity_effects",
				"DELETE FROM activities",
				"DELETE FROM history_snapshot_state",
				"DELETE FROM history_origins",
			} {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return fmt.Errorf("synthetic origin setup: %w", err)
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var householdID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE id = ?`, data.Origin.HouseholdID.String()).Scan(&householdID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "Household was not found"}
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origins(id, household_id, timezone, started_at, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), data.Origin.HouseholdID.String(), data.Origin.Timezone, formatTimestamp(data.Origin.StartedAt), formatTimestamp(data.Origin.CreatedAt)); err != nil {
			return err
		}
		for _, component := range data.Components {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_components(id, origin_id, component_kind, account_id, holding_id, instrument_id, amount, currency, quantity, unit_cost, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, component.ID.String(), data.Origin.ID.String(), string(component.Kind), nullableID(component.AccountID), nullableID(component.HoldingID), nullableID(component.InstrumentID), nullableMoney(component.Amount), nullableMoneyCurrency(component.Amount), nullableQuantity(component.Quantity), nullableUnitPrice(component.UnitCost), formatTimestamp(component.CreatedAt)); err != nil {
				return err
			}
		}
		for _, state := range data.AccountStates {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_account_states(origin_id, account_id, archived_at, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, data.Origin.ID.String(), state.AccountID.String(), nullableTime(state.ArchivedAt), boolValue(state.IncludeInNetWorth), boolValue(state.IncludeInPortfolio), boolValue(state.IncludeInLiquidAssets), formatTimestamp(state.CreatedAt)); err != nil {
				return err
			}
		}
		for _, ownership := range data.Ownership {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_ownership(origin_id, account_id, member_id, share_bps) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), ownership.AccountID.String(), ownership.MemberID.String(), ownership.ShareBPS); err != nil {
				return err
			}
		}
		for _, preference := range data.InstrumentPreferences {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_instrument_preferences(origin_id, instrument_id, source_kind, created_at) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), preference.InstrumentID.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
				return err
			}
		}
		for _, preference := range data.FXPreferences {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_fx_preferences(origin_id, currency_a, currency_b, source_kind, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), preference.CurrencyA.String(), preference.CurrencyB.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at, input_generation, resolver_policy_version) VALUES(?, NULL, NULL, ?, 0, ?)`, data.Origin.HouseholdID.String(), formatTimestamp(data.Origin.CreatedAt), domain.MarketDataResolverPolicy); err != nil {
			return err
		}
		result = data.Origin
		return nil
	})
	if err == nil {
		stagingDatabases.Delete(r.database.SQL)
	}
	return result, err
}

func (r *Writer) CreateAccount(ctx context.Context, account domain.Account, ownership domain.Ownership, initial *domain.AccountValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return insertAccountCore(ctx, tx, account, ownership, initial, false)
	})
}

func (r *Writer) UpdateAccount(ctx context.Context, account domain.Account, ownership domain.Ownership) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var currentInstitution, currentGroup sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT institution_id, group_id FROM accounts WHERE id = ? AND household_id = ?`, account.ID.String(), account.HouseholdID.String()).Scan(&currentInstitution, &currentGroup); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
			}
			return err
		}
		if err := validateAccountReferences(ctx, tx, account, nullableStringValue(currentInstitution), nullableStringValue(currentGroup)); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE accounts SET institution_id = ?, group_id = ?, name = ?, account_type = ?, balance_sheet_role = ?, tracking_mode = ?, default_currency = ?, note = ?, icon_key = ?, include_in_net_worth = ?, include_in_portfolio = ?, include_in_liquid_assets = ?, opened_on = ?, closed_on = ?, sort_order = ?, updated_at = ? WHERE id = ? AND household_id = ?`, nullableID(account.InstitutionID), nullableID(account.GroupID), account.Name, account.AccountType.String(), string(account.BalanceSheetRole), string(account.TrackingMode), account.DefaultCurrency.String(), nullableString(account.Note), accountIconValue(account.IconKey), boolValue(account.IncludeInNetWorth), boolValue(account.IncludeInPortfolio), boolValue(account.IncludeInLiquidAssets), nullableString(account.OpenedOn), nullableString(account.ClosedOn), account.SortOrder, formatTimestamp(account.UpdatedAt), account.ID.String(), account.HouseholdID.String())
		if err != nil {
			return err
		}
		if err := requireAffected(result, "account"); err != nil {
			return err
		}
		return replaceOwnership(ctx, tx, account.ID, ownership, true)
	})
}

func (r *Writer) AppendAccountValue(ctx context.Context, value domain.AccountValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error { return insertAccountValue(ctx, tx, value) })
}

func (r *Writer) SetAccountArchive(ctx context.Context, householdID domain.HouseholdID, id domain.AccountID, archived bool, now time.Time) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE accounts SET archived_at = ?, updated_at = ? WHERE id = ? AND household_id = ?`, archiveValue(archived, now), formatTimestamp(now), id.String(), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "account")
}

func (r *Writer) CreateInstrument(ctx context.Context, instrument domain.Instrument) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, instrument.HouseholdID); err != nil {
			return err
		}
		if err := validateInstrumentBinding(instrument); err != nil {
			return err
		}
		if err := insertInstrumentTx(ctx, tx, instrument); err != nil {
			return err
		}
		return syncInstrumentBindingFromInstrumentTx(ctx, tx, instrument)
	})
}

func (r *Writer) UpdateInstrument(ctx context.Context, instrument domain.Instrument) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateInstrumentBinding(instrument); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET name = ?, short_name = ?, instrument_type = ?, quote_currency = ?, symbol = ?, market_code = ?, country_code = ?, isin = ?, note = ?, icon_key = ?, sort_order = ?, quote_source = ?, provider_key = ?, provider_symbol = ?, metal_template = ?, quantity_unit = ?, updated_at = ? WHERE id = ? AND household_id = ?`, instrument.Name, instrument.ShortName, string(instrument.Type), instrument.QuoteCurrency.String(), nullableString(instrument.Symbol), nullableString(instrument.MarketCode), nullableString(instrument.CountryCode), nullableString(instrument.ISIN), nullableString(instrument.Note), nullableString(instrument.IconKey), instrument.SortOrder, string(instrument.QuoteSource), nullableString(instrument.ProviderKey), nullableString(instrument.ProviderSymbol), instrument.MetalTemplate, instrument.QuantityUnit, formatTimestamp(instrument.UpdatedAt), instrument.ID.String(), instrument.HouseholdID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		if err := requireAffected(result, "instrument"); err != nil {
			return err
		}
		return syncInstrumentBindingFromInstrumentTx(ctx, tx, instrument)
	})
}

func (r *Writer) SetInstrumentQuoteSource(ctx context.Context, householdID domain.HouseholdID, id domain.InstrumentID, source domain.QuoteSourceKind, now time.Time) error {
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
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET quote_source = ?, updated_at = ? WHERE id = ? AND household_id = ? AND archived_at IS NULL`, string(parsedSource), formatTimestamp(now), id.String(), householdID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "instrument")
		}
		if err := requireAffected(result, "instrument"); err != nil {
			return err
		}
		return nil
	})
}

func (r *Writer) CreateHolding(ctx context.Context, holding domain.Holding) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateHoldingReferences(ctx, tx, holding.AccountID, holding.InstrumentID, nil); err != nil {
			return err
		}
		return insertHolding(ctx, tx, holding)
	})
}

func (r *Writer) UpdateHolding(ctx context.Context, holding domain.Holding) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var accountArchived sql.NullString
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT h.archived_at, a.archived_at FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE h.id = ? AND h.account_id = ?`, holding.ID.String(), holding.AccountID.String()).Scan(&archived, &accountArchived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
			}
			return err
		}
		if archived.Valid || accountArchived.Valid {
			return &domain.Error{Code: domain.ErrConflict, Message: "holding is no longer active"}
		}
		result, err := tx.ExecContext(ctx, `UPDATE holdings SET quantity = ?, note = ?, sort_order = ?, updated_at = ? WHERE id = ? AND account_id = ? AND archived_at IS NULL`, holding.Quantity.Canonical(), nullableString(holding.Note), holding.SortOrder, formatTimestamp(holding.UpdatedAt), holding.ID.String(), holding.AccountID.String())
		if err != nil {
			return mapPortfolioWriteError(err, "holding")
		}
		if err := requireAffected(result, "holding"); err != nil {
			return &domain.Error{Code: domain.ErrConflict, Message: "holding is no longer active"}
		}
		return nil
	})
}

func (r *Writer) AppendAccountCashValue(ctx context.Context, value domain.AccountCashValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var mode string
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT tracking_mode, archived_at FROM accounts WHERE id = ?`, value.AccountID.String()).Scan(&mode, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
			}
			return err
		}
		if archived.Valid {
			return &domain.Error{Code: domain.ErrConflict, Message: "account is no longer active"}
		}
		if mode != string(domain.TrackingHoldings) {
			return &domain.Error{Code: domain.ErrValidation, Field: "trackingMode", Message: "cash observations require a Holdings account"}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, NULL, 'baseline')`, value.ID.String(), value.AccountID.String(), value.Amount.CanonicalAmount(), value.Amount.Currency().String(), formatTimestamp(value.EffectiveAt), formatTimestamp(value.CreatedAt))
		return mapPortfolioWriteError(err, "cash value")
	})
}

func (r *Writer) SetFXPreference(ctx context.Context, preference domain.FXPreference) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureHousehold(ctx, tx, preference.HouseholdID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO fx_preferences(household_id, currency_a, currency_b, source_kind, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(household_id, currency_a, currency_b) DO UPDATE SET source_kind = excluded.source_kind, updated_at = excluded.updated_at`, preference.HouseholdID.String(), preference.CurrencyA.String(), preference.CurrencyB.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt), formatTimestamp(preference.UpdatedAt))
		return mapPortfolioWriteError(err, "FX preference")
	})
}

func accountIconValue(value *string) any {
	if value == nil {
		return ""
	}
	return *value
}

func archiveValue(archived bool, now time.Time) any {
	if archived {
		return formatTimestamp(now)
	}
	return nil
}

func boolValue(value bool) int { return nullableBool(value) }

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

func formatTimestamp(value time.Time) string {
	return value.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z07:00")
}

func insertAccount(ctx context.Context, tx *sql.Tx, account domain.Account) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, household_id, institution_id, group_id, name, account_type, balance_sheet_role, tracking_mode, default_currency, note, icon_key, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, opened_on, closed_on, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, account.ID.String(), account.HouseholdID.String(), nullableID(account.InstitutionID), nullableID(account.GroupID), account.Name, account.AccountType.String(), string(account.BalanceSheetRole), string(account.TrackingMode), account.DefaultCurrency.String(), nullableString(account.Note), nullableString(account.IconKey), boolValue(account.IncludeInNetWorth), boolValue(account.IncludeInPortfolio), boolValue(account.IncludeInLiquidAssets), nullableString(account.OpenedOn), nullableString(account.ClosedOn), account.SortOrder, formatTimestamp(account.CreatedAt), formatTimestamp(account.UpdatedAt))
	return err
}

func insertAccountCore(ctx context.Context, tx *sql.Tx, account domain.Account, ownership domain.Ownership, initial *domain.AccountValue, requireInitial bool) error {
	if err := validateAccountReferences(ctx, tx, account, nil, nil); err != nil {
		return err
	}
	if account.TrackingMode == domain.TrackingHoldings && initial != nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "Holdings accounts cannot have an initial Account Value"}
	}
	if requireInitial && account.TrackingMode != domain.TrackingHoldings && initial == nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "an initial Account Value is required"}
	}
	if initial != nil && (initial.AccountID != account.ID || initial.ValueKind != account.TrackingMode || initial.Amount.Currency() != account.DefaultCurrency) {
		return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "initial value does not match the account"}
	}
	if err := insertAccount(ctx, tx, account); err != nil {
		return err
	}
	if err := replaceOwnership(ctx, tx, account.ID, ownership, false); err != nil {
		return err
	}
	if initial == nil {
		return nil
	}
	return insertAccountValue(ctx, tx, *initial)
}

func insertAccountValue(ctx context.Context, tx *sql.Tx, value domain.AccountValue) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, ?, ?, ?, ?, ?, NULL, 'baseline')`, value.ID.String(), value.AccountID.String(), string(value.ValueKind), value.Amount.CanonicalAmount(), value.Amount.Currency().String(), formatTimestamp(value.EffectiveAt), formatTimestamp(value.CreatedAt))
	return err
}

func insertHolding(ctx context.Context, tx *sql.Tx, holding domain.Holding) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO holdings(id, account_id, instrument_id, quantity, note, sort_order, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, holding.ID.String(), holding.AccountID.String(), holding.InstrumentID.String(), holding.Quantity.Canonical(), nullableString(holding.Note), holding.SortOrder, formatTimestamp(holding.CreatedAt), formatTimestamp(holding.UpdatedAt), nullableTime(holding.ArchivedAt))
	return mapPortfolioWriteError(err, "holding")
}

func insertInstrumentTx(ctx context.Context, tx *sql.Tx, instrument domain.Instrument) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO instruments(id, household_id, name, short_name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, icon_key, sort_order, quote_source, provider_key, provider_symbol, metal_template, quantity_unit, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, instrument.ID.String(), instrument.HouseholdID.String(), instrument.Name, instrument.ShortName, string(instrument.Type), instrument.QuoteCurrency.String(), nullableString(instrument.Symbol), nullableString(instrument.MarketCode), nullableString(instrument.CountryCode), nullableString(instrument.ISIN), nullableString(instrument.Note), nullableString(instrument.IconKey), instrument.SortOrder, string(instrument.QuoteSource), nullableString(instrument.ProviderKey), nullableString(instrument.ProviderSymbol), instrument.MetalTemplate, instrument.QuantityUnit, formatTimestamp(instrument.CreatedAt), formatTimestamp(instrument.UpdatedAt), nullableTime(instrument.ArchivedAt))
	return mapPortfolioWriteError(err, "instrument")
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

func markHistoryDirtyAndAdvanceGenerationTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, effectiveLocalDate, timezone string, asOf time.Time) (bool, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	var startedAt string
	if err := tx.QueryRowContext(ctx, `SELECT started_at FROM history_origins WHERE household_id = ?`, householdID.String()).Scan(&startedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	origin, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return false, &domain.Error{Code: domain.ErrIntegrity, Message: "stored history Starting point is invalid"}
	}
	originDate := origin.In(location).Format("2006-01-02")
	if effectiveLocalDate < originDate {
		effectiveLocalDate = originDate
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if effectiveLocalDate >= asOf.In(location).Format("2006-01-02") {
		return false, nil
	}
	// A later ordinary change can extend beyond a prior bounded correction or
	// market-history invalidation. Preserve unbounded work, but widen an existing
	// bound through the last closed household day in this same transaction.
	yesterday := asOf.In(location).AddDate(0, 0, -1).Format("2006-01-02")
	result, err := tx.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from IS NULL OR dirty_from > ? THEN ? ELSE dirty_from END, dirty_to = CASE WHEN dirty_to IS NOT NULL AND dirty_to < ? THEN ? ELSE dirty_to END, input_generation = input_generation + 1, resolver_policy_version = CASE WHEN resolver_policy_version IS NULL OR resolver_policy_version = '' THEN ? ELSE resolver_policy_version END, updated_at = ? WHERE household_id = ?`, effectiveLocalDate, effectiveLocalDate, yesterday, yesterday, domain.MarketDataResolverPolicy, formatTimestamp(asOf), householdID.String())
	if err != nil {
		return false, err
	}
	if err := requireAffected(result, "history snapshot state"); err != nil {
		return false, err
	}
	return true, nil
}

func markHistoryDirtyTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, effectiveLocalDate, timezone string, asOf time.Time) error {
	_, err := markHistoryDirtyAndAdvanceGenerationTx(ctx, tx, householdID, effectiveLocalDate, timezone, asOf)
	return err
}

func markInstrumentBindingHistoryDirtyTx(ctx context.Context, tx *sql.Tx, instrumentID domain.InstrumentID, at time.Time) error {
	var householdID, timezone string
	if err := tx.QueryRowContext(ctx, `SELECT i.household_id, o.timezone FROM instruments i JOIN history_origins o ON o.household_id = i.household_id WHERE i.id = ?`, instrumentID.String()).Scan(&householdID, &timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	parsedHouseholdID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return err
	}
	return markHistoryDirtyTx(ctx, tx, parsedHouseholdID, observationEffectiveDate(at, timezone), timezone, at)
}

func nullableBool(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableID[T ~string](value *T) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func nullableMoney(value *domain.Money) any {
	if value == nil {
		return nil
	}
	return value.CanonicalAmount()
}

func nullableMoneyCurrency(value *domain.Money) any {
	if value == nil {
		return nil
	}
	return value.Currency().String()
}

func nullableQuantity(value *domain.Quantity) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTimestamp(*value)
}

func nullableUnitPrice(value *domain.UnitPrice) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}

func observationEffectiveDate(value time.Time, timezone string) string {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return value.UTC().Format("2006-01-02")
	}
	return value.In(location).Format("2006-01-02")
}

func replaceOwnership(ctx context.Context, tx *sql.Tx, accountID domain.AccountID, ownership domain.Ownership, allowArchivedExisting bool) error {
	if _, err := domain.ParseOwnership(ownership.Shares()); err != nil {
		return err
	}
	var householdID string
	if err := tx.QueryRowContext(ctx, `SELECT household_id FROM accounts WHERE id = ?`, accountID.String()).Scan(&householdID); err != nil {
		return err
	}
	existing := make(map[string]bool)
	rows, err := tx.QueryContext(ctx, `SELECT member_id FROM account_ownership WHERE account_id = ?`, accountID.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			return err
		}
		existing[memberID] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_ownership WHERE account_id = ?`, accountID.String()); err != nil {
		return err
	}
	for _, share := range ownership.Shares() {
		var memberHousehold string
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT household_id, archived_at FROM members WHERE id = ?`, share.MemberID.String()).Scan(&memberHousehold, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "member was not found"}
			}
			return err
		}
		if memberHousehold != householdID {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "owner belongs to another household"}
		}
		if archived.Valid && (!allowArchivedExisting || !existing[share.MemberID.String()]) {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "new owners must be active members"}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_ownership(account_id, member_id, share_bps) VALUES(?, ?, ?)`, accountID.String(), share.MemberID.String(), share.ShareBPS); err != nil {
			return err
		}
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(share_bps), 0) FROM account_ownership WHERE account_id = ?`, accountID.String()).Scan(&total); err != nil {
		return err
	}
	if total != domain.TotalOwnershipBPS {
		return storedIntegrity("ownership", "shares must total exactly 10000 basis points")
	}
	return nil
}

func requireAffected(result sql.Result, entity string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return &domain.Error{Code: domain.ErrNotFound, Message: entity + " was not found"}
	}
	return nil
}

func scanHistoryOrigin(row interface{ Scan(...any) error }, destination ...*domain.HistoryOrigin) (domain.HistoryOrigin, error) {
	var id, householdID, timezone, startedAt, createdAt string
	if err := row.Scan(&id, &householdID, &timezone, &startedAt, &createdAt); err != nil {
		return domain.HistoryOrigin{}, err
	}
	parsedID, err := domain.ParseHistoryOriginID(id)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	result := domain.HistoryOrigin{ID: parsedID, HouseholdID: parsedHousehold, Timezone: timezone, StartedAt: started.UTC(), CreatedAt: created.UTC()}
	if len(destination) > 0 && destination[0] != nil {
		*destination[0] = result
	}
	return result, nil
}

func storedIntegrity(field, message string) *domain.Error {
	return &domain.Error{Code: domain.ErrIntegrity, Field: field, Message: message}
}

func syncInstrumentBindingFromInstrumentTx(ctx context.Context, tx *sql.Tx, instrument domain.Instrument) error {
	if instrument.ProviderKey == nil || instrument.ProviderSymbol == nil {
		return nil
	}
	market := ""
	if instrument.MarketCode != nil {
		market = *instrument.MarketCode
	}
	_, changed, err := syncInstrumentProviderBindingTx(ctx, tx, instrument.ID.String(), *instrument.ProviderKey, *instrument.ProviderSymbol, market, instrument.QuoteCurrency.String(), instrument.UpdatedAt)
	if err != nil || !changed {
		return err
	}
	return markInstrumentBindingHistoryDirtyTx(ctx, tx, instrument.ID, instrument.UpdatedAt)
}

func syncInstrumentProviderBindingTx(ctx context.Context, tx *sql.Tx, instrumentID, providerKey, providerSymbol, market, currency string, at time.Time) (int, bool, error) {
	providerKey = strings.TrimSpace(providerKey)
	providerSymbol = strings.TrimSpace(providerSymbol)
	if providerKey == "" || providerSymbol == "" {
		return 0, false, nil
	}
	var revision int
	var currentSymbol string
	err := tx.QueryRowContext(ctx, `SELECT binding_revision, provider_symbol FROM instrument_provider_bindings WHERE instrument_id = ? AND provider_key = ?`, instrumentID, providerKey).Scan(&revision, &currentSymbol)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_bindings(instrument_id, provider_key, provider_symbol, market, currency, enabled, created_at, updated_at, binding_revision, effective_from) VALUES(?, ?, ?, ?, ?, 1, ?, ?, 1, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, 1, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, false, err
		}
		return 1, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	// Binding revisions identify a provider-symbol revision. Market/currency
	// metadata may be learned later (for example, on the first history fetch)
	// without creating a second economic route for the same symbol.
	if currentSymbol == providerSymbol {
		return revision, false, nil
	}
	next := revision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE instrument_provider_bindings SET provider_symbol = ?, market = ?, currency = ?, binding_revision = ?, effective_from = ?, updated_at = ? WHERE instrument_id = ? AND provider_key = ?`, providerSymbol, nullableEmpty(market), nullableEmpty(currency), next, formatTimestamp(at), formatTimestamp(at), instrumentID, providerKey); err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, next, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
		return 0, false, err
	}
	return next, true, nil
}

func validateAccountReferences(ctx context.Context, tx *sql.Tx, account domain.Account, retainedInstitution, retainedGroup *string) error {
	check := func(table, id string, retained *string) error {
		var householdID string
		var archived sql.NullString
		query := fmt.Sprintf("SELECT household_id, archived_at FROM %s WHERE id = ?", table)
		if err := tx.QueryRowContext(ctx, query, id).Scan(&householdID, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: table + " reference was not found"}
			}
			return err
		}
		if householdID != account.HouseholdID.String() {
			return &domain.Error{Code: domain.ErrValidation, Field: table, Message: "reference belongs to another household"}
		}
		if archived.Valid && (retained == nil || *retained != id) {
			return &domain.Error{Code: domain.ErrValidation, Field: table, Message: "reference is archived"}
		}
		return nil
	}
	if account.InstitutionID != nil {
		if err := check("institutions", account.InstitutionID.String(), retainedInstitution); err != nil {
			return err
		}
	}
	if account.GroupID != nil {
		if err := check("account_groups", account.GroupID.String(), retainedGroup); err != nil {
			return err
		}
	}
	return nil
}

func validateHistoryOriginData(data domain.HistoryOriginData) error {
	if _, err := domain.ParseHistoryOriginID(data.Origin.ID.String()); err != nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "originId", Message: "origin ID is required"}
	}
	if _, err := domain.NewHistoryOrigin(data.Origin.HouseholdID, data.Origin.Timezone, data.Origin.StartedAt, data.Origin.CreatedAt); err != nil {
		return err
	}
	for index := range data.Components {
		component := data.Components[index]
		component.OriginID = data.Origin.ID
		if err := component.Validate(); err != nil {
			return err
		}
		data.Components[index].OriginID = data.Origin.ID
	}
	sharesByAccount := make(map[domain.AccountID][]domain.OwnershipShare)
	for _, ownership := range data.Ownership {
		sharesByAccount[ownership.AccountID] = append(sharesByAccount[ownership.AccountID], domain.OwnershipShare{MemberID: ownership.MemberID, ShareBPS: ownership.ShareBPS})
	}
	for _, shares := range sharesByAccount {
		if _, err := domain.ParseOwnership(shares); err != nil {
			return err
		}
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

func validateInstrumentBinding(instrument domain.Instrument) error {
	if instrument.QuoteSource == domain.QuoteSourceProvider && (instrument.ProviderKey == nil || instrument.ProviderSymbol == nil || strings.TrimSpace(*instrument.ProviderKey) == "" || strings.TrimSpace(*instrument.ProviderSymbol) == "") {
		return &domain.Error{Code: domain.ErrValidation, Field: "provider", Message: "provider key and symbol are required when Provider is selected"}
	}
	return nil
}
