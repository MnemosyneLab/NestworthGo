package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"reflect"
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

func TestRemainingBalanceTypesMatchWails(t *testing.T) {
	root := createFixtureRoot(t)
	_, seed := openBankFixture(t, root)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	service, session := openBankFixture(t, root)
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	wails := account.NewService(reference)
	group := fixtureDefaultAccountGroup(t, reference)
	var accepted []domain.AccountRecord
	for _, tc := range []struct {
		name, label, initial string
		kind                 domain.AccountType
		role                 domain.BalanceSheetRole
		liquid               bool
	}{
		{"Sample cash", "Cash on hand", "12.05", domain.TypeCashOnHand, domain.RoleAsset, true},
		{"Sample wallet", "Digital wallet", "22.25", domain.TypeDigitalWallet, domain.RoleAsset, true},
		{"Sample receivable", "Receivable", "44", domain.TypeReceivable, domain.RoleAsset, false},
		{"Sample other asset", "Other account", "", domain.TypeOther, domain.RoleAsset, false},
		{"Sample other liability", "Other account", "51.25", domain.TypeOther, domain.RoleLiability, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			p := newAccountsPage(ctx, service, func() time.Time { return accountTestClock }, func() {})
			p.creation = &balanceAccountCreation{service: service}
			t.Cleanup(p.workers.Wait)
			h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
			h.size = image.Pt(700, 460)
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
			h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute}}
			finish()
			mode, picker, done, start := "asset", "Asset type", "Asset account created.", "Cash on hand"
			if tc.role == domain.RoleLiability {
				mode, picker, done, start = "liability", "Liability type", "Liability account created.", "Credit card"
			}
			startNewAccount(h, finish, start)
			d := p.creation.draft
			if d == nil || d.role != tc.role || len(d.owners) != 1 {
				t.Fatal("expected role and one owner in the new draft")
			}
			if tc.kind == domain.TypeCashOnHand {
				setBankField(h, "Account name", &d.name, "Blocked creation")
				d.kind = domain.TypeLoan
				reviewBankAccount(h)
				assertPageLabel(t, &h.router, "Choose an account type compatible with this role and tracking method.")
				rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
				if err != nil || len(rows) != 1 || p.creation.review != nil {
					t.Fatalf("incompatible creation passed the form: %v, %v", rows, err)
				}
			}
			h.click(picker, true)
			finish()
			h.click(tc.label, false)
			h.frame()
			setBankField(h, "Account name", &d.name, tc.name)
			opening := "Opening balance"
			if tc.role == domain.RoleLiability {
				opening = "Opening amount owed"
			}
			setBankField(h, opening, &d.opening, tc.initial)
			reviewBankAccount(h)
			assertPageLabel(t, &h.router, "Review "+mode+" account")
			h.click("Create "+mode+" account", false)
			finish()
			assertPageLabel(t, &h.router, done)
			assertPageLabel(t, &h.router, tc.name)
			actual := readBankRecord(t, service, p.target.account)

			amount := tc.initial
			if amount == "" {
				amount = "0"
			}
			var institution *string
			if tc.kind == domain.TypeDigitalWallet {
				if d.institution.Name != "Harbor Sample Bank" {
					t.Fatal("wallet did not select the suggested bank")
				}
				id := d.institution.ID.String()
				institution = &id
			}
			want, err := wails.CreateAccount(ctx, account.CreateAccountRequest{
				Name: tc.name, AccountType: tc.kind.String(), BalanceSheetRole: tc.role.String(), TrackingMode: "balance",
				DefaultCurrency: "USD", InstitutionID: institution, GroupID: &group, InitialAmount: amount, IncludeInNetWorth: true, IncludeInLiquidAssets: tc.liquid,
				Ownership: []wire.OwnershipShareDTO{{MemberID: d.owners[0].String(), ShareBPS: 10000}},
			})
			if err != nil {
				t.Fatal(err)
			}
			expected := readBankRecord(t, reference, domain.AccountID(want.Account.ID))
			normalize := func(r domain.AccountRecord) wire.AccountRecordDTO {
				dto := wire.FromAccountRecord(r)
				dto.Account.ID, dto.LatestValue.ID, dto.LatestValue.AccountID = "", "", ""
				return dto
			}
			if actual.Account.AccountType != tc.kind || actual.Account.BalanceSheetRole != tc.role ||
				actual.LatestValue.Amount.CanonicalAmount() != amount || !reflect.DeepEqual(normalize(actual), normalize(expected)) {
				t.Fatalf("Gio record %+v differs from Wails %+v", normalize(actual), normalize(expected))
			}
			for _, entry := range []struct {
				service *application.Service
				id      domain.AccountID
			}{{service, actual.Account.ID}, {reference, expected.Account.ID}} {
				page, err := entry.service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &entry.id})
				if err != nil {
					t.Fatal(err)
				}
				if amount == "0" {
					if len(page.Activities) != 0 {
						t.Fatal("zero opening amount created an activity")
					}
				} else if len(page.Activities) != 1 || page.Activities[0].Reason != domain.ReasonContribution {
					t.Fatalf("unexpected opening evidence: %+v", page.Activities)
				}
			}
			accepted = append(accepted, actual)
			h.click("Back to accounts", false)
			finish()
			h.focused(&p.createAccount)
		})
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	for _, want := range accepted {
		if got := readBankRecord(t, reopened, want.Account.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("account changed during reopen: %+v, want %+v", got, want)
		}
	}
}

type countedTypeMaintenance struct {
	*application.Service
	updates atomic.Int32
}

func (s *countedTypeMaintenance) UpdateAccount(ctx context.Context, id domain.AccountID, input application.AccountInput) (domain.AccountRecord, error) {
	s.updates.Add(1)
	return s.Service.UpdateAccount(ctx, id, input)
}

func TestBalanceTypeConversionMatchesWailsAndRejectsIncompatible(t *testing.T) {
	for _, tc := range []struct {
		role                              domain.BalanceSheetRole
		source, to, invalid, invalidTrack domain.AccountType
		sourceLabel, toLabel, absentLabel string
	}{
		{domain.RoleAsset, domain.TypeBankAccount, domain.TypeReceivable, domain.TypeLoan, domain.TypeProperty, "Bank account", "Receivable", "Loan"},
		{domain.RoleLiability, domain.TypeCreditCard, domain.TypeLoan, domain.TypeBankAccount, domain.TypeProperty, "Credit card", "Loan", "Bank account"},
	} {
		t.Run(tc.role.String(), func(t *testing.T) {
			ctx := t.Context()
			root := createFixtureRoot(t)
			seed, initial := openBankFixture(t, root)
			rows, err := seed.ListAccounts(ctx, domain.AccountFilter{})
			if err != nil || len(rows) != 1 {
				t.Fatalf("sample balance ledger: %v, %v", rows, err)
			}
			old := rows[0]
			if tc.role == domain.RoleLiability {
				old, err = seed.CreateAccount(ctx, application.AccountInput{
					Name: "Conversion sample card", AccountType: tc.source.String(), BalanceSheetRole: tc.role.String(), TrackingMode: "balance",
					DefaultCurrency: "USD", InitialAmount: "10.25", OwnerIDs: []domain.MemberID{old.Ownership.Shares()[0].MemberID}, IncludeInNetWorth: true,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			id := old.Account.ID
			beforeHistory, err := seed.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &id})
			if err != nil || len(beforeHistory.Activities) == 0 {
				t.Fatalf("missing linked history: %v, %v", beforeHistory, err)
			}
			if err := initial.Close(); err != nil {
				t.Fatal(err)
			}
			reference, _ := openBankFixture(t, copyBankProfile(t, root))
			service, session := openBankFixture(t, root)
			original := readBankRecord(t, service, id)
			wails := account.NewService(reference)
			counted := &countedTypeMaintenance{Service: service}
			p := newAccountsPage(ctx, service, func() time.Time { return accountTestClock }, func() {})
			p.maintenance = &accountMaintenance{service: counted}
			t.Cleanup(p.workers.Wait)
			h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
			h.size = image.Pt(700, 460)
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			finish := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
			h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: id}}
			finish()
			h.click("Account settings", false)
			h.frame()
			assertPageLabel(t, &h.router, fmt.Sprintf("%s · Total balance · USD. Role, tracking, and currency are fixed.", map[domain.BalanceSheetRole]string{domain.RoleAsset: "Asset", domain.RoleLiability: "Liability"}[tc.role]))
			h.click("Account type", true)
			assertPageLabel(t, &h.router, tc.toLabel)
			if findPageLabel(&h.router, tc.absentLabel) != nil || findPageLabel(&h.router, "Property") != nil {
				t.Fatal("picker offered a different role or tracking combination")
			}
			h.click(tc.sourceLabel, false)
			h.frame()
			observations := func() int {
				t.Helper()
				var n int
				if err := session.DB.SQL.QueryRowContext(ctx, "SELECT count(*) FROM account_state_observations WHERE account_id = ?", id.String()).Scan(&n); err != nil {
					t.Fatal(err)
				}
				return n
			}
			priorObservations := observations()
			for _, invalid := range []domain.AccountType{tc.invalid, tc.invalidTrack} {
				p.maintenance.draft.kind = invalid
				h.frame()
				h.click("Save changes", false)
				finish()
				assertPageLabel(t, &h.router, "Choose an account type compatible with this role and tracking method.")
				if counted.updates.Load() != 0 || observations() != priorObservations || !reflect.DeepEqual(readBankRecord(t, service, id), original) {
					t.Fatal("invalid setting wrote before the shared service")
				}
				value := invalid.String()
				_, err = wails.UpdateAccount(ctx, id.String(), account.UpdateAccountRequest{AccountType: &value})
				var invalidWire *apierror.WireError
				if !errors.As(err, &invalidWire) || invalidWire.Code != string(domain.ErrValidation) ||
					!reflect.DeepEqual(readBankRecord(t, reference, id), original) {
					t.Fatalf("Wails adapter did not reject the same incompatible type: %v", err)
				}
			}
			h.click("Account type", true)
			h.click(tc.toLabel, false)
			h.frame()
			h.click("Save changes", false)
			finish()
			assertPageLabel(t, &h.router, "Account settings saved.")
			result := readBankRecord(t, service, id)
			kind := tc.to.String()
			if _, err := wails.UpdateAccount(ctx, id.String(), account.UpdateAccountRequest{AccountType: &kind}); err != nil {
				t.Fatal(err)
			}
			if counted.updates.Load() != 1 || observations() != priorObservations+1 || result.Account.AccountType != tc.to ||
				!reflect.DeepEqual(wire.FromAccountRecord(result), wire.FromAccountRecord(readBankRecord(t, reference, id))) {
				t.Fatalf("accepted conversion differs from Wails: %+v", result)
			}
			// With the type put back, all financial and historical fields
			// remain equal, including the creation-time inclusion flags.
			unchanged := result
			unchanged.Account.AccountType, unchanged.Account.UpdatedAt = original.Account.AccountType, original.Account.UpdatedAt
			if !reflect.DeepEqual(unchanged, original) {
				t.Fatalf("type edit altered balance, ownership or other fields: %+v, before %+v", unchanged, original)
			}
			page, err := service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &id})
			if err != nil || !reflect.DeepEqual(page, beforeHistory) {
				t.Fatalf("type edit changed historical activity: %+v, %v", page, err)
			}
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, _ := openBankFixture(t, root)
			if !reflect.DeepEqual(readBankRecord(t, reopened, id), result) {
				t.Fatal("type edit did not survive reopening")
			}
		})
	}
}
