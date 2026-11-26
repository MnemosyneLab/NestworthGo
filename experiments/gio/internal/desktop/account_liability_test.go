package desktop

import (
	"image"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestLiabilityCreationMatchesWailsAndKeepsRetryReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name, currency, opening, exact string
		kind                           domain.AccountType
		history, linked, failWrite     bool
	}{
		{"Sample credit card", "USD", "321.09", "USD · -US$321.09", domain.TypeCreditCard, true, true, true},
		{"Sample loan", "SGD", "43.21", "SGD · -S$43.21", domain.TypeLoan, false, false, false},
		{"Sample zero loan", "USD", "0", "USD · US$0.00", domain.TypeLoan, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			root := createFixtureRoot(t)
			if !tc.history {
				seedAccountFixture(t, root)
			}
			_, seed := openBankFixture(t, root)
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			reference, _ := openBankFixture(t, copyBankProfile(t, root))
			service, session := openBankFixture(t, root)
			fault := &accountCreationTestService{Service: service, failWrite: tc.failWrite, failRead: true}
			clock := func() time.Time { return accountTestClock }
			p := newAccountsPage(ctx, fault, clock, func() {})
			p.creation = &balanceAccountCreation{service: fault}
			p.maintenance = &accountMaintenance{service: service}
			overview := newOverviewPage(ctx, service, clock, func() {})
			t.Cleanup(func() { p.workers.Wait(); overview.workers.Wait() })
			h := newShellHarnessWithPages(t, ledgerPages{accounts: p, overview: overview})
			h.size = image.Pt(700, 460)
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
			label := func(value string) { assertPageLabel(t, &h.router, value) }
			h.frame()
			overview.workers.Wait()
			h.frame()
			if overview.current == nil {
				t.Fatal("Overview did not load before creating the account")
			}
			starting := overview.current.value
			h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute}}
			finish()
			startNewAccount(h, finish, "Credit card")
			d := p.creation.draft
			if d == nil || !d.available || d.kind != domain.TypeCreditCard || len(d.owners) != 1 || d.institution.InstitutionType != domain.InstitutionBank {
				t.Fatal("the liability form did not start with the card, sole owner and suggested bank")
			}
			owner := d.owners[0]
			label("New liability account")
			label("Enter the amount owed as a positive number, or leave blank for zero.")
			if tc.kind == domain.TypeLoan {
				h.click("Credit card", false)
				h.click("Loan", false)
				h.frame()
			}
			pickAccountReference(h, "Institution", "No institution", "No institution")
			if d.institution.ID != "" || !d.institutionExplicit {
				t.Fatal("explicit no institution failed to bind")
			}
			if tc.linked {
				pickAccountReference(h, "Institution", "Harbor", "Harbor Sample Bank")
				if d.institution.ID == "" || !d.institutionExplicit {
					t.Fatal("selected bank failed to bind")
				}
			}
			if d.currency.String() != tc.currency {
				h.click("Account currency", true)
				for range 6 {
					h.press(key.NameDownArrow)
				}
				h.press(key.NameReturn)
				if d.currency.String() != tc.currency {
					t.Fatalf("selected currency = %s", d.currency)
				}
			}
			setBankField(h, "Account name", &d.name, tc.name)
			setBankField(h, "Opening amount owed", &d.opening, "-3")
			reviewBankAccount(h)
			label("Enter a supported nonnegative amount using a decimal point.")
			if fault.writes.Load() != 0 {
				t.Fatal("a negative opening amount reached the writer")
			}
			setBankField(h, "Opening amount owed", &d.opening, tc.opening)
			reviewBankAccount(h)
			label("Review liability account")
			if !liabilityTextAppears(h.router.AppendSemantics(nil), "Opening amount owed · "+tc.opening+" "+tc.currency) {
				t.Fatal("the review did not present the exact positive opening amount")
			}
			if f := p.creation.review; f == nil || f.input.AccountType != tc.kind.String() || f.input.BalanceSheetRole != "liability" || f.input.TrackingMode != "balance" {
				t.Fatalf("unexpected review input: %+v", f)
			}
			h.click("Create liability account", false)
			finish()
			expectedWrites := int32(1)
			if tc.failWrite {
				label("Could not create the liability account.")
				if p.creation.draft != d || d.name.Text() != tc.name || d.opening.Text() != tc.opening || p.target.account != "" {
					t.Fatal("a known failed write lost the draft or opened a phantom account")
				}
				rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
				if err != nil || slices.ContainsFunc(rows, func(r domain.AccountRecord) bool { return r.Account.Name == tc.name }) {
					t.Fatalf("a failed submission wrote an account: %v %v", rows, err)
				}
				reviewBankAccount(h)
				h.click("Create liability account", false)
				finish()
				expectedWrites++
			}
			label("Liability account created.")
			label("Could not load account data.")
			id := p.target.account
			if id == "" || p.creation.draft != nil {
				t.Fatal("a committed write did not route to the returned account ID")
			}
			h.click("Retry", false)
			finish()
			label(tc.name)
			label(tc.exact)
			if p.target.account != id || fault.writes.Load() != expectedWrites {
				t.Fatal("detail Retry wrote again or lost the account ID")
			}
			if tc.currency == "SGD" {
				label("AUD value unavailable")
			}

			wails := account.NewService(reference)
			kind := tc.kind.String()
			group := fixtureDefaultAccountGroup(t, reference)
			var institution *string
			if tc.linked {
				value := d.institution.ID.String()
				institution = &value
			}
			expected, err := wails.CreateAccount(ctx, account.CreateAccountRequest{
				Name: tc.name, AccountType: kind, BalanceSheetRole: "liability", TrackingMode: "balance",
				DefaultCurrency: tc.currency, InstitutionID: institution, GroupID: &group, InitialAmount: tc.opening, IncludeInNetWorth: true,
				Ownership: []wire.OwnershipShareDTO{{MemberID: owner.String(), ShareBPS: domain.TotalOwnershipBPS}},
			})
			if err != nil {
				t.Fatal(err)
			}
			expectedID := domain.AccountID(expected.Account.ID)
			assertEquivalent := func() {
				t.Helper()
				actual, other := readBankRecord(t, service, id), readBankRecord(t, reference, expectedID)
				normalize := func(r domain.AccountRecord) wire.AccountRecordDTO {
					dto := wire.FromAccountRecord(r)
					dto.Account.ID, dto.LatestValue.ID, dto.LatestValue.AccountID = "", "", ""
					return dto
				}
				if !reflect.DeepEqual(normalize(actual), normalize(other)) ||
					actual.LatestValue.Amount.CanonicalAmount() != tc.opening || !actual.Account.IsLiability() {
					t.Fatalf("Gio record %+v differs from Wails %+v", normalize(actual), normalize(other))
				}
			}
			assertEquivalent()
			assertLiabilityValuation(t, p.current.rows[0].value, wails, kind, expectedID, tc.opening, tc.currency)

			wantActivities := 0
			if tc.history && tc.opening != "0" {
				wantActivities = 1
			}
			checkOpening := func(reader *application.Service, accountID domain.AccountID) {
				t.Helper()
				page, err := reader.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &accountID})
				if err != nil || len(page.Activities) != wantActivities {
					t.Fatalf("opening activity count: got %+v want %d, error %v", page, wantActivities, err)
				}
				if wantActivities == 1 {
					entry := page.Activities[0]
					if entry.Kind != domain.ActivityCashIn || entry.Reason != domain.ReasonContribution || len(entry.Effects) != 1 ||
						entry.Effects[0].AccountID == nil || *entry.Effects[0].AccountID != accountID {
						t.Fatalf("opening activity differs from shared create semantics: %+v", entry)
					}
				}
			}
			checkOpening(service, id)
			checkOpening(reference, expectedID)

			h.click("Back to accounts", false)
			finish()
			h.focused(&p.createAccount)
			h.click("Forward", false)
			finish()
			if p.target.account != id {
				t.Fatal("Forward did not reopen the created account by ID")
			}
			h.click("Account settings", false)
			h.frame()
			setBankField(h, "Account name", &p.maintenance.draft.name, tc.name+" updated")
			h.click("Save changes", false)
			finish()
			n := tc.name + " updated"
			if _, err := wails.UpdateAccount(ctx, expectedID.String(), account.UpdateAccountRequest{Name: &n}); err != nil {
				t.Fatal(err)
			}
			label("Account settings saved.")
			label(n)
			assertEquivalent()
			for _, archive := range []bool{true, false} {
				action := "Restore account"
				if archive {
					action = "Archive account"
				}
				h.click(action, false)
				h.frame()
				h.click(action, false)
				finish()
				if err := wails.ArchiveAccount(ctx, expectedID.String(), archive); err != nil {
					t.Fatal(err)
				}
				assertEquivalent()
				label(tc.exact)
			}
			checkOpening(service, id)
			checkOpening(reference, expectedID)
			if tc.linked {
				h.click("Overview", false)
				overview.workers.Wait()
				h.frame()
				v := overview.current.value
				if !v.Assets.Equal(starting.Assets) || v.Liabilities.String() != "321.09" || v.NetWorth.String() != "1413.04" {
					t.Fatalf("unexpected post-create Overview: %+v", v)
				}
			}
			stored := readBankRecord(t, service, id)
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, _ := openBankFixture(t, root)
			if got := readBankRecord(t, reopened, id); !reflect.DeepEqual(got, stored) {
				t.Fatal("reopening changed the created liability")
			}
		})
	}
}

func assertLiabilityValuation(t *testing.T, got *domain.AccountValuation, wails *account.Service, kind string, id domain.AccountID, amount, currency string) {
	t.Helper()
	want, err := wails.AccountValuations(t.Context(), account.AccountFilterRequest{AccountType: &kind})
	if err != nil || len(want) != 1 || want[0].Account.ID != id.String() || got == nil {
		t.Fatalf("paired valuations: %+v %+v %v", got, want, err)
	}
	pair := wire.FromAccountValuation(*got)
	// Strip only the generated IDs. Compare the money amounts and missing
	// inputs, including the native total when there is no household FX rate.
	for _, inputs := range [][]wire.MissingInputDTO{pair.MissingInputs, want[0].MissingInputs} {
		for i := range inputs {
			inputs[i].AccountID = ""
		}
	}
	if pair.DisplayValue == nil || want[0].DisplayValue == nil || pair.DisplayValue.Value == nil ||
		pair.DisplayValue.Value.Amount != amount || pair.DisplayValue.Value.Currency != currency ||
		!reflect.DeepEqual(pair.DisplayValue.Value, want[0].DisplayValue.Value) ||
		!reflect.DeepEqual(pair.BaseValue, want[0].BaseValue) || pair.Complete != want[0].Complete ||
		!reflect.DeepEqual(pair.MissingInputs, want[0].MissingInputs) {
		t.Fatalf("Gio and Wails disagree: %+v vs %+v", pair, want[0])
	}
	if currency == "SGD" && (pair.BaseValue != nil || pair.Complete || !slices.ContainsFunc(pair.MissingInputs, func(issue wire.MissingInputDTO) bool {
		return issue.Kind == "fx_rate" && issue.BaseCurrency == "AUD" && issue.QuoteCurrency == "SGD"
	})) {
		t.Fatalf("unavailable conversion treated as a complete or zero value: %+v", pair)
	}
}

func liabilityTextAppears(nodes []input.SemanticNode, text string) bool {
	for _, node := range nodes {
		if strings.Contains(node.Desc.Label, text) || liabilityTextAppears(node.Children, text) {
			return true
		}
	}
	return false
}

func TestLiabilityCreationDoesNotSilentlyDropAnArchivedInstitution(t *testing.T) {
	ctx := t.Context()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	p := newAccountsPage(ctx, service, func() time.Time { return accountTestClock }, func() {})
	p.creation = &balanceAccountCreation{service: service}
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute}}
	finish()
	startNewAccount(h, finish, "Credit card")
	d := p.creation.draft
	setBankField(h, "Account name", &d.name, "Synthetic revalidated liability")
	pickAccountReference(h, "Institution", "No institution", "No institution")
	pickAccountReference(h, "Institution", "Harbor", "Harbor Sample Bank")
	if d.institution.ID == "" || !d.institutionExplicit {
		t.Fatal("no institution was selected")
	}
	reviewBankAccount(h)
	if err := service.ArchiveInstitution(ctx, d.institution.ID, true); err != nil {
		t.Fatal(err)
	}
	h.click("Create liability account", false)
	finish()
	assertPageLabel(t, &h.router, "Choose an active institution or no institution.")
	if p.target.account != "" || p.creation.draft != d || d.institution.ID == "" {
		t.Fatal("the rejected write navigated, lost the draft or cleared the unavailable choice")
	}
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("an unavailable institution wrote a liability: %+v %v", rows, err)
	}
	pickAccountReference(h, "Institution", "No institution", "No institution")
	reviewBankAccount(h)
	h.click("Create liability account", false)
	finish()
	if p.target.account == "" {
		t.Fatal("removing the invalid reference did not allow a valid confirmation")
	}
	record := readBankRecord(t, service, p.target.account)
	if record.Account.InstitutionID != nil || record.Account.Name != "Synthetic revalidated liability" {
		t.Fatalf("updated selection was not saved: %+v", record)
	}
}
