package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ImportAgentQuoteBatch writes the receipt, quote facts, audit trail, and dirty
// history markers in one transaction. RequestKey is durable across retries.
func (r *Repository) ImportAgentQuoteBatch(ctx context.Context, batch domain.AgentQuoteBatch) (domain.AgentQuoteBatchResult, error) {
	result := domain.AgentQuoteBatchResult{RequestKey: batch.RequestKey, RecordIDs: []string{}, QuoteIDs: []string{}}
	if err := validateAgentQuoteBatch(batch); err != nil {
		return result, err
	}
	hash, err := agentQuotePayloadHash(batch.Records)
	if err != nil {
		return result, err
	}
	if batch.RequestHash != "" {
		if len(batch.RequestHash) != 64 || strings.Trim(batch.RequestHash, "0123456789abcdef") != "" {
			return result, agentQuoteError(domain.ErrValidation, "requestHash", "must be a lowercase SHA-256 digest")
		}
		hash = batch.RequestHash
	}
	err = r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var existingHousehold, existingHash string
		err := tx.QueryRowContext(ctx, `SELECT household_id, payload_hash FROM agent_quote_batches WHERE request_key = ?`, batch.RequestKey).Scan(&existingHousehold, &existingHash)
		if err == nil {
			if existingHousehold != batch.HouseholdID.String() || existingHash != hash {
				return agentQuoteError(domain.ErrConflict, "requestKey", "request key was used for different Agent quote data")
			}
			result.Replayed = true
			return loadAgentBatchResultTx(ctx, tx, &result)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := ensureHousehold(ctx, tx, batch.HouseholdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_quote_batches(request_key, household_id, payload_hash, created_at) VALUES(?, ?, ?, ?)`, batch.RequestKey, batch.HouseholdID.String(), hash, formatTimestamp(batch.CreatedAt)); err != nil {
			return err
		}
		for _, record := range batch.Records {
			if err := importAgentQuoteRecordTx(ctx, tx, batch, record, &result); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

// AgentQuoteBatchReceipt reads a committed receipt before the caller
// reconstructs mutable target state for a retry.
func (r *Repository) AgentQuoteBatchReceipt(ctx context.Context, householdID domain.HouseholdID, requestKey string) (domain.AgentQuoteBatchResult, string, bool, error) {
	result := domain.AgentQuoteBatchResult{RequestKey: requestKey, RecordIDs: []string{}, QuoteIDs: []string{}}
	var owner, hash string
	err := r.database.SQL.QueryRowContext(ctx, `SELECT household_id, payload_hash FROM agent_quote_batches WHERE request_key = ?`, requestKey).Scan(&owner, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return result, "", false, nil
	}
	if err != nil {
		return result, "", false, err
	}
	if owner != householdID.String() {
		return result, "", false, agentQuoteError(domain.ErrConflict, "requestKey", "request key belongs to another household")
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, "", false, err
	}
	defer tx.Rollback()
	if err := loadAgentBatchResultTx(ctx, tx, &result); err != nil {
		return result, "", false, err
	}
	if err := tx.Commit(); err != nil {
		return result, "", false, err
	}
	result.Replayed = true
	return result, hash, true, nil
}

func validateAgentQuoteBatch(batch domain.AgentQuoteBatch) error {
	if _, err := domain.ParseHouseholdID(batch.HouseholdID.String()); err != nil {
		return err
	}
	if strings.TrimSpace(batch.RequestKey) == "" || len(batch.RequestKey) > 128 {
		return agentQuoteError(domain.ErrValidation, "requestKey", "must be 1 to 128 characters")
	}
	if batch.CreatedAt.IsZero() {
		return agentQuoteError(domain.ErrValidation, "createdAt", "is required")
	}
	if len(batch.Records) == 0 || len(batch.Records) > 500 {
		return agentQuoteError(domain.ErrValidation, "records", "must contain 1 to 500 records")
	}
	for _, record := range batch.Records {
		if len([]rune(strings.TrimSpace(record.SourceTitle))) == 0 || len([]rune(record.SourceTitle)) > 256 {
			return agentQuoteError(domain.ErrValidation, "sourceTitle", "must contain 1 to 256 characters")
		}
		if strings.TrimSpace(record.SourceURL) != "" {
			parsedURL, err := url.Parse(strings.TrimSpace(record.SourceURL))
			if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" || parsedURL.User != nil || len(record.SourceURL) > 2048 {
				return agentQuoteError(domain.ErrValidation, "sourceURL", "must be an HTTP or HTTPS URL without credentials")
			}
		}
		if record.InstrumentQuote != nil && record.FXQuote != nil {
			return agentQuoteError(domain.ErrValidation, "record", "must contain one quote target")
		}
		switch record.Operation {
		case domain.AgentQuoteAppend:
			if record.TargetQuoteID != "" || (record.InstrumentQuote == nil && record.FXQuote == nil) {
				return agentQuoteError(domain.ErrValidation, "record", "append needs one quote and no target quote")
			}
		case domain.AgentQuoteCorrect:
			if record.TargetQuoteID == "" || (record.InstrumentQuote == nil && record.FXQuote == nil) {
				return agentQuoteError(domain.ErrValidation, "record", "correction needs one quote and a target quote")
			}
		case domain.AgentQuoteRetract:
			if record.TargetQuoteID == "" || record.InstrumentQuote != nil || record.FXQuote != nil {
				return agentQuoteError(domain.ErrValidation, "record", "retraction needs only a target quote")
			}
		default:
			return agentQuoteError(domain.ErrValidation, "operation", "is not supported")
		}
		if record.InstrumentQuote != nil {
			quote := record.InstrumentQuote
			if quote.SourceKind != domain.QuoteSourceAgent || quote.SourceKey != "agent" || quote.QuotedAt.IsZero() || quote.ID == "" {
				return agentQuoteError(domain.ErrValidation, "instrumentQuote", "must be a dated Agent quote")
			}
		}
		if record.FXQuote != nil {
			quote := record.FXQuote
			if quote.SourceKind != domain.QuoteSourceAgent || quote.SourceKey != "agent" || quote.QuotedAt.IsZero() || quote.ID == "" || quote.HouseholdID != batch.HouseholdID {
				return agentQuoteError(domain.ErrValidation, "fxQuote", "must be a dated Agent quote in this household")
			}
		}
	}
	return nil
}

func agentQuotePayloadHash(records []domain.AgentQuoteRecord) (string, error) {
	type semanticRecord struct {
		Record domain.AgentQuoteRecord
		Value  string
	}
	canonical := make([]semanticRecord, len(records))
	for index, record := range records {
		value := ""
		record.RecordID = ""
		record.CreatedAt = time.Time{}
		if record.InstrumentQuote != nil {
			quote := *record.InstrumentQuote
			value = quote.UnitPrice.Canonical()
			quote.ID = ""
			quote.CreatedAt = time.Time{}
			quote.FetchedAt = time.Time{}
			record.InstrumentQuote = &quote
		}
		if record.FXQuote != nil {
			quote := *record.FXQuote
			value = quote.Rate.Canonical()
			quote.ID = ""
			quote.CreatedAt = time.Time{}
			quote.FetchedAt = time.Time{}
			record.FXQuote = &quote
		}
		canonical[index] = semanticRecord{Record: record, Value: value}
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func loadAgentBatchResultTx(ctx context.Context, tx *sql.Tx, result *domain.AgentQuoteBatchResult) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, quote_id FROM agent_quote_records WHERE request_key = ? ORDER BY rowid`, result.RequestKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var recordID string
		var quoteID sql.NullString
		if err := rows.Scan(&recordID, &quoteID); err != nil {
			return err
		}
		result.RecordIDs = append(result.RecordIDs, recordID)
		if quoteID.Valid {
			result.QuoteIDs = append(result.QuoteIDs, quoteID.String)
		}
		result.Inserted++
	}
	return rows.Err()
}

func importAgentQuoteRecordTx(ctx context.Context, tx *sql.Tx, batch domain.AgentQuoteBatch, record domain.AgentQuoteRecord, result *domain.AgentQuoteBatchResult) error {
	quoteType := ""
	quoteID := ""
	if record.InstrumentQuote != nil {
		quoteType, quoteID = "instrument", record.InstrumentQuote.ID.String()
	} else if record.FXQuote != nil {
		quoteType, quoteID = "fx", record.FXQuote.ID.String()
	}
	if record.TargetQuoteID != "" {
		targetType, err := activeAgentTargetTypeTx(ctx, tx, batch.HouseholdID, record.TargetQuoteID)
		if err != nil {
			return err
		}
		if quoteType == "" {
			quoteType = targetType
		} else if quoteType != targetType {
			return agentQuoteError(domain.ErrValidation, "targetQuoteId", "target quote type differs from correction")
		}
		if record.InstrumentQuote != nil {
			var instrumentID string
			if err := tx.QueryRowContext(ctx, `SELECT instrument_id FROM instrument_quotes WHERE id = ?`, record.TargetQuoteID).Scan(&instrumentID); err != nil {
				return err
			}
			if instrumentID != record.InstrumentQuote.InstrumentID.String() {
				return agentQuoteError(domain.ErrValidation, "targetQuoteId", "target quote belongs to another instrument")
			}
		} else if record.FXQuote != nil {
			var base, quote string
			if err := tx.QueryRowContext(ctx, `SELECT base_currency, quote_currency FROM fx_quotes WHERE id = ?`, record.TargetQuoteID).Scan(&base, &quote); err != nil {
				return err
			}
			if base != record.FXQuote.BaseCurrency.String() || quote != record.FXQuote.QuoteCurrency.String() {
				return agentQuoteError(domain.ErrValidation, "targetQuoteId", "target quote belongs to another FX pair")
			}
		}
		if record.Operation == domain.AgentQuoteCorrect {
			if err := markRetractedAgentQuoteDirtyTx(ctx, tx, batch.HouseholdID, quoteType, record.TargetQuoteID, batch.CreatedAt); err != nil {
				return err
			}
		}
	}
	if record.InstrumentQuote != nil {
		if err := insertAgentInstrumentQuoteTx(ctx, tx, batch.HouseholdID, *record.InstrumentQuote, record.TargetQuoteID); err != nil {
			return err
		}
		if err := markAgentInstrumentQuoteDirtyTx(ctx, tx, *record.InstrumentQuote, batch.CreatedAt); err != nil {
			return err
		}
	} else if record.FXQuote != nil {
		if err := insertAgentFXQuoteTx(ctx, tx, *record.FXQuote, record.TargetQuoteID); err != nil {
			return err
		}
		if err := markAgentFXQuoteDirtyTx(ctx, tx, *record.FXQuote, batch.CreatedAt); err != nil {
			return err
		}
		if err := ensureAgentFXPreferenceTx(ctx, tx, *record.FXQuote, batch.CreatedAt); err != nil {
			return err
		}
	} else {
		if err := markRetractedAgentQuoteDirtyTx(ctx, tx, batch.HouseholdID, quoteType, record.TargetQuoteID, batch.CreatedAt); err != nil {
			return err
		}
	}
	recordID := strings.TrimSpace(record.RecordID)
	if recordID == "" {
		recordID = uuid.NewString()
	}
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = batch.CreatedAt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_quote_records(id, request_key, household_id, target_type, operation, quote_id, target_quote_id, source_title, source_url, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, recordID, batch.RequestKey, batch.HouseholdID.String(), quoteType, string(record.Operation), nullableEmpty(quoteID), nullableEmpty(record.TargetQuoteID), strings.TrimSpace(record.SourceTitle), strings.TrimSpace(record.SourceURL), formatTimestamp(createdAt)); err != nil {
		return mapPortfolioWriteError(err, "Agent quote record")
	}
	result.RecordIDs = append(result.RecordIDs, recordID)
	if quoteID != "" {
		result.QuoteIDs = append(result.QuoteIDs, quoteID)
	}
	result.Inserted++
	return nil
}

func activeAgentTargetTypeTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, quoteID string) (string, error) {
	var targetType string
	err := tx.QueryRowContext(ctx, `SELECT r.target_type FROM agent_quote_records r WHERE r.quote_id = ? AND r.household_id = ? AND NOT EXISTS (SELECT 1 FROM agent_quote_records withdrawn WHERE withdrawn.target_type = r.target_type AND withdrawn.target_quote_id = r.quote_id)`, quoteID, householdID.String()).Scan(&targetType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", agentQuoteError(domain.ErrNotFound, "targetQuoteId", "active Agent quote was not found")
	}
	return targetType, err
}

func agentQuoteError(code domain.ErrorCode, field, message string) error {
	return &domain.Error{Code: code, Field: field, Message: message}
}

func insertAgentInstrumentQuoteTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, quote domain.InstrumentQuote, supersedes string) error {
	var owner, currency string
	var archived sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT household_id, quote_currency, archived_at FROM instruments WHERE id = ?`, quote.InstrumentID.String()).Scan(&owner, &currency, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return agentQuoteError(domain.ErrNotFound, "instrumentId", "instrument was not found")
	}
	if err != nil {
		return err
	}
	if owner != householdID.String() || archived.Valid {
		return agentQuoteError(domain.ErrValidation, "instrumentId", "instrument is outside the active household")
	}
	if currency != quote.Currency.String() {
		return agentQuoteError(domain.ErrValidation, "currency", "quote currency does not match instrument")
	}
	if _, err := domain.ParseUnitPrice(quote.UnitPrice.Canonical()); err != nil {
		return err
	}
	if err := validateAgentEffectiveDate(quote.EffectiveDate); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, provider_timestamp, fetched_at, value_effective_at, binding_revision, source_policy_version, price_basis, timestamp_basis, revision, supersedes_quote_id, split_factor, dividend_cash, conversion_json)
		VALUES(?, ?, ?, ?, 'agent', 'agent', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		quote.ID.String(), quote.InstrumentID.String(), quote.UnitPrice.Canonical(), quote.Currency.String(),
		formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed), quote.ObservationKind,
		nullableEmpty(quote.EffectiveDate), nullableTimeValue(quote.ProviderTimestamp), nullableTimeValue(quote.FetchedAt), nullableTimeValue(quote.ValueEffectiveAt),
		nullablePositiveInt(quote.BindingRevision), nullableEmpty(quote.SourcePolicyVersion), nullableEmpty(quote.PriceBasis), nullableEmpty(quote.TimestampBasis),
		quoteRevision(quote.Revision), nullableEmpty(supersedes), nullableEmpty(quote.SplitFactor), nullableEmpty(quote.DividendCash), nullableEmpty(quote.ConversionJSON))
	return mapPortfolioWriteError(err, "Agent instrument quote")
}

func insertAgentFXQuoteTx(ctx context.Context, tx *sql.Tx, quote domain.FXQuote, supersedes string) error {
	if quote.BaseCurrency == quote.QuoteCurrency {
		return agentQuoteError(domain.ErrValidation, "currencyPair", "base and quote currencies must differ")
	}
	if _, err := domain.ParseFxRate(quote.Rate.Canonical()); err != nil {
		return err
	}
	if err := validateAgentEffectiveDate(quote.EffectiveDate); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, fetched_at, value_effective_at, source_policy_version, timestamp_basis, revision, supersedes_quote_id)
		VALUES(?, ?, ?, ?, ?, 'agent', 'agent', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		quote.ID.String(), quote.HouseholdID.String(), quote.BaseCurrency.String(), quote.QuoteCurrency.String(), quote.Rate.Canonical(),
		formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed), quote.ObservationKind, nullableEmpty(quote.EffectiveDate),
		nullableTimeValue(quote.FetchedAt), nullableTimeValue(quote.ValueEffectiveAt), nullableEmpty(quote.SourcePolicyVersion), nullableEmpty(quote.TimestampBasis), quoteRevision(quote.Revision), nullableEmpty(supersedes))
	return mapPortfolioWriteError(err, "Agent FX quote")
}

func validateAgentEffectiveDate(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return agentQuoteError(domain.ErrValidation, "effectiveDate", "must use YYYY-MM-DD")
	}
	return nil
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func quoteRevision(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func ensureAgentFXPreferenceTx(ctx context.Context, tx *sql.Tx, quote domain.FXQuote, at time.Time) error {
	a, b, err := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO fx_preferences(household_id, currency_a, currency_b, source_kind, created_at, updated_at)
		VALUES(?, ?, ?, 'agent', ?, ?) ON CONFLICT(household_id, currency_a, currency_b) DO NOTHING`,
		quote.HouseholdID.String(), a.String(), b.String(), formatTimestamp(at), formatTimestamp(at))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	var historyCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM history_origins WHERE household_id = ?`, quote.HouseholdID.String()).Scan(&historyCount); err != nil {
		return err
	}
	if historyCount == 0 {
		return nil
	}
	effectiveAt, err := clampFXHistoryEffectiveAtTx(ctx, tx, quote.HouseholdID, quote.QuotedAt)
	if err != nil {
		return err
	}
	observation := domain.FXPreferenceObservation{
		ID: domain.NewFXPreferenceObservationID(), HouseholdID: quote.HouseholdID,
		CurrencyA: a, CurrencyB: b, SourceKind: domain.QuoteSourceAgent,
		EffectiveAt: effectiveAt, CreatedAt: at,
	}
	return appendFXPreferenceObservationTx(ctx, tx, observation)
}

func markRetractedAgentQuoteDirtyTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, targetType, targetID string, at time.Time) error {
	if targetType == "instrument" {
		var instrumentID, quotedAt string
		var effectiveDate sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT instrument_id, quoted_at, effective_date FROM instrument_quotes WHERE id = ?`, targetID).Scan(&instrumentID, &quotedAt, &effectiveDate); err != nil {
			return err
		}
		when, err := parsePortfolioTime(quotedAt)
		if err != nil {
			return err
		}
		return markAgentInstrumentQuoteDirtyTx(ctx, tx, domain.InstrumentQuote{InstrumentID: domain.InstrumentID(instrumentID), QuotedAt: when, EffectiveDate: effectiveDate.String}, at)
	}
	var quotedAt string
	var effectiveDate sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT quoted_at, effective_date FROM fx_quotes WHERE id = ? AND household_id = ?`, targetID, householdID.String()).Scan(&quotedAt, &effectiveDate); err != nil {
		return err
	}
	when, err := parsePortfolioTime(quotedAt)
	if err != nil {
		return err
	}
	return markAgentFXQuoteDirtyTx(ctx, tx, domain.FXQuote{HouseholdID: householdID, QuotedAt: when, EffectiveDate: effectiveDate.String}, at)
}

func markAgentInstrumentQuoteDirtyTx(ctx context.Context, tx *sql.Tx, quote domain.InstrumentQuote, at time.Time) error {
	var householdID, timezone string
	err := tx.QueryRowContext(ctx, `SELECT i.household_id, o.timezone FROM instruments i JOIN history_origins o ON o.household_id = i.household_id WHERE i.id = ?`, quote.InstrumentID.String()).Scan(&householdID, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	date := observationEffectiveDate(quote.QuotedAt, timezone)
	if quote.EffectiveDate != "" && quote.EffectiveDate < date {
		date = quote.EffectiveDate
	}
	return markHistoryDirtyTx(ctx, tx, domain.HouseholdID(householdID), date, timezone, at)
}

func markAgentFXQuoteDirtyTx(ctx context.Context, tx *sql.Tx, quote domain.FXQuote, at time.Time) error {
	var timezone string
	err := tx.QueryRowContext(ctx, `SELECT timezone FROM history_origins WHERE household_id = ?`, quote.HouseholdID.String()).Scan(&timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	date := observationEffectiveDate(quote.QuotedAt, timezone)
	if quote.EffectiveDate != "" && quote.EffectiveDate < date {
		date = quote.EffectiveDate
	}
	return markHistoryDirtyTx(ctx, tx, quote.HouseholdID, date, timezone, at)
}

// ListAgentQuoteRecords returns every Agent audit record, including corrections
// and retractions. Quote selectors use the active subset separately.
func (r *Repository) ListAgentQuoteRecords(ctx context.Context, householdID domain.HouseholdID) ([]domain.AgentQuoteRecord, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, target_type, operation, quote_id, target_quote_id, source_title, source_url, created_at
		FROM agent_quote_records WHERE household_id = ? ORDER BY rowid`, householdID.String())
	if err != nil {
		return nil, err
	}
	type auditRow struct {
		record domain.AgentQuoteRecord
		kind   string
		quote  sql.NullString
	}
	var audits []auditRow
	for rows.Next() {
		var item auditRow
		var operation, created string
		var target sql.NullString
		if err := rows.Scan(&item.record.RecordID, &item.kind, &operation, &item.quote, &target, &item.record.SourceTitle, &item.record.SourceURL, &created); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.record.Operation = domain.AgentQuoteOperation(operation)
		item.record.TargetQuoteID = target.String
		item.record.CreatedAt, err = parsePortfolioTime(created)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		audits = append(audits, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]domain.AgentQuoteRecord, 0, len(audits))
	for _, item := range audits {
		if item.quote.Valid {
			if item.kind == "instrument" {
				quote, err := scanInstrumentQuote(tx.QueryRowContext(ctx, `SELECT id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, provider_timestamp, fetched_at, value_effective_at, binding_revision, source_policy_version, price_basis, timestamp_basis, revision, supersedes_quote_id, split_factor, dividend_cash, conversion_json FROM instrument_quotes WHERE id = ?`, item.quote.String))
				if err != nil {
					return nil, err
				}
				item.record.InstrumentQuote = &quote
			} else {
				quote, err := scanFXQuote(tx.QueryRowContext(ctx, `SELECT id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, fetched_at, value_effective_at, source_policy_version, timestamp_basis, revision, supersedes_quote_id FROM fx_quotes WHERE id = ?`, item.quote.String))
				if err != nil {
					return nil, err
				}
				item.record.FXQuote = &quote
			}
		}
		result = append(result, item.record)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
