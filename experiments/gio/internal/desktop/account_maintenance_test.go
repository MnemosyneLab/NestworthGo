package desktop

import (
	"context"
	"errors"
	"gioui.org/io/key"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type bankMaintenanceFault struct {
	*application.Service
	updates, archives atomic.Int32
	failDetail        atomic.Bool
	started, release  chan struct{}
}

func (s *bankMaintenanceFault) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	if len(filter.AccountIDs) == 1 && s.failDetail.Swap(false) {
		return nil, errors.New("synthetic detail refresh failed")
	}
	return s.Service.ListAccounts(ctx, filter)
}

func (s *bankMaintenanceFault) UpdateAccount(ctx context.Context, id domain.AccountID, input application.AccountInput) (domain.AccountRecord, error) {
	s.updates.Add(1)
	if s.started != nil {
		close(s.started)
		<-s.release
	}
	record, err := s.Service.UpdateAccount(ctx, id, input)
	if err == nil {
		s.failDetail.Store(true)
	}
	return record, err
}

func (s *bankMaintenanceFault) ArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error {
	s.archives.Add(1)
	err := s.Service.ArchiveAccount(ctx, id, archived)
	if err == nil && archived {
		s.failDetail.Store(true)
	}
	return err
}

func copyBankProfile(t *testing.T, root string) string {
	t.Helper()
	target := createFixtureRoot(t)
	data := filepath.Join(target, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nestworth.db", "settings.json"} {
		raw, err := os.ReadFile(filepath.Join(root, "data", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(data, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return target
}

func readBankRecord(t *testing.T, service *application.Service, id domain.AccountID) domain.AccountRecord {
	t.Helper()
	rows, err := service.ListAccounts(t.Context(), domain.AccountFilter{AccountIDs: []domain.AccountID{id}, IncludeArchived: true})
	if err != nil || len(rows) != 1 {
		t.Fatalf("read account %s: %v, %v", id, rows, err)
	}
	return rows[0]
}

func TestBankMaintenanceRetainsBalanceAndHistoryAndMatchesWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	gio, prepared := openBankFixture(t, root)
	clock := func() time.Time { return accountTestClock }
	core, err := gio.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := gio.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 || rows[0].Account.InstitutionID == nil {
		t.Fatalf("sample account: %+v %v", rows, err)
	}
	old := rows[0]
	id, original := old.Account.ID, *old.Account.InstitutionID
	member := core.Members[0].ID
	partner, err := gio.CreateMember(ctx, "Partner Sample")
	if err != nil {
		t.Fatal(err)
	}
	former, err := gio.CreateMember(ctx, "Former Sample")
	if err != nil {
		t.Fatal(err)
	}
	group, err := gio.CreateGroup(ctx, "Former group")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := gio.UpdateAccount(ctx, id, application.AccountInput{
		GroupID: group.ID.String(), GroupIDSet: true,
		Ownership: []domain.OwnershipShare{{MemberID: member, ShareBPS: 9000}, {MemberID: former.ID, ShareBPS: 1000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gio.ArchiveGroup(ctx, group.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := gio.ArchiveMember(ctx, former.ID, true); err != nil {
		t.Fatal(err)
	}
	history, err := gio.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &id})
	if err != nil || len(history.Activities) != 6 {
		t.Fatalf("activity fixture: %+v %v", history, err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	gio, session := openBankFixture(t, root)
	fault := &bankMaintenanceFault{Service: gio, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	p := newAccountsPage(ctx, fault, clock, func() {})
	p.maintenance = &accountMaintenance{service: fault}
	overview := newOverviewPage(ctx, gio, clock, func() {})
	t.Cleanup(func() { release(); p.workers.Wait(); overview.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, overview: overview})
	h.size = image.Pt(700, 620)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	label := func(value string) { assertPageLabel(t, &h.router, value) }
	h.frame()
	overview.workers.Wait()
	h.frame()
	if overview.current == nil || overview.current.value.AccountCount != 1 {
		t.Fatal("initial Overview should show the sample account")
	}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, institution: original}}
	finish()
	h.click("Harbor Checking", false)
	finish()
	h.click("Account settings", false)
	h.frame()
	d := p.maintenance.draft
	if !d.custom || !slices.Contains(d.owners, former.ID) {
		t.Fatal("the form discarded an existing custom allocation or archived owner")
	}
	label("Former group · Archived")
	setBankField(h, "Account name", &d.name, "Harbor Renamed")
	h.click("Former Sample · Archived", false)
	h.click("Partner Sample", false)
	h.frame()
	setBankField(h, "Share for Sample member (%)", &d.owner(member).percent, "75.25")
	bounds := semanticBounds(t, &h.router, "Save changes", false)
	save := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(save)
	h.frame()
	label("Enter positive owner shares totaling exactly 100%, with up to two decimals.")
	if fault.updates.Load() != 0 {
		t.Fatal("invalid allocation reached the writer")
	}
	setBankField(h, "Share for Partner Sample (%)", &d.owner(partner.ID).percent, "24.75")
	h.click("Harbor Sample Bank", false)
	h.click("Filter options", true)
	h.router.Queue(key.EditEvent{Text: "Evergreen"})
	h.frame()
	h.click("Evergreen Sample Brokerage", false)
	h.frame()
	if d.institution == original.String() || d.institution == "" {
		t.Fatal("institution did not change")
	}
	h.click("Included in net worth", false)
	h.clickAt(save)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("valid submission did not start")
	}
	h.clickAt(save)
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	h.frame()
	if fault.updates.Load() != 1 || p.target.account != id {
		t.Fatal("duplicate submit or navigation escaped the pending write")
	}
	release()
	finish()
	label("Account settings saved.")
	label("Update failed. Showing the previous result.")
	if findPageLabel(&h.router, "Account settings") != nil {
		t.Fatal("stale detail still allowed changes")
	}
	h.click("Retry", false)
	finish()
	label("Harbor Renamed")
	label("US$1,734.13")
	label("Group · Former group · Archived")
	if fault.updates.Load() != 1 || p.target.account != id {
		t.Fatal("retry replayed a write or lost the ID")
	}

	wails := account.NewService(reference)
	name, institution := "Harbor Renamed", d.institution
	includeInNetWorth := false
	if _, err := wails.UpdateAccount(ctx, id.String(), account.UpdateAccountRequest{
		Name: &name, InstitutionID: &institution, InstitutionIDSet: true,
		IncludeInNetWorth: &includeInNetWorth,
		Ownership:         []wire.OwnershipShareDTO{{MemberID: member.String(), ShareBPS: 7525}, {MemberID: partner.ID.String(), ShareBPS: 2475}},
	}); err != nil {
		t.Fatal(err)
	}
	assertParity := func(archived bool) domain.AccountRecord {
		t.Helper()
		state := readBankRecord(t, gio, id)
		ref, err := wails.ListAccounts(ctx, account.AccountFilterRequest{IncludeArchived: true})
		if err != nil || len(ref) != 1 || !reflect.DeepEqual(wire.FromAccountRecord(state), ref[0]) {
			t.Fatalf("Gio and Wails disagree: %+v %+v %v", wire.FromAccountRecord(state), ref, err)
		}
		if state.Account.ID != id || (state.Account.ArchivedAt != nil) != archived ||
			!reflect.DeepEqual(state.LatestValue, changed.LatestValue) || state.Account.GroupID == nil || *state.Account.GroupID != group.ID ||
			state.Account.IncludeInNetWorth {
			t.Fatalf("unexpected identity, balance or retained group: %+v", state)
		}
		return state
	}
	assertParity(false)
	h.click("Back to accounts", false)
	finish()
	if p.target.institution != original || findPageLabel(&h.router, "Harbor Renamed") != nil {
		t.Fatal("Back lost the original institution scope or showed a moved account")
	}
	h.click("Forward", false)
	finish()
	label("Harbor Renamed")
	h.click("Archive account", false)
	h.frame()
	h.click("Cancel", false)
	h.frame()
	if fault.archives.Load() != 0 {
		t.Fatal("canceling archive wrote")
	}
	h.click("Archive account", false)
	h.frame()
	h.click("Archive account", false)
	finish()
	label("Account archived.")
	label("Update failed. Showing the previous result.")
	h.click("Retry", false)
	finish()
	if fault.archives.Load() != 1 {
		t.Fatal("archive refresh replayed the mutation")
	}
	label("Archived account. Restore it to edit its settings.")
	label("US$1,734.13")
	if err := wails.ArchiveAccount(ctx, id.String(), true); err != nil {
		t.Fatal(err)
	}
	assertParity(true)
	if p.current.rows[0].value != nil || findPageLabel(&h.router, "Account settings") != nil {
		t.Fatal("archived detail exposed valuation or settings")
	}
	h.click("Overview", false)
	overview.workers.Wait()
	h.frame()
	if overview.current == nil || overview.current.value.AccountCount != 0 {
		t.Fatal("Overview did not refresh the archive")
	}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, institution: domain.InstitutionID(institution)}}
	finish()
	label("No accounts to show.")
	h.click("Show archived", false)
	finish()
	h.click("Harbor Renamed", false)
	finish()
	h.click("Restore account", false)
	h.frame()
	h.click("Restore account", false)
	finish()
	label("Account restored.")
	label("Account settings")
	if err := wails.ArchiveAccount(ctx, id.String(), false); err != nil {
		t.Fatal(err)
	}
	final := assertParity(false)
	if fault.archives.Load() != 2 {
		t.Fatalf("archive and restore called %d times", fault.archives.Load())
	}
	h.click("Back to accounts", false)
	finish()
	label("Hide archived")
	label("Harbor Renamed")
	after, err := gio.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &id})
	if err != nil || !reflect.DeepEqual(after, history) {
		t.Fatalf("maintenance changed linked activities: %+v %v", after, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	if stored := readBankRecord(t, reopened, id); !reflect.DeepEqual(stored, final) {
		t.Fatal("maintenance did not persist across reopen")
	}
}

func TestBankMaintenanceRejectsStaleAffiliationWithoutADifferentWrite(t *testing.T) {
	ctx := t.Context()
	gio, _ := openBankFixture(t, createFixtureRoot(t))
	core, err := gio.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	base, err := gio.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(base) != 1 {
		t.Fatalf("fixture records: %v %v", base, err)
	}
	before := base[0]
	group, err := gio.CreateGroup(ctx, "Pending group")
	if err != nil {
		t.Fatal(err)
	}
	p := newAccountsPage(ctx, gio, func() time.Time { return accountTestClock }, func() {})
	p.maintenance = &accountMaintenance{service: gio}
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	show := func(id domain.AccountID) {
		h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: id}}
		finish()
	}
	show(before.Account.ID)
	h.click("Account settings", false)
	h.frame()
	draft := p.maintenance.draft
	setBankField(h, "Account name", &draft.name, "Should stay as a draft")
	h.click("No group", false)
	h.click("Filter options", true)
	h.router.Queue(key.EditEvent{Text: "Pending group"})
	h.frame()
	h.click("Pending group", false)
	h.frame()
	if draft.group != group.ID.String() {
		t.Fatal("the test did not select the live group")
	}
	if err := gio.ArchiveGroup(ctx, group.ID, true); err != nil {
		t.Fatal(err)
	}
	h.click("Save changes", false)
	finish()
	assertPageLabel(t, &h.router, "Choose an active group or clear the group.")
	if got := readBankRecord(t, gio, before.Account.ID); !reflect.DeepEqual(got, before) {
		t.Fatalf("a rejected update committed account state: %+v", got)
	}
	if p.maintenance.draft != draft || draft.name.Text() != "Should stay as a draft" {
		t.Fatal("the form lost other input after the reference changed")
	}
	h.click("Cancel", false)
	h.frame()

	// Managed contracts require a Holdings account. Use a valid synthetic
	// contract to check the service guard without assigning it to a bank-balance account.
	holdingAccount, err := gio.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic contracts", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{core.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	maturity, dayBasis, fee, days := "2026-12-20", "calendar", "0", 0
	product := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{
		AccountID: holdingAccount.Account.ID.String(), Currency: "USD", Principal: "100", CurrentValue: "100", TotalCostBasis: "100", CashExcludesProduct: true,
		Terms:  application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic managed deposit", StartOn: "2026-11-20", MaturityOn: &maturity, InterestMode: "none"},
		Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &days, DayBasis: &dayBasis, NormalExitFee: &fee},
	}}
	preview, err := gio.PreviewProductOperation(ctx, product)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gio.RecordProductOperation(ctx, product, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	wailsErr := account.NewService(gio).ArchiveAccount(ctx, holdingAccount.Account.ID.String(), true)
	var wireErr *apierror.WireError
	if !errors.As(wailsErr, &wireErr) || wireErr.Code != string(domain.ErrManagedPosition) {
		t.Fatalf("Wails did not enforce the open contract guard: %v", wailsErr)
	}
	show(holdingAccount.Account.ID)
	assertPageLabel(t, &h.router, "Account settings")
	beforeBlockedArchive := readBankRecord(t, gio, holdingAccount.Account.ID)
	beforeBlockedActivity := openingActivity(t, gio, holdingAccount.Account.ID)
	h.click("Archive account", false)
	h.frame()
	h.click("Archive account", false)
	finish()
	assertPageLabel(t, &h.router, "Accounts with open managed contracts cannot be archived.")
	if !reflect.DeepEqual(readBankRecord(t, gio, holdingAccount.Account.ID), beforeBlockedArchive) ||
		!reflect.DeepEqual(openingActivity(t, gio, holdingAccount.Account.ID), beforeBlockedActivity) {
		t.Fatal("blocked archive changed the managed account or history")
	}
}
