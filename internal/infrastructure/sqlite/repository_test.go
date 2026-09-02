package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// sort_order must be assigned atomically inside the INSERT transaction, so
// concurrent CreateMember calls never race on a stale read-then-write value.
func TestCreateMemberAssignsUniqueSortOrderUnderConcurrency(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)
	ctx := context.Background()

	now := time.Now()
	currency, err := domain.ParseCurrency("CNY")
	if err != nil {
		t.Fatalf("ParseCurrency: %v", err)
	}
	household, err := domain.NewHousehold("Test", currency, now)
	if err != nil {
		t.Fatalf("NewHousehold: %v", err)
	}
	seedMember, err := domain.NewMember(household.ID, "Seed", now)
	if err != nil {
		t.Fatalf("NewMember (seed): %v", err)
	}
	if err := repository.CreateOnboarding(ctx, household, []domain.Member{seedMember}); err != nil {
		t.Fatalf("CreateOnboarding: %v", err)
	}

	const concurrentCreates = 8
	var wg sync.WaitGroup
	errs := make(chan error, concurrentCreates)
	for i := 0; i < concurrentCreates; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			member, err := domain.NewMember(household.ID, fmt.Sprintf("Member %d", index), now)
			if err != nil {
				errs <- err
				return
			}
			errs <- repository.CreateMember(ctx, member)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("CreateMember: %v", err)
		}
	}

	members, err := repository.ListMembers(ctx, true)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if want := concurrentCreates + 1; len(members) != want {
		t.Fatalf("member count = %d, want %d", len(members), want)
	}
	seen := make(map[int]bool, len(members))
	for _, member := range members {
		if seen[member.SortOrder] {
			t.Fatalf("duplicate sort_order %d among concurrently created members", member.SortOrder)
		}
		seen[member.SortOrder] = true
	}
}

func TestPortfolioRepositoriesEnforceModesUniquenessAndSnapshotBoundary(t *testing.T) {
	_, repository, household, account, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	quantity, err := domain.ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	holding, err := domain.NewHoldingForAccount(account, instrument, quantity, nil, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, holding); err != nil {
		t.Fatalf("create holding: %v", err)
	}
	if err := repository.CreateHolding(ctx, holding); err == nil {
		t.Fatal("duplicate active holding succeeded")
	}
	cash, err := domain.ParseMoney("5000", domain.CurrencyCode("SGD"))
	if err != nil {
		t.Fatal(err)
	}
	cashValue, err := domain.NewAccountCashValue(account, cash, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendAccountCashValue(ctx, cashValue); err != nil {
		t.Fatalf("append cash: %v", err)
	}
	price, err := domain.ParseUnitPrice("700")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: now.Add(-time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendInstrumentQuote(ctx, quote); err != nil {
		t.Fatalf("append instrument quote: %v", err)
	}
	rate, err := domain.ParseFxRate("5.3")
	if err != nil {
		t.Fatal(err)
	}
	fx, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: household.ID, BaseCurrency: domain.CurrencyCode("SGD"), QuoteCurrency: domain.CurrencyCode("CNY"), Rate: rate, SourceKind: domain.QuoteSourceManual, QuotedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendFXQuote(ctx, fx); err != nil {
		t.Fatalf("append FX quote: %v", err)
	}
	preference, err := domain.NewFXPreference(household.ID, domain.CurrencyCode("CNY"), domain.CurrencyCode("SGD"), domain.QuoteSourceManual, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetFXPreference(ctx, preference); err != nil {
		t.Fatalf("set FX preference: %v", err)
	}
	snapshot, err := repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("read portfolio snapshot: %v", err)
	}
	if snapshot.Household == nil || len(snapshot.Accounts) != 1 || len(snapshot.Instruments) != 1 || len(snapshot.Holdings) != 1 || len(snapshot.CashValues) != 1 || len(snapshot.InstrumentQuotes) != 1 || len(snapshot.FXQuotes) != 1 || len(snapshot.FXPreferences) != 1 {
		t.Fatalf("portfolio snapshot counts = accounts %d instruments %d holdings %d cash %d prices %d FX %d preferences %d", len(snapshot.Accounts), len(snapshot.Instruments), len(snapshot.Holdings), len(snapshot.CashValues), len(snapshot.InstrumentQuotes), len(snapshot.FXQuotes), len(snapshot.FXPreferences))
	}
	badQuote := quote
	badQuote.ID = domain.NewInstrumentQuoteID()
	badQuote.Currency = domain.CurrencyCode("EUR")
	if err := repository.AppendInstrumentQuote(ctx, badQuote); err == nil {
		t.Fatal("instrument quote with invalid currency succeeded")
	}
	if err := repository.SetHoldingArchive(ctx, household.ID, holding.ID, true, now); err != nil {
		t.Fatalf("archive holding: %v", err)
	}
	replacement, err := domain.NewHoldingForAccount(account, instrument, quantity, nil, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, replacement); err != nil {
		t.Fatalf("create replacement holding: %v", err)
	}
	if err := repository.SetHoldingArchive(ctx, household.ID, holding.ID, false, now); err == nil {
		t.Fatal("restoring a conflicting holding succeeded")
	}
}

func TestPortfolioSnapshotLoadsOnlyLatestCashAndQuoteCandidates(t *testing.T) {
	_, repository, household, account, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for index, effective := range []time.Time{now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), now.Add(-time.Hour)} {
		amount, err := domain.ParseMoney(fmt.Sprintf("%d", index+1), domain.CurrencyCode("CNY"))
		if err != nil {
			t.Fatal(err)
		}
		value, err := domain.NewAccountCashValue(account, amount, effective, effective.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.AppendAccountCashValue(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for index, quotedAt := range []time.Time{now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), now.Add(-time.Hour)} {
		price, err := domain.ParseUnitPrice(fmt.Sprintf("%d", 10+index))
		if err != nil {
			t.Fatal(err)
		}
		quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: quotedAt}, quotedAt.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.AppendInstrumentQuote(ctx, quote); err != nil {
			t.Fatal(err)
		}
	}
	providerPrice, _ := domain.ParseUnitPrice("20")
	providerQuote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: providerPrice, SourceKind: domain.QuoteSourceProvider, SourceKey: "fake", QuotedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendInstrumentQuote(ctx, providerQuote); err != nil {
		t.Fatal(err)
	}
	manualRate, _ := domain.ParseFxRate("6.8")
	providerRate, _ := domain.ParseFxRate("6.9")
	fxQuotes := []domain.FXQuote{
		newTestFXQuote(t, household.ID, "USD", "CNY", manualRate, domain.QuoteSourceManual, "manual", now.Add(-2*time.Hour)),
		newTestFXQuote(t, household.ID, "CNY", "USD", manualRate, domain.QuoteSourceManual, "manual", now.Add(-time.Hour)),
		newTestFXQuote(t, household.ID, "USD", "CNY", providerRate, domain.QuoteSourceProvider, "fake", now.Add(2*time.Minute)),
	}
	for _, quote := range fxQuotes {
		if err := repository.AppendFXQuote(ctx, quote); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.CashValues) != 1 || len(snapshot.InstrumentQuotes) != 2 || len(snapshot.FXQuotes) != 2 {
		t.Fatalf("bounded snapshot history counts = cash %d, instrument quotes %d, FX quotes %d", len(snapshot.CashValues), len(snapshot.InstrumentQuotes), len(snapshot.FXQuotes))
	}
	history, err := repository.ListFXQuotes(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("FX history API returned %d rows, want all 3", len(history))
	}
}

func newTestFXQuote(t *testing.T, householdID domain.HouseholdID, base, quote string, rate domain.FxRate, source domain.QuoteSourceKind, sourceKey string, quotedAt time.Time) domain.FXQuote {
	t.Helper()
	baseCurrency, err := domain.ParseCurrency(base)
	if err != nil {
		t.Fatal(err)
	}
	quoteCurrency, err := domain.ParseCurrency(quote)
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: householdID, BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: rate, SourceKind: source, SourceKey: sourceKey, QuotedAt: quotedAt}, quotedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPortfolioRepositoriesRejectHoldingsOnNonHoldingsAccount(t *testing.T) {
	database, repository, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	memberID := domain.NewMemberID()
	member, err := domain.NewMember(household.ID, "Second", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	member.ID = memberID
	if err := repository.CreateMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	accountInput := domain.AccountInput{HouseholdID: household.ID, Name: "Balance", AccountType: domain.TypeCashOnHand, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: domain.CurrencyCode("CNY"), Ownership: []domain.OwnershipShare{{MemberID: member.ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "1"}
	balance, ownership, initial, err := domain.NewAccount(accountInput, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewAccountValue(balance, *initial, time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateAccount(ctx, balance, ownership, &value); err != nil {
		t.Fatal(err)
	}
	instrument, err := domain.NewInstrument(domain.InstrumentInput{HouseholdID: household.ID, Name: "Cash proxy", Type: domain.InstrumentOther, QuoteCurrency: domain.CurrencyCode("CNY")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateInstrument(ctx, instrument); err != nil {
		t.Fatal(err)
	}
	quantity, _ := domain.ParseQuantity("0")
	holding, err := domain.NewHolding(domain.HoldingInput{AccountID: balance.ID, InstrumentID: instrument.ID, Quantity: quantity}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, holding); err == nil {
		t.Fatal("holding on a Balance account succeeded")
	}
	database.Close()
}

func TestArchivedHoldingAndAccountRejectFinalWrites(t *testing.T) {
	_, repository, household, account, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	holding, err := domain.NewHoldingForAccount(account, instrument, quantity, nil, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, holding); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetHoldingArchive(ctx, household.ID, holding.ID, true, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateHolding(ctx, holding.ReplaceQuantity(quantity, now)); !hasDomainErrorCode(err, domain.ErrConflict) {
		t.Fatalf("archived holding update error = %v, want conflict", err)
	}
	if err := repository.SetAccountArchive(ctx, household.ID, account.ID, true, now); err != nil {
		t.Fatal(err)
	}
	money, err := domain.ParseMoney("1", domain.CurrencyCode("CNY"))
	if err != nil {
		t.Fatal(err)
	}
	cash, err := domain.NewAccountCashValue(account, money, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendAccountCashValue(ctx, cash); !hasDomainErrorCode(err, domain.ErrConflict) {
		t.Fatalf("archived account cash error = %v, want conflict", err)
	}
}

func hasDomainErrorCode(err error, want domain.ErrorCode) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr.Code == want
}

func seedPortfolioRepository(t *testing.T) (*DB, *Repository, domain.Household, domain.Account, domain.Instrument) {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "portfolio.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	repository := NewRepository(database)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	household, err := domain.NewHousehold("Portfolio", domain.CurrencyCode("CNY"), now)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	member, err := domain.NewMember(household.ID, "Owner", now)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := repository.CreateOnboarding(context.Background(), household, []domain.Member{member}); err != nil {
		database.Close()
		t.Fatalf("onboarding: %v", err)
	}
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{HouseholdID: household.ID, Name: "Holdings", AccountType: domain.TypeBrokerage, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingHoldings, DefaultCurrency: domain.CurrencyCode("CNY"), Ownership: []domain.OwnershipShare{{MemberID: member.ID, ShareBPS: domain.TotalOwnershipBPS}}}, now)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := repository.CreateAccount(context.Background(), account, ownership, nil); err != nil {
		database.Close()
		t.Fatalf("holdings account: %v", err)
	}
	instrument, err := domain.NewInstrument(domain.InstrumentInput{HouseholdID: household.ID, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: domain.CurrencyCode("USD")}, now)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := repository.CreateInstrument(context.Background(), instrument); err != nil {
		database.Close()
		t.Fatalf("instrument: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database, repository, household, account, instrument
}

func TestCreateAccountPersistsMultiCurrencyCashOnHand(t *testing.T) {
	_, repository, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	members, err := repository.ListMembers(ctx, true)
	if err != nil || len(members) == 0 {
		t.Fatalf("ListMembers: %v", err)
	}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	account, ownership, initial, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Travel cash", AccountType: domain.TypeCashOnHand, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingHoldings, DefaultCurrency: domain.CurrencyCode("CNY"),
		IncludeInNetWorth: true, IncludeInLiquidAssets: true,
		Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if initial != nil {
		t.Fatal("holdings cash on hand must not carry an initial Account Value")
	}
	if err := repository.CreateAccount(ctx, account, ownership, nil); err != nil {
		t.Fatalf("CreateAccount cash_on_hand holdings: %v", err)
	}
	loaded, err := repository.AccountRecord(ctx, household.ID, account.ID)
	if err != nil {
		t.Fatalf("AccountRecord: %v", err)
	}
	if loaded.Account.AccountType != domain.TypeCashOnHand || loaded.Account.TrackingMode != domain.TrackingHoldings {
		t.Fatalf("loaded account = %+v", loaded.Account)
	}
	if loaded.LatestValue != nil {
		t.Fatalf("LatestValue = %+v, want nil", loaded.LatestValue)
	}
}

func TestCreateAccountAfterLegacyCheckRepairPersistsMultiCurrencyCashOnHand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-check.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := rewriteAccountsCheckFragment(ctx, first.SQL, cashOnHandBalanceOrHoldingsCheck, cashOnHandBalanceOnlyCheck); err != nil {
		t.Fatalf("install legacy check: %v", err)
	}
	repository := NewRepository(first)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	household, err := domain.NewHousehold("Repair", domain.CurrencyCode("CNY"), now)
	if err != nil {
		t.Fatal(err)
	}
	member, err := domain.NewMember(household.ID, "Owner", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateOnboarding(ctx, household, []domain.Member{member}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open repaired database: %v", err)
	}
	defer reopened.Close()
	repository = NewRepository(reopened)
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Travel cash", AccountType: domain.TypeCashOnHand, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingHoldings, DefaultCurrency: domain.CurrencyCode("CNY"),
		IncludeInNetWorth: true, IncludeInLiquidAssets: true,
		Ownership: []domain.OwnershipShare{{MemberID: member.ID, ShareBPS: domain.TotalOwnershipBPS}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateAccount(ctx, account, ownership, nil); err != nil {
		t.Fatalf("CreateAccount after repair: %v", err)
	}
	loaded, err := repository.AccountRecord(ctx, household.ID, account.ID)
	if err != nil {
		t.Fatalf("AccountRecord: %v", err)
	}
	if loaded.Account.TrackingMode != domain.TrackingHoldings {
		t.Fatalf("loaded tracking = %s, want holdings", loaded.Account.TrackingMode)
	}
}

func TestCreateAccountMapsIllegalCombinationToDomainError(t *testing.T) {
	_, repository, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	members, err := repository.ListMembers(ctx, true)
	if err != nil || len(members) == 0 {
		t.Fatalf("ListMembers: %v", err)
	}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	account, ownership, initial, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Invalid", AccountType: domain.TypeCashOnHand, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingBalance, DefaultCurrency: domain.CurrencyCode("CNY"),
		Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "1",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	account.BalanceSheetRole = domain.RoleLiability
	value, err := domain.NewAccountValue(account, *initial, now, now)
	if err != nil {
		t.Fatal(err)
	}
	err = repository.CreateAccount(ctx, account, ownership, &value)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrValidation {
		t.Fatalf("CreateAccount error = %v, want validation", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "check") || strings.Contains(strings.ToLower(err.Error()), "sqlite") {
		t.Fatalf("domain error leaked SQL detail: %v", err)
	}
}

func TestListActivitiesScanErrorDoesNotPinConnection(t *testing.T) {
	database, repository, household, _, _ := seedPortfolioRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := database.SQL.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at) VALUES(?, ?, 'cash_in', 'contribution', ?, ?, ?)`,
		"bad", household.ID.String(), "2026-08-23T12:00:00.000Z", "2026-08-23", "2026-08-23T12:00:00.000Z")
	if err != nil {
		t.Fatalf("insert malformed activity: %v", err)
	}
	_, err = repository.ListActivities(ctx, household.ID, 10)
	if err == nil {
		t.Fatal("ListActivities accepted a malformed activity id")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("scan error pinned the one-connection pool: %v", err)
	}
	if _, err := repository.Household(ctx); err != nil {
		t.Fatalf("Household after scan error: %v", err)
	}
	members, err := repository.ListMembers(ctx, true)
	if err != nil {
		t.Fatalf("ListMembers after scan error: %v", err)
	}
	if len(members) == 0 {
		t.Fatal("expected members after scan error")
	}
	page, err := repository.ListActivityPage(ctx, household.ID, domain.ActivityQuery{Limit: 10})
	if err == nil {
		t.Fatalf("ListActivityPage accepted a malformed activity id: %+v", page)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("paged scan error pinned the one-connection pool: %v", err)
	}
	if _, err := repository.Household(ctx); err != nil {
		t.Fatalf("Household after paged scan error: %v", err)
	}
}
