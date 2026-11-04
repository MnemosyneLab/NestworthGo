package builtinai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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
	if len(definitions) != 12 {
		t.Fatalf("combined discovery lost bounded tools: got %d, want 12", len(definitions))
	}
	outputs := map[string]*jsonschema.Resolved{}
	for _, d := range definitions {
		for _, input := range []bool{true, false} {
			supplied := d.OutputSchema
			if input {
				supplied = d.InputSchema
			}
			if supplied == nil {
				if !input && d.Name == "get_current_page" {
					continue
				}
				t.Fatalf("missing schema: %s input=%v", d.Name, input)
			}
			encoded, e := jsoncodec.Marshal(supplied)
			if e != nil {
				t.Fatal(e)
			}
			var schema jsonschema.Schema
			if e = jsoncodec.Unmarshal(encoded, &schema); e != nil {
				t.Fatal(e)
			}
			resolved, e := schema.Resolve(nil)
			if e != nil {
				t.Fatalf("%s schema: %v", d.Name, e)
			}
			if !input {
				outputs[d.Name] = resolved
			}
		}
	}
	validateOutput := func(name string, raw json.RawMessage) {
		t.Helper()
		var value any
		if e := jsoncodec.Unmarshal(raw, &value); e != nil {
			t.Fatal(e)
		}
		if e := outputs[name].Validate(&value); e != nil {
			t.Fatalf("%s output: %v", name, e)
		}
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
	var current struct {
		Data json.RawMessage `json:"data"`
	}
	if err := jsoncodec.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	validateOutput("list_accounts", current.Data)
	trace := session.Evidence()
	if len(trace) != 1 || trace[0].Source != "list_accounts" || !trace[0].HasMore || trace[0].Missing != "not_summarized" {
		t.Fatalf("bad host trace: %+v", trace)
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
		validateOutput("get_read_page", raw)
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
	if err := shared.RevokeForRestore(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.Definitions(); err == nil {
		t.Fatal("restore retained discovery")
	}
	shared.Close()
	if _, err := detail.Call(t.Context(), "get_current_page", []byte(`{}`)); err == nil {
		t.Fatal("revoked session accepted")
	}
}

type brokenShared struct{ raw json.RawMessage }

func (b brokenShared) Definitions() ([]agentcapability.Definition, error) { return nil, nil }
func (b brokenShared) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return b.raw, nil
}
func TestReadResultOverflowAndMalformedDataFailClosed(t *testing.T) {
	for _, raw := range []string{`not json`, `"` + strings.Repeat("x", 64<<10) + `"`} {
		session := New(brokenShared{raw: []byte(raw)}, assistant.PageSelection{Kind: "overview"})
		if _, err := session.Call(t.Context(), "get_current_page", []byte(`{}`)); err == nil {
			t.Fatal("invalid/oversized result reached model")
		}
		if len(session.Evidence()) != 0 {
			t.Fatal("failed read published evidence")
		}
	}
	session := New(brokenShared{raw: []byte(`{"page":{"nextCursor":"continuation"},"summary":{"complete":false}}`)}, assistant.PageSelection{Kind: "overview"})
	if _, err := session.Call(t.Context(), "get_current_page", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	evidence := session.Evidence()
	if len(evidence) != 1 || !evidence[0].HasMore || evidence[0].Missing != "reported" {
		t.Fatal("cursor/missing metadata lost")
	}
}
