package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/testledger"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type accountCreationTestService struct {
	*application.Service
	writes             atomic.Int32
	started, release   chan struct{}
	failWrite          bool
	failRead           bool
	pendingReadFailure atomic.Bool
}

func (s *accountCreationTestService) CreateAccount(ctx context.Context, input application.AccountInput) (domain.AccountRecord, error) {
	first := s.writes.Add(1) == 1
	if first && s.failWrite {
		return domain.AccountRecord{}, errors.New("synthetic write failure before the account transaction")
	}
	if first && s.started != nil {
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return domain.AccountRecord{}, ctx.Err()
		}
	}
	value, err := s.Service.CreateAccount(ctx, input)
	if err == nil && s.failRead {
		s.pendingReadFailure.Store(true)
	}
	return value, err
}

func (s *accountCreationTestService) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	if len(filter.AccountIDs) == 1 && s.pendingReadFailure.Swap(false) {
		return nil, errors.New("synthetic failure rereading the committed account")
	}
	return s.Service.ListAccounts(ctx, filter)
}

func createFixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func openBankFixture(t *testing.T, root string) (*application.Service, *pilot.Session) {
	t.Helper()
	database := filepath.Join(root, "data", "nestworth.db")
	settingsPath := filepath.Join(root, "data", "settings.json")
	if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
		if err := testledger.Seed(t.Context(), database, settingsPath); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_DATABASE_PATH", database)
	t.Setenv("NESTWORTH_SETTINGS_PATH", settingsPath)
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	session, err := pilot.OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	s := application.NewService(sqlite.NewRepository(session.DB))
	s.SetClock(func() time.Time { return accountTestClock })
	if err := s.SetStoredFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	s.SetQuoteCacheTTL(time.Hour)
	return s, session
}

func showBankAccountForm(h *previewHarness, id domain.InstitutionID) {
	h.t.Helper()
	p := h.shell.accounts
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, institution: id}}
	h.frame()
	p.workers.Wait()
	h.frame()
	h.click("Add bank account", false)
	p.workers.Wait()
	h.frame()
	if p.creation.draft == nil || !p.creation.draft.available {
		h.t.Fatal("bank account form did not load")
	}
}

func setBankField(h *previewHarness, label string, field *ui.TextField, text string) {
	h.t.Helper()
	h.click(label, true)
	h.router.Queue(key.EditEvent{Text: text, Range: key.Range{End: field.Editor.Len()}})
	h.frame()
}

func reviewBankAccount(h *previewHarness) {
	h.t.Helper()
	h.click("Review account", false)
	h.frame()
}

func TestBankAccountCreateMatchesWailsAndRefreshesWithoutReplayingWrite(t *testing.T) {
	ctx := t.Context()
	root, reference := createFixtureRoot(t), createFixtureRoot(t)
	ids := seedAccountFixture(t, root)
	if err := os.Mkdir(filepath.Join(reference, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nestworth.db", "settings.json"} {
		raw, err := os.ReadFile(filepath.Join(root, "data", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(reference, "data", name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gio, session := openBankFixture(t, root)
	wails, _ := openBankFixture(t, reference)
	members, err := gio.ListMembers(ctx, true)
	if err != nil || len(members) != 2 {
		t.Fatalf("fixture members: %v %v", members, err)
	}
	var archived domain.MemberID
	for _, member := range members {
		if member.ArchivedAt != nil {
			archived = member.ID
		}
	}
	for _, service := range []*application.Service{gio, wails} {
		if err := service.ArchiveMember(ctx, archived, false); err != nil {
			t.Fatal(err)
		}
	}
	fault := &accountCreationTestService{Service: gio, started: make(chan struct{}), release: make(chan struct{}), failRead: true}
	clock := func() time.Time { return accountTestClock }
	p := newAccountsPage(ctx, fault, clock, func() {})
	p.creation = &balanceAccountCreation{service: fault}
	overview := newOverviewPage(ctx, gio, clock, func() {})
	directory := newDirectoryPage(newInstitutionsPage(ctx, gio, func() {}))
	release := sync.OnceFunc(func() { close(fault.release) })
	t.Cleanup(func() { release(); p.workers.Wait(); overview.workers.Wait(); directory.wait() })
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, overview: overview, directory: directory})
	h.size = image.Pt(1120, 900)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	finish := func() { h.frame(); p.workers.Wait(); directory.wait(); h.frame() }
	h.frame()
	overview.workers.Wait()
	h.frame()
	if overview.current == nil {
		t.Fatal("initial overview did not load")
	}
	before := overview.current.value.AccountCount
	h.click("Directory", false)
	finish()
	searchDirectory(h, "Search institutions", "Pacific")
	h.click("View", false)
	finish()
	h.click("View accounts", false)
	finish()
	h.click("Add bank account", false)
	finish()
	label("New bank account")
	d := p.creation.draft
	if len(d.owners) != 0 || d.currency != "AUD" || d.institution.ID != ids.pacific {
		t.Fatal("unexpected scoped form defaults")
	}
	reviewBankAccount(h)
	label("Enter an account name.")
	setBankField(h, "Account name", &d.name, "Fixture bank savings")
	setBankField(h, "Opening balance", &d.opening, "1,000")
	reviewBankAccount(h)
	label("Enter a supported nonnegative amount using a decimal point.")
	setBankField(h, "Opening balance", &d.opening, "123.45")
	reviewBankAccount(h)
	label("Choose at least one active owner.")
	h.click("Bea Example", false)
	h.click("Ana Example", false)
	h.click("AUD", false)
	for _ = range 8 {
		h.press(key.NameDownArrow)
	}
	h.press(key.NameReturn)
	if d.currency != "USD" {
		t.Fatalf("currency picker selected %v", d.currency)
	}
	reviewBankAccount(h)
	label("Review bank account")
	h.click("Back to details", false)
	h.frame()
	if d.name.Text() != "Fixture bank savings" || fault.writes.Load() != 0 {
		t.Fatal("canceling review cleared the draft or submitted it")
	}
	reviewBankAccount(h)
	bounds := semanticBounds(t, &h.router, "Create bank account", false)
	c := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(c)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not start the writer")
	}
	h.clickAt(c)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: overviewRoute}}
	h.frame()
	if fault.writes.Load() != 1 || h.shell.activeRoute() != accountsRoute || p.creation.submission == nil {
		t.Fatal("a duplicate confirmation or navigation escaped the pending write")
	}
	release()
	p.workers.Wait()
	finish()
	label("Bank account created.")
	label("Could not load account data.")
	if p.target.account == "" || p.creation.draft != nil {
		t.Fatal("writer success did not switch to the created ID")
	}
	createdID := p.target.account
	h.click("Retry", false)
	finish()
	label("Fixture bank savings")
	label("USD · $ 123.45")
	label("AUD · A$ 185.18")
	label("Group · Default group")
	if fault.writes.Load() != 1 {
		t.Fatal("read retry wrote again")
	}

	inst, group := ids.pacific.String(), fixtureDefaultAccountGroup(t, wails)
	want, err := account.NewService(wails).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Fixture bank savings", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InstitutionID: &inst, GroupID: &group, InitialAmount: "123.45", IncludeInNetWorth: true, IncludeInLiquidAssets: true,
		Ownership: []wire.OwnershipShareDTO{{MemberID: d.owners[0].String(), ShareBPS: 5000}, {MemberID: d.owners[1].String(), ShareBPS: 5000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	readCreated := func(service *application.Service, id domain.AccountID) domain.AccountRecord {
		t.Helper()
		rows, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("could not reread account %s: %v", id, err)
		}
		return rows[0]
	}
	normalize := func(r domain.AccountRecord) wire.AccountRecordDTO {
		dto := wire.FromAccountRecord(r)
		dto.Account.ID, dto.LatestValue.ID, dto.LatestValue.AccountID = "", "", ""
		return dto
	}
	got := normalize(readCreated(gio, createdID))
	if expected := normalize(readCreated(wails, domain.AccountID(want.Account.ID))); !reflect.DeepEqual(got, expected) {
		t.Fatalf("Gio created %+v; Wails created %+v", got, expected)
	}
	h.click("Back to accounts", false)
	finish()
	if p.target.institution != ids.pacific || p.target.account != "" {
		t.Fatal("Back lost institution scope")
	}
	h.focused(&p.createBank)
	h.click("Overview", false)
	overview.workers.Wait()
	h.frame()
	expected, err := wails.Overview(ctx, domain.AccountFilter{})
	if err != nil || overview.current == nil || overview.current.value.AccountCount != before+1 || !overview.current.value.NetWorth.Equal(expected.NetWorth) {
		t.Fatalf("Overview did not reread the committed account: %v %+v", err, overview.current)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	if after := normalize(readCreated(reopened, createdID)); !reflect.DeepEqual(after, got) {
		t.Fatal("committed record changed on reopen")
	}
}

func TestBankAccountRevalidatesArchivedReferencesAtConfirmation(t *testing.T) {
	root := createFixtureRoot(t)
	ids := seedAccountFixture(t, root)
	s, _ := openBankFixture(t, root)
	ctx := t.Context()
	p := newAccountsPage(ctx, s, func() time.Time { return accountTestClock }, func() {})
	fault := &accountCreationTestService{Service: s}
	p.creation = &balanceAccountCreation{service: fault}
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	showBankAccountForm(h, ids.pacific)
	d := p.creation.draft
	if len(d.owners) != 1 || len(d.members) != 1 || d.members[0].Name != "Ana Example" {
		t.Fatal("active sole owner was not selected or archived member was offered")
	}
	setBankField(h, "Account name", &d.name, "Refused bank account")
	reviewBankAccount(h)
	if err := s.ArchiveInstitution(ctx, ids.pacific, true); err != nil {
		t.Fatal(err)
	}
	h.click("Create bank account", false)
	finish()
	assertPageLabel(t, &h.router, "This institution is no longer active.")
	if findPageLabel(&h.router, "Review account") != nil || p.creation.draft.available {
		t.Fatal("archived institution still offered confirmation")
	}
	h.click("Cancel", false)
	finish()
	if findPageLabel(&h.router, "Add bank account") != nil {
		t.Fatal("archived institution still offered creation")
	}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, institution: domain.NewInstitutionID()}}
	finish()
	assertPageLabel(t, &h.router, "Account destination unavailable.")
	if findPageLabel(&h.router, "Add bank account") != nil {
		t.Fatal("missing institution offered creation")
	}

	showBankAccountForm(h, ids.unused)
	d = p.creation.draft
	setBankField(h, "Account name", &d.name, "Refused owner account")
	// Give the household another active owner, then archive the old sole owner
	// only after existing live accounts also retain an active owner.
	members, err := s.ListMembers(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	other := slices.IndexFunc(members, func(m domain.Member) bool { return m.ArchivedAt != nil })
	if other < 0 {
		t.Fatal("fixture has no archived member")
	}
	if err := s.ArchiveMember(ctx, members[other].ID, false); err != nil {
		t.Fatal(err)
	}
	reviewBankAccount(h)
	if err := s.ArchiveMember(ctx, d.owners[0], true); err != nil {
		t.Fatal(err)
	}
	h.click("Create bank account", false)
	finish()
	assertPageLabel(t, &h.router, "An owner is no longer active. Review the owners again.")
	if len(d.owners) != 0 || d.name.Text() != "Refused owner account" {
		t.Fatal("reference refresh replaced the draft or silently chose another owner")
	}
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Choose at least one active owner.")
	if fault.writes.Load() != 2 {
		t.Fatalf("unexpected submits %d", fault.writes.Load())
	}
	rows, err := s.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || slices.ContainsFunc(rows, func(row domain.AccountRecord) bool {
		return row.Account.Name == "Refused bank account" || row.Account.Name == "Refused owner account"
	}) {
		t.Fatalf("invalid reference wrote an account: %v", err)
	}
}

// The existing creation fixtures have one active group.
func fixtureDefaultAccountGroup(t *testing.T, service *application.Service) string {
	t.Helper()
	groups, err := service.ListGroups(t.Context(), false)
	if err != nil || len(groups) != 1 || groups[0].Name != "Default group" {
		t.Fatalf("fixture default group: %+v, %v", groups, err)
	}
	return groups[0].ID.String()
}
