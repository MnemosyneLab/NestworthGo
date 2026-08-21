package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Regression coverage for BUG-1/BUG-1b (docs/development/code-review-2026-08-21.md):
// the ownership editor must key everything by MemberID, never by name, so
// duplicate member names and archived owners both work correctly.

func newTestMember(t *testing.T, householdID domain.HouseholdID, name string, archived bool) domain.Member {
	t.Helper()
	member, err := domain.NewMember(householdID, name, time.Now())
	if err != nil {
		t.Fatalf("NewMember(%q) error = %v", name, err)
	}
	if archived {
		archivedAt := time.Now()
		member.ArchivedAt = &archivedAt
	}
	return member
}

func TestCollectOwnershipDisambiguatesDuplicateMemberNames(t *testing.T) {
	householdID := domain.NewHouseholdID()
	alice1 := newTestMember(t, householdID, "Alice", false)
	alice2 := newTestMember(t, householdID, "Alice", false)

	fields := []*ownershipField{
		{memberID: alice1.ID, check: newCheckState(false), percent: newEntryState("")},
		{memberID: alice2.ID, check: newCheckState(true), percent: newEntryState("100")},
	}

	ids, percentages := collectOwnership(fields)
	if len(ids) != 1 || ids[0] != alice2.ID {
		t.Fatalf("collectOwnership ids = %#v, want only the checked namesake %s", ids, alice2.ID)
	}
	if len(percentages) != 1 || percentages[0] != "100" {
		t.Fatalf("collectOwnership percentages = %#v, want [\"100\"]", percentages)
	}
}

func TestCollectOwnershipEqualSplitWhenAllPercentagesBlank(t *testing.T) {
	householdID := domain.NewHouseholdID()
	alice := newTestMember(t, householdID, "Alice", false)
	bob := newTestMember(t, householdID, "Bob", false)

	fields := []*ownershipField{
		{memberID: alice.ID, check: newCheckState(true), percent: newEntryState("")},
		{memberID: bob.ID, check: newCheckState(true), percent: newEntryState("")},
	}

	ids, percentages := collectOwnership(fields)
	if len(ids) != 2 {
		t.Fatalf("collectOwnership ids = %#v, want both checked owners", ids)
	}
	if percentages != nil {
		t.Fatalf("collectOwnership percentages = %#v, want nil so the service applies an equal split", percentages)
	}
}

func TestOwnershipEditorCandidatesKeepsArchivedCurrentOwner(t *testing.T) {
	householdID := domain.NewHouseholdID()
	alice := newTestMember(t, householdID, "Alice", false)
	archivedBob := newTestMember(t, householdID, "Bob", true)
	archivedCarol := newTestMember(t, householdID, "Carol", true)

	ownership, err := domain.ParseOwnership([]domain.OwnershipShare{
		{MemberID: alice.ID, ShareBPS: 6000},
		{MemberID: archivedBob.ID, ShareBPS: 4000},
	})
	if err != nil {
		t.Fatalf("ParseOwnership error = %v", err)
	}

	candidates, selected, archivedFlags := ownershipEditorCandidates([]domain.Member{alice, archivedBob, archivedCarol}, ownership)

	byID := map[domain.MemberID]bool{}
	for _, member := range candidates {
		byID[member.ID] = true
	}
	if !byID[alice.ID] {
		t.Fatal("active member missing from ownership candidates")
	}
	if !byID[archivedBob.ID] {
		t.Fatal("archived current owner must stay editable, see BUG-1b")
	}
	if byID[archivedCarol.ID] {
		t.Fatal("archived non-owner must not be offered as a new owner")
	}
	if !archivedFlags[archivedBob.ID] {
		t.Fatal("archived current owner must be flagged as archived")
	}
	if selected[alice.ID] != "60.00" || selected[archivedBob.ID] != "40.00" {
		t.Fatalf("selected percentages = %#v", selected)
	}
}

func newCheckState(checked bool) *widget.Check {
	check := widget.NewCheck("", nil)
	check.SetChecked(checked)
	return check
}

func newEntryState(text string) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetText(text)
	return entry
}
