package mobileapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type httpFixture struct {
	s       *Server
	tls     *httptest.Server
	cookie  *http.Cookie
	csrf    string
	account string
}

func fixture(t *testing.T, p Permissions) *httpFixture {
	t.Helper()
	app := wailstest.NewService(t)
	ctx := t.Context()
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "SYNTHETIC MOBILE TEST", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.CreateAccount(ctx, application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	s := New(app, filepath.Join(t.TempDir(), "mobile.json"), fstest.MapFS{"mobile.html": {Data: []byte("<html>Login shell</html>")}})
	ts := httptest.NewTLSServer(s)
	s.config = Config{Enabled: true, Origin: ts.URL, Mode: "tls"}
	s.listener = ts.Listener
	t.Cleanup(ts.Close)
	f := &httpFixture{s: s, tls: ts, account: a.Account.ID.String()}
	pair, err := s.Pair("Synthetic phone", p)
	if err != nil {
		t.Fatal(err)
	}
	response, raw := f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("login %d %s", response.StatusCode, raw)
	}
	for _, c := range response.Cookies() {
		if c.Name == cookieName {
			f.cookie = c
		}
	}
	if f.cookie == nil || !f.cookie.Secure || !f.cookie.HttpOnly || f.cookie.SameSite != http.SameSiteStrictMode || f.cookie.Domain != "" {
		t.Fatal("unsafe session cookie")
	}
	var body struct {
		CSRF string `json:"csrfToken"`
	}
	if err = jsoncodec.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	f.csrf = body.CSRF
	response, _ = f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, nil)
	if response.StatusCode != 401 {
		t.Fatal("token reused")
	}
	return f
}
func (f *httpFixture) request(t *testing.T, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = jsoncodec.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := http.NewRequest(method, f.tls.URL+prefix+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", f.tls.URL)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Nestworth-Mobile", "1")
	if f.cookie != nil {
		r.AddCookie(f.cookie)
		r.Header.Set("X-CSRF-Token", f.csrf)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, err := f.tls.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, raw
}

func TestHTTPSAuthAllowlistCSRFRevocationAndPermissions(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true, Market: true})
	for _, headers := range []map[string]string{{"Origin": "https://evil.example"}, {"Origin": "null"}, {"X-CSRF-Token": "wrong"}, {"X-Nestworth-Mobile": ""}} {
		response, _ := f.request(t, "POST", "/record-previews", map[string]string{}, headers)
		if response.StatusCode != 403 {
			t.Fatalf("request accepted %v: %d", headers, response.StatusCode)
		}
	}
	response, raw := f.request(t, "GET", "/bootstrap", nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte("SYNTHETIC MOBILE TEST")) {
		t.Fatalf("bootstrap %d %s", response.StatusCode, raw)
	}
	for _, route := range []string{"/backup", "/restore", "/settings", "/rpc", "/recordChange"} {
		response, _ := f.request(t, "POST", route, map[string]string{}, nil)
		if response.StatusCode != 404 {
			t.Fatalf("route %s = %d", route, response.StatusCode)
		}
	}
	f.s.Revoke("")
	response, raw = f.request(t, "GET", "/accounts", nil, nil)
	if response.StatusCode != 401 || bytes.Contains(raw, []byte("Synthetic bank")) {
		t.Fatalf("revoked %d %s", response.StatusCode, raw)
	}
	ro := fixture(t, Permissions{Read: true})
	response, _ = ro.request(t, "POST", "/record-previews", application.MobileRecordInput{OperationID: uuid.NewString(), Kind: "expense", AccountID: ro.account, Amount: "1", Currency: "CNY"}, nil)
	if response.StatusCode != 403 {
		t.Fatal("read-only phone recorded")
	}
}

func TestHTTPRecordRepeatedConfirmationAndReceiptAfterNewSession(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true})
	id := uuid.NewString()
	response, raw := f.request(t, "POST", "/record-previews", application.MobileRecordInput{OperationID: id, Kind: "expense", AccountID: f.account, Amount: "1.2345", Currency: "CNY"}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("preview %d %s", response.StatusCode, raw)
	}
	var p struct {
		PlanID string `json:"planId"`
	}
	if err := jsoncodec.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		response, raw = f.request(t, "POST", "/record-confirmations", map[string]string{"planId": p.PlanID, "operationId": id}, nil)
		if response.StatusCode != 200 || !bytes.Contains(raw, []byte("committed")) {
			t.Fatalf("confirm %d %s", response.StatusCode, raw)
		}
	}
	f.s.Revoke("")
	pair, err := f.s.Pair("Replacement phone", Permissions{Read: true, Record: true})
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = nil
	response, raw = f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("relogin %d %s", response.StatusCode, raw)
	}
	f.cookie = response.Cookies()[0]
	response, raw = f.request(t, "GET", "/operations/"+id, nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte("committed")) {
		t.Fatalf("receipt %d %s", response.StatusCode, raw)
	}
	response, raw = f.request(t, "GET", "/operations/"+uuid.NewString(), nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte("unknown")) {
		t.Fatalf("absent receipt claimed rejection: %d %s", response.StatusCode, raw)
	}
}

func TestHTTPMarketPlanRetryDoesNotStartAnotherCompletedJob(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Market: true})
	response, raw := f.request(t, "POST", "/market-previews", application.MobileMarketRequest{Mode: "latest", Scope: "stale"}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("preview %d %s", response.StatusCode, raw)
	}
	var plan struct {
		ID string `json:"planId"`
	}
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	var first string
	for n := 0; n < 2; n++ {
		response, raw = f.request(t, "POST", "/market-jobs", map[string]string{"planId": plan.ID}, nil)
		if response.StatusCode != 200 {
			t.Fatalf("start %d %s", response.StatusCode, raw)
		}
		var receipt struct {
			Job struct {
				ID string `json:"jobId"`
			} `json:"job"`
		}
		if err := jsoncodec.Unmarshal(raw, &receipt); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			first = receipt.Job.ID
			f.s.app.CancelMobileMarketAndWait()
		} else if receipt.Job.ID != first {
			t.Fatal("same reviewed plan started another job")
		}
	}
}

func TestRevocationOrdersAfterAdmittedCommit(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true})
	f.s.mu.RLock()
	var d *session
	for _, v := range f.s.sessions {
		copy := *v
		d = &copy
	}
	f.s.mu.RUnlock()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.s.app.MobileAuthorizedWrite(context.Background(), func() (func(), error) { return f.s.authorize(d, "record") }, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); f.s.Revoke(d.ID) }()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if release, err := f.s.authorize(d, "record"); err == nil {
		release()
		t.Fatal("revoked grant admitted")
	}
}

func TestConfigRejectsPublicPlaintextAndWildcardListeners(t *testing.T) {
	for _, c := range []Config{{true, "http://private.example", "127.0.0.1:9443", "loopback_proxy", "", ""}, {true, "https://private.example", "0.0.0.0:9443", "loopback_proxy", "", ""}, {true, "https://private.example", "192.168.1.2:9443", "loopback_proxy", "", ""}, {true, "https://8.8.8.8", "127.0.0.1:9443", "loopback_proxy", "", ""}, {true, "https://private.example/path", "127.0.0.1:9443", "loopback_proxy", "", ""}} {
		if err := c.Validate(); err == nil {
			t.Fatalf("unsafe config accepted %+v", c)
		}
	}
	s := New(nil, filepath.Join(t.TempDir(), "absent.json"), nil)
	if s.Status().Running || s.Status().Config.Enabled {
		t.Fatal("enabled by default")
	}
}

func TestSessionExpirationBodyBoundsAndNoCredentialsInErrors(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true})
	response, raw := f.request(t, "POST", "/record-previews", map[string]string{"token": "sentinel-secret", "garbage": strings.Repeat("x", 33<<10)}, nil)
	if response.StatusCode < 400 || bytes.Contains(raw, []byte("sentinel-secret")) {
		t.Fatal("unsafe oversized response")
	}
	f.s.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	response, _ = f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("idle expiration ignored")
	}
}

func TestMobileGrantExpiresAndRestartDropsSessions(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	pair, err := f.s.Pair("Synthetic pending grant", Permissions{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	f.s.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	response, _ := f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, nil)
	if response.StatusCode != 401 {
		t.Fatal("expired token accepted")
	}
	fresh := New(f.s.app, filepath.Join(t.TempDir(), "absent.json"), nil)
	fresh.config = f.s.config
	r := httptest.NewRequest("GET", f.tls.URL+prefix+"/session", nil)
	r.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	fresh.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("session survived process replacement")
	}
	r = httptest.NewRequest("GET", f.tls.URL+prefix+"/bootstrap", nil)
	r.Host = "evil.example"
	r.AddCookie(f.cookie)
	w = httptest.NewRecorder()
	f.s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("unexpected host accepted")
	}
}

func TestMobileHealthyReportHasAnEmptyArray(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	response, raw := f.request(t, "GET", "/health", nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"items":[]`)) {
		t.Fatalf("empty report %d %s", response.StatusCode, raw)
	}
}

func TestMobileRevocationRejectsAQueuedCommitBeforePersistence(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true})
	f.s.mu.RLock()
	var d *session
	for _, value := range f.s.sessions {
		copy := *value
		d = &copy
	}
	f.s.mu.RUnlock()
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 2)
	go func() {
		finished <- f.s.app.MobileAuthorizedWrite(context.Background(), func() (func(), error) { return func() {}, nil }, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	ran := false
	go func() {
		finished <- f.s.app.MobileAuthorizedWrite(context.Background(), func() (func(), error) { return f.s.authorize(d, "record") }, func(context.Context) error { ran = true; return nil })
	}()
	f.s.Revoke(d.ID)
	close(release)
	failures := 0
	for range 2 {
		if err := <-finished; err != nil {
			failures++
		}
	}
	if ran || failures != 1 {
		t.Fatal("revoked queued commit entered persistence")
	}
}
