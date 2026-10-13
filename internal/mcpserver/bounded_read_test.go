package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBoundedReadCompletenessAndFrozenRetry(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(7)
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	rows := make([]string, 1001)
	for i := range rows {
		rows[i] = strings.Repeat("中文😀\\\"", 40)
	}
	raw, err := readRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.store(7, readCapture{Kind: "synthetic", Summary: map[string]int{"count": 1001}, Sections: map[string][]json.RawMessage{"items": raw}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Sections["items"]
	total := s.Page.Returned
	for s.Page.HasMore {
		in := ReadPageInput{r.CaptureID, "items", *s.Page.NextCursor, 100}
		a, err := c.page(7, in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := c.page(7, in)
		if err != nil {
			t.Fatal(err)
		}
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		if string(aj) != string(bj) {
			t.Fatal("retry changed")
		}
		size, _ := financialContextWireSize(a)
		if size > contextWireLimit {
			t.Fatal(size)
		}
		if a.Page.Returned == 0 {
			t.Fatal("no progress")
		}
		total += a.Page.Returned
		s = ReadSection{a.Items, *a.Page}
	}
	if total != 1001 {
		t.Fatalf("total %d", total)
	}
	now = now.Add(readTTL)
	if _, err := c.page(7, ReadPageInput{r.CaptureID, "items", c.cursor(r.CaptureID, "items", c.entries[r.CaptureID], 0), 25}); err == nil || !strings.Contains(err.Error(), "read_expired") {
		t.Fatalf("expiry %v", err)
	}
}
func TestBoundedReadCursorIsolation(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(1)
	rows, _ := readRows([]string{"one", "two"})
	r, err := c.store(1, readCapture{Kind: "test", Sections: map[string][]json.RawMessage{"a": rows, "b": rows}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	cursor := *r.Sections["a"].Page.NextCursor
	for _, in := range []ReadPageInput{{r.CaptureID, "b", cursor, 1}, {r.CaptureID, "a", cursor + "x", 1}, {"other", "a", cursor, 1}} {
		if _, e := c.page(1, in); e == nil {
			t.Fatal("accepted invalid cursor")
		}
	}
	c.revoke()
	if _, e := c.page(1, ReadPageInput{r.CaptureID, "a", cursor, 1}); e == nil || !strings.Contains(e.Error(), "read_revoked") {
		t.Fatal(e)
	}
}
func TestBoundedReadSingleItemAndSummaryOverflow(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(1)
	rows, _ := readRows([]string{strings.Repeat("😀", 20000)})
	r, err := c.store(1, readCapture{Kind: "test", Sections: map[string][]json.RawMessage{"items": rows}}, 25)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sections["items"].Page.Returned != 0 {
		t.Fatal("oversized row returned")
	}
	if _, err = c.page(1, ReadPageInput{r.CaptureID, "items", *r.Sections["items"].Page.NextCursor, 25}); err == nil || !strings.Contains(err.Error(), "single_item") {
		t.Fatal(err)
	}
	if _, err = c.store(1, readCapture{Kind: "test", Summary: strings.Repeat("x", 40000), Sections: map[string][]json.RawMessage{}}, 25); err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatal(err)
	}
}

func TestBoundedReadInflightRevocationAndAdmission(t *testing.T) {
	s := New(nil, t.TempDir(), nil)
	s.reads.activate(1)
	started, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := s.buildRead(t.Context(), 1, 25, func(ctx context.Context) (readCapture, error) {
			close(started)
			<-release
			rows, _ := readRows([]string{"one"})
			return readCapture{Kind: "test", Sections: map[string][]json.RawMessage{"items": rows}}, nil
		})
		result <- err
	}()
	<-started
	s.reads.revoke()
	close(release)
	err := <-result
	if err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal(err)
	}
	if len(s.reads.entries) != 0 {
		t.Fatal("revoked capture published")
	}
	s.reads.activate(2)
	for range readBuildConcurrency {
		s.reads.builds <- struct{}{}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.buildRead(ctx, 2, 25, func(context.Context) (readCapture, error) {
		t.Fatal("cancelled builder admitted")
		return readCapture{}, nil
	}); err == nil {
		t.Fatal("cancelled admission accepted")
	}
	for range readBuildConcurrency {
		<-s.reads.builds
	}
}
