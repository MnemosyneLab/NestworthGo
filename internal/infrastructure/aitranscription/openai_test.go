package aitranscription

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMultipartContract(t *testing.T) {
	text, err := transcribe(context.Background(), []byte("synthetic-key"), []byte("synthetic-audio"), "mp4", roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/audio/transcriptions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("request contract")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("model") != assistant.TranscriptionModel {
			t.Fatal("model")
		}
		f, h, e := r.FormFile("file")
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if string(b) != "synthetic-audio" || h.Filename != "recording.mp4" {
			t.Fatal("audio")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"text":"你好","languages":[{"code":"zh"}]}`)), Header: http.Header{}}, nil
	}))
	if err != nil || text != "你好" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}
func TestRejectResponsesWithoutLeakingOrRetrying(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{401, "synthetic-key"}, {429, "rate limit"}, {500, "audio"}, {302, "redirect"}, {200, `{"text":`}, {200, `{"text":"ok"} trailing`}, {200, strings.Repeat("x", 65537)}} {
		calls := 0
		text, err := transcribe(context.Background(), []byte("synthetic-key"), []byte("audio"), "webm", roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
		}))
		if err != errTranscription || text != "" || calls != 1 {
			t.Fatalf("status=%d calls=%d err=%v", tc.status, calls, err)
		}
	}
}
func TestCancellationAndLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := transcribe(ctx, []byte("synthetic-key"), []byte("audio"), "wav", roundTrip(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }))
	if err != errTranscription {
		t.Fatal(err)
	}
	for _, format := range []string{"ogg", "../key"} {
		_, err = transcribe(context.Background(), nil, []byte("x"), format, roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected request"); return nil, nil }))
		if err != errTranscription {
			t.Fatal(err)
		}
	}
}
