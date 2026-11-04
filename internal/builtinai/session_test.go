package builtinai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

func TestSelectedPageSharedPermissionPagingAndBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	app := application.NewService(repo)
	appports.Wire(app)
	app.SetLiveDatabasePath(path)
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic AI bounds", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: "UTC", HistoryStartDate: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	members, _ := app.ListMembers(t.Context(), false)
	var accountID string
	for i := range 27 {
		a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: fmt.Sprintf("Synthetic %02d", i), AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "1.23", OwnerIDs: []domain.MemberID{members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		accountID = a.Account.ID.String()
	}
	shared, err := mcpserver.NewInProcessSession(t.Context(), app, dir, uuid.NewString(), mcpserver.ReadOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	session := New(shared, assistant.PageSelection{Kind: "accounts"})
	definitions, err := session.Definitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range definitions {
		if !allowed(d.Name) && d.Name != "get_current_page" {
			t.Fatal("extra tool exposed")
		}
	}
	for _, name := range []string{"commit_change", "preview_change", "get_context", "get_market_data", "analyze_period", "start_data_repair"} {
		if _, err := session.Call(t.Context(), name, []byte(`{}`)); err == nil {
			t.Fatalf("extra authority: %s", name)
		}
	}
	raw, err := session.Call(t.Context(), "get_current_page", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Source     string `json:"source"`
		CapturedAt string `json:"capturedAt"`
		Data       struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if jsoncodec.Unmarshal(raw, &view) != nil || view.Source != "list_accounts" || view.CapturedAt == "" {
		t.Fatalf("missing backend provenance: %s", raw)
	}
	capture := view.Data.Data
	if capture == nil {
		t.Fatalf("missing capture: %s", raw)
	}
	sections := capture["sections"].(map[string]any)
	section := sections["accounts"].(map[string]any)
	count := len(section["items"].([]any))
	page := section["page"].(map[string]any)
	if count >= 27 || page["hasMore"] != true {
		t.Fatal("initial page falsely complete")
	}
	for page["hasMore"] == true {
		args, _ := jsoncodec.Marshal(map[string]any{"captureId": capture["captureId"], "section": "accounts", "cursor": page["nextCursor"], "limit": 1})
		raw, err = session.Call(t.Context(), "get_read_page", args)
		if err != nil || len(raw) > 64<<10 {
			t.Fatal("page/budget failed", err)
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if jsoncodec.Unmarshal(raw, &envelope) != nil {
			t.Fatal("invalid page")
		}
		count += len(envelope.Data["items"].([]any))
		page = envelope.Data["page"].(map[string]any)
	}
	if count != 27 {
		t.Fatalf("lost rows: %d", count)
	}
	for _, args := range []string{`{"permission":"ledger_write"}`, `null`, `{"route":"settings"}`} {
		if _, err := session.Call(t.Context(), "get_current_page", []byte(args)); err == nil {
			t.Fatal("client granted page authority")
		}
	}
	detail := New(shared, assistant.PageSelection{Kind: "account", AccountID: accountID})
	raw, err = detail.Call(t.Context(), "get_current_page", []byte(`{}`))
	if err != nil || !strings.Contains(string(raw), "get_account_snapshot") {
		t.Fatal("account detail failed", err)
	}
	unknown := New(shared, assistant.PageSelection{Kind: "account", AccountID: uuid.NewString()})
	if _, err := unknown.Call(t.Context(), "get_current_page", []byte(`{}`)); err == nil {
		t.Fatal("nonexistent account accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := detail.Call(ctx, "get_current_page", []byte(`{}`)); err == nil {
		t.Fatal("cancelled read dispatched")
	}
	shared.Close()
	if _, err := detail.Call(t.Context(), "get_current_page", []byte(`{}`)); err == nil {
		t.Fatal("revoked session accepted")
	}
}
