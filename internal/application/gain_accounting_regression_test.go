package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type accountingGainFixture struct {
	service    *Service
	repo       *sqlite.Repository
	db         *sqlite.DB
	boot       Bootstrap
	instrument domain.Instrument
	accounts   []domain.AccountRecord
	holdings   []domain.Holding
	now        time.Time
}

func newAccountingGainFixture(t *testing.T, quantity, cost string) *accountingGainFixture {
	t.Helper()
	return newAccountingGainFixtureCurrency(t, quantity, cost, "CNY")
}

func newAccountingGainFixtureCurrency(t *testing.T, quantity, cost, quoteCurrency string) *accountingGainFixture {
	t.Helper()
	db, err := sqlite.Open(t.TempDir() + "/accounting.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &accountingGainFixture{db: db, repo: sqlite.NewRepository(db), now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	f.service = NewService(f.repo)
	f.service.setClock(func() time.Time { return f.now })
	ctx := context.Background()
	if err := f.service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic accounting", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	f.boot, err = f.service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.instrument, err = f.service.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic ETF", Type: "etf", QuoteCurrency: quoteCurrency, QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B", "C"} {
		account, err := f.service.CreateAccount(ctx, AccountInput{Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{f.boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		f.accounts = append(f.accounts, account)
		q := "0"
		if name == "A" {
			q = quantity
		}
		holding, err := f.service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: f.instrument.ID.String(), Quantity: q})
		if err != nil {
			t.Fatal(err)
		}
		f.holdings = append(f.holdings, holding)
		if _, err := f.service.AppendAccountCashValue(ctx, account.Account.ID, "1000000000", quoteCurrency, "2026-09-25"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.AppendManualInstrumentQuote(ctx, f.instrument.ID, cost, "2026-09-25T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if quoteCurrency != "CNY" {
		if _, err := f.service.AppendManualFXQuote(ctx, quoteCurrency, "CNY", "6", "2026-09-25T12:00:00Z"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.SetFXPreference(ctx, quoteCurrency, "CNY", "manual"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	return f
}

func (f *accountingGainFixture) transfer(t *testing.T, from, to int, quantity string, effective time.Time) domain.ChangePreview {
	t.Helper()
	result, err := f.service.RecordChange(t.Context(), domain.PositionTransferInput{HouseholdID: f.boot.Household.ID, FromHoldingID: f.holdings[from].ID, ToHoldingID: f.holdings[to].ID, Quantity: mustQuantity(t, quantity), EffectiveAt: effective})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *accountingGainFixture) requireCost(t *testing.T, index int, quantity, cost string) {
	t.Helper()
	gain, err := f.service.HoldingGain(t.Context(), f.holdings[index].ID)
	if err != nil {
		t.Fatal(err)
	}
	if gain.Quantity != quantity || gain.TotalCost.Amount != cost {
		t.Fatalf("holding %d gain=%+v, want quantity %s cost %s", index, gain, quantity, cost)
	}
}

func TestAccountingFullTransferSourceBoundary(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "3", "10")
	f.transfer(t, 0, 1, "3", f.now)
	f.requireCost(t, 1, "3", "30")
	f.requireInstrumentCost(t, "3", "30")
}

func TestAccountingPartialTransferExcludesLaterSameTimeCostAdjustment(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "3", "10")
	effective := f.now
	f.transfer(t, 0, 1, "1", effective)
	f.requireCost(t, 1, "1", "10")
	f.now = f.now.Add(time.Hour)
	if _, err := f.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID, UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective}); err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 1, "1", "10")
	f.requireCost(t, 0, "2", "40")
	f.requireInstrumentCost(t, "3", "50")
}

func TestAccountingSameTimeTransferChainRoundTripAndMemoBoundaries(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "3", "10")
	effective := f.now
	f.transfer(t, 0, 1, "1", effective)
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 1, 2, "1", effective)
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 2, 0, "1", effective)
	f.requireCost(t, 0, "3", "30")
	f.requireCost(t, 1, "0", "0")
	f.requireCost(t, 2, "0", "0")
	f.requireInstrumentCost(t, "3", "30")
	// The second transfer from A shares an effective timestamp with the first,
	// but must resolve a distinct historical average in the same replay context.
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID, UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 0, 1, "1", effective)
	f.requireCost(t, 0, "2", "40")
	f.requireCost(t, 1, "1", "20")
	f.requireInstrumentCost(t, "3", "60")
}

func TestAccountingEarlierSameTimeBuyAndCorrectedTransfer(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "0", "10")
	effective := f.now
	buy := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "3"), Gross: mustMoney(t, "30", "CNY"), EffectiveAt: effective}
	original, err := f.service.RecordChange(t.Context(), buy)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	transfer := f.transfer(t, 0, 1, "3", effective)
	f.requireCost(t, 1, "3", "30")
	f.now = f.now.Add(time.Minute)
	buy.Gross = mustMoney(t, "60", "CNY")
	fixed, err := f.service.FixChange(t.Context(), original.Activity.ID, buy)
	if err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 1, "3", "60")
	// A correction of a correction keeps the root economic position.
	f.now = f.now.Add(time.Minute)
	buy.Gross = mustMoney(t, "90", "CNY")
	if _, err := f.service.FixChange(t.Context(), fixed.Activity.ID, buy); err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 1, "3", "90")
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.UndoChange(t.Context(), transfer.Activity.ID); err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 0, "3", "90")
	f.requireCost(t, 1, "0", "0")
}

func TestAccountingTransferCorrectionRetainsSourceBoundary(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "3", "10")
	effective := f.now
	transfer := f.transfer(t, 0, 1, "1", effective)
	f.now = f.now.Add(time.Minute)
	adjustment := domain.PositionCostAdjustmentInput{HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID, UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective}
	_, err := f.service.RecordChange(t.Context(), adjustment)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	replacement := domain.PositionTransferInput{HouseholdID: f.boot.Household.ID, FromHoldingID: f.holdings[0].ID, ToHoldingID: f.holdings[1].ID, Quantity: mustQuantity(t, "2"), EffectiveAt: effective}
	if _, err := f.service.FixChange(t.Context(), transfer.Activity.ID, replacement); err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 1, "2", "20")
	f.requireCost(t, 0, "1", "20")
}

func (f *accountingGainFixture) requireInstrumentCost(t *testing.T, quantity, cost string) {
	t.Helper()
	groups, err := f.service.InstrumentHoldings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Amounts.Quantity != quantity || groups[0].Amounts.TotalCost == nil || groups[0].Amounts.TotalCost.Amount != cost {
		t.Fatalf("instrument aggregate=%+v want quantity=%s cost=%s", groups, quantity, cost)
	}
}
