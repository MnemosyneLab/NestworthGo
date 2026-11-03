package mobileapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestHTTPLANConfiguration(t *testing.T) {
	for _, address := range []string{"192.168.1.20:9443", "10.0.0.2:9443", "172.16.0.2:9443", "[fd12::1]:9443", "127.0.0.1:9443"} {
		c := Config{Enabled: true, Origin: "http://" + address, Listen: address, Mode: "http_lan"}
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:9443", "[::]:9443", "8.8.8.8:9443", "private.example:9443", "192.168.1.20:0", "192.168.1.20:65536"} {
		c := Config{Enabled: true, Origin: "http://" + address, Listen: address, Mode: "http_lan"}
		if c.Validate() == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	c := Config{Enabled: true, Origin: "http://192.168.1.20:9443", Listen: "192.168.1.21:9443", Mode: "http_lan"}
	if c.Validate() == nil {
		t.Fatal("origin/listener mismatch accepted")
	}
	c.Listen = "192.168.1.20:9443"
	c.FrontendOrigin = "https://synthetic.example"
	if c.Validate() == nil {
		t.Fatal("HTTP CORS accepted")
	}
}

func TestHTTPLANKeepsPairingCSRFPermissionsAndRevocation(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	// Serve only on an ephemeral loopback address with synthetic ledger data.
	server := httptest.NewServer(f.s)
	t.Cleanup(server.Close)
	f.tls = server
	f.s.config.Mode = "http_lan"
	f.s.config.Origin = server.URL
	f.s.listener = server.Listener
	response, raw := f.request(t, "GET", "/connection", nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"authentication":"bearer"`)) {
		t.Fatalf("probe: %s", raw)
	}
	response, _ = f.request(t, "GET", "/bootstrap", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("ambient cookie accepted over HTTP policy")
	}
	pair, err := f.s.Pair("Synthetic HTTP phone", Permissions{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	response, raw = f.request(t, "POST", "/session", map[string]string{"token": pair.Token}, nil)
	var login struct {
		AccessToken string `json:"accessToken"`
		CSRF        string `json:"csrfToken"`
	}
	if err := jsoncodec.Unmarshal(raw, &login); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || login.AccessToken == "" || len(response.Cookies()) != 0 {
		t.Fatal("HTTP must use memory bearer, never insecure cookie")
	}
	headers := map[string]string{"Authorization": "Bearer " + login.AccessToken, "X-CSRF-Token": login.CSRF}
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 200 {
		t.Fatal("reconnect rejected")
	}
	headers["Origin"] = ""
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 200 {
		t.Fatal("same-origin GET without Origin rejected")
	}
	headers["Origin"] = "https://evil.example"
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 403 || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cross-origin HTTP accepted")
	}
	headers["Origin"] = f.tls.URL
	response, _ = f.request(t, "POST", "/market-jobs/synthetic/cancel", struct{}{}, headers)
	if response.StatusCode != 403 {
		t.Fatal("read-only permission bypassed")
	}
	headers["X-CSRF-Token"] = "wrong"
	response, _ = f.request(t, "DELETE", "/session", nil, headers)
	if response.StatusCode != 403 {
		t.Fatal("CSRF bypassed")
	}
	headers["X-CSRF-Token"] = login.CSRF
	response, _ = f.request(t, http.MethodDelete, "/session", nil, headers)
	if response.StatusCode != 200 {
		t.Fatal("logout failed")
	}
	response, _ = f.request(t, "GET", "/bootstrap", nil, headers)
	if response.StatusCode != 401 {
		t.Fatal("session not revoked")
	}
}

func TestHTTPLANDefaultPortOrigin(t *testing.T) {
	c := Config{Enabled: true, Origin: "http://192.168.1.20:80", Listen: "192.168.1.20:80", Mode: "http_lan"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.browserOrigin() != "http://192.168.1.20" {
		t.Fatal("default HTTP port not normalized for browser headers")
	}
}
