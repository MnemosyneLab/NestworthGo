package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type authorizedTransport struct{ token string }

func (t authorizedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}
func fixture(t *testing.T) (*Service, *application.Service, *atomic.Int32) {
	t.Helper()
	app := wailstest.NewService(t)
	if err := app.CompleteOnboarding(context.Background(), application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "USD", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatal(err)
	}
	changes := &atomic.Int32{}
	s := New(app, t.TempDir(), func() { changes.Add(1) })
	t.Cleanup(s.Close)
	return s, app, changes
}
func connect(t *testing.T, s *Service) *mcp.ClientSession {
	t.Helper()
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: cfg.Endpoint, HTTPClient: &http.Client{Transport: authorizedTransport{cfg.Token}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func call(t *testing.T, c *mcp.ClientSession, name string, args any, wantError bool) map[string]any {
	t.Helper()
	res, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError != wantError {
		raw, _ := json.Marshal(res.Content)
		t.Fatalf("%s error=%v content=%s", name, res.IsError, raw)
	}
	if wantError {
		return nil
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err = json.Unmarshal(data, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}
func mutate(t *testing.T, c *mcp.ClientSession, name string, input any) map[string]any {
	t.Helper()
	return call(t, c, name, map[string]any{"operationId": uuid.NewString(), "input": input}, false)["data"].(map[string]any)["result"].(map[string]any)
}
func TestHTTPDiscoveryAndReadOnly(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	listed, err := c.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 27 {
		t.Fatalf("tools=%d", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "create_") {
			t.Fatal("write tool in read-only mode")
		}
	}
	info := call(t, c, "get_context", Empty{}, false)["data"].(map[string]any)
	if info["mode"] != ReadOnly {
		t.Fatal(info)
	}
	call(t, c, "get_catalog", Empty{}, false)
	call(t, c, "get_overview", map[string]any{}, false)
	_, err = c.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_member", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]string{"name": "Bob"}}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("read-only write error: %v", err)
	}
}
func TestDirectoryWritesAndDurableRetries(t *testing.T) {
	s, app, changes := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	operationID := uuid.NewString()
	args := map[string]any{"operationId": operationID, "input": map[string]string{"name": "Bob"}}
	first := call(t, c, "create_member", args, false)["data"].(map[string]any)
	call(t, c, "create_member", args, false)
	if changes.Load() != 1 {
		t.Fatalf("duplicate write emitted %d changes", changes.Load())
	}
	id := first["result"].(map[string]any)["id"].(string)
	mutate(t, c, "update_member", map[string]any{"id": id, "name": "Bobby"})
	mutate(t, c, "archive_member", map[string]any{"id": id, "archived": true})
	members, err := app.ListMembers(context.Background(), false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members %v %v", members, err)
	}
	mutate(t, c, "archive_member", map[string]any{"id": id, "archived": false})
	args["input"] = map[string]string{"name": "Different"}
	call(t, c, "create_member", args, true)
	call(t, c, "create_member", map[string]any{"operationId": "../bad", "input": map[string]string{"name": "Bad"}}, true)
	call(t, c, "create_member", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"name": "Typo", "unexpected": true}}, true)
	s.Close()
	resumed := New(app, s.dir, nil)
	t.Cleanup(resumed.Close)
	if err = resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	c2 := connect(t, resumed)
	args["input"] = map[string]string{"name": "Bob"}
	call(t, c2, "create_member", args, false)
	members, err = app.ListMembers(context.Background(), false)
	if err != nil || len(members) != 2 {
		t.Fatalf("restart duplicated %v %v", members, err)
	}
	call(t, c2, "get_operation", IDInput{ID: operationID}, false)
	info, err := os.Stat(filepath.Join(s.dir, "connection.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("credential mode %v %v", info, err)
	}
}
func TestAccountInstrumentAndDirectoryLifecycle(t *testing.T) {
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	group := mutate(t, c, "create_group", map[string]string{"name": "Investments"})
	mutate(t, c, "update_group", map[string]any{"id": group["id"], "name": "Long term"})
	institution := mutate(t, c, "create_institution", map[string]string{"name": "Broker", "institutionType": "brokerage"})
	mutate(t, c, "update_institution", map[string]any{"id": institution["id"], "name": "Broker Two"})
	members, _ := app.ListMembers(context.Background(), false)
	accountInput := map[string]any{"initialAmount": "0", "name": "Cash", "accountType": "cash_on_hand", "balanceSheetRole": "asset", "trackingMode": "balance", "defaultCurrency": "USD", "includeInNetWorth": true, "includeInPortfolio": false, "includeInLiquidAssets": true, "ownerIds": []string{members[0].ID.String()}, "groupId": group["id"], "institutionId": institution["id"]}
	record := mutate(t, c, "create_account", accountInput)
	accountID := record["account"].(map[string]any)["id"]
	mutate(t, c, "update_account", map[string]any{"id": accountID, "changes": map[string]string{"name": "Cash renamed"}})
	records, err := app.ListAccounts(context.Background(), domain.AccountFilter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("records %v %v", records, err)
	}
	if records[0].Account.Name != "Cash renamed" || !records[0].Account.IncludeInNetWorth {
		t.Fatal("partial update lost fields")
	}
	accountInput["initialAmount"] = "100"
	call(t, c, "create_account", map[string]any{"operationId": uuid.NewString(), "input": accountInput}, true)
	inst := mutate(t, c, "create_instrument", map[string]any{"name": "Example", "type": "stock", "quoteCurrency": "USD", "symbol": "EX", "marketCode": "NASDAQ"})
	mutate(t, c, "update_instrument", map[string]any{"id": inst["id"], "changes": map[string]any{"name": "Example renamed"}})
	mutate(t, c, "archive_instrument", map[string]any{"id": inst["id"], "archived": true})
	call(t, c, "list_instruments", ListInput{IncludeArchived: true, Query: "example"}, false)
}
func TestUnknownOutcomeNeverReexecutes(t *testing.T) {
	s, _, _ := fixture(t)
	id := uuid.NewString()
	path, _ := s.operationPath(id)
	calls := 0
	_, err := s.execute(context.Background(), id, "test", Empty{}, func(context.Context) (any, error) { calls++; return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	op, _ := s.GetOperation(id)
	op.Status = "pending"
	op.Result = nil
	if err = writePrivateJSON(path, op); err != nil {
		t.Fatal(err)
	}
	_, err = s.execute(context.Background(), id, "test", Empty{}, func(context.Context) (any, error) { calls++; return "duplicate", nil })
	if err == nil || calls != 1 {
		t.Fatalf("unknown repeated: %d %v", calls, err)
	}
}
func TestConcurrentRetryExecutesOnce(t *testing.T) {
	s, _, _ := fixture(t)
	id := uuid.NewString()
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := s.execute(context.Background(), id, "test", Empty{}, func(context.Context) (any, error) { calls.Add(1); return "ok", nil })
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestHTTPBoundary(t *testing.T) {
	endpoint := "http://127.0.0.1:12345/mcp"
	handler := protect(endpoint, "secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "too large", 413)
			return
		}
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		name, host, origin, token, path string
		status                          int
	}{{"valid", "127.0.0.1:12345", "", "Bearer secret", "/mcp", 204}, {"browser", "127.0.0.1:12345", "https://evil.test", "Bearer secret", "/mcp", 403}, {"rebind", "evil.test:12345", "", "Bearer secret", "/mcp", 403}, {"no token", "127.0.0.1:12345", "", "", "/mcp", 401}, {"path", "127.0.0.1:12345", "", "Bearer secret", "/settings", 404}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://"+tc.host+tc.path, strings.NewReader("{}"))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Authorization", tc.token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
	r := httptest.NewRequest("POST", endpoint, strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
}
func TestPermissionChangeRevokesTokenAndDisablePersists(t *testing.T) {
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Connection()
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Connection()
	if old.Token == current.Token {
		t.Fatal("credential not rotated")
	}
	req, _ := http.NewRequest("POST", current.Endpoint, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+old.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal(resp.StatusCode)
	}
	if _, err := s.Disable(); err != nil {
		t.Fatal(err)
	}
	resumed := New(app, s.dir, nil)
	t.Cleanup(resumed.Close)
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	if resumed.Status().Running {
		t.Fatal("disabled server resumed")
	}
	if _, err := resumed.Connection(); err == nil {
		t.Fatal("disabled credential exposed")
	}
}

func TestDirectoryIsolation(t *testing.T) {
	a := Directory("/tmp/home/a.json", "/tmp/home/main.db")
	if a == Directory("/tmp/home/b.json", "/tmp/home/main.db") {
		t.Fatal("settings files share credentials")
	}
	if a == Directory("/tmp/home/a.json", "/tmp/home/dev.db") {
		t.Fatal("databases share credentials")
	}
}
func TestImmediateEnableDisableReleasesListener(t *testing.T) {
	s, _, _ := fixture(t)
	for range 10 {
		if _, err := s.Enable(ReadOnly); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Disable(); err != nil {
			t.Fatal(err)
		}
	}
}
