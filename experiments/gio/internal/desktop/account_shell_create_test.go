package desktop

import (
	"image"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestValuedAndHoldingsCreationThroughInstitution(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, initial := openBankFixture(t, root)
	affiliations, err := seed.ListInstitutions(ctx, false)
	if err != nil || len(affiliations) == 0 {
		t.Fatalf("synthetic institutions: %v", err)
	}
	institution := affiliations[0]
	group, err := seed.CreateGroup(ctx, "Long-term sample")
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	copy := copyBankProfile(t, root)
	service, session := openBankFixture(t, root)
	reference, _ := openBankFixture(t, copy)
	adapter := account.NewService(reference)
	fault := &accountCreationTestService{Service: service, failRead: true}
	p, h, finish := accountShellHarness(t, fault, fault, institution.ID)
	var accepted []domain.AccountRecord
	for _, tc := range []struct {
		name, kindLabel, opening, canonical, detail string
		kind                                        domain.AccountType
		mode                                        domain.TrackingMode
		portfolio, liquid, openingOptions           bool
	}{
		{"Sample estimate", "Property", "1234.56", "1234.56", "USD · $ 1,234.56", domain.TypeProperty, domain.TrackingManualValue, false, false, true},
		{"Sample zero", "Property", "", "0", "USD · $ 0.00", domain.TypeProperty, domain.TrackingManualValue, false, false, false},
		{"Sample brokerage", "Brokerage", "", "", "No investment holdings.", domain.TypeBrokerage, domain.TrackingHoldings, true, false, false},
		{"Sample foreign cash", "Cash on hand", "", "", "No cash balances.", domain.TypeCashOnHand, domain.TrackingHoldings, false, true, false},
	} {
		label := func(text string) { assertPageLabel(t, &h.router, text) }
		h.click("Add valued or holdings account", false)
		finish()
		d := p.creation.draft
		if d == nil || d.institution.ID != institution.ID || d.currency != "USD" || len(d.owners) != 1 {
			t.Fatalf("scoped defaults unavailable for %s", tc.name)
		}
		setBankField(h, "Account name", &d.name, tc.name)
		if tc.mode == domain.TrackingHoldings {
			// Changing paths clears the previous estimate and updates the available types.
			setBankField(h, "Opening valuation", &d.opening, "999")
			h.click("Tracking method", true)
			h.click("Cash and holdings", false)
			h.frame()
			if d.mode != tc.mode || d.opening.Text() != "" {
				t.Fatal("mode switch retained an incompatible opening valuation")
			}
			h.click("Asset type", true)
			if tc.kind == domain.TypeCashOnHand {
				h.press(key.NameUpArrow)
				h.press(key.NameUpArrow)
			}
			h.press(key.NameReturn)
			h.frame()
			if d.kind != tc.kind {
				t.Fatalf("selected %s, want %s", d.kind, tc.kindLabel)
			}
			label("Starts empty.")
		} else if tc.opening != "" {
			setBankField(h, "Opening valuation", &d.opening, tc.opening)
		}
		var openedOn, groupID *string
		if d.group != "" {
			id := d.group
			groupID = &id
		}
		if tc.openingOptions {
			if d.openingDates.Min != "2026-11-01" || d.openingDates.Max != "2026-11-22" {
				t.Fatalf("date window did not use the History timezone: %+v", d.openingDates)
			}
			h.click("More account details", false)
			h.frame()
			h.click("Group", true)
			h.router.Queue(key.EditEvent{Text: "Long-term"})
			h.frame()
			h.click("Long-term sample", false)
			h.frame()
			if d.group != group.ID.String() {
				t.Fatalf("group popup did not select the active reference, got %q", d.group)
			}
			h.click("Opening date", true)
			h.press(key.NameLeftArrow)
			h.press(key.NameLeftArrow)
			h.press(key.NameReturn)
			day, id := "2026-11-20", group.ID.String()
			if d.openedOn != day {
				t.Fatalf("date picker got %q, want %q", d.openedOn, day)
			}
			openedOn, groupID = &day, &id
		}
		before := fault.writes.Load()
		reviewBankAccount(h)
		if p.creation.review == nil {
			t.Fatalf("%s review did not open, problem %v, group %s, date %s, owners %v", tc.name, d.problem, d.group, d.openedOn, d.owners)
		}
		label("Review asset account")
		if tc.mode == domain.TrackingHoldings && !liabilityTextAppears(h.router.AppendSemantics(nil), "Default currency: USD\nStarts empty.") {
			t.Fatal("Holdings review omitted the empty account or its default currency")
		}
		if tc.mode == domain.TrackingManualValue && !liabilityTextAppears(h.router.AppendSemantics(nil), "Opening valuation: "+tc.canonical+" USD") {
			t.Fatal("manual review omitted the exact opening amount")
		}
		h.click("Create asset account", false)
		finish()
		label("Asset account created.")
		label("Could not load account data.")
		h.click("Retry", false)
		finish()
		label(tc.name)
		label(tc.detail)
		id := p.target.account
		if id == "" || fault.writes.Load() != before+1 || p.creation.draft != nil {
			t.Fatal("successful create lost the stable ID or replayed the write")
		}
		if tc.kind == domain.TypeCashOnHand {
			label("Cash on hand · Cash by currency")
			if liabilityTextAppears(h.router.AppendSemantics(nil), "No investment holdings.") {
				t.Fatal("cash-only account displayed investments")
			}
		}
		if tc.openingOptions {
			label("Group · Long-term sample")
		}
		institutionID := institution.ID.String()
		req := account.CreateAccountRequest{
			Name: tc.name, AccountType: tc.kind.String(), BalanceSheetRole: "asset", TrackingMode: string(tc.mode),
			InstitutionID: &institutionID, GroupID: groupID, OpenedOn: openedOn,
			InitialAmount: tc.canonical, DefaultCurrency: "USD", OwnerIDs: []string{d.owners[0].String()},
			IncludeInNetWorth: true, IncludeInPortfolio: tc.portfolio, IncludeInLiquidAssets: tc.liquid,
		}
		out, err := adapter.CreateAccount(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		actual := readBankRecord(t, service, id)
		expected := readBankRecord(t, reference, domain.AccountID(out.Account.ID))
		normalize := func(r domain.AccountRecord) wire.AccountRecordDTO {
			dto := wire.FromAccountRecord(r)
			dto.Account.ID = ""
			if dto.LatestValue != nil {
				dto.LatestValue.ID, dto.LatestValue.AccountID = "", ""
			}
			return dto
		}
		if !reflect.DeepEqual(normalize(actual), normalize(expected)) {
			t.Fatalf("Gio record %+v differs from Wails %+v", normalize(actual), normalize(expected))
		}
		if tc.mode == domain.TrackingHoldings && actual.LatestValue != nil {
			t.Fatal("empty holdings acquired a simple account value")
		}
		wantValuation, err := adapter.AccountValuations(ctx, account.AccountFilterRequest{InstitutionID: &institutionID})
		index := slices.IndexFunc(wantValuation, func(value wire.AccountValuationDTO) bool { return value.Account.ID == out.Account.ID })
		if err != nil || index < 0 || p.current.rows[0].value == nil {
			t.Fatalf("missing detail valuation: %v", err)
		}
		got := wire.FromAccountValuation(*p.current.rows[0].value)
		want := wantValuation[index]
		if !reflect.DeepEqual(got.BaseValue, want.BaseValue) || !reflect.DeepEqual(got.DisplayValue, want.DisplayValue) ||
			!reflect.DeepEqual(got.Composition, want.Composition) || got.Complete != want.Complete {
			t.Fatalf("Gio valuation %+v differs from Wails %+v", got, want)
		}
		gotActivity := openingActivity(t, service, actual.Account.ID)
		wantActivity := openingActivity(t, reference, expected.Account.ID)
		if tc.openingOptions {
			when := time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)
			if len(gotActivity) != 1 || len(wantActivity) != 1 || !gotActivity[0].EffectiveAt.Equal(when) ||
				!gotActivity[0].EffectiveAt.Equal(wantActivity[0].EffectiveAt) || gotActivity[0].Reason != domain.ReasonContribution ||
				len(gotActivity[0].Effects) != 1 || len(wantActivity[0].Effects) != 1 ||
				gotActivity[0].Effects[0].Target != wantActivity[0].Effects[0].Target {
				t.Fatalf("unexpected opening contribution: %+v vs %+v", gotActivity, wantActivity)
			}
		} else if len(gotActivity) != 0 || len(wantActivity) != 0 {
			t.Fatalf("%s acquired unexpected activities", tc.name)
		}
		accepted = append(accepted, actual)
		h.click("Back to accounts", false)
		finish()
		if p.target.institution != institution.ID {
			t.Fatal("return lost the originating institution")
		}
		h.focused(&p.createShell)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	for _, want := range accepted {
		if got := readBankRecord(t, reopened, want.Account.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("account changed after reopen: %+v, want %+v", got, want)
		}
	}
}

func accountShellHarness(t *testing.T, reader accountsReader, creator accountCreator, institution domain.InstitutionID) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	p := newAccountsPage(t.Context(), reader, func() time.Time { return accountTestClock }, func() {})
	p.creation = &balanceAccountCreation{service: creator}
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.size = image.Pt(700, 460)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, institution: institution}}
	finish()
	return p, h, finish
}

func openingActivity(t *testing.T, service *application.Service, id domain.AccountID) []domain.Activity {
	t.Helper()
	page, err := service.ListActivityPage(t.Context(), domain.ActivityQuery{AccountID: &id})
	if err != nil {
		t.Fatal(err)
	}
	return page.Activities
}

func TestAccountShellRejectsInvalidRequestsAndShowsMissingValuation(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seedAccountFixture(t, root) // Has no History Starting Point.
	service, session := openBankFixture(t, root)
	counted := &accountCreationTestService{Service: service}
	p, h, finish := accountShellHarness(t, counted, counted, "")
	h.click("Add valued or holdings account", false)
	finish()
	d := p.creation.draft
	setBankField(h, "Account name", &d.name, "Rejected or missing")
	if len(d.owners) != 1 {
		t.Fatal("the sole active owner should be selected by default")
	}
	before, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	d.kind = domain.TypeLoan
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Choose an account type compatible with this role and tracking method.")
	d.selectMode(domain.TrackingHoldings)
	d.opening.Editor.SetText("0")
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Clear the opening valuation for a holdings account.")
	d.selectMode(domain.TrackingManualValue)
	d.openedOn = "2026-11-21"
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Choose a date from the Starting Point through today, or clear the date.")
	after, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || !reflect.DeepEqual(before, after) || counted.writes.Load() != 0 || p.creation.review != nil {
		t.Fatalf("rejected input changed the ledger: %v", err)
	}
	if liabilityTextAppears(h.router.AppendSemantics(nil), "Opening date") {
		t.Fatal("opening date offered without an existing Starting Point")
	}
	d.openedOn = ""
	h.click("Back to details", false)
	h.frame()
	setBankField(h, "Opening valuation", &d.opening, "3.25")
	reviewBankAccount(h)
	h.click("Create asset account", false)
	finish()
	id := p.target.account
	assertPageLabel(t, &h.router, "AUD · A$ 3.25")
	// A legacy synthetic account with absent valuation evidence must stay
	// distinct from the explicit zero case accepted by the normal writer.
	if _, err := session.DB.SQL.ExecContext(ctx, "DELETE FROM account_values WHERE account_id = ?", id.String()); err != nil {
		t.Fatal(err)
	}
	h.click("Reload", false)
	finish()
	assertPageLabel(t, &h.router, "AUD value unavailable")
	value := p.current.rows[0].value
	if p.current.rows[0].record.LatestValue != nil || value == nil || value.Complete ||
		len(value.MissingInputs) != 1 || value.MissingInputs[0].Kind != domain.MissingAccountValue ||
		liabilityTextAppears(h.router.AppendSemantics(nil), "AUD · A$ 0.00") {
		t.Fatalf("missing valuation appears as recorded zero: %+v", value)
	}
}

func TestBalanceCreationOptionalFieldsMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, prepared := openBankFixture(t, root)
	core, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	member := core.Members[0].ID
	partner, err := seed.CreateMember(ctx, "Partner Sample")
	if err != nil {
		t.Fatal(err)
	}
	group, err := seed.CreateGroup(ctx, "New accounts sample group")
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, _ := openBankFixture(t, root)
	fault := &accountCreationTestService{Service: service}
	inst := core.Institutions[0].ID
	p, h, finish := accountShellHarness(t, fault, fault, inst)
	h.click("Add bank account", false)
	finish()
	d := p.creation.draft
	setBankField(h, "Account name", &d.name, "Synthetic account options")
	h.click("More account details", false)
	h.frame()
	choose := func(field, search, label string) {
		t.Helper()
		h.click(field, true)
		h.router.Queue(key.EditEvent{Text: search})
		h.frame()
		h.click(label, false)
		h.frame()
	}
	choose("Group", "New accounts", group.Name)
	h.click("Opening date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	choose("Icon", "Vault", "Vault")
	h.click("Sample member", false)
	h.click("Partner Sample", false)
	h.click("Equal shares", false)
	setBankField(h, "Share for Sample member (%)", &d.owner(member).percent, "75.25")
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Enter positive owner shares totaling exactly 100%, with up to two decimals.")
	if fault.writes.Load() != 0 {
		t.Fatal("invalid create allocation called the writer")
	}
	setBankField(h, "Share for Partner Sample (%)", &d.owner(partner.ID).percent, "24.75")
	reviewBankAccount(h)
	assertPageLabel(t, &h.router, "Review bank account")
	for _, text := range []string{"Icon · Vault", "Group · " + group.Name, "Sample member · 75.25%"} {
		if !liabilityTextAppears(h.router.AppendSemantics(nil), text) {
			t.Fatalf("review omitted %s", text)
		}
	}
	h.click("Create bank account", false)
	finish()
	assertPageLabel(t, &h.router, "Bank account created.")
	actual := readBankRecord(t, service, p.target.account)
	institutionID, groupID, icon, day := inst.String(), group.ID.String(), "vault", "2026-11-21"
	want, err := account.NewService(reference).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Synthetic account options", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InstitutionID: &institutionID, GroupID: &groupID, IconKey: &icon, OpenedOn: &day,
		InitialAmount: "0", IncludeInNetWorth: true, IncludeInLiquidAssets: true,
		Ownership: []wire.OwnershipShareDTO{{MemberID: member.String(), ShareBPS: 7525}, {MemberID: partner.ID.String(), ShareBPS: 2475}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = wire.FromAccountRecord(readBankRecord(t, reference, domain.AccountID(want.Account.ID)))
	got := wire.FromAccountRecord(actual)
	got.Account.ID, got.LatestValue.ID, got.LatestValue.AccountID = "", "", ""
	want.Account.ID, want.LatestValue.ID, want.LatestValue.AccountID = "", "", ""
	if fault.writes.Load() != 1 || !reflect.DeepEqual(got, want) {
		t.Fatalf("Gio options do not match Wails: %+v, want %+v", got, want)
	}
}
