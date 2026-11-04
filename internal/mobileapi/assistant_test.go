package mobileapi

import (
	"bytes"
	"net/http"
	"testing"
)

func TestAssistantStatusUsesLiveMobileReadBoundary(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	response, raw := f.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"surface":"mobile"`)) || !bytes.Contains(raw, []byte(`"financialWrites":false`)) || !bytes.Contains(raw, []byte(`"modelRuns":false`)) {
		t.Fatalf("assistant status %d %s", response.StatusCode, raw)
	}
	for _, secret := range []string{f.token, f.csrf, f.s.config.Token, "Synthetic bank"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("assistant status disclosed a secret or financial data")
		}
	}
	for _, route := range []string{"/assistant/run", "/assistant/preview", "/assistant/confirm", "/assistant/credentials"} {
		response, _ = f.request(t, "POST", route, map[string]string{}, nil)
		if response.StatusCode != 404 {
			t.Fatalf("unsupported assistant route enabled: %s %d", route, response.StatusCode)
		}
	}
	f.s.Revoke("")
	response, _ = f.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != 401 {
		t.Fatalf("revoked assistant status: %d", response.StatusCode)
	}
	ro := fixture(t, Permissions{Read: false, Record: true})
	response, _ = ro.request(t, "GET", "/assistant/status", nil, nil)
	if response.StatusCode != 403 {
		t.Fatalf("record permission must not imply assistant read: %d", response.StatusCode)
	}
}
