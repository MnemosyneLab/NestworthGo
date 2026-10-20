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
	peer := overviewAccount(t, s, owner, "Outside broker", "brokerage", "asset", "holdings", "CNY", "")
	if _, err = s.AppendAccountCashValue(ctx, peer.Account.ID, "100", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	peerHeld, err := s.CreateHolding(ctx, HoldingInput{AccountID: peer.Account.ID.String(), InstrumentID: priced.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "America/New_York", map[domain.HoldingID]string{held.ID: "8", peerHeld.ID: "8"}); err != nil {
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
	measure := false
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
		for _, valuation := range []domain.Valuation{domain.ValuationBase, domain.ValuationNative} {
			for _, restrictCurrency := range []bool{false, true} {
				q := query
				q.Valuation = valuation
				if restrictCurrency {
					currency := domain.CurrencyCode("CNY")
					q.Filters.Currency = &currency
				}
				for _, investment := range []bool{false, true} {
					expected, wantErr := computeAnalysisForValuationUniverseContext(ctx, want, q, investment)
					actual, gotErr := computeAnalysisForValuationUniverseContext(ctx, got, q, investment)
					if !reflect.DeepEqual(wantErr, gotErr) || !reflect.DeepEqual(expected, actual) {
						t.Fatalf("complete differential valuation=%s investment=%v filter=%v: %v / %v", valuation, investment, restrictCurrency, wantErr, gotErr)
					}
					if wantErr == nil && len(actual.Days) == 0 {
						t.Fatal("vacuous result")
					}
				}
			}
		}
		// Every selected component retains its exact financial/source fields.
		for i, snapshot := range want.Snapshots {
			if snapshot.LocalDate < "2026-08-01" {
				continue
			}
			for _, item := range snapshot.Items {
				if item.AccountID != selected.Account.ID {
					continue
				}
				found := false
				for _, candidate := range got.Snapshots[i].Items {
					if candidate.ID == item.ID {
						found = true
						if !reflect.DeepEqual(item, candidate) {
							t.Fatal("selected snapshot/source evidence changed")
						}
					}
				}
				if !found {
					t.Fatal("selected historical component omitted")
				}
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
			if newSQL.queries > oldSQL.queries+1 {
				t.Fatalf("unexpected query overhead: %d -> %d", oldSQL.queries, newSQL.queries)
			}
			var admissionQueries int64
			for _, statement := range newSQL.statements {
				if strings.Contains(statement.query, "SELECT DISTINCT account_id FROM daily_valuation_snapshot_items") {
					admissionQueries += statement.queries
				}
			}
			if admissionQueries != 1 {
				t.Fatalf("historical ID admission queries=%d", admissionQueries)
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
			if measure {
				t.Logf("from=%s stored item rows/decodes %d -> %d; whole loader SELECTs %d -> %d; paired warm-process load %s -> %s (prebuilt snapshots, no memo)", query.From, oldRows, newRows, oldSQL.queries, newSQL.queries, oldTime, newTime)
			}
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
	unknownMetadata := func(t *testing.T) {
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
	}
	t.Run("captured metadata absent", unknownMetadata)
	// Transfers retain both counterparties and all correction effects while
	// selected component hydration remains active.
	*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	fee := mustMoney(t, "1", "CNY")
	transfer := domain.CashTransferInput{HouseholdID: selected.Account.HouseholdID, FromAccountID: selected.Account.ID, ToAccountID: others[0].Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), Fee: &fee, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, transfer)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	// Future activity does not require outside snapshot components.
	query.From = "2026-08-01"
	query.To = "2026-08-01"
	before := reader.calls
	compare(t, true)
	if reader.calls != before+1 {
		t.Fatal("future activity did not use scoped loader")
	}
	query.From = "2026-08-02"
	query.To = "2026-08-03"
	before = reader.calls
	compare(t, true)
	if reader.calls != before+1 {
		t.Fatal("transfer did not use scoped loader")
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
	compare(t, true)
	if reader.calls != before+1 {
		t.Fatal("corrected transfer did not use scoped loader")
	}
	// Reverse direction, then correct that transfer as well.
	transfer.FromAccountID, transfer.ToAccountID = others[0].Account.ID, selected.Account.ID
	transfer.Sent, transfer.Received = mustMoney(t, "10", "CNY"), mustMoney(t, "10", "CNY")
	reverse, err := s.RecordChange(ctx, transfer)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	compare(t, true)
	transfer.Sent, transfer.Received = mustMoney(t, "12", "CNY"), mustMoney(t, "12", "CNY")
	if _, err = s.FixChange(ctx, reverse.Activity.ID, transfer); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	compare(t, true)
	// Both in-kind directions and an unpriced carrying-value removal.
	for _, pair := range [][2]domain.HoldingID{{held.ID, peerHeld.ID}, {peerHeld.ID, held.ID}} {
		if _, err = s.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: selected.Account.HouseholdID, FromHoldingID: pair[0], ToHoldingID: pair[1], Quantity: mustQuantity(t, "1"), EffectiveAt: transfer.EffectiveAt}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.RecordChange(ctx, domain.PositionAdjustmentInput{HouseholdID: selected.Account.HouseholdID, HoldingID: held.ID, Quantity: mustQuantity(t, "1"), Added: false, EffectiveAt: transfer.EffectiveAt}); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	compare(t, true)
	input, err := narrow.loadInputs(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	carrying := false
	for _, day := range result.Days {
		if day.Component.HoldingID != nil && *day.Component.HoldingID == held.ID && day.AssetBucketExact[domain.BucketAdjustment].String() == "-5" {
			carrying = true
		}
	}
	if !carrying {
		t.Fatal("fixture did not exercise selected carrying-value removal")
	}

	// Public commands settle trades/dividends on the holding's own account.
	// Persist synthetic cross-account cash evidence through the real adapter:
	// preview normal commands, redirect only cash effects, and recompute endpoint
	// projections with ApplyEffects before atomic persistence. No kernel-only input.
	outside := func(command any) {
		t.Helper()
		state, e := s.changeState(ctx)
		if e != nil {
			t.Fatal(e)
		}
		preview, e := domain.PreviewChange(state, command)
		if e != nil {
			t.Fatal(e)
		}
		for i := range preview.Effects {
			if preview.Effects[i].Target == domain.EffectTargetAccountCash {
				id := peer.Account.ID
				preview.Effects[i].AccountID = &id
			}
		}
		preview.Activity.Effects = preview.Effects
		_, endpoints, e := domain.ApplyEffects(state, preview.Effects)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.repository.CommitActivity(ctx, preview.Activity, preview.Effects, endpoints, *now); e != nil {
			t.Fatal(e)
		}
	}
	outside(domain.TradeInput{HouseholdID: selected.Account.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: selected.Account.ID, HoldingID: held.ID, InstrumentID: priced.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "5", "CNY"), Fee: &fee, EffectiveAt: transfer.EffectiveAt})
	outside(domain.CashDividendInput{HouseholdID: selected.Account.HouseholdID, HoldingID: held.ID, Amount: mustMoney(t, "2", "USD"), EffectiveAt: transfer.EffectiveAt})
	outside(domain.MoneyRemovedInput{HouseholdID: selected.Account.HouseholdID, AccountID: selected.Account.ID, HoldingID: &held.ID, Amount: fee, Reason: domain.ReasonFee, EffectiveAt: transfer.EffectiveAt})
	if err = s.ensureClosedDaySnapshots(ctx, "2026-08-01", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	measure = true
	compare(t, true)
	query.From = "2026-08-01"
	compare(t, true)
	input, err = narrow.loadInputs(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	result, err = ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	associatedFees, associatedDividends := 0, 0
	for _, day := range result.Days {
		if day.Component.HoldingID == nil || *day.Component.HoldingID != held.ID {
			continue
		}
		for _, effect := range day.AttributedEffects {
			if effect.ReturnComponent == nil || effect.SourceEffect.AccountID == nil || *effect.SourceEffect.AccountID != peer.Account.ID {
				continue
			}
			switch *effect.ReturnComponent {
			case domain.ReturnInvestmentFee:
				if effect.Amount.Amount().String() != "-1" {
					t.Fatal("outside fee amount changed")
				}
				associatedFees++
			case domain.ReturnDividendInterest:
				if effect.Amount.Amount().String() != "14" {
					t.Fatal("outside dividend amount changed")
				}
				associatedDividends++
			}
		}
	}
	if associatedFees != 2 || associatedDividends != 1 {
		t.Fatalf("outside attribution fees=%d dividends=%d", associatedFees, associatedDividends)
	}
	// Exercise the normal service native-to-base fallback after the USD cash
	// dividend introduces an unsupported native cash currency for CNY holdings.
	nativeQuery := query
	nativeQuery.Valuation = domain.ValuationNative
	currency := domain.CurrencyCode("CNY")
	nativeQuery.Filters.Currency = &currency
	for _, investment := range []bool{false, true} {
		want, forcedWant, wantErr := full.computeWithValuationFallback(ctx, nativeQuery, investment)
		got, forcedGot, gotErr := narrow.computeWithValuationFallback(ctx, nativeQuery, investment)
		if wantErr != nil || gotErr != nil || forcedWant != "base" || forcedGot != forcedWant || !reflect.DeepEqual(want, got) {
			t.Fatalf("native fallback investment=%v: %s/%s %v/%v", investment, forcedWant, forcedGot, wantErr, gotErr)
		}
	}
	query.From = "2026-08-02"
	measure = false
	t.Run("captured metadata absent with activities", unknownMetadata)

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
