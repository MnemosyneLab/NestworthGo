package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func decodeComparison(t *testing.T, obj map[string]any) FinancialComparisonResponse {
	t.Helper()
	raw, _ := jsoncodec.Marshal(financialPageTestData(t, obj["data"]))
	var r FinancialComparisonResponse
	if err := jsoncodec.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func comparisonFixture(t *testing.T, s *Service, app *application.Service) domain.AccountID {
	t.Helper()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	members, err := app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	var first domain.AccountID
	for i := 0; i < 4; i++ {
		a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Secret", AccountType: "property", BalanceSheetRole: "asset", TrackingMode: "manual_value", DefaultCurrency: "USD", InitialAmount: "10", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		first = a.Account.ID
	}
	if _, err = app.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	return first
}
func TestFinancialComparisonHTTPFrozenPaginationAndPermissions(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app, changes := fixture(t)
			first := comparisonFixture(t, s, app)
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			initial := decodeComparison(t, call(t, client, "compare_financial_context", map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "current", "limit": 1}, false))
			if initial.ComparisonID == "" || initial.Content.Change.NetWorth.Value == nil || *initial.Content.Change.NetWorth.Value != "0" {
				t.Fatal(initial)
			}
			if _, err := app.AppendAccountValue(t.Context(), first, "20", ""); err != nil {
				t.Fatal(err)
			}
			for _, section := range []string{"positions", "gaps", "evidence"} {
				info := initial.PositionsPage
				if section == "gaps" {
					info = initial.GapsPage
				}
				if section == "evidence" {
					info = initial.EvidencePage
				}
				total, count := info.Total, info.Returned
				_, rows := skillPageState(t, initial)
				got := rows[section]
				for info.HasMore {
					page := decodeComparison(t, call(t, client, "get_read_page", ReadPageInput{Cursor: info.NextCursor, Limit: 2}, false))
					if page.ComparisonID != initial.ComparisonID || page.CapturedAt != initial.CapturedAt || page.Content.Change.NetWorth.Value != nil {
						t.Fatal(page)
					}
					_, nextRows := skillPageState(t, page)
					got = append(got, nextRows[section]...)
					info = page.PositionsPage
					if section == "gaps" {
						info = page.GapsPage
					}
					if section == "evidence" {
						info = page.EvidencePage
					}
					if info.Returned == 0 {
						t.Fatal("no progress")
					}
					count += info.Returned
					size, err := financialContextWireSize(page)
					if err != nil || size > contextWireLimit {
						t.Fatal(size, err)
					}
				}
				if count != total {
					t.Fatal(count, total)
				}
				full, err := readSummary(s.contexts.entries[initial.ComparisonID].comparison.Content)
				if err != nil {
					t.Fatal(err)
				}
				productJSONEqual(t, got, full[section])
			}
			call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: initial.ComparisonID, Ref: initial.Content.Positions[0].Ref}, true)
			single := decodeContext(t, call(t, client, "get_financial_context", map[string]any{}, false))
			// The authenticated cursor selects the context/comparison family, so a
			// cursor can never be replayed under another package's identity.
			call(t, client, "get_read_page", ReadPageInput{CaptureID: single.ContextID, Section: "positions", Cursor: initial.PositionsPage.NextCursor}, true)
			call(t, client, "get_read_page", ReadPageInput{CaptureID: initial.ComparisonID, Section: "positions", Cursor: single.PositionsPage.NextCursor}, true)
			other := decodeComparison(t, call(t, client, "compare_financial_context", map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "current"}, false))
			call(t, client, "get_read_page", ReadPageInput{CaptureID: other.ComparisonID, Section: "positions", Cursor: initial.PositionsPage.NextCursor}, true)
			if changes.Load() != 0 {
				t.Fatal("write event")
			}
		})
	}
}
func TestFinancialComparisonCacheBoundsRevocationAndAdmission(t *testing.T) {
	t.Parallel()
	c := newFinancialContextCache()
	g := c.activate()
	now := time.Now()
	c.now = func() time.Time { return now }
	s := &Service{contexts: c}
	result := application.FinancialComparisonResult{CapturedAt: now, ContentHash: "hash", Content: application.FinancialComparisonContent{Positions: []application.FinancialComparisonPosition{{Ref: "a"}, {Ref: "b"}}}}
	entry := cachedFinancialContext{comparison: &result, expires: now.Add(contextTTL), size: 100}
	c.entries[financialFixtureID] = entry
	c.bytes = 100
	cursor := c.cursor(financialFixtureID, "positions", g, 1)
	for _, in := range []FinancialComparisonPageInput{{ComparisonID: financialFixtureID, Section: "gaps", Cursor: cursor}, {ComparisonID: financialFixtureID, Section: "positions", Cursor: cursor + "x"}, {ComparisonID: financialFixtureID, Section: "positions", Cursor: c.cursor(financialFixtureID, "positions", g+1, 1)}, {ComparisonID: financialFixtureID, Section: "positions", Cursor: c.cursor(financialFixtureID, "positions", g, 9)}} {
		if _, err := s.financialComparisonPage(t.Context(), g, in); err == nil {
			t.Fatal("accepted invalid cursor")
		}
	}
	now = entry.expires
	if _, err := s.financialComparisonPage(t.Context(), g, FinancialComparisonPageInput{ComparisonID: financialFixtureID, Section: "positions", Cursor: cursor}); err == nil {
		t.Fatal("expired served")
	}
	result.Content.Positions[1].Left = &application.FinancialContextPosition{Name: strings.Repeat("\"\\", 40000)}
	if _, err := c.comparisonSectionResponse(financialFixtureID, entry, g, "positions", 1, 50); err == nil {
		t.Fatal("oversized row")
	}
	result.Content.Left.Scope.AccountRefs = []string{strings.Repeat("x", contextWireLimit)}
	if _, err := c.comparisonResponse(financialFixtureID, entry, g, FinancialInitialPageInput{Sections: financialSections, Limit: 25}); err == nil {
		t.Fatal("oversized summary")
	}
	c.entries[financialFixtureID] = entry
	c.bytes = 100
	if err := s.RevokeForRestore(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.financialComparisonPage(t.Context(), g, FinancialComparisonPageInput{ComparisonID: financialFixtureID, Section: "positions", Cursor: cursor}); err == nil {
		t.Fatal("revoked served")
	}
	g = c.activate()
	c.builds <- struct{}{}
	c.builds <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.buildFinancialComparison(ctx, g, FinancialComparisonInput{FinancialComparisonRequest: application.FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "current"}, FinancialInitialPageInput: FinancialInitialPageInput{Limit: 1}}); err == nil {
		t.Fatal("cancelled admission")
	}
	<-c.builds
	<-c.builds
}

func TestFinancialComparisonRevocationPreventsInflightPublication(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"disable", "enable", "close", "restore"} {
		t.Run(action, func(t *testing.T) {
			db, err := sqlite.Open(t.TempDir() + "/context.db")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := sqlite.NewRepository(db)
			port := &blockingContextRepository{Repository: repo, port: repo, captured: make(chan struct{}), release: make(chan struct{})}
			app := application.NewService(port)
			if err = app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
			app.SetClock(func() time.Time { return now })
			if _, err = app.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			now = now.AddDate(0, 0, 2)
			s := New(app, t.TempDir(), nil)
			defer s.Close()
			if _, err = s.Enable(ReadOnly); err != nil {
				t.Fatal(err)
			}
			g := s.contextGeneration
			done := make(chan error, 1)
			go func() {
				_, err := s.buildFinancialComparison(context.Background(), g, FinancialComparisonInput{FinancialComparisonRequest: application.FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "current"}, FinancialInitialPageInput: FinancialInitialPageInput{Limit: 1}})
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
				s.operationMu.Lock()
				err = s.RevokeForRestore(t.Context())
				s.operationMu.Unlock()
			}
			if err != nil {
				t.Fatal(err)
			}
			close(port.release)
			if err = <-done; err == nil {
				t.Fatal("revoked build published")
			}
			s.contexts.mu.Lock()
			count := len(s.contexts.entries)
			s.contexts.mu.Unlock()
			if count != 0 {
				t.Fatal("revoked cache repopulated")
			}
		})
	}
}
