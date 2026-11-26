package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/directory"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestAccountRecommendationsAndExplicitChoices(t *testing.T) {
	ctx := t.Context()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	create := func(name string) domain.Institution {
		t.Helper()
		item, err := service.CreateInstitution(ctx, name, domain.InstitutionExchange)
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	archived := create("Archived exchange")
	first := create("Zeta synthetic market")
	second := create("Alpha synthetic market")
	if err := service.ArchiveInstitution(ctx, archived.ID, true); err != nil {
		t.Fatal(err)
	}
	p, h, finish := accountShellHarness(t, service, service, "")
	startNewAccount(h, finish, "Property")
	d := p.creation.draft
	group := fixtureDefaultAccountGroup(t, service)
	if d == nil || d.kind != domain.TypeProperty || d.mode != domain.TrackingManualValue || d.institution.ID != "" || d.group != group {
		t.Fatal("property should open directly, without an institution, and with the active default group")
	}
	h.click("Cancel", false)
	finish()
	startNewAccount(h, finish, "Brokerage")
	d = p.creation.draft
	if d == nil || d.kind != domain.TypeBrokerage || d.mode != domain.TrackingHoldings || d.institution.Name != "Harbor Sample Bank" {
		t.Fatalf("brokerage should keep holdings and the earliest matching institution, got %#v", d)
	}
	chooseAccountType(h, "Asset type", 2)
	if d.kind != domain.TypeCryptoExchange || d.institution.ID != first.ID {
		t.Fatalf("crypto exchange selected %+v, want earliest active exchange %s", d.institution, first.Name)
	}
	pickAccountReference(h, "Institution", "AlPhA", second.Name)
	if d.institution.ID != second.ID || !d.institutionExplicit {
		t.Fatal("name search did not bind an explicit institution")
	}
	chooseAccountType(h, "Asset type", -1)
	if d.kind != domain.TypeInvestmentAccount || d.institution.ID != second.ID {
		t.Fatal("an explicit institution was replaced during the type change")
	}
	pickAccountReference(h, "Institution", "No institution", "No institution")
	chooseAccountType(h, "Asset type", 1)
	h.click("More account details", false)
	h.frame()
	pickAccountReference(h, "Group", "No group", "No group")
	// A refresh after a stale submission must respect explicit absence.
	p.loadAccountCreationReferences()
	finish()
	if d.kind != domain.TypeCryptoExchange || d.institution.ID != "" || d.group != "" || !d.groupExplicit {
		t.Fatal("an explicit empty institution or group was replaced by a default")
	}
}

func chooseAccountType(h *previewHarness, label string, steps int) {
	h.t.Helper()
	h.click(label, true)
	direction := key.NameDownArrow
	if steps < 0 {
		direction, steps = key.NameUpArrow, -steps
	}
	for range steps {
		h.press(direction)
	}
	h.press(key.NameReturn)
	h.frame()
}

func pickAccountReference(h *previewHarness, field, search, choice string) {
	h.t.Helper()
	h.click(field, true)
	h.router.Queue(key.EditEvent{Text: search})
	h.frame()
	h.click(choice, false)
	h.frame()
}

type inlineInstitutionFault struct {
	*accountCreationTestService
	institutionWrites atomic.Int32
	failReferences    atomic.Bool
	started, release  chan struct{}
}

func (s *inlineInstitutionFault) CreateInstitution(ctx context.Context, name string, kind domain.InstitutionType, icons ...string) (domain.Institution, error) {
	if s.institutionWrites.Add(1) == 1 {
		return domain.Institution{}, errors.New("synthetic institution writer failure")
	}
	close(s.started)
	<-s.release
	record, err := s.Service.CreateInstitution(ctx, name, kind, icons...)
	if err == nil {
		s.failReferences.Store(true)
	}
	return record, err
}

func (s *inlineInstitutionFault) Bootstrap(ctx context.Context) (application.Bootstrap, error) {
	if s.failReferences.Swap(false) {
		return application.Bootstrap{}, errors.New("synthetic reference read failure after institution write")
	}
	return s.Service.Bootstrap(ctx)
}

func TestInlineInstitutionSeparatelyPersistsAndOnlyReadsAfterSuccess(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	_, seed := openBankFixture(t, root)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	fault := &inlineInstitutionFault{
		accountCreationTestService: &accountCreationTestService{Service: service, failWrite: true, failRead: true},
		started:                    make(chan struct{}), release: make(chan struct{}),
	}
	p := newAccountsPage(ctx, fault, func() time.Time { return accountTestClock }, func() {})
	p.creation = &balanceAccountCreation{service: fault}
	dir := newDirectoryPage(newInstitutionsPage(ctx, service, func() {}))
	release := sync.OnceFunc(func() { close(fault.release) })
	t.Cleanup(func() { release(); p.workers.Wait(); dir.wait() })
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, directory: dir})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		h.frame()
		p.workers.Wait()
		dir.wait()
		h.frame()
		p.workers.Wait()
		dir.wait()
		h.frame()
	}
	label := func(s string) { assertPageLabel(t, &h.router, s) }
	h.frame()
	h.click("Directory", false)
	finish()
	searchDirectory(h, "Search institutions", "Paper")
	h.click("Accounts", false)
	finish()
	startNewAccount(h, finish, "Credit card")
	d := p.creation.draft
	setBankField(h, "Account name", &d.name, "Abandoned sample debt")
	setBankField(h, "Opening amount owed", &d.opening, "84.25")
	h.click("Add institution", false)
	h.frame()
	label("The institution is saved separately and stays in Directory if you cancel the account.")
	if d.newInstitution.kind != domain.InstitutionBank {
		t.Fatal("credit-card institution form did not default to a bank")
	}
	h.click("Add", false)
	h.frame()
	label("Enter an institution name.")
	setBankField(h, "Institution name", &d.newInstitution.name, "Discarded synthetic institution")
	h.click("Cancel", false)
	h.frame()
	h.focused(&d.addInstitution)
	if fault.institutionWrites.Load() != 0 || d.name.Text() != "Abandoned sample debt" || d.opening.Text() != "84.25" {
		t.Fatal("canceling the institution lost the account draft or wrote data")
	}
	h.click("Add institution", false)
	h.frame()
	edit := d.newInstitution
	setBankField(h, "Institution name", &edit.name, "Paper Synthetic Employer")
	chooseAccountType(h, "Institution type", 4)
	if edit.kind != domain.InstitutionEmployer || edit.icon != domain.DefaultIconForInstitutionType(domain.InstitutionEmployer) {
		t.Fatal("changing the institution type did not update its default icon")
	}
	pickAccountReference(h, "Icon", "Vault", "Vault")
	h.click("Add", false)
	finish()
	label("Could not save the institution. Try again.")
	if d.newInstitution != edit || edit.name.Text() != "Paper Synthetic Employer" || edit.icon != "vault" || edit.kind != domain.InstitutionEmployer {
		t.Fatal("failed institution write discarded draft fields")
	}
	bounds := semanticBounds(t, &h.router, "Add", false)
	center := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(center)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("second institution submit did not start")
	}
	h.clickAt(center)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: directoryRoute}}
	h.frame()
	if fault.institutionWrites.Load() != 2 || h.shell.activeRoute() != accountsRoute {
		t.Fatal("duplicate confirm or navigation escaped an active institution write")
	}
	release()
	finish()
	saved := "Institution saved: Paper Synthetic Employer. It stays in Directory if you cancel the account."
	label(saved)
	label("Could not load the owners and institution.")
	h.click("Retry", false)
	finish()
	label(saved)
	if p.creation.draft != d || d.newInstitution != nil || fault.institutionWrites.Load() != 2 || d.institution.Name != "Paper Synthetic Employer" ||
		d.name.Text() != "Abandoned sample debt" || d.opening.Text() != "84.25" {
		t.Fatal("read retry lost the account draft, replayed the institution writer or failed to auto-select its saved ID")
	}
	institutionID := d.institution.ID
	h.click("Cancel", false)
	finish()
	if fault.writes.Load() != 0 || p.creation.draft != nil {
		t.Fatal("canceling the account after creating the institution submitted an account")
	}
	h.click("Directory", false)
	finish()
	label("Paper Synthetic Employer")
	h.click("Accounts", false)
	finish()
	startNewAccount(h, finish, "Credit card")
	d = p.creation.draft
	pickAccountReference(h, "Institution", "paPER", "Paper Synthetic Employer")
	setBankField(h, "Account name", &d.name, "Linked sample debt")
	setBankField(h, "Opening amount owed", &d.opening, "84.25")
	reviewBankAccount(h)
	h.click("Create liability account", false)
	finish()
	label("Could not create the liability account.")
	if records, err := service.ListAccounts(ctx, domain.AccountFilter{}); err != nil || len(records) != 1 {
		t.Fatalf("the failed account attempt created an account: %v, %v", records, err)
	}
	if d.institution.ID != institutionID || d.name.Text() != "Linked sample debt" || fault.institutionWrites.Load() != 2 {
		t.Fatal("account failure changed the institution or lost the account draft")
	}
	reviewBankAccount(h)
	h.click("Create liability account", false)
	finish()
	label("Liability account created.")
	label("Could not load account data.")
	h.click("Retry", false)
	finish()
	label("Linked sample debt")
	id := p.target.account
	if fault.writes.Load() != 2 || fault.institutionWrites.Load() != 2 {
		t.Fatal("read Retry repeated a write")
	}

	wailsDirectory := directory.NewService(reference)
	wantInstitution, err := wailsDirectory.CreateInstitution(ctx, "Paper Synthetic Employer", "employer", "vault")
	if err != nil {
		t.Fatal(err)
	}
	gotInstitution := wire.FromInstitution(d.institution)
	referenceInstitutions, err := wailsDirectory.ListInstitutions(ctx, false)
	index := slices.IndexFunc(referenceInstitutions, func(row wire.InstitutionDTO) bool { return row.ID == wantInstitution.ID })
	if err != nil || index < 0 {
		t.Fatalf("Wails institution could not be reread: %v", err)
	}
	wantInstitution = referenceInstitutions[index]
	wantID := wantInstitution.ID
	gotInstitution.ID, wantInstitution.ID = "", ""
	if !reflect.DeepEqual(gotInstitution, wantInstitution) {
		t.Fatalf("Gio and Wails institution differs: %+v, %+v", gotInstitution, wantInstitution)
	}
	group := fixtureDefaultAccountGroup(t, reference)
	want, err := account.NewService(reference).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Linked sample debt", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance",
		DefaultCurrency: "USD", InstitutionID: &wantID, GroupID: &group, InitialAmount: "84.25", IncludeInNetWorth: true,
		OwnerIDs: []string{d.owners[0].String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	actual := readBankRecord(t, service, id)
	normalize := func(record domain.AccountRecord) wire.AccountRecordDTO {
		dto := wire.FromAccountRecord(record)
		dto.Account.InstitutionID = nil
		dto.Account.ID, dto.LatestValue.ID, dto.LatestValue.AccountID = "", "", ""
		return dto
	}
	if actual.Account.InstitutionID == nil || *actual.Account.InstitutionID != institutionID ||
		!reflect.DeepEqual(normalize(actual), normalize(readBankRecord(t, reference, domain.AccountID(want.Account.ID)))) {
		t.Fatal("account or linked institution differs from Wails")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	if !reflect.DeepEqual(readBankRecord(t, reopened, id), actual) {
		t.Fatal("reopening lost the account or its saved institution")
	}
	institutions, err := reopened.ListInstitutions(ctx, true)
	if err != nil || len(institutions) != len(referenceInstitutions) ||
		!slices.ContainsFunc(institutions, func(item domain.Institution) bool { return item.ID == institutionID }) {
		t.Fatalf("reopened institution missing or duplicated: %v", err)
	}
}
