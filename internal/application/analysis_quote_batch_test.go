package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Hide optional batching to exercise the unchanged legacy SQL, while retaining
// the same snapshot quote evidence port as the SQLite repository.
type analysisLegacyQuoteRepository struct {
	Repository
	quoteReads int
}

func (r *analysisLegacyQuoteRepository) ListInstrumentQuotes(ctx context.Context, id domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	r.quoteReads++
	return r.Repository.ListInstrumentQuotes(ctx, id)
}

func (r *analysisLegacyQuoteRepository) ReadSnapshotQuoteEvidence(ctx context.Context, id domain.HouseholdID, snapshots []domain.DailyValuationSnapshot) (domain.PortfolioSnapshot, error) {
	return r.Repository.(snapshotQuoteEvidenceRepository).ReadSnapshotQuoteEvidence(ctx, id, snapshots)
}

type analysisBatchQuoteRepository struct {
	*analysisLegacyQuoteRepository
	batchReads   int
	failure      error
	requestedIDs []domain.InstrumentID
	returnedRows int
}

func (r *analysisBatchQuoteRepository) LoadAnalysisQuoteBatch(ctx context.Context, id domain.HouseholdID, ids []domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	r.batchReads++
	if r.failure != nil {
		return nil, r.failure
	}
	r.requestedIDs = append([]domain.InstrumentID(nil), ids...)
	quotes, err := r.Repository.(AnalysisQuoteBatchRepository).LoadAnalysisQuoteBatch(ctx, id, ids)
	r.returnedRows = len(quotes)
	return quotes, err
}

func TestAnalysisQuoteBatchInputsResultsAndSQLCount(t *testing.T) {
	// Deliberately serial: the existing driver metrics collector is global.
	s, db, owner, now := overviewFixture(t)
	ctx := t.Context()
	broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	if _, err := s.AppendAccountCashValue(ctx, broker.Account.ID, "100", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	var cleared domain.Holding
	costs := map[domain.HoldingID]string{}
	for i := 0; i < 12; i++ {
		currency := "CNY"
		if i == 1 {
			currency = "USD"
		}
		instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic %02d", i), Type: "etf", QuoteCurrency: currency, QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "10.12345678", "2026-08-01", false); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2"})
			if err != nil {
				t.Fatal(err)
			}
			costs[h.ID] = "9"
			if i == 0 {
				cleared = h
			}
		}
		// Unselected provider candidates, including dates far before origin.
		price, err := domain.ParseUnitPrice("11.98765432")
		if err != nil {
			t.Fatal(err)
		}
		for _, at := range []time.Time{now.AddDate(-3, 0, 0), *now, now.AddDate(0, 0, 10)} {
			quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceProvider, SourceKey: "synthetic", QuotedAt: at}, *now)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.repository.AppendInstrumentQuote(ctx, quote); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "7.12345678", "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.RecordChange(ctx, domain.TradeInput{HouseholdID: broker.Account.HouseholdID, Side: domain.TradeSell, SettlementAccountID: broker.Account.ID, HoldingID: cleared.ID, InstrumentID: cleared.InstrumentID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "22", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if err := s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	metrics := enableReviewSQLMetrics(t, reviewFullPipelineFixture{db: db, dbPath: db.Path})
	legacyRepo := &analysisLegacyQuoteRepository{Repository: s.repository}
	batchRepo := &analysisBatchQuoteRepository{analysisLegacyQuoteRepository: &analysisLegacyQuoteRepository{Repository: s.repository}}
	legacy := NewAnalysisService(legacyRepo, s.clock)
	batch := NewAnalysisService(batchRepo, s.clock)
	for _, from := range []domain.LocalDate{"2026-08-01", "2026-08-02"} {
		query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: from, To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
		metrics.reset()
		started := time.Now()
		want, err := legacy.loadInputs(ctx, query)
		oldElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		oldSQL := metrics.snapshot(0)
		metrics.reset()
		started = time.Now()
		got, err := batch.loadInputs(ctx, query)
		newElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		newSQL := metrics.snapshot(0)
		// Only the in-memory opening snapshot has fresh IDs/creation time.
		// Keep its complete financial fields, content hash and quote evidence.
		for _, input := range []*AnalysisInputs{&want, &got} {
			for i := range input.Snapshots {
				snapshot := &input.Snapshots[i]
				if snapshot.LocalDate >= "2026-08-01" {
					continue
				}
				snapshot.ID = ""
				snapshot.CreatedAt = time.Time{}
				for j := range snapshot.Items {
					snapshot.Items[j].ID = ""
				}
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("batch changed full analysis inputs (including quote candidates)")
		}
		if len(got.InstrumentQuotes) != 48 || len(got.FXQuotes) == 0 || len(got.Activities) == 0 {
			t.Fatalf("fixture lost candidates/FX/history: quotes=%d FX=%d activities=%d", len(got.InstrumentQuotes), len(got.FXQuotes), len(got.Activities))
		}
		foundCleared := false
		for _, h := range got.Portfolio.Holdings {
			if h.ID == cleared.ID {
				foundCleared = true
				if !h.Quantity.IsZero() {
					t.Fatal("fixture position not cleared")
				}
			}
		}
		if !foundCleared {
			t.Fatal("cleared holding was omitted")
		}
		for _, scope := range []domain.AnalysisScope{{Kind: domain.ScopeHousehold}, {Kind: domain.ScopeAccount, ID: broker.Account.ID.String()}} {
			query.Scope = scope
			for _, valuation := range []domain.Valuation{domain.ValuationBase, domain.ValuationNative} {
				query.Valuation = valuation
				query.Filters.Currency = nil
				if valuation == domain.ValuationNative {
					currency := domain.CurrencyCode("CNY")
					query.Filters.Currency = &currency
				}
				wantResult, err := ComputeAnalysis(want, query)
				if err != nil {
					t.Fatal(err)
				}
				gotResult, err := ComputeAnalysis(got, query)
				if err != nil || !reflect.DeepEqual(gotResult, wantResult) {
					t.Fatalf("analysis result differs: %v", err)
				}
				if valuation == domain.ValuationBase && (gotResult.Status != domain.CompletenessOK || len(gotResult.Days) == 0) {
					t.Fatalf("vacuous/incomplete result comparison: %+v", gotResult)
				}
			}
		}
		// The complete household capture above is the independent full quote
		// oracle for this account query; only the candidate quote slice changes.
		narrowRepo := &analysisBatchQuoteRepository{analysisLegacyQuoteRepository: &analysisLegacyQuoteRepository{Repository: s.repository}}
		narrowed, err := NewAnalysisService(narrowRepo, s.clock).loadInputs(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(narrowRepo.requestedIDs) != 2 || narrowRepo.returnedRows != 8 {
			t.Fatalf("requested IDs=%d returned rows=%d", len(narrowRepo.requestedIDs), narrowRepo.returnedRows)
		}
		if len(narrowed.InstrumentQuotes) != 8 {
			t.Fatalf("narrowed rows=%d, want 8", len(narrowed.InstrumentQuotes))
		}
		// Compare both arms using the identical complete captured opening input;
		// fresh opening IDs are incidental to separate origin reconstruction.
		candidate := want
		candidate.InstrumentQuotes = narrowed.InstrumentQuotes
		expected, err := ComputeAnalysis(want, query)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := ComputeAnalysis(candidate, query)
		if err != nil || !reflect.DeepEqual(expected, actual) {
			t.Fatalf("narrowed complete analysis differs: %v", err)
		}
		t.Logf("H2a from=%s requested instruments 12->2; actual returned quote rows 48->%d (origin opening maintained separately)", from, len(narrowed.InstrumentQuotes))
		// All SELECTs in loadInputs are counted by the driver through Rows.Close,
		// including portfolio hydration, origin opening and evidence reads.
		if oldSQL.queries-newSQL.queries != 9 {
			t.Fatalf("whole loading query count: legacy=%d batch=%d, want reduction 12-(1+1+1)", oldSQL.queries, newSQL.queries)
		}
		var oldQuoteQueries, newQuoteQueries int64
		for _, statement := range oldSQL.statements {
			if strings.Contains(statement.query, "FROM instrument_quotes WHERE instrument_id = ?") {
				oldQuoteQueries += statement.queries
			}
		}
		for _, statement := range newSQL.statements {
			if strings.Contains(statement.query, "FROM instrument_quotes WHERE instrument_id = ?") {
				t.Fatal("batch moved repeated quote queries elsewhere")
			}
			if strings.Contains(statement.query, "FROM instrument_quotes WHERE instrument_id IN (") {
				newQuoteQueries += statement.queries
			}
		}
		if oldQuoteQueries != 12 || newQuoteQueries != 1 {
			t.Fatalf("quote SQL legacy=%d batch=%d", oldQuoteQueries, newQuoteQueries)
		}
		t.Logf("from=%s instruments=12 candidates=48 loadInputs (durable snapshots prebuilt; no memo/ensurer): legacy=%s SELECTs=%d; batch=%s SELECTs=%d; quote SELECTs=12->1 (+2 bounded validation SELECTs); SQL consumption=%s -> %s", from, oldElapsed, oldSQL.queries, newElapsed, newSQL.queries, oldSQL.queryTime, newSQL.queryTime)
	}
	if legacyRepo.quoteReads != 24 || batchRepo.quoteReads != 0 || batchRepo.batchReads != 2 {
		t.Fatalf("dispatch legacy=%d batch legacy=%d batch=%d", legacyRepo.quoteReads, batchRepo.quoteReads, batchRepo.batchReads)
	}
	batchRepo.failure = errors.New("synthetic batch failure")
	if _, err := batch.loadInputs(ctx, domain.AnalysisQuery{From: "2026-08-02", To: "2026-08-03"}); !errors.Is(err, batchRepo.failure) || batchRepo.quoteReads != 0 {
		t.Fatalf("batch failure masked by fallback: %v", err)
	}
}
