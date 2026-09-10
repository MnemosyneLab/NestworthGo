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
	return commit
}
