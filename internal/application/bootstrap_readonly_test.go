package application

import (
	"context"
	"testing"
	"time"
)

func TestBootstrapDoesNotRecreateDefaultDirectory(t *testing.T) {
	service, ctx, first, _ := newOnboardedService(t, "bootstrap-readonly", []string{"Alice"})
	if len(first.Institutions) == 0 || len(first.Groups) == 0 {
		t.Fatal("onboarding should create default directory entries")
	}
	second, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Institutions) != len(first.Institutions) || len(second.Groups) != len(first.Groups) {
		t.Fatalf("bootstrap mutated directory: institutions %d->%d groups %d->%d",
			len(first.Institutions), len(second.Institutions), len(first.Groups), len(second.Groups))
	}
	if err := service.ArchiveInstitution(ctx, first.Institutions[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveGroup(ctx, first.Groups[0].ID, true); err != nil {
		t.Fatal(err)
	}
	third, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Institutions) != 0 || len(third.Groups) != 0 {
		t.Fatalf("bootstrap recreated defaults: institutions=%d groups=%d", len(third.Institutions), len(third.Groups))
	}
}

func TestCSVSessionExpiresAndEvicts(t *testing.T) {
	service, _, _, _ := newOnboardedService(t, "csv-ttl", []string{"Alice"})
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	data := []byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,Alice:100%\n")
	first, err := service.SelectCSV(CSVProfileAccounts, "", "accounts.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := service.SelectCSV(CSVProfileAccounts, "", "accounts.csv", data); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := service.SelectCSV(CSVProfileAccounts, "", "accounts.csv", data); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewCSV(context.Background(), CSVPreviewRequest{Token: first.Token, Profile: CSVProfileAccounts, DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none"}); err == nil {
		t.Fatal("expected evicted CSV token to fail")
	}
	clock = clock.Add(CSVImportSessionTTL)
	latest, err := service.SelectCSV(CSVProfileAccounts, "", "accounts.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(CSVImportSessionTTL)
	if _, err := service.PreviewCSV(context.Background(), CSVPreviewRequest{Token: latest.Token, Profile: CSVProfileAccounts, DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none"}); err == nil {
		t.Fatal("expected expired CSV token to fail")
	}
}
