package application

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/media"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountFiltersLatestValueAndMediaAttachment(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	service := NewServiceWithImageNormalizer(sqlite.NewRepository(database), media.Normalizer{})
	clock := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	institution, err := service.CreateInstitution(ctx, "Bank")
	if err != nil {
		t.Fatalf("institution: %v", err)
	}
	group, err := service.CreateGroup(ctx, "Emergency")
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	withRefs, err := service.CreateAccount(ctx, AccountInput{Name: "Reserve", PrimaryCategory: "cash_equivalent", SecondaryCategory: "bank_account", TrackingMode: "balance", DefaultCurrency: "CNY", InstitutionID: institution.ID.String(), GroupID: group.ID.String(), IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "10"})
	if err != nil {
		t.Fatalf("account with references: %v", err)
	}
	if institution.IconKey == nil || *institution.IconKey != domain.DefaultInstitutionIcon {
		t.Fatalf("institution icon = %#v, want %q", institution.IconKey, domain.DefaultInstitutionIcon)
	}
	if group.IconKey == nil || *group.IconKey != domain.DefaultGroupIcon {
		t.Fatalf("group icon = %#v, want %q", group.IconKey, domain.DefaultGroupIcon)
	}
	if withRefs.Account.IconKey == nil || *withRefs.Account.IconKey != domain.DefaultAccountIcon {
		t.Fatalf("account icon = %#v, want %q", withRefs.Account.IconKey, domain.DefaultAccountIcon)
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
	if err != nil || len(institutions) != 1 || institutions[0].IconKey == nil || *institutions[0].IconKey != "home" {
		t.Fatalf("reloaded institution icon = %#v, err = %v", institutions, err)
	}
	if !institutions[0].UpdatedAt.Equal(clock) {
		t.Fatalf("institution UpdatedAt = %s, want injected clock %s", institutions[0].UpdatedAt, clock)
	}
	groups, err := service.ListGroups(ctx, true)
	if err != nil || len(groups) != 1 || groups[0].IconKey == nil || *groups[0].IconKey != "folder" {
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
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Other", PrimaryCategory: "cash_equivalent", SecondaryCategory: "cash", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "5"}); err != nil {
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
	var imageBuffer bytes.Buffer
	if err := png.Encode(&imageBuffer, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode image fixture: %v", err)
	}
	asset, err := service.CreateMediaAsset(ctx, "image/png", imageBuffer.Bytes())
	if err != nil {
		t.Fatalf("media asset: %v", err)
	}
	if err := service.SetAccountLogo(ctx, withRefs.Account.ID, asset.ID); err != nil {
		t.Fatalf("account logo: %v", err)
	}
	if _, err := media.Normalize([]byte("not an image")); err == nil {
		t.Fatal("invalid image normalized")
	}
}
