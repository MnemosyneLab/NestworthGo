package einoopenai

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

func TestFakeProviderUsesAuthoritativeReadOnlyPeriodReturn(t *testing.T) {
	app, _, _ := ledgerSession(t)
	members, _ := app.ListMembers(t.Context(), false)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic period broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic period fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	for _, q := range []struct{ date, price string }{{"2026-09-27", "5"}, {"2026-09-28", "6"}} {
		if _, err := app.AppendManualInstrumentQuote(t.Context(), i.ID, q.price, q.date, false); err != nil {
			t.Fatal(err)
		}
	}
	now = now.AddDate(0, 0, -2)
	if _, err := app.CreateHolding(t.Context(), application.HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10", UnitCost: "5"}); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	before, err := app.ListActivities(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := mcpserver.NewInProcessSession(t.Context(), app, t.TempDir(), uuid.NewString(), mcpserver.ReadOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	session := builtinai.New(shared, assistant.PageSelection{})
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		if round == 0 {
			writeMode(w, request, []any{liveCallItem(round, "compare_financial_attribution", obj{"leftAsOf": "2026-09-27", "rightAsOf": "2026-09-28", "scope": obj{"kind": "accounts", "accountIds": []string{a.Account.ID.String()}}})})
			return
		}
		raw := latestToolOutput(t, request)
		var value struct {
			Data struct {
				Content application.FinancialComparisonContent `json:"content"`
			} `json:"data"`
		}
		if jsoncodec.Unmarshal([]byte(raw), &value) != nil {
			t.Error("invalid attribution output")
		}
		link := value.Data.Content.Attribution
		if link == nil || link.Status != "compatible" || link.InvestmentReturn == nil || link.InvestmentReturn.Amount.Value == nil || *link.InvestmentReturn.Amount.Value != "10" {
			t.Errorf("authoritative return did not reach provider: %s", raw)
		}
		writeMode(w, request, []any{textItem()})
	})
	engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	result, err := engine.Complete(context.Background(), nil, "Explain this account's investment return from closed 27 September to closed 28 September; no writes.")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Source != "compare_financial_attribution" {
		t.Fatal("missing attribution host trace")
	}
	after, err := app.ListActivities(t.Context(), 100)
	if err != nil || len(after) != len(before) {
		t.Fatal("model wrote financial facts")
	}
}
