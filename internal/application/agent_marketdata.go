package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// AgentMarketDataRepository atomically commits observations, their provenance,
// withdrawals, snapshot invalidation and an idempotency receipt.
type AgentMarketDataRepository interface {
	ImportAgentQuoteBatch(context.Context, domain.AgentQuoteBatch) (domain.AgentQuoteBatchResult, error)
	ListAgentQuoteRecords(context.Context, domain.HouseholdID) ([]domain.AgentQuoteRecord, error)
	AgentQuoteBatchReceipt(context.Context, domain.HouseholdID, string) (domain.AgentQuoteBatchResult, string, bool, error)
}

type AgentMarketDataInput struct {
	Items []AgentMarketDataItem `json:"items" jsonschema:"1 to 100 observations or withdrawals committed atomically"`
}

type AgentMarketDataItem struct {
	Operation     string `json:"operation,omitempty" jsonschema:"append (default), correct, or retract; corrections and withdrawals require targetQuoteId"`
	TargetQuoteID string `json:"targetQuoteId,omitempty"`
	InstrumentID  string `json:"instrumentId,omitempty" jsonschema:"For an instrument quote; omit for FX"`
	BaseCurrency  string `json:"baseCurrency,omitempty" jsonschema:"FX only: 1 baseCurrency equals value quoteCurrency"`
	QuoteCurrency string `json:"quoteCurrency,omitempty" jsonschema:"FX only"`
	Currency      string `json:"currency,omitempty" jsonschema:"Instrument quote currency; must match the instrument"`
	Value         string `json:"value,omitempty" jsonschema:"Exact decimal string: raw unit price, unit NAV or FX rate. Never cumulative NAV or annualized yield."`
	Kind          string `json:"kind,omitempty" jsonschema:"Instrument: latest, close, nav. FX: latest, daily_reference."`
	Date          string `json:"date,omitempty" jsonschema:"Required YYYY-MM-DD market/NAV/reference date for daily observations"`
	QuotedAt      string `json:"quotedAt,omitempty" jsonschema:"Actual RFC3339 quote timestamp required for latest; optional for daily data, never the lookup time"`
	Delayed       bool   `json:"delayed,omitempty" jsonschema:"Whether the source identifies this as a delayed quote"`
	Timezone      string `json:"timezone,omitempty" jsonschema:"Daily date timezone; defaults to instrument market timezone, then household history timezone, then UTC"`
	SplitFactor   string `json:"splitFactor,omitempty" jsonschema:"Optional positive split factor for a daily instrument observation; preserved as source metadata"`
	DividendCash  string `json:"dividendCash,omitempty" jsonschema:"Optional nonnegative cash dividend per unit in instrument quote currency; metadata only, not a ledger dividend"`
	SourceTitle   string `json:"sourceTitle" jsonschema:"Name of the actual information source, document or correction reason"`
	SourceURL     string `json:"sourceUrl,omitempty" jsonschema:"Optional HTTP(S) source URL, stored as provenance only; the App does not fetch it"`
}

type AgentMarketDataResult struct {
	RequestKey             string   `json:"requestKey"`
	Replayed               bool     `json:"replayed"`
	Inserted               int      `json:"inserted"`
	RecordIDs              []string `json:"recordIds"`
	QuoteIDs               []string `json:"quoteIds"`
	SnapshotDaysRebuilt    int      `json:"snapshotDaysRebuilt"`
	CurrentValuationStatus string   `json:"currentValuationStatus"`
	SnapshotStatus         string   `json:"snapshotStatus"`
}

func agentDataValidation(field, message string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: message}
}

func (s *Service) ImportAgentMarketData(ctx context.Context, requestKey string, input AgentMarketDataInput) (AgentMarketDataResult, error) {
	var result AgentMarketDataResult
	if _, err := uuid.Parse(requestKey); err != nil {
		return result, agentDataValidation("operationId", "must be a UUID")
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return result, agentDataValidation("items", "must contain 1 to 100 items")
	}
	repo, ok := s.repository.(AgentMarketDataRepository)
	if !ok {
		return result, &domain.Error{Code: domain.ErrUnavailable, Message: "agent market data persistence is unavailable"}
	}
	writeCtx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return result, err
	}
	stored, err := func() (domain.AgentQuoteBatchResult, error) {
		household, err := s.requireHousehold(writeCtx)
		if err != nil {
			return domain.AgentQuoteBatchResult{}, err
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return domain.AgentQuoteBatchResult{}, err
		}
		digest := sha256.Sum256(encoded)
		requestHash := hex.EncodeToString(digest[:])
		receipt, priorHash, exists, err := repo.AgentQuoteBatchReceipt(writeCtx, household.ID, requestKey)
		if err != nil {
			return domain.AgentQuoteBatchResult{}, err
		}
		if exists {
			if priorHash != requestHash {
				return domain.AgentQuoteBatchResult{}, &domain.Error{Code: domain.ErrConflict, Field: "operationId", Message: "request key was used for different Agent market data"}
			}
			receipt.Replayed = true
			return receipt, nil
		}
		batch := domain.AgentQuoteBatch{HouseholdID: household.ID, RequestKey: requestKey, RequestHash: requestHash, CreatedAt: s.clock()}
		for _, item := range input.Items {
			record, err := s.agentMarketDataRecord(writeCtx, household.ID, item)
			if err != nil {
				return domain.AgentQuoteBatchResult{}, err
			}
			batch.Records = append(batch.Records, record)
		}
		return repo.ImportAgentQuoteBatch(writeCtx, batch)
	}()
	unlock()
	if err != nil {
		return result, err
	}
	result = AgentMarketDataResult{RequestKey: stored.RequestKey, Replayed: stored.Replayed, Inserted: stored.Inserted, RecordIDs: stored.RecordIDs, QuoteIDs: stored.QuoteIDs, SnapshotStatus: "not_started", CurrentValuationStatus: "updated"}
	s.invalidateAnalysis()
	// FX also affects stored, converted metal references. Reuse the same local
	// dependent revaluation as manual/provider FX updates; never fetch a quote.
	needsFXReprice := false
	for _, item := range input.Items {
		if item.BaseCurrency != "" || item.Operation == "retract" {
			needsFXReprice = true
		}
	}
	if needsFXReprice {
		household, err := s.requireHousehold(ctx)
		if err != nil {
			result.CurrentValuationStatus = "pending"
		} else {
			preferences, err := s.repository.ListFXPreferences(ctx, household.ID)
			if err != nil {
				result.CurrentValuationStatus = "pending"
			} else {
				for _, preference := range preferences {
					if err := s.repriceMetalsForFX(ctx, household.ID, preference.CurrencyA, preference.CurrencyB); err != nil {
						result.CurrentValuationStatus = "pending"
					}
				}
			}
		}
	}
	// Quote persistence has already committed. Report rebuild separately rather
	// than converting a durable import into a failed/ambiguous write receipt.
	origin, originErr := s.HistoryOrigin(ctx)
	if originErr != nil {
		result.SnapshotStatus = "pending"
		return result, nil
	}
	if origin == nil {
		return result, nil
	}
	count, rebuildErr := s.RebuildDirtySnapshots(ctx)
	result.SnapshotDaysRebuilt = count
	result.SnapshotStatus = "rebuilt_check_health"
	if rebuildErr != nil {
		result.SnapshotStatus = "pending"
	}
	return result, nil
}

func (s *Service) AgentMarketDataRecords(ctx context.Context) ([]domain.AgentQuoteRecord, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repository.(AgentMarketDataRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "agent market data persistence is unavailable"}
	}
	return repo.ListAgentQuoteRecords(ctx, household.ID)
}

func (s *Service) agentMarketDataRecord(ctx context.Context, householdID domain.HouseholdID, item AgentMarketDataItem) (domain.AgentQuoteRecord, error) {
	record := domain.AgentQuoteRecord{RecordID: uuid.NewString(), Operation: domain.AgentQuoteOperation(item.Operation), TargetQuoteID: item.TargetQuoteID, SourceTitle: strings.TrimSpace(item.SourceTitle), SourceURL: strings.TrimSpace(item.SourceURL), CreatedAt: s.clock()}
	if record.Operation == "" {
		record.Operation = domain.AgentQuoteAppend
	}
	if record.Operation != domain.AgentQuoteAppend && record.Operation != domain.AgentQuoteCorrect && record.Operation != domain.AgentQuoteRetract {
		return record, agentDataValidation("operation", "must be append, correct or retract")
	}
	if record.SourceTitle == "" || len([]rune(record.SourceTitle)) > 256 {
		return record, agentDataValidation("sourceTitle", "source title is required and must not exceed 256 characters")
	}
	if record.SourceURL != "" {
		u, err := url.Parse(record.SourceURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(record.SourceURL) > 2048 {
			return record, agentDataValidation("sourceUrl", "must be an HTTP(S) URL without credentials, at most 2048 characters")
		}
	}
	if record.Operation != domain.AgentQuoteAppend {
		if _, err := uuid.Parse(record.TargetQuoteID); err != nil {
			return record, agentDataValidation("targetQuoteId", "must reference an existing Agent quote UUID")
		}
	} else if record.TargetQuoteID != "" {
		return record, agentDataValidation("targetQuoteId", "only corrections or withdrawals may target an existing quote")
	}
	if record.Operation == domain.AgentQuoteRetract {
		if item.InstrumentID != "" || item.BaseCurrency != "" || item.QuoteCurrency != "" || item.Currency != "" || item.Value != "" || item.Kind != "" || item.Date != "" || item.QuotedAt != "" || item.Timezone != "" || item.SplitFactor != "" || item.DividendCash != "" || item.Delayed {
			return record, agentDataValidation("items", "a withdrawal accepts only targetQuoteId and provenance")
		}
		return record, nil
	}
	var instrument *domain.Instrument
	if item.InstrumentID != "" {
		if item.BaseCurrency != "" || item.QuoteCurrency != "" {
			return record, agentDataValidation("items", "choose an instrument or an FX pair, not both")
		}
		id, err := domain.ParseInstrumentID(item.InstrumentID)
		if err != nil {
			return record, err
		}
		v, err := s.repository.Instrument(ctx, householdID, id)
		if err != nil {
			return record, err
		}
		if v.ArchivedAt != nil {
			return record, agentDataValidation("instrumentId", "instrument is archived")
		}
		if err := s.rejectManagedInstrument(ctx, id); err != nil {
			return record, err
		}
		instrument = &v
		if item.Kind != "latest" && item.Kind != "close" && item.Kind != "nav" {
			return record, agentDataValidation("kind", "instrument kind must be latest, close or nav")
		}
	} else {
		if item.Currency != "" || item.BaseCurrency == "" || item.QuoteCurrency == "" {
			return record, agentDataValidation("items", "FX requires baseCurrency and quoteCurrency, without currency")
		}
		if item.Kind != "latest" && item.Kind != "daily_reference" {
			return record, agentDataValidation("kind", "FX kind must be latest or daily_reference")
		}
	}
	if (item.SplitFactor != "" || item.DividendCash != "") && (instrument == nil || item.Kind == "latest") {
		return record, agentDataValidation("items", "split/dividend metadata belongs to a daily instrument observation")
	}
	when, basis, err := s.agentMarketDataTime(ctx, item, instrument)
	if err != nil {
		return record, err
	}
	if instrument != nil {
		if item.Currency != instrument.QuoteCurrency.String() {
			return record, agentDataValidation("currency", "must explicitly match the instrument quote currency")
		}
		price, err := domain.ParseUnitPrice(item.Value)
		if err != nil {
			return record, err
		}
		q, err := domain.NewInstrumentQuote(*instrument, domain.InstrumentQuoteInput{UnitPrice: price, Currency: instrument.QuoteCurrency, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: when, Delayed: item.Delayed}, s.clock())
		if err != nil {
			return record, err
		}
		q.ObservationKind, q.PriceBasis = "close", "agent_raw_close_v1"
		if item.Kind == "nav" {
			q.PriceBasis = "agent_unit_nav_v1"
		}
		if item.Kind == "latest" {
			q.ObservationKind, q.PriceBasis = "realtime", "agent_latest_v1"
		}
		q.EffectiveDate, q.ValueEffectiveAt, q.ProviderTimestamp, q.FetchedAt = item.Date, when, when, s.clock()
		q.TimestampBasis, q.SourcePolicyVersion = basis, "agent_supplied_v1"
		q.BindingRevision = instrument.ProviderBindingRevision
		if item.SplitFactor != "" {
			factor, err := domain.ParseQuantity(item.SplitFactor)
			if err != nil {
				return record, err
			}
			if factor.IsZero() {
				return record, agentDataValidation("splitFactor", "must be positive")
			}
			q.SplitFactor = factor.Canonical()
		}
		if item.DividendCash != "" {
			dividend, err := domain.ParseUnitPrice(item.DividendCash)
			if err != nil {
				return record, err
			}
			q.DividendCash = dividend.Canonical()
		}
		record.InstrumentQuote = &q
	} else {
		base, err := domain.ParseSupportedCurrency(item.BaseCurrency)
		if err != nil {
			return record, err
		}
		quote, err := domain.ParseSupportedCurrency(item.QuoteCurrency)
		if err != nil {
			return record, err
		}
		rate, err := domain.ParseFxRate(item.Value)
		if err != nil {
			return record, err
		}
		q, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: householdID, BaseCurrency: base, QuoteCurrency: quote, Rate: rate, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: when, Delayed: item.Delayed}, s.clock())
		if err != nil {
			return record, err
		}
		q.ObservationKind = item.Kind
		q.EffectiveDate, q.ValueEffectiveAt, q.FetchedAt = item.Date, when, s.clock()
		q.TimestampBasis, q.SourcePolicyVersion = basis, "agent_supplied_v1"
		record.FXQuote = &q
	}
	return record, nil
}

func (s *Service) agentMarketDataTime(ctx context.Context, item AgentMarketDataItem, instrument *domain.Instrument) (time.Time, string, error) {
	location := s.seriesLocation(ctx)
	if instrument != nil && instrument.MarketCode != nil {
		if schedule, ok := domain.EquitySessionScheduleForMarket(*instrument.MarketCode); ok {
			location, _ = time.LoadLocation(schedule.Timezone)
		}
	}
	if item.Timezone != "" {
		var err error
		location, err = time.LoadLocation(item.Timezone)
		if err != nil {
			return time.Time{}, "", agentDataValidation("timezone", "must be an IANA timezone")
		}
	}
	var when time.Time
	basis := "source_timestamp"
	if item.Kind == "latest" {
		if item.Date != "" {
			return when, basis, agentDataValidation("date", "latest requires quotedAt, not a daily date")
		}
		parsed, err := time.Parse(time.RFC3339Nano, item.QuotedAt)
		if err != nil {
			return when, basis, agentDataValidation("quotedAt", "latest requires an actual RFC3339 quote time")
		}
		when = parsed
	} else {
		date, err := time.ParseInLocation("2006-01-02", item.Date, location)
		if err != nil || date.Format("2006-01-02") != item.Date {
			return when, basis, agentDataValidation("date", "must be a valid YYYY-MM-DD observation date")
		}
		when, basis = date, "date_label"
		if item.QuotedAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, item.QuotedAt)
			if err != nil || parsed.In(location).Format("2006-01-02") != item.Date {
				return when, basis, agentDataValidation("quotedAt", "daily quote time must belong to date in its timezone")
			}
			when, basis = parsed, "source_timestamp"
		}
	}
	if when.After(s.clock()) {
		return when, basis, agentDataValidation("quotedAt", "future observations are not allowed")
	}
	return when.UTC(), basis, nil
}
