package account_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func onboardedApp(t *testing.T) (*application.Service, []wire.MemberDTO) {
	t.Helper()
	app := wailstest.NewService(t)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice", "Bob"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return app, bootstrap.Members
}

func TestCreateAccountMinimalFields(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	ctx := context.Background()

	record, err := service.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if record.Account.Name != "Bank" || record.Account.DefaultCurrency != "USD" {
		t.Fatalf("record = %+v", record)
	}
	if record.LatestValue == nil || record.LatestValue.Amount.Amount != "1000" {
		t.Fatalf("LatestValue = %+v, want 1000", record.LatestValue)
	}
	if len(record.Ownership) != 1 || record.Ownership[0].ShareBPS != domainTotalOwnershipBPS {
		t.Fatalf("Ownership = %+v, want sole ownership", record.Ownership)
	}
}

const domainTotalOwnershipBPS = 10000

func TestCreateAccountWithExplicitOwnershipPercentages(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	record, err := service.CreateAccount(context.Background(), account.CreateAccountRequest{
		Name: "Joint", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID, members[1].ID}, OwnershipPercentages: []string{"60", "40"},
		InitialAmount: "500",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	total := 0
	for _, share := range record.Ownership {
		total += share.ShareBPS
	}
	if total != domainTotalOwnershipBPS {
		t.Fatalf("total shares = %d, want %d", total, domainTotalOwnershipBPS)
	}
}

func TestCreateAccountValidationError(t *testing.T) {
	app, _ := onboardedApp(t)
	service := account.NewService(app)
	_, err := service.CreateAccount(context.Background(), account.CreateAccountRequest{
		Name: "", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD",
	})
	assertWireCode(t, err, "validation")
}

func TestUpdateAccountSetFlagsPattern(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	ctx := context.Background()

	record, err := service.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000", Note: strPtr("original note"),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	// Update without touching Note: nil means "leave unchanged."
	renamed, err := service.UpdateAccount(ctx, record.Account.ID, account.UpdateAccountRequest{Name: strPtr("Bank Renamed")})
	if err != nil {
		t.Fatalf("UpdateAccount (name only): %v", err)
	}
	if renamed.Account.Name != "Bank Renamed" {
		t.Fatalf("Name = %q, want Bank Renamed", renamed.Account.Name)
	}
	if renamed.Account.Note == nil || *renamed.Account.Note != "original note" {
		t.Fatalf("Note = %v, want preserved \"original note\"", renamed.Account.Note)
	}

	// Now clear the note explicitly: NoteSet=true with Note=nil means "clear."
	cleared, err := service.UpdateAccount(ctx, record.Account.ID, account.UpdateAccountRequest{NoteSet: true, Note: nil})
	if err != nil {
		t.Fatalf("UpdateAccount (clear note): %v", err)
	}
	if cleared.Account.Note != nil {
		t.Fatalf("Note = %v, want nil after explicit clear", cleared.Account.Note)
	}
}

func TestUpdateAccountRejectsTrackingModeChange(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	ctx := context.Background()
	record, err := service.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	_, err = service.UpdateAccount(ctx, record.Account.ID, account.UpdateAccountRequest{TrackingMode: strPtr("manual_value")})
	assertWireCode(t, err, "validation")
}

func TestArchiveAndRestoreAccount(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	ctx := context.Background()
	record, err := service.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := service.ArchiveAccount(ctx, record.Account.ID, true); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	active, err := service.ListAccounts(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active accounts after archive, got %+v", active)
	}
	all, err := service.ListAccounts(ctx, account.AccountFilterRequest{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts(includeArchived): %v", err)
	}
	if len(all) != 1 || all[0].Account.ArchivedAt == nil {
		t.Fatalf("all = %+v, want one archived account", all)
	}
	if err := service.ArchiveAccount(ctx, record.Account.ID, false); err != nil {
		t.Fatalf("restore ArchiveAccount: %v", err)
	}
}

func TestAppendAccountValueAndValuation(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	ctx := context.Background()
	record, err := service.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	value, err := service.AppendAccountValue(ctx, record.Account.ID, "1200.50", "")
	if err != nil {
		t.Fatalf("AppendAccountValue: %v", err)
	}
	if value.Amount.Amount != "1200.5" {
		t.Fatalf("Amount = %q, want 1200.5 (canonical, trailing zero trimmed)", value.Amount.Amount)
	}
	valuation, err := service.AccountValuation(ctx, record.Account.ID)
	if err != nil {
		t.Fatalf("AccountValuation: %v", err)
	}
	if valuation.BaseValue == nil || valuation.BaseValue.Amount != "1200.5" {
		t.Fatalf("BaseValue = %+v, want 1200.5", valuation.BaseValue)
	}
	if !valuation.Complete {
		t.Fatalf("Complete = false, want true for a single-currency balance account")
	}
}

func TestAccountValuationNotFound(t *testing.T) {
	app, _ := onboardedApp(t)
	service := account.NewService(app)
	_, err := service.AccountValuation(context.Background(), "00000000-0000-7000-8000-000000000000")
	if err == nil {
		t.Fatal("want an error for an unknown account")
	}
}

func TestAccountRecordDTORoundTripsAsJSON(t *testing.T) {
	app, members := onboardedApp(t)
	service := account.NewService(app)
	record, err := service.CreateAccount(context.Background(), account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{members[0].ID}, InitialAmount: "1000", Note: strPtr("hello"),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded wire.AccountRecordDTO
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Account.Name != "Bank" || decoded.Account.Note == nil || *decoded.Account.Note != "hello" {
		t.Fatalf("round-tripped record = %+v", decoded)
	}
	if decoded.LatestValue == nil || decoded.LatestValue.Amount.Amount != "1000" {
		t.Fatalf("round-tripped LatestValue = %+v", decoded.LatestValue)
	}
}

func strPtr(value string) *string { return &value }

func assertWireCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error with code %q, got nil", wantCode)
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok {
		t.Fatalf("error is not a parseable WireError: %v", err)
	}
	if wireErr.Code != wantCode {
		t.Fatalf("Code = %q, want %q", wireErr.Code, wantCode)
	}
}
