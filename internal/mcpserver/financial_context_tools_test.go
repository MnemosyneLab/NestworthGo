package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func decodeContext(t *testing.T, obj map[string]any) FinancialContextResponse {
	t.Helper()
	raw, _ := json.Marshal(obj["data"])
	var r FinancialContextResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestFinancialContextHTTPFrozenPaginationAndPermissions(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app, changes := fixture(t)
			members, err := app.ListMembers(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			var first domain.AccountID
			for i := 0; i < 5; i++ {
				a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: strings.Repeat("Untrusted name", 5), AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "10", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
				if err != nil {
					t.Fatal(err)
				}
				first = a.Account.ID
			}
			if _, err = s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			initial := decodeContext(t, call(t, client, "get_financial_context", map[string]any{}, false))
			if initial.PositionsPage.Returned == 0 || !initial.PositionsPage.HasMore || initial.Content.Summary.NetWorth.Value == nil || *initial.Content.Summary.NetWorth.Value != "50" {
				t.Fatal(initial)
			}
			if _, err = app.AppendAccountValue(t.Context(), first, "20", ""); err != nil {
				t.Fatal(err)
			}
			cursor := initial.PositionsPage.NextCursor
			count := initial.PositionsPage.Returned
			for cursor != "" {
				page := decodeContext(t, call(t, client, "get_financial_context_page", FinancialContextPageInput{ContextID: initial.ContextID, Section: "positions", Cursor: cursor, Limit: 2}, false))
				if page.ContentHash != initial.ContentHash || page.CacheExpiresAt != initial.CacheExpiresAt || page.PositionsPage.Returned == 0 || *page.Content.Summary.NetWorth.Value != "50" {
					t.Fatal(page)
				}
				count += page.PositionsPage.Returned
				cursor = page.PositionsPage.NextCursor
			}
			if count != initial.PositionsPage.Total {
				t.Fatal(count, initial.PositionsPage)
			}
			if changes.Load() != 0 {
				t.Fatal("context emitted write event")
			}
			call(t, client, "get_financial_context_page", FinancialContextPageInput{ContextID: initial.ContextID, Section: "gaps", Cursor: initial.PositionsPage.NextCursor}, true)
			call(t, client, "get_financial_context", map[string]any{"unexpected": true}, true)
		})
	}
}
func TestFinancialContextFinalHTTPWireBound(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{`1`, `"` + strings.Repeat("x", 500) + `"`} {
		payload := `{"jsonrpc":"2.0","id":` + id + `,"method":"tools/call","params":{"name":"get_financial_context","arguments":{}}}`
		req, _ := http.NewRequest("POST", cfg.Endpoint, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || len(raw) > contextWireLimit {
			t.Fatalf("wire %d status %d %s", len(raw), res.StatusCode, raw)
		}
	}
}
func TestFinancialContextCacheExpiryEvictionCursorAndOversize(t *testing.T) {
	c := newFinancialContextCache()
	g := c.activate()
	now := time.Now()
	c.now = func() time.Time { return now }
	result := application.FinancialContextResult{CapturedAt: now, ContentHash: "hash", Content: application.FinancialContextContent{Positions: []application.FinancialContextPosition{{Ref: "a"}, {Ref: "b"}}, Gaps: []application.FinancialContextGap{}, Evidence: []application.FinancialContextEvidence{}}}
	s := &Service{contexts: c}
	e := cachedFinancialContext{result: result, expires: now.Add(contextTTL), size: 100}
	c.entries["id"] = e
	c.bytes = 100
	cursor := c.cursor("id", "positions", g, 1)
	for _, in := range []FinancialContextPageInput{{ContextID: "id", Section: "gaps", Cursor: cursor}, {ContextID: "id", Section: "positions", Cursor: cursor + "x"}, {ContextID: "id", Section: "positions", Cursor: c.cursor("other", "positions", g, 1)}, {ContextID: "id", Section: "positions", Cursor: c.cursor("id", "positions", g+1, 1)}, {ContextID: "id", Section: "positions", Cursor: c.cursor("id", "positions", g, 9)}} {
		if _, err := s.financialContextPage(t.Context(), g, in); err == nil {
			t.Fatal("accepted wrong cursor", in)
		}
	}
	now = now.Add(time.Minute)
	page, err := s.financialContextPage(t.Context(), g, FinancialContextPageInput{ContextID: "id", Section: "positions", Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	if page.CacheExpiresAt != e.expires.UTC().Format(time.RFC3339Nano) {
		t.Fatal("TTL renewed")
	}
	now = e.expires
	if _, err = s.financialContextPage(t.Context(), g, FinancialContextPageInput{ContextID: "id", Section: "positions", Cursor: cursor}); err == nil {
		t.Fatal("expired cache served")
	}
	c.entries["id"] = e
	c.bytes = 100
	c.evictOldest()
	if len(c.entries) != 0 || c.bytes != 0 {
		t.Fatal("eviction failed")
	}
	result.Content.Positions[1].Name = strings.Repeat("\"\\", 40000)
	e.result = result
	if _, err = c.response("id", e, g, "positions", 1, 100); err == nil {
		t.Fatal("oversized row accepted")
	}
	result.Content.Scope.AccountRefs = []string{strings.Repeat("x", contextWireLimit)}
	e.result = result
	if _, err = c.response("id", e, g, "", 0, 1); err == nil {
		t.Fatal("oversized summary accepted")
	}
}

type blockingContextRepository struct {
	application.Repository
	port              application.FinancialContextRepository
	captured, release chan struct{}
}

func (r *blockingContextRepository) ReadFinancialContextInputs(ctx context.Context, historical bool, ids []domain.AccountID, now time.Time) (domain.FinancialContextInputs, error) {
	inputs, err := r.port.ReadFinancialContextInputs(ctx, historical, ids, now)
	close(r.captured)
	select {
	case <-r.release:
	case <-ctx.Done():
		return inputs, ctx.Err()
	}
	return inputs, err
}
func TestFinancialContextRevocationPreventsInflightPublication(t *testing.T) {
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
			s := New(app, t.TempDir(), nil)
			defer s.Close()
			if _, err = s.Enable(ReadOnly); err != nil {
				t.Fatal(err)
			}
			g := s.contextGeneration
			done := make(chan error, 1)
			go func() {
				_, err := s.buildFinancialContext(context.Background(), g, application.FinancialContextRequest{})
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
func TestFinancialContextCancelledBuildAdmission(t *testing.T) {
	s, _, _ := fixture(t)
	g := s.contexts.activate()
	s.contexts.builds <- struct{}{}
	s.contexts.builds <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.buildFinancialContext(ctx, g, application.FinancialContextRequest{}); err == nil {
		t.Fatal("cancelled queue admission accepted")
	}
	<-s.contexts.builds
	<-s.contexts.builds
}
func TestFinancialContextActualRestoreRevokesFrozenResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewService(sqlite.NewRepository(db))
	appports.Wire(app)
	app.SetLiveDatabasePath(path)
	if err = app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Restore test", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	s := New(app, t.TempDir(), nil)
	defer s.Close()
	if _, err = s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	g := s.contextGeneration
	if _, err = s.buildFinancialContext(t.Context(), g, application.FinancialContextRequest{}); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "copy.db")
	if err = db.SnapshotTo(t.Context(), copyPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "restore.nestworth-backup")
	settings := []byte("{}\n")
	pkg := backup.Package{Manifest: backup.NewManifest(time.Now(), data, settings, sqlite.EntityCounts{}), Database: data, Settings: settings}
	if err = backup.WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	recovery := application.NewRecovery(path, backup.NewRuntime(), app)
	defer recovery.Shutdown()
	paused := false
	recovery.SetBeforeRestore(func(ctx context.Context) error { paused = true; return s.RevokeForRestore(ctx) })
	preview, err := recovery.InspectBackup(t.Context(), archive)
	if err != nil {
		t.Fatal(err)
	}
	result, err := recovery.ConfirmRestore(t.Context(), application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err != nil {
		t.Fatal(err)
	}
	if !paused || !result.RestartRequired || s.contexts.active(g) || s.Status().Running {
		t.Fatal("restore did not revoke")
	}
	if len(s.contexts.entries) != 0 {
		t.Fatal("restore left cached data")
	}
}
func TestFinancialContextPositionTransferScopeMatchesFullReplay(t *testing.T) {
	fx := newLedgerFixture(t)
	dest := transferDestinationAccount(t, fx)
	c := ledgerSession(t, fx)
	source := importTransferSource(t, fx, c)
	plan := ledgerPreview(t, c, map[string]any{"kind": "position_transfer", "fromHoldingId": source, "toAccountId": dest, "quantity": "3", "effectiveAt": "2026-09-21T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	if _, err := fx.app.AppendManualInstrumentQuote(t.Context(), domain.InstrumentID(fx.instrument), "10", "2026-09-21", false); err != nil {
		t.Fatal(err)
	}
	full, err := fx.app.HistoricalOverview(t.Context(), "2026-09-21", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{fx.brokerage, dest} {
		r, err := fx.app.BuildFinancialContext(t.Context(), application.FinancialContextRequest{AsOf: "2026-09-21", Disclosure: "named", Scope: application.FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{id}}})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Content.Positions {
			matched := false
			for _, row := range full.Rows {
				if row.Key == p.Ref {
					matched = true
					if (p.BaseAmount == nil) != (row.Left.BaseAmount == nil) || p.BaseAmount != nil && *p.BaseAmount != *row.Left.BaseAmount {
						t.Fatal(p, row.Left)
					}
					if p.Quantity != nil && *p.Quantity != *row.Left.Quantity {
						t.Fatal("quantity mismatch")
					}
				}
			}
			if !matched {
				t.Fatal("scope row missing from full", p)
			}
		}
	}
}

func TestFinancialContextRejectsUnboundedRPCID(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"jsonrpc":"2.0","id":"` + strings.Repeat("x", 513) + `","method":"tools/call","params":{"name":"get_financial_context","arguments":{}}}`
	req, _ := http.NewRequest("POST", cfg.Endpoint, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatal(res.StatusCode)
	}
}
