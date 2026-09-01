package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) CommitCSVImport(ctx context.Context, batch domain.CSVImportBatch) error {
	if r == nil || r.database == nil {
		return &domain.Error{Code: domain.ErrCSVCommitFailed, Message: "database is not open"}
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateCSVImportTx(ctx, tx, batch); err != nil {
			return err
		}
		return commitCSVImportTx(ctx, tx, batch)
	})
}

func validateCSVImportTx(ctx context.Context, tx *sql.Tx, batch domain.CSVImportBatch) error {
	institutionName := func(id *domain.InstitutionID) (string, error) {
		if id == nil {
			return "", nil
		}
		var name string
		err := tx.QueryRowContext(ctx, `SELECT name FROM institutions WHERE id = ?`, id.String()).Scan(&name)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		for _, institution := range batch.Institutions {
			if institution.ID == *id {
				return institution.Name, nil
			}
		}
		return "", nil
	}
	for _, item := range batch.Accounts {
		name, err := institutionName(item.Account.InstitutionID)
		if err != nil {
			return mapCSVWriteError(err)
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a LEFT JOIN institutions i ON i.id = a.institution_id WHERE a.household_id = ? AND a.name = ? AND a.default_currency = ? AND COALESCE(i.name, '') = ? AND a.archived_at IS NULL`, item.Account.HouseholdID.String(), item.Account.Name, item.Account.DefaultCurrency.String(), name).Scan(&count); err != nil {
			return mapCSVWriteError(err)
		}
		if count > 0 {
			return &domain.Error{Code: domain.ErrCSVDuplicate, Message: "an account with this name, currency, and institution already exists"}
		}
	}
	for _, holding := range batch.Holdings {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM holdings WHERE account_id = ? AND instrument_id = ? AND archived_at IS NULL`, holding.AccountID.String(), holding.InstrumentID.String()).Scan(&count); err != nil {
			return mapCSVWriteError(err)
		}
		if count > 0 {
			return &domain.Error{Code: domain.ErrCSVDuplicate, Message: "a holding for this account and instrument already exists"}
		}
	}
	return nil
}

func commitCSVImportTx(ctx context.Context, tx *sql.Tx, batch domain.CSVImportBatch) error {
	for _, member := range batch.Members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO members(id, household_id, name, icon_key, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM members WHERE household_id = ?), ?, ?)`, member.ID.String(), member.HouseholdID.String(), member.Name, nullableString(member.IconKey), nullableString(member.Note), member.HouseholdID.String(), formatTimestamp(member.CreatedAt), formatTimestamp(member.UpdatedAt)); err != nil {
			return mapCSVWriteError(err)
		}
	}
	for _, institution := range batch.Institutions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO institutions(id, household_id, name, icon_key, institution_type, country_code, website, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM institutions WHERE household_id = ?), ?, ?)`, institution.ID.String(), institution.HouseholdID.String(), institution.Name, nullableString(institution.IconKey), string(institution.InstitutionType), nullableString(institution.CountryCode), nullableString(institution.Website), nullableString(institution.Note), institution.HouseholdID.String(), formatTimestamp(institution.CreatedAt), formatTimestamp(institution.UpdatedAt)); err != nil {
			return mapCSVWriteError(err)
		}
	}
	for _, group := range batch.Groups {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(id, household_id, name, icon_key, color, description, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM account_groups WHERE household_id = ?), ?, ?)`, group.ID.String(), group.HouseholdID.String(), group.Name, nullableString(group.IconKey), nullableString(group.Color), nullableString(group.Description), group.HouseholdID.String(), formatTimestamp(group.CreatedAt), formatTimestamp(group.UpdatedAt)); err != nil {
			return mapCSVWriteError(err)
		}
	}
	for _, item := range batch.Accounts {
		requireInitial := item.Observation != nil
		if err := insertAccountCore(ctx, tx, item.Account, item.Ownership, item.Value, requireInitial); err != nil {
			return mapCSVWriteError(err)
		}
		if item.Observation != nil {
			if err := appendAccountStateObservationTx(ctx, tx, *item.Observation); err != nil {
				return mapCSVWriteError(err)
			}
		}
		if item.Activity != nil {
			var timezone string
			if err := tx.QueryRowContext(ctx, `SELECT timezone FROM history_origins WHERE household_id = ?`, item.Account.HouseholdID.String()).Scan(&timezone); err != nil {
				return mapCSVWriteError(err)
			}
			if err := commitActivityTx(ctx, tx, *item.Activity, batch.AsOf); err != nil {
				return mapCSVWriteError(err)
			}
			if err := markHistoryDirtyTx(ctx, tx, item.Account.HouseholdID, item.Activity.Activity.EffectiveLocalDate, timezone, batch.AsOf); err != nil {
				return mapCSVWriteError(err)
			}
		}
	}
	for _, item := range batch.Instruments {
		if err := validateInstrumentBinding(item.Instrument); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instruments(id, household_id, name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, icon_key, sort_order, quote_source, provider_key, provider_symbol, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.Instrument.ID.String(), item.Instrument.HouseholdID.String(), item.Instrument.Name, string(item.Instrument.Type), item.Instrument.QuoteCurrency.String(), nullableString(item.Instrument.Symbol), nullableString(item.Instrument.MarketCode), nullableString(item.Instrument.CountryCode), nullableString(item.Instrument.ISIN), nullableString(item.Instrument.Note), nullableString(item.Instrument.IconKey), item.Instrument.SortOrder, string(item.Instrument.QuoteSource), nullableString(item.Instrument.ProviderKey), nullableString(item.Instrument.ProviderSymbol), formatTimestamp(item.Instrument.CreatedAt), formatTimestamp(item.Instrument.UpdatedAt), nullableTime(item.Instrument.ArchivedAt)); err != nil {
			return mapCSVWriteError(err)
		}
		if item.Observation != nil {
			if err := appendInstrumentPreferenceObservationTx(ctx, tx, *item.Observation); err != nil {
				return mapCSVWriteError(err)
			}
		}
	}
	for _, holding := range batch.Holdings {
		if err := validateHoldingReferences(ctx, tx, holding.AccountID, holding.InstrumentID, nil); err != nil {
			return mapCSVWriteError(err)
		}
		if err := insertHolding(ctx, tx, holding); err != nil {
			return mapCSVWriteError(err)
		}
	}
	for _, quote := range batch.Quotes {
		if err := appendInstrumentQuoteTx(ctx, tx, quote); err != nil {
			return mapCSVWriteError(err)
		}
		if err := markQuoteHistoryDirtyTx(ctx, tx, quote.InstrumentID, quote.QuotedAt, quote.CreatedAt); err != nil {
			return mapCSVWriteError(err)
		}
	}
	return nil
}

func mapCSVWriteError(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return err
	}
	return &domain.Error{Code: domain.ErrCSVCommitFailed, Message: "the import could not be written"}
}
