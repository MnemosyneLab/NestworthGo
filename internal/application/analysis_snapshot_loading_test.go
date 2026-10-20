package application

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type analysisAccountSnapshotReader struct {
	Repository
	calls int
	rows  int
}

func (r *analysisAccountSnapshotReader) LoadAnalysisAccountSnapshots(ctx context.Context, h domain.HouseholdID, from, to time.Time, selected domain.AccountID, known []domain.AccountID) ([]domain.DailyValuationSnapshot, error) {
	r.calls++
	snapshots, err := r.Repository.(AnalysisSnapshotRepository).LoadAnalysisAccountSnapshots(ctx, h, from, to, selected, known)
	r.rows = 0
	for _, snapshot := range snapshots {
		r.rows += len(snapshot.Items)
	}
	return snapshots, err
}
func (r *analysisAccountSnapshotReader) ReadSnapshotQuoteEvidence(ctx context.Context, h domain.HouseholdID, snapshots []domain.DailyValuationSnapshot) (domain.PortfolioSnapshot, error) {
	return r.Repository.(snapshotQuoteEvidenceRepository).ReadSnapshotQuoteEvidence(ctx, h, snapshots)
}
func TestAnalysisAccountSnapshotLoading(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	ctx := t.Context()
	selected := overviewAccount(t, s, owner, "Selected", "brokerage", "asset", "holdings", "CNY", "")
	if _, err := s.AppendAccountCashValue(ctx, selected.Account.ID, "100", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	var others []domain.AccountRecord
	for i := 0; i < 12; i++ {
		others = append(others, overviewAccount(t, s, owner, fmt.Sprintf("Unrelated %d", i), "bank_account", "asset", "balance", "CNY", "100"))
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic zero", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateHolding(ctx, HoldingInput{AccountID: selected.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	priced, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic held", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, priced.ID, "5", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	held, err := s.CreateHolding(ctx, HoldingInput{AccountID: selected.Account.ID.String(), InstrumentID: priced.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "America/New_York", map[domain.HoldingID]string{held.ID: "8"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 3)
	if err = s.ArchiveAccount(ctx, others[len(others)-1].Account.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	metrics := enableReviewSQLMetrics(t, reviewFullPipelineFixture{db: db, dbPath: db.Path})
	reader := &analysisAccountSnapshotReader{Repository: s.repository}
	narrow := NewAnalysisService(reader, s.clock)
	full := NewAnalysisService(&analysisBatchQuoteRepository{analysisLegacyQuoteRepository: &analysisLegacyQuoteRepository{Repository: s.repository}}, s.clock)
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeAccount, ID: selected.Account.ID.String()}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	compare := func(t *testing.T, wantNarrow bool) {
		t.Helper()
		metrics.reset()
		started := time.Now()
		want, err := full.loadInputs(ctx, query)
		oldTime := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		oldSQL := metrics.snapshot(0)
		metrics.reset()
		started = time.Now()
		got, err := narrow.loadInputs(ctx, query)
		newTime := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		newSQL := metrics.snapshot(0)
		if !reflect.DeepEqual(want.Portfolio, got.Portfolio) || !reflect.DeepEqual(want.Activities, got.Activities) || !reflect.DeepEqual(want.FXQuotes, got.FXQuotes) {
			t.Fatal("snapshot narrowing changed other inputs")
		}
		for i, snapshot := range want.Snapshots {
			if snapshot.LocalDate < "2026-08-01" {
				continue
			}
			snapshot.Items = nil
			actual := got.Snapshots[i]
			actual.Items = nil
			if !reflect.DeepEqual(snapshot, actual) {
				t.Fatal("household snapshot header changed")
			}
		}
		for _, investment := range []bool{false, true} {
			expected, err := computeAnalysisForValuationUniverseContext(ctx, want, query, investment)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := computeAnalysisForValuationUniverseContext(ctx, got, query, investment)
			if err != nil || !reflect.DeepEqual(expected, actual) {
				t.Fatalf("complete differential (investment=%v): %v", investment, err)
			}
			if len(actual.Days) == 0 {
				t.Fatal("vacuous result")
			}
		}
		count := func(in AnalysisInputs) int {
			n := 0
			for _, v := range in.Snapshots {
				if v.LocalDate >= "2026-08-01" {
					n += len(v.Items)
				}
			}
			return n
		}
		if wantNarrow {
			if count(got) >= count(want) || reader.rows != count(got) {
				t.Fatalf("no actual narrowing: %d -> %d", count(want), count(got))
			}
		} else if count(got) != count(want) {
			t.Fatalf("fallback clipped items: %d -> %d", count(want), count(got))
		}
		if wantNarrow {
			if newSQL.queries != oldSQL.queries+2 {
				t.Fatalf("unexpected query overhead: %d -> %d", oldSQL.queries, newSQL.queries)
			}
			var oldRows, newRows int64
			for _, v := range oldSQL.statements {
				if strings.Contains(v.query, "SELECT i.snapshot_id, i.id") {
					oldRows += v.rows
				}
			}
			for _, v := range newSQL.statements {
				if strings.Contains(v.query, "SELECT i.snapshot_id, i.id") {
					newRows += v.rows
				}
			}
			if newRows >= oldRows {
				t.Fatalf("SQL did not reduce decoded item rows: %d -> %d", oldRows, newRows)
			}
			t.Logf("from=%s stored item rows/decodes %d -> %d; whole loader SELECTs %d -> %d; paired warm-process load %s -> %s (prebuilt snapshots, no memo)", query.From, oldRows, newRows, oldSQL.queries, newSQL.queries, oldTime, newTime)
		}
	}
	compare(t, true)
	query.From = "2026-08-01"
	compare(t, true) // full immutable origin construction remains
	query.From = "2026-08-02"
	t.Run("household scope", func(t *testing.T) {
		old := query.Scope
		query.Scope = domain.AnalysisScope{Kind: domain.ScopeHousehold}
		defer func() { query.Scope = old }()
		before := reader.calls
		compare(t, false)
		if reader.calls != before {
			t.Fatal("household scope attempted narrowing")
		}
	})
	t.Run("unknown selected", func(t *testing.T) {
		old := query.Scope.ID
		query.Scope.ID = domain.NewAccountID().String()
		defer func() { query.Scope.ID = old }()
		want, e1 := full.loadInputs(ctx, query)
		got, e2 := narrow.loadInputs(ctx, query)
		if e1 != nil || e2 != nil || !reflect.DeepEqual(want.Snapshots, got.Snapshots) {
			t.Fatalf("unknown selected fallback: %v %v", e1, e2)
		}
	})
	t.Run("captured metadata absent", func(t *testing.T) {
		// Exercise the repository's same-transaction historical ID admission through
		// the service port, with an account missing from captured portfolio metadata.
		old := reader.Repository
		oldFull := full
		reader.Repository = &analysisMissingMetadataRepository{Repository: old, hidden: others[0].Account.ID}
		full = NewAnalysisService(&analysisBatchQuoteRepository{analysisLegacyQuoteRepository: &analysisLegacyQuoteRepository{Repository: reader.Repository}}, s.clock)
		defer func() { reader.Repository = old; full = oldFull }()
		compare(t, false)
		input, err := narrow.loadInputs(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		universe, err := resolveAnalysisUniverse(input, query)
		if err != nil || !universe.hasUnknownAccount("2026-08-02") {
			t.Fatalf("unknown-account evidence lost: %v", err)
		}
	})
	// A transfer (with fee) rejects service admission, preserving both counterparties.
	*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	fee := mustMoney(t, "1", "CNY")
	transfer := domain.CashTransferInput{HouseholdID: selected.Account.HouseholdID, FromAccountID: others[0].Account.ID, ToAccountID: others[1].Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), Fee: &fee, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, transfer)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	// Before the transfer date, the service sees no activities but the
	// repository's activity guard still forces full loading.
	query.From = "2026-08-01"
	query.To = "2026-08-01"
	before := reader.calls
	compare(t, false)
	if reader.calls != before+1 {
		t.Fatal("future activity guard was not exercised")
	}
	query.From = "2026-08-02"
	query.To = "2026-08-03"
	before = reader.calls
	compare(t, false)
	if reader.calls != before {
		t.Fatal("activity path attempted narrowing")
	}
	transfer.Sent = mustMoney(t, "25", "CNY")
	transfer.Received = mustMoney(t, "25", "CNY")
	if _, err = s.FixChange(ctx, change.Activity.ID, transfer); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	before = reader.calls
	compare(t, false)
	if reader.calls != before {
		t.Fatal("corrected transfer attempted narrowing")
	}
}

type analysisMissingMetadataRepository struct {
	Repository
	hidden domain.AccountID
}

func (r *analysisMissingMetadataRepository) LoadAnalysisAccountSnapshots(ctx context.Context, h domain.HouseholdID, from, to time.Time, selected domain.AccountID, known []domain.AccountID) ([]domain.DailyValuationSnapshot, error) {
	return r.Repository.(AnalysisSnapshotRepository).LoadAnalysisAccountSnapshots(ctx, h, from, to, selected, known)
}

func (r *analysisMissingMetadataRepository) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	p, err := r.Repository.ReadPortfolioSnapshot(ctx, filter)
	accounts := make([]domain.AccountRecord, 0, len(p.Accounts))
	for _, a := range p.Accounts {
		if a.Account.ID != r.hidden {
			accounts = append(accounts, a)
		}
	}
	p.Accounts = accounts
	return p, err
}

func (r *analysisMissingMetadataRepository) ReadSnapshotQuoteEvidence(ctx context.Context, h domain.HouseholdID, snapshots []domain.DailyValuationSnapshot) (domain.PortfolioSnapshot, error) {
	return r.Repository.(snapshotQuoteEvidenceRepository).ReadSnapshotQuoteEvidence(ctx, h, snapshots)
}

func (r *analysisAccountSnapshotReader) LoadAnalysisQuoteBatch(ctx context.Context, h domain.HouseholdID, ids []domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	return r.Repository.(AnalysisQuoteBatchRepository).LoadAnalysisQuoteBatch(ctx, h, ids)
}
func (r *analysisMissingMetadataRepository) LoadAnalysisQuoteBatch(ctx context.Context, h domain.HouseholdID, ids []domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	return r.Repository.(AnalysisQuoteBatchRepository).LoadAnalysisQuoteBatch(ctx, h, ids)
}
