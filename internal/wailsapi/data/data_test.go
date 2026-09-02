package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type memoryDialogs struct {
	save, open string
	replace    bool
}

func (m memoryDialogs) SaveFile(string, string, string, string) (string, error) { return m.save, nil }
func (m memoryDialogs) OpenFile(string, string, string) (string, error)         { return m.open, nil }
func (m memoryDialogs) ConfirmReplace(string) (bool, error)                     { return m.replace, nil }

func TestCreateBackupCancel(t *testing.T) {
	app := wailstest.NewService(t)
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewService(app, nil, database, memoryDialogs{}, native.NoopRefresh{})
	result, err := service.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled {
		t.Fatal("empty save path should cancel")
	}
}

func TestSelectCSVCancel(t *testing.T) {
	service := NewService(nil, nil, nil, memoryDialogs{}, nil)
	result, err := service.SelectCSV("accounts", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled {
		t.Fatal("empty open path should cancel")
	}
}

func TestLastBackupStatusMissing(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewService(nil, nil, database, nil, nil)
	status, err := service.LastBackupStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Fatal("expected no backup status")
	}
}

func TestCommitCSVRequiresPreviewAndConfirm(t *testing.T) {
	app := wailstest.NewService(t)
	if err := app.CompleteOnboarding(context.Background(), application.OnboardingInput{
		HouseholdName: "H", BaseCurrency: "CNY", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "accounts.csv")
	if err := os.WriteFile(path, []byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,Alice:100%\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(app, nil, nil, memoryDialogs{open: path}, native.NoopRefresh{})
	selected, err := service.SelectCSV("accounts", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitCSV(selected.Token); err == nil {
		t.Fatal("commit without preview should fail")
	}
	preview, err := service.PreviewCSV(CSVOptionsRequest{
		Token: selected.Token, Profile: "accounts", DateFormat: "iso", DecimalSep: ".", GroupingSep: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanCommit {
		t.Fatalf("preview errors: %+v", preview.Errors)
	}
	if _, err := service.CommitCSV(selected.Token); err == nil {
		t.Fatal("commit without confirm should fail")
	}
	if _, err := service.ConfirmCSV(selected.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitCSV(selected.Token); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewCSVKeepsAccountsErrorsForSharedSession(t *testing.T) {
	app := wailstest.NewService(t)
	if err := app.CompleteOnboarding(context.Background(), application.OnboardingInput{
		HouseholdName: "H", BaseCurrency: "CNY", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateAccount(context.Background(), application.AccountInput{
		Name: "Broker", AccountType: string(domain.TypeBrokerage), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingHoldings), DefaultCurrency: "CNY", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{mustMemberID(t, app)},
	}); err != nil {
		t.Fatal(err)
	}
	accountsPath := filepath.Join(t.TempDir(), "accounts.csv")
	if err := os.WriteFile(accountsPath, []byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nBroken,not-a-type,asset,balance,CNY,10,2026-08-01,Alice:100%\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdingsPath := filepath.Join(t.TempDir(), "holdings.csv")
	if err := os.WriteFile(holdingsPath, []byte("account_name,instrument_type,instrument_name,quantity,quote_currency,unit_price,quote_date\nBroker,etf,QQQ,10,USD,400.12,2026-08-01\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(app, nil, nil, memoryDialogs{open: accountsPath}, native.NoopRefresh{})
	selected, err := service.SelectCSV("accounts", "")
	if err != nil {
		t.Fatal(err)
	}
	service.dialogs = memoryDialogs{open: holdingsPath}
	if _, err := service.SelectCSV("holdings", selected.Token); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewCSV(CSVOptionsRequest{
		Token: selected.Token, Profile: "holdings", DateFormat: "iso", DecimalSep: ".", GroupingSep: "none",
		Unresolved: []application.CSVUnresolvedAction{{Kind: "instrument", Name: "QQQ", Action: application.CSVUnresolvedCreate}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanCommit || len(preview.Errors) == 0 {
		t.Fatalf("expected aggregated accounts errors, got %+v", preview)
	}
	accountsPreview, err := service.PreviewCSV(CSVOptionsRequest{
		Token: selected.Token, Profile: "accounts", DateFormat: "iso", DecimalSep: ".", GroupingSep: "none",
		Unresolved: []application.CSVUnresolvedAction{{Kind: "instrument", Name: "QQQ", Action: "create"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(accountsPreview.Headers) == 0 || accountsPreview.Headers[0] != "account_name" || len(accountsPreview.PreviewRows) == 0 || accountsPreview.PreviewRows[0][0] != "Broken" {
		t.Fatalf("accounts preview used the wrong file rows: %+v", accountsPreview)
	}
	if _, err := service.ConfirmCSV(selected.Token); err == nil {
		t.Fatal("confirm with errors should fail")
	}
}

func mustMemberID(t *testing.T, app *application.Service) domain.MemberID {
	t.Helper()
	bootstrap, err := app.Bootstrap(context.Background())
	if err != nil || len(bootstrap.Members) == 0 {
		t.Fatalf("bootstrap: %v", err)
	}
	return bootstrap.Members[0].ID
}

func TestCSVSessionExpiresAndEvicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.csv")
	if err := os.WriteFile(path, []byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,Alice:100%\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, nil, nil, memoryDialogs{open: path}, nil)
	service.now = func() time.Time { return clock }
	first, err := service.SelectCSV("accounts", "")
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := service.SelectCSV("accounts", ""); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := service.SelectCSV("accounts", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewCSV(CSVOptionsRequest{Token: first.Token, Profile: "accounts", DateFormat: "iso", DecimalSep: ".", GroupingSep: "none"}); err == nil {
		t.Fatal("expected evicted CSV token to fail")
	}
	clock = clock.Add(pendingCSVTTL)
	latest, err := service.SelectCSV("accounts", "")
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(pendingCSVTTL)
	if _, err := service.PreviewCSV(CSVOptionsRequest{Token: latest.Token, Profile: "accounts", DateFormat: "iso", DecimalSep: ".", GroupingSep: "none"}); err == nil {
		t.Fatal("expected expired CSV token to fail")
	}
}
