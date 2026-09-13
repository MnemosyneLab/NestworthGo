package marketdata

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestTiingoCompleteFixturePersistsClosesCoverageAndDirtyGeneration(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	result, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, fetchedAt))
	if err != nil {
		t.Fatal(err)
	}
	if result.PersistedObservations != 3 || result.CanonicalSlots != 3 || result.Unchanged {
		t.Fatalf("result = %+v", result)
	}
	if result.CoverageDays < 3 {
		t.Fatalf("coverage days = %d, want weekend no-observation plus pending", result.CoverageDays)
	}

	var quotes int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ? AND observation_kind = 'close'`, instrument.ID.String()).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if quotes != 3 {
		t.Fatalf("close quotes = %d", quotes)
	}
	var slots int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_observation_slots WHERE instrument_id = ?`, instrument.ID.String()).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if slots != 3 {
		t.Fatalf("canonical slots = %d", slots)
	}
	var noData, pending int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM market_data_day_status WHERE target_id = ? AND status = 'no_observation'`, instrument.ID.String()).Scan(&noData); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM market_data_day_status WHERE target_id = ? AND status = 'pending'`, instrument.ID.String()).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if noData != 2 {
		t.Fatalf("no_observation days = %d, want 2026-09-05 and 2026-09-06", noData)
	}
	if pending != 1 {
		t.Fatalf("pending days = %d, want 2026-09-09", pending)
	}
	var bindingSymbol string
	var bindingRevision int
	if err := database.SQL.QueryRow(`SELECT provider_symbol, binding_revision FROM instrument_provider_bindings WHERE instrument_id = ? AND provider_key = ?`, instrument.ID.String(), application.TiingoProviderKey).Scan(&bindingSymbol, &bindingRevision); err != nil {
		t.Fatal(err)
	}
	if bindingSymbol != "AAPL" || bindingRevision != 1 {
		t.Fatalf("binding symbol=%s revision=%d", bindingSymbol, bindingRevision)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.InputGeneration == 0 || state.DirtyFrom == nil || *state.DirtyFrom == "" || state.ResolverPolicyVersion != domain.MarketDataResolverPolicy {
		t.Fatalf("dirty state = %+v", state)
	}
	if state.DirtyTo == nil || *state.DirtyTo != "2026-09-09" {
		t.Fatalf("dirty_to = %v, want last closed Singapore day 2026-09-09 (upper bound, not earliest market date)", state.DirtyTo)
	}

	again, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, fetchedAt))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Unchanged || again.NewRevisions != 0 {
		t.Fatalf("identical retry created revisions: %+v", again)
	}
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ? AND observation_kind = 'close'`, instrument.ID.String()).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if quotes != 3 {
		t.Fatalf("retry duplicated quotes: %d", quotes)
	}
}

func TestTiingoCorrectionCreatesImmutableRevisionAndUpdatesSlot(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	completeMeta, completeBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	complete, err := QualifyTiingoHistory(completeMeta, completeBody)
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, complete, fetchedAt)); err != nil {
		t.Fatal(err)
	}
	correctionMeta, correctionBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-correction.json")
	correction, err := QualifyTiingoHistory(correctionMeta, correctionBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, correction, fetchedAt.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	var quoteCount int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ? AND effective_date = '2026-09-08'`, instrument.ID.String()).Scan(&quoteCount); err != nil {
		t.Fatal(err)
	}
	if quoteCount != 2 {
		t.Fatalf("sep 8 quotes = %d, want original plus correction", quoteCount)
	}
	var canonical, superseded sql.NullString
	var revision int
	if err := database.SQL.QueryRow(`
		SELECT q.unit_price, q.supersedes_quote_id, q.revision
		FROM instrument_observation_slots s
		JOIN instrument_quotes q ON q.id = s.quote_id
		WHERE s.instrument_id = ? AND s.market_date = '2026-09-08' AND s.observation_kind = 'close'`, instrument.ID.String()).Scan(&canonical, &superseded, &revision); err != nil {
		t.Fatal(err)
	}
	if canonical.String != "185.2500037" || !superseded.Valid || revision != 2 {
		t.Fatalf("canonical correction price=%s supersedes=%v revision=%d", canonical.String, superseded, revision)
	}
}

func TestTiingoFailClosedBatchesCommitNothing(t *testing.T) {
	ctx := context.Background()
	cases := []string{
		"providers/tiingo/aapl-eod-adjclose-only.json",
		"providers/tiingo/cn-equity-unsupported.json",
		"providers/tiingo/aapl-eod-malformed.json",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			database, repo, household, instrument := seedHistoryWorkspace(t)
			meta, body := mustLoadVNext(t, path)
			outcome, err := QualifyTiingoHistory(meta, body)
			if err != nil {
				t.Fatal(err)
			}
			if err := application.RefuseFailClosedInstrumentHistory(outcome); err == nil {
				t.Fatal("expected fail-closed mapping")
			}
			_, persistErr := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)))
			if persistErr == nil {
				t.Fatal("fail-closed batch was persisted")
			}
			var domainErr *domain.Error
			if !errors.As(persistErr, &domainErr) || domainErr.Code != domain.ErrValidation {
				t.Fatalf("persist error = %v", persistErr)
			}
			var quotes, slots, coverage int
			if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ?`, instrument.ID.String()).Scan(&quotes); err != nil {
				t.Fatal(err)
			}
			if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_observation_slots WHERE instrument_id = ?`, instrument.ID.String()).Scan(&slots); err != nil {
				t.Fatal(err)
			}
			if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM market_data_day_status WHERE target_id = ?`, instrument.ID.String()).Scan(&coverage); err != nil {
				t.Fatal(err)
			}
			if quotes != 0 || slots != 0 || coverage != 0 {
				t.Fatalf("fail-closed persist leaked quotes=%d slots=%d coverage=%d", quotes, slots, coverage)
			}
			state, err := repo.DailySnapshotState(ctx, household.ID)
			if err != nil {
				t.Fatal(err)
			}
			if state.InputGeneration != 0 {
				t.Fatalf("generation bumped on fail-closed persist: %d", state.InputGeneration)
			}
		})
	}
}

func TestUncertainTruncatedBatchPersistsNoNegativeCache(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-truncated.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != application.MappingUncertain {
		t.Fatalf("status = %s", outcome.Status)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	var noData int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM market_data_day_status WHERE target_id = ? AND status = 'no_observation'`, instrument.ID.String()).Scan(&noData); err != nil {
		t.Fatal(err)
	}
	if noData != 0 {
		t.Fatal("truncated/uncertain batch wrote verified no-observation coverage")
	}
}

func TestPersistDirtyFromUsesMarketDateWhenUTCCalendarDiffers(t *testing.T) {
	ctx := context.Background()
	_, repo, household, instrument := seedHistoryWorkspace(t)
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	commit := sqlite.InstrumentHistoryCommit{
		HouseholdID:    household.ID,
		InstrumentID:   instrument.ID,
		ProviderKey:    application.TiingoProviderKey,
		ProviderSymbol: "AAPL",
		QuoteCurrency:  "USD",
		Market:         "US",
		Status:         string(application.MappingMapped),
		Adapter:        "tiingo_eod",
		SourcePolicy:   string(application.PriceBasisTiingoRawClose),
		FetchedAt:      fetchedAt,
		Observations: []sqlite.InstrumentHistoryObservation{{
			MarketDate:       "2026-09-08",
			Value:            "185.25",
			Currency:         "USD",
			ValueEffectiveAt: time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC),
			Kind:             string(application.InstrumentObservationClose),
			PriceBasis:       string(application.PriceBasisTiingoRawClose),
		}},
	}
	if _, err := repo.CommitInstrumentHistory(ctx, commit); err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-08" {
		t.Fatalf("dirty_from = %v, want market date 2026-09-08 not UTC 2026-09-09", state.DirtyFrom)
	}
	if state.DirtyTo == nil || *state.DirtyTo != "2026-09-09" {
		t.Fatalf("dirty_to = %v, want last closed local day", state.DirtyTo)
	}
}

func TestLatestManualQuotesKeepWorkingAfterSchema10(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "latest.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repo := sqlite.NewRepository(database)
	service := application.NewService(repo)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Latest", BaseCurrency: "SGD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Manual", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "185.25", "2026-09-10T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := database.SQL.QueryRow(`SELECT observation_kind FROM instrument_quotes WHERE instrument_id = ?`, instrument.ID.String()).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != string(application.InstrumentObservationManual) {
		t.Fatalf("manual quote kind = %s", kind)
	}
	quotes, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 || quotes[0].UnitPrice.Canonical() != "185.25" {
		t.Fatalf("latest list = %+v err=%v", quotes, err)
	}

	if _, err := service.AppendManualFXQuote(ctx, "USD", "SGD", "1.35", "2026-09-10T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var fxKind string
	if err := database.SQL.QueryRow(`SELECT observation_kind FROM fx_quotes WHERE source_kind = 'manual'`).Scan(&fxKind); err != nil {
		t.Fatal(err)
	}
	if fxKind != string(application.FXObservationManual) {
		t.Fatalf("manual FX kind = %s", fxKind)
	}

	providerInstrument, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Yahoo", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	price, err := domain.ParseUnitPrice("185.25")
	if err != nil {
		t.Fatal(err)
	}
	quotedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	providerQuote, err := domain.NewInstrumentQuote(providerInstrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceProvider, SourceKey: application.YahooFinanceProviderKey, QuotedAt: quotedAt, Delayed: true}, quotedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendInstrumentQuote(ctx, providerQuote); err != nil {
		t.Fatal(err)
	}
	var realtimeKind string
	if err := database.SQL.QueryRow(`SELECT observation_kind FROM instrument_quotes WHERE instrument_id = ?`, providerInstrument.ID.String()).Scan(&realtimeKind); err != nil {
		t.Fatal(err)
	}
	if realtimeKind != string(application.InstrumentObservationRealtime) {
		t.Fatalf("provider latest kind = %s, want realtime not close", realtimeKind)
	}
}

func TestFrankfurterHistoryPersistsDailyReferenceSlots(t *testing.T) {
	ctx := context.Background()
	_, repo, household, _ := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/frankfurter/usd-sgd-history.json")
	outcome, err := QualifyFrankfurterHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repo.CommitFXHistory(ctx, fxCommit(household, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if result.PersistedObservations != 3 || result.CanonicalSlots != 3 {
		t.Fatalf("FX result = %+v", result)
	}
}

func TestCanonicalInstrumentCloseReconcilesPendingCoverageIncludingIdempotentWrite(t *testing.T) {
	ctx := context.Background()
	_, repo, household, instrument := seedHistoryWorkspace(t)
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	pending := sqlite.InstrumentHistoryCommit{
		HouseholdID: household.ID, InstrumentID: instrument.ID, ProviderKey: application.TiingoProviderKey,
		ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US", Status: string(application.MappingMapped),
		Reason: "pending_publication", Adapter: "tiingo_eod", SourcePolicy: string(application.PriceBasisTiingoRawClose),
		FetchedAt: fetchedAt, PendingRanges: []sqlite.DateSpan{{Start: "2026-09-08", End: "2026-09-08"}},
	}
	if _, err := repo.CommitInstrumentHistory(ctx, pending); err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-08" {
		t.Fatalf("pending instrument status did not dirty the affected date: %+v", state)
	}
	coverage, err := repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 1 || !containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("pending instrument coverage = %+v", coverage)
	}
	canonical := pending
	canonical.PendingRanges = nil
	canonical.Observations = []sqlite.InstrumentHistoryObservation{{
		MarketDate: "2026-09-08", Value: "185.25", Currency: "USD",
		ValueEffectiveAt: time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC),
		Kind:             string(application.InstrumentObservationClose), PriceBasis: string(application.PriceBasisTiingoRawClose),
		TimestampBasis: string(application.TimestampBasisSessionClose),
	}}
	if _, err := repo.CommitInstrumentHistory(ctx, canonical); err != nil {
		t.Fatal(err)
	}
	coverage, err = repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 1 || !containsDate(coverage[0].CloseMarketDates, "2026-09-08") || containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("reconciled instrument coverage = %+v", coverage)
	}

	// Recreate an obsolete pending row after the identical canonical observation;
	// the next idempotent write must still remove it and advance dirty state.
	if _, err := repo.CommitInstrumentHistory(ctx, pending); err != nil {
		t.Fatal(err)
	}
	before, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repo.CommitInstrumentHistory(ctx, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Unchanged || retry.PersistedObservations != 0 {
		t.Fatalf("idempotent reconciliation result = %+v", retry)
	}
	after, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.InputGeneration <= before.InputGeneration {
		t.Fatalf("status reconciliation did not advance generation: before=%+v after=%+v", before, after)
	}
	coverage, err = repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("idempotent canonical write left pending coverage: %+v", coverage[0])
	}
}

func TestCanonicalFXReferenceReconcilesPendingCoverageIncludingIdempotentWrite(t *testing.T) {
	ctx := context.Background()
	_, repo, household, _ := seedHistoryWorkspace(t)
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	pending := sqlite.FXHistoryCommit{
		HouseholdID: household.ID, ProviderKey: application.FrankfurterProviderKey, BaseCurrency: "USD", QuoteCurrency: "SGD",
		Status: string(application.MappingMapped), Reason: "pending_publication", Adapter: "frankfurter_v2",
		SourcePolicy: domain.FrankfurterV2BlendedPolicy, FetchedAt: fetchedAt,
		PendingRanges: []sqlite.DateSpan{{Start: "2026-09-08", End: "2026-09-08"}},
	}
	if _, err := repo.CommitFXHistory(ctx, pending); err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-08" {
		t.Fatalf("pending FX status did not dirty the affected date: %+v", state)
	}
	coverage, err := repo.ListFXHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 1 || !containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("pending FX coverage = %+v", coverage)
	}
	canonical := pending
	canonical.PendingRanges = nil
	canonical.Observations = []sqlite.FXHistoryObservation{{
		MarketDate: "2026-09-08", Rate: "1.35", BaseCurrency: "USD", QuoteCurrency: "SGD",
		ValueEffectiveAt: time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC), Kind: string(application.FXObservationDailyReference), TimestampBasis: string(application.TimestampBasisPolicyDerived),
	}}
	if _, err := repo.CommitFXHistory(ctx, canonical); err != nil {
		t.Fatal(err)
	}
	coverage, err = repo.ListFXHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 1 || !containsDate(coverage[0].DailyReferenceDates, "2026-09-08") || containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("reconciled FX coverage = %+v", coverage)
	}
	if _, err := repo.CommitFXHistory(ctx, pending); err != nil {
		t.Fatal(err)
	}
	retry, err := repo.CommitFXHistory(ctx, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Unchanged || retry.PersistedObservations != 0 {
		t.Fatalf("idempotent FX reconciliation result = %+v", retry)
	}
	coverage, err = repo.ListFXHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if containsDate(coverage[0].UnverifiedDates, "2026-09-08") {
		t.Fatalf("idempotent canonical FX write left pending coverage: %+v", coverage[0])
	}
}

func TestHistoricalCoverageRetainsTheEffectiveOlderBindingRevision(t *testing.T) {
	ctx := context.Background()
	_, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}

	service := application.NewService(repo)
	if _, err := service.UpdateInstrument(ctx, instrument.ID, application.InstrumentInput{ProviderSymbol: "MSFT"}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].ProviderSymbol != "MSFT" || current[0].BindingRevision != 2 || len(current[0].CloseMarketDates) != 0 {
		t.Fatalf("current-route coverage = %+v", current)
	}
	historical, err := repo.ListHistoricalInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	var foundOlder bool
	for _, item := range historical {
		if item.ProviderKey == application.TiingoProviderKey && item.ProviderSymbol == "AAPL" && item.BindingRevision == 1 && containsDate(item.CloseMarketDates, "2026-09-04") {
			foundOlder = true
		}
	}
	if !foundOlder {
		t.Fatalf("historical coverage lost the older effective binding: %+v", historical)
	}
}

func TestHistoricalCoverageSurvivesSwitchToManual(t *testing.T) {
	ctx := context.Background()
	_, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repo)
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatal(err)
	}

	current, err := repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 0 {
		t.Fatalf("current-route coverage after manual switch = %+v, want none", current)
	}
	historical, err := repo.ListHistoricalInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	var foundProvider bool
	for _, item := range historical {
		if item.InstrumentID == instrument.ID && item.ProviderKey == application.TiingoProviderKey && containsDate(item.CloseMarketDates, "2026-09-04") {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("historical coverage lost provider closes after manual switch: %+v", historical)
	}
}

func containsDate(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestHistoryCommitIsAtomicOnInvalidObservation(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	commit := instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))
	commit.Observations = append(commit.Observations, sqlite.InstrumentHistoryObservation{
		MarketDate: "2026-09-08",
		Value:      "not-a-decimal",
		Currency:   "USD",
		Kind:       string(application.InstrumentObservationClose),
		PriceBasis: string(application.PriceBasisTiingoRawClose),
	})
	if _, err := repo.CommitInstrumentHistory(ctx, commit); err == nil {
		t.Fatal("poisoned batch committed")
	}
	var quotes, slots, coverage int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ?`, instrument.ID.String()).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_observation_slots WHERE instrument_id = ?`, instrument.ID.String()).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM market_data_day_status WHERE target_id = ?`, instrument.ID.String()).Scan(&coverage); err != nil {
		t.Fatal(err)
	}
	if quotes != 0 || slots != 0 || coverage != 0 {
		t.Fatalf("partial persist leaked quotes=%d slots=%d coverage=%d", quotes, slots, coverage)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.InputGeneration != 0 || state.DirtyFrom != nil {
		t.Fatalf("invalidation escaped rollback: %+v", state)
	}
}

func seedHistoryWorkspace(t *testing.T) (*sqlite.DB, *sqlite.Repository, domain.Household, domain.Instrument) {
	t.Helper()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repo := sqlite.NewRepository(database)
	service := application.NewService(repo)
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Persist", BaseCurrency: "SGD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil || bootstrap.Household == nil {
		t.Fatalf("bootstrap: %v", err)
	}
	location, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 6, 0, 0, 0, 0, location)
	origin, err := domain.NewHistoryOrigin(bootstrap.Household.ID, "Asia/Singapore", started, started)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartHistory(ctx, domain.HistoryOriginData{Origin: origin}); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Apple", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: application.TiingoProviderKey, ProviderSymbol: "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	return database, repo, *bootstrap.Household, instrument
}

func instrumentCommit(household domain.Household, instrument domain.Instrument, outcome application.MappingOutcome[application.InstrumentDailyObservation], fetchedAt time.Time) sqlite.InstrumentHistoryCommit {
	commit := sqlite.InstrumentHistoryCommit{
		HouseholdID:    household.ID,
		InstrumentID:   instrument.ID,
		ProviderKey:    application.TiingoProviderKey,
		ProviderSymbol: "AAPL",
		QuoteCurrency:  "USD",
		Market:         "US",
		Status:         string(outcome.Status),
		Reason:         outcome.Reason,
		Adapter:        outcome.Batch.Evidence.Adapter,
		SourcePolicy:   application.SourcePolicyVersion(outcome.Batch.Evidence),
		FetchedAt:      fetchedAt,
		NextCheckAt:    outcome.Batch.NextCheckAt,
	}
	for _, observation := range outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.InstrumentHistoryObservation{
			MarketDate:        string(observation.MarketDate),
			Value:             observation.Value,
			Currency:          observation.Currency,
			ValueEffectiveAt:  observation.ValueEffectiveAt,
			ProviderTimestamp: observation.ProviderTimestamp,
			Kind:              string(observation.Kind),
			PriceBasis:        string(observation.PriceBasis),
			TimestampBasis:    string(observation.TimestampBasis),
			SplitFactor:       observation.SplitFactor,
			DividendCash:      observation.DividendCash,
		})
	}
	for _, rng := range outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	for _, rng := range outcome.Batch.PendingRanges {
		commit.PendingRanges = append(commit.PendingRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	return commit
}

func fxCommit(household domain.Household, outcome application.MappingOutcome[application.FXDailyObservation], fetchedAt time.Time) sqlite.FXHistoryCommit {
	commit := sqlite.FXHistoryCommit{
		HouseholdID:   household.ID,
		ProviderKey:   application.FrankfurterProviderKey,
		BaseCurrency:  "USD",
		QuoteCurrency: "SGD",
		Status:        string(outcome.Status),
		Reason:        outcome.Reason,
		Adapter:       outcome.Batch.Evidence.Adapter,
		SourcePolicy:  application.SourcePolicyVersion(outcome.Batch.Evidence),
		FetchedAt:     fetchedAt,
		NextCheckAt:   outcome.Batch.NextCheckAt,
	}
	for _, observation := range outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.FXHistoryObservation{
			MarketDate:       string(observation.MarketDate),
			Rate:             observation.Rate,
			BaseCurrency:     observation.BaseCurrency,
			QuoteCurrency:    observation.QuoteCurrency,
			ValueEffectiveAt: observation.ValueEffectiveAt,
			Kind:             string(observation.Kind),
			TimestampBasis:   string(observation.TimestampBasis),
		})
	}
	for _, rng := range outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	for _, rng := range outcome.Batch.PendingRanges {
		commit.PendingRanges = append(commit.PendingRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	return commit
}

func TestNoObservationExpiryIsPersisted(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, fetchedAt)); err != nil {
		t.Fatal(err)
	}
	var expires sql.NullString
	if err := database.SQL.QueryRow(`SELECT expires_at FROM market_data_day_status WHERE target_id = ? AND effective_date = '2026-09-05' AND status = 'no_observation'`, instrument.ID.String()).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if !expires.Valid || expires.String == "" {
		t.Fatal("no_observation expires_at was left NULL")
	}
	coverage, err := repo.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil || len(coverage) != 1 {
		t.Fatalf("coverage = %+v err=%v", coverage, err)
	}
	if coverage[0].NoObservationExpiresAt["2026-09-05"].IsZero() {
		t.Fatal("coverage omitted no-observation expiry")
	}
}

func TestYahooHistoryIsNotPersisted(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/yahoo/aapl-history-split.json")
	outcome, err := QualifyYahooHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.RefuseFailClosedInstrumentHistory(outcome); err == nil {
		t.Fatal("Yahoo unverified close was accepted")
	}
	commit := instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))
	commit.ProviderKey = application.YahooFinanceProviderKey
	commit.Adapter = "yahoo_chart"
	if _, err := repo.CommitInstrumentHistory(ctx, commit); err == nil {
		t.Fatal("Yahoo history batch was persisted")
	}
	var quotes int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_quotes WHERE instrument_id = ?`, instrument.ID.String()).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if quotes != 0 {
		t.Fatalf("Yahoo close leaked: %d", quotes)
	}
}

func TestFailedRecheckPreservesExistingClose(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, fetchedAt)); err != nil {
		t.Fatal(err)
	}
	badMeta, badBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-malformed.json")
	bad, err := QualifyTiingoHistory(badMeta, badBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, bad, fetchedAt.Add(time.Minute))); err == nil {
		t.Fatal("malformed Force Recheck was persisted")
	}
	var closePrice string
	if err := database.SQL.QueryRow(`SELECT q.unit_price FROM instrument_observation_slots s JOIN instrument_quotes q ON q.id = s.quote_id WHERE s.instrument_id = ? AND s.market_date = '2026-09-04'`, instrument.ID.String()).Scan(&closePrice); err != nil {
		t.Fatal(err)
	}
	if closePrice != "185.25" {
		t.Fatalf("failed recheck overwrote close: %s", closePrice)
	}
}

func TestProviderModeChangeKeepsPriorProviderSlots(t *testing.T) {
	ctx := context.Background()
	database, repo, household, instrument := seedHistoryWorkspace(t)
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-split.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, outcome, time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	var split string
	if err := database.SQL.QueryRow(`SELECT split_factor FROM instrument_quotes WHERE instrument_id = ? AND observation_kind = 'close'`, instrument.ID.String()).Scan(&split); err != nil {
		t.Fatal(err)
	}
	if split != "4.0" {
		t.Fatalf("split_factor = %s", split)
	}
	service := application.NewService(repo)
	if _, err := service.UpdateInstrument(ctx, instrument.ID, application.InstrumentInput{
		Name: "Apple", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "AAPL", MarketCode: "US",
	}); err != nil {
		t.Fatal(err)
	}
	var tiingoSlots, yahooSlots int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_observation_slots WHERE instrument_id = ? AND provider_key = ?`, instrument.ID.String(), application.TiingoProviderKey).Scan(&tiingoSlots); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM instrument_observation_slots WHERE instrument_id = ? AND provider_key = ?`, instrument.ID.String(), application.YahooFinanceProviderKey).Scan(&yahooSlots); err != nil {
		t.Fatal(err)
	}
	if tiingoSlots == 0 {
		t.Fatal("switching to Yahoo deleted Tiingo historical slots")
	}
	if yahooSlots != 0 {
		t.Fatal("Yahoo unverified closes were synthesized after a mode change")
	}
}
