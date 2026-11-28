package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type investmentDebtReplacementFixture struct {
	investmentAmendmentFixture
	loan, bank domain.AccountID
}

func seedInvestmentDebtReplacements(t *testing.T, now time.Time) investmentDebtReplacementFixture {
	t.Helper()
	ctx := t.Context()
	fixture := seedInvestmentAmendments(t, now)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "debt-and-value"), fixture.seed)
	beforeActivities := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -4).Add(time.Hour)
	db, _, service := openStage6Database(t, path, func() time.Time { return beforeActivities })
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name string, kind domain.AccountType, role domain.BalanceSheetRole, initial string) domain.AccountID {
		t.Helper()
		account, err := service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: string(kind), BalanceSheetRole: string(role),
			TrackingMode: "balance", DefaultCurrency: "EUR", InitialAmount: initial,
			OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		if value := account.LatestValue; value == nil || value.Amount.CanonicalAmount() != initial || value.Amount.Currency() != "EUR" {
			t.Fatalf("%q initial balance = %+v, want %s EUR", name, value, initial)
		}
		return account.Account.ID
	}
	loan := create("Sample Replacement Loan", domain.TypeLoan, domain.RoleLiability, "400")
	bank := create("Sample Replacement Bank", domain.TypeBankAccount, domain.RoleAsset, "1000")
	fixture.seed = stage6SeedBytes(t, db)
	return investmentDebtReplacementFixture{investmentAmendmentFixture: fixture, loan: loan, bank: bank}
}

func TestExportInvestmentDebtReplacementFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_INVESTMENT_DEBT_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_INVESTMENT_DEBT_QA_DIR to an existing disposable directory to export native QA fixtures")
	}
	root, err := os.MkdirTemp(output, "investment-debt-replacement-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedInvestmentDebtReplacements(t, time.Now())
	for _, profile := range []string{"source", "wails", "gio"} {
		writeStage6Fixture(t, filepath.Join(root, profile), fixture.seed)
	}
	t.Logf("fixtures=%s household=%q buy=%s sell=%s dividend=%s", root, investmentAmendmentHousehold,
		fixture.buy.activity.EffectiveLocalDate, fixture.sell.activity.EffectiveLocalDate, fixture.dividend.activity.EffectiveLocalDate)
}

func TestHistoryInvestmentDebtAndValueReplacementsMatchWails(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	fixture := seedInvestmentDebtReplacements(t, now)
	gio, reference := openInvestmentReplacementPair(t, fixture.seed, now)
	cases := []struct {
		original          investmentAmendmentTarget
		fields            historyapi.ChangeCommandRequest
		amount, fee       string
		bank, outstanding string
		originalCash      map[domain.CurrencyCode]string
		originalQuantity  string
	}{
		{
			original: fixture.buy, fields: historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangeDebtDraw, DebtAccountID: fixture.loan.String(), CashAccountID: fixture.bank.String(), PrincipalCurrency: "EUR",
			},
			amount: "125.125", bank: "1125.125", outstanding: "525.125",
			originalCash: map[domain.CurrencyCode]string{"EUR": "1000"}, originalQuantity: "0",
		},
		{
			original: fixture.sell, fields: historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangeDebtPayment, DebtAccountID: fixture.loan.String(), CashAccountID: fixture.bank.String(), PrincipalCurrency: "EUR",
			},
			amount: "85.25", fee: "2.125", bank: "1037.75", outstanding: "439.875",
			originalCash: map[domain.CurrencyCode]string{"EUR": "800"}, originalQuantity: "20",
		},
		{
			original: fixture.dividend, fields: historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangeValueUpdate, AccountID: fixture.bank.String(), NewValueCurrency: "EUR",
			},
			amount: "1234.5678", bank: "1234.5678", outstanding: "439.875",
			originalCash: map[domain.CurrencyCode]string{"EUR": "900", "USD": "30"}, originalQuantity: "10",
		},
	}
	for _, step := range cases {
		t.Logf("replace %s with %s", step.original.activity.Kind, step.fields.Kind)
		original, err := gio.Activity(ctx, domain.ActivityID(step.original.activity.ID))
		if err != nil {
			t.Fatal(err)
		}
		sources, err := history.LoadInvestmentReplacementSources(ctx, gio, original)
		if err != nil {
			t.Fatal(err)
		}
		form := &historyAmendmentDraft{investment: &investmentAmendmentDraft{command: step.fields, sources: sources}}
		form.amount.Editor.SetText(step.amount)
		form.fee.Editor.SetText(step.fee)
		form.investment.note.Editor.SetText("Synthetic investment replacement")
		replacement, ready := investmentReplacementRequest(form)
		if !ready {
			t.Fatalf("valid %s replacement was not ready: %+v", step.fields.Kind, replacement)
		}
		assertInvestmentReplacementMatches(t, gio, reference, step.original.activity, replacement, now)
		for _, service := range []*application.Service{gio, reference} {
			assertNativeTradeCash(t, service, step.original.account, step.originalCash)
			assertReplacementHolding(t, service, step.original, step.originalQuantity, false)
			rows, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{fixture.bank, fixture.loan}})
			if err != nil || len(rows) != 2 {
				t.Fatalf("read exact debt and value balances: %+v (%v)", rows, err)
			}
			for _, row := range rows {
				want := step.bank
				if row.Account.ID == fixture.loan {
					want = step.outstanding
				}
				if value := row.LatestValue; value == nil || value.Amount.CanonicalAmount() != want || value.Amount.Currency() != "EUR" {
					t.Fatalf("%s current value = %+v, want %s EUR", row.Account.Name, value, want)
				}
			}
		}
	}
}
