package application

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// TestReviewFullPipelinePerformanceEvidence is an opt-in end-to-end budget
// measurement. It uses a temporary SQLite database populated only with
// synthetic accounts, positions, rules, quote history, FX references, and
// activity rows. Set NW_REVIEW_PERF_EVIDENCE=1 to run it.
func TestReviewFullPipelinePerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_REVIEW_PERF_EVIDENCE") != "1" {
		t.Skip("set NW_REVIEW_PERF_EVIDENCE=1 to run full-scale SQLite evidence")
	}
	mode := os.Getenv("NW_REVIEW_PERF_MODE")
	historyDays := 365
	if mode == "warm" || mode == "stages" {
		historyDays = 30
	}
	fixture := newReviewFullPipelineFixture(t, historyDays)
	ctx := context.Background()

	started := time.Now()
	result, err := fixture.service.ReconcileInvestmentReviewsV2(ctx)
	cold := time.Since(started)
	if err != nil {
		t.Fatalf("cold reconcile: %v", err)
	}
	if result.RuleInstrumentCount != 2_000 {
		t.Fatalf("cold publication covered %d rule/instrument pairs, want 2000", result.RuleInstrumentCount)
	}
	t.Logf("fixture cold publish complete: historyDays=%d elapsed=%s", historyDays, cold)
	if mode == "cold" {
		logReviewFullPipelineSourceCounts(t, fixture, historyDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}
	if mode == "stages" {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		repository := sqlite.NewRepository(fixture.db)
		started = time.Now()
		capture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		captureElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		started = time.Now()
		capturedFingerprint, err := domain.ReviewHistoricalFingerprint(capture.Historical)
		fingerprintElapsed := time.Since(started)
		if err != nil || capturedFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("fingerprint recheck in memory differs: %v", err)
		}
		started = time.Now()
		publication, err := PrepareReviewPublication(ctx, capture, *fixture.now, fixture.service.FXProviderKey())
		prepareElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		plannedDays := 0
		for _, item := range publication.Rules {
			plannedDays += len(item.Days)
		}
		started = time.Now()
		var inputBytes, resultBytes int64
		for _, item := range publication.Rules {
			for _, day := range item.Days {
				inputJSON, err := json.Marshal(day.Input)
				if err != nil {
					t.Fatal(err)
				}
				coverageJSON, err := json.Marshal(day.CoverageMissing)
				if err != nil {
					t.Fatal(err)
				}
				resultJSON, err := json.Marshal(day.Result)
				if err != nil {
					t.Fatal(err)
				}
				inputBytes += int64(len(inputJSON) + len(coverageJSON))
				resultBytes += int64(len(resultJSON))
			}
		}
		serializationElapsed := time.Since(started)
		started = time.Now()
		recheck, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		recheckElapsed := time.Since(started)
		if err != nil || recheck.HistoricalFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("read-only publication recheck differs: %v", err)
		}
		started = time.Now()
		if err := repository.PublishReviewProjection(ctx, publication, *fixture.now); err != nil {
			t.Fatalf("publish staged warm increment: %v", err)
		}
		publishElapsed := time.Since(started)
		t.Logf("one warm increment stages: SQLite capture+source fingerprint=%s (positionActivitiesCaptured=%d); in-memory source fingerprint alone=%s; planning+daily fingerprints=%s; repeated input/result/coverage JSON serialization alone=%s (%d input+coverage bytes, %d result bytes); read-only publish recheck capture=%s; atomic publish recheck+serialization+writes=%s; staged ruleInstrumentPairs=%d newDays=%d", captureElapsed, len(capture.Historical.History.Activities), fingerprintElapsed, prepareElapsed, serializationElapsed, inputBytes, resultBytes, recheckElapsed, publishElapsed, len(publication.Rules), plannedDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}

	incremental := make([]time.Duration, 30)
	for index := range incremental {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		started = time.Now()
		result, err = fixture.service.ReconcileInvestmentReviewsV2(ctx)
		incremental[index] = time.Since(started)
		if err != nil {
			t.Fatalf("incremental reconcile %d: %v", index+1, err)
		}
		if result.RuleInstrumentCount != 2_000 {
			t.Fatalf("incremental publication %d covered %d rule/instrument pairs, want 2000", index+1, result.RuleInstrumentCount)
		}
	}

	pageReads := make([]time.Duration, 30)
	for index := range pageReads {
		started = time.Now()
		page, err := fixture.service.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "pending"}, "", 50)
		pageReads[index] = time.Since(started)
		if err != nil {
			t.Fatalf("first page read %d: %v", index+1, err)
		}
		if page.ProjectionStatus != "complete" || len(page.Items) == 0 {
			t.Fatalf("first page read returned status=%q items=%d", page.ProjectionStatus, len(page.Items))
		}
	}

	quoteCount, activityCount, fxCount := 0, 0, 0
	for _, query := range []struct {
		sql  string
		into *int
	}{{`SELECT count(*) FROM instrument_quotes`, &quoteCount}, {`SELECT count(*) FROM activities`, &activityCount}, {`SELECT count(*) FROM fx_quotes`, &fxCount}} {
		if err := fixture.db.SQL.QueryRowContext(ctx, query.sql).Scan(query.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("fixture: holdings=100 rules=20 ruleInstrumentPairs=2000 instrumentQuotes=%d activities=%d fxReferences=%d historyDays=%d", quoteCount, activityCount, fxCount, historyDays)
	t.Logf("cold full reconcile n=1: %s (budget <=3s)", cold)
	t.Logf("warm incremental full reconcile n=%d: median=%s p95=%s max=%s (budget p95 <=500ms)", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	t.Logf("published alert first page n=%d: median=%s p95=%s max=%s (budget p95 <=150ms)", len(pageReads), durationPercentile(pageReads, 0.50), durationPercentile(pageReads, 0.95), durationPercentile(pageReads, 1))
	logReviewFullPipelineStorage(t, fixture, historyDays)
}

type reviewFullPipelineFixture struct {
	service     *Service
	db          *sqlite.DB
	now         *time.Time
	householdID domain.HouseholdID
	dbPath      string
}

func newReviewFullPipelineFixture(t *testing.T, historyDays int) reviewFullPipelineFixture {
	t.Helper()
	ctx := context.Background()
	start := time.Date(2025, 10, 17, 12, 0, 0, 0, time.UTC)
	now := start
	dbPath := t.TempDir() + "/review-full-pipeline.db"
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic review pipeline", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Synthetic broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	currencies := []string{"CNY", "USD", "EUR", "JPY", "SGD"}
	types := []domain.InstrumentType{domain.InstrumentStock, domain.InstrumentMutualFund, domain.InstrumentCrypto, domain.InstrumentBond}
	instruments := make([]domain.Instrument, 100)
	costs := make(map[domain.HoldingID]string, 100)
	for index := range instruments {
		currency := currencies[index%len(currencies)]
		kind := types[index%len(types)]
		instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic holding %03d", index), Type: string(kind), QuoteCurrency: currency, QuoteSource: "manual"})
		if err != nil {
			t.Fatalf("create instrument %d: %v", index, err)
		}
		instruments[index] = instrument
		if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", start.Format(time.RFC3339), false); err != nil {
			t.Fatalf("add synthetic unit-cost seed quote %d: %v", index, err)
		}
		holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2"})
		if err != nil {
			t.Fatalf("create holding %d: %v", index, err)
		}
		costs[holding.ID] = "10"
	}
	for _, currency := range currencies[1:] {
		if _, err := service.SetFXPreference(ctx, currency, "CNY", "manual"); err != nil {
			t.Fatalf("set synthetic FX preference for %s: %v", currency, err)
		}
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}

	if err := seedReviewFullPipelineFacts(ctx, db, bootstrap.Household.ID, instruments, account.Account.ID, start, historyDays); err != nil {
		t.Fatalf("seed synthetic source facts: %v", err)
	}
	now = start.AddDate(0, 0, historyDays)
	for index := 0; index < 20; index++ {
		spec := syntheticHistoricalReviewSpec()
		spec.Schedule.Timezone = []string{"UTC", "America/Los_Angeles", "Asia/Shanghai", "Europe/London"}[index%4]
		spec.Target = domain.ReviewRuleTarget{Kind: "all_current"}
		metric := index % 3
		if metric == 0 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "100000", Currency: "CNY", FXMode: "convert_on_day"}}
		} else if metric == 1 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "-100", LookbackDays: 7}}
		} else {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "0", Currency: "CNY", FXMode: "convert_on_day"}}
		}
		if index%4 == 1 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
		} else if index%4 == 2 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 7, MinHits: 3}
		}
		if _, err := service.CreateReviewRuleV2(ctx, SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: fmt.Sprintf("Synthetic rule %02d", index), Enabled: true, Severity: "warning", Spec: spec}); err != nil {
			t.Fatalf("create rule %d: %v", index, err)
		}
	}
	return reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: bootstrap.Household.ID, dbPath: dbPath}
}

func logReviewFullPipelineStorage(t *testing.T, fixture reviewFullPipelineFixture, historyDays int) {
	t.Helper()
	ctx := context.Background()
	counts := map[string]int{}
	for _, table := range []string{
		"investment_review_daily_inputs", "investment_review_daily_results", "investment_review_position_epochs",
		"investment_review_episodes", "investment_review_episode_events", "investment_review_evaluation_state",
	} {
		var count int
		if err := fixture.db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	bytes := int64(0)
	for _, path := range []string{fixture.dbPath, fixture.dbPath + "-wal", fixture.dbPath + "-shm"} {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", path, err)
		}
		bytes += info.Size()
	}
	t.Logf("published storage: historyDays=%d database+wal+shm=%d bytes inputRows=%d resultRows=%d positionEpochRows=%d episodeRows=%d episodeEventRows=%d evaluationStateRows=%d", historyDays, bytes, counts["investment_review_daily_inputs"], counts["investment_review_daily_results"], counts["investment_review_position_epochs"], counts["investment_review_episodes"], counts["investment_review_episode_events"], counts["investment_review_evaluation_state"])
}

func logReviewFullPipelineSourceCounts(t *testing.T, fixture reviewFullPipelineFixture, historyDays int) {
	t.Helper()
	ctx := context.Background()
	var quotes, activities, fx int
	for _, query := range []struct {
		sql  string
		into *int
	}{{`SELECT count(*) FROM instrument_quotes`, &quotes}, {`SELECT count(*) FROM activities`, &activities}, {`SELECT count(*) FROM fx_quotes`, &fx}} {
		if err := fixture.db.SQL.QueryRowContext(ctx, query.sql).Scan(query.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("synthetic source rows: holdings=100 rules=20 quoteRows=%d activities=%d fxRows=%d historyDays=%d", quotes, activities, fx, historyDays)
}

func seedReviewFullPipelineFacts(ctx context.Context, db *sqlite.DB, householdID domain.HouseholdID, instruments []domain.Instrument, accountID domain.AccountID, start time.Time, historyDays int) error {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	quoteStatement, err := tx.PrepareContext(ctx, `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,?,'agent','agent',?,?, 'close',?,?,0,?,?,?)`)
	if err != nil {
		return err
	}
	defer quoteStatement.Close()
	quoteIndex := 0
	quoteSlots := len(instruments) * historyDays
	quotesPerSlot, quoteRemainder := 100_000/quoteSlots, 100_000%quoteSlots
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for instrumentIndex, instrument := range instruments {
			copies := quotesPerSlot
			if instrumentIndex*historyDays+day < quoteRemainder {
				copies++
			}
			for duplicate := 0; duplicate < copies; duplicate++ {
				priceBasis, timestampBasis := "agent_raw_close_v1", "date_label"
				if instrument.Type == domain.InstrumentMutualFund || instrument.Type == domain.InstrumentBankInvestmentProduct {
					priceBasis = "agent_unit_nav_v1"
				} else if instrument.Type == domain.InstrumentCrypto {
					priceBasis = domain.CoinGeckoDailyPriceBasis
					timestampBasis = "observed_publication"
				}
				when := date.Add(17 * time.Hour)
				price := fmt.Sprintf("%d", 100+day/30)
				id := fmt.Sprintf("00000000-0000-4000-8000-%012x", quoteIndex+1)
				createdAt := when.Add(time.Duration(duplicate) * time.Second)
				if _, err := quoteStatement.ExecContext(ctx, id, instrument.ID.String(), price, instrument.QuoteCurrency.String(), formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date.Format(time.DateOnly), formatReviewPerfTime(when), priceBasis, timestampBasis, duplicate+1); err != nil {
					return fmt.Errorf("insert quote %d: %w", quoteIndex, err)
				}
				quoteIndex++
			}
		}
	}
	if quoteIndex != 100_000 {
		return fmt.Errorf("seeded %d quote facts, want 100000", quoteIndex)
	}
	fxStatement, err := tx.PrepareContext(ctx, `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,fetched_at,value_effective_at,timestamp_basis,revision) VALUES(?,?,?,'CNY',?,'manual','manual',?,?, 'daily_reference',?,?,?,'date_label',1)`)
	if err != nil {
		return err
	}
	defer fxStatement.Close()
	fxIndex := 0
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for _, currency := range []string{"USD", "EUR", "JPY", "SGD"} {
			when := date.Add(16 * time.Hour)
			id := fmt.Sprintf("00000000-0000-4000-8001-%012x", fxIndex+1)
			if _, err := fxStatement.ExecContext(ctx, id, householdID.String(), currency, "2", formatReviewPerfTime(when), formatReviewPerfTime(when), date.Format(time.DateOnly), formatReviewPerfTime(when), formatReviewPerfTime(when)); err != nil {
				return fmt.Errorf("insert synthetic FX reference %d: %w", fxIndex, err)
			}
			fxIndex++
		}
	}
	activityStatement, err := tx.PrepareContext(ctx, `INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`)
	if err != nil {
		return err
	}
	defer activityStatement.Close()
	effectStatement, err := tx.PrepareContext(ctx, `INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'0','CNY')`)
	if err != nil {
		return err
	}
	defer effectStatement.Close()
	valueStatement, err := tx.PrepareContext(ctx, `INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0','CNY',?,?,?,'event')`)
	if err != nil {
		return err
	}
	defer valueStatement.Close()
	for index := 0; index < 50_000; index++ {
		when := start.AddDate(0, 0, index%historyDays).Add(time.Duration(index) * time.Microsecond)
		at := formatReviewPerfTime(when)
		activityID := fmt.Sprintf("00000000-0000-4000-8002-%012x", index+1)
		effectID := fmt.Sprintf("00000000-0000-4000-8003-%012x", index+1)
		valueID := fmt.Sprintf("00000000-0000-4000-8004-%012x", index+1)
		if _, err := activityStatement.ExecContext(ctx, activityID, householdID.String(), at, when.Format(time.DateOnly), at); err != nil {
			return fmt.Errorf("insert activity %d: %w", index, err)
		}
		if _, err := effectStatement.ExecContext(ctx, effectID, activityID, accountID.String()); err != nil {
			return fmt.Errorf("insert activity effect %d: %w", index, err)
		}
		if _, err := valueStatement.ExecContext(ctx, valueID, accountID.String(), at, at, effectID); err != nil {
			return fmt.Errorf("insert account cash value %d: %w", index, err)
		}
	}
	return tx.Commit()
}

func formatReviewPerfTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func durationPercentile(values []time.Duration, percentile float64) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := int(math.Ceil(percentile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(ordered) {
		index = len(ordered) - 1
	}
	return ordered[index]
}
