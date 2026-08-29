package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountFiltersLatestValueAndIcons(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	institution, err := service.CreateInstitution(ctx, "Bank", domain.InstitutionBank)
	if err != nil {
		t.Fatalf("institution: %v", err)
	}
	group, err := service.CreateGroup(ctx, "Emergency")
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	withRefs, err := service.CreateAccount(ctx, AccountInput{Name: "Reserve", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InstitutionID: institution.ID.String(), GroupID: group.ID.String(), IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "10"})
	if err != nil {
		t.Fatalf("account with references: %v", err)
	}
	if institution.IconKey == nil || *institution.IconKey != domain.DefaultInstitutionIcon {
		t.Fatalf("institution icon = %#v, want %q", institution.IconKey, domain.DefaultInstitutionIcon)
	}
	if group.IconKey == nil || *group.IconKey != domain.DefaultGroupIcon {
		t.Fatalf("group icon = %#v, want %q", group.IconKey, domain.DefaultGroupIcon)
	}
	if withRefs.Account.IconKey == nil || *withRefs.Account.IconKey != domain.DefaultAccountIcon(domain.TypeBankAccount) {
		t.Fatalf("account icon = %#v, want %q", withRefs.Account.IconKey, domain.DefaultAccountIcon(domain.TypeBankAccount))
	}
	if err := service.SetInstitutionIcon(ctx, institution.ID, "home"); err != nil {
		t.Fatalf("institution icon: %v", err)
	}
	if err := service.SetGroupIcon(ctx, group.ID, "folder"); err != nil {
		t.Fatalf("group icon: %v", err)
	}
	if err := service.SetAccountIcon(ctx, withRefs.Account.ID, "wallet"); err != nil {
		t.Fatalf("account icon: %v", err)
	}
	institutions, err := service.ListInstitutions(ctx, true)
	if err != nil {
		t.Fatalf("reloaded institutions: %v", err)
	}
	var reloadedInstitution *domain.Institution
	for index := range institutions {
		if institutions[index].ID == institution.ID {
			copied := institutions[index]
			reloadedInstitution = &copied
			break
		}
	}
	if reloadedInstitution == nil || reloadedInstitution.IconKey == nil || *reloadedInstitution.IconKey != "home" {
		t.Fatalf("reloaded institution icon = %#v, err = %v", institutions, err)
	}
	if !reloadedInstitution.UpdatedAt.Equal(clock) {
		t.Fatalf("institution UpdatedAt = %s, want injected clock %s", reloadedInstitution.UpdatedAt, clock)
	}
	groups, err := service.ListGroups(ctx, true)
	if err != nil {
		t.Fatalf("reloaded groups: %v", err)
	}
	var reloadedGroup *domain.Group
	for index := range groups {
		if groups[index].ID == group.ID {
			copied := groups[index]
			reloadedGroup = &copied
			break
		}
	}
	if reloadedGroup == nil || reloadedGroup.IconKey == nil || *reloadedGroup.IconKey != "folder" {
		t.Fatalf("reloaded group icon = %#v, err = %v", groups, err)
	}
	accountsWithIcons, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("reload account icons: %v", err)
	}
	for _, account := range accountsWithIcons {
		if account.Account.ID == withRefs.Account.ID && (account.Account.IconKey == nil || *account.Account.IconKey != "wallet") {
			t.Fatalf("reloaded account icon = %#v", account.Account.IconKey)
		}
		if account.Account.ID == withRefs.Account.ID && !account.Account.UpdatedAt.Equal(clock) {
			t.Fatalf("account UpdatedAt = %s, want injected clock %s", account.Account.UpdatedAt, clock)
		}
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Other", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "5"}); err != nil {
		t.Fatalf("other account: %v", err)
	}
	institutionID, _ := domain.ParseInstitutionID(institution.ID.String())
	groupID, _ := domain.ParseGroupID(group.ID.String())
	filtered, err := service.ListAccounts(ctx, domain.AccountFilter{InstitutionID: &institutionID, GroupID: &groupID})
	if err != nil {
		t.Fatalf("filtered accounts: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Account.ID != withRefs.Account.ID {
		t.Fatalf("filtered accounts = %#v", filtered)
	}
	if _, err := service.AppendAccountValue(ctx, withRefs.Account.ID, "12", "2026-08-22"); err != nil {
		t.Fatalf("append value: %v", err)
	}
	latest, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("reload accounts: %v", err)
	}
	found := false
	for _, record := range latest {
		if record.Account.ID != withRefs.Account.ID {
			continue
		}
		found = true
		if record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "12" {
			t.Fatalf("latest value = %#v", record.LatestValue)
		}
	}
	if !found {
		t.Fatal("updated account was omitted from reloaded list")
	}
}
