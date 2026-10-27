package reconciliation

import (
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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

func TestDesktopAdjustmentCanonicalCommandRoundTrip(t *testing.T) {
	h := domain.NewHouseholdID()
	a := domain.NewAccountID()
	held := domain.NewHoldingID()
	at := time.Date(2026, 9, 1, 12, 0, 0, 123000000, time.UTC)
	money, _ := domain.ParseMoney("12.0001", domain.CurrencyCode("USD"))
	quantity, _ := domain.ParseQuantity("0.12345678")
	unit, _ := domain.ParseUnitPrice("1.00000001")
	note := "Original reviewed note"
	plan := adjustmentPlan{id: uuid.NewString(), hash: "original hash", token: "original token", expires: at, commands: []any{
		domain.ValueUpdateInput{HouseholdID: h, AccountID: a, NewValue: money, Reason: domain.ReasonReconciliation, EffectiveAt: at, Note: &note},
		domain.MoneyAddedInput{HouseholdID: h, AccountID: a, Amount: money, Reason: domain.ReasonReconciliation, EffectiveAt: at, Note: &note},
		domain.MoneyRemovedInput{HouseholdID: h, AccountID: a, Amount: money, Reason: domain.ReasonReconciliation, EffectiveAt: at, Note: &note},
		domain.PositionAdjustmentInput{HouseholdID: h, HoldingID: held, Quantity: quantity, Added: true, UnitCost: &unit, EffectiveAt: at, Note: &note},
		domain.PositionCostAdjustmentInput{HouseholdID: h, HoldingID: held, UnitCost: unit, EffectiveAt: at, Note: &note},
	}}
	raw, err := jsoncodec.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var restored adjustmentPlan
	if err = jsoncodec.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	again, err := jsoncodec.Marshal(restored)
	if err != nil || string(raw) != string(again) {
		t.Fatalf("canonical recovery changed %s -> %s (%v)", raw, again, err)
	}
	if len(restored.commands) != 5 || restored.commands[3].(domain.PositionAdjustmentInput).Quantity.Canonical() != "0.12345678" || restored.commands[4].(domain.PositionCostAdjustmentInput).UnitCost.Canonical() != "1.00000001" {
		t.Fatal("financial precision lost")
	}
}

func TestDesktopAdjustmentCostErrorsNameTheReviewedField(t *testing.T) {
	for _, field := range []string{"unitCost", "totalCost"} {
		input := AdjustmentRequest{HoldingID: uuid.NewString(), Quantity: "2", Currency: "USD"}
		if field == "unitCost" {
			input.UnitCost = "invalid"
		} else {
			input.TotalCost = "invalid"
		}
		_, err := NewService(nil).PreviewAdjustment(t.Context(), input)
		if err == nil {
			t.Fatal("invalid cost accepted")
		}
		wire, ok := apierror.Parse(err.Error())
		if !ok || wire.Field != field {
			t.Fatalf("%s: %v", field, err)
		}
	}
}

func TestDesktopReconciliationOptionalReference(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewService(sqlite.NewRepository(db))
	app.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	if err = app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Tester"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := app.Bootstrap(ctx)
	account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(app)
	preview, err := service.Preview(ctx, application.ReconciliationAction{Action: "start", AccountIDs: []string{account.Account.ID.String()}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.Commit(ctx, preview.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	// Decode an API request with reference omitted; keep the persisted source shape.
	var action application.ReconciliationAction
	if err = jsoncodec.Unmarshal([]byte(`{"action":"check","itemId":"scope","outcome":"confirmed","source":{"asOfDate":"2026-08-31"}}`), &action); err != nil {
		t.Fatal(err)
	}
	action.SessionID, action.AccountID = session.ID, account.Account.ID.String()
	action.ExpectedRevision, action.FinancialRevision = session.Revision, session.Accounts[0].FinancialRevision
	preview, err = service.Preview(ctx, action)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := NewService(app).Commit(ctx, preview.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Counts.Confirmed != 1 {
		t.Fatalf("check not confirmed: %+v", saved.Counts)
	}
	for _, item := range saved.Accounts[0].Items {
		if item.ID == "scope" && (item.Outcome != "confirmed" || item.Source != (domain.ReconciliationSource{AsOfDate: "2026-08-31"})) {
			t.Fatalf("changed evidence: %+v", item)
		}
	}
}
