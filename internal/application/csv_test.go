package application

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountsCSVImportSetsEffectiveAtFromValueDate(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "csv-accounts", []string{"Alice", "Bob"})
	clock := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	table, err := csvcodec.Parse([]byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,100.50,2026-08-15,Alice:33.33%;Bob:66.67%\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileAccounts, table, nil, CSVParseOptions{DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Errors) != 0 {
		t.Fatalf("errors: %+v", plan.Errors)
	}
	if len(plan.Batch.Accounts) != 1 || plan.Batch.Accounts[0].Value == nil {
		t.Fatal("expected one account value")
	}
	if plan.Batch.Accounts[0].Value.EffectiveAt.Format("2006-01-02") != "2026-08-15" {
		t.Fatalf("effectiveAt = %s", plan.Batch.Accounts[0].Value.EffectiveAt)
	}
	if plan.Batch.Accounts[0].Value.EffectiveAt.Equal(clock) {
		t.Fatal("effectiveAt was replaced with import time")
	}
	if _, err := service.CommitCSVImport(ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].LatestValue == nil || records[0].LatestValue.EffectiveAt.Format("2006-01-02") != "2026-08-15" {
		t.Fatalf("records = %+v", records)
	}
	_ = bootstrap
}

func TestAccountsCSVRejectsFutureDate(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "csv-future", []string{"Alice"})
	service.setClock(func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) })
	table, err := csvcodec.Parse([]byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-09-02,Alice:100%\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileAccounts, table, nil, CSVParseOptions{DateFormat: CSVDateISO}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Errors) == 0 {
		t.Fatal("expected future date to block")
	}
}

func TestAccountsCSVParsesCommaDecimalOwnership(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "csv-comma-ownership", []string{"Alice", "Bob"})
	// The ownership field is quoted because the CSV delimiter is also a comma.
	table, err := csvcodec.Parse([]byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,\"Alice:33,33%;Bob:66,67%\"\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileAccounts, table, nil, CSVParseOptions{DateFormat: CSVDateISO, DecimalSep: ",", GroupingSep: "."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Errors) != 0 {
		t.Fatalf("errors: %+v", plan.Errors)
	}
}

func TestCSVRejectsConflictingNumberSeparators(t *testing.T) {
	if err := validateCSVParseOptions(CSVParseOptions{DecimalSep: ",", GroupingSep: ","}); err == nil {
		t.Fatal("expected conflicting separators to be rejected")
	}
	if err := validateCSVParseOptions(CSVParseOptions{DecimalSep: ".", GroupingSep: "."}); err == nil {
		t.Fatal("expected conflicting separators to be rejected")
	}
}

func TestHoldingsCSVRequiresEmptyAccountValue(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "csv-holdings", []string{"Alice"})
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Broker", AccountType: string(domain.TypeBrokerage), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingHoldings), DefaultCurrency: "CNY", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	table, err := csvcodec.Parse([]byte("account_name,instrument_type,instrument_name,quantity,quote_currency,unit_price,quote_date\nBroker,etf,QQQ,10,USD,400.12,2026-08-01\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	options := CSVParseOptions{DateFormat: CSVDateISO, Unresolved: []CSVUnresolvedAction{{Kind: "instrument", Name: "QQQ", Action: CSVUnresolvedCreate}}}
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileHoldings, table, nil, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Errors) != 0 {
		t.Fatalf("errors: %+v", plan.Errors)
	}
	if len(plan.Batch.Quotes) != 1 || plan.Batch.Quotes[0].QuotedAt.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("quotes = %+v", plan.Batch.Quotes)
	}
	if _, err := service.CommitCSVImport(ctx, plan); err != nil {
		t.Fatal(err)
	}
}

func TestHoldingsCSVBlocksUnmatchedInstrument(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "csv-unresolved", []string{"Alice"})
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Broker", AccountType: string(domain.TypeBrokerage), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingHoldings), DefaultCurrency: "CNY", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	table, err := csvcodec.Parse([]byte("account_name,instrument_type,instrument_name,quantity,quote_currency,unit_price,quote_date\nBroker,etf,QQQ,10,USD,400.12,2026-08-01\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileHoldings, table, nil, CSVParseOptions{DateFormat: CSVDateISO}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Errors) == 0 {
		t.Fatal("expected unmatched instrument to block")
	}
	if len(plan.Unresolved) == 0 {
		t.Fatal("expected unresolved instrument name")
	}
}

func TestSharedCSVImportBlocksWhenAccountsHaveErrors(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "csv-shared-errors", []string{"Alice"})
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Broker", AccountType: string(domain.TypeBrokerage), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingHoldings), DefaultCurrency: "CNY", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	accountsTable, err := csvcodec.Parse([]byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nBroken,not-a-type,asset,balance,CNY,10,2026-08-01,Alice:100%\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	holdingsTable, err := csvcodec.Parse([]byte("account_name,instrument_type,instrument_name,quantity,quote_currency,unit_price,quote_date\nBroker,etf,QQQ,10,USD,400.12,2026-08-01\n"), csvcodec.DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	options := CSVParseOptions{DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none", Unresolved: []CSVUnresolvedAction{{Kind: "instrument", Name: "QQQ", Action: CSVUnresolvedCreate}}}
	accountsPlan, err := service.BuildCSVImportPlan(ctx, CSVProfileAccounts, accountsTable, nil, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountsPlan.Errors) == 0 {
		t.Fatal("expected accounts errors")
	}
	combined, err := service.BuildCSVImportPlan(ctx, CSVProfileHoldings, holdingsTable, nil, options, &accountsPlan)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Errors) == 0 {
		t.Fatal("expected combined plan to keep accounts errors")
	}
	if _, err := service.CommitCSVImport(ctx, combined); err == nil {
		t.Fatal("expected commit to fail")
	}
	records, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("accounts = %d", len(records))
	}
	holdings, err := service.ListHoldings(ctx, records[0].Account.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(holdings) != 0 {
		t.Fatalf("holdings = %+v", holdings)
	}
}

func TestOwnershipCSVEscapesDelimitersInMemberNames(t *testing.T) {
	name := "Ops\\:East;Desk\\North"
	encoded := escapeOwnershipName(name) + ":100%"
	parts := splitOwnershipParts(encoded)
	if len(parts) != 1 {
		t.Fatalf("parts = %#v", parts)
	}
	colon := ownershipColonIndex(parts[0])
	if colon < 0 || unescapeOwnershipName(parts[0][:colon]) != name {
		t.Fatalf("encoded ownership %q did not decode to %q", encoded, name)
	}
}

func TestExportAccountsCSVUsesHistoryOriginCalendarDate(t *testing.T) {
	singapore, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	origin := &domain.HistoryOrigin{Timezone: "Asia/Singapore"}
	value := time.Date(2026, 9, 1, 0, 0, 0, 0, singapore)
	exported := calendarDate(value, origin)
	if exported != "2026-09-01" {
		t.Fatalf("exported = %s", exported)
	}
	if calendarDate(value, origin) == value.UTC().Format("2006-01-02") {
		t.Fatal("exported UTC date instead of History Origin calendar date")
	}
	observed, err := observationTime(exported, CSVDateISO, origin, value.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, singapore)
	if !observed.Equal(want) {
		t.Fatalf("observed = %s want %s", observed, want)
	}
}

func TestExportAccountsCSVRoundTripSingaporeDate(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "csv-tz.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewService(sqlite.NewRepository(database))
	singapore, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 9, 1, 0, 0, 0, 0, singapore)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}, Timezone: "Asia/Singapore"}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Cash", AccountType: string(domain.TypeBankAccount), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingBalance), DefaultCurrency: "CNY", InitialAmount: "25",
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, IncludeInNetWorth: true,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := service.ExportAccountsCSV(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "2026-09-01") {
		t.Fatalf("expected Singapore calendar date, got %s", data)
	}
	if strings.Contains(string(data), "2026-08-31") {
		t.Fatalf("exported UTC date: %s", data)
	}
}

func TestExportAccountsCSVRoundTrip(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "csv-export", []string{"Alice"})
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Cash", AccountType: string(domain.TypeBankAccount), BalanceSheetRole: string(domain.RoleAsset),
		TrackingMode: string(domain.TrackingBalance), DefaultCurrency: "CNY", InitialAmount: "25",
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, IncludeInNetWorth: true,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := service.ExportAccountsCSV(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("expected BOM")
	}
	if !strings.Contains(string(data), "Cash") {
		t.Fatalf("%s", data)
	}
}

func TestOwnershipBPSAllows3333And6667(t *testing.T) {
	if bps, err := domain.PercentToBasisPoints("33.33%"); err != nil || bps != 3333 {
		t.Fatalf("33.33%% = %d %v", bps, err)
	}
	if bps, err := domain.PercentToBasisPoints("66.67%"); err != nil || bps != 6667 {
		t.Fatalf("66.67%% = %d %v", bps, err)
	}
	if _, err := domain.PercentToBasisPoints("33.333%"); err == nil {
		t.Fatal("expected three decimals to fail")
	}
}
