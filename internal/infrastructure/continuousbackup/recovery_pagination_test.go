package continuousbackup

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type recoveryTestTransport struct {
	t               *testing.T
	objects         map[string]string
	requests, lists int
	failList        bool
	failHead        bool
	emptyFirst      bool
}

func (f *recoveryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.requests++
	response := func(status int, body string, headers http.Header) (*http.Response, error) {
		return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d status", status), Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}
	if !strings.HasSuffix(r.URL.Host, ".r2.cloudflarestorage.com") {
		f.t.Fatalf("unexpected external request: %s", r.URL.Host)
	}
	if r.Method == "GET" && r.URL.Query().Get("list-type") == "2" {
		f.lists++
		if f.failList {
			return response(503, "<Error><Code>Unavailable</Code></Error>", http.Header{})
		}
		if r.URL.Query().Get("prefix") != remotePrefix || r.URL.Query().Get("delimiter") != "" {
			f.t.Fatal("discovery escaped its flat prefix scope")
		}
		limit, e := strconv.Atoi(r.URL.Query().Get("max-keys"))
		if e != nil || limit < 1 || limit > recoveryObjectLimit {
			f.t.Fatalf("unbounded LIST: %s", r.URL.RawQuery)
		}
		token := r.URL.Query().Get("continuation-token")
		offset := 0
		if token != "" {
			var e error
			offset, e = strconv.Atoi(strings.TrimPrefix(token, "page:"))
			if e != nil {
				return response(400, "<Error><Code>InvalidToken</Code></Error>", http.Header{})
			}
		}
		keys := []string{}
		for key := range f.objects {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		page := struct {
			XMLName               xml.Name `xml:"ListBucketResult"`
			IsTruncated           bool
			NextContinuationToken string
			Contents              []struct {
				Key          string
				LastModified string
			}
		}{}
		if f.emptyFirst && token == "" {
			page.IsTruncated = true
			page.NextContinuationToken = "page:0"
		} else {
			end := offset + limit
			if end > len(keys) {
				end = len(keys)
			}
			for _, key := range keys[offset:end] {
				page.Contents = append(page.Contents, struct {
					Key          string
					LastModified string
				}{key, "2026-10-01T00:00:00Z"})
			}
			if end < len(keys) {
				page.IsTruncated = true
				page.NextContinuationToken = fmt.Sprintf("page:%d", end)
			}
		}
		data, _ := xml.Marshal(page)
		return response(200, string(data), http.Header{"Content-Type": []string{"application/xml"}})
	}
	key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
	if r.Method == "HEAD" {
		if f.failHead {
			return response(503, "", http.Header{})
		}
		return response(200, "", http.Header{"X-Amz-Meta-Litestream-Timestamp": []string{"2026-10-01T00:00:00.11Z"}})
	}
	if r.Method == "GET" && strings.HasSuffix(key, "/"+identityFile) {
		stream := strings.TrimSuffix(strings.TrimPrefix(key, remotePrefix), "/"+identityFile)
		data, _ := json.Marshal(newIdentity(stream))
		return response(200, string(data), http.Header{})
	}
	f.t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
	return nil, nil
}
func installRecoveryTransport(t *testing.T) *recoveryTestTransport {
	t.Helper()
	f := &recoveryTestTransport{t: t, objects: map[string]string{}}
	before := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = before })
	return f
}
func paginationManager(t *testing.T) (*Manager, *recoveryTestTransport) {
	t.Helper()
	m, _, _ := fixture(t)
	if e := m.Configure(context.Background(), testUpdate(false)); e != nil {
		t.Fatal(e)
	}
	transport := installRecoveryTransport(t)
	m.factory = func(c config) backend { return r2Backend{c: c} }
	return m, transport
}
func addRecoveryObject(f *recoveryTestTransport, owner, stream string, tx int) {
	f.objects[fmt.Sprintf("%s%s/%s/0000/%016x-%016x.ltx", remotePrefix, owner, stream, tx, tx)] = ""
	f.objects[remotePrefix+owner+"/"+stream+"/"+identityFile] = ""
}
func TestRecoveryPaginationReachesAllOwnersStreamsAndOldPoints(t *testing.T) {
	m, f := paginationManager(t)
	ctx := context.Background()
	for n := 0; n < 513; n++ {
		addRecoveryObject(f, fmt.Sprintf("10000000-0000-4000-8000-%012d", n%3), fmt.Sprintf("20000000-0000-4000-8000-%012d", n), 1)
	}
	for n := 2; n <= 151; n++ {
		addRecoveryObject(f, "10000000-0000-4000-8000-000000000000", "20000000-0000-4000-8000-000000000000", n)
	}
	cursor := ""
	seen := map[string]bool{}
	pages := 0
	for {
		before := f.requests
		page, e := m.RecoveryPointPage(ctx, cursor)
		if e != nil {
			t.Fatal(e)
		}
		if delta := f.requests - before; delta > 1+2*recoveryObjectLimit {
			t.Fatalf("page used %d requests", delta)
		}
		pages++
		for _, p := range page.Points {
			seen[p.StreamID+":"+p.TXID] = true
			if p.CapturedAt != "2026-10-01T00:00:00.11Z" {
				t.Fatalf("lost metadata timestamp: %+v", p)
			}
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 663 {
		t.Fatalf("discovery silently omitted old history: points=%d", len(seen))
	}
	before := f.requests
	if e := m.TestConnection(ctx); e != nil {
		t.Fatal(e)
	}
	if f.requests-before != 1 {
		t.Fatal("connection probe traversed backup history")
	}
	t.Logf("513 streams across 3 owners, including 151 points in one stream: %d points in %d bounded pages; probe=1 request", len(seen), pages)
}
func TestRecoveryCursorScopeEmptyPageAndRetry(t *testing.T) {
	m, f := paginationManager(t)
	ctx := context.Background()
	f.emptyFirst = true
	addRecoveryObject(f, "10000000-0000-4000-8000-000000000000", "20000000-0000-4000-8000-000000000000", 1)
	first, e := m.RecoveryPointPage(ctx, "")
	if e != nil || len(first.Points) != 0 || first.NextCursor == "" {
		t.Fatalf("empty continuing page: %+v %v", first, e)
	}
	before := f.requests
	if _, e = m.RecoveryPointPage(ctx, "invalid cursor"); e != ErrConfiguration {
		t.Fatalf("invalid cursor=%v", e)
	}
	if f.requests != before {
		t.Fatal("invalid cursor reached network")
	}
	c, _, e := m.store.load()
	if e != nil {
		t.Fatal(e)
	}
	other := c
	other.Bucket = "other-bucket"
	wrong := encodeRecoveryCursor("page:0", other)
	if _, e = m.RecoveryPointPage(ctx, wrong); e != ErrConfiguration {
		t.Fatalf("cross-target cursor=%v", e)
	}
	other = c
	other.SecretAccessKey = "replacement"
	if _, e = m.RecoveryPointPage(ctx, encodeRecoveryCursor("page:0", other)); e != ErrConfiguration {
		t.Fatal("cursor survived credential change")
	}
	f.failList = true
	if _, e = m.RecoveryPointPage(ctx, first.NextCursor); e == nil {
		t.Fatal("list failure accepted")
	}
	f.failList = false
	f.failHead = true
	if _, e = m.RecoveryPointPage(ctx, first.NextCursor); e == nil {
		t.Fatal("mid-page failure accepted")
	}
	f.failHead = false
	last, e := m.RecoveryPointPage(ctx, first.NextCursor)
	if e != nil || len(last.Points) != 1 || last.NextCursor != "" {
		t.Fatalf("same cursor retry lost point: %+v %v", last, e)
	}
}

func TestRecoveryCursorRejectsRepeatedStorageToken(t *testing.T) {
	f := &repeatingRecoveryLister{}
	if _, e := listRecoveryObjects(context.Background(), f, "bucket", "same"); e != ErrUnavailable {
		t.Fatalf("nonadvancing cursor=%v", e)
	}
}

type repeatingRecoveryLister struct{}

func (*repeatingRecoveryLister) ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	return &awss3.ListObjectsV2Output{IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("same")}, nil
}
