package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func reviewIntegrationFixture(t *testing.T) (*Service, *sqlite.DB, domain.Instrument, func(time.Time)) {
	t.Helper()
	s, ctx, b, setClock := newOnboardedService(t, "investment-review", []string{"Synthetic owner"})
	setClock(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	a := createHoldingsAccount(t, s, ctx, b.Members[0].ID, "Synthetic brokerage")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"}); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(s.repository.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	return s, db, i, setClock
}
func importReviewNAV(t *testing.T, s *Service, i domain.Instrument, date, value string) {
	t.Helper()
	if _, err := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{agentNAV(i, value, date)}}); err != nil {
		t.Fatal(err)
	}
}
func reviewTableCounts(t *testing.T, db *sqlite.DB) map[string]int {
	t.Helper()
	result := map[string]int{}
	for _, table := range []string{"activities", "instrument_quotes", "daily_valuation_snapshots", "agent_quote_records"} {
		var n int
		if err := db.SQL.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		result[table] = n
	}
	return result
}
func TestInvestmentReviewOptInPersistenceEpisodesAndReadonlyEvaluation(t *testing.T) {
	s, db, i, setClock := reviewIntegrationFixture(t)
	ctx := t.Context()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []AgentMarketDataItem{}
	for n := 0; n < 39; n++ {
		items = append(items, agentNAV(i, "1", day.AddDate(0, 0, n).Format(time.DateOnly)))
	}
	if _, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	before := reviewTableCounts(t, db)
	r, err := s.InvestmentReviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 1 || r.Items[0].Monitored || r.Items[0].EpisodeID != "" || r.Items[0].ObservationCount != 31 || r.Items[0].LatestOn != "2026-10-09" {
		t.Fatalf("default/real history %+v", r)
	}
	store := sqlite.NewConfigurationRepository(db)
	if data, err := store.ListConfiguration("investment_review/"); err != nil || len(data) != 0 {
		t.Fatalf("evaluation wrote configuration %v %v", data, err)
	}
	r, err = s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), true, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	episode := r.Items[0].EpisodeID
	if episode == "" || r.Items[0].Status != "low_yield" {
		t.Fatalf("opt in %+v", r)
	}
	if _, err = s.AcknowledgeInvestmentReview(ctx, "unknown"); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("unknown episode accepted %v", err)
	}
	for range 2 {
		r, err = s.AcknowledgeInvestmentReview(ctx, episode)
		if err != nil || !r.Items[0].Acknowledged {
			t.Fatalf("ack %+v %v", r, err)
		}
	}
	// A fresh service uses the same SQLite-backed configuration and observations.
	restarted := NewService(sqlite.NewRepository(db))
	restarted.SetInvestmentReviewConfiguration(store)
	restarted.SetClock(func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) })
	for range 3 {
		r, err = restarted.InvestmentReviews(ctx)
		if err != nil || r.Items[0].EpisodeID != episode || !r.Items[0].Acknowledged {
			t.Fatalf("relaunch spam %+v %v", r, err)
		}
	}
	after := reviewTableCounts(t, db)
	for table, n := range before {
		if after[table] != n {
			t.Fatalf("review mutated %s", table)
		}
	}
	setClock(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	importReviewNAV(t, s, i, "2026-10-10", "0.98")
	r, err = s.InvestmentReviews(ctx)
	if err != nil || r.Items[0].EpisodeID != episode || !r.Items[0].Acknowledged {
		t.Fatalf("new observation spam %+v %v", r, err)
	}
	setClock(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	importReviewNAV(t, s, i, "2026-10-11", "1.01")
	for n := 12; n <= 19; n++ {
		importReviewNAV(t, s, i, time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), "0.98")
	}
	r, err = s.InvestmentReviews(ctx)
	if err != nil || r.Items[0].EpisodeID == "" || r.Items[0].EpisodeID == episode || r.Items[0].Acknowledged {
		t.Fatalf("new episode %+v %v", r, err)
	}
	p := r.Policy
	p.Enabled = false
	r, err = s.SaveInvestmentReviewPolicy(ctx, p, r.Revision)
	if err != nil || r.Items[0].EpisodeID != "" {
		t.Fatalf("pause %+v %v", r, err)
	}
	if _, err = s.SaveInvestmentReviewPolicy(ctx, p, r.Revision-1); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("stale save accepted %v", err)
	}
	p.WindowDays = 8
	if _, err = s.SaveInvestmentReviewPolicy(ctx, p, r.Revision); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("bad policy accepted %v", err)
	}
}
func TestInvestmentReviewActualRevisionsWithdrawalAndUnsupportedDividend(t *testing.T) {
	s, _, i, setClock := reviewIntegrationFixture(t)
	ctx := t.Context()
	importReviewNAV(t, s, i, "2026-09-01", "1")
	importReviewNAV(t, s, i, "2026-10-01", "1")
	original, err := s.CurrentInstrumentQuote(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	correction := agentNAV(i, "1.03", "2026-10-01")
	correction.Operation, correction.TargetQuoteID = "correct", original.ID.String()
	if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
		t.Fatal(err)
	}
	r, err := s.InvestmentReviews(ctx)
	if err != nil || r.Items[0].ObservationCount != 2 || *r.Items[0].PeriodPercent != "3" {
		t.Fatalf("revision %+v %v", r, err)
	}
	quote, err := s.CurrentInstrumentQuote(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{Operation: "retract", TargetQuoteID: quote.ID.String(), SourceTitle: "Synthetic issuer withdrawal"}}}); err != nil {
		t.Fatal(err)
	}
	r, err = s.InvestmentReviews(ctx)
	if err != nil || r.Items[0].LatestOn != "2026-09-01" || r.Items[0].PeriodPercent != nil {
		t.Fatalf("withdrawn NAV used %+v %v", r, err)
	}
	setClock(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	item := agentNAV(i, "0.99", "2026-10-01")
	item.DividendCash = "0.01"
	if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); err != nil {
		t.Fatal(err)
	}
	r, err = s.InvestmentReviews(ctx)
	if err != nil || r.Items[0].Reason != "distribution_or_unit_change" || r.Items[0].AnnualizedPercent != nil {
		t.Fatalf("payout double count %+v %v", r, err)
	}
}

type failingReviewStore struct {
	fail  error
	raw   []byte
	found bool
}

func (s *failingReviewStore) LoadConfiguration(string) ([]byte, bool, error) {
	return s.raw, s.found, s.fail
}
func (s *failingReviewStore) SaveConfiguration(string, []byte) error { return s.fail }
func TestInvestmentReviewReadFailuresAndInvalidConfiguration(t *testing.T) {
	s, _, _, _ := reviewIntegrationFixture(t)
	sentinel := errors.New("synthetic storage failure")
	s.SetInvestmentReviewConfiguration(&failingReviewStore{fail: sentinel})
	if _, err := s.InvestmentReviews(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("error hidden %v", err)
	}
	raw, _ := json.Marshal(investmentReviewState{Policy: domain.InvestmentReviewPolicy{WindowDays: 8}, Acknowledged: map[string]bool{}, Enrolled: map[string]bool{}})
	s.SetInvestmentReviewConfiguration(&failingReviewStore{raw: raw, found: true})
	if _, err := s.InvestmentReviews(t.Context()); !hasDomainCode(err, domain.ErrIntegrity) {
		t.Fatalf("unsafe recovery %v", err)
	}
	s.SetInvestmentReviewConfiguration(nil)
	if _, err := s.InvestmentReviews(t.Context()); !hasDomainCode(err, domain.ErrUnavailable) {
		t.Fatalf("missing storage %v", err)
	}
}

func TestInvestmentReviewConcurrentEnrollmentAndExclusiveGate(t *testing.T) {
	s, _, i, _ := reviewIntegrationFixture(t)
	ctx := t.Context()
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), true, 0); results <- err }()
	}
	success, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if hasDomainCode(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("optimistic enrollment = %d success/%d conflict", success, conflicts)
	}
	release := holdExclusive(t, s, ExclusiveBackup)
	if _, err := s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), false, 1); !isBackupRestoreBusy(err) {
		release()
		t.Fatalf("write escaped exclusive gate %v", err)
	}
	release()
	for range 4 {
		go func() {
			for range 10 {
				_, err := s.InvestmentReviews(ctx)
				if err != nil {
					results <- err
					return
				}
			}
			results <- nil
		}()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
func TestInvestmentReviewWorksWithReadOnlyDatabase(t *testing.T) {
	s, _, i, _ := reviewIntegrationFixture(t)
	importReviewNAV(t, s, i, "2026-09-01", "1")
	importReviewNAV(t, s, i, "2026-10-01", "1.003")
	db, err := sqlite.OpenReadOnlyForVerify(s.repository.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader := NewService(sqlite.NewRepository(db))
	reader.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	reader.SetClock(func() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) })
	r, err := reader.InvestmentReviews(t.Context())
	if err != nil || len(r.Items) != 1 || r.Items[0].AnnualizedPercent == nil || *r.Items[0].AnnualizedPercent != "3.65" {
		t.Fatalf("read-only evaluation %+v %v", r, err)
	}
}

func TestInvestmentReviewAcknowledgementSurvivesEarlierHistoryEnrichment(t *testing.T) {
	s, db, i, setClock := reviewIntegrationFixture(t)
	ctx := t.Context()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []AgentMarketDataItem{}
	for n := 0; n < 39; n++ {
		items = append(items, agentNAV(i, "1", day.AddDate(0, 0, n).Format(time.DateOnly)))
	}
	if _, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	report, err := s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), true, 0)
	if err != nil {
		t.Fatal(err)
	}
	original := report.Items[0]
	if original.EpisodeID == "" {
		t.Fatal("expected confirmed episode")
	}
	if _, err = s.AcknowledgeInvestmentReview(ctx, original.EpisodeID); err != nil {
		t.Fatal(err)
	}
	// Earlier coverage moves the reconstructed first breach from Oct 1 to Sep 30.
	// The current evidence and the acknowledged ongoing episode have not changed.
	importReviewNAV(t, s, i, "2026-08-31", "1")
	restarted := NewService(sqlite.NewRepository(db))
	restarted.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	restarted.SetClock(func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) })
	for range 3 {
		report, err = restarted.InvestmentReviews(ctx)
		if err != nil {
			t.Fatal(err)
		}
		item := report.Items[0]
		if item.EpisodeID != original.EpisodeID || !item.Acknowledged || item.LatestOn != original.LatestOn || item.BaselineOn != original.BaselineOn || *item.PeriodPercent != "0" || *item.AnnualizedPercent != "0" {
			t.Fatalf("earlier enrichment reissued acknowledged episode: %+v; original: %+v", item, original)
		}
	}
	// A supported recovery, then another confirmed run, must still issue a new episode.
	setClock(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	importReviewNAV(t, s, i, "2026-10-10", "1.01")
	report, err = s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Status != "healthy" || report.Items[0].EpisodeID != "" {
		t.Fatalf("recovery %+v %v", report, err)
	}
	setClock(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	for n := 11; n <= 18; n++ {
		importReviewNAV(t, s, i, time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), "0.98")
	}
	report, err = s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].EpisodeID == "" || report.Items[0].EpisodeID == original.EpisodeID || report.Items[0].Acknowledged {
		t.Fatalf("real recovery suppressed new episode %+v %v", report, err)
	}
	nextID := report.Items[0].EpisodeID
	if _, err = s.AcknowledgeInvestmentReview(ctx, nextID); err != nil {
		t.Fatal(err)
	}
	policy := report.Policy
	policy.LowAnnualizedPercent = "2"
	report, err = s.SaveInvestmentReviewPolicy(ctx, policy, report.Revision)
	if err != nil || report.Items[0].EpisodeID != nextID || !report.Items[0].Acknowledged {
		t.Fatalf("preference edit reset acknowledged episode %+v %v", report, err)
	}
}

func TestInvestmentReviewDatedNAVTimestampVariants(t *testing.T) {
	for _, scenario := range []string{"date_only", "timestamped", "mixed", "mixed_timezone"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, i, _ := reviewIntegrationFixture(t)
			baseline, latest := agentNAV(i, "1", "2026-09-01"), agentNAV(i, "1.003", "2026-10-01")
			baseline.Timezone, latest.Timezone = "UTC", "UTC"
			if scenario == "timestamped" {
				baseline.QuotedAt = "2026-09-01T15:30:00Z"
			}
			if scenario == "timestamped" || scenario == "mixed" {
				latest.QuotedAt = "2026-10-01T15:30:00Z"
			}
			if scenario == "mixed_timezone" {
				baseline.Timezone = "Asia/Singapore"
				baseline.QuotedAt = "2026-08-31T17:30:00Z"
			}
			if _, err := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{baseline, latest}}); err != nil {
				t.Fatal(err)
			}
			report, err := s.InvestmentReviews(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			item := report.Items[0]
			if item.PeriodPercent == nil || *item.PeriodPercent != "0.3" || item.AnnualizedPercent == nil || *item.AnnualizedPercent != "3.65" || item.ElapsedDays != 30 || item.BaselineOn != "2026-09-01" || item.LatestOn != "2026-10-01" {
				t.Fatalf("valid dated NAV excluded or timestamp used as denominator: %+v", item)
			}
		})
	}
}

func confirmedReviewFixture(t *testing.T) (*Service, *sqlite.DB, domain.Instrument, func(time.Time)) {
	t.Helper()
	s, db, i, setClock := reviewIntegrationFixture(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []AgentMarketDataItem{}
	for n := 0; n < 39; n++ {
		items = append(items, agentNAV(i, "1", day.AddDate(0, 0, n).Format(time.DateOnly)))
	}
	if _, err := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetInvestmentReviewEnrollment(t.Context(), i.ID.String(), true, 0); err != nil {
		t.Fatal(err)
	}
	return s, db, i, setClock
}
func TestInvestmentReviewSnoozeExpiryAndFreshness(t *testing.T) {
	s, db, i, setClock := confirmedReviewFixture(t)
	ctx := t.Context()
	original, err := s.InvestmentReviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, original.Revision)
	if err != nil || !report.Items[0].Monitored || !report.Items[0].Snoozed || report.Items[0].SnoozedUntilOn != "2026-10-16" || report.Items[0].EpisodeID != "" || report.Items[0].Acknowledged || *report.Items[0].AnnualizedPercent != "0" {
		t.Fatalf("snooze %+v %v", report, err)
	}
	restarted := NewService(sqlite.NewRepository(db))
	restarted.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	restarted.SetClock(func() time.Time { return time.Date(2026, 10, 15, 23, 59, 59, 0, time.UTC) })
	report, err = restarted.InvestmentReviews(ctx)
	if err != nil || !report.Items[0].Snoozed || report.Items[0].EpisodeID != "" {
		t.Fatalf("before expiry %+v %v", report, err)
	}
	restarted.SetClock(func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) })
	report, err = restarted.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID != original.Items[0].EpisodeID || report.Items[0].AgeDays != 7 || report.Items[0].Acknowledged {
		t.Fatalf("expiry boundary %+v %v", report, err)
	}
	setClock(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	report, err = s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].EpisodeID != "" || report.Items[0].Status != "stale" {
		t.Fatalf("stale at expiry %+v %v", report, err)
	}
}
func TestInvestmentReviewThirtyDaySnoozeAndBackupPersistence(t *testing.T) {
	s, _, i, setClock := confirmedReviewFixture(t)
	ctx := t.Context()
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 30, 1)
	if err != nil || report.Items[0].SnoozedUntilOn != "2026-11-08" {
		t.Fatalf("30 civil days %+v %v", report, err)
	}
	setClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	items := []AgentMarketDataItem{}
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for n := 0; n < 29; n++ {
		items = append(items, agentNAV(i, "1", day.AddDate(0, 0, n).Format(time.DateOnly)))
	}
	if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	backupPath := t.TempDir() + "/backup.db"
	if err = s.repository.SnapshotTo(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restored := NewService(sqlite.NewRepository(db))
	restored.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	restored.SetClock(func() time.Time { return time.Date(2026, 11, 7, 23, 59, 59, 0, time.UTC) })
	report, err = restored.InvestmentReviews(ctx)
	if err != nil || !report.Items[0].Snoozed || report.Items[0].SnoozedUntilOn != "2026-11-08" || report.Items[0].EpisodeID != "" {
		t.Fatalf("backup before expiry %+v %v", report, err)
	}
	restored.SetClock(func() time.Time { return time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC) })
	report, err = restored.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID == "" || report.Items[0].AgeDays != 1 {
		t.Fatalf("30-day expiry %+v %v", report, err)
	}
}
func TestInvestmentReviewSnoozeUsesHouseholdCivilDateAcrossDST(t *testing.T) {
	s, _, i, setClock := confirmedReviewFixture(t)
	ctx := t.Context()
	if _, err := s.StartHistory(ctx, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	setClock(time.Date(2026, 11, 1, 3, 30, 0, 0, time.UTC)) // Oct 31 23:30 EDT
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, 1)
	if err != nil || report.LocalDate != "2026-10-31" || report.Timezone != "America/New_York" || report.Items[0].SnoozedUntilOn != "2026-11-07" {
		t.Fatalf("household snooze date %+v %v", report, err)
	}
	setClock(time.Date(2026, 11, 7, 4, 59, 59, 0, time.UTC)) // Nov 6 23:59 EST
	report, err = s.InvestmentReviews(ctx)
	if err != nil || !report.Items[0].Snoozed {
		t.Fatalf("local pre-expiry %+v %v", report, err)
	}
	setClock(time.Date(2026, 11, 7, 5, 0, 0, 0, time.UTC))
	report, err = s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID != "" || report.Items[0].Status != "stale" {
		t.Fatalf("local expiry or stale suppression %+v %v", report, err)
	}
}
func TestInvestmentReviewSnoozePreservesAcknowledgementBackfillAndPreferences(t *testing.T) {
	s, db, i, setClock := confirmedReviewFixture(t)
	ctx := t.Context()
	original, err := s.InvestmentReviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcknowledgeInvestmentReview(ctx, original.Items[0].EpisodeID); err != nil {
		t.Fatal(err)
	}
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	importReviewNAV(t, s, i, "2026-08-31", "1")
	policy := report.Policy
	policy.WindowDays = 7
	policy.LowAnnualizedPercent = "2"
	report, err = s.SaveInvestmentReviewPolicy(ctx, policy, report.Revision)
	if err != nil || !report.Items[0].Snoozed || report.Items[0].SnoozedUntilOn != "2026-10-16" || report.Items[0].EpisodeID != "" || !report.Items[0].Acknowledged {
		t.Fatalf("preference/backfill overrode ignore or acknowledgement %+v %v", report, err)
	}
	restarted := NewService(sqlite.NewRepository(db))
	restarted.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	restarted.SetClock(func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) })
	report, err = restarted.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Snoozed || !report.Items[0].Acknowledged || report.Items[0].EpisodeID != original.Items[0].EpisodeID {
		t.Fatalf("acknowledgement lost at expiry %+v %v", report, err)
	}
	setClock(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	report, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 0, report.Revision)
	if err != nil || report.Items[0].Snoozed || !report.Items[0].Acknowledged {
		t.Fatalf("manual resume acknowledged episode %+v %v", report, err)
	}
}
func TestInvestmentReviewRecoveryAndUnsupportedDataDuringSnooze(t *testing.T) {
	for _, scenario := range []string{"recovery", "distribution"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, i, setClock := confirmedReviewFixture(t)
			ctx := t.Context()
			original, err := s.InvestmentReviews(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, 1); err != nil {
				t.Fatal(err)
			}
			setClock(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
			item := agentNAV(i, "1.01", "2026-10-15")
			if scenario == "distribution" {
				item.DividendCash = "0.01"
			}
			if _, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); err != nil {
				t.Fatal(err)
			}
			report, err := s.InvestmentReviews(ctx)
			if err != nil || !report.Items[0].Snoozed || report.Items[0].EpisodeID != "" {
				t.Fatalf("snooze overridden %+v %v", report, err)
			}
			setClock(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
			report, err = s.InvestmentReviews(ctx)
			if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID != "" {
				t.Fatalf("invalid reminder on expiry %+v %v", report, err)
			}
			if scenario == "distribution" {
				if report.Items[0].Reason != "distribution_or_unit_change" {
					t.Fatalf("unsupported %+v", report)
				}
				return
			}
			if report.Items[0].Status != "healthy" {
				t.Fatalf("recovery %+v", report)
			}
			setClock(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
			importReviewNAV(t, s, i, "2026-10-17", "0.98")
			importReviewNAV(t, s, i, "2026-10-24", "0.98")
			report, err = s.InvestmentReviews(ctx)
			if err != nil || report.Items[0].EpisodeID == "" || report.Items[0].EpisodeID == original.Items[0].EpisodeID || report.Items[0].Acknowledged {
				t.Fatalf("new breach after recovery %+v %v", report, err)
			}
		})
	}
}
func TestInvestmentReviewManualResumePermanentIgnoreAndConcurrentEdits(t *testing.T) {
	s, _, i, setClock := confirmedReviewFixture(t)
	ctx := t.Context()
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	report, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 0, report.Revision)
	if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID == "" || !report.Items[0].Monitored {
		t.Fatalf("manual resume %+v %v", report, err)
	}
	report, err = s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), false, report.Revision)
	if err != nil {
		t.Fatal(err)
	}
	setClock(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	report, err = s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].Monitored || report.Items[0].EpisodeID != "" || report.Items[0].AnnualizedPercent == nil {
		t.Fatalf("permanent ignore %+v %v", report, err)
	}
	report, err = s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), true, report.Revision)
	if err != nil || report.Items[0].EpisodeID == "" {
		t.Fatalf("re-enable %+v %v", report, err)
	}
	results := make(chan error, 2)
	revision := report.Revision
	go func() { _, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, revision); results <- err }()
	go func() {
		p := report.Policy
		p.LowAnnualizedPercent = "2"
		_, err := s.SaveInvestmentReviewPolicy(ctx, p, revision)
		results <- err
	}()
	success, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if hasDomainCode(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent edits %d success/%d conflict", success, conflicts)
	}
	release := holdExclusive(t, s, ExclusiveBackup)
	if _, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, revision); !isBackupRestoreBusy(err) {
		release()
		t.Fatalf("exclusive gate %v", err)
	}
	release()
	if _, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 14, revision); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("unsupported duration %v", err)
	}
}

func TestInvestmentReviewLegacyAcknowledgementSurvivesBackfillAndPreferences(t *testing.T) {
	s, db, i, _ := confirmedReviewFixture(t)
	ctx := t.Context()
	household, err := s.repository.Household(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.DefaultInvestmentReviewPolicy()
	legacyID := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("investment-review/1|%s|%s|%+v|%s", household.ID, i.ID, policy, "2026-10-01"))))
	// Initial PR #59 configuration had only these fields; retain compatibility.
	raw, err := json.Marshal(struct {
		Policy       domain.InvestmentReviewPolicy `json:"policy"`
		Revision     int                           `json:"revision"`
		Enrolled     map[string]bool               `json:"enrolled"`
		Acknowledged map[string]bool               `json:"acknowledged"`
	}{policy, 1, map[string]bool{i.ID.String(): true}, map[string]bool{legacyID: true}})
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewConfigurationRepository(db)
	key := "investment_review/" + household.ID.String()
	if err = store.SaveConfiguration(key, raw); err != nil {
		t.Fatal(err)
	}
	importReviewNAV(t, s, i, "2026-08-31", "1")
	report, err := s.InvestmentReviews(ctx)
	if err != nil || report.Items[0].EpisodeID != legacyID || !report.Items[0].Acknowledged {
		t.Fatalf("legacy acknowledgement lost %+v %v", report, err)
	}
	unchanged, _, err := store.LoadConfiguration(key)
	if err != nil || !bytes.Equal(raw, unchanged) {
		t.Fatal("legacy read rewrote configuration")
	}
	policy.LowAnnualizedPercent = "2"
	report, err = s.SaveInvestmentReviewPolicy(ctx, policy, report.Revision)
	if err != nil || report.Items[0].EpisodeID != legacyID || !report.Items[0].Acknowledged {
		t.Fatalf("legacy preference edit lost acknowledgement %+v %v", report, err)
	}
}
func TestInvestmentReviewSnoozeSurvivesPreferenceEditsWithoutAcknowledging(t *testing.T) {
	s, _, i, _ := confirmedReviewFixture(t)
	ctx := t.Context()
	report, err := s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	importReviewNAV(t, s, i, "2026-08-31", "1")
	policy := report.Policy
	policy.WindowDays = 7
	policy.Enabled = false
	report, err = s.SaveInvestmentReviewPolicy(ctx, policy, report.Revision)
	if err != nil || !report.Items[0].Snoozed || report.Items[0].SnoozedUntilOn != "2026-10-16" || report.Items[0].Acknowledged || !report.Items[0].Monitored {
		t.Fatalf("preference reset snooze %+v %v", report, err)
	}
	report, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 0, report.Revision)
	if err != nil || report.Items[0].Snoozed || report.Items[0].EpisodeID != "" || report.Items[0].Acknowledged {
		t.Fatalf("resume overrode master pause or acknowledged %+v %v", report, err)
	}
	policy.Enabled = true
	report, err = s.SaveInvestmentReviewPolicy(ctx, policy, report.Revision)
	if err != nil || report.Items[0].EpisodeID == "" || report.Items[0].Acknowledged {
		t.Fatalf("resume silently acknowledged %+v %v", report, err)
	}
}

func TestInvestmentReviewAliasEditsPreserveEnrollmentAcknowledgementAndSnooze(t *testing.T) {
	s, db, i, _ := reviewIntegrationFixture(t)
	ctx := t.Context()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []AgentMarketDataItem{}
	for n := 0; n < 39; n++ {
		items = append(items, agentNAV(i, "1", day.AddDate(0, 0, n).Format(time.DateOnly)))
	}
	if _, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	counts := reviewTableCounts(t, db)
	report, err := s.InvestmentReviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"unenrolled", "enrolled", "acknowledged", "snoozed"} {
		switch state {
		case "enrolled":
			report, err = s.SetInvestmentReviewEnrollment(ctx, i.ID.String(), true, report.Revision)
		case "acknowledged":
			report, err = s.AcknowledgeInvestmentReview(ctx, report.Items[0].EpisodeID)
		case "snoozed":
			report, err = s.SetInvestmentReviewSnooze(ctx, i.ID.String(), 7, report.Revision)
		}
		if err != nil {
			t.Fatal(err)
		}
		original := report
		original.Items = append([]domain.InvestmentReviewItem(nil), report.Items...)
		store := sqlite.NewConfigurationRepository(db)
		configBefore, err := store.ListConfiguration("investment_review/")
		if err != nil {
			t.Fatal(err)
		}
		for _, alias := range []string{"Family fund", "", "家用基金", ""} {
			if _, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{ShortName: &alias, Symbol: "000001"}); err != nil {
				t.Fatal(err)
			}
			report, err = s.InvestmentReviews(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := alias
			if want == "" {
				want = "000001"
			}
			if report.Items[0].DisplayName != want {
				t.Fatalf("%s label: %+v", state, report)
			}
			report.Items[0].DisplayName = original.Items[0].DisplayName
			if !reflect.DeepEqual(report, original) {
				t.Fatalf("%s state changed: %+v / %+v", state, report, original)
			}
			configAfter, err := store.ListConfiguration("investment_review/")
			if err != nil || !reflect.DeepEqual(configAfter, configBefore) {
				t.Fatalf("alias changed review configuration: %v", err)
			}
		}
	}
	if !reflect.DeepEqual(reviewTableCounts(t, db), counts) {
		t.Fatal("alias edits changed financial rows")
	}
}
