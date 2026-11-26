package desktop

import (
	"slices"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAccountTypeChooserStartsLegalCombinations(t *testing.T) {
	for _, kind := range slices.Concat(primaryAccountTypes, moreAccountTypes) {
		role, mode := accountTypeSetup(kind)
		if !domain.IsValidAccountCombination(kind, role, mode) {
			t.Errorf("%s starts as %s/%s, which is not a legal combination", kind, role, mode)
		}
		modes := accountTypeModes(kind)
		if len(modes) == 0 || !slices.Contains(modes, mode) {
			t.Errorf("%s card lists %v, want it to include the starting mode %s", kind, modes, mode)
		}
		for _, listed := range modes {
			if !domain.IsValidAccountCombination(kind, role, listed) {
				t.Errorf("%s card promises %s, which is not legal for %s", kind, listed, role)
			}
		}
	}
	for _, kind := range primaryAccountTypes {
		if slices.Contains(moreAccountTypes, kind) {
			t.Errorf("%s is listed as both common and more", kind)
		}
	}
}

func TestAccountTypeChooserLeadsToTheDetailsForm(t *testing.T) {
	root := createFixtureRoot(t)
	seedAccountFixture(t, root)
	service, session := openBankFixture(t, root)
	defer func() { _ = session.Close() }()
	p, h, finish := accountShellHarness(t, service, service, "")

	h.click("Add account", false)
	finish()
	if p.creation.chooser == nil || p.creation.draft != nil {
		t.Fatal("Add account did not open the type chooser")
	}
	assertPageLabel(t, &h.router, "What kind of account is it?")
	h.click("Cancel", false)
	finish()
	if p.creation.chooser != nil || p.creation.draft != nil {
		t.Fatal("cancelling the chooser left a draft behind")
	}
	h.focused(&p.createAccount)

	startNewAccount(h, finish, "Brokerage")
	d := p.creation.draft
	if p.creation.chooser != nil || d == nil || d.kind != domain.TypeBrokerage || d.mode != domain.TrackingHoldings || d.role != domain.RoleAsset {
		t.Fatalf("continuing with Brokerage started %+v", d)
	}
	assertPageLabel(t, &h.router, "Account details")
	h.click("Cancel", false)
	finish()
	if p.creation.draft != nil {
		t.Fatal("cancelling the form kept the draft")
	}
}
