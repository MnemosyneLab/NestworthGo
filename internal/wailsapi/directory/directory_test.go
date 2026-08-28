package directory_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/directory"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func newOnboardedServices(t *testing.T) *directory.Service {
	t.Helper()
	app := wailstest.NewService(t)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	return directory.NewService(app)
}

func TestMemberLifecycle(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()

	member, err := service.CreateMember(ctx, "Bob", "")
	if err != nil {
		t.Fatalf("CreateMember: %v", err)
	}
	if member.Name != "Bob" {
		t.Fatalf("Name = %q, want Bob", member.Name)
	}
	if member.IconKey != "user" {
		t.Fatalf("IconKey = %q, want user", member.IconKey)
	}

	updated, err := service.UpdateMember(ctx, member.ID, "Bobby")
	if err != nil {
		t.Fatalf("UpdateMember: %v", err)
	}
	if updated.Name != "Bobby" {
		t.Fatalf("Name = %q, want Bobby", updated.Name)
	}

	if err := service.ArchiveMember(ctx, member.ID, true); err != nil {
		t.Fatalf("ArchiveMember: %v", err)
	}
	active, err := service.ListMembers(ctx, false)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range active {
		if m.ID == member.ID {
			t.Fatalf("archived member %q still listed among active members", member.ID)
		}
	}
	all, err := service.ListMembers(ctx, true)
	if err != nil {
		t.Fatalf("ListMembers(includeArchived): %v", err)
	}
	found := false
	for _, m := range all {
		if m.ID == member.ID {
			found = true
			if m.ArchivedAt == nil {
				t.Fatalf("expected ArchivedAt to be set for archived member %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("archived member %q missing from includeArchived=true list", member.ID)
	}
}

func TestUpdateMemberNotFound(t *testing.T) {
	service := newOnboardedServices(t)
	_, err := service.UpdateMember(context.Background(), "00000000-0000-7000-8000-000000000000", "Ghost")
	assertWireCode(t, err, "not_found")
}

func TestUpdateMemberInvalidID(t *testing.T) {
	service := newOnboardedServices(t)
	_, err := service.UpdateMember(context.Background(), "not-a-uuid", "Ghost")
	assertWireCode(t, err, "validation")
}

func TestInstitutionLifecycleWithIcon(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()

	institution, err := service.CreateInstitution(ctx, "DBS", "bank", "bank")
	if err != nil {
		t.Fatalf("CreateInstitution: %v", err)
	}
	if institution.IconKey != "bank" {
		t.Fatalf("IconKey = %v, want bank", institution.IconKey)
	}

	if err := service.SetInstitutionIcon(ctx, institution.ID, "bank-branch"); err != nil {
		t.Fatalf("SetInstitutionIcon: %v", err)
	}
	list, err := service.ListInstitutions(ctx, false)
	if err != nil {
		t.Fatalf("ListInstitutions: %v", err)
	}
	if len(list) != 1 || list[0].IconKey != "bank-branch" {
		t.Fatalf("ListInstitutions = %+v, want icon bank-branch", list)
	}

	if err := service.ArchiveInstitution(ctx, institution.ID, true); err != nil {
		t.Fatalf("ArchiveInstitution: %v", err)
	}
	afterArchive, err := service.ListInstitutions(ctx, false)
	if err != nil {
		t.Fatalf("ListInstitutions after archive: %v", err)
	}
	if len(afterArchive) != 0 {
		t.Fatalf("expected no active institutions after archive, got %+v", afterArchive)
	}
}

func TestGroupLifecycle(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()

	group, err := service.CreateGroup(ctx, "Family", "")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	updated, err := service.UpdateGroup(ctx, group.ID, "Household")
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if updated.Name != "Household" {
		t.Fatalf("Name = %q, want Household", updated.Name)
	}
	if err := service.SetGroupIcon(ctx, group.ID, "folder"); err != nil {
		t.Fatalf("SetGroupIcon: %v", err)
	}
	if err := service.ArchiveGroup(ctx, group.ID, true); err != nil {
		t.Fatalf("ArchiveGroup: %v", err)
	}
	if err := service.ArchiveGroup(ctx, group.ID, false); err != nil {
		t.Fatalf("restore ArchiveGroup: %v", err)
	}
}

func TestCreateInstitutionRequiresTypeAndDefaultsIcon(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()

	if _, err := service.CreateInstitution(ctx, "Ghost", "", ""); err == nil {
		t.Fatal("want an error for a missing institution type")
	}

	institution, err := service.CreateInstitution(ctx, "AIA", "insurer", "")
	if err != nil {
		t.Fatalf("CreateInstitution: %v", err)
	}
	if institution.InstitutionType != "insurer" {
		t.Fatalf("InstitutionType = %q, want insurer", institution.InstitutionType)
	}
	if institution.IconKey != "shield-plus" {
		t.Fatalf("IconKey = %q, want shield-plus", institution.IconKey)
	}
}

func TestSetMemberIconRejectsUnknownIcon(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()
	member, err := service.CreateMember(ctx, "Bob", "")
	if err != nil {
		t.Fatalf("CreateMember: %v", err)
	}
	err = service.SetMemberIcon(ctx, member.ID, "not-an-icon")
	if err == nil {
		t.Fatal("want an error for an unknown icon")
	}
	if err := service.SetMemberIcon(ctx, member.ID, ""); err == nil {
		t.Fatal("want an error for an empty icon")
	}
}

func TestDirectoryDTOsRoundTripAsJSON(t *testing.T) {
	service := newOnboardedServices(t)
	ctx := context.Background()
	institution, err := service.CreateInstitution(ctx, "DBS", "bank", "bank")
	if err != nil {
		t.Fatalf("CreateInstitution: %v", err)
	}
	group, err := service.CreateGroup(ctx, "Family", "folder")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	for name, value := range map[string]any{"institution": institution, "group": group} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: json.Marshal: %v", name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("%s: json.Unmarshal: %v", name, err)
		}
		if len(decoded) == 0 {
			t.Fatalf("%s round-trip produced an empty object", name)
		}
	}
}

func assertWireCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error with code %q, got nil", wantCode)
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok {
		t.Fatalf("error is not a parseable WireError: %v", err)
	}
	if wireErr.Code != wantCode {
		t.Fatalf("Code = %q, want %q", wireErr.Code, wantCode)
	}
}
