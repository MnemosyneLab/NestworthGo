package application

import (
	"context"
	"encoding/json"
	"errors"
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
