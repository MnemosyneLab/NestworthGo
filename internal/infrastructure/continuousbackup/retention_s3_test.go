package continuousbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/benbjohnson/litestream"
	lss3 "github.com/benbjohnson/litestream/s3"
)

// The endpoint captures the pinned S3 writer's actual PUT keys, never derives
// object keys from the file backend layout. It contains synthetic fixture data
// only and deliberately gives LIST a different timestamp from LTX metadata.
type contractObject struct {
	data            []byte
	etag, timestamp string
}
type contractS3 struct {
	mu      sync.Mutex
	objects map[string]contractObject
	deleted []string
	bucket  string
}

func (s *contractS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := "/" + s.bucket + "/"
	if r.URL.Path != "/"+s.bucket && !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "wrong bucket", 400)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
		type entry struct {
			Key                string
			Size               int
			ETag, LastModified string
		}
		page := struct {
			XMLName     xml.Name `xml:"ListBucketResult"`
			IsTruncated bool
			Contents    []entry
		}{}
		for k, o := range s.objects {
			if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
				page.Contents = append(page.Contents, entry{k, len(o.data), o.etag, "2020-01-01T00:00:00Z"})
			}
		}
		sort.Slice(page.Contents, func(i, j int) bool { return page.Contents[i].Key < page.Contents[j].Key })
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(page)
		return
	}
	if r.Method == http.MethodPut {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body", 400)
			return
		}
		etag := fmt.Sprintf("\"%x\"", sha256.Sum256(data))
		s.objects[key] = contractObject{data, etag, r.Header.Get("X-Amz-Meta-Litestream-Timestamp")}
		w.Header().Set("ETag", etag)
		return
	}
	o, ok := s.objects[key]
	if !ok {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(404)
		_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
		return
	}
	if match := r.Header.Get("If-Match"); match != "" && match != o.etag {
		w.WriteHeader(412)
		return
	}
	if r.Method == http.MethodDelete {
		// Cleanup must retain its precondition and remove metadata last.
		if r.Header.Get("If-Match") == "" {
			http.Error(w, "missing precondition", 400)
			return
		}
		if strings.HasSuffix(key, "/"+identityFile) {
			for k := range s.objects {
				if k != key && strings.HasPrefix(k, strings.TrimSuffix(key, identityFile)) {
					http.Error(w, "metadata first", 400)
					return
				}
			}
		}
		delete(s.objects, key)
		s.deleted = append(s.deleted, key)
		w.WriteHeader(204)
		return
	}
	w.Header().Set("ETag", o.etag)
	w.Header().Set("Last-Modified", "Wed, 01 Jan 2020 00:00:00 GMT")
	if o.timestamp != "" {
		w.Header().Set("X-Amz-Meta-Litestream-Timestamp", o.timestamp)
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(o.data)))
		return
	}
	if r.Method == http.MethodGet {
		http.ServeContent(w, r, key, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(o.data))
		return
	}
	http.Error(w, "unsupported method", 400)
}

// Only this test adapter redirects clients to loopback. Production credentials,
// endpoint validation and transport configuration have no test override.
type contractBackend struct {
	fileBackend
	production r2Backend
	endpoint   string
	sdkClient  *awss3.Client
}

func (b contractBackend) client(stream string) litestream.ReplicaClient {
	c := b.production.client(stream).(*lss3.ReplicaClient)
	c.Endpoint = b.endpoint
	return c
}
func (b contractBackend) inventory(ctx context.Context, owner string) ([]storedObject, error) {
	return inventoryPages(ctx, b.sdkClient, b.production.c.Bucket, remotePrefix+owner+"/")
}
func (b contractBackend) identity(ctx context.Context, stream string) (streamIdentity, error) {
	out, err := b.sdkClient.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(b.production.c.Bucket), Key: aws.String(remotePrefix + stream + "/" + identityFile)})
	if err != nil {
		return streamIdentity{}, err
	}
	defer out.Body.Close()
	return readIdentity(out.Body, stream)
}
func (b contractBackend) deleteObject(ctx context.Context, o storedObject) error {
	if _, err := objectStream(b.production.c.BackupID, o.Key); err != nil || o.ETag == "" {
		return ErrUnavailable
	}
	return deleteMatchedObject(ctx, b.sdkClient, b.production.c.Bucket, o)
}
func uploadS3Fixture(t *testing.T, m *Manager, c config, root string, streams []string) (contractBackend, *contractS3) {
	t.Helper()
	state := &contractS3{objects: map[string]contractObject{}, bucket: c.Bucket}
	server := httptest.NewServer(state)
	t.Cleanup(server.Close)
	production := r2Backend{c: c}
	sdk := awss3.New(production.sdk().Options(), func(o *awss3.Options) { o.BaseEndpoint = aws.String(server.URL) })
	b := contractBackend{fileBackend{root}, production, server.URL, sdk}
	for _, stream := range streams {
		local := fileBackend{root}.client(stream)
		itr, err := local.LTXFiles(context.Background(), 0, 0, true)
		if err != nil {
			t.Fatal(err)
		}
		for itr.Next() {
			info := itr.Item()
			src, err := local.OpenLTXFile(context.Background(), 0, info.MinTXID, info.MaxTXID, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(src)
			src.Close()
			if err != nil {
				t.Fatal(err)
			}
			// This call—not the test—constructs the remote L0 object key and metadata.
			if _, err = b.client(stream).WriteLTXFile(context.Background(), 0, info.MinTXID, info.MaxTXID, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := itr.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(stream), identityFile))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = sdk.PutObject(context.Background(), &awss3.PutObjectInput{Bucket: aws.String(c.Bucket), Key: aws.String(remotePrefix + stream + "/" + identityFile), Body: bytes.NewReader(data)}); err != nil {
			t.Fatal(err)
		}
	}
	m.op.Lock()
	m.factory = func(config) backend { return b }
	m.op.Unlock()
	return b, state
}

func TestPinnedS3WriterActiveRetentionPreview(t *testing.T) {
	m, _, root := fixture(t)
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, m, "recent-backup-confirmed")
	c, _, err := m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := uploadS3Fixture(t, m, c, root, []string{v.StreamID})
	objects, err := b.inventory(context.Background(), c.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	dataObjects := 0
	for _, o := range objects {
		if strings.HasSuffix(o.Key, ".ltx") {
			dataObjects++
			if !strings.HasPrefix(o.Key, remotePrefix+v.StreamID+"/0000/") {
				t.Fatalf("pinned S3 layout changed: %s", o.Key)
			}
			if _, err := objectStream(c.BackupID, o.Key); err != nil {
				t.Errorf("actual pinned S3 writer key rejected: %s: %v", o.Key, err)
			}
		}
	}
	if dataObjects == 0 {
		t.Fatal("no S3 PUT captured")
	}
	for _, enabled := range []bool{false, true} {
		if enabled {
			activateCleanup(t, m)
		}
		p, err := m.PreviewRetention(context.Background(), 30)
		if err != nil {
			t.Fatalf("active-only S3 preview must be readable (cleanup enabled=%t): %v", enabled, err)
		}
		if p.Ready || p.EligibleBytes != 0 || p.ScannedBytes == 0 || len(p.Items) != 1 || p.Items[0].Reason != "current" || p.Items[0].Eligible {
			t.Fatalf("active stream not protected: %+v", p)
		}
	}
}

func TestPinnedS3RetentionSurvivorsCleanupAndTimestamps(t *testing.T) {
	m, c, root, streams := sealedFixture(t, 3)
	b, state := uploadS3Fixture(t, m, c, root, streams)
	activateCleanup(t, m)
	p, err := m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ready || p.EligibleBytes == 0 || len(p.Items) != 3 {
		t.Fatalf("no S3 survivors: %+v", p)
	}
	for _, item := range p.Items {
		if item.StreamID == streams[0] {
			if !item.Eligible || item.Reason != "expired-sealed" {
				t.Fatalf("seal age ignored: %+v", item)
			}
		} else if item.Eligible || item.Reason != "verified-survivor" {
			t.Fatalf("survivor not verified: %+v", item)
		}
	}
	// S3 HEAD metadata is the LTX capture time; LIST LastModified deliberately
	// differs. Recovery uses the pinned adapter's metadata, retention uses seals.
	points, err := m.RecoveryPoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("no S3 recovery points")
	}
	state.mu.Lock()
	for _, point := range points {
		found := false
		for key, o := range state.objects {
			if strings.HasPrefix(key, remotePrefix+point.StreamID+"/0000/") && strings.HasSuffix(key, "-"+point.TXID+".ltx") {
				if point.CapturedAt != o.timestamp || point.CapturedAt == "2020-01-01T00:00:00Z" {
					t.Errorf("capture metadata lost: got %s want %s", point.CapturedAt, o.timestamp)
				}
				found = true
			}
		}
		if !found {
			t.Errorf("recovery key mismatch: %+v", point)
		}
	}
	state.mu.Unlock()
	if _, err = m.ExecuteRetention(context.Background(), CleanupRequest{Token: p.Token, Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	deleted := append([]string(nil), state.deleted...)
	state.mu.Unlock()
	if len(deleted) < 2 || deleted[len(deleted)-1] != remotePrefix+streams[0]+"/"+identityFile {
		t.Fatalf("metadata not last: %v", deleted)
	}
	for _, key := range deleted {
		if !strings.HasPrefix(key, remotePrefix+streams[0]+"/") {
			t.Fatalf("protected stream deleted: %s", key)
		}
	}
	records, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	if records[streams[0]].State != "deleted" {
		t.Fatal("missing completion tombstone")
	}
	for _, stream := range streams[1:] {
		if err := m.verifySurvivor(context.Background(), b, records[stream]); err != nil {
			t.Fatalf("retained S3 stream cannot restore: %v", err)
		}
	}
}

func TestRetentionObjectLayoutsFailClosed(t *testing.T) {
	m, c, root, streams := sealedFixture(t, 1)
	b, state := uploadS3Fixture(t, m, c, root, streams)
	const name = "0000000000000001-000000000000000a.ltx"
	base := remotePrefix + streams[0] + "/"
	for _, suffix := range []string{"0000/" + name, "ltx/0/" + name, identityFile} {
		if got, err := objectStream(c.BackupID, base+suffix); err != nil || got != streams[0] {
			t.Fatalf("legitimate layout rejected: %s %v", suffix, err)
		}
	}
	invalid := []string{
		"0001/" + name, "000a/" + name, "0/" + name, "00000/" + name,
		"ltx/1/" + name, "ltx/0000/" + name, "ltx/0/extra/" + name, "0000/extra/" + name,
		"0000/../" + name, "0000//" + name, "0000/" + name + "/extra", "0000/" + name + "/",
		"0000/0000000000000000-0000000000000001.ltx",
		"0000/000000000000000a-0000000000000001.ltx",
		"0000/0000000000000001-000000000000000A.ltx",
		"0000/1-a.ltx", "0000/" + name + ".tmp", "0000/stream.json", "stream.json/extra", "unknown",
	}
	keys := make([]string, 0, len(invalid)+4)
	for _, suffix := range invalid {
		keys = append(keys, base+suffix)
	}
	keys = append(keys, remotePrefix+"not-an-owner/"+strings.Split(streams[0], "/")[1]+"/0000/"+name,
		remotePrefix+c.BackupID+"-other/"+strings.Split(streams[0], "/")[1]+"/0000/"+name,
		remotePrefix+c.BackupID+"/not-a-stream/0000/"+name,
		remotePrefix+c.BackupID+"/AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA/0000/"+name,
		"/"+base+"0000/"+name)
	for _, key := range keys {
		t.Run(strings.TrimPrefix(key, base), func(t *testing.T) {
			o := storedObject{Key: key, Size: 1, ETag: "synthetic"}
			if _, err := objectStream(c.BackupID, key); err == nil {
				t.Fatal("unknown path accepted")
			}
			if _, err := groupInventory(c.BackupID, []storedObject{o}); err == nil {
				t.Fatal("inventory accepted unknown path")
			}
			if err := b.deleteObject(context.Background(), o); err == nil {
				t.Fatal("delete accepted unknown path")
			}
			state.mu.Lock()
			state.objects[key] = contractObject{data: []byte("x"), etag: "synthetic"}
			state.mu.Unlock()
			_, err := m.PreviewRetention(context.Background(), 30)
			if strings.HasPrefix(key, remotePrefix+c.BackupID+"/") {
				if err == nil {
					t.Fatal("preview accepted unknown path inside its target")
				}
			} else if err != nil {
				t.Fatal("object outside the exact target affected preview")
			}
			state.mu.Lock()
			delete(state.objects, key)
			state.mu.Unlock()
		})
	}
	valid := storedObject{Key: base + "0000/" + name, Size: 1, ETag: "synthetic"}
	if _, err := groupInventory(c.BackupID, []storedObject{valid, valid}); err == nil {
		t.Fatal("duplicate accepted")
	}
	valid.ETag = ""
	if _, err := groupInventory(c.BackupID, []storedObject{valid}); err == nil {
		t.Fatal("empty ETag accepted")
	}
	if err := b.deleteObject(context.Background(), valid); err == nil {
		t.Fatal("delete accepted empty ETag")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.deleted) != 0 {
		t.Fatal("invalid inventory caused deletion")
	}
}
