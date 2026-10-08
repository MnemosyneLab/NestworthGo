package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Reference orchestration from main ae84b305. Keep its two independent reads
// so regressions exercise the reuse boundary rather than duplicate new code.
func legacyPlanHistorySync(s *Service, ctx context.Context, opts HistorySyncOptions) (HistoryRepairPlan, error) {
	plan, err := s.PlanMarketDataRepair(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	plan.ForceRecheck = opts.ForceRecheck
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	coverage, err := s.repository.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	var agentDates map[domain.InstrumentID][]string
	if !opts.ForceRecheck {
		var agentErr error
		agentDates, agentErr = s.agentInstrumentDailyDates(ctx)
		if agentErr != nil {
			return HistoryRepairPlan{}, agentErr
		}
		for index := range coverage {
			coverage[index].CloseMarketDates = append(coverage[index].CloseMarketDates, agentDates[coverage[index].InstrumentID]...)
		}
	}
	byID := make(map[domain.InstrumentID]domain.InstrumentHistoryCoverage, len(coverage))
	for _, item := range coverage {
		byID[item.InstrumentID] = item
	}
	now := s.clock()
	for index, need := range plan.Instruments {
		if need.LatestOnly {
			continue
		}
		item := byID[need.InstrumentID]
		lastFinalized := need.LastFinalizedMarketDate
		if lastFinalized == "" {
			lastFinalized = plan.LastFinalizedMarketDate
		}
		enriched, enrichErr := applyHistorySyncPolicy(need, item, lastFinalized, now, opts.ForceRecheck)
		if enrichErr != nil {
			return HistoryRepairPlan{}, enrichErr
		}
		if !opts.ForceRecheck {
			enriched.FetchRanges, enrichErr = excludeExactDates(enriched.FetchRanges, indexStrings(agentDates[need.InstrumentID]))
			if enrichErr != nil {
				return HistoryRepairPlan{}, enrichErr
			}
		}
		if len(enriched.MissingRanges) == 0 && !enriched.OpeningAnchorMissing && len(enriched.FetchRanges) == 0 {
			enriched.RouteStatus = domain.InstrumentRouteOK
			enriched.SkipReason = ""
		}
		plan.Instruments[index] = s.applyInstrumentHistoryCapability(enriched)
	}
	return plan, nil
}

type historyReadCounter struct {
	Repository
	coverage, quotes int
	failure          error
}

func (r *historyReadCounter) ListInstrumentHistoryCoverage(ctx context.Context, id domain.HouseholdID) ([]domain.InstrumentHistoryCoverage, error) {
	r.coverage++
	if r.failure != nil {
		return nil, r.failure
	}
	return r.Repository.ListInstrumentHistoryCoverage(ctx, id)
}

func (r *historyReadCounter) ListInstrumentQuotes(ctx context.Context, id domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	r.quotes++
	return r.Repository.ListInstrumentQuotes(ctx, id)
}

func assertHistoryPlanMatchesLegacy(t *testing.T, s *Service) {
	t.Helper()
	ctx := context.Background()
	for _, force := range []bool{false, true} {
		opts := HistorySyncOptions{ForceRecheck: force}
		want, wantErr := legacyPlanHistorySync(s, ctx, opts)
		got, gotErr := s.PlanHistorySync(ctx, opts)
		if wantErr != nil || gotErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("force=%v plan differs\ngot: %+v %v\nwant: %+v %v", force, got, gotErr, want, wantErr)
		}
	}
}

func TestHistoryPlanReusesCoverageAndQuotes(t *testing.T) {
	t.Parallel()
	s, _ := marketReadFixture(t, 50, 8)
	repo := s.repository
	ctx := context.Background()
	for _, force := range []bool{false, true} {
		counter := &historyReadCounter{Repository: repo}
		s.repository = counter
		if _, err := s.PlanHistorySync(ctx, HistorySyncOptions{ForceRecheck: force}); err != nil {
			t.Fatal(err)
		}
		if counter.coverage != 1 || counter.quotes != 50 {
			t.Fatalf("force=%v coverage=%d quotes=%d want 1/50", force, counter.coverage, counter.quotes)
		}
	}
	s.repository = repo
	assertHistoryPlanMatchesLegacy(t, s)
}

func TestHistoryPlanReuseRetainsOverridesAndFreshReads(t *testing.T) {
	t.Parallel()
	s, repo, first, second := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, &syncFakeProvider{key: YahooFinanceProviderKey})
	ctx := context.Background()
	// Include observed and negative coverage, then an Agent override on the
	// provider instrument. Ordinary and forced plans must retain their policies.
	fetched := s.clock().Add(-48 * time.Hour)
	commit := sqlite.InstrumentHistoryCommit{HouseholdID: first.HouseholdID, InstrumentID: first.ID, ProviderKey: TiingoProviderKey, ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US", Status: "mapped", FetchedAt: fetched, SourcePolicy: domain.TiingoRawClosePriceBasis,
		Observations:   []sqlite.InstrumentHistoryObservation{{MarketDate: "2026-09-04", Value: "100.12345678", Currency: "USD", ValueEffectiveAt: fetched}},
		VerifiedRanges: []sqlite.DateSpan{{Start: "2026-09-04", End: "2026-09-06"}},
	}
	if _, err := repo.CommitInstrumentHistory(ctx, commit); err != nil {
		t.Fatal(err)
	}
	manual, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Manual with missing anchor", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, manual.ID, "123.12345678", s.clock().Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	assertHistoryPlanMatchesLegacy(t, s)
	price, _ := domain.ParseUnitPrice("100.12345678")
	quote, err := domain.NewInstrumentQuote(second, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: fetched}, s.clock())
	if err != nil {
		t.Fatal(err)
	}
	quote.ObservationKind, quote.EffectiveDate = "close", "2026-09-08"
	quote.SourcePolicyVersion, quote.PriceBasis, quote.TimestampBasis = "agent_supplied_v1", "agent_raw_close_v1", "source_timestamp"
	quote.ValueEffectiveAt = fetched
	if _, err := repo.ImportAgentQuoteBatch(ctx, domain.AgentQuoteBatch{HouseholdID: second.HouseholdID, RequestKey: "reuse-append", CreatedAt: s.clock(), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteAppend, InstrumentQuote: &quote, SourceTitle: "Synthetic exchange"}}}); err != nil {
		t.Fatal(err)
	}
	assertHistoryPlanMatchesLegacy(t, s)
	plan, err := s.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, second.ID, map[string]bool{"2026-09-08": false})
	corrected := quote
	corrected.ID, corrected.EffectiveDate = domain.NewInstrumentQuoteID(), "2026-09-07"
	if _, err := repo.ImportAgentQuoteBatch(ctx, domain.AgentQuoteBatch{HouseholdID: second.HouseholdID, RequestKey: "reuse-correct", CreatedAt: s.clock(), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteCorrect, InstrumentQuote: &corrected, TargetQuoteID: quote.ID.String(), SourceTitle: "Synthetic correction"}}}); err != nil {
		t.Fatal(err)
	}
	assertHistoryPlanMatchesLegacy(t, s)
	plan, err = s.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, second.ID, map[string]bool{"2026-09-08": true, "2026-09-07": false})
	if _, err := repo.ImportAgentQuoteBatch(ctx, domain.AgentQuoteBatch{HouseholdID: second.HouseholdID, RequestKey: "reuse-retract", CreatedAt: s.clock(), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteRetract, TargetQuoteID: corrected.ID.String(), SourceTitle: "Synthetic withdrawal"}}}); err != nil {
		t.Fatal(err)
	}
	assertHistoryPlanMatchesLegacy(t, s)
	plan, err = s.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, second.ID, map[string]bool{"2026-09-07": true, "2026-09-08": true})
	// Source and archive changes must be visible on the next request as well.
	if err := s.SetInstrumentQuoteSource(ctx, second.ID, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetInstrumentArchive(ctx, first.HouseholdID, first.ID, true, s.clock()); err != nil {
		t.Fatal(err)
	}
	assertHistoryPlanMatchesLegacy(t, s)
}

func TestHistoryPlanReusePreservesReadErrors(t *testing.T) {
	t.Parallel()
	s, _ := marketReadFixture(t, 3, 1)
	repo := s.repository
	for _, failure := range []error{context.Canceled, errors.New("coverage unavailable")} {
		s.repository = &historyReadCounter{Repository: repo, failure: failure}
		got, err := s.PlanHistorySync(context.Background(), HistorySyncOptions{})
		if !errors.Is(err, failure) || !reflect.DeepEqual(got, HistoryRepairPlan{}) {
			t.Fatalf("error did not propagate: %+v %v", got, err)
		}
	}
	s.repository = repo
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.PlanHistorySync(canceled, HistorySyncOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan: %v", err)
	}
	// A malformed stored quote remains an error even on Force Recheck; reuse
	// must not turn bad input into missing data or silently skip its parsing.
	db, err := sql.Open("sqlite", repo.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`UPDATE instrument_quotes SET unit_price = 'malformed'`); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		opts := HistorySyncOptions{ForceRecheck: force}
		_, want := legacyPlanHistorySync(s, context.Background(), opts)
		got, err := s.PlanHistorySync(context.Background(), opts)
		if want == nil || err == nil || err.Error() != want.Error() || !reflect.DeepEqual(got, HistoryRepairPlan{}) {
			t.Fatalf("force=%v malformed quote: got %+v %v want %v", force, got, err, want)
		}
	}
}

type cancelAfterPlanningReadsRepository struct {
	Repository
	cancel          context.CancelFunc
	instrumentReads int
}

func (r *cancelAfterPlanningReadsRepository) ListInstruments(ctx context.Context, id domain.HouseholdID, archived bool) ([]domain.Instrument, error) {
	instruments, err := r.Repository.ListInstruments(ctx, id, archived)
	if err != nil {
		return nil, err
	}
	r.instrumentReads++
	if r.instrumentReads == 2 {
		// Agent dates read instruments first; history starts read them last.
		// Cancel after that successful read, before initial CPU planning ends.
		r.cancel()
	}
	return instruments, nil
}

func TestHistoryPlanCancellationAfterInitialReads(t *testing.T) {
	t.Parallel()
	s, _ := marketReadFixture(t, 3, 1)
	repo := s.repository
	for _, force := range []bool{false, true} {
		for _, reference := range []bool{false, true} {
			t.Run(fmt.Sprintf("force=%v/reference=%v", force, reference), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				wrapped := &cancelAfterPlanningReadsRepository{Repository: repo, cancel: cancel}
				s.repository = wrapped
				opts := HistorySyncOptions{ForceRecheck: force}
				var got HistoryRepairPlan
				var err error
				if reference {
					got, err = legacyPlanHistorySync(s, ctx, opts)
				} else {
					got, err = s.PlanHistorySync(ctx, opts)
				}
				if wrapped.instrumentReads != 2 || !errors.Is(ctx.Err(), context.Canceled) {
					t.Fatalf("cancellation did not reach the CPU planning boundary: reads=%d err=%v", wrapped.instrumentReads, ctx.Err())
				}
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, HistoryRepairPlan{}) {
					t.Fatalf("cancelled planning returned %+v, %v", got, err)
				}
			})
		}
	}
}
