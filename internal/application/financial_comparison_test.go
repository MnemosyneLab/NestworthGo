package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func comparisonFor(t *testing.T, s *Service, left, right string, ids ...domain.AccountID) FinancialComparisonResult {
	t.Helper()
	in := FinancialComparisonRequest{LeftAsOf: left, RightAsOf: right}
	if len(ids) > 0 {
		in.Scope.Kind = "accounts"
		for _, id := range ids {
			in.Scope.AccountIDs = append(in.Scope.AccountIDs, id.String())
		}
	}
	r, err := s.BuildFinancialComparison(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestFinancialComparisonTransfersLifecycleScopeHashAndReadOnly(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Secret A", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "Secret B", "bank_account", "asset", "balance", "CNY", "0")
	debt := overviewAccount(t, s, owner, "Secret debt", "loan", "liability", "balance", "CNY", "30")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.RecordChange(t.Context(), domain.CashTransferInput{HouseholdID: a.Account.HouseholdID, FromAccountID: a.Account.ID, ToAccountID: b.Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	later := overviewAccount(t, s, owner, "Secret later", "bank_account", "asset", "balance", "CNY", "0")
	*now = now.AddDate(0, 0, 1)
	if err := s.ArchiveAccount(t.Context(), debt.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	r := comparisonFor(t, s, "2026-08-01", "2026-08-02")
	wantOverviewAmount(t, r.Content.Left.Summary.NetWorth.Value, "70")
	wantOverviewAmount(t, r.Content.Right.Summary.NetWorth.Value, "70")
	wantOverviewAmount(t, r.Content.Change.NetWorth.Value, "0")
	wantOverviewAmount(t, r.Content.Change.Cash.Value, "0")
	wantOverviewAmount(t, r.Content.Change.Investments.Value, "0")
	for _, p := range r.Content.Positions {
		if p.Left != nil && p.Right != nil && (p.Ref != p.Left.Ref || p.Ref != p.Right.Ref) {
			t.Fatal("side alias mismatch", p)
		}
	}
	raw, _ := json.Marshal(r.Content)
	for _, secret := range []string{"Secret", a.Account.ID.String(), b.Account.ID.String(), debt.Account.ID.String()} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("identity leak", secret)
		}
	}
	one := comparisonFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantOverviewAmount(t, one.Content.Change.Cash.Value, "-20")
	if _, err := db.SQL.Exec("UPDATE accounts SET name='changed',note='private' WHERE id=?", a.Account.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAccountValue(t.Context(), b.Account.ID, "99", ""); err != nil {
		t.Fatal(err)
	}
	if next := comparisonFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID); next.ContentHash != one.ContentHash {
		t.Fatal("hidden/out-of-scope changed hash")
	}
	// Current is actual DB state, including observations economically after capture.
	*now = now.AddDate(0, 0, 2)
	if _, err := s.AppendAccountValue(t.Context(), a.Account.ID, "150", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, -2)
	current := comparisonFor(t, s, "2026-08-01", "current", a.Account.ID)
	wantOverviewAmount(t, current.Content.Change.Cash.Value, "50")
	if current.Content.Right.AsOf.CutoffAt != "" || current.Content.Right.Basis.HistoryEvidence != "current_database_state" {
		t.Fatal(current.Content.Right)
	}
	if current.ContentHash == one.ContentHash {
		t.Fatal("change did not affect hash")
	}
	*now = now.Add(time.Hour)
	if comparisonFor(t, s, "2026-08-01", "current", a.Account.ID).ContentHash != current.ContentHash {
		t.Fatal("capture clock in hash")
	}
	if _, err := db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	life := comparisonFor(t, s, "2026-08-01", "2026-08-03", later.Account.ID, debt.Account.ID)
	wantOverviewAmount(t, life.Content.Change.Liabilities.Value, "-30")
	foundNew, foundArchive := false, false
	for _, p := range life.Content.Positions {
		if p.Kind == "account" && p.Left.Status == "not_created" {
			foundNew = true
			if p.Right.Status != "zero" || p.BaseChange != nil {
				t.Fatal(p)
			}
		}
		if p.Right != nil && p.Right.Status == "archived" {
			foundArchive = true
			if p.Right.Included {
				t.Fatal(p)
			}
		}
	}
	if !foundNew || !foundArchive {
		t.Fatal("lifecycle not retained")
	}
}
func TestFinancialComparisonMissingFXNativeDeltaAndDates(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "FX", "bank_account", "asset", "balance", "USD", "10")
	if _, err := s.BuildFinancialComparison(t.Context(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "current"}); !hasDomainCode(err, domain.ErrHistoryNotStarted) {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(t.Context(), "America/New_York"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.AppendAccountValue(t.Context(), a.Account.ID, "13", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	r := comparisonFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	if r.Content.Change.NetWorth.Value != nil || r.Content.Change.Cash.Value != nil {
		t.Fatal("invented base delta")
	}
	for _, p := range r.Content.Positions {
		wantOverviewAmount(t, p.NativeChange, "3")
		if p.BaseChange != nil {
			t.Fatal(p)
		}
	}
	if len(r.Content.Gaps) == 0 || len(r.Content.Evidence) == 0 {
		t.Fatal("no scoped evidence")
	}
	for _, pair := range [][2]string{{"", "current"}, {"current", "current"}, {"2026-07-31", "current"}, {"2026-08-03", "current"}, {"2026-02-30", "current"}, {"2026-08-01", "2026-08-04"}, {"2026-08-01", ""}} {
		if _, err := s.BuildFinancialComparison(t.Context(), FinancialComparisonRequest{LeftAsOf: pair[0], RightAsOf: pair[1]}); err == nil {
			t.Fatal("invalid dates", pair)
		}
	}
	// Date boundary authority is the same helper used by HistoricalOverview.
	*now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	dst := comparisonFor(t, s, "2026-10-31", "2026-11-01")
	l, _ := time.Parse(time.RFC3339Nano, dst.Content.Left.AsOf.CutoffAt)
	rr, _ := time.Parse(time.RFC3339Nano, dst.Content.Right.AsOf.CutoffAt)
	if rr.Sub(l) != 25*time.Hour {
		t.Fatal("DST", rr.Sub(l))
	}
	rows := alignHistoricalOverview([]HistoricalOverviewRow{{Key: "x", Left: &HistoricalOverviewCell{Currency: "USD", NativeAmount: historicalString("2")}}}, []HistoricalOverviewRow{{Key: "x", Left: &HistoricalOverviewCell{Currency: "CNY", NativeAmount: historicalString("2")}}}, true)
	if rows[0].NativeChange != nil {
		t.Fatal("unlike currencies compared")
	}
}
func TestFinancialComparisonSingleCaptureWithConcurrentWrite(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "A", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	repo := &delayedContextRepository{Repository: s.repository, FinancialContextRepository: s.repository.(FinancialContextRepository), captured: make(chan struct{}), release: make(chan struct{})}
	reader := NewService(repo)
	reader.SetClock(s.clock)
	result := make(chan FinancialComparisonResult, 1)
	errs := make(chan error, 1)
	go func() {
		r, err := reader.BuildFinancialComparison(context.Background(), FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "current"})
		result <- r
		errs <- err
	}()
	<-repo.captured
	if _, err := s.AppendAccountValue(t.Context(), a.Account.ID, "200", ""); err != nil {
		t.Fatal(err)
	}
	close(repo.release)
	r := <-result
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, r.Content.Left.Summary.Assets.Value, "100")
	wantOverviewAmount(t, r.Content.Right.Summary.Assets.Value, "100")
	wantOverviewAmount(t, r.Content.Change.NetWorth.Value, "0")
	wantOverviewAmount(t, comparisonFor(t, s, "2026-08-01", "current").Content.Change.NetWorth.Value, "100")
}

func TestFinancialComparisonPositionTransferAndMissingPrice(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "From", "brokerage", "asset", "holdings", "CNY", "")
	b := overviewAccount(t, s, owner, "To", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(t.Context(), i.ID, "5", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: b.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{h.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err = s.RecordChange(t.Context(), domain.PositionTransferInput{HouseholdID: a.Account.HouseholdID, FromHoldingID: h.ID, ToHoldingID: target.ID, Quantity: mustQuantity(t, "2"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err = db.SQL.Exec("DELETE FROM instrument_quotes"); err != nil {
		t.Fatal(err)
	}
	missing := comparisonFor(t, s, "2026-08-01", "2026-08-02")
	if missing.Content.Change.Investments.Value != nil {
		t.Fatal("missing price became known")
	}
	if _, err = s.AppendManualInstrumentQuote(t.Context(), i.ID, "5", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	full := comparisonFor(t, s, "2026-08-01", "2026-08-02")
	wantOverviewAmount(t, full.Content.Change.NetWorth.Value, "0")
	wantOverviewAmount(t, full.Content.Left.Investments.Value, "50")
	wantOverviewAmount(t, full.Content.Right.Investments.Value, "50")
	from := comparisonFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	to := comparisonFor(t, s, "2026-08-01", "2026-08-02", b.Account.ID)
	wantOverviewAmount(t, from.Content.Change.Investments.Value, "-10")
	wantOverviewAmount(t, to.Content.Change.Investments.Value, "10")
	for _, r := range []FinancialComparisonResult{from, to} {
		for _, p := range r.Content.Positions {
			if p.Kind == "holding" {
				if p.Left.Ref != p.Right.Ref {
					t.Fatal(p)
				}
			}
		}
		if r.Content.Left.Scope.AccountCount != 1 || r.Content.Right.Scope.AccountCount != 1 {
			t.Fatal("scope widened")
		}
	}
}
func TestFinancialComparisonRetainedCorrectionAndNamedDisclosure(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Chosen name", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	command := domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "20", "CNY"), Reason: domain.ReasonOther, EffectiveAt: *now}
	change, err := s.RecordChange(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	before := comparisonFor(t, s, "2026-08-02", "current")
	command.Amount = mustMoney(t, "30", "CNY")
	command.EffectiveAt = *now
	if _, err = s.FixChange(t.Context(), change.Activity.ID, command); err != nil {
		t.Fatal(err)
	}
	after := comparisonFor(t, s, "2026-08-02", "current")
	wantOverviewAmount(t, after.Content.Left.Summary.NetWorth.Value, "130")
	wantOverviewAmount(t, after.Content.Right.Summary.NetWorth.Value, "130")
	if after.ContentHash == before.ContentHash {
		t.Fatal("correction hidden from hash")
	}
	named, err := s.BuildFinancialComparison(t.Context(), FinancialComparisonRequest{LeftAsOf: "2026-08-02", RightAsOf: "current", Disclosure: "named"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range named.Content.Positions {
		if p.Ref == a.Account.ID.String() {
			found = true
			if p.Left.Name != "Chosen name" || p.Right.Name != "Chosen name" {
				t.Fatal(p)
			}
		}
	}
	if !found {
		t.Fatal("named identity absent")
	}
}

func TestFinancialComparisonExclusionIsNotAValueDelta(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.UpdateAccount(t.Context(), a.Account.ID, AccountInput{Name: "Cash", IncludeInNetWorthSet: true, IncludeInNetWorth: false}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	r := comparisonFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantOverviewAmount(t, r.Content.Change.NetWorth.Value, "-100")
	for _, p := range r.Content.Positions {
		if p.Left == nil || p.Right == nil || !p.Left.Included || p.Right.Included || p.Right.ExclusionReason != "not_in_net_worth" {
			t.Fatal(p)
		}
		wantOverviewAmount(t, p.BaseChange, "0")
		if !p.Changed {
			t.Fatal("inclusion change not marked")
		}
	}
	same := comparisonFor(t, s, "2026-08-02", "2026-08-02", a.Account.ID)
	wantOverviewAmount(t, same.Content.Change.NetWorth.Value, "0")
	for _, p := range same.Content.Positions {
		if p.Changed {
			t.Fatal("identical sides changed")
		}
	}
}
