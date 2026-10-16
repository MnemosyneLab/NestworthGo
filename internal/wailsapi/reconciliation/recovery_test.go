package reconciliation

import (
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopReconciliationDurableIdentityAndReceiptWithoutBody(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	app := application.NewService(sqlite.NewRepository(db))
	app.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	if e = app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Tester"}}); e != nil {
		t.Fatal(e)
	}
	b, _ := app.Bootstrap(ctx)
	a, e := app.CreateAccount(ctx, application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = app.StartHistory(ctx, "UTC"); e != nil {
		t.Fatal(e)
	}
	service := NewService(app)
	p, e := service.Preview(ctx, application.ReconciliationAction{Action: "start", AccountIDs: []string{a.Account.ID.String()}, Note: "Opening note"})
	if e != nil {
		t.Fatal(e)
	}
	// Drop the adapter and application, then read the same registered identity.
	app = application.NewService(sqlite.NewRepository(db))
	app.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	service = NewService(app)
	v, e := service.Commit(ctx, p.PlanID)
	if e != nil {
		t.Fatal(e)
	}
	if v.ID != p.PlanID || v.Note != "Opening note" {
		t.Fatalf("unstable identity %+v", v)
	}
	if _, e = db.SQL.Exec("DELETE FROM app_configuration WHERE key LIKE 'reconciliation.desktop.%'"); e != nil {
		t.Fatal(e)
	}
	again, e := NewService(app).Commit(ctx, p.PlanID)
	if e != nil || again.ID != v.ID {
		t.Fatalf("missing-plan receipt %+v %v", again, e)
	}
	before, _ := app.ListReconciliationSessions(ctx)
	if _, e = service.Commit(ctx, uuid.NewString()); e == nil {
		t.Fatal("unknown ID executed")
	} else if wire, ok := apierror.Parse(e.Error()); !ok || wire.Field != "planId" {
		t.Fatalf("unknown outcome misclassified %v", e)
	}
	after, _ := app.ListReconciliationSessions(ctx)
	if len(before) != len(after) {
		t.Fatal("unknown ID wrote a session")
	}
	link := domain.ReconciliationLink{SessionID: v.ID, AccountID: a.Account.ID.String(), ItemID: "balance", ExpectedRevision: v.Revision, FinancialRevision: v.Accounts[0].FinancialRevision}
	adjustment, e := service.PreviewAdjustment(ctx, AdjustmentRequest{Link: link, AccountID: a.Account.ID.String(), Balance: "120.0001", Currency: "USD", Note: "Exact synthetic adjustment"})
	if e != nil {
		t.Fatal(e)
	}
	committed, e := NewService(app).CommitAdjustment(ctx, adjustment.PlanID)
	if e != nil {
		t.Fatal(e)
	}
	if len(committed) != 1 || committed[0].Resulting[0].Amount != "120.0001" {
		t.Fatalf("canonical command lost: %+v", committed)
	}
	if _, e = db.SQL.Exec("DELETE FROM app_configuration WHERE key LIKE 'reconciliation.desktop.%'"); e != nil {
		t.Fatal(e)
	}
	replay, e := NewService(app).CommitAdjustment(ctx, adjustment.PlanID)
	if e != nil {
		t.Fatal(e)
	}
	if replay[0].Activity.ID != committed[0].Activity.ID || replay[0].Resulting[0].Amount != "120.0001" || replay[0].Effects[0].Money.Amount != "20.0001" {
		t.Fatalf("economic receipt changed: %+v", replay)
	}
}
