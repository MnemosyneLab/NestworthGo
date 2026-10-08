package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestArchiveMemberPreservesActiveOwnerForLiveAccounts(t *testing.T) {
	t.Parallel()
	service, ctx, bootstrap, _ := newOnboardedService(t, "member-archive-owners", []string{"Alice", "Bob", "Carol"})
	alice, bob, carol := bootstrap.Members[0].ID, bootstrap.Members[1].ID, bootstrap.Members[2].ID
	sole, err := service.CreateAccount(ctx, AccountInput{Name: "Sole", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "0", OwnerIDs: []domain.MemberID{alice}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveMember(ctx, alice, true); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("archive sole owner = %v, want conflict", err)
	}
	members, err := service.ListMembers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !containsActiveMember(members, alice) {
		t.Fatal("rejected archive changed the member")
	}
	if err := service.ArchiveMember(ctx, carol, true); err != nil {
		t.Fatalf("archive non-owner: %v", err)
	}
	if _, err := service.UpdateAccount(ctx, sole.Account.ID, AccountInput{OwnerIDs: []domain.MemberID{alice, bob}}); err != nil {
		t.Fatalf("share account ownership: %v", err)
	}
	if err := service.ArchiveMember(ctx, alice, true); err != nil {
		t.Fatalf("archive member with another active owner: %v", err)
	}
	members, err = service.ListMembers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if containsActiveMember(members, alice) || !containsActiveMember(members, bob) {
		t.Fatalf("unexpected active members after archive: %+v", members)
	}
	if err := service.ArchiveMember(ctx, alice, false); err != nil {
		t.Fatalf("restore archived member: %v", err)
	}
}

func TestArchiveMemberIgnoresArchivedAccounts(t *testing.T) {
	t.Parallel()
	service, ctx, bootstrap, _ := newOnboardedService(t, "member-archive-old-account", []string{"Alice", "Bob"})
	alice := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Old", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "0", OwnerIDs: []domain.MemberID{alice}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveAccount(ctx, account.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveMember(ctx, alice, true); err != nil {
		t.Fatalf("archive owner of archived account: %v", err)
	}
}

func containsActiveMember(members []domain.Member, id domain.MemberID) bool {
	for _, member := range members {
		if member.ID == id {
			return true
		}
	}
	return false
}
