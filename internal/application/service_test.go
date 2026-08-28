package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHouseholdAccountAndOverviewFlow(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "nestworth", []string{"Alice", "Bob"})
	setClock(time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC))
	if bootstrap.Household == nil || len(bootstrap.Members) != 2 {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
	ownership, err := domain.ParseOwnership([]domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: 6000}, {MemberID: bootstrap.Members[1].ID, ShareBPS: 4000}})
	if err != nil {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: ownership.Shares(), InitialAmount: "1000"}); err != nil {
		t.Fatalf("create asset account: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "250"}); err != nil {
		t.Fatalf("create liability account: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Excluded", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: false, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "9000"}); err != nil {
		t.Fatalf("create excluded account: %v", err)
	}
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("list accounts for update: %v", err)
	}
	if len(accounts) < 1 {
		t.Fatal("expected account for update")
	}
	if _, err := service.UpdateAccount(ctx, accounts[0].Account.ID, AccountInput{AccountType: "investment_account", BalanceSheetRole: "asset", TrackingMode: "manual_value"}); err == nil {
		t.Fatal("tracking mode change was accepted")
	}
	result, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !result.Assets.Equal(decimal.RequireFromString("1000")) || !result.Liabilities.Equal(decimal.RequireFromString("250")) || !result.NetWorth.Equal(decimal.RequireFromString("750")) {
		t.Fatalf("overview totals = assets %s liabilities %s net worth %s", result.Assets, result.Liabilities, result.NetWorth)
	}
	if len(result.ByMember) != 2 {
		t.Fatalf("member breakdown length = %d", len(result.ByMember))
	}
}

type directoryCountingRepository struct {
	Repository
	members      int
	institutions int
	groups       int
}

func (r *directoryCountingRepository) ListMembers(ctx context.Context, includeArchived bool) ([]domain.Member, error) {
	r.members++
	return r.Repository.ListMembers(ctx, includeArchived)
}

func (r *directoryCountingRepository) ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error) {
	r.institutions++
	return r.Repository.ListInstitutions(ctx, includeArchived)
}

func (r *directoryCountingRepository) ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error) {
	r.groups++
	return r.Repository.ListGroups(ctx, includeArchived)
}

func TestIdentityOnlyMutationsDoNotLoadBootstrapDirectories(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	base := sqlite.NewRepository(database)
	ctx := context.Background()
	service := NewService(base)
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}

	counting := &directoryCountingRepository{Repository: base}
	service = NewService(counting)
	if _, err := service.CreateMember(ctx, "Bob"); err != nil {
		t.Fatalf("CreateMember: %v", err)
	}
	if _, err := service.CreateInstitution(ctx, "Bank"); err != nil {
		t.Fatalf("CreateInstitution: %v", err)
	}
	if _, err := service.CreateGroup(ctx, "Emergency"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"}); err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}

	if counting.members != 0 || counting.institutions != 0 || counting.groups != 0 {
		t.Fatalf("identity-only mutations loaded Bootstrap directories: members=%d institutions=%d groups=%d", counting.members, counting.institutions, counting.groups)
	}
}

func TestUpdateAccountAllowsCompatibleTypeChangeWithoutActivity(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "type-edit", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "2500", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	before, err := service.ListActivities(ctx, 20)
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	updated, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{AccountType: "investment_account"})
	if err != nil {
		t.Fatalf("compatible type edit: %v", err)
	}
	if updated.Account.AccountType != domain.TypeInvestmentAccount {
		t.Fatalf("account type = %s, want investment_account", updated.Account.AccountType)
	}
	if updated.Account.BalanceSheetRole != domain.RoleAsset || updated.Account.TrackingMode != domain.TrackingHoldings {
		t.Fatalf("frozen fields changed: %+v", updated.Account)
	}
	after, err := service.ListActivities(ctx, 20)
	if err != nil {
		t.Fatalf("list activities after edit: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("type edit created activities: before %d after %d", len(before), len(after))
	}
	cash, err := service.ListAccountCashValues(ctx, account.Account.ID)
	if err != nil || len(cash) != 1 || cash[0].Amount.CanonicalAmount() != "2500" {
		t.Fatalf("cash after type edit = %+v err=%v", cash, err)
	}
}

func TestUpdateAccountRejectsIncompatibleTypeAndImmutableRole(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "type-reject", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{AccountType: "cash_on_hand"}); err == nil {
		t.Fatal("incompatible type edit was accepted")
	}
	if _, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{BalanceSheetRole: "liability"}); err == nil {
		t.Fatal("role edit was accepted")
	}
	if _, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{TrackingMode: "manual_value"}); err == nil {
		t.Fatal("tracking edit was accepted")
	}
}

func TestOverviewClassifiesCompositeAndSimpleBuckets(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "overview-buckets", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "招行", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "800",
	}); err != nil {
		t.Fatalf("bank: %v", err)
	}
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("brokerage: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerage.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "2"}); err != nil {
		t.Fatalf("holding: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "200", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "50", "2026-08-01", false); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "150",
	}); err != nil {
		t.Fatalf("card: %v", err)
	}

	result, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !result.Complete {
		t.Fatalf("overview incomplete: %+v", result.MissingInputs)
	}
	if !result.Assets.Equal(decimal.RequireFromString("1100")) || !result.Liabilities.Equal(decimal.RequireFromString("150")) {
		t.Fatalf("totals assets=%s liabilities=%s", result.Assets, result.Liabilities)
	}
	buckets := map[string]string{}
	for _, item := range result.AssetsByType {
		buckets[item.Key] = item.Amount.String()
	}
	if buckets[domain.BucketCash] != "1000" || buckets[domain.BucketStock] != "100" {
		t.Fatalf("assetsByType = %+v, want cash=1000 stock=100", result.AssetsByType)
	}
	if len(result.LiabilitiesByType) != 1 || result.LiabilitiesByType[0].Key != domain.BucketCreditCard || result.LiabilitiesByType[0].Amount.String() != "150" {
		t.Fatalf("liabilitiesByType = %+v", result.LiabilitiesByType)
	}
	byType := map[string]string{}
	for _, item := range result.ByAccountType {
		byType[item.Key] = item.Amount.String()
	}
	if byType["bank_account"] != "800" || byType["brokerage"] != "300" {
		t.Fatalf("byAccountType = %+v, want bank_account=800 brokerage=300", result.ByAccountType)
	}
	if _, ok := byType["credit_card"]; ok {
		t.Fatalf("byAccountType included a liability: %+v", result.ByAccountType)
	}
}

func TestOverviewByAccountTypeKeepsMixedBankAccountWhole(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "overview-by-account-type", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	bank, err := service.CreateAccount(ctx, AccountInput{
		Name: "招商银行综合账户", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("bank: %v", err)
	}
	fund, err := service.CreateInstrument(ctx, InstrumentInput{Name: "招银理财A", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("fund: %v", err)
	}
	gold, err := service.CreateInstrument(ctx, InstrumentInput{Name: "黄金", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("gold: %v", err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: bank.Account.ID.String(), InstrumentID: fund.ID.String(), Quantity: "1"}); err != nil {
		t.Fatalf("fund holding: %v", err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: bank.Account.ID.String(), InstrumentID: gold.ID.String(), Quantity: "2"}); err != nil {
		t.Fatalf("gold holding: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, bank.Account.ID, "50000", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, fund.ID, "200000", "2026-08-01", false); err != nil {
		t.Fatalf("fund quote: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, gold.ID, "400", "2026-08-01", false); err != nil {
		t.Fatalf("gold quote: %v", err)
	}

	result, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !result.Complete {
		t.Fatalf("overview incomplete: %+v", result.MissingInputs)
	}
	if !result.Assets.Equal(decimal.RequireFromString("250800")) {
		t.Fatalf("assets = %s, want 250800", result.Assets)
	}
	buckets := map[string]string{}
	for _, item := range result.AssetsByType {
		buckets[item.Key] = item.Amount.String()
	}
	if buckets[domain.BucketCash] != "50000" || buckets[domain.BucketMutualFund] != "200000" || buckets[domain.BucketPreciousMetal] != "800" {
		t.Fatalf("assetsByType = %+v", result.AssetsByType)
	}
	if len(result.ByAccountType) != 1 || result.ByAccountType[0].Key != "bank_account" || result.ByAccountType[0].Amount.String() != "250800" {
		t.Fatalf("byAccountType = %+v, want one bank_account row of 250800", result.ByAccountType)
	}
}

func TestCreateAccountRejectsEmptyOwnership(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "empty-owners", []string{"Alice", "Bob"})
	_, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, InitialAmount: "1000",
	})
	if err == nil {
		t.Fatal("CreateAccount with no owners succeeded")
	}
	domainErr, ok := err.(*domain.Error)
	if !ok || domainErr.Code != domain.ErrValidation || domainErr.Field != "ownership" {
		t.Fatalf("error = %v, want ownership validation", err)
	}
}

func TestUpdateAccountRejectsExplicitEmptyOwnership(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "empty-owner-update", []string{"Alice"})
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, InitialAmount: "1000",
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = service.UpdateAccount(ctx, account.Account.ID, AccountInput{Ownership: []domain.OwnershipShare{}})
	if err == nil {
		t.Fatal("UpdateAccount with empty ownership succeeded")
	}
	domainErr, ok := err.(*domain.Error)
	if !ok || domainErr.Code != domain.ErrValidation || domainErr.Field != "ownership" {
		t.Fatalf("error = %v, want ownership validation", err)
	}
}

func TestUpdateAccountOmittingOwnershipPreservesCustomSplit(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "preserve-split", []string{"Alice", "Bob"})
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Joint", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, InitialAmount: "1000",
		OwnerIDs:             []domain.MemberID{bootstrap.Members[0].ID, bootstrap.Members[1].ID},
		OwnershipPercentages: []string{"70", "30"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{Name: "Joint renamed"})
	if err != nil {
		t.Fatalf("name-only update: %v", err)
	}
	if updated.Account.Name != "Joint renamed" {
		t.Fatalf("name = %q, want Joint renamed", updated.Account.Name)
	}
	byMember := map[domain.MemberID]int{}
	for _, share := range updated.Ownership.Shares() {
		byMember[share.MemberID] = share.ShareBPS
	}
	if byMember[bootstrap.Members[0].ID] != 7000 || byMember[bootstrap.Members[1].ID] != 3000 {
		t.Fatalf("Ownership = %+v, want preserved 70/30", updated.Ownership.Shares())
	}
}
