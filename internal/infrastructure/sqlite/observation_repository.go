package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) AppendAccountStateObservation(ctx context.Context, observation domain.AccountStateObservation) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return appendAccountStateObservationTx(ctx, tx, observation)
	})
}

func (r *Repository) AppendInstrumentPreferenceObservation(ctx context.Context, observation domain.InstrumentPreferenceObservation) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return appendInstrumentPreferenceObservationTx(ctx, tx, observation)
	})
}

func (r *Repository) AppendFXPreferenceObservation(ctx context.Context, observation domain.FXPreferenceObservation) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return appendFXPreferenceObservationTx(ctx, tx, observation)
	})
}

func (r *Repository) ListAccountStateObservations(ctx context.Context, householdID domain.HouseholdID) ([]domain.AccountStateObservation, error) {
	return listAccountStateObservationsQuery(ctx, r.database.SQL, householdID)
}

func listAccountStateObservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.AccountStateObservation, error) {
	rows, err := query.QueryContext(ctx, `SELECT aso.id, aso.account_id, aso.effective_at, aso.archived_at, aso.include_in_net_worth, aso.include_in_portfolio, aso.include_in_liquid_assets, aso.activity_id, aso.created_at FROM account_state_observations aso JOIN accounts a ON a.id = aso.account_id WHERE a.household_id = ? ORDER BY aso.account_id, aso.effective_at, aso.created_at, aso.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	observations := make([]domain.AccountStateObservation, 0)
	for rows.Next() {
		var id, accountID, effectiveAt, createdAt string
		var archivedAt, activityID sql.NullString
		var includeNetWorth, includeInvestment, includeLiquid int
		if err := rows.Scan(&id, &accountID, &effectiveAt, &archivedAt, &includeNetWorth, &includeInvestment, &includeLiquid, &activityID, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseAccountStateObservationID(id)
		if err != nil {
			return nil, err
		}
		parsedAccount, err := domain.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		archived, err := parseTimePtr(archivedAt)
		if err != nil {
			return nil, err
		}
		var parsedActivity *domain.ActivityID
		if activityID.Valid && activityID.String != "" {
			value, parseErr := domain.ParseActivityID(activityID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedActivity = &value
		}
		observations = append(observations, domain.AccountStateObservation{ID: parsedID, AccountID: parsedAccount, EffectiveAt: effective.UTC(), ArchivedAt: archived, IncludeInNetWorth: includeNetWorth != 0, IncludeInPortfolio: includeInvestment != 0, IncludeInLiquidAssets: includeLiquid != 0, ActivityID: parsedActivity, CreatedAt: created.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	indices := make(map[domain.AccountStateObservationID]int, len(observations))
	for index := range observations {
		indices[observations[index].ID] = index
	}
	rows, err = query.QueryContext(ctx, `SELECT aso.observation_id, aso.member_id, aso.share_bps FROM account_state_ownership aso JOIN account_state_observations state ON state.id = aso.observation_id JOIN accounts a ON a.id = state.account_id WHERE a.household_id = ? ORDER BY aso.observation_id, aso.member_id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var observationID, memberID string
		var shareBPS int
		if err := rows.Scan(&observationID, &memberID, &shareBPS); err != nil {
			return nil, err
		}
		index, ok := indices[domain.AccountStateObservationID(observationID)]
		if !ok {
			continue
		}
		parsedMember, err := domain.ParseMemberID(memberID)
		if err != nil {
			return nil, err
		}
		observations[index].Ownership = append(observations[index].Ownership, domain.OwnershipShare{MemberID: parsedMember, ShareBPS: shareBPS})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return observations, nil
}

func (r *Repository) ListInstrumentStateObservations(ctx context.Context, householdID domain.HouseholdID) ([]domain.InstrumentStateObservation, error) {
	return listInstrumentStateObservationsQuery(ctx, r.database.SQL, householdID)
}

func listInstrumentStateObservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.InstrumentStateObservation, error) {
	rows, err := query.QueryContext(ctx, `SELECT iso.id, iso.instrument_id, iso.effective_at, iso.archived_at, iso.activity_id, iso.created_at FROM instrument_state_observations iso JOIN instruments i ON i.id = iso.instrument_id WHERE i.household_id = ? ORDER BY iso.instrument_id, iso.effective_at, iso.created_at, iso.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.InstrumentStateObservation
	for rows.Next() {
		var id, instrumentID, effectiveAt, createdAt string
		var archivedAt, activityID sql.NullString
		if err := rows.Scan(&id, &instrumentID, &effectiveAt, &archivedAt, &activityID, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseInstrumentStateObservationID(id)
		if err != nil {
			return nil, err
		}
		parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		archived, err := parseTimePtr(archivedAt)
		if err != nil {
			return nil, err
		}
		var parsedActivity *domain.ActivityID
		if activityID.Valid && activityID.String != "" {
			value, parseErr := domain.ParseActivityID(activityID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedActivity = &value
		}
		result = append(result, domain.InstrumentStateObservation{ID: parsedID, InstrumentID: parsedInstrument, EffectiveAt: effective.UTC(), ArchivedAt: archived, ActivityID: parsedActivity, CreatedAt: created.UTC()})
	}
	return result, rows.Err()
}

func (r *Repository) ListHoldingStateObservations(ctx context.Context, householdID domain.HouseholdID) ([]domain.HoldingStateObservation, error) {
	return listHoldingStateObservationsQuery(ctx, r.database.SQL, householdID)
}

func listHoldingStateObservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.HoldingStateObservation, error) {
	rows, err := query.QueryContext(ctx, `SELECT hso.id, hso.holding_id, hso.effective_at, hso.archived_at, hso.activity_id, hso.created_at FROM holding_state_observations hso JOIN holdings h ON h.id = hso.holding_id JOIN accounts a ON a.id = h.account_id WHERE a.household_id = ? ORDER BY hso.holding_id, hso.effective_at, hso.created_at, hso.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.HoldingStateObservation
	for rows.Next() {
		var id, holdingID, effectiveAt, createdAt string
		var archivedAt, activityID sql.NullString
		if err := rows.Scan(&id, &holdingID, &effectiveAt, &archivedAt, &activityID, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseHoldingStateObservationID(id)
		if err != nil {
			return nil, err
		}
		parsedHolding, err := domain.ParseHoldingID(holdingID)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		archived, err := parseTimePtr(archivedAt)
		if err != nil {
			return nil, err
		}
		var parsedActivity *domain.ActivityID
		if activityID.Valid && activityID.String != "" {
			value, parseErr := domain.ParseActivityID(activityID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedActivity = &value
		}
		result = append(result, domain.HoldingStateObservation{ID: parsedID, HoldingID: parsedHolding, EffectiveAt: effective.UTC(), ArchivedAt: archived, ActivityID: parsedActivity, CreatedAt: created.UTC()})
	}
	return result, rows.Err()
}

func (r *Repository) ListInstrumentPreferenceObservations(ctx context.Context, householdID domain.HouseholdID) ([]domain.InstrumentPreferenceObservation, error) {
	return listInstrumentPreferenceObservationsQuery(ctx, r.database.SQL, householdID)
}

func listInstrumentPreferenceObservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.InstrumentPreferenceObservation, error) {
	rows, err := query.QueryContext(ctx, `SELECT ipo.id, ipo.instrument_id, ipo.source_kind, ipo.effective_at, ipo.activity_id, ipo.created_at FROM instrument_preference_observations ipo JOIN instruments i ON i.id = ipo.instrument_id WHERE i.household_id = ? ORDER BY ipo.instrument_id, ipo.effective_at, ipo.created_at, ipo.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.InstrumentPreferenceObservation, 0)
	for rows.Next() {
		var id, instrumentID, sourceKind, effectiveAt, createdAt string
		var activityID sql.NullString
		if err := rows.Scan(&id, &instrumentID, &sourceKind, &effectiveAt, &activityID, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseInstrumentPreferenceObservationID(id)
		if err != nil {
			return nil, err
		}
		parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
		if err != nil {
			return nil, err
		}
		parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		var parsedActivity *domain.ActivityID
		if activityID.Valid && activityID.String != "" {
			value, parseErr := domain.ParseActivityID(activityID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedActivity = &value
		}
		result = append(result, domain.InstrumentPreferenceObservation{ID: parsedID, InstrumentID: parsedInstrument, SourceKind: parsedSource, EffectiveAt: effective.UTC(), ActivityID: parsedActivity, CreatedAt: created.UTC()})
	}
	return result, rows.Err()
}

func (r *Repository) ListFXPreferenceObservations(ctx context.Context, householdID domain.HouseholdID) ([]domain.FXPreferenceObservation, error) {
	return listFXPreferenceObservationsQuery(ctx, r.database.SQL, householdID)
}

func listFXPreferenceObservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.FXPreferenceObservation, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, household_id, currency_a, currency_b, source_kind, effective_at, activity_id, created_at FROM fx_preference_observations WHERE household_id = ? ORDER BY currency_a, currency_b, effective_at, created_at, id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.FXPreferenceObservation, 0)
	for rows.Next() {
		var id, rowHousehold, currencyA, currencyB, sourceKind, effectiveAt, createdAt string
		var activityID sql.NullString
		if err := rows.Scan(&id, &rowHousehold, &currencyA, &currencyB, &sourceKind, &effectiveAt, &activityID, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseFXPreferenceObservationID(id)
		if err != nil {
			return nil, err
		}
		parsedHousehold, err := domain.ParseHouseholdID(rowHousehold)
		if err != nil {
			return nil, err
		}
		parsedA, err := domain.ParseCurrency(currencyA)
		if err != nil {
			return nil, err
		}
		parsedB, err := domain.ParseCurrency(currencyB)
		if err != nil {
			return nil, err
		}
		parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		var parsedActivity *domain.ActivityID
		if activityID.Valid && activityID.String != "" {
			value, parseErr := domain.ParseActivityID(activityID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedActivity = &value
		}
		result = append(result, domain.FXPreferenceObservation{ID: parsedID, HouseholdID: parsedHousehold, CurrencyA: parsedA, CurrencyB: parsedB, SourceKind: parsedSource, EffectiveAt: effective.UTC(), ActivityID: parsedActivity, CreatedAt: created.UTC()})
	}
	return result, rows.Err()
}

func appendAccountStateObservationTx(ctx context.Context, tx *sql.Tx, observation domain.AccountStateObservation) error {
	if _, err := domain.ParseOwnership(observation.Ownership); err != nil {
		return err
	}
	household, timezone, err := historyOwnerTx(ctx, tx, observationAccountHouseholdQuery, observation.AccountID.String())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO account_state_observations(id, account_id, effective_at, archived_at, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, activity_id, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, observation.ID.String(), observation.AccountID.String(), formatTimestamp(observation.EffectiveAt), nullableTime(observation.ArchivedAt), boolValue(observation.IncludeInNetWorth), boolValue(observation.IncludeInPortfolio), boolValue(observation.IncludeInLiquidAssets), nullableActivityID(observation.ActivityID), formatTimestamp(observation.CreatedAt)); err != nil {
		return err
	}
	for _, share := range observation.Ownership {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_state_ownership(observation_id, member_id, share_bps) VALUES(?, ?, ?)`, observation.ID.String(), share.MemberID.String(), share.ShareBPS); err != nil {
			return err
		}
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(share_bps), 0) FROM account_state_ownership WHERE observation_id = ?`, observation.ID.String()).Scan(&total); err != nil {
		return err
	}
	if total != domain.TotalOwnershipBPS {
		return storedIntegrity("ownership", "shares must total exactly 10000 basis points")
	}
	return markHistoryDirtyTx(ctx, tx, household, observationEffectiveDate(observation.EffectiveAt, timezone), timezone, observation.CreatedAt)
}

func appendInstrumentPreferenceObservationTx(ctx context.Context, tx *sql.Tx, observation domain.InstrumentPreferenceObservation) error {
	household, timezone, err := historyOwnerTx(ctx, tx, `SELECT i.household_id, o.timezone FROM history_origins o JOIN instruments i ON i.household_id = o.household_id WHERE i.id = ?`, observation.InstrumentID.String())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_preference_observations(id, instrument_id, source_kind, effective_at, activity_id, created_at) VALUES(?, ?, ?, ?, ?, ?)`, observation.ID.String(), observation.InstrumentID.String(), string(observation.SourceKind), formatTimestamp(observation.EffectiveAt), nullableActivityID(observation.ActivityID), formatTimestamp(observation.CreatedAt)); err != nil {
		return err
	}
	return markHistoryDirtyTx(ctx, tx, household, observationEffectiveDate(observation.EffectiveAt, timezone), timezone, observation.CreatedAt)
}

func appendFXPreferenceObservationTx(ctx context.Context, tx *sql.Tx, observation domain.FXPreferenceObservation) error {
	var timezone string
	if err := tx.QueryRowContext(ctx, `SELECT timezone FROM history_origins WHERE household_id = ?`, observation.HouseholdID.String()).Scan(&timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a preference"}
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fx_preference_observations(id, household_id, currency_a, currency_b, source_kind, effective_at, activity_id, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, observation.ID.String(), observation.HouseholdID.String(), observation.CurrencyA.String(), observation.CurrencyB.String(), string(observation.SourceKind), formatTimestamp(observation.EffectiveAt), nullableActivityID(observation.ActivityID), formatTimestamp(observation.CreatedAt)); err != nil {
		return err
	}
	return markHistoryDirtyTx(ctx, tx, observation.HouseholdID, observationEffectiveDate(observation.EffectiveAt, timezone), timezone, observation.CreatedAt)
}

const observationAccountHouseholdQuery = `SELECT a.household_id, o.timezone FROM history_origins o JOIN accounts a ON a.household_id = o.household_id WHERE a.id = ?`

func historyOwnerTx(ctx context.Context, tx *sql.Tx, query string, id string) (domain.HouseholdID, string, error) {
	var householdID, timezone string
	if err := tx.QueryRowContext(ctx, query, id).Scan(&householdID, &timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording an observation"}
		}
		return "", "", err
	}
	household, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return "", "", err
	}
	return household, timezone, nil
}

func observationEffectiveDate(value time.Time, timezone string) string {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return value.UTC().Format("2006-01-02")
	}
	return value.In(location).Format("2006-01-02")
}
