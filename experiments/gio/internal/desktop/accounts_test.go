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

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

var accountTestClock = time.Date(2026, 11, 21, 17, 0, 0, 0, time.UTC)

type accountFixture struct {
	pacific, unused                 domain.InstitutionID
	usd, sgd, mixed, zero, archived domain.AccountID
}

func seedAccountFixture(t *testing.T, root string) accountFixture {
	t.Helper()
	ctx := t.Context()
	data := filepath.Join(root, "data")
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(os.Mkdir(data, 0o700))
	db, err := sqlite.Open(filepath.Join(data, "nestworth.db"))
	check(err)
	defer db.Close()
	repo := sqlite.NewRepository(db)
	service := application.NewService(repo)
	service.SetClock(func() time.Time { return accountTestClock.Add(-4 * time.Hour) })
	check(service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Account Test Household", BaseCurrency: "AUD", MemberNames: []string{"Ana Example", "Bea Example"},
	}))
	core, err := service.Bootstrap(ctx)
	check(err)
	pacific, err := service.CreateInstitution(ctx, "Pacific Sample Brokerage", domain.InstitutionBrokerage)
	check(err)
	unused, err := service.CreateInstitution(ctx, "Empty Sample Institution", domain.InstitutionBank)
	check(err)
	legacy, err := service.CreateInstitution(ctx, "Former Sample Bank", domain.InstitutionBank)
	check(err)
	group, err := service.CreateGroup(ctx, "Long-term sample")
	check(err)
	create := func(name string, kind domain.AccountType, mode domain.TrackingMode, currency, amount string, bank domain.InstitutionID) domain.AccountID {
		t.Helper()
		record, err := service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind.String(), BalanceSheetRole: "asset", TrackingMode: string(mode), DefaultCurrency: currency,
			InitialAmount: amount, InstitutionID: bank.String(), GroupID: group.ID.String(), IncludeInNetWorth: true,
			OwnerIDs: []domain.MemberID{core.Members[0].ID, core.Members[1].ID},
		})
		check(err)
		return record.Account.ID
	}
	ids := accountFixture{pacific: pacific.ID, unused: unused.ID}
	ids.usd = create("USD savings", domain.TypeBankAccount, domain.TrackingBalance, "USD", "125.50", pacific.ID)
	ids.sgd = create("SGD savings", domain.TypeBankAccount, domain.TrackingBalance, "SGD", "75", pacific.ID)
	ids.zero = create("Zero balance", domain.TypeBankAccount, domain.TrackingBalance, "USD", "0", "")
	ids.mixed = create("Mixed brokerage", domain.TypeBrokerage, domain.TrackingHoldings, "USD", "", pacific.ID)
	ids.archived = create("Legacy savings", domain.TypeBankAccount, domain.TrackingBalance, "AUD", "123", legacy.ID)
	unpriced := create("Unpriced portfolio", domain.TypeBrokerage, domain.TrackingHoldings, "USD", "", "")
	_, err = service.AppendAccountCashValue(ctx, ids.mixed, "15", "USD", "")
	check(err)
	_, err = service.AppendAccountCashValue(ctx, ids.mixed, "30", "SGD", "")
	check(err)
	inst, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic equity", Symbol: "DEMO", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	check(err)
	_, err = service.CreateHolding(ctx, application.HoldingInput{AccountID: ids.mixed.String(), InstrumentID: inst.ID.String(), Quantity: "2"})
	check(err)
	_, err = service.CreateHolding(ctx, application.HoldingInput{AccountID: unpriced.String(), InstrumentID: inst.ID.String(), Quantity: "1"})
	check(err)
	rate, err := domain.ParseFxRate("1.5")
	check(err)
	at := accountTestClock.Add(-4 * time.Hour)
	check(repo.AppendFXQuote(ctx, domain.FXQuote{
		ID: domain.NewFXQuoteID(), HouseholdID: core.Household.ID, BaseCurrency: "USD", QuoteCurrency: "AUD", Rate: rate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: application.FrankfurterProviderKey, QuotedAt: at, CreatedAt: at,
	}))
	check(service.ArchiveAccount(ctx, ids.archived, true))
	check(service.ArchiveInstitution(ctx, legacy.ID, true))
	check(service.ArchiveGroup(ctx, group.ID, true))
	check(service.ArchiveMember(ctx, core.Members[1].ID, true))
	saved := settings.Default()
	saved.Currency, saved.Language, saved.Timezone = "AUD", settings.LanguageEnglish, "Asia/Singapore"
	saved.QuoteCacheTTL = settings.QuoteCacheTTL1h
	check(settings.NewStore(filepath.Join(data, "settings.json")).Save(saved))
	check(db.CheckpointWAL(ctx))
	return ids
}

func openAccountTestFixture(t *testing.T) (accountFixture, *application.Service, *application.Service) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chmod := os.Chmod(root, 0o700)
	if chmod != nil {
		t.Fatal(chmod)
	}
	ids := seedAccountFixture(t, root)
	seed, err := os.ReadFile(filepath.Join(root, "data", "nestworth.db"))
	if err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(t.TempDir(), "reference.db")
	if err := os.WriteFile(reference, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	rdb, err := sqlite.OpenReadOnlyForVerifyContext(t.Context(), reference)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	wails := application.NewService(sqlite.NewRepository(rdb), application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	wails.SetClock(func() time.Time { return accountTestClock })
	wails.SetQuoteCacheTTL(time.Hour)
	if err := wails.SetFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_DATABASE_PATH", filepath.Join(root, "data", "nestworth.db"))
	t.Setenv("NESTWORTH_SETTINGS_PATH", filepath.Join(root, "data", "settings.json"))
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	session, err := pilot.OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	gio := application.NewService(sqlite.NewRepository(session.DB))
	gio.SetClock(func() time.Time { return accountTestClock })
	prefs, err := preferences.Open(paths.SettingsPath(), session.DB, gio)
	if err != nil {
		t.Fatal(err)
	}
	if err := prefs.ConfigureValuation(); err != nil {
		t.Fatal(err)
	}
	return ids, gio, wails
}

func TestAccountsReadsMatchWailsAndNavigateFromInstitution(t *testing.T) {
	ids, service, reference := openAccountTestFixture(t)
	ctx := t.Context()
	wails := account.NewService(reference)
	for _, archived := range []bool{false, true} {
		institution := ids.pacific.String()
		for _, ref := range []*string{nil, &institution} {
			request := account.AccountFilterRequest{IncludeArchived: archived, InstitutionID: ref}
			wantAccounts, err := wails.ListAccounts(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			wantValues, err := wails.AccountValuations(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			target := navigationTarget{route: accountsRoute}
			if ref != nil {
				target.institution = ids.pacific
			}
			got := readAccounts(ctx, service, target, archived)
			if got.err != nil || len(wantAccounts) != len(got.rows) {
				t.Fatalf("account scope failed: %+v, %v", request, got.err)
			}
			for i, row := range got.rows {
				j := slices.IndexFunc(wantValues, func(v wire.AccountValuationDTO) bool { return v.Account.ID == row.record.Account.ID.String() })
				matchesValue := func(value *domain.AccountValuation) bool {
					if j < 0 {
						// Wails returns archived records but omits their current valuations.
						return value == nil && row.record.Account.ArchivedAt != nil
					}
					return value != nil && reflect.DeepEqual(wantValues[j], wire.FromAccountValuation(*value))
				}
				if !reflect.DeepEqual(wantAccounts[i], wire.FromAccountRecord(row.record)) || !matchesValue(row.value) {
					t.Fatalf("record or valuation changed for %s in %+v", row.record.Account.Name, request)
				}
				if archived && ref == nil {
					detail := readAccounts(ctx, service, navigationTarget{route: accountsRoute, account: row.record.Account.ID}, false)
					if detail.err != nil || detail.unavailable || len(detail.rows) != 1 ||
						!reflect.DeepEqual(wantAccounts[i], wire.FromAccountRecord(detail.rows[0].record)) || !matchesValue(detail.rows[0].value) {
						t.Fatalf("exact-ID detail changed the Wails result for %s: %v", row.record.Account.Name, detail.err)
					}
				}
			}
		}
	}

	p := newAccountsPage(ctx, service, func() time.Time { return accountTestClock }, func() {})
	directory := newDirectoryPage(newInstitutionsPage(ctx, service, func() {}))
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory, accounts: p})
	h.size = image.Pt(1120, 1000)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.prefs.saved.Timezone = "Asia/Singapore"
	h.shell.prefs.savedLocation = time.FixedZone("Asia/Singapore", 8*60*60)
	finish := func() { h.frame(); p.workers.Wait(); directory.wait(); h.frame() }
	label := func(s string) { assertPageLabel(t, &h.router, s) }
	missing := func(s string) {
		if findPageLabel(&h.router, s) != nil {
			t.Fatalf("unexpected %q", s)
		}
	}
	description := func(name, want string) {
		t.Helper()
		node := findPageLabel(&h.router, name)
		if node == nil || node.Desc.Description != want {
			t.Fatalf("account %q is missing its balance or valuation description: %+v", name, node)
		}
	}
	h.frame()
	h.click("Directory", false)
	finish()
	searchDirectory(h, "Search institutions", "Pacific")
	h.click("View", false)
	finish()
	h.click("View accounts", false)
	finish()
	if p.target.institution != ids.pacific {
		t.Fatal("the institution link didn't open its account scope")
	}
	label("Pacific Sample Brokerage")
	label("Complete valuation")
	description("USD savings", "US$125.50 · A$188.25 · Complete valuation · Cached quotes are out of date.")
	description("SGD savings", "S$75.00 · AUD value unavailable · Incomplete valuation · Missing exchange rate")
	if len(p.rows) == 0 || p.rows[0].subtotal != nil {
		t.Fatal("an institution with missing valuations has a complete-looking subtotal")
	}
	missing("Zero balance")
	h.click("USD savings", false)
	finish()
	if p.target.account != ids.usd {
		t.Fatal("the account row didn't open its exact ID")
	}
	label("US$125.50")
	label("A$188.25")
	missing("USD · US$125.50")
	label("Cached quotes are out of date.")
	label("Group · Long-term sample · Archived")
	label("Owners · Ana Example · 50.00%  /  Bea Example · Archived · 50.00%")
	label("Read 2026-11-22 01:00 (Asia/Singapore)")
	h.click("Back to accounts", false)
	finish()
	h.focused(p.buttons[ids.usd])
	h.click("SGD savings", false)
	finish()
	label("S$75.00")
	label("AUD value unavailable")
	label("Missing exchange rate · AUD/SGD")
	h.click("Back", false)
	finish()
	h.click("Mixed brokerage", false)
	finish()
	label("US$15.00 · Known subtotal")
	label("Missing price · DEMO")
	label("Missing exchange rate · AUD/SGD")
	label("DEMO · Quantity 2 · Unavailable")
	h.click("Back", false)
	finish()
	h.click("Back", false)
	finish()
	h.focused(&directory.object.accounts)
	h.click("Accounts", false)
	finish()
	missing("Legacy savings")
	label("No institution")
	h.click("Zero balance", false)
	finish()
	label("US$0.00")
	label("A$0.00")
	h.click("Back", false)
	finish()
	h.click("Unpriced portfolio", false)
	finish()
	label("USD value unavailable")
	label("AUD value unavailable")
	missing("US$0.00")
	h.click("Back", false)
	finish()
	h.click("Show archived", false)
	finish()
	description("Legacy savings", "A$123.00")
	h.click("Legacy savings", false)
	finish()
	label("Bank account · Archived")
	label("Institution · Former Sample Bank · Archived")
	missing("Restore")
	h.click("Reload", false)
	finish()
	label("A$123.00")
	h.click("Back", false)
	finish()
	label("Hide archived")
	h.focused(p.buttons[ids.archived])
	selectRecent(h, "Accounts · Pacific Sample Brokerage")
	finish()
	if p.target.institution != ids.pacific || p.archived {
		t.Fatal("Recent mixed the original institution and archive scopes")
	}
	h.click("Show archived", false)
	finish()
	h.click("All institutions", false)
	finish()
	if p.target.institution != "" || !p.archived {
		t.Fatal("clearing the institution also cleared the archive filter")
	}
	label("Legacy savings")
	h.click("Back", false)
	finish()
	if p.target.institution != ids.pacific || !p.archived {
		t.Fatal("Back did not restore the archived institution scope")
	}
	selectRecent(h, "USD savings")
	finish()
	if p.target.account != ids.usd {
		t.Fatal("Recent did not reopen the account's exact ID")
	}
	label("A$188.25")
}

func TestAccountInstitutionSubtotalRequiresCompleteBaseValues(t *testing.T) {
	institution := domain.InstitutionID("sample")
	account := func(name, amount string, role domain.BalanceSheetRole) accountRow {
		return accountRow{
			record: domain.AccountRecord{Account: domain.Account{Name: name, InstitutionID: &institution, BalanceSheetRole: role},
				InstitutionName: "Sample institution"},
			value: &domain.AccountValuation{Complete: true, BaseValue: &domain.MoneyView{Currency: "USD", Amount: amount}},
		}
	}
	asset := account("Checking", "100.12345", domain.RoleAsset)
	debt := account("Card", "30.01234", domain.RoleLiability)
	result := accountsResult{rows: []accountRow{asset, debt}}
	subtotal := func() *domain.MoneyView { return result.listRows(i18n.English)[0].subtotal }
	if got := subtotal(); got == nil || *got != (domain.MoneyView{Currency: "USD", Amount: "70.11111"}) {
		t.Fatalf("wrong complete institution net: %+v", got)
	}
	for _, value := range []*domain.AccountValuation{
		nil,
		{BaseValue: &domain.MoneyView{Currency: "USD", Amount: "30.01234"}},
		{Complete: true, BaseValue: &domain.MoneyView{Currency: "EUR", Amount: "30.01234"}},
	} {
		result.rows[1].value = value
		if got := subtotal(); got != nil {
			t.Fatalf("incomplete or mixed currency institution published %+v", got)
		}
	}
	result.rows = result.rows[:1]
	if got := subtotal(); got != nil {
		t.Fatalf("single account repeated its balance in the institution heading: %+v", got)
	}
}

type accountReadFaults struct {
	*application.Service
	held    atomic.Bool
	started chan struct{}
	release chan struct{}
	fail    atomic.Bool
	calls   atomic.Int32
}

func (s *accountReadFaults) WithRead(ctx context.Context, read func(context.Context) error) error {
	first := !s.held.Swap(true)
	err := s.Service.WithRead(ctx, read)
	if first {
		// Hold the completed reply after its coherent snapshot releases the gate.
		close(s.started)
		<-s.release
	}
	return err
}

func (s *accountReadFaults) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	s.calls.Add(1)
	if s.fail.Load() {
		return nil, errors.New("synthetic account read failure")
	}
	return s.Service.ListAccounts(ctx, filter)
}

func TestAccountsDiscardLateReadsAndRetryWithoutLosingTheSelectedObject(t *testing.T) {
	ids, service, _ := openAccountTestFixture(t)
	ctx := t.Context()
	faults := &accountReadFaults{Service: service, started: make(chan struct{}), release: make(chan struct{})}
	p := newAccountsPage(ctx, faults, func() time.Time { return accountTestClock }, func() {})
	release := sync.OnceFunc(func() { close(faults.release) })
	t.Cleanup(func() { release(); p.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	finish := func() { h.frame(); p.workers.Wait(); h.frame() }
	label := func(s string) { assertPageLabel(t, &h.router, s) }
	open := func(target navigationTarget) {
		h.shell.pendingNavigation = &navigationRequest{target: target}
		h.frame()
	}
	open(navigationTarget{route: accountsRoute, account: ids.usd})
	select {
	case <-faults.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first detail read did not start")
	}
	label("Loading accounts…")
	open(navigationTarget{route: accountsRoute, account: ids.sgd})
	deadline := time.Now().Add(3 * time.Second)
	for findPageLabel(&h.router, "SGD savings") == nil {
		if time.Now().After(deadline) {
			t.Fatal("new detail did not finish while the abandoned request was still blocked")
		}
		time.Sleep(time.Millisecond)
		h.frame()
	}
	release()
	finish()
	label("SGD savings")
	if findPageLabel(&h.router, "USD savings") != nil {
		t.Fatal("late result replaced the selected account")
	}
	faults.fail.Store(true)
	h.click("Reload", false)
	finish()
	label("Update failed. Showing the previous result.")
	label("S$75.00")
	faults.fail.Store(false)
	h.click("Retry", false)
	finish()
	label("AUD value unavailable")
	open(navigationTarget{route: accountsRoute, account: domain.NewAccountID()})
	finish()
	label("Account destination unavailable.")
	if findPageLabel(&h.router, "SGD savings") != nil {
		t.Fatal("missing account fell through to another record")
	}
	reads := faults.calls.Load()
	open(navigationTarget{route: accountsRoute, account: "malformed"})
	label("Account destination unavailable.")
	if faults.calls.Load() != reads || findPageLabel(&h.router, "Reload") != nil {
		t.Fatal("malformed account exposed an unusable action")
	}
	open(navigationTarget{route: accountsRoute, institution: domain.NewInstitutionID()})
	finish()
	label("Account destination unavailable.")
	open(navigationTarget{route: accountsRoute, institution: ids.unused})
	finish()
	label("Empty Sample Institution")
	label("No accounts to show.")
	if findPageLabel(&h.router, "SGD savings") != nil {
		t.Fatal("empty institution fell back to the account browser")
	}
	faults.fail.Store(true)
	h.click("All institutions", false)
	finish()
	label("Could not load account data.")
	faults.fail.Store(false)
	h.click("Retry", false)
	finish()
	label("SGD savings")
}
