package mobileapi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestSeparateFrontendRequiresOptInAndUsesOriginBoundBearer(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true, Market: true})
	origin := "https://synthetic-frontend.example"
	preflight := map[string]string{"Origin": origin, "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "content-type, x-nestworth-mobile, authorization, x-csrf-token", "Access-Control-Request-Private-Network": "true"}
	response, _ := f.request(t, "OPTIONS", "/session", nil, preflight)
	if response.StatusCode != 403 || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS enabled by default")
	}
	f.s.mu.Lock()
	f.s.config.FrontendOrigin = origin
	f.s.mu.Unlock()
	response, _ = f.request(t, "OPTIONS", "/session", nil, preflight)
	if response.StatusCode != 204 || response.Header.Get("Access-Control-Allow-Origin") != origin || response.Header.Get("Access-Control-Allow-Credentials") != "" || response.Header.Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("preflight: %d %v", response.StatusCode, response.Header)
	}
	for _, invalid := range []map[string]string{
		{"Origin": "null"}, {"Origin": "https://evil.example"},
		{"Origin": origin, "Access-Control-Request-Method": "PATCH"},
		{"Origin": origin, "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "x-arbitrary-header"},
	} {
		response, _ = f.request(t, "OPTIONS", "/session", nil, invalid)
		if response.StatusCode != 403 {
			t.Fatalf("unsafe preflight accepted: %v", invalid)
		}
	}
	response, raw := f.request(t, "GET", "/connection", nil, map[string]string{"Origin": origin})
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"authentication":"bearer"`)) {
		t.Fatalf("probe: %d %s", response.StatusCode, raw)
	}
	// An ambient same-origin cookie is never sufficient for a cross-origin read.
	response, _ = f.request(t, "GET", "/bootstrap", nil, map[string]string{"Origin": origin})
	if response.StatusCode != 401 {
		t.Fatal("cookie crossed origin boundary")
	}
	pair, err := f.s.Pair("Synthetic separate browser", Permissions{Read: true, Record: true})
	if err != nil {
		t.Fatal(err)
	}
	response, raw = f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, map[string]string{"Origin": origin})
	var login struct {
		AccessToken string `json:"accessToken"`
		CSRF        string `json:"csrfToken"`
	}
	if err := jsoncodec.Unmarshal(raw, &login); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || len(login.AccessToken) < 40 || len(response.Cookies()) != 0 || strings.Contains(string(raw), pair.Token) {
		t.Fatalf("bearer exchange: %d", response.StatusCode)
	}
	headers := map[string]string{"Origin": origin, "Authorization": "Bearer " + login.AccessToken, "X-CSRF-Token": login.CSRF}
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 200 {
		t.Fatal("bearer rejected")
	}
	f.s.mu.Lock()
	f.s.config.FrontendOrigin = "https://other-frontend.example"
	f.s.mu.Unlock()
	other := map[string]string{"Origin": "https://other-frontend.example", "Authorization": "Bearer " + login.AccessToken}
	response, _ = f.request(t, "GET", "/bootstrap", nil, other)
	if response.StatusCode != 401 {
		t.Fatal("session crossed its bound frontend origin")
	}
	f.s.mu.Lock()
	f.s.config.FrontendOrigin = origin
	f.s.mu.Unlock()
	response, _ = f.request(t, "GET", "/bootstrap", nil, map[string]string{"Authorization": "Bearer " + login.AccessToken})
	if response.StatusCode != 401 {
		t.Fatal("bearer accepted in cookie mode")
	}
	response, _ = f.request(t, "DELETE", "/session", nil, map[string]string{"Origin": origin, "Authorization": "Bearer " + login.AccessToken, "X-CSRF-Token": "wrong"})
	if response.StatusCode != 403 {
		t.Fatal("bearer bypassed CSRF")
	}
	response, _ = f.request(t, "DELETE", "/session", nil, headers)
	if response.StatusCode != 200 {
		t.Fatal("logout failed")
	}
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 401 {
		t.Fatal("logout did not revoke")
	}
}

func TestFrontendOriginValidationAndSafeJobDetails(t *testing.T) {
	c := Config{Enabled: true, Origin: "https://synthetic.private.example", Listen: "127.0.0.1:9443", Mode: "loopback_proxy"}
	for _, origin := range []string{"*", "null", "http://frontend.example", "https://*.example", "https://user:secret@frontend.example", "https://frontend.example/", "https://frontend.example/path", "https://frontend.example?", "https://frontend.example#", "https://frontend.example:bad"} {
		c.FrontendOrigin = origin
		if c.Validate() == nil {
			t.Fatalf("accepted %q", origin)
		}
	}
	c.FrontendOrigin = "https://frontend.example:9443"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	job := application.MobileMarketJob{Items: []application.SyncItemProgress{{Label: "Synthetic target", Status: "failed", Detail: "provider_authentication"}, {Label: "Synthetic target 2", Status: "failed", Detail: "secret-path-provider-message"}}, Blockers: []application.SyncBlocker{{Code: "provider_rate_limit", Reason: "raw-provider-secret"}, {Code: "raw-secret"}}}
	raw, err := jsoncodec.Marshal(mobileJobDTO(job, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret")) || !bytes.Contains(raw, []byte("provider_authentication")) || !bytes.Contains(raw, []byte("provider_rate_limit")) || bytes.Contains(raw, []byte("target 2")) {
		t.Fatalf("unsafe or unbounded details %s", raw)
	}
}

func TestReadOnlySessionCannotCancelProviderWork(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	response, _ := f.request(t, http.MethodPost, "/market-jobs/synthetic/cancel", struct{}{}, nil)
	if response.StatusCode != 403 {
		t.Fatalf("cancellation accepted: %d", response.StatusCode)
	}
}

func TestJobCursorFromCurrentPinsResolvedJobAcrossNavigation(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Market: true})
	for _, name := range []string{"Synthetic A", "Synthetic B"} {
		if _, err := f.s.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: name, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	start := func() string {
		t.Helper()
		job, _, conflict, err := f.s.app.StartMobileMarket(t.Context(), application.MobileMarketRequest{Mode: "latest", Scope: "all"})
		if err != nil || conflict {
			t.Fatalf("start: %v %v", conflict, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			current, _ := f.s.app.MobileMarketJob(job.ID)
			if current.Outcome != application.SyncOutcomeRunning {
				return job.ID
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("synthetic local task did not finish")
		return ""
	}
	id := start()
	response, raw := f.request(t, "GET", "/market-jobs/current?limit=1", nil, nil)
	var page struct {
		Next string `json:"next"`
	}
	if err := jsoncodec.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || page.Next == "" {
		t.Fatalf("first page: %d %s", response.StatusCode, raw)
	}
	path := "/market-jobs/" + id + "?limit=1&cursor=" + url.QueryEscape(page.Next)
	response, raw = f.request(t, "GET", path, nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte("Synthetic B")) {
		t.Fatalf("pinned page: %d %s", response.StatusCode, raw)
	}
	start()
	response, _ = f.request(t, "GET", path, nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("navigation changed the pinned task")
	}
	response, _ = f.request(t, "GET", "/market-jobs/current?limit=1&cursor="+url.QueryEscape(page.Next), nil, nil)
	if response.StatusCode != 409 {
		t.Fatal("old cursor attached to another current task")
	}
}

func TestJobDetailsPageEachCollectionFromTheOriginalOffset(t *testing.T) {
	for _, counts := range [][3]int{{0, 0, 65}, {0, 65, 65}, {5, 65, 40}, {65, 5, 0}} {
		t.Run(fmt.Sprint(counts), func(t *testing.T) {
			var job application.MobileMarketJob
			for i := range counts[0] {
				job.Items = append(job.Items, application.SyncItemProgress{Label: fmt.Sprintf("item-%02d", i)})
			}
			for i := range counts[1] {
				job.Result.Items = append(job.Result.Items, application.RefreshTargetResult{TargetKey: fmt.Sprintf("result-%02d", i)})
			}
			for i := range counts[2] {
				code := "provider_authentication"
				if i >= 30 {
					code = "provider_rate_limit"
				}
				if i >= 60 {
					code = "unavailable"
				}
				job.Blockers = append(job.Blockers, application.SyncBlocker{Code: code})
			}
			for _, offset := range []int{0, 30, 60, 90} {
				page := mobileJobDTO(job, offset, 30)
				items := page["items"].([]map[string]string)
				results := page["result"].(application.RefreshResult).Items
				blockers := page["blockers"].([]string)
				if len(items) != min(30, max(0, counts[0]-offset)) || len(results) != min(30, max(0, counts[1]-offset)) || len(blockers) != min(30, max(0, counts[2]-offset)) {
					t.Fatalf("offset %d repeated or lost rows: items=%d results=%d blockers=%d", offset, len(items), len(results), len(blockers))
				}
				if len(items) > 0 && items[0]["label"] != fmt.Sprintf("item-%02d", offset) {
					t.Fatalf("wrong item page %d", offset)
				}
				if len(results) > 0 && results[0].TargetKey != fmt.Sprintf("result-%02d", offset) {
					t.Fatalf("wrong result page %d: %+v", offset, results)
				}
				if len(blockers) > 0 && blockers[0] != job.Blockers[offset].Code {
					t.Fatalf("wrong blocker page %d: %v", offset, blockers)
				}
			}
		})
	}
}
