package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func assertReconciliation(t *testing.T, result HistoricalOverviewResult, supported, residual string) *HistoricalReconciliation {
	t.Helper()
	r := result.Reconciliation
	if r == nil {
		t.Fatal("missing reconciliation")
	}
	wantOverviewAmount(t, r.Supported, supported)
	wantOverviewAmount(t, r.Residual, residual)
	total := decimal.RequireFromString(*r.Total)
	sum := decimal.Zero
	for _, c := range r.Contributions {
		sum = sum.Add(decimal.RequireFromString(c.Amount))
	}
	if !sum.Equal(decimal.RequireFromString(*r.Supported)) || !sum.Add(decimal.RequireFromString(*r.Residual)).Equal(total) {
		t.Fatalf("identity failed: %+v", r)
	}
	return r
}
func reconciliationBucket(r *HistoricalReconciliation, key string) string {
	for _, c := range r.Contributions {
		if c.Key == key {
			return c.Amount
		}
	}
	return "0"
}
func TestHistoricalReconciliationRetainedCorrectionsAndReversal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Bank", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	input := domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "20", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.AppendAccountValue(ctx, a.Account.ID, "150", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	input.Amount = mustMoney(t, "30", "CNY")
	input.EffectiveAt = *now
	fix, err := s.FixChange(ctx, change.Activity.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.UndoChange(ctx, fix.Activity.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ a, b, supported, residual, income, adjustment string }{
		{"2026-08-01", "2026-08-02", "30", "0", "30", "0"},
		{"2026-08-02", "2026-08-03", "20", "0", "0", "20"},
		{"2026-08-03", "2026-08-05", "-30", "0", "-30", "0"},
		{"2026-08-05", "2026-08-01", "-20", "0", "0", "-20"},
		{"2026-08-02", "2026-08-02", "0", "0", "0", "0"},
	} {
		r, err := s.HistoricalOverview(ctx, tc.a, tc.b)
		if err != nil {
			t.Fatal(err)
		}
		reconciliation := assertReconciliation(t, r, tc.supported, tc.residual)
		if reconciliationBucket(reconciliation, "income") != tc.income || reconciliationBucket(reconciliation, "adjustment") != tc.adjustment {
			t.Fatalf("categories: %+v", reconciliation)
		}
	}
}
func TestHistoricalReconciliationInternalTransfersDebtAndFees(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "A", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "B", "bank_account", "asset", "balance", "CNY", "0")
	d := overviewAccount(t, s, owner, "Debt", "loan", "liability", "balance", "CNY", "50")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	fee := mustMoney(t, "0.0001", "CNY")
	if _, err := s.RecordChange(ctx, domain.CashTransferInput{HouseholdID: a.Account.HouseholdID, FromAccountID: a.Account.ID, ToAccountID: b.Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), Fee: &fee, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	interest := mustMoney(t, "1", "CNY")
	if _, err := s.RecordChange(ctx, domain.DebtPaymentInput{HouseholdID: a.Account.HouseholdID, DebtAccountID: d.Account.ID, CashAccountID: a.Account.ID, Principal: mustMoney(t, "10", "CNY"), InterestOrFee: &interest, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	r, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	rec := assertReconciliation(t, r, "-1.0001", "0")
	if reconciliationBucket(rec, "external_flow") != "0" || reconciliationBucket(rec, "fee") != "-0.0001" || reconciliationBucket(rec, "spending") != "-1" || len(rec.Reasons) != 0 {
		t.Fatalf("neutral principals: %+v", rec)
	}
}

func bridgeCell(native, base, currency, rate string) *HistoricalOverviewCell {
	c := &HistoricalOverviewCell{Included: true, Complete: true, Status: "active", NativeAmount: historicalString(native), BaseAmount: historicalString(base), Currency: currency}
	if currency != "CNY" {
		c.FX = &HistoricalOverviewEvidence{Source: "manual", Value: historicalString(rate), Operation: "multiply"}
	}
	return c
}
func bridgeOverview(rows []HistoricalOverviewRow, total string) HistoricalOverviewResult {
	a := HistoricalOverviewState{Currency: "CNY", CutoffAt: "2026-08-01T23:59:59.999999999Z"}
	b := a
	b.CutoffAt = "2026-08-02T23:59:59.999999999Z"
	return HistoricalOverviewResult{Left: a, Right: &b, Change: &HistoricalOverviewTotalsChange{NetWorth: historicalString(total)}, Rows: rows}
}
func runBridge(t *testing.T, b domain.HistoricalSnapshotBatch, o HistoricalOverviewResult, supported, residual string) *HistoricalReconciliation {
	t.Helper()
	before, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	o.Reconciliation, err = historicalReconciliation(context.Background(), &b, o)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("mutated immutable batch")
	}
	return assertReconciliation(t, o, supported, residual)
}
func TestHistoricalReconciliationMultiCurrencyIncomePriceAndFX(t *testing.T) {
	t.Parallel()
	id := domain.NewAccountID()
	h := domain.NewHoldingID()
	money := mustMoney(t, "10", "USD")
	rows := []HistoricalOverviewRow{
		{Key: id.String() + ":balance", AccountID: id.String(), Kind: "balance", Role: "asset", Left: bridgeCell("100", "700", "USD", "7"), Right: bridgeCell("110", "880", "USD", "8")},
		{Key: "holding", AccountID: id.String(), HoldingID: h.String(), Kind: "holding", Role: "asset", Left: bridgeCell("20", "140", "USD", "7"), Right: bridgeCell("24", "192", "USD", "8")},
	}
	for _, c := range []*HistoricalOverviewCell{rows[1].Left, rows[1].Right} {
		c.Quantity = historicalString("2")
		c.Price = &HistoricalOverviewEvidence{Source: "manual", Currency: "USD", PriceBasis: "raw", Value: historicalString("10")}
	}
	rows[1].Right.Price.Value = historicalString("12")
	batch := domain.HistoricalSnapshotBatch{Portfolio: domain.PortfolioSnapshot{Accounts: []domain.AccountRecord{{Account: domain.Account{ID: id}}}}, Activities: []domain.Activity{{ID: domain.NewActivityID(), Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), Effects: []domain.ActivityEffect{{AccountID: &id, Money: &money, Direction: domain.EffectAdded}}}}}
	rec := runBridge(t, batch, bridgeOverview(rows, "232"), "232", "0")
	if reconciliationBucket(rec, "income") != "80" || reconciliationBucket(rec, "price_change") != "32" || reconciliationBucket(rec, "fx_impact") != "120" {
		t.Fatalf("FX/price overlap: %+v", rec)
	}
	reverse := bridgeOverview(rows, "-232")
	reverse.Left, *reverse.Right = *reverse.Right, reverse.Left
	for i := range reverse.Rows {
		reverse.Rows[i].Left, reverse.Rows[i].Right = reverse.Rows[i].Right, reverse.Rows[i].Left
	}
	rec = runBridge(t, batch, reverse, "-232", "0")
	if reconciliationBucket(rec, "income") != "-80" || reconciliationBucket(rec, "price_change") != "-32" || reconciliationBucket(rec, "fx_impact") != "-120" {
		t.Fatal(rec)
	}
}
func TestHistoricalReconciliationForeignTransferResidualAndZeroUncertainty(t *testing.T) {
	t.Parallel()
	a, b := domain.NewAccountID(), domain.NewAccountID()
	sent, received := mustMoney(t, "70", "CNY"), mustMoney(t, "10", "USD")
	rows := []HistoricalOverviewRow{
		{Key: a.String() + ":balance", AccountID: a.String(), Kind: "balance", Role: "asset", Left: bridgeCell("100", "100", "CNY", ""), Right: bridgeCell("30", "30", "CNY", "")},
		{Key: b.String() + ":balance", AccountID: b.String(), Kind: "balance", Role: "asset", Left: bridgeCell("10", "70", "USD", "7"), Right: bridgeCell("20", "160", "USD", "8")},
	}
	batch := domain.HistoricalSnapshotBatch{Portfolio: domain.PortfolioSnapshot{Accounts: []domain.AccountRecord{{Account: domain.Account{ID: a}}, {Account: domain.Account{ID: b}}}}, Activities: []domain.Activity{{Kind: domain.ActivityCashTransfer, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), Effects: []domain.ActivityEffect{{AccountID: &a, Money: &sent, Direction: domain.EffectRemoved, Classification: domain.ClassificationInternalTransfer}, {AccountID: &b, Money: &received, Direction: domain.EffectAdded, Classification: domain.ClassificationInternalTransfer}}}}}
	rec := runBridge(t, batch, bridgeOverview(rows, "20"), "0", "20")
	if !slices.Contains(rec.Reasons, "foreign_internal_exposure") || !slices.Contains(rec.Reasons, "internal_imbalance") || reconciliationBucket(rec, "external_flow") != "0" {
		t.Fatal(rec)
	}
	// Net zero does not hide a transient scope change or suggest causality.
	batch.AccountStateObservations = []domain.AccountStateObservation{{AccountID: a, EffectiveAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)}}
	rec = runBridge(t, batch, bridgeOverview(rows, "0"), "0", "0")
	if rec.Status != "partial" || !slices.Contains(rec.Reasons, "scope_changed") {
		t.Fatal(rec)
	}
}
func TestHistoricalReconciliationUnavailableBudgetsAndCancellation(t *testing.T) {
	t.Parallel()
	o := bridgeOverview(nil, "0")
	o.Change.NetWorth = nil
	rec, err := historicalReconciliation(context.Background(), &domain.HistoricalSnapshotBatch{}, o)
	if err != nil || rec.Residual != nil || rec.Supported != nil || rec.Total != nil || !slices.Contains(rec.Reasons, "incomplete_endpoints") {
		t.Fatalf("unknown: %+v %v", rec, err)
	}
	o = bridgeOverview(nil, "0")
	o.Right.Current = true
	rec, err = historicalReconciliation(context.Background(), &domain.HistoricalSnapshotBatch{}, o)
	if err != nil || rec.Residual != nil || !slices.Contains(rec.Reasons, "current_endpoint") {
		t.Fatal(rec, err)
	}
	o = bridgeOverview(nil, "1")
	batch := domain.HistoricalSnapshotBatch{Activities: make([]domain.Activity, historicalReconciliationMaxWork+1)}
	rec, err = historicalReconciliation(context.Background(), &batch, o)
	if err != nil || rec.Residual != nil || !slices.Contains(rec.Reasons, "budget_exceeded") {
		t.Fatal(rec, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = historicalReconciliation(ctx, &batch, o); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestHistoricalReconciliationUnchangedDebtFXAndMissingQuotes(t *testing.T) {
	t.Parallel()
	rows := []HistoricalOverviewRow{{Key: "debt", Kind: "balance", Role: "liability", Left: bridgeCell("10", "70", "USD", "7"), Right: bridgeCell("10", "80", "USD", "8")}}
	rec := runBridge(t, domain.HistoricalSnapshotBatch{}, bridgeOverview(rows, "-10"), "-10", "0")
	if reconciliationBucket(rec, "fx_impact") != "-10" {
		t.Fatal(rec)
	}
	rows[0].Right.FX = nil
	rec = runBridge(t, domain.HistoricalSnapshotBatch{}, bridgeOverview(rows, "-10"), "0", "-10")
	if !slices.Contains(rec.Reasons, "missing_evidence") {
		t.Fatal(rec)
	}
}

func TestHistoricalReconciliationRoundTripQuantityAndTinyResidual(t *testing.T) {
	t.Parallel()
	id, h := domain.NewAccountID(), domain.NewHoldingID()
	row := HistoricalOverviewRow{Key: "h", AccountID: id.String(), HoldingID: h.String(), Kind: "holding", Role: "asset", Left: bridgeCell("1", "1", "CNY", ""), Right: bridgeCell("1.000000000001", "1.000000000001", "CNY", "")}
	row.Left.Quantity = historicalString("1")
	row.Right.Quantity = historicalString("1")
	row.Left.Price = &HistoricalOverviewEvidence{Source: "manual", Currency: "CNY", Value: historicalString("1")}
	row.Right.Price = row.Left.Price
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	batch := domain.HistoricalSnapshotBatch{Activities: []domain.Activity{{EffectiveAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), Effects: []domain.ActivityEffect{{HoldingID: &h, Quantity: &quantity, Direction: domain.EffectAdded}, {HoldingID: &h, Quantity: &quantity, Direction: domain.EffectRemoved}}}}}
	r := runBridge(t, batch, bridgeOverview([]HistoricalOverviewRow{row}, "0.000000000001"), "0", "0.000000000001")
	if !slices.Contains(r.Reasons, "quantity_activity") || reconciliationBucket(r, "price_change") != "0" {
		t.Fatal(r)
	}
}
func TestHistoricalReconciliationCivilCutoffsAndDivisionPrecision(t *testing.T) {
	t.Parallel()
	id := domain.NewAccountID()
	money := mustMoney(t, "0.0001", "USD")
	row := HistoricalOverviewRow{Key: id.String() + ":balance", AccountID: id.String(), Kind: "balance", Role: "asset", Left: bridgeCell("1", "0.3333333333333333", "USD", "3"), Right: bridgeCell("1.0001", "0.250025", "USD", "4")}
	row.Left.FX.Operation = "divide"
	row.Right.FX.Operation = "divide"
	o := bridgeOverview([]HistoricalOverviewRow{row}, "-0.0833083333333333")
	// A DST civil day is 23 hours; selection uses instants, not fixed durations.
	o.Left.CutoffAt = "2026-03-08T04:59:59.999999999Z"
	o.Right.CutoffAt = "2026-03-09T03:59:59.999999999Z"
	cutoff, _ := time.Parse(time.RFC3339Nano, o.Left.CutoffAt)
	batch := domain.HistoricalSnapshotBatch{Portfolio: domain.PortfolioSnapshot{Accounts: []domain.AccountRecord{{Account: domain.Account{ID: id}}}}, Activities: []domain.Activity{
		{Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, EffectiveAt: cutoff, Effects: []domain.ActivityEffect{{AccountID: &id, Money: &money, Direction: domain.EffectAdded}}},
		{Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, EffectiveAt: cutoff.Add(time.Nanosecond), Effects: []domain.ActivityEffect{{AccountID: &id, Money: &money, Direction: domain.EffectAdded}}},
	}}
	r := runBridge(t, batch, o, "-0.0833083333333333", "0")
	if reconciliationBucket(r, "income") != "0.000025" {
		t.Fatal("included the start boundary twice", r)
	}
}
func TestHistoricalReconciliationMissingQuoteAndScopeReasons(t *testing.T) {
	t.Parallel()
	row := HistoricalOverviewRow{Key: "holding", Kind: "holding", Role: "asset", Left: bridgeCell("10", "10", "CNY", ""), Right: bridgeCell("11", "11", "CNY", "")}
	row.Left.Quantity = historicalString("1")
	row.Right.Quantity = historicalString("1")
	r := runBridge(t, domain.HistoricalSnapshotBatch{}, bridgeOverview([]HistoricalOverviewRow{row}, "1"), "0", "1")
	if !slices.Contains(r.Reasons, "missing_evidence") {
		t.Fatal(r)
	}
	row.Right.Included = false
	r = runBridge(t, domain.HistoricalSnapshotBatch{}, bridgeOverview([]HistoricalOverviewRow{row}, "-10"), "0", "-10")
	if !slices.Contains(r.Reasons, "scope_changed") {
		t.Fatal(r)
	}
}

func TestHistoricalReconciliationSQLiteMultiCurrencyTransfer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "CNY bank", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "USD bank", "bank_account", "asset", "balance", "USD", "10")
	if _, err := s.SetFXPreference(ctx, "CNY", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "7", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "8", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(ctx, domain.CashTransferInput{HouseholdID: a.Account.HouseholdID, FromAccountID: a.Account.ID, ToAccountID: b.Account.ID, Sent: mustMoney(t, "70", "CNY"), Received: mustMoney(t, "10", "USD"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	r, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	rec := assertReconciliation(t, r, "0", "20")
	if !slices.Contains(rec.Reasons, "foreign_internal_exposure") || reconciliationBucket(rec, "external_flow") != "0" {
		t.Fatal(rec)
	}
	reverse, err := s.HistoricalOverview(ctx, "2026-08-02", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	assertReconciliation(t, reverse, "0", "-20")
}
