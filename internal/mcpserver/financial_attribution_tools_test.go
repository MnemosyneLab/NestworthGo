package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestFinancialAttributionHTTPFreshCapturePermissionsAndFrozenPages(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app, accountID, _ := historyFixture(t)
			addDividendAttribution(t, app, "private broker", "5")
			addDividendAttribution(t, app, "other broker", "7")
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			instructions := client.InitializeResult().Instructions
			for _, phrase := range []string{"call compare_financial_attribution directly without get_context", "A+1 through B", "new coherent comparisonId", "may maintain derived snapshots and invalidate ledger previews", "even in read_only mode"} {
				if !strings.Contains(instructions, phrase) {
					t.Fatal("missing attribution route/side-effect instruction", phrase)
				}
			}
			listed, err := client.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range listed.Tools {
				if tool.Name == "compare_financial_attribution" {
					found = true
					if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
						t.Fatal("incorrect derived-write annotation")
					}
				}
			}
			if !found {
				t.Fatal("tool not available")
			}
			values := map[string]string{"leftDate": "2025-03-02", "rightDate": "2025-03-03", "accountId": accountID}
			args := skillExample(t, "analysis", "financial-attribution", values)
			old := decodeComparison(t, call(t, client, "compare_financial_context", args, false))
			if _, err := history.NewService(app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: accountID, Amount: "10", Currency: "USD", Reason: "income", EffectiveAt: "2025-03-03T13:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			initial := decodeComparison(t, call(t, client, "compare_financial_attribution", args, false))
			link := initial.Content.Attribution
			if initial.ComparisonID == old.ComparisonID || old.Content.Attribution != nil || link == nil || link.Status != "compatible" || link.Period.From != "2025-03-03" || link.Period.To != "2025-03-03" {
				t.Fatal(initial)
			}
			if *initial.Content.Change.NetWorth.Value != "-3" || *old.Content.Change.NetWorth.Value != "-13" || *link.InvestmentReturn.Amount.Value != "12" {
				t.Fatal("change/return conflation", initial, old)
			}
			direct, err := analysis.NewService(app).AssetChange(t.Context(), analysis.AnalysisQueryRequest{From: link.Period.From, To: link.Period.To})
			if err != nil || direct.Summary.Change.Amount != *initial.Content.Change.NetWorth.Value {
				t.Fatal("adapter mismatch", direct, err)
			}
			directReturn, err := analysis.NewService(app).ReturnTrend(t.Context(), analysis.AnalysisQueryRequest{From: link.Period.From, To: link.Period.To}, "period_return_amount")
			if err != nil || directReturn.Amount == nil || directReturn.Amount.Amount != *link.InvestmentReturn.Amount.Value || directReturn.Status != link.InvestmentReturn.Status || directReturn.RatedDays != link.InvestmentReturn.RatedDays || directReturn.TotalDays != link.InvestmentReturn.TotalDays {
				t.Fatal("return adapter mismatch", directReturn, err)
			}
			// Three independent cursors, retaining only the requested section's state.
			states, rows := skillPageState(t, initial)
			original, _ := json.Marshal(link)
			if _, err := app.AppendAccountValue(t.Context(), domain.AccountID(accountID), "999", ""); err != nil {
				t.Fatal(err)
			}
			for _, section := range skillSections {
				info := states[section]
				for info.HasMore {
					page := decodeComparison(t, call(t, client, "get_financial_comparison_page", FinancialComparisonPageInput{ComparisonID: initial.ComparisonID, Section: section, Cursor: info.NextCursor, Limit: 1}, false))
					raw, _ := json.Marshal(page.Content.Attribution)
					if string(raw) != string(original) || page.ContentHash != initial.ContentHash || page.CacheExpiresAt != initial.CacheExpiresAt || page.CapturedAt != initial.CapturedAt {
						t.Fatal("link not frozen")
					}
					size, err := financialContextWireSize(page)
					if err != nil || size > contextWireLimit {
						t.Fatal("wire budget", size, err)
					}
					next, part := skillPageState(t, page)
					info = next[section]
					rows[section] = append(rows[section], part[section]...)
				}
				if len(rows[section]) != states[section].Total {
					t.Fatal("pagination lost rows", section)
				}
			}
			// Old package remains a strictly frozen value comparison after recapture.
			page := decodeComparison(t, call(t, client, "get_financial_comparison_page", FinancialComparisonPageInput{ComparisonID: old.ComparisonID, Section: "positions", Cursor: old.PositionsPage.NextCursor}, false))
			if page.Content.Attribution != nil || page.ContentHash != old.ContentHash || *page.Content.Change.NetWorth.Value != "-13" {
				t.Fatal("old cache rewritten")
			}
			oldArgs := skillExample(t, "analysis", "financial-attribution", values)
			oldArgs["comparisonId"] = old.ComparisonID
			call(t, client, "compare_financial_attribution", oldArgs, true)
			single := decodeComparison(t, call(t, client, "compare_financial_attribution", skillExample(t, "analysis", "financial-attribution-account", values), false))
			if single.Content.Attribution.Status != "compatible" || single.Content.Attribution.Scope.AccountCount != 1 || *single.Content.Change.NetWorth.Value != "-15" {
				t.Fatal("single account mismatch", single)
			}
			raw, _ := json.Marshal(single.Content)
			for _, secret := range []string{accountID, "Checking", "private broker", "other broker"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("minimal leak", secret)
				}
			}
			expired, _ := time.Parse(time.RFC3339Nano, initial.CacheExpiresAt)
			s.contexts.mu.Lock()
			s.contexts.now = func() time.Time { return expired }
			s.contexts.mu.Unlock()
			call(t, client, "get_financial_comparison_page", FinancialComparisonPageInput{ComparisonID: initial.ComparisonID, Section: "positions", Cursor: initial.PositionsPage.NextCursor}, true)
			restarted := decodeComparison(t, call(t, client, "compare_financial_attribution", args, false))
			if restarted.ComparisonID == initial.ComparisonID {
				t.Fatal("expired package reused")
			}
			call(t, client, "get_financial_comparison_page", FinancialComparisonPageInput{ComparisonID: restarted.ComparisonID, Section: "positions", Cursor: initial.PositionsPage.NextCursor}, true)
		})
	}
}

func TestFinancialAttributionHTTPPrecisionBackfillAndFrozenPages(t *testing.T) {
	s, app, _ := fixture(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	members, err := app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Private selected", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Private unselected", AccountType: "property", BalanceSheetRole: "asset", TrackingMode: "manual_value", DefaultCurrency: "USD", InitialAmount: "99", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, quote := range []struct{ date, fx string }{{"2026-08-01", "7.123456"}, {"2026-08-02", "7.234567"}} {
		if _, err := app.AppendManualFXQuote(t.Context(), "CNY", "USD", quote.fx, quote.date); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.SetFXPreference(t.Context(), "CNY", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	args := skillExample(t, "analysis", "financial-attribution-account", map[string]string{"leftDate": "2026-08-10", "rightDate": "2026-08-11", "accountId": selected.Account.ID.String()})
	later := decodeComparison(t, call(t, client, "compare_financial_attribution", args, false))
	if later.Content.Attribution.Status != "compatible" {
		t.Fatal(later)
	}
	args["leftAsOf"], args["rightAsOf"] = "2026-08-01", "2026-08-02"
	old := decodeComparison(t, call(t, client, "compare_financial_context", args, false))
	initial := decodeComparison(t, call(t, client, "compare_financial_attribution", args, false))
	link := initial.Content.Attribution
	if link.Status != "compatible" || link.Precision.AmountScale != 4 || link.Precision.Rounding != "half_even" || *link.AnalysisDelta.Value != "0.1111" || *link.ExplainedDelta.Value != "0.1111" || *link.PrecisionAdjustment.Value != "0.000011" || *initial.Content.Change.NetWorth.Value != "0.111111" || old.Content.Attribution != nil {
		t.Fatal(initial, old)
	}
	if _, err := app.AppendManualFXQuote(t.Context(), "CNY", "USD", "7.345678", "2026-08-02"); err != nil {
		t.Fatal(err)
	}
	revised := decodeComparison(t, call(t, client, "compare_financial_attribution", args, false))
	if revised.ComparisonID == initial.ComparisonID || revised.ContentHash == initial.ContentHash || *revised.Content.Change.NetWorth.Value != "0.222222" || *revised.Content.Attribution.PrecisionAdjustment.Value != "0.000022" {
		t.Fatal(revised)
	}
	states, _ := skillPageState(t, initial)
	originalLink, _ := json.Marshal(link)
	for _, section := range skillSections {
		info := states[section]
		for info.HasMore {
			page := decodeComparison(t, call(t, client, "get_financial_comparison_page", FinancialComparisonPageInput{ComparisonID: initial.ComparisonID, Section: section, Cursor: info.NextCursor, Limit: 1}, false))
			rawLink, _ := json.Marshal(page.Content.Attribution)
			if string(rawLink) != string(originalLink) || page.ContentHash != initial.ContentHash || page.CapturedAt != initial.CapturedAt || page.CacheExpiresAt != initial.CacheExpiresAt {
				t.Fatal("precision summary thawed", page)
			}
			raw, _ := json.Marshal(page)
			for _, secret := range []string{"Private", selected.Account.ID.String(), other.Account.ID.String()} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("scope/privacy leak", secret)
				}
			}
			size, err := financialContextWireSize(page)
			if err != nil || size > contextWireLimit {
				t.Fatal(size, err)
			}
			next, _ := skillPageState(t, page)
			info = next[section]
		}
	}
}

type attributionCaptureRepository struct {
	application.Repository
	port              application.FinancialContextRepository
	calls             atomic.Int32
	captured, release chan struct{}
}

func (r *attributionCaptureRepository) ReadFinancialContextInputs(ctx context.Context, historical bool, ids []domain.AccountID, now time.Time) (application.FinancialContextInputs, error) {
	input, err := r.port.ReadFinancialContextInputs(ctx, historical, ids, now)
	if r.calls.Add(1) == 2 {
		close(r.captured)
		select {
		case <-r.release:
		case <-ctx.Done():
			return application.FinancialContextInputs{}, ctx.Err()
		}
	}
	return input, err
}
func TestFinancialAttributionRevocationPreventsPublicationAfterMaintenance(t *testing.T) {
	for _, action := range []string{"disable", "enable", "close", "restore"} {
		t.Run(action, func(t *testing.T) {
			db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := sqlite.NewRepository(db)
			port := &attributionCaptureRepository{Repository: repo, port: repo, captured: make(chan struct{}), release: make(chan struct{})}
			app := application.NewService(port)
			if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			s := New(app, t.TempDir(), nil)
			defer s.Close()
			comparisonFixture(t, s, app)
			if _, err := s.Enable(ReadOnly); err != nil {
				t.Fatal(err)
			}
			g := s.contextGeneration
			done := make(chan error, 1)
			go func() {
				_, err := s.buildFinancialAttribution(context.Background(), g, application.FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02"})
				done <- err
			}()
			<-port.captured
			switch action {
			case "disable":
				_, err = s.Disable()
			case "enable":
				_, err = s.Enable(DirectoryWrite)
			case "close":
				s.Close()
			case "restore":
				err = s.RevokeForRestore(t.Context())
			}
			if err != nil {
				t.Fatal(err)
			}
			close(port.release)
			if err := <-done; err == nil {
				t.Fatal("revoked attribution republished")
			}
			s.contexts.mu.Lock()
			count := len(s.contexts.entries)
			s.contexts.mu.Unlock()
			if count != 0 {
				t.Fatal("cache repopulated after revocation")
			}
		})
	}
}

func TestFinancialAttributionWireSummaryAndMalformedRequestPrivacy(t *testing.T) {
	c := newFinancialContextCache()
	g := c.activate()
	result := application.FinancialComparisonResult{Content: application.FinancialComparisonContent{Attribution: &application.FinancialAttributionLink{Basis: strings.Repeat("\\\"", contextWireLimit)}}}
	if _, err := c.comparisonResponse("id", cachedFinancialContext{comparison: &result}, g, "", 0, 1); err == nil {
		t.Fatal("oversized attribution summary")
	}
	s, app, _ := fixture(t)
	comparisonFixture(t, s, app)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"leftAsOf":{"private_account_note":"sensitive-value"},"rightAsOf":"2026-08-02"}`, `{"leftAsOf":"2026-08-01","rightAsOf":"2026-08-02","private_account_note":"sensitive-value"}`, `{"leftAsOf":"2026-08-01","rightAsOf":"2026-08-02","scope":{"kind":"accounts","accountIds":["sensitive-value"]}}`} {
		payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"compare_financial_attribution","arguments":` + args + `}}`
		req, _ := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if len(body) > contextWireLimit || strings.Contains(string(body), "private_account_note") || strings.Contains(string(body), "sensitive-value") {
			t.Fatalf("request echoed: %s", body)
		}
	}
}
