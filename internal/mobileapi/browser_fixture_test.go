package mobileapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Opt-in real-HTTPS browser fixture. All credentials and financial data are
// synthetic and ephemeral; no production listener or settings are enabled.
func TestMobileBrowserFixture(t *testing.T) {
	directory := os.Getenv("NESTWORTH_MOBILE_BROWSER_FIXTURE")
	if directory == "" {
		t.Skip("opt-in isolated browser fixture")
	}
	f := fixture(t, Permissions{Read: true, Record: true, Market: true, Manual: true})
	f.s.assets = os.DirFS("../../frontend/dist")
	b, err := f.s.app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.s.app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic investments", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := f.s.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic manual fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.app.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic USD savings", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	pair, err := f.s.Pair("Synthetic browser QA", Permissions{Read: true, Record: true, Market: true, Manual: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(pair)
	if err != nil {
		t.Fatal(err)
	}
	connection := filepath.Join(directory, "connection.json")
	if err := os.WriteFile(connection, raw, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(connection)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(directory, "done")); err == nil {
				return
			}
		case <-timeout.C:
			t.Fatal("browser fixture timed out")
		}
	}
}
