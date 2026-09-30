package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAgentQuoteBatchReplayCorrectionRetractionAndExport(t *testing.T) {
	_, repository, household, _, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	quotedAt := now.Add(-24 * time.Hour)
	price, _ := domain.ParseUnitPrice("100.125")
	rate, _ := domain.ParseFxRate("7.123456789012")
	instrumentQuote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: quotedAt}, now)
	if err != nil {
		t.Fatal(err)
	}
	instrumentQuote.ObservationKind = "close"
	instrumentQuote.EffectiveDate = "2026-09-28"
	instrumentQuote.ValueEffectiveAt = quotedAt
	instrumentQuote.SourcePolicyVersion = "agent_supplied_v1"
	instrumentQuote.PriceBasis = "agent_raw_close_v1"
	instrumentQuote.TimestampBasis = "source_timestamp"
	fxQuote, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: household.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: quotedAt}, now)
	if err != nil {
		t.Fatal(err)
	}
	fxQuote.ObservationKind = "daily_reference"
	fxQuote.EffectiveDate = "2026-09-28"
	batch := domain.AgentQuoteBatch{HouseholdID: household.ID, RequestKey: "agent-batch-1", CreatedAt: now, Records: []domain.AgentQuoteRecord{
		{Operation: domain.AgentQuoteAppend, InstrumentQuote: &instrumentQuote, SourceTitle: "Exchange close", SourceURL: "https://example.com/close"},
		{Operation: domain.AgentQuoteAppend, FXQuote: &fxQuote, SourceTitle: "FX reference"},
	}}
	first, err := repository.ImportAgentQuoteBatch(ctx, batch)
	if err != nil || first.Inserted != 2 || first.Replayed || len(first.QuoteIDs) != 2 {
		t.Fatalf("first import: %+v %v", first, err)
	}
	preference, err := repository.FXPreference(ctx, household.ID, "USD", "CNY")
	if err != nil || preference.SourceKind != domain.QuoteSourceAgent {
		t.Fatalf("new FX preference: %+v %v", preference, err)
	}
	retry := batch
	retry.CreatedAt = now.Add(time.Hour)
	retry.Records = append([]domain.AgentQuoteRecord(nil), batch.Records...)
	newInstrumentQuote, newFXQuote := instrumentQuote, fxQuote
	newInstrumentQuote.ID, newFXQuote.ID = domain.NewInstrumentQuoteID(), domain.NewFXQuoteID()
	newInstrumentQuote.CreatedAt, newFXQuote.CreatedAt = retry.CreatedAt, retry.CreatedAt
	newInstrumentQuote.FetchedAt, newFXQuote.FetchedAt = retry.CreatedAt, retry.CreatedAt
	retry.Records[0].InstrumentQuote = &newInstrumentQuote
	retry.Records[1].FXQuote = &newFXQuote
	replayed, err := repository.ImportAgentQuoteBatch(ctx, retry)
	if err != nil || !replayed.Replayed || replayed.Inserted != 2 || replayed.QuoteIDs[0] != first.QuoteIDs[0] {
		t.Fatalf("replay: %+v %v", replayed, err)
	}
	changed := retry
	changed.Records = append([]domain.AgentQuoteRecord(nil), retry.Records...)
	changedQuote := newInstrumentQuote
	changedQuote.UnitPrice, _ = domain.ParseUnitPrice("101.125")
	changed.Records[0].InstrumentQuote = &changedQuote
	_, err = repository.ImportAgentQuoteBatch(ctx, changed)
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != domain.ErrConflict {
		t.Fatalf("changed price reused request key: %v", err)
	}
	for name, mutate := range map[string]func(*domain.AgentQuoteBatch){
		"instrument currency": func(candidate *domain.AgentQuoteBatch) { candidate.Records[0].InstrumentQuote.Currency = "EUR" },
		"quote date": func(candidate *domain.AgentQuoteBatch) {
			candidate.Records[0].InstrumentQuote.EffectiveDate = "2026-09-27"
		},
		"FX pair": func(candidate *domain.AgentQuoteBatch) { candidate.Records[1].FXQuote.QuoteCurrency = "EUR" },
		"source":  func(candidate *domain.AgentQuoteBatch) { candidate.Records[0].SourceTitle = "Different source" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := batch
			candidate.Records = append([]domain.AgentQuoteRecord(nil), batch.Records...)
			instrumentCopy, fxCopy := instrumentQuote, fxQuote
			candidate.Records[0].InstrumentQuote = &instrumentCopy
			candidate.Records[1].FXQuote = &fxCopy
			mutate(&candidate)
			_, err := repository.ImportAgentQuoteBatch(ctx, candidate)
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Code != domain.ErrConflict {
				t.Fatalf("changed %s reused request key: %v", name, err)
			}
		})
	}
	correctionQuote := changedQuote
	correctionQuote.ID = domain.NewInstrumentQuoteID()
	correctionQuote.CreatedAt = now.Add(2 * time.Hour)
	corrected, err := repository.ImportAgentQuoteBatch(ctx, domain.AgentQuoteBatch{HouseholdID: household.ID, RequestKey: "agent-batch-2", CreatedAt: now.Add(2 * time.Hour), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteCorrect, InstrumentQuote: &correctionQuote, TargetQuoteID: instrumentQuote.ID.String(), SourceTitle: "Corrected close"}}})
	if err != nil || corrected.Inserted != 1 {
		t.Fatalf("correction: %+v %v", corrected, err)
	}
	quotes, err := repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 || quotes[0].ID != correctionQuote.ID || quotes[0].PriceBasis != "agent_raw_close_v1" {
		t.Fatalf("active corrected quotes: %+v %v", quotes, err)
	}
	snapshot, err := repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || len(snapshot.InstrumentQuotes) != 1 || snapshot.InstrumentQuotes[0].ID != correctionQuote.ID {
		t.Fatalf("snapshot selected withdrawn quote: %+v %v", snapshot.InstrumentQuotes, err)
	}
	_, err = repository.ImportAgentQuoteBatch(ctx, domain.AgentQuoteBatch{HouseholdID: household.ID, RequestKey: "agent-batch-3", CreatedAt: now.Add(3 * time.Hour), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteRetract, TargetQuoteID: correctionQuote.ID.String(), SourceTitle: "Source withdrew close"}}})
	if err != nil {
		t.Fatalf("retraction: %v", err)
	}
	quotes, err = repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 0 {
		t.Fatalf("retracted quote still active: %+v %v", quotes, err)
	}
	audit, err := repository.ListAgentQuoteRecords(ctx, household.ID)
	if err != nil || len(audit) != 4 || audit[0].InstrumentQuote == nil || audit[0].InstrumentQuote.UnitPrice.Canonical() != "100.125" || audit[3].Operation != domain.AgentQuoteRetract {
		t.Fatalf("audit: %+v %v", audit, err)
	}
	export, err := repository.ReadExportSnapshot(ctx)
	if err != nil || len(export.Facts.MarketData["agentQuoteBatches"]) != 3 || len(export.Facts.MarketData["agentQuoteRecords"]) != 4 {
		t.Fatalf("agent quote export: %+v %v", export.Facts.MarketData, err)
	}
}

func TestAgentQuoteBatchRollbackAndEffectiveDateDirty(t *testing.T) {
	database, repository, household, _, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	location, _ := time.LoadLocation("Asia/Singapore")
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, location)
	origin, err := domain.NewHistoryOrigin(household.ID, location.String(), start, start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StartHistory(ctx, domain.HistoryOriginData{Origin: origin}); err != nil {
		t.Fatal(err)
	}
	quotedAt := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC) // Sep 29 in Singapore.
	price, _ := domain.ParseUnitPrice("50")
	quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: quotedAt}, quotedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	quote.ObservationKind, quote.EffectiveDate = "close", "2026-09-28"
	bad := quote
	bad.ID, bad.InstrumentID = domain.NewInstrumentQuoteID(), domain.NewInstrumentID()
	batch := domain.AgentQuoteBatch{HouseholdID: household.ID, RequestKey: "atomic-failure", CreatedAt: quotedAt.Add(time.Hour), Records: []domain.AgentQuoteRecord{
		{Operation: domain.AgentQuoteAppend, InstrumentQuote: &quote, SourceTitle: "Close"},
		{Operation: domain.AgentQuoteAppend, InstrumentQuote: &bad, SourceTitle: "Bad target"},
	}}
	if _, err := repository.ImportAgentQuoteBatch(ctx, batch); err == nil {
		t.Fatal("batch with missing instrument succeeded")
	}
	var count int
	if err := database.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_quote_batches WHERE request_key = ?`, batch.RequestKey).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial receipt: %d %v", count, err)
	}
	if err := database.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM instrument_quotes WHERE id = ?`, quote.ID.String()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial quote: %d %v", count, err)
	}
	batch.RequestKey, batch.Records = "cross-timezone", batch.Records[:1]
	if _, err := repository.ImportAgentQuoteBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	var dirtyFrom string
	if err := database.SQL.QueryRowContext(ctx, `SELECT dirty_from FROM history_snapshot_state WHERE household_id = ?`, household.ID.String()).Scan(&dirtyFrom); err != nil || dirtyFrom != "2026-09-28" {
		t.Fatalf("dirty range began %q, want effective date: %v", dirtyFrom, err)
	}
}

func TestV14ToV15PreservesQuoteSourceData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v14-agent-migration.db")
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(legacyV14QuoteSourceSchema(string(schema)), "PRAGMA user_version = 15", "PRAGMA user_version = 14", 1)
	seed, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	const h = "00000000-0000-4000-8000-000000000001"
	const i = "00000000-0000-4000-8000-000000000002"
	const q = "00000000-0000-4000-8000-000000000003"
	const stamp = "2026-09-28T00:00:00Z"
	for _, statement := range []string{
		`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('` + h + `','Home','USD','` + stamp + `','` + stamp + `')`,
		`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,quote_source,created_at,updated_at) VALUES('` + i + `','` + h + `','Fund','etf','USD','investment','manual','` + stamp + `','` + stamp + `')`,
		`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES('` + q + `','` + i + `','123.45','USD','manual','manual','` + stamp + `','` + stamp + `')`,
	} {
		if _, err := seed.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if err := opened.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	var price string
	if err := opened.SQL.QueryRow(`SELECT unit_price FROM instrument_quotes WHERE id = ?`, q).Scan(&price); err != nil || price != "123.45" {
		t.Fatalf("migrated quote: %q %v", price, err)
	}
	if _, err := opened.SQL.Exec(`UPDATE instruments SET quote_source = 'agent' WHERE id = ?`, i); err != nil {
		t.Fatalf("agent source CHECK not widened: %v", err)
	}
	if _, err := opened.SQL.Exec(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES(?,?,'125','USD','agent','agent',?,?)`, domain.NewInstrumentQuoteID().String(), i, stamp, stamp); err != nil {
		t.Fatalf("agent quote CHECK not widened: %v", err)
	}
	if err := opened.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}
