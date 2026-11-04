package mobileapi

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestAssistantAccountNativeDisplayIndependentOfMissingHouseholdFX(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	b, err := f.s.app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.app.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	account, err := f.s.app.CreateAccount(t.Context(), application.AccountInput{
		Name: "Synthetic USD with no household FX", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "123.4567", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, raw := f.request(t, "GET", "/accounts/"+account.Account.ID.String(), nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("account read %d %s", response.StatusCode, raw)
	}
	var envelope struct {
		Data wire.AccountValuationDTO `json:"data"`
	}
	if err := jsoncodec.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope.Data
	if got.Complete || got.BaseValue != nil || len(got.MissingInputs) == 0 {
		t.Fatalf("expected missing household FX: %+v", got)
	}
	if got.DisplayValue == nil || !got.DisplayValue.Complete || got.DisplayValue.Value == nil || got.DisplayValue.Value.Amount != "123.4567" || got.DisplayValue.Value.Currency != "USD" || len(got.DisplayValue.MissingInputs) != 0 {
		t.Fatalf("native projection must remain complete and exact: %+v", got.DisplayValue)
	}
}

func TestAssistantStatusUsesLiveMobileReadBoundary(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	response, raw := f.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"surface":"mobile"`)) || !bytes.Contains(raw, []byte(`"financialWrites":false`)) || !bytes.Contains(raw, []byte(`"modelRuns":false`)) {
		t.Fatalf("assistant status %d %s", response.StatusCode, raw)
	}
	for _, secret := range []string{f.token, f.csrf, f.s.config.Token, "Synthetic bank"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("assistant status disclosed a secret or financial data")
		}
	}
	for _, route := range []string{"/assistant/run", "/assistant/preview", "/assistant/confirm", "/assistant/credentials"} {
		response, _ = f.request(t, "POST", route, map[string]string{}, nil)
		if response.StatusCode != 404 {
			t.Fatalf("unsupported assistant route enabled: %s %d", route, response.StatusCode)
		}
	}
	f.s.Revoke("")
	response, _ = f.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != 401 {
		t.Fatalf("revoked assistant status: %d", response.StatusCode)
	}
	ro := fixture(t, Permissions{Read: false, Record: true})
	response, _ = ro.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != 403 {
		t.Fatalf("record permission must not imply assistant read: %d", response.StatusCode)
	}
}
