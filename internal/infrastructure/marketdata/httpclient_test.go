package marketdata

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

func TestSanitizeProviderErrorOmitsURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://example.test/secret-symbol", Err: errors.New("connection refused")}
	if got := sanitizeProviderError(err); got != "transport" {
		t.Fatalf("sanitizeProviderError() = %q, want transport", got)
	}
	if got := sanitizeProviderError(context.Canceled); got != "canceled" {
		t.Fatalf("canceled = %q", got)
	}

	var buf bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	logProviderRootCause(err)
	logged := buf.String()
	if strings.Contains(logged, "example.test") || strings.Contains(logged, "secret-symbol") || strings.Contains(logged, "https://") {
		t.Fatalf("provider log leaked URL: %s", logged)
	}
}
