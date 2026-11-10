package mcpserver

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestHostReviewFreezesReadableCashIdentity(t *testing.T) {
	fx := newHostFixture(t)
	members, _ := fx.app.ListMembers(t.Context(), false)
	alice := members[0]
	bob, err := fx.app.CreateMember(t.Context(), "Bob")
	if err != nil {
		t.Fatal(err)
	}
	bankA, err := fx.app.CreateInstitution(t.Context(), "Bank A", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	bankB, err := fx.app.CreateInstitution(t.Context(), "Bank B", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	makeAccount := func(bank domain.Institution, owner domain.Member) domain.AccountRecord {
		t.Helper()
		a, err := fx.app.CreateAccount(t.Context(), application.AccountInput{Name: "Same cash account", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "0", InstitutionID: bank.ID.String(), OwnerIDs: []domain.MemberID{owner.ID}})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a, b := makeAccount(bankA, alice), makeAccount(bankB, bob)
	command := hostCommand(fx)
	command.AccountID = a.Account.ID.String()
	command.Currency = " usd "
	first, err := fx.local.previewLegacyFixture(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	command.AccountID = b.Account.ID.String()
	second, err := fx.local.previewLegacyFixture(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	identity := first.Binding.CashAccount
	if identity == nil || identity.Institution != "Bank A" || !reflect.DeepEqual(identity.Owners, []string{"Alice"}) || identity.AccountType != "bank_account" || first.Binding.Plan.Command.Currency != "USD" {
		t.Fatalf("identity: %+v", identity)
	}
	if second.Binding.CashAccount.Institution != "Bank B" || !reflect.DeepEqual(second.Binding.CashAccount.Owners, []string{"Bob"}) {
		t.Fatal("same-name account identity lost")
	}
	// Every frozen visible field participates in the confirmation digest.
	for _, change := range []func(*HostCashAccountIdentity){func(v *HostCashAccountIdentity) { v.Institution = "Changed" }, func(v *HostCashAccountIdentity) { v.Owners = []string{"Changed"} }, func(v *HostCashAccountIdentity) { v.AccountType = "cash" }, func(v *HostCashAccountIdentity) { v.DefaultCurrency = "EUR" }} {
		altered := first.Binding
		copy := *identity
		change(&copy)
		altered.CashAccount = &copy
		digest, _ := reviewDigest(altered)
		if digest == first.Digest {
			t.Fatal("identity omitted from digest")
		}
	}
	if _, err := fx.app.UpdateInstitution(t.Context(), bankA.ID, "Renamed bank"); err != nil {
		t.Fatal(err)
	}
	frozen, err := fx.local.InspectHostChange(t.Context(), first.Binding.ID)
	if err != nil || frozen.State != ReviewNeedsPreview || frozen.Binding.CashAccount.Institution != "Bank A" {
		t.Fatal("live label changed frozen review or failed to invalidate")
	}
	if _, err := fx.local.executeLegacyFixture(t.Context(), first.Binding.ID); err == nil {
		t.Fatal("changed identity committed")
	}
	assertActivityCount(t, fx, 0)
	command.AccountID = a.Account.ID.String()
	fresh, err := fx.local.previewLegacyFixture(t.Context(), command)
	if err != nil || fresh.Binding.CashAccount.Institution != "Renamed bank" || fresh.Digest == first.Digest {
		t.Fatal("fresh identity not captured")
	}
	if _, err := fx.app.UpdateMember(t.Context(), alice.ID, "Renamed Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.local.executeLegacyFixture(t.Context(), fresh.Binding.ID); err == nil {
		t.Fatal("owner rename did not invalidate")
	}
	assertActivityCount(t, fx, 0)
	// Even metadata cannot distinguish completely identical readable identities.
	_ = makeAccount(bankB, bob)
	command.AccountID = b.Account.ID.String()
	_, err = fx.local.previewLegacyFixture(t.Context(), command)
	requireLocalError(t, err, "ambiguous_account_identity")
	assertActivityCount(t, fx, 0)
}

func TestHostReviewLegacyIdentityRequiresPreviewButKeepsRecovery(t *testing.T) {
	fx := newHostFixture(t)
	review := hostPreview(t, fx)
	review.Binding.CashAccount = nil
	review.Digest, _ = reviewDigest(review.Binding)
	if err := fx.local.saveHostReview(review); err != nil {
		t.Fatal(err)
	}
	got, err := fx.local.InspectHostChange(t.Context(), review.Binding.ID)
	if err != nil || got.State != ReviewNeedsPreview {
		t.Fatalf("legacy: %s %v", got.State, err)
	}
	if _, err := fx.local.executeLegacyFixture(t.Context(), review.Binding.ID); err == nil {
		t.Fatal("legacy identity confirmed")
	}
	assertActivityCount(t, fx, 0)
	// Completed legacy receipts are still recoverable, not converted to new work.
	completed := hostPreview(t, fx)
	completed, err = fx.local.executeLegacyFixture(t.Context(), completed.Binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed.Binding.CashAccount = nil
	completed.Digest, _ = reviewDigest(completed.Binding)
	if err := fx.local.saveHostReview(completed); err != nil {
		t.Fatal(err)
	}
	got, err = fx.local.InspectHostChange(t.Context(), completed.Binding.ID)
	if err != nil || got.State != ReviewSucceeded {
		t.Fatalf("legacy receipt: %s %v", got.State, err)
	}
	assertActivityCount(t, fx, 1)
}

// Deliberately withhold AfterFunc delivery: revocation must be observed
// synchronously after acquiring the application gate, not only via callbacks.
type delayedReviewCancellation struct{ context.Context }

func (delayedReviewCancellation) Value(any) any                { return nil }
func (delayedReviewCancellation) AfterFunc(func()) func() bool { return func() bool { return true } }
func TestHostReviewRevokedWhileWaitingForApplicationGate(t *testing.T) {
	fx := newHostFixture(t)
	review := hostPreview(t, fx)
	fx.local.ctx = delayedReviewCancellation{fx.local.ctx}
	entered, release, gateDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		gateDone <- fx.app.WithPrivateConfiguration(context.Background(), func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	done := make(chan error, 1)
	go func() {
		_, err := fx.local.executeLegacyFixture(context.Background(), review.Binding.ID)
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for fx.local.reviewMu.TryLock() {
		fx.local.reviewMu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("confirmation did not enter review gate")
		}
		time.Sleep(time.Millisecond)
	}
	if err := fx.local.RevokeForRestore(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(release)
	released = true
	if err := <-gateDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		requireLocalError(t, err, "session_revoked")
	case <-time.After(time.Second):
		t.Fatal("revoked confirmation did not finish")
	}
	assertActivityCount(t, fx, 0)
	saved, err := fx.local.loadHostReview(review.Binding.ID)
	if err != nil || saved.State != ReviewAwaitingConfirmation {
		t.Fatal("revoked request durably admitted a submission")
	}
}

func TestHostReviewCurrencyFailureDoesNotPersistInvisibleReview(t *testing.T) {
	fx := newHostFixture(t)
	before, err := fx.local.service.repository.ListConfiguration("mcp.")
	if err != nil {
		t.Fatal(err)
	}
	for _, currency := range []string{"", "US D", "ZZZ"} {
		command := hostCommand(fx)
		command.Currency = currency
		if _, err := fx.local.previewLegacyFixture(t.Context(), command); err == nil {
			t.Fatal("invalid currency accepted")
		}
	}
	after, err := fx.local.service.repository.ListConfiguration("mcp.")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("invalid currency persisted private state")
	}
	assertActivityCount(t, fx, 0)
}

func TestHostReviewInstitutionFallbackNameIsDistinctFromUnassigned(t *testing.T) {
	fx := newHostFixture(t)
	members, _ := fx.app.ListMembers(t.Context(), false)
	institution, err := fx.app.CreateInstitution(t.Context(), "没有对应机构", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	other, err := fx.app.CreateAccount(t.Context(), application.AccountInput{Name: "Checking", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "0", InstitutionID: institution.ID.String(), OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	unassigned := hostPreview(t, fx)
	command := hostCommand(fx)
	command.AccountID = other.Account.ID.String()
	assigned, err := fx.local.previewLegacyFixture(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if unassigned.Binding.CashAccount.Institution != "" || assigned.Binding.CashAccount.Institution != "没有对应机构" {
		t.Fatal("assigned/unassigned identity lost")
	}
	// The desktop renders an explicit assigned/not-assigned marker from this
	// digest-bound presence, never a user-controlled name as the missing label.
	if _, err := fx.app.UpdateAccount(t.Context(), other.Account.ID, application.AccountInput{InstitutionIDSet: true, InstitutionID: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.local.executeLegacyFixture(t.Context(), assigned.Binding.ID); err == nil {
		t.Fatal("institution removal reused old identity")
	}
	_, err = fx.local.previewLegacyFixture(t.Context(), command)
	requireLocalError(t, err, "ambiguous_account_identity")
	assertActivityCount(t, fx, 0)
}
