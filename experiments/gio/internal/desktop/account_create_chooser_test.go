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
	bank := accountTypeModes(domain.TypeBankAccount)
	wallet := accountTypeModes(domain.TypeDigitalWallet)
	if !slices.Equal(bank, []domain.TrackingMode{domain.TrackingBalance, domain.TrackingHoldings}) {
		t.Fatalf("bank modes %v, want balance then holdings", bank)
	}
	if !slices.Equal(wallet, bank) {
		t.Fatalf("wallet modes %v, want balance then holdings", wallet)
	}
	if crypto := accountTypeModes(domain.TypeCryptoExchange); len(crypto) != 1 || crypto[0] != domain.TrackingHoldings {
		t.Fatalf("crypto modes %v, want only holdings", crypto)
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

func TestNewAccountChoosesTrackingWithoutChangingType(t *testing.T) {
	root := createFixtureRoot(t)
	seedAccountFixture(t, root)
	service, session := openBankFixture(t, root)
	defer func() { _ = session.Close() }()
	p, h, finish := accountShellHarness(t, service, service, "")

	openType := func() {
		t.Helper()
		h.click("Add account", false)
		finish()
	}
	reviewNamed := func(name string) {
		t.Helper()
		d := p.creation.draft
		if d == nil || !d.available {
			t.Fatal("details form did not load")
		}
		setBankField(h, "Account name", &d.name, name)
		reviewBankAccount(h)
		if p.creation.review == nil {
			t.Fatalf("review did not open, problem %v", d.problem)
		}
	}
	leaveForm := func() {
		t.Helper()
		if p.creation.review != nil {
			h.click("Back to details", false)
			h.frame()
		}
		h.click("Cancel", false)
		finish()
	}

	openType()
	h.click("Bank account", false)
	h.click("Continue", false)
	finish()
	assertPageLabel(t, &h.router, "How should this account be tracked?")
	assertPageLabel(t, &h.router, "Choose a tracking method for Bank account.")
	if findPageLabel(&h.router, "Manual valuation") != nil {
		t.Fatal("bank tracking choices include manual valuation")
	}
	h.click("Cash and holdings", false)
	h.click("Continue", false)
	finish()
	d := p.creation.draft
	if p.creation.chooser != nil || d == nil || d.kind != domain.TypeBankAccount || d.mode != domain.TrackingHoldings {
		t.Fatalf("bank holdings draft = %+v, chooser open %v", d, p.creation.chooser != nil)
	}
	reviewNamed("Synthetic bank holdings")
	got := p.creation.review.input
	if got.AccountType != domain.TypeBankAccount.String() || got.TrackingMode != string(domain.TrackingHoldings) {
		t.Fatalf("review submitted type %s mode %s", got.AccountType, got.TrackingMode)
	}
	leaveForm()

	openType()
	h.click("Bank account", false)
	h.click("Continue", false)
	finish()
	h.click("Continue", false)
	finish()
	d = p.creation.draft
	if d == nil || d.kind != domain.TypeBankAccount || d.mode != domain.TrackingBalance {
		t.Fatalf("default bank draft = %+v", d)
	}
	reviewNamed("Synthetic bank balance")
	got = p.creation.review.input
	if got.AccountType != domain.TypeBankAccount.String() || got.TrackingMode != string(domain.TrackingBalance) {
		t.Fatalf("default review submitted type %s mode %s", got.AccountType, got.TrackingMode)
	}
	leaveForm()

	openType()
	h.click("Crypto exchange", false)
	h.click("Continue", false)
	finish()
	if findPageLabel(&h.router, "How should this account be tracked?") != nil {
		t.Fatal("a single-mode account stopped to choose a tracking method")
	}
	d = p.creation.draft
	if p.creation.chooser != nil || d == nil || d.kind != domain.TypeCryptoExchange || d.mode != domain.TrackingHoldings {
		t.Fatalf("crypto draft = %+v", d)
	}
	assertPageLabel(t, &h.router, "Account details")
}
