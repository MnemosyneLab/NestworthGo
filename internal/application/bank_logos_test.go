package application

import (
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestBankIconsSurviveEditsExportAndSnapshotReopen(t *testing.T) {
	t.Parallel()
	s, ctx, b, _ := newOnboardedService(t, "bank-icons", []string{"Owner"})
	institution, err := s.CreateInstitution(ctx, "Test bank", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetInstitutionIcon(ctx, institution.ID, "bank-logo:icbc"); err != nil {
		t.Fatal(err)
	}
	ids := map[string]domain.AccountID{}
	for _, key := range []string{"", "wallet", "bank-logo:boc", "bank-logo:bochk", "bank-logo:standard-chartered", "fund", "bond", "term-deposit", "lending", "land", "commercial-property", "collectible", "digital-asset", "crypto-logo:btc", "crypto-logo:eth", "crypto-logo:sol", "crypto-logo:usdc"} {
		a, err := s.CreateAccount(ctx, AccountInput{Name: "Test " + key, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "0", InstitutionID: institution.ID.String(), IconKey: key, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		ids[key] = a.Account.ID
	}
	assertIcons := func(service *Service) {
		t.Helper()
		for key, id := range ids {
			a, err := service.accountRecord(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if a.Account.IconKey != nil {
				got = *a.Account.IconKey
			}
			if got != key {
				t.Fatalf("stored icon = %q, want %q", got, key)
			}
		}
		institutions, err := service.ListInstitutions(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range institutions {
			if item.ID == institution.ID {
				found = item.IconKey != nil && *item.IconKey == "bank-logo:abchina"
			}
		}
		if !found {
			t.Fatal("institution logo did not persist")
		}
	}
	if err := s.SetInstitutionIcon(ctx, institution.ID, "bank-logo:abchina"); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := s.UpdateAccount(ctx, id, AccountInput{Name: "Renamed"}); err != nil {
			t.Fatal(err)
		}
	}
	assertIcons(s)
	// A logo override can be reset to the existing empty sentinel and saved again.
	id := ids[""]
	if _, err := s.UpdateAccount(ctx, id, AccountInput{IconKey: "bank-logo:boc", IconKeySet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAccount(ctx, id, AccountInput{IconKeySet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAccount(ctx, id, AccountInput{Name: "Inherited"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAccount(ctx, id, AccountInput{IconKey: "bank-logo:unknown", IconKeySet: true}); err == nil {
		t.Fatal("unknown logo accepted")
	}
	assertIcons(s)
	exported := decodeExport(t, s)
	if len(exported.Facts.Directory["accounts"]) != len(ids) {
		t.Fatal("missing exported accounts")
	}
	// Version 2 intentionally excludes UI icons; keep that export contract.
	for _, a := range exported.Facts.Directory["accounts"] {
		if _, ok := a["iconKey"]; ok {
			t.Fatal("JSON business export leaked a UI icon")
		}
	}
	// Use the same verified SQLite snapshot format consumed by backup restore.
	path := filepath.Join(t.TempDir(), "icons.db")
	if err := s.SnapshotTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	verified.Close()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertIcons(NewService(sqlite.NewRepository(db)))
}
