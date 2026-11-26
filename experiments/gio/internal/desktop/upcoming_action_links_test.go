package desktop

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestUpcomingActionLinksOpenFreshFundsTargetsAndRestoreOrigin(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, ids := openFundsFixture(t, true)
	bank := domain.AccountValueSourceRef(ids.bank)
	if _, err := service.SaveLiquidityReservation(ctx, application.SaveReservationInput{
		Source: bank, Label: "Additional reserve", Amount: "150.01",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, ids.foreign, "5", "AUD", ""); err != nil {
		t.Fatal(err)
	}
	cash := domain.AccountCashSourceRef(ids.foreign, "EUR")
	policy := simpleFundsPolicy(policyDate, application.ProductPolicyInput{})
	policy.UnlockOn = productOptional("2026-11-27")
	saved, err := service.SaveLiquidityPolicy(ctx, application.SavePolicyInput{Source: cash, Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	// Checkpoint and close the synthetic seed before cloning it. Both service
	// instances then use their own ledger, while keeping identical IDs and facts.
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(storage.DB.Path)
	if err != nil {
		t.Fatal(err)
	}
	referencePath := filepath.Join(t.TempDir(), "reference.db")
	if err := os.WriteFile(referencePath, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	referenceDB, err := sqlite.Open(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = referenceDB.Close() })
	clock := func() time.Time { return accountTestClock }
	reference := application.NewService(sqlite.NewRepository(referenceDB))
	reference.SetClock(clock)
	service, _ = reopenOnboardingProfile(t, paths)
	service.SetClock(clock)
	p := newOverviewPage(ctx, service, clock, func() {})
	tasks := newTasksPage(ctx, service, func() {})
	funds := newAvailableFundsPage(ctx, service, func() {})
	funds.reservationLedger = service
	h := newShellHarnessWithPages(t, ledgerPages{overview: p, tasks: tasks, funds: funds})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1050, 920)
	for _, drain := range []func(){p.workers.Wait, tasks.workers.Wait, funds.workers.Wait} {
		t.Cleanup(drain)
	}
	settle := func() {
		for range 5 {
			h.frame()
			p.workers.Wait()
			tasks.workers.Wait()
			funds.workers.Wait()
		}
		h.frame()
	}
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		settle()
	}

	settle()
	p.list.ScrollTo(p.trendIndex() - 1)
	settle()
	if p.upcoming.current == nil {
		t.Fatal("Overview has no upcoming projection")
	}
	target := func(data *domain.UpcomingActions, kind string, ref domain.LiquiditySourceRef) domain.UpcomingAction {
		t.Helper()
		for _, row := range data.Items {
			if row.Kind == kind && row.SourceKey == ref.Key() {
				return row
			}
		}
		t.Fatalf("missing %s for %s in %+v", kind, ref.Key(), data.Items)
		return domain.UpcomingAction{}
	}
	scheduled := target(p.upcoming.current, "source_access", cash)
	policyKey := upcomingActionLinkKey{row: scheduled.ID, funds: upcomingFundsFocus(scheduled.PrimaryTarget)}
	// Advance just the persisted revision while the Overview renders the old
	// projection. The deep link must open the latest policy for this cash leg.
	policy.UnlockOn = productOptional("2026-11-28")
	fresh, err := service.SaveLiquidityPolicy(ctx, application.SavePolicyInput{Source: cash, Policy: policy, ExpectedRevision: saved.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reference.SaveLiquidityPolicy(ctx, application.SavePolicyInput{Source: cash, Policy: policy, ExpectedRevision: saved.Revision}); err != nil {
		t.Fatal(err)
	}
	click("View availability rules · " + scheduled.DisplayName)
	if h.shell.activeRoute() != fundsRoute || funds.policy == nil || funds.policy.source.Key() != cash.Key() ||
		funds.policy.revision != fresh.Revision || productString(funds.policy.fields.value.UnlockOn) != "2026-11-28" {
		t.Fatalf("policy destination did not use the fresh account and currency: %+v", funds.policy)
	}
	click("Back")
	if h.shell.activeRoute() != overviewRoute {
		t.Fatal("back did not return to the Overview link")
	}
	h.focused(p.upcoming.links.buttons[policyKey])
	click("Forward")
	if h.shell.activeRoute() != fundsRoute || funds.policy != nil {
		t.Fatal("forward reapplied the consumed editor request")
	}
	click("Back")

	unresolvedAt := slices.IndexFunc(p.upcoming.current.Items, func(row domain.UpcomingAction) bool { return row.Kind == "reservation_unresolved" })
	if unresolvedAt < 0 {
		t.Fatal("archived fixture reservation did not project")
	}
	unresolved := p.upcoming.current.Items[unresolvedAt]
	if unresolved.PrimaryTarget.ReservationID == nil || unresolved.PrimaryTarget.SourceRef != nil {
		t.Fatalf("unresolved reservation has the wrong target: %+v", unresolved.PrimaryTarget)
	}
	reserveKey := upcomingActionLinkKey{row: unresolved.ID, funds: upcomingFundsFocus(unresolved.PrimaryTarget)}
	click("Check reservations · Old reserve")
	r := funds.reservation
	id := *unresolved.PrimaryTarget.ReservationID
	if h.shell.activeRoute() != fundsRoute || r == nil || r.source.Kind != "" || r.actions[id] == nil {
		t.Fatalf("unresolved item did not focus the exact reservation in the global manager: %+v", r)
	}
	h.focused(&r.actions[id].edit)
	click("Back")
	h.focused(p.upcoming.links.buttons[reserveKey])

	click("Tasks & checks")
	want, err := liquidityapi.NewService(reference).UpcomingActions(ctx)
	if err != nil || tasks.current == nil || !reflect.DeepEqual(liquidityapi.FromUpcomingActions(*tasks.current), want) {
		t.Fatalf("Tasks dropped shared projection evidence: %+v / %v", tasks.current, err)
	}
	click("Reservations")
	short := target(tasks.current, "reservation_shortfall", bank)
	if short.Amount == nil || short.Amount.CanonicalAmount() != "64.7029" || short.Amount.Currency() != "USD" {
		t.Fatalf("unexpected exact shortfall: %+v", short.Amount)
	}
	shortKey := upcomingActionLinkKey{row: short.ID, funds: upcomingFundsFocus(short.PrimaryTarget)}
	scrollProductControl(t, h, "Check reservations · Harbor Savings")
	click("Check reservations · Harbor Savings")
	r = funds.reservation
	if r == nil || r.source.Key() != bank.Key() || r.current == nil || len(r.current.records) != 2 {
		t.Fatalf("shortfall did not open the two reservations for the exact account: %+v", r)
	}
	click("Back")
	if tasks.upcomingFilter != taskUpcomingReservations {
		t.Fatal("back discarded the Upcoming reservations filter")
	}
	h.focused(tasks.upcomingLinks.buttons[shortKey])
	// Keep the visible shortfall row stale while the source goes away.
	if err := service.ArchiveAccount(ctx, ids.bank, true); err != nil {
		t.Fatal(err)
	}
	click("Check reservations · Harbor Savings")
	if r := funds.reservation; r == nil || r.source.Kind != "" || r.current == nil ||
		r.notice != i18n.TasksReserveSourceChanged || len(r.current.records) != 3 {
		t.Fatalf("a vanished source did not fall back to the global reservation manager: %+v", r)
	}
}

func TestUpcomingFundsNavigationDoesNotOpenMissingPolicy(t *testing.T) {
	ctx := t.Context()
	service, _, _, _ := openFundsFixture(t, true)
	funds := newAvailableFundsPage(ctx, service, func() {})
	t.Cleanup(funds.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	ref := domain.AccountValueSourceRef(domain.NewAccountID())
	focus := upcomingFundsFocus(domain.UpcomingTarget{Kind: "source_policy", SourceRef: &ref})
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: fundsRoute, fundsAction: focus}}
	for range 3 {
		h.frame()
		funds.workers.Wait()
	}
	h.frame()
	if funds.policy != nil || funds.actionNotice != i18n.TasksTargetMissing || funds.actionFocus != (fundsActionFocus{}) {
		t.Fatalf("a stale source opened a policy editor: %+v", funds.policy)
	}
	assertPageLabel(t, &h.router, "The requested item is no longer available.")
}
