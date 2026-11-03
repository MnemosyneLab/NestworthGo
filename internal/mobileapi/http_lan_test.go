package mobileapi

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestLANConfigurationAndToken(t *testing.T) {
	for _, address := range []string{"192.168.1.20:9443", "10.0.0.2:9443", "172.16.0.2:9443", "[fd12::1]:9443", "127.0.0.1:9443", "[::1]:80"} {
		c := Config{Enabled: true, Listen: address, Token: "MYTOKEN1"}
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:9443", "[::]:9443", "8.8.8.8:9443", "[::ffff:8.8.8.8]:9443", "private.example:9443", "192.168.1.20:0", "192.168.1.20:65536", "192.168.1.20:080", "[fe80::1%en0]:9443", "http://192.168.1.20:9443", "192.168.001.20:9443"} {
		if (Config{Enabled: true, Listen: address, Token: "MYTOKEN1"}).Validate() == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	for _, token := range []string{"", "1234567", "has space", "new\nline", "符号测试12345678", strings.Repeat("x", 65)} {
		if (Config{Enabled: true, Listen: "192.168.1.20:9443", Token: token}).Validate() == nil {
			t.Fatal("accepted invalid token")
		}
	}
	s := New(nil, filepath.Join(t.TempDir(), "absent.json"), nil)
	if s.Status().Running || s.Status().Config.Enabled {
		t.Fatal("enabled by default")
	}
	generated := map[string]bool{}
	for range 100 {
		token, err := s.GenerateToken()
		if err != nil || len(token) != 10 || generated[token] {
			t.Fatal("invalid generated token")
		}
		if (Config{Enabled: true, Listen: "127.0.0.1:9443", Token: token}).Validate() != nil {
			t.Fatal("generated token rejected")
		}
		generated[token] = true
	}
	if (Config{Listen: "192.168.1.20:80"}).browserOrigin() != "http://192.168.1.20" {
		t.Fatal("default port not normalized")
	}
}

func TestLANReusableTokenAndSessionBoundary(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	// A stored token works for multiple logins and grants normal mobile actions.
	response, raw := f.request(t, "POST", "/session", map[string]string{"token": f.s.config.Token}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("reusable token: %d %s", response.StatusCode, raw)
	}
	old := f.token
	f.acceptLogin(t, response, raw)
	if f.token == old {
		t.Fatal("shared token reused as session credential")
	}
	response, raw = f.request(t, "GET", "/session", nil, map[string]string{"Origin": ""})
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"manual_quote":true`)) {
		t.Fatalf("normal mobile capabilities: %d %s", response.StatusCode, raw)
	}
	for _, headers := range []map[string]string{
		{"Origin": "https://evil.example"}, {"Origin": "null"}, {"Authorization": ""}, {"Authorization": "Bearer " + f.s.config.Token},
	} {
		response, raw = f.request(t, "GET", "/bootstrap", nil, headers)
		if response.StatusCode < 400 || bytes.Contains(raw, []byte("SYNTHETIC MOBILE TEST")) || response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("unauthorized read")
		}
	}
	response, _ = f.request(t, "OPTIONS", "/session", nil, map[string]string{"Origin": "https://separate.example", "Access-Control-Request-Method": "POST"})
	if response.StatusCode != 403 {
		t.Fatal("separate frontend accepted")
	}
	response, _ = f.request(t, http.MethodDelete, "/session", nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("logout failed")
	}
	response, _ = f.request(t, "GET", "/bootstrap", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("logout did not revoke")
	}
}

func TestLoginFailureAndRateLimit(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	for n := 0; n < 5; n++ {
		response, raw := f.request(t, "POST", "/session", map[string]string{"token": "WRONGTOKEN"}, nil)
		expected := 401
		if n == 4 {
			expected = 429
		} // fixture has already logged in once.
		if response.StatusCode != expected || bytes.Contains(raw, []byte("WRONGTOKEN")) || bytes.Contains(raw, []byte(f.s.config.Token)) {
			t.Fatalf("failure %d: %d %s", n, response.StatusCode, raw)
		}
	}
	response, _ := f.request(t, "POST", "/session", map[string]string{"token": f.s.config.Token}, nil)
	if response.StatusCode != 429 {
		t.Fatal("correct token bypassed throttle")
	}
	f.s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	response, _ = f.request(t, "POST", "/session", map[string]string{"token": f.s.config.Token}, nil)
	if response.StatusCode != 200 {
		t.Fatal("login did not recover after throttle")
	}
}

func TestLegacyConfigRequiresExplicitReenableWithoutChangingFile(t *testing.T) {
	for _, mode := range []string{"tls", "loopback_proxy", "http_lan"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mobile-access.json")
			raw := []byte(`{"enabled":true,"listen":"127.0.0.1:9443","origin":"https://old.example","certificate":"/old/cert","key":"/old/key","frontendOrigin":"https://frontend.example","mode":"` + mode + `"}`)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			s := New(nil, path, nil)
			if err := s.Resume(); err != nil {
				t.Fatal(err)
			}
			status := s.Status()
			if status.Running || status.Config.Enabled || status.Config.Token != "" || status.Config.Listen != "127.0.0.1:9443" || status.Error != "configuration_reset" {
				t.Fatal("unsafe migration")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, raw) {
				t.Fatal("legacy file changed before apply")
			}
			if _, err := s.Configure(Config{Listen: status.Config.Listen, Token: "NEWVALUE1"}); err != nil {
				t.Fatal(err)
			}
			saved, _ := os.ReadFile(path)
			if bytes.Contains(saved, []byte("https")) || bytes.Contains(saved, []byte("certificate")) || bytes.Contains(saved, []byte("mode")) {
				t.Fatal("legacy options retained after apply")
			}
		})
	}
}

func TestConfigurePersistsTokenAndRestartOrChangeRevokesSessions(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	// Configure owns a different loopback listener; the fixture HTTP server remains
	// only as a synthetic client path for checking revocation.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	probe.Close()
	c := Config{Enabled: true, Listen: address, Token: "NEWTOKEN1"}
	if _, err := f.s.Configure(c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.s.Close() })
	if len(f.s.sessions) != 0 {
		t.Fatal("settings did not revoke sessions")
	}
	raw, err := os.ReadFile(f.s.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved storedConfig
	if err := jsoncodec.Unmarshal(raw, &saved); err != nil || saved.Version != configVersion || saved.Config != c {
		t.Fatalf("persistence: %v", err)
	}
	info, _ := os.Stat(f.s.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("token file is not private")
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := New(f.s.app, f.s.path, nil)
	if fresh.Status().Config != c || len(fresh.sessions) != 0 {
		t.Fatal("restart lost token or retained sessions")
	}
	if err := fresh.Resume(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	f.s = fresh
	f.http.URL = fresh.Status().URL
	response, _ := f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("old session survived restart")
	}
	response, raw = f.request(t, "POST", "/session", map[string]string{"token": "NEWTOKEN1"}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("persistent login: %d %s", response.StatusCode, raw)
	}
	f.acceptLogin(t, response, raw)
	c.Token = "CHANGED12"
	if _, err := fresh.Configure(c); err != nil {
		t.Fatal(err)
	}
	response, _ = f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("token edit retained old session")
	}
	response, _ = f.request(t, "POST", "/session", map[string]string{"token": "NEWTOKEN1"}, nil)
	if response.StatusCode != 401 {
		t.Fatal("old shared token still worked")
	}
	c.Enabled = false
	if _, err := fresh.Configure(c); err != nil || fresh.Status().Running {
		t.Fatalf("disable: %v", err)
	}
}

func TestSharedTokenLoginReplacesOldIdleSessionsAfterRepeatedReloads(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	old := f.token
	now := time.Now()
	for n := range 8 {
		// Remain below the existing per-IP/minute attempt bound.
		f.s.now = func() time.Time { return now.Add(time.Duration(n+1) * time.Minute) }
		response, raw := f.request(t, "POST", "/session", map[string]string{"token": f.s.config.Token}, nil)
		if response.StatusCode != 200 {
			t.Fatalf("reload %d: %d %s", n, response.StatusCode, raw)
		}
		f.acceptLogin(t, response, raw)
	}
	if len(f.s.sessions) != 5 {
		t.Fatal("session set is unbounded")
	}
	response, _ := f.request(t, "GET", "/bootstrap", nil, map[string]string{"Authorization": "Bearer " + old})
	if response.StatusCode != 401 {
		t.Fatal("old idle session retained")
	}
	response, _ = f.request(t, "GET", "/bootstrap", nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("fresh session rejected")
	}
}

func TestInvalidAndFutureConfigurationNeverStarts(t *testing.T) {
	for _, raw := range []string{`{`, `{"version":99,"enabled":true,"listen":"127.0.0.1:9443","token":"TOKEN1234"}`, `{"version":2,"enabled":true,"listen":"0.0.0.0:9443","token":"TOKEN1234"}`} {
		path := filepath.Join(t.TempDir(), "mobile-access.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s := New(nil, path, nil)
		_ = s.Resume()
		if s.Status().Running || s.Status().Error == "" {
			t.Fatal("invalid or unknown configuration activated")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != raw {
			t.Fatal("loading rewrote saved settings")
		}
	}
}
