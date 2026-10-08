package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func newAgentMarketDataFixture(t *testing.T) (*Service, domain.Instrument, func(time.Time)) {
	t.Helper()
	service, ctx, _, setClock := newOnboardedService(t, "agent-market-data", []string{"Owner"})
	setClock(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "Private Fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, instrument, setClock
}

func agentNAV(instrument domain.Instrument, value, date string) AgentMarketDataItem {
	return AgentMarketDataItem{
		InstrumentID: instrument.ID.String(), Currency: "USD", Value: value,
		Kind: "nav", Date: date, Timezone: "Asia/Singapore", SourceTitle: "Fund issuer NAV",
	}
}

func TestAgentMarketDataDateBoundaryAndAtomicValidation(t *testing.T) {
	t.Parallel()
	service, instrument, _ := newAgentMarketDataFixture(t)
	ctx := t.Context()
	nav := agentNAV(instrument, "1.0197", "2026-09-28")
	nav.SplitFactor, nav.DividendCash, nav.Delayed = "2", "0.05", true
	result, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{nav}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 || result.Replayed || result.SnapshotStatus != "not_started" {
		t.Fatalf("unexpected import receipt: %+v", result)
	}
	quote, err := service.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if quote == nil || quote.SourceKind != domain.QuoteSourceAgent || quote.EffectiveDate != "2026-09-28" || quote.TimestampBasis != "date_label" {
		t.Fatalf("daily NAV lost its source date/basis: %+v", quote)
	}
	if want := time.Date(2026, 9, 27, 16, 0, 0, 0, time.UTC); !quote.QuotedAt.Equal(want) {
		t.Fatalf("date anchor = %s, want %s", quote.QuotedAt, want)
	}
	if quote.SplitFactor != "2" || quote.DividendCash != "0.05" || !quote.Delayed {
		t.Fatalf("source metadata was lost: %+v", quote)
	}
	if quote.QuotedAt.Equal(quote.CreatedAt) {
		t.Fatal("a daily NAV was stamped with its import time")
	}

	invalid := []AgentMarketDataItem{
		{InstrumentID: instrument.ID.String(), Currency: "USD", Value: "2", Kind: "latest", SourceTitle: "Issuer"},
		{InstrumentID: instrument.ID.String(), Currency: "USD", Value: "2", Kind: "latest", QuotedAt: "2026-09-30T00:00:00Z", SourceTitle: "Issuer"},
		agentNAV(instrument, "2", "2026-10-01"),
	}
	for index, item := range invalid {
		if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); !hasDomainCode(err, domain.ErrValidation) {
			t.Fatalf("invalid item %d accepted: %v", index, err)
		}
	}
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{
		agentNAV(instrument, "1.03", "2026-09-27"), agentNAV(instrument, "1.04", "2026-10-01"),
	}}); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("invalid batch accepted: %v", err)
	}
	records, err := service.AgentMarketDataRecords(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("invalid batch changed saved records: %d, %v", len(records), err)
	}
}

func TestAgentMarketDataZeroPriceRemainsAnAvailableObservation(t *testing.T) {
	t.Parallel()
	service, instrument, _ := newAgentMarketDataFixture(t)
	ctx := t.Context()
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{
		agentNAV(instrument, "0", "2026-09-28"),
	}}); err != nil {
		t.Fatal(err)
	}
	quote, err := service.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil || quote == nil || quote.UnitPrice.Canonical() != "0" {
		t.Fatalf("zero quote was mistaken for missing data: %+v, %v", quote, err)
	}
}

func TestAgentMarketDataCorrectionRetractionAndSourceOnlyHistory(t *testing.T) {
	t.Parallel()
	service, instrument, setClock := newAgentMarketDataFixture(t)
	ctx := t.Context()
	first := agentNAV(instrument, "1.0197", "2026-09-28")
	key := uuid.NewString()
	created, err := service.ImportAgentMarketData(ctx, key, AgentMarketDataInput{Items: []AgentMarketDataItem{first}})
	if err != nil {
		t.Fatal(err)
	}
	setClock(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	replayed, err := service.ImportAgentMarketData(ctx, key, AgentMarketDataInput{Items: []AgentMarketDataItem{first}})
	if err != nil || !replayed.Replayed || len(replayed.QuoteIDs) != 1 || replayed.QuoteIDs[0] != created.QuoteIDs[0] {
		t.Fatalf("identical retry did not return the original receipt: %+v, %v", replayed, err)
	}
	correction := agentNAV(instrument, "1.0201", "2026-09-28")
	correction.Operation, correction.TargetQuoteID = "correct", created.QuoteIDs[0]
	corrected, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := service.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil || quote == nil || quote.UnitPrice.Canonical() != "1.0201" {
		t.Fatalf("correction did not become current: %+v, %v", quote, err)
	}
	_, err = service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		Operation: "retract", TargetQuoteID: corrected.QuoteIDs[0], SourceTitle: "Issuer withdrew corrected NAV",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	quote, err = service.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil || quote != nil {
		t.Fatalf("withdrawn Agent-only NAV remained current: %+v, %v", quote, err)
	}
	series, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.TrendAllTime, domain.QuoteSourceFilterAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Observations) != 0 {
		t.Fatalf("withdrawn NAV remained in usable local history: %+v", series.Observations)
	}
	records, err := service.AgentMarketDataRecords(ctx)
	if err != nil || len(records) != 3 {
		t.Fatalf("correction audit trail incomplete: %d, %v", len(records), err)
	}
}

func TestAgentMarketDataReplaySurvivesInstrumentArchive(t *testing.T) {
	t.Parallel()
	service, instrument, _ := newAgentMarketDataFixture(t)
	ctx := t.Context()
	input := AgentMarketDataInput{Items: []AgentMarketDataItem{agentNAV(instrument, "1.0197", "2026-09-28")}}
	key := uuid.NewString()
	first, err := service.ImportAgentMarketData(ctx, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.ImportAgentMarketData(ctx, key, input)
	if err != nil || !replayed.Replayed || len(replayed.QuoteIDs) != 1 || replayed.QuoteIDs[0] != first.QuoteIDs[0] {
		t.Fatalf("archiving a target broke durable retry: %+v, %v", replayed, err)
	}
}
