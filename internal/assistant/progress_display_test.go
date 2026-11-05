package assistant

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToolDisplayPreservesFinanceAndRemovesCredentials(t *testing.T) {
	raw := `{"account":"Synthetic account","amount":12345678901234567890.123456789,"nested":[{"api_key":"synthetic-host-key","token":"secret","quantity":"1.230000"}]}`
	display, truncated := toolDisplay(raw, []byte("synthetic-host-key"))
	for _, required := range []string{"Synthetic account", "12345678901234567890.123456789", "1.230000"} {
		if !strings.Contains(display, required) {
			t.Fatal("financial detail altered")
		}
	}
	if truncated || strings.Contains(display, "synthetic-host-key") || strings.Contains(display, `"secret"`) {
		t.Fatal("credential leaked")
	}
	long, clipped := toolDisplay(`{"text":"`+strings.Repeat("合成", 3000)+`"}`, nil)
	if !clipped || len(long) > 8192 || !utf8.ValidString(long) {
		t.Fatal("unbounded or broken UTF-8")
	}
}
