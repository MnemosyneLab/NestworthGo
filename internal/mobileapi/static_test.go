package mobileapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"testing/fstest"

	webassets "github.com/waltwang/nestworth-go"
)

func staticRequest(s *Server, method, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://mobile.example"+target, nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestMobileStaticAssetAllowlist(t *testing.T) {
	assets := fstest.MapFS{
		"mobile.html": {Data: []byte("<html>synthetic mobile shell</html>")},
	}
	for _, name := range []string{"assets/logo-mark-test.png", "assets/mobile-test.js", "assets/mobile-test.css", "assets/font-test.woff2", "assets/private.json", "assets/mobile-test.js.map", "assets/page.html", "assets/image.svg", "logo.png"} {
		assets[name] = &fstest.MapFile{Data: []byte("synthetic asset")}
	}
	s := New(nil, "", assets)
	s.config = Config{Origin: "https://mobile.example", Mode: "tls"}
	for _, tc := range []struct {
		method, target string
		status         int
	}{
		{"GET", "/", 200},
		{"GET", "/mobile.html", 200},
		{"GET", "/assets/logo-mark-test.png", 200},
		{"GET", "/assets/mobile-test.js", 200},
		{"GET", "/assets/mobile-test.css", 200},
		{"GET", "/assets/font-test.woff2", 200},
		{"GET", "/assets/private.json", 404},
		{"GET", "/assets/mobile-test.js.map", 404},
		{"GET", "/assets/page.html", 404},
		{"GET", "/assets/image.svg", 404},
		{"GET", "/logo.png", 404},
		{"GET", "/assets/../logo.png", 404},
		{"GET", "/assets/missing.png", 404},
		{"POST", "/assets/logo-mark-test.png", 404},
	} {
		t.Run(tc.method+tc.target, func(t *testing.T) {
			w := staticRequest(s, tc.method, tc.target)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
	w := staticRequest(s, "GET", "/assets/logo-mark-test.png")
	if w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || w.Header().Get("Content-Encoding") != "" || w.Body.String() != "synthetic asset" {
		t.Fatalf("PNG response: headers=%v body=%q", w.Header(), w.Body.String())
	}
}

// Build the production frontend first, then opt in to verify the same embedded
// filesystem and HTTP handler used by the packaged application.
func TestPackagedMobileLogo(t *testing.T) {
	if os.Getenv("NESTWORTH_MOBILE_PACKAGE_TEST") != "1" {
		t.Skip("requires production frontend build and NESTWORTH_MOBILE_PACKAGE_TEST=1")
	}
	assets, err := fs.Sub(webassets.Dist, "frontend/dist")
	if err != nil {
		t.Fatal(err)
	}
	s := New(nil, "", assets)
	s.config = Config{Origin: "https://mobile.example", Mode: "tls"}
	shell := staticRequest(s, "GET", "/mobile.html")
	if shell.Code != http.StatusOK {
		t.Fatalf("mobile shell status = %d", shell.Code)
	}
	// Vite can move the logo into a shared preloaded chunk.
	scripts := regexp.MustCompile(`(?:src|href)="(/assets/[^" ]+\.js)"`).FindAllSubmatch(shell.Body.Bytes(), -1)
	var logo []byte
	for _, script := range scripts {
		raw, err := fs.ReadFile(assets, string(script[1][1:]))
		if err != nil {
			t.Fatal(err)
		}
		logo = regexp.MustCompile(`/assets/logo-mark-[^"\s]+\.png`).Find(raw)
		if logo != nil {
			break
		}
	}
	if logo == nil {
		t.Fatal("mobile entry and preloaded chunks do not reference the emitted PNG logo")
	}
	expected, err := fs.ReadFile(assets, string(logo[1:]))
	if err != nil {
		t.Fatal(err)
	}
	w := staticRequest(s, "GET", string(logo))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), expected) {
		t.Fatalf("packaged logo %s: status=%d type=%q bytes=%d, want %d", logo, w.Code, w.Header().Get("Content-Type"), w.Body.Len(), len(expected))
	}
	t.Logf("served embedded production logo %s (%d bytes)", logo, len(expected))
}
