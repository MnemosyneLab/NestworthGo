package desktop

import (
	"image"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestValuedAndHoldingsMaintenanceMatchesWailsWithoutRewritingFacts(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, prepared := openBankFixture(t, root)
	seed.SetClock(func() time.Time { return accountTestClock.Add(-time.Hour) })
	core, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := core.Members[0].ID
	partner, err := seed.CreateMember(ctx, "Partner Sample")
	if err != nil {
		t.Fatal(err)
	}
	group, err := seed.CreateGroup(ctx, "Former shell group")
	if err != nil {
		t.Fatal(err)
	}
	bank := core.Institutions[0].ID
	create := func(name string, kind domain.AccountType, mode domain.TrackingMode, amount string) domain.AccountID {
		t.Helper()
		record, err := seed.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind.String(), BalanceSheetRole: "asset", TrackingMode: string(mode),
			DefaultCurrency: "USD", InitialAmount: amount, OwnerIDs: []domain.MemberID{first}, GroupID: group.ID.String(), InstitutionID: bank.String(),
			IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	property := create("Synthetic home", domain.TypeProperty, domain.TrackingManualValue, "1234.56")
	broker := create("Synthetic brokerage", domain.TypeBrokerage, domain.TrackingHoldings, "")
	cash := create("Synthetic currency pocket", domain.TypeCashOnHand, domain.TrackingHoldings, "")
	empty := create("Synthetic empty holdings", domain.TypeBrokerage, domain.TrackingHoldings, "")
	for _, id := range []domain.AccountID{broker, cash} {
		if _, err := seed.AppendAccountCashValue(ctx, id, "15.25", "USD", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.AppendAccountCashValue(ctx, id, "18.75", "SGD", ""); err != nil {
			t.Fatal(err)
		}
	}
	instrument, err := seed.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic fund", Symbol: "DEMO", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateHolding(ctx, application.HoldingInput{AccountID: broker.String(), InstrumentID: instrument.ID.String(), Quantity: "2.5", UnitCost: "8.25"}); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.AppendManualInstrumentQuote(ctx, instrument.ID, "12.5", "", false); err != nil {
		t.Fatal(err)
	}
	if err := seed.ArchiveGroup(ctx, group.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	wails := account.NewService(reference)
	service, session := openBankFixture(t, root)
	fault := &bankMaintenanceFault{Service: service}
	p, h, finish := shellMaintenanceHarness(t, fault)
	var final []domain.AccountRecord

	for _, tc := range []struct {
		id                 domain.AccountID
		name, tracking     string
		next, incompatible domain.AccountType
		steps              int
	}{
		{property, "Updated synthetic estimate", "Manual valuation", domain.TypeCollectible, domain.TypeDigitalWallet, 2},
		{broker, "Updated synthetic holdings", "Cash and holdings", domain.TypeInvestmentAccount, domain.TypeCashOnHand, 1},
		{cash, "Updated currency pocket", "Cash by currency", domain.TypeCashOnHand, domain.TypeBrokerage, 0},
		{empty, "Updated empty holdings", "Cash and holdings", domain.TypeInvestmentAccount, domain.TypeCashOnHand, 1},
	} {
		h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: tc.id}}
		finish()
		prior := readBankRecord(t, service, tc.id)
		before := shellFinancialState(t, service, tc.id)
		if p.current == nil || len(p.current.rows) != 1 || p.current.rows[0].value == nil {
			t.Fatal("initial detail valuation unavailable")
		}
		valuationBefore := wire.FromAccountValuation(*p.current.rows[0].value)
		if tc.id == empty && (len(before.activities) != 0 || len(before.cash) != 0 || len(before.holdings) != 0) {
			t.Fatalf("unexpected financial facts in empty Holdings: %+v", before)
		}
		if tc.id != empty && (len(before.activities) == 0 || (tc.id != property && len(before.cash) < 2) || (tc.id == broker && len(before.holdings) != 1)) {
			t.Fatalf("financial fixture is incomplete: %+v", before)
		}
		label := func(s string) { assertPageLabel(t, &h.router, s) }
		h.click("Account settings", false)
		h.frame()
		label("Asset · " + tc.tracking + " · USD. Role, tracking, and currency are fixed.")
		label("Former shell group · Archived")
		d := p.maintenance.draft
		if d == nil {
			t.Fatal("maintenance draft was unavailable")
		}
		setBankField(h, "Account name", &d.name, tc.name)
		h.click("Partner Sample", false)
		h.click("Equal shares", false)
		setBankField(h, "Share for Sample member (%)", &d.owner(first).percent, "75.25")
		setBankField(h, "Share for Partner Sample (%)", &d.owner(partner.ID).percent, "24.75")
		updates := fault.updates.Load()
		d.kind = tc.incompatible
		h.click("Save changes", false)
		finish()
		label("Choose an account type compatible with this role and tracking method.")
		if fault.updates.Load() != updates || !reflect.DeepEqual(readBankRecord(t, service, tc.id), prior) {
			t.Fatal("incompatible type reached the writer or changed the account")
		}
		// The worker also checks a stale or invalid conversion before mutation.
		if result := p.writeAccountMaintenance(tc.id, nil, application.AccountInput{AccountType: tc.incompatible.String(), Name: tc.name}); result.message != i18n.AccountTypeIncompatible ||
			result.err != nil || fault.updates.Load() != updates {
			t.Fatal("worker accepted a forbidden conversion")
		}
		invalid := tc.incompatible.String()
		if _, err := wails.UpdateAccount(ctx, tc.id.String(), account.UpdateAccountRequest{AccountType: &invalid, Name: &tc.name}); err == nil ||
			!reflect.DeepEqual(readBankRecord(t, reference, tc.id), prior) {
			t.Fatal("Wails did not reject the same incompatible conversion")
		}
		d.kind = prior.Account.AccountType
		h.frame()
		if tc.steps != 0 {
			h.click("Account type", true)
			if tc.id == broker && findPageLabel(&h.router, "Cash on hand") != nil {
				t.Fatal("cash-only type offered as a mixed Holdings conversion")
			}
			for range tc.steps {
				h.press(key.NameDownArrow)
			}
			h.press(key.NameReturn)
		}
		if d.kind != tc.next {
			t.Fatalf("picker selected %s, want %s", d.kind, tc.next)
		}
		h.click("Save changes", false)
		finish()
		label("Account settings saved.")
		label("Update failed. Showing the previous result.")
		if findPageLabel(&h.router, "Account settings") != nil {
			t.Fatal("stale detail offered another edit")
		}
		h.click("Retry", false)
		finish()
		label(tc.name)
		label("Group · Former shell group · Archived")
		if fault.updates.Load() != updates+1 || p.target.account != tc.id {
			t.Fatal("settings refresh repeated the write or changed the ID")
		}
		kind := tc.next.String()
		_, err := wails.UpdateAccount(ctx, tc.id.String(), account.UpdateAccountRequest{
			AccountType: &kind, Name: &tc.name,
			Ownership: []wire.OwnershipShareDTO{{MemberID: first.String(), ShareBPS: 7525}, {MemberID: partner.ID.String(), ShareBPS: 2475}},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertParity := func(archived bool) domain.AccountRecord {
			t.Helper()
			state := readBankRecord(t, service, tc.id)
			adapterRecords, err := wails.ListAccounts(ctx, account.AccountFilterRequest{IncludeArchived: true})
			index := slices.IndexFunc(adapterRecords, func(dto wire.AccountRecordDTO) bool { return dto.Account.ID == tc.id.String() })
			if err != nil || index < 0 || !reflect.DeepEqual(wire.FromAccountRecord(state), adapterRecords[index]) {
				t.Fatalf("Gio and Wails account state differs: %+v %v", state, err)
			}
			if (state.Account.ArchivedAt != nil) != archived || state.Account.TrackingMode != prior.Account.TrackingMode || !state.Account.CreatedAt.Equal(prior.Account.CreatedAt) ||
				!reflect.DeepEqual(shellFinancialState(t, service, tc.id), before) || !reflect.DeepEqual(shellFinancialState(t, reference, tc.id), before) {
				t.Fatalf("maintenance changed identity, archival state or financial facts: %+v", state)
			}
			if !archived {
				if p.current == nil || len(p.current.rows) != 1 || p.current.rows[0].value == nil {
					t.Fatal("active detail valuation unavailable after maintenance")
				}
				current := wire.FromAccountValuation(*p.current.rows[0].value)
				institution := state.Account.InstitutionID.String()
				oracle, err := wails.AccountValuations(ctx, account.AccountFilterRequest{InstitutionID: &institution})
				i := slices.IndexFunc(oracle, func(value wire.AccountValuationDTO) bool { return value.Account.ID == tc.id.String() })
				if err != nil || i < 0 || !reflect.DeepEqual(current, oracle[i]) ||
					!reflect.DeepEqual(current.DisplayValue, valuationBefore.DisplayValue) || !reflect.DeepEqual(current.BaseValue, valuationBefore.BaseValue) {
					t.Fatalf("active detail valuation changed or diverged from Wails: %+v %v", current, err)
				}
			}
			return state
		}
		assertParity(false)
		archives := fault.archives.Load()
		h.click("Archive account", false)
		h.frame()
		h.click("Archive account", false)
		finish()
		label("Account archived.")
		label("Update failed. Showing the previous result.")
		h.click("Retry", false)
		finish()
		label("Archived account. Restore it to edit its settings.")
		if fault.archives.Load() != archives+1 || findPageLabel(&h.router, "Account settings") != nil {
			t.Fatal("archived account was editable or Retry archived it twice")
		}
		if err := wails.ArchiveAccount(ctx, tc.id.String(), true); err != nil {
			t.Fatal(err)
		}
		assertParity(true)
		h.click("Restore account", false)
		h.frame()
		h.click("Restore account", false)
		finish()
		label("Account restored.")
		label("Account settings")
		if err := wails.ArchiveAccount(ctx, tc.id.String(), false); err != nil {
			t.Fatal(err)
		}
		final = append(final, assertParity(false))
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	for _, want := range final {
		if got := readBankRecord(t, reopened, want.Account.ID); !reflect.DeepEqual(got, want) ||
			!reflect.DeepEqual(shellFinancialState(t, reopened, want.Account.ID), shellFinancialState(t, reference, want.Account.ID)) {
			t.Fatalf("reopened state or facts changed: %+v", got)
		}
	}
}

func TestManualAccountMissingValueCanArchiveButCannotEdit(t *testing.T) {
	root := createFixtureRoot(t)
	seedAccountFixture(t, root) // Keep the legacy missing-value fixture outside History.
	service, session := openBankFixture(t, root)
	core, err := service.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateAccount(t.Context(), application.AccountInput{
		Name: "Legacy missing estimate", AccountType: "property", BalanceSheetRole: "asset", TrackingMode: "manual_value",
		DefaultCurrency: "AUD", InitialAmount: "0", OwnerIDs: []domain.MemberID{core.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := asset.Account.ID
	if _, err := session.DB.SQL.ExecContext(t.Context(), "DELETE FROM account_values WHERE account_id = ?", id.String()); err != nil {
		t.Fatal(err)
	}
	fault := &bankMaintenanceFault{Service: service}
	p, h, finish := shellMaintenanceHarness(t, fault)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: id}}
	finish()
	assertPageLabel(t, &h.router, "Settings require a recorded account value.")
	assertPageLabel(t, &h.router, "AUD value unavailable")
	if findPageLabel(&h.router, "Account settings") != nil {
		t.Fatal("legacy missing valuation was editable")
	}
	if got := p.writeAccountMaintenance(id, nil, application.AccountInput{Name: "Forbidden"}); got.message != i18n.AccountMaintenanceChanged || fault.updates.Load() != 0 {
		t.Fatal("stale manual settings could submit with a missing valuation")
	}
	h.click("Archive account", false)
	h.frame()
	h.click("Archive account", false)
	finish()
	h.click("Retry", false)
	finish()
	assertPageLabel(t, &h.router, "Archived account. Restore it to edit its settings.")
	h.click("Restore account", false)
	h.frame()
	h.click("Restore account", false)
	finish()
	assertPageLabel(t, &h.router, "Settings require a recorded account value.")
	if restored := readBankRecord(t, service, id); restored.LatestValue != nil || restored.Account.ArchivedAt != nil ||
		restored.Account.Name != asset.Account.Name || fault.archives.Load() != 2 || fault.updates.Load() != 0 {
		t.Fatalf("lifecycle changed the missing-value account: %+v", restored)
	}
}

func shellMaintenanceHarness(t *testing.T, service *bankMaintenanceFault) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	p := newAccountsPage(t.Context(), service, func() time.Time { return accountTestClock }, func() {})
	p.maintenance = &accountMaintenance{service: service}
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.size = image.Pt(700, 620)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	return p, h, finish
}

type shellAccountFacts struct {
	value      *domain.AccountValue
	cash       []domain.AccountCashValue
	holdings   []domain.Holding
	activities []domain.Activity
}

func shellFinancialState(t *testing.T, service *application.Service, id domain.AccountID) shellAccountFacts {
	t.Helper()
	ctx := t.Context()
	state := shellAccountFacts{value: readBankRecord(t, service, id).LatestValue, activities: openingActivity(t, service, id)}
	cash, err := service.ListAccountCashValues(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	holdings, err := service.HoldingsByAccounts(ctx, []domain.AccountID{id})
	if err != nil {
		t.Fatal(err)
	}
	state.cash, state.holdings = cash, holdings[id]
	return state
}
