package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func overviewFixture(t *testing.T) (*Service, *sqlite.DB, domain.MemberID, *time.Time) {
	t.Helper()
	db, err := sqlite.Open(t.TempDir() + "/overview.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(context.Background(), OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s, db, b.Members[0].ID, &now
}

func overviewAccount(t *testing.T, s *Service, owner domain.MemberID, name, kind, role, mode, currency, amount string) domain.AccountRecord {
	t.Helper()
	a, err := s.CreateAccount(context.Background(), AccountInput{Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: currency, InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func overviewRow(t *testing.T, result HistoricalOverviewResult, key string) HistoricalOverviewRow {
	t.Helper()
	for _, row := range result.Rows {
		if row.Key == key {
			return row
		}
	}
	t.Fatalf("row missing: %s", key)
	return HistoricalOverviewRow{}
}

func wantOverviewAmount(t *testing.T, value *string, want string) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("amount=%v want %s", value, want)
	}
}

func TestHistoricalOverviewReadOnlyLifecycleAndSourceDates(t *testing.T) {
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	home := overviewAccount(t, s, owner, "Home", "property", "asset", "manual_value", "CNY", "1000")
	debt := overviewAccount(t, s, owner, "Loan", "loan", "liability", "balance", "CNY", "250")
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.AppendAccountValue(ctx, home.Account.ID, "1200", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if err := s.ArchiveAccount(ctx, debt.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	newAccount := overviewAccount(t, s, owner, "New zero", "bank_account", "asset", "balance", "CNY", "0")
	if _, err := s.UpdateAccount(ctx, home.Account.ID, AccountInput{Name: "Renamed home"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	var changesBefore, changesAfter int
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changesBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	result, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, result.Left.NetWorth, "750")
	wantOverviewAmount(t, result.Right.NetWorth, "1200")
	row := overviewRow(t, result, home.Account.ID.String())
	if row.Name != "Renamed home" || !row.Left.Manual || row.Left.ValueSourceAt != "2026-08-01T12:00:00Z" || row.Right.ValueSourceAt != "2026-08-02T12:00:00Z" {
		t.Fatalf("manual metadata/source: %+v / %+v", row.Left, row.Right)
	}
	wantOverviewAmount(t, row.BaseChange, "200")
	loan := overviewRow(t, result, debt.Account.ID.String())
	if !loan.Left.Included || loan.Right.Included || loan.Right.Status != "archived" {
		t.Fatalf("archive semantics: %+v", loan)
	}
	wantOverviewAmount(t, loan.Right.BaseAmount, "250")
	zero := overviewRow(t, result, newAccount.Account.ID.String())
	if zero.Left != nil || zero.Right.Status != "zero" || zero.BaseChange != nil {
		t.Fatalf("absence is not zero: %+v", zero)
	}
	current, err := s.HistoricalOverview(ctx, "2026-08-03", "current")
	if err != nil {
		t.Fatal(err)
	}
	if !current.Right.Current || current.Right.CutoffAt != now.UTC().Format(time.RFC3339Nano) {
		t.Fatal("current snapshot is not captured")
	}
	wantOverviewAmount(t, current.Right.NetWorth, "1200")
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changesAfter); err != nil {
		t.Fatal(err)
	}
	if changesBefore != changesAfter {
		t.Fatal("browsing wrote to database")
	}
}

func TestHistoricalOverviewUnknownFXAndExactPrecision(t *testing.T) {
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	foreign := overviewAccount(t, s, owner, "USD", "bank_account", "asset", "balance", "USD", "10")
	unknown := overviewAccount(t, s, owner, "Unknown", "property", "asset", "manual_value", "CNY", "0")
	if _, err := db.SQL.Exec(`DELETE FROM account_values WHERE account_id = ?`, unknown.Account.ID.String()); err != nil {
		t.Fatal(err)
	}
	broker := overviewAccount(t, s, owner, "Broker", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Dust", Type: "crypto", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "0.00006"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	result, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	if result.Left.Complete || result.Left.NetWorth != nil {
		t.Fatal("incomplete household became complete")
	}
	usd := overviewRow(t, result, foreign.Account.ID.String())
	wantOverviewAmount(t, usd.Left.NativeAmount, "10")
	if usd.Left.BaseAmount != nil || usd.Left.Complete {
		t.Fatal("missing FX replaced")
	}
	u := overviewRow(t, result, unknown.Account.ID.String())
	if u.Left.NativeAmount != nil || u.Left.Status != "unknown" {
		t.Fatal("missing balance became zero")
	}
	row := overviewRow(t, result, broker.Account.ID.String()+":holding:"+h.ID.String())
	wantOverviewAmount(t, row.Left.BaseAmount, "0.00012")
	wantOverviewAmount(t, row.BaseChange, "0")
	if result.Left.KnownAssets != "0.00012" || row.Left.Price == nil || row.Left.Price.Source != "manual" {
		t.Fatalf("precision/evidence: %+v", result.Left)
	}
}

func TestHistoricalOverviewCorrectionsUseEconomicTime(t *testing.T) {
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	command := domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "20", "CNY"), Reason: domain.ReasonOther, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err = s.AppendAccountValue(ctx, a.Account.ID, "150", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	command.Amount = mustMoney(t, "30", "CNY")
	command.EffectiveAt = *now
	if _, err = s.FixChange(ctx, change.Activity.ID, command); err != nil {
		t.Fatal(err)
	}
	result, err := s.HistoricalOverview(ctx, "2026-08-02", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, result.Left.NetWorth, "130")
	wantOverviewAmount(t, result.Right.NetWorth, "150")
}

func TestHistoricalOverviewDateValidationAndDST(t *testing.T) {
	ctx := context.Background()
	s, _, _, now := overviewFixture(t)
	if _, err := s.HistoricalOverview(ctx, "2026-08-01", ""); err == nil {
		t.Fatal("history not started accepted")
	}
	*now = time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	if _, err := s.StartHistory(ctx, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	result, err := s.HistoricalOverview(ctx, "2026-03-07", "2026-03-08")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := time.Parse(time.RFC3339Nano, result.Left.CutoffAt)
	b, _ := time.Parse(time.RFC3339Nano, result.Right.CutoffAt)
	if b.Sub(a) != 23*time.Hour {
		t.Fatalf("DST day: %s", b.Sub(a))
	}
	for _, date := range []string{"2026-03-06", "2026-03-10", "2026-02-30", "2026-3-08", "current"} {
		if _, err := s.HistoricalOverview(ctx, date, ""); err == nil {
			t.Fatalf("accepted date %s", date)
		}
	}
	if _, err := s.HistoricalOverview(ctx, "2026-03-08", "2026-03-10"); err == nil {
		t.Fatal("open compare day accepted")
	}
}

type overviewBatchRepository struct {
	Repository
	loads     int
	batch     domain.HistoricalSnapshotBatch
	afterLoad func()
}

func (r *overviewBatchRepository) LoadHistoricalSnapshotBatch(ctx context.Context, id domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
	r.loads++
	b, err := r.Repository.LoadHistoricalSnapshotBatch(ctx, id, cutoff)
	r.batch = b
	if r.afterLoad != nil {
		r.afterLoad()
	}
	return b, err
}

func TestHistoricalOverviewCurrentIsCapturedWithHistoricalInputs(t *testing.T) {
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "10")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	writer := NewService(s.repository)
	writer.setClock(func() time.Time { return *now })
	repo := &overviewBatchRepository{Repository: s.repository}
	repo.afterLoad = func() {
		if _, err := writer.AppendAccountValue(ctx, a.Account.ID, "20", ""); err != nil {
			t.Fatal(err)
		}
	}
	s.repository = repo
	result, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, result.Left.NetWorth, "10")
	wantOverviewAmount(t, result.Right.NetWorth, "10")
	if repo.loads != 1 {
		t.Fatal("multiple batch reads")
	}
	repo.afterLoad = nil
	refreshed, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, refreshed.Right.NetWorth, "20")
}

func TestHistoricalOverviewPriceGapsBackfillClearingAndArchive(t *testing.T) {
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Broker", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "5", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a retained position whose old pricing evidence is unavailable.
	if _, err = db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id = ?`, i.ID.String()); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "10", "2026-08-03", false); err != nil {
		t.Fatal(err)
	}
	gap, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil {
		t.Fatal(err)
	}
	key := a.Account.ID.String() + ":holding:" + h.ID.String()
	row := overviewRow(t, gap, key)
	wantOverviewAmount(t, row.Left.Quantity, "2")
	if row.Left.NativeAmount != nil || row.Left.Complete || len(row.Left.Missing) == 0 {
		t.Fatalf("price gap: %+v", row.Left)
	}
	wantOverviewAmount(t, row.Right.NativeAmount, "20")
	// A quote recorded today but economically effective in the past repairs
	// reconstruction without pretending it was known on that historical date.
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "5", "2026-08-01T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	filled, err := s.HistoricalOverview(ctx, "2026-08-01", "")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, filled.Left.NetWorth, "10")
	if _, err = s.RecordChange(ctx, domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeSell, SettlementAccountID: a.Account.ID, HoldingID: h.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "20", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	cleared, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	row = overviewRow(t, cleared, key)
	if row.Right.Status != "cleared" {
		t.Fatalf("zero quantity not cleared: %+v", row.Right)
	}
	wantOverviewAmount(t, row.QuantityChange, "-2")
	wantOverviewAmount(t, cleared.Right.NetWorth, "20")
	if err = s.ArchiveHolding(ctx, h.ID, true); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	archived, err := s.HistoricalOverview(ctx, "2026-08-03", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	row = overviewRow(t, archived, key)
	if row.Left.Status != "cleared" || row.Right.Status != "archived" || row.Right.Included {
		t.Fatal("holding archive leaked backward")
	}
}

func TestHistoricalOverviewInternalTransferAndDebtPrincipalAreNeutral(t *testing.T) {
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Bank A", "bank_account", "asset", "balance", "CNY", "100")
	b := overviewAccount(t, s, owner, "Bank B", "bank_account", "asset", "balance", "CNY", "0")
	d := overviewAccount(t, s, owner, "Debt", "loan", "liability", "balance", "CNY", "50")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.RecordChange(ctx, domain.CashTransferInput{HouseholdID: a.Account.HouseholdID, FromAccountID: a.Account.ID, ToAccountID: b.Account.ID, Sent: mustMoney(t, "20", "CNY"), Received: mustMoney(t, "20", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(ctx, domain.DebtPaymentInput{HouseholdID: a.Account.HouseholdID, DebtAccountID: d.Account.ID, CashAccountID: a.Account.ID, Principal: mustMoney(t, "10", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	result, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, result.Left.NetWorth, "50")
	wantOverviewAmount(t, result.Right.NetWorth, "50")
	wantOverviewAmount(t, result.Right.Assets, "90")
	wantOverviewAmount(t, result.Right.Liabilities, "40")
}

func TestHistoricalOverviewChangesIgnoreNewEvidenceWithSameBalance(t *testing.T) {
	a := HistoricalOverviewCell{Status: "active", Included: true, Complete: true, NativeAmount: historicalString("1"), BaseAmount: historicalString("1"), ValueSourceID: "old", ValueSourceAt: "yesterday", Price: &HistoricalOverviewEvidence{ID: "old"}}
	b := a
	b.ValueSourceID = "new"
	b.ValueSourceAt = "today"
	b.Price = &HistoricalOverviewEvidence{ID: "new"}
	if !historicalCellsEqual(&a, &b) {
		t.Fatal("new source is not a balance change")
	}
	b.Included = false
	if historicalCellsEqual(&a, &b) {
		t.Fatal("scope change was hidden")
	}
}

func TestHistoricalOverviewSameDayComparisonSharesOneUnmodifiedBatch(t *testing.T) {
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	overviewAccount(t, s, owner, "Balance", "bank_account", "asset", "balance", "CNY", "10")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	repo := &overviewBatchRepository{Repository: s.repository}
	s.repository = repo
	result, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if repo.loads != 1 || !reflect.DeepEqual(result.Left, *result.Right) {
		t.Fatal("comparison used inconsistent reads")
	}
	for _, row := range result.Rows {
		if row.Changed {
			t.Fatalf("same-day change: %+v", row)
		}
	}
	if repo.batch.Portfolio.Accounts[0].LatestValue.Amount.CanonicalAmount() != "10" {
		t.Fatal("batch changed")
	}
}

func TestHistoricalOverviewMarketDateCloseAndCoverage(t *testing.T) {
	// Singapore's household day ends before the US market close. The existing
	// daily-summary policy still prices its market-date label, not one instant.
	ctx := context.Background()
	s, _, _, _ := overviewFixture(t)
	accountID, instrumentID, holdingID := domain.NewAccountID(), domain.NewInstrumentID(), domain.NewHoldingID()
	provider := domain.TiingoProviderKey
	originAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 9, 15, 59, 59, 999000000, time.UTC)
	quantity := mustQuantity(t, "10")
	account := domain.Account{ID: accountID, Name: "US broker", AccountType: "brokerage", TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true, CreatedAt: originAt}
	instrument := domain.Instrument{ID: instrumentID, Name: "Stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderBindingRevision: 1, CreatedAt: originAt}
	quote := func(price, date string, at time.Time) domain.InstrumentQuote {
		unitPrice, err := domain.ParseUnitPrice(price)
		if err != nil {
			t.Fatal(err)
		}
		return domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitPrice, Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: date, ValueEffectiveAt: at, QuotedAt: at.Add(48 * time.Hour), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose), Revision: 1}
	}
	batch := domain.HistoricalSnapshotBatch{
		Origin:                    domain.HistoryOrigin{StartedAt: originAt, Timezone: "Asia/Singapore"},
		OriginData:                domain.HistoryOriginData{Components: []domain.HistoryOriginComponent{{Kind: domain.HistoryOriginHoldingQuantity, AccountID: &accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}}},
		Portfolio:                 domain.PortfolioSnapshot{Household: &domain.Household{BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity, CreatedAt: originAt}}},
		InstrumentQuoteFacts:      []domain.InstrumentQuote{quote("180", "2026-09-08", cutoff.Add(-20*time.Hour))},
		InstrumentHistoryCoverage: []domain.InstrumentHistoryCoverage{{InstrumentID: instrumentID, ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: string(PriceBasisTiingoRawClose), CloseMarketDates: []string{"2026-09-08"}, UnverifiedDates: []string{"2026-09-09"}}},
	}
	state, rows, err := s.historicalOverviewSide(ctx, &batch, "2026-09-09", cutoff, false, "", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if state.Complete || state.NetWorth != nil || state.KnownAssets != "1800" {
		t.Fatalf("pending close: %+v", state)
	}
	var pending *HistoricalOverviewCell
	for _, row := range rows {
		if row.Kind == "holding" {
			pending = row.Left
		}
	}
	if pending == nil || pending.Complete || !reflect.DeepEqual(pending.Missing, []string{string(domain.MissingHistoryCoverage)}) {
		t.Fatalf("missing coverage hidden: %+v", pending)
	}
	late := quote("186", "2026-09-09", cutoff.Add(4*time.Hour))
	batch.InstrumentQuoteFacts = append(batch.InstrumentQuoteFacts, late)
	batch.InstrumentHistoryCoverage[0].CloseMarketDates = append(batch.InstrumentHistoryCoverage[0].CloseMarketDates, "2026-09-09")
	batch.InstrumentHistoryCoverage[0].UnverifiedDates = nil
	state, rows, err = s.historicalOverviewSide(ctx, &batch, "2026-09-09", cutoff, false, "", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, state.NetWorth, "1860")
	for _, row := range rows {
		if row.Kind == "holding" && (row.Left.Price == nil || row.Left.Price.MarketDate != "2026-09-09" || row.Left.Price.EffectiveAt != late.ValueEffectiveAt.Format(time.RFC3339Nano) || row.Left.Price.Freshness != "fresh") {
			t.Fatalf("late-close provenance: %+v", row.Left.Price)
		}
	}
}

func TestHistoricalOverviewCivilDayBoundaries(t *testing.T) {
	for _, tc := range []struct{ zone, date, cutoff string }{
		{"America/Santiago", "2026-09-05", "2026-09-06T03:59:59.999Z"},
		{"America/Santiago", "2026-09-06", "2026-09-07T02:59:59.999Z"},
		{"America/Santiago", "2026-04-04", "2026-04-05T03:59:59.999Z"},
		{"America/Havana", "2026-11-01", "2026-11-02T04:59:59.999Z"},
		{"Pacific/Apia", "2011-12-29", "2011-12-30T09:59:59.999Z"},
	} {
		t.Run(tc.zone+"/"+tc.date, func(t *testing.T) {
			s, _, _, now := overviewFixture(t)
			day, err := time.Parse("2006-01-02", tc.date)
			if err != nil {
				t.Fatal(err)
			}
			*now = day.AddDate(0, 0, -2).Add(12 * time.Hour)
			if _, err := s.StartHistory(context.Background(), tc.zone); err != nil {
				t.Fatal(err)
			}
			*now = day.AddDate(0, 0, 3).Add(12 * time.Hour)
			result, err := s.HistoricalOverview(context.Background(), tc.date, tc.date)
			if err != nil {
				t.Fatal(err)
			}
			if result.Left.CutoffAt != tc.cutoff || result.Right.CutoffAt != tc.cutoff {
				t.Fatalf("cutoffs %s / %s; want %s", result.Left.CutoffAt, result.Right.CutoffAt, tc.cutoff)
			}
			if tc.zone == "Pacific/Apia" {
				if _, err := s.HistoricalOverview(context.Background(), "2011-12-30", ""); err == nil {
					t.Fatal("a skipped civil date was normalized instead of rejected")
				}
			}
		})
	}
}

func TestHistoricalOverviewMidnightDSTIncludesBothEconomicDays(t *testing.T) {
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Cash", "bank_account", "asset", "balance", "CNY", "100")
	*now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := s.StartHistory(ctx, "America/Santiago"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	if _, err := s.AppendAccountValue(ctx, a.Account.ID, "120", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(24 * time.Hour)
	if _, err := s.AppendAccountValue(ctx, a.Account.ID, "130", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(4 * 24 * time.Hour)
	result, err := s.HistoricalOverview(ctx, "2026-09-05", "2026-09-06")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, result.Left.NetWorth, "120")
	wantOverviewAmount(t, result.Right.NetWorth, "130")
	for _, invalid := range []string{"2026-9-06", "2026-09-31", "2026-09-06 ", "2026-09-10"} {
		if _, err := s.HistoricalOverview(ctx, invalid, ""); err == nil {
			t.Fatalf("accepted invalid date %q", invalid)
		}
		if _, err := s.HistoricalOverview(ctx, "2026-09-05", invalid); err == nil {
			t.Fatalf("accepted invalid comparison %q", invalid)
		}
	}
	// Shortly after the following midnight, label arithmetic must still expose
	// the just-closed skipped-midnight day rather than normalize it backward.
	*now = time.Date(2026, 9, 7, 3, 30, 0, 0, time.UTC)
	result, err = s.HistoricalOverview(ctx, "2026-09-06", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.LastClosedDate != "2026-09-06" {
		t.Fatalf("last closed date: %s", result.LastClosedDate)
	}

}

func TestHistoricalOverviewMissingFXStaysWithItsComponent(t *testing.T) {
	for _, exposure := range []string{"archived-only", "included-holding", "included-cash"} {
		t.Run(exposure, func(t *testing.T) {
			s, _, _, now := overviewFixture(t)
			aid := domain.NewAccountID()
			i1, i2, i3 := domain.NewInstrumentID(), domain.NewInstrumentID(), domain.NewInstrumentID()
			h1, h2, h3 := domain.NewHoldingID(), domain.NewHoldingID(), domain.NewHoldingID()
			price, err := domain.ParseUnitPrice("1")
			if err != nil {
				t.Fatal(err)
			}
			batch := domain.HistoricalSnapshotBatch{Portfolio: domain.PortfolioSnapshot{
				Household:        &domain.Household{BaseCurrency: "CNY"},
				Accounts:         []domain.AccountRecord{{Account: domain.Account{ID: aid, Name: "Broker", AccountType: "brokerage", TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true}}},
				Instruments:      []domain.Instrument{{ID: i1, Name: "Archived", Type: "stock", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual}, {ID: i2, Name: "Zero", Type: "stock", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual}, {ID: i3, Name: "Included", Type: "stock", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual}},
				Holdings:         []domain.Holding{{ID: h1, AccountID: aid, InstrumentID: i1, Quantity: mustQuantity(t, "1"), ArchivedAt: now}, {ID: h2, AccountID: aid, InstrumentID: i2, Quantity: mustQuantity(t, "0")}},
				InstrumentQuotes: []domain.InstrumentQuote{{ID: domain.NewInstrumentQuoteID(), InstrumentID: i1, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceManual, QuotedAt: *now}, {ID: domain.NewInstrumentQuoteID(), InstrumentID: i3, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceManual, QuotedAt: *now}},
			}}
			if exposure == "included-holding" {
				batch.Portfolio.Holdings = append(batch.Portfolio.Holdings, domain.Holding{ID: h3, AccountID: aid, InstrumentID: i3, Quantity: mustQuantity(t, "2")})
			}
			if exposure == "included-cash" {
				batch.Portfolio.CashValues = []domain.AccountCashValue{{AccountID: aid, Amount: mustMoney(t, "5", "USD"), EffectiveAt: *now}}
			}
			batch.Origin = domain.HistoryOrigin{StartedAt: now.Add(-24 * time.Hour), Timezone: "UTC"}
			batch.InstrumentQuoteFacts = batch.Portfolio.InstrumentQuotes
			for _, holding := range batch.Portfolio.Holdings {
				batch.OriginData.Components = append(batch.OriginData.Components, domain.HistoryOriginComponent{Kind: domain.HistoryOriginHoldingQuantity, AccountID: &aid, HoldingID: &holding.ID, InstrumentID: &holding.InstrumentID, Quantity: &holding.Quantity})
			}
			for _, cash := range batch.Portfolio.CashValues {
				batch.OriginData.Components = append(batch.OriginData.Components, domain.HistoryOriginComponent{Kind: domain.HistoryOriginAccountCash, AccountID: &aid, Amount: &cash.Amount})
			}
			for _, current := range []bool{true, false} {
				mode := "historical"
				if current {
					mode = "current"
				}
				t.Run(mode, func(t *testing.T) {
					state, rows, err := s.historicalOverviewSide(context.Background(), &batch, now.Format("2006-01-02"), *now, current, "", time.Hour)
					if err != nil {
						t.Fatal(err)
					}
					result := HistoricalOverviewResult{Rows: rows}
					zero := overviewRow(t, result, aid.String()+":holding:"+h2.String()).Left
					if !zero.Complete || len(zero.Missing) != 0 {
						t.Fatalf("known zero contaminated: %+v", zero)
					}
					wantOverviewAmount(t, zero.BaseAmount, "0")
					archived := overviewRow(t, result, aid.String()+":holding:"+h1.String()).Left
					if archived.Included || archived.Complete || !reflect.DeepEqual(archived.Missing, []string{string(domain.MissingFXRate)}) {
						t.Fatalf("lost archived FX provenance: %+v", archived)
					}
					if exposure == "archived-only" {
						if !state.Complete {
							t.Fatalf("excluded archive invalidated total: %+v", state)
						}
						wantOverviewAmount(t, state.NetWorth, "0")
					} else {
						if state.Complete || state.NetWorth != nil {
							t.Fatalf("real included FX gap suppressed: %+v", state)
						}
						key, native := aid.String()+":holding:"+h3.String(), "2"
						if exposure == "included-cash" {
							key, native = aid.String()+":cash:USD", "5"
						}
						missing := overviewRow(t, result, key).Left
						if missing.Complete || missing.BaseAmount != nil || !reflect.DeepEqual(missing.Missing, []string{string(domain.MissingFXRate)}) {
							t.Fatalf("missing included FX provenance: %+v", missing)
						}
						wantOverviewAmount(t, missing.NativeAmount, native)
					}
				})
			}
		})
	}
}

func TestHistoricalOverviewYearBoundaryProgress(t *testing.T) {
	for _, zone := range []string{"America/New_York", "America/Santiago", "Europe/London", "UTC"} {
		for _, date := range []string{"2026-12-31", "2040-12-30", "2040-12-31", "2041-01-01"} {
			t.Run(zone+"/"+date, func(t *testing.T) {
				location, err := time.LoadLocation(zone)
				if err != nil {
					t.Fatal(err)
				}
				day, err := time.Parse("2006-01-02", date)
				if err != nil {
					t.Fatal(err)
				}
				cutoff, err := historicalOverviewDayCutoff(day, location)
				if err != nil {
					// Some Go/tzdata versions return a non-advancing ZoneBounds
					// interval around the leap-year POSIX extension boundary.
					// Fail explicitly there, but allow a correct result on fixed
					// runtimes. Ordinary 2026 dates must continue to succeed.
					problem, ok := err.(*domain.Error)
					if date == "2026-12-31" || !ok || problem.Code != domain.ErrInvalidChangeTime || problem.Field != "timezone" || problem.Message != "cannot resolve the day boundary: timezone interval did not advance" || !cutoff.IsZero() {
						t.Fatalf("unexpected boundary error: %s / %v", cutoff, err)
					}
					t.Logf("explicit timezone boundary error: %v", err)
					return
				}
				if cutoff.In(location).Format("2006-01-02") != date || cutoff.Add(time.Millisecond).In(location).Format("2006-01-02") == date {
					t.Fatalf("incorrect day boundary: %s", cutoff)
				}
			})
		}
	}
}
