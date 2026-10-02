package continuousbackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

func TestPureRetentionProtections(t *testing.T) {
	now := time.Now().UTC()
	records := map[string]streamRecord{}
	objects := map[string][]storedObject{}
	for _, name := range []string{"current", "survivor1", "survivor2", "old", "recent", "crash", "legacy", "foreign", "pinned", "future"} {
		records[name] = streamRecord{State: "sealed", SealedAt: now.AddDate(0, 0, -100).Format(time.RFC3339Nano), FinalTXID: "0000000000000001"}
		objects[name] = []storedObject{{Size: 10}}
	}
	delete(records, "legacy")
	delete(records, "foreign")
	r := records["crash"]
	r.State = "open"
	records["crash"] = r
	r = records["recent"]
	r.SealedAt = now.AddDate(0, 0, -5).Format(time.RFC3339Nano)
	records["recent"] = r
	r = records["future"]
	r.SealedAt = now.AddDate(0, 0, 1).Format(time.RFC3339Nano)
	records["future"] = r
	verified := map[string]bool{"survivor1": true, "survivor2": true}
	for _, days := range []int{30, 90} {
		p := planRetention(now, days, "current", records, objects, verified, map[string]bool{"pinned": true})
		if !p.Ready || p.EligibleBytes != 10 || p.ScannedBytes != 100 {
			t.Fatalf("wrong totals %+v", p)
		}
		for _, i := range p.Items {
			if i.Eligible != (i.StreamID == "old") {
				t.Fatalf("unsafe plan %+v", i)
			}
		}
	}
	delete(verified, "survivor2")
	p := planRetention(now, 30, "current", records, objects, verified, nil)
	if p.Ready || p.EligibleBytes != 0 {
		t.Fatal("insufficient survivors allowed cleanup")
	}
}

type fakePages struct {
	t              *testing.T
	pages          [][]storedObject
	call           int
	prefix, bucket string
}

func (f *fakePages) ListObjectsV2(ctx context.Context, in *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	if aws.ToString(in.Prefix) != f.prefix || aws.ToString(in.Bucket) != f.bucket || in.Delimiter != nil {
		f.t.Fatal("inventory scope changed")
	}
	if f.call > 0 && aws.ToString(in.ContinuationToken) != fmt.Sprint(f.call) {
		f.t.Fatal("lost continuation")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p := &awss3.ListObjectsV2Output{}
	for _, o := range f.pages[f.call] {
		p.Contents = append(p.Contents, types.Object{Key: aws.String(o.Key), Size: aws.Int64(o.Size), ETag: aws.String(o.ETag)})
	}
	f.call++
	p.IsTruncated = aws.Bool(f.call < len(f.pages))
	if aws.ToBool(p.IsTruncated) {
		p.NextContinuationToken = aws.String(fmt.Sprint(f.call))
	}
	return p, nil
}
func TestRetentionInventoryUsesEveryPageAndRejectsUnknownObjects(t *testing.T) {
	owner := uuid.NewString()
	all := []storedObject{}
	for i := 0; i < 600; i++ {
		stream := uuid.NewString()
		for j := 1; j <= 120; j++ {
			all = append(all, storedObject{Key: fmt.Sprintf("%s%s/%s/ltx/0/%016x-%016x.ltx", remotePrefix, owner, stream, j, j), Size: 1, ETag: "test"})
		}
	}
	f := &fakePages{t: t, prefix: remotePrefix + owner + "/", bucket: "test-bucket"}
	for offset := 0; offset < len(all); offset += 1000 {
		end := offset + 1000
		if end > len(all) {
			end = len(all)
		}
		f.pages = append(f.pages, all[offset:end])
	}
	result, err := inventoryPages(context.Background(), f, f.bucket, f.prefix)
	if err != nil || len(result) != 72000 || f.call != 72 {
		t.Fatalf("truncated inventory: %d %v", len(result), err)
	}
	grouped, err := groupInventory(owner, result)
	if err != nil || len(grouped) != 600 {
		t.Fatal("stream cap")
	}
	for _, items := range grouped {
		if len(items) != 120 {
			t.Fatal("point cap")
		}
	}
	for _, key := range []string{remotePrefix + owner + "-other/" + uuid.NewString() + "/stream.json", remotePrefix + owner + "/../stream.json", remotePrefix + owner + "/" + uuid.NewString() + "/unknown", all[0].Key + "/extra"} {
		if _, err := groupInventory(owner, []storedObject{{Key: key, Size: 1, ETag: "test"}}); err == nil {
			t.Fatalf("accepted unknown key %s", key)
		}
	}
	if _, err := groupInventory(owner, append(all[:1], all[0])); err == nil {
		t.Fatal("accepted duplicate")
	}
}
func sealedFixture(t *testing.T, n int) (*Manager, config, string, []string) {
	t.Helper()
	m, _, root := fixture(t)
	streams := []string{}
	for i := 0; i < n; i++ {
		if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
			t.Fatal(err)
		}
		v := waitState(t, m, "recent-backup-confirmed")
		streams = append(streams, v.StreamID)
		if err := m.Configure(context.Background(), testUpdate(false)); err != nil {
			t.Fatal(err)
		}
	}
	c, _, _ := m.store.load()
	records, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	for i, stream := range streams {
		r := records[stream]
		if r.State != "sealed" {
			t.Fatal("fixture not sealed")
		}
		r.SealedAt = time.Now().UTC().AddDate(0, 0, -100+i).Format(time.RFC3339Nano)
		if err := m.store.writeStream(c, r); err != nil {
			t.Fatal(err)
		}
	}
	return m, c, root, streams
}
func TestPreviewRestoresTwoSurvivorsAndPinsRestoreCandidates(t *testing.T) {
	m, _, root, streams := sealedFixture(t, 4)
	p, err := m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ready || p.EligibleBytes == 0 {
		t.Fatalf("no verified survivors: %+v", p)
	}
	count := 0
	for _, i := range p.Items {
		if i.Reason == "verified-survivor" {
			count++
		}
	}
	if count != 2 {
		t.Fatal("did not verify two survivors")
	}
	points, err := m.RecoveryPoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range points {
		if point.StreamID == streams[0] {
			candidate, err := m.Stage(context.Background(), point)
			if err != nil {
				t.Fatal(err)
			}
			os.RemoveAll(filepath.Dir(candidate.Path))
			break
		}
	}
	p, err = m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range p.Items {
		if i.StreamID == streams[0] && (i.Eligible || i.Reason != "restore-preview") {
			t.Fatal("preview not protected")
		}
	}
	// A corrupt newest survivor must never be replaced by an unverified claim.
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(streams[3]), "ltx", "0", "*.ltx"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing fixture")
	}
	if err = os.WriteFile(files[0], []byte("not-an-ltx-file"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err = m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ready || p.EligibleBytes != 0 {
		t.Fatal("corrupt survivor permitted cleanup")
	}
	entries, _ := filepath.Glob(filepath.Join(filepath.Dir(m.path), ".nestworth-retention-verify-*"))
	if len(entries) != 0 {
		t.Fatal("verification directory leaked")
	}
}
func TestDeletingStreamExcludedFromRecoveryAndStaging(t *testing.T) {
	m, c, _, streams := sealedFixture(t, 1)
	points, err := m.RecoveryPoints(context.Background())
	if err != nil || len(points) == 0 {
		t.Fatal("missing recovery")
	}
	records, _ := m.store.streams(c)
	r := records[streams[0]]
	r.State = "deleting"
	if err = m.store.writeStream(c, r); err != nil {
		t.Fatal(err)
	}
	got, err := m.RecoveryPoints(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatal("incomplete stream in recovery")
	}
	_, err = m.Stage(context.Background(), points[0])
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("staged incomplete stream")
	}
	if strings.Contains(err.Error(), c.SecretAccessKey) {
		t.Fatal("secret in error")
	}
}
