package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type onboardingTestService struct {
	*application.Service
	reads, writes atomic.Int32
	failNextRead  atomic.Bool
	started       chan struct{}
	release       chan struct{}
}

func (s *onboardingTestService) Bootstrap(ctx context.Context) (application.Bootstrap, error) {
	if s.reads.Add(1) == 1 || s.failNextRead.Swap(false) {
		return application.Bootstrap{}, errors.New("test bootstrap read failure")
	}
	return s.Service.Bootstrap(ctx)
}

func (s *onboardingTestService) CompleteOnboarding(ctx context.Context, input application.OnboardingInput) error {
	if s.writes.Add(1) == 1 {
		return errors.New("test write failure before commit")
	}
	close(s.started)
	<-s.release
	err := s.Service.CompleteOnboarding(ctx, input)
	s.failNextRead.Store(true)
	return err
}

var onboardingTestTime = time.Date(2026, time.November, 21, 17, 0, 0, 0, time.UTC)

func prepareOnboardingProfile(t *testing.T) pilot.Paths {
	t.Helper()
	t.Setenv("NESTWORTH_GIO_RUN_DIR", createFixtureRoot(t))
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if err := pilot.PrepareOnboarding(t.Context(), paths); err != nil {
		t.Fatal(err)
	}
	return paths
}

func reopenOnboardingProfile(t *testing.T, paths pilot.Paths) (*application.Service, *pilot.Session) {
	t.Helper()
	session, err := pilot.OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	service := application.NewService(sqlite.NewRepository(session.DB))
	service.SetClock(func() time.Time { return onboardingTestTime })
	return service, session
}

func onboardedPreferenceState(t *testing.T, paths pilot.Paths, service *application.Service, session *pilot.Session) *preferenceState {
	t.Helper()
	store, err := preferences.Open(paths.SettingsPath(), session.DB, service)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	next := value
	next.Language, next.Timezone = settings.LanguageEnglish, "Asia/Singapore"
	if _, err := store.Save(t.Context(), value, next); err != nil {
		t.Fatal(err)
	}
	p := newPreferenceState(t.Context(), store, func() {})
	p.setLanguage = service.SetUILanguage
	p.reload()
	t.Cleanup(p.wait)
	return p
}

func TestOnboardingWindowMatchesWailsAndRetriesReadsAfterCommit(t *testing.T) {
	ctx := t.Context()
	paths := prepareOnboardingProfile(t)
	service, session := reopenOnboardingProfile(t, paths)
	reference, _ := reopenOnboardingProfile(t, prepareOnboardingProfile(t))
	prefs := onboardedPreferenceState(t, paths, service, session)
	clock := func() time.Time { return onboardingTestTime }
	fault := &onboardingTestService{Service: service, started: make(chan struct{}), release: make(chan struct{})}
	page := newOnboardingPage(ctx, fault, clock, func() {})
	release := sync.OnceFunc(func() { close(fault.release) })
	accounts := newAccountsPage(ctx, service, clock, func() {})
	t.Cleanup(func() { release(); page.workers.Wait(); accounts.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{prefs: prefs, accounts: accounts})
	h.size = image.Pt(700, 460)
	h.shell.onboarding = page
	wait := func() {
		h.frame()
		page.workers.Wait()
		prefs.wait()
		h.frame()
	}
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	page.read()
	wait()
	label("Could not load household data.")
	if findPageLabel(&h.router, "Create household") != nil {
		t.Fatal("a read error offered the setup form")
	}
	h.click("Retry", false)
	wait()
	if page.currency != "CNY" || page.historical {
		t.Fatal("unexpected default currency or history choice")
	}
	label("Set up your household")
	h.click("Create household", false)
	h.frame()
	label("Enter a household name.")
	setBankField(h, "Household name", &page.name, "  Synthetic Mei household  ")
	h.click("Create household", false)
	h.frame()
	label("Enter at least one member name.")
	h.click("Member name", true)
	h.router.Queue(key.EditEvent{Text: "  Mei Sample  "})
	h.frame()
	h.click("Add member", false)
	h.frame()
	h.focused(&page.members[1].name.Editor)
	h.router.Queue(key.EditEvent{Text: "Jun Sample"})
	h.frame()
	h.click("CNY", false)
	for range 5 {
		h.press(key.NameDownArrow)
	}
	h.press(key.NameReturn)
	if page.currency != "SGD" {
		t.Fatalf("selected currency = %s", page.currency)
	}
	h.click("Start ledger history", false)
	h.frame()
	label("History time zone · Asia/Singapore")
	h.click("2026-11-22", false)
	h.press(key.NameLeftArrow)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	if page.day != "2026-11-20" {
		t.Fatalf("selected civil date = %s", page.day)
	}
	h.click("Create household", false)
	wait()
	wait()
	label("Could not create the household. Review your details and try again.")
	if page.currency != "SGD" || page.day != "2026-11-20" || len(page.members) != 2 {
		t.Fatal("a failed write lost the submitted draft")
	}
	if core, err := service.Bootstrap(ctx); err != nil || core.Household != nil {
		t.Fatalf("failed write left household state: %+v, %v", core, err)
	}

	button := semanticBounds(t, &h.router, "Create household", false)
	position := button.Min.Add(button.Max).Div(2)
	h.clickAt(position)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not write")
	}
	h.clickAt(position)
	if fault.writes.Load() != 2 {
		t.Fatal("a duplicate click reached the pending writer")
	}
	release()
	wait()
	wait()
	label("Household created.")
	label("Could not load household data.")
	if findPageLabel(&h.router, "Create household") != nil {
		t.Fatal("a failed read after commit exposed another write")
	}
	h.click("Retry", false)
	wait()
	accounts.workers.Wait()
	prefs.wait()
	h.frame()
	label("No accounts to show.")
	if h.shell.onboarding != nil || h.shell.activeRoute() != accountsRoute || fault.writes.Load() != 2 {
		t.Fatal("read-only retry failed to enter the empty Accounts page")
	}
	if prefs.saved.HouseholdCurrency != "SGD" || prefs.saved.HistoryTimezone != "Asia/Singapore" {
		t.Fatalf("settings context did not refresh: %+v", prefs.saved)
	}

	wails := household.NewService(reference)
	if err := wails.CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "Synthetic Mei household", BaseCurrency: "SGD", MemberNames: []string{"Mei Sample", "Jun Sample"},
		Timezone: "Asia/Singapore", HistoryStartDate: "2026-11-20",
	}); err != nil {
		t.Fatal(err)
	}
	want, err := wails.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertEquivalentHousehold(t, got, want)
	wantOrigin, err := reference.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gotOrigin, err := service.HistoryOrigin(ctx)
	if err != nil || gotOrigin == nil || wantOrigin == nil || gotOrigin.HouseholdID != got.Household.ID ||
		gotOrigin.Timezone != wantOrigin.Timezone || !gotOrigin.CreatedAt.Equal(wantOrigin.CreatedAt) ||
		!gotOrigin.StartedAt.Equal(wantOrigin.StartedAt) || !gotOrigin.StartedAt.Equal(time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("history origins differ: Gio %+v, Wails %+v, %v", gotOrigin, wantOrigin, err)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	after, err := reopened.Bootstrap(ctx)
	if err != nil || !reflect.DeepEqual(after, got) {
		t.Fatalf("household changed on reopen: %+v, %v", after, err)
	}
	afterOrigin, err := reopened.HistoryOrigin(ctx)
	if err != nil || !reflect.DeepEqual(afterOrigin, gotOrigin) {
		t.Fatalf("history origin changed on reopen: %+v, %v", afterOrigin, err)
	}
	directory := newDirectoryPage(newMembersList(ctx, reopened, func() {}))
	reopenedShell := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	reopenedShell.shell.prefs.saved.Language = settings.LanguageEnglish
	reopenedShell.shell.onboarding = newOnboardingPage(ctx, reopened, clock, func() {})
	reopenedShell.shell.onboarding.read()
	reopenedShell.shell.onboarding.workers.Wait()
	reopenedShell.frame()
	reopenedShell.click("Directory", false)
	directory.wait()
	reopenedShell.frame()
	assertPageLabel(t, &reopenedShell.router, want.Household.Name)
	assertPageLabel(t, &reopenedShell.router, "Mei Sample")
}

func assertEquivalentHousehold(t *testing.T, got application.Bootstrap, want household.BootstrapResult) {
	t.Helper()
	if got.Household == nil || want.Household == nil {
		t.Fatalf("missing household: %+v / %+v", got, want)
	}
	home := wire.FromHousehold(*got.Household)
	assertMatched := func(gotID, expectedID string, gotHousehold domain.HouseholdID) {
		t.Helper()
		if gotID == "" || expectedID == "" || gotHousehold.String() != home.ID {
			t.Fatal("invalid household reference")
		}
	}
	members, institutions, groups := wire.FromMembers(got.Members), wire.FromInstitutions(got.Institutions), wire.FromGroups(got.Groups)
	if len(members) != 2 || len(want.Members) != len(members) || len(institutions) != 1 || len(want.Institutions) != len(institutions) ||
		len(groups) != 1 || len(want.Groups) != len(groups) {
		t.Fatalf("unexpected directory: %+v, %+v", got, want)
	}
	for i := range members {
		g, w := &members[i], &want.Members[i]
		assertMatched(g.ID, w.ID, got.Members[i].HouseholdID)
		g.ID, w.ID = "", ""
	}
	inst, wantInst := &institutions[0], &want.Institutions[0]
	assertMatched(inst.ID, wantInst.ID, got.Institutions[0].HouseholdID)
	inst.ID, wantInst.ID = "", ""
	group, wantGroup := &groups[0], &want.Groups[0]
	assertMatched(group.ID, wantGroup.ID, got.Groups[0].HouseholdID)
	group.ID, wantGroup.ID = "", ""
	home.ID, want.Household.ID = "", ""
	if home != *want.Household || !reflect.DeepEqual(members, want.Members) ||
		!reflect.DeepEqual(institutions, want.Institutions) || !reflect.DeepEqual(groups, want.Groups) {
		t.Fatalf("household or directory differs: %+v %+v %+v %+v, want %+v", home, members, institutions, groups, want)
	}
}

type postCommitOnboardingError struct {
	*application.Service
	writes atomic.Int32
}

func (s *postCommitOnboardingError) CompleteOnboarding(ctx context.Context, input application.OnboardingInput) error {
	s.writes.Add(1)
	if err := s.Service.CompleteOnboarding(ctx, input); err != nil {
		return err
	}
	return errors.New("test error reported after the household transaction")
}

func TestOnboardingDefaultLeavesHistoryOffAndACommitErrorCannotResubmit(t *testing.T) {
	paths := prepareOnboardingProfile(t)
	service, session := reopenOnboardingProfile(t, paths)
	prefs := onboardedPreferenceState(t, paths, service, session)
	api := &postCommitOnboardingError{Service: service}
	clock := func() time.Time { return onboardingTestTime }
	page := newOnboardingPage(t.Context(), api, clock, func() {})
	accounts := newAccountsPage(t.Context(), service, clock, func() {})
	t.Cleanup(func() { page.workers.Wait(); accounts.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{prefs: prefs, accounts: accounts})
	h.shell.onboarding = page
	page.read()
	page.workers.Wait()
	prefs.wait()
	h.frame()

	setBankField(h, "Household name", &page.name, "Synthetic no-history")
	h.click("Member name", true)
	h.router.Queue(key.EditEvent{Text: "Sample member"})
	h.frame()
	h.click("Create household", false)
	page.workers.Wait()
	h.frame()
	page.workers.Wait()
	h.frame()
	accounts.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No accounts to show.")
	core, err := service.Bootstrap(t.Context())
	if err != nil || core.Household == nil || core.Household.BaseCurrency != domain.CurrencyCode("CNY") || api.writes.Load() != 1 {
		t.Fatalf("unresolved committed write: %+v, %v", core, err)
	}
	if origin, err := service.HistoryOrigin(t.Context()); err != nil || origin != nil {
		t.Fatalf("the default setup wrote an origin: %+v, %v", origin, err)
	}
}
