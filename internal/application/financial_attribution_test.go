package application

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func attributionFor(t *testing.T, s *Service, left, right string, ids ...domain.AccountID) FinancialComparisonResult {
	t.Helper()
	in := FinancialComparisonRequest{LeftAsOf: left, RightAsOf: right}
	if len(ids) > 0 {
		in.Scope.Kind = "accounts"
		for _, id := range ids {
			in.Scope.AccountIDs = append(in.Scope.AccountIDs, id.String())
		}
	}
	r, err := s.BuildFinancialAttribution(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Content.Attribution == nil {
		t.Fatal("missing attribution link")
	}
	return r
}
func wantAttributionStatus(t *testing.T, r FinancialComparisonResult, status, reason string) {
	t.Helper()
	link := r.Content.Attribution
	if link.Status != status || (reason != "" && (len(link.MismatchReasons) != 1 || link.MismatchReasons[0] != reason)) {
		t.Fatalf("link: %+v", link)
	}
}
func attributionDriver(link *FinancialAttributionLink, key string) *string {
	for _, row := range link.Drivers {
		if row.Key == key {
			return row.Amount.Value
		}
	}
	zero := "0"
	return &zero
}

func TestFinancialAttributionIncomeTransfersRepaymentAndScope(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Private salary", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "Private destination", "bank_account", "asset", "balance", "CNY", "0")
	debt := overviewAccount(t, s, owner, "Private loan", "loan", "liability", "balance", "CNY", "30")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	commands := []any{
		domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now},
		domain.CashTransferInput{HouseholdID: a.Account.HouseholdID, FromAccountID: a.Account.ID, ToAccountID: b.Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), EffectiveAt: now.Add(time.Minute)},
		domain.DebtPaymentInput{HouseholdID: a.Account.HouseholdID, DebtAccountID: debt.Account.ID, CashAccountID: a.Account.ID, Principal: mustMoney(t, "10", "CNY"), EffectiveAt: now.Add(2 * time.Minute)},
	}
	*now = now.Add(3 * time.Minute)
	for _, command := range commands {
		if _, err := s.RecordChange(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.AddDate(0, 0, 1)
	var before int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&before); err != nil {
		t.Fatal(err)
	}
	r := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, r, "compatible", "")
	link := r.Content.Attribution
	wantOverviewAmount(t, r.Content.Change.NetWorth.Value, "50")
	wantOverviewAmount(t, link.BeginningValue.Value, "70")
	wantOverviewAmount(t, link.EndingValue.Value, "120")
	wantOverviewAmount(t, link.ExplainedDelta.Value, "50")
	wantOverviewAmount(t, link.Residual.Value, "0")
	wantOverviewAmount(t, attributionDriver(link, "income"), "50")
	if link.Period.From != "2026-08-02" || link.Period.To != "2026-08-02" || link.BasisHash == "" {
		t.Fatal(link)
	}
	// Salary is wealth growth, not investment return. Repayment and transfer
	// principal cancel in the household net worth, without creating profit.
	if link.InvestmentReturn.Amount.Value != nil && *link.InvestmentReturn.Amount.Value != "0" {
		t.Fatal(link.InvestmentReturn)
	}
	if link.InvestmentReturn.Rate != nil {
		t.Fatal("invented cash-only investment rate")
	}
	raw, _ := json.Marshal(r.Content)
	for _, secret := range []string{"Private", a.Account.ID.String(), b.Account.ID.String(), debt.Account.ID.String()} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("minimal leak", secret)
		}
	}
	var after int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatal("snapshot side effect not exercised")
	}
	scoped := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantAttributionStatus(t, scoped, "compatible", "")
	wantOverviewAmount(t, scoped.Content.Attribution.ExplainedDelta.Value, "20")
	if scoped.Content.Attribution.Scope.AccountCount != 1 {
		t.Fatal("scope widened")
	}
	oldHash := scoped.ContentHash
	// Hidden current metadata and unrelated values do not change this package.
	if _, err := db.SQL.Exec("UPDATE accounts SET name='do not execute',note='private note' WHERE id=?", b.Account.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAccountValue(t.Context(), b.Account.ID, "99", ""); err != nil {
		t.Fatal(err)
	}
	if next := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID); next.ContentHash != oldHash {
		t.Fatal("out-of-scope hash changed")
	}
	named, err := s.BuildFinancialAttribution(t.Context(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02", Disclosure: "named", Scope: FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{a.Account.ID.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(named.Content)
	if !strings.Contains(string(raw), "Private salary") || strings.Contains(string(raw), "do not execute") {
		t.Fatal("named scope disclosure", string(raw))
	}
}

func TestFinancialAttributionMissingFXInclusionAndUnsupported(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "A", "bank_account", "asset", "balance", "USD", "100")
	b := overviewAccount(t, s, owner, "B", "bank_account", "asset", "balance", "CNY", "0")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	missing := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantAttributionStatus(t, missing, "unavailable", "missing_valuation_evidence")
	if missing.Content.Change.NetWorth.Value != nil || missing.Content.Attribution.ExplainedDelta.Value != nil || missing.Content.Attribution.Residual.Value != nil || missing.Content.Attribution.AnalysisDelta.Value != nil || missing.Content.Attribution.PrecisionAdjustment.Value != nil || missing.Content.Attribution.Precision.BoundaryAdjustment.Value != nil || missing.Content.Attribution.Precision.DriverAdjustment.Value != nil {
		t.Fatal("missing FX substituted", missing)
	}
	current := attributionFor(t, s, "2026-08-01", "current")
	wantAttributionStatus(t, current, "unavailable", "right_endpoint_not_closed")
	if current.Content.Attribution.Period != nil || current.Content.Attribution.InvestmentReturn != nil {
		t.Fatal("fabricated current returns")
	}
	set := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID, b.Account.ID)
	wantAttributionStatus(t, set, "unavailable", "unsupported_account_set")
	if set.Content.Attribution.InvestmentReturn != nil {
		t.Fatal("summed account rates")
	}
	if err := s.ArchiveAccount(t.Context(), a.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	// The historical values coincide, but the present analysis universe excludes A.
	mismatch := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantAttributionStatus(t, mismatch, "incompatible", "inclusion_mismatch")
	if len(mismatch.Content.Attribution.Drivers) != 0 || mismatch.Content.Attribution.InvestmentReturn != nil {
		t.Fatal("mixed incompatible reports")
	}
	for _, pair := range [][2]string{{"2026-08-02", "2026-08-01"}, {"2026-08-01", "2026-08-01"}, {"2026-07-31", "2026-08-02"}, {"2026-08-01", "2026-08-03"}, {"invalid", "2026-08-02"}} {
		if _, err := s.BuildFinancialAttribution(t.Context(), FinancialComparisonRequest{LeftAsOf: pair[0], RightAsOf: pair[1]}); err == nil {
			t.Fatal("invalid dates accepted", pair)
		}
	}
	if _, err := s.BuildFinancialAttribution(t.Context(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02", Scope: FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{"account-1"}}}); err == nil {
		t.Fatal("alias accepted as UUID")
	}
}

func TestFinancialAttributionPriceFXAndMissingPrice(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Broker", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Private fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct{ date, price, fx string }{{"2026-08-01", "5", "7"}, {"2026-08-02", "6", "8"}} {
		if _, err := s.AppendManualInstrumentQuote(t.Context(), i.ID, q.price, q.date, false); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendManualFXQuote(t.Context(), "USD", "CNY", q.fx, q.date); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{h.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	r := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, r, "compatible", "")
	wantOverviewAmount(t, r.Content.Change.NetWorth.Value, "130")
	wantOverviewAmount(t, r.Content.Attribution.InvestmentReturn.Amount.Value, "130")
	wantOverviewAmount(t, r.Content.Attribution.Residual.Value, "0")
	if r.Content.Attribution.InvestmentReturn.Rate == nil || len(r.Content.Attribution.InvestmentReturn.Sources) != 2 {
		t.Fatal(r.Content.Attribution.InvestmentReturn)
	}
	if _, err := db.SQL.Exec("DELETE FROM instrument_quotes"); err != nil {
		t.Fatal(err)
	}
	// Deliberately bypassing the coordinator illustrates why stored values alone
	// are no compatibility proof: stale snapshots must be refused even if totals match.
	bad := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, bad, "incompatible", "snapshot_evidence_mismatch")
	if bad.Content.Attribution.InvestmentReturn != nil {
		t.Fatal("stale price certified")
	}
}

func TestFinancialAttributionDSTCivilDayInterval(t *testing.T) {
	for _, pair := range []struct {
		left, right string
		hours       time.Duration
	}{{"2026-03-07", "2026-03-08", 23}, {"2026-10-31", "2026-11-01", 25}} {
		t.Run(pair.right, func(t *testing.T) {
			s, _, owner, now := overviewFixture(t)
			*now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
			if _, err := s.StartHistory(t.Context(), "America/New_York"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
			r := attributionFor(t, s, pair.left, pair.right)
			wantAttributionStatus(t, r, "compatible", "")
			p := r.Content.Attribution.Period
			a, _ := time.Parse(time.RFC3339Nano, p.StartExclusive)
			b, _ := time.Parse(time.RFC3339Nano, p.EndInclusive)
			if b.Sub(a) != pair.hours*time.Hour || p.From != pair.right || p.Timezone != "America/New_York" {
				t.Fatal(p, b.Sub(a))
			}
		})
	}
}

// Hold the immutable capture while an app writer attempts to revise the ledger.
// Snapshot maintenance and both projections must finish before that writer.
func TestFinancialAttributionSerializesConcurrentRevision(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	repo := sqlite.NewRepository(db)
	port := &blockingAttributionRepository{Repository: repo, port: repo, captured: make(chan struct{}), release: make(chan struct{})}
	// Both services must share the same application's coordinator in this test.
	s.repository = port
	done := make(chan FinancialComparisonResult, 1)
	errors := make(chan error, 1)
	go func() {
		r, err := s.BuildFinancialAttribution(context.Background(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02"})
		done <- r
		errors <- err
	}()
	<-port.captured
	wrote := make(chan error, 1)
	go func() { _, err := s.AppendAccountValue(context.Background(), a.Account.ID, "200", ""); wrote <- err }()
	select {
	case err := <-wrote:
		t.Fatal("writer escaped gate", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(port.release)
	r := <-done
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	wantAttributionStatus(t, r, "compatible", "")
	wantOverviewAmount(t, r.Content.Right.Summary.NetWorth.Value, "100")
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
}

type blockingAttributionRepository struct {
	Repository
	port              FinancialContextRepository
	captured, release chan struct{}
	calls             atomic.Int32
}

func (r *blockingAttributionRepository) ReadFinancialContextInputs(ctx context.Context, historical bool, ids []domain.AccountID, now time.Time) (FinancialContextInputs, error) {
	input, err := r.port.ReadFinancialContextInputs(ctx, historical, ids, now)
	if r.calls.Add(1) == 2 {
		close(r.captured)
		select {
		case <-r.release:
		case <-ctx.Done():
			return FinancialContextInputs{}, ctx.Err()
		}
	}
	return input, err
}

func TestFinancialAttributionResidualIsPreserved(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{analysisSnapshot("2026-08-01", analysisItem(t, account.ID, "CNY", "100", "100", nil, nil, "", "")), analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "CNY", "150", "150", nil, nil, "", ""))}}
	result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	link := &FinancialAttributionLink{Drivers: []FinancialAttributionDriver{}}
	comparison := FinancialComparisonContent{Left: FinancialComparisonSide{Basis: FinancialContextBasis{BaseCurrency: "CNY"}, Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("100"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Right: FinancialComparisonSide{Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("150"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Change: FinancialComparisonChange{NetWorth: contextAmount(historicalString("50"), "CNY")}}
	status, _, err := projectFinancialAttribution(link, &comparison, result, input)
	if err != nil || status != "compatible" {
		t.Fatal(status, err)
	}
	wantOverviewAmount(t, link.ExplainedDelta.Value, "0")
	wantOverviewAmount(t, link.Residual.Value, "50")
	if link.ResidualIssueCount != 1 || link.AssetStatus != "partial" {
		t.Fatal(link)
	}
}
func TestFinancialAttributionUnsupportedHistoricalMidnight(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	*now = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "America/Havana"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	r := attributionFor(t, s, "2026-10-31", "2026-11-01")
	wantAttributionStatus(t, r, "incompatible", "historical_boundary_unsupported")
	var count int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&count); err != nil || count != 0 {
		t.Fatal("unsupported boundary materialized", count, err)
	}
}

func TestFinancialAttributionEvidencePolicyAndScopeProof(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "A", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "B", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	attributionFor(t, s, "2026-08-01", "2026-08-02")
	ids := []domain.AccountID{a.Account.ID}
	inputs, captured, provider, ttl, err := s.captureFinancialContext(t.Context(), true, ids)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse("2006-01-02", "2026-08-01")
	end := start.AddDate(0, 0, 1)
	original, err := s.repository.ListDailyValuationSnapshots(t.Context(), a.Account.HouseholdID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, reason string
		mutate       func([]domain.DailyValuationSnapshot)
	}{
		{"unchanged", "", func(snaps []domain.DailyValuationSnapshot) {}},
		{"equal_amount_different_evidence", "snapshot_evidence_mismatch", func(snaps []domain.DailyValuationSnapshot) {
			for n := range snaps[0].Items {
				if snaps[0].Items[n].AccountID == a.Account.ID {
					fake := "different-observation"
					snaps[0].Items[n].QuoteID = &fake
				}
			}
		}},
		{"out_of_scope_evidence", "", func(snaps []domain.DailyValuationSnapshot) {
			for n := range snaps[0].Items {
				if snaps[0].Items[n].AccountID == b.Account.ID {
					fake := "private-observation"
					snaps[0].Items[n].QuoteID = &fake
				}
			}
		}},
		{"header_completeness", "snapshot_evidence_mismatch", func(snaps []domain.DailyValuationSnapshot) { snaps[0].Complete = !snaps[0].Complete }},
		{"cutoff", "snapshot_cutoff_mismatch", func(snaps []domain.DailyValuationSnapshot) {
			snaps[0].CutoffAt = snaps[0].CutoffAt.Add(-time.Millisecond)
		}},
		{"policy", "resolver_policy_mismatch", func(snaps []domain.DailyValuationSnapshot) { snaps[0].ResolverPolicyVersion = "old" }},
		{"currency", "snapshot_evidence_mismatch", func(snaps []domain.DailyValuationSnapshot) { snaps[0].Currency = "USD" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snaps := append([]domain.DailyValuationSnapshot{}, original...)
			for n := range snaps {
				snaps[n].Items = append([]domain.DailyValuationSnapshotItem{}, snaps[n].Items...)
			}
			tc.mutate(snaps)
			_, reason, err := s.validateAttributionSnapshots(t.Context(), inputs, snaps, start, end, ids, captured, provider, ttl)
			if err != nil || reason != tc.reason {
				t.Fatal(reason, err)
			}
		})
	}
}

func TestFinancialAttributionConcurrentExternalRevisionIsRejected(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	repo := sqlite.NewRepository(db)
	port := &blockingAttributionRepository{Repository: repo, port: repo, captured: make(chan struct{}), release: make(chan struct{})}
	s.repository = port
	done := make(chan FinancialComparisonResult, 1)
	errs := make(chan error, 1)
	go func() {
		r, err := s.BuildFinancialAttribution(context.Background(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02"})
		done <- r
		errs <- err
	}()
	<-port.captured
	other := NewService(repo)
	other.SetClock(s.clock)
	if _, err := other.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "10", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: now.AddDate(0, 0, -1)}); err != nil {
		close(port.release)
		t.Fatal(err)
	}
	close(port.release)
	r := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	wantAttributionStatus(t, r, "incompatible", "source_revision_changed")
	if r.Content.Attribution.InvestmentReturn != nil || len(r.Content.Attribution.Drivers) != 0 {
		t.Fatal("mixed source revision")
	}
}
func TestFinancialAttributionInteriorInclusionAndCloseBoundaries(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	for _, entry := range []struct{ at, amount string }{{"2026-08-01T23:59:59.999Z", "5"}, {"2026-08-02T00:00:00Z", "6"}} {
		at, _ := time.Parse(time.RFC3339Nano, entry.at)
		if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, entry.amount, "CNY"), Reason: domain.ReasonIncome, EffectiveAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	r := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, r, "compatible", "")
	wantOverviewAmount(t, r.Content.Left.Summary.NetWorth.Value, "105")
	wantOverviewAmount(t, r.Content.Attribution.ExplainedDelta.Value, "6")
	*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if err := s.ArchiveAccount(t.Context(), a.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if err := s.ArchiveAccount(t.Context(), a.Account.ID, false); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	mismatch := attributionFor(t, s, "2026-08-01", "2026-08-04", a.Account.ID)
	wantAttributionStatus(t, mismatch, "incompatible", "inclusion_mismatch")
	if mismatch.Content.Left.Scope.IncludedAccountCount != 1 || mismatch.Content.Right.Scope.IncludedAccountCount != 1 {
		t.Fatal("test must include both endpoints")
	}
}
