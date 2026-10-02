package continuousbackup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	lss3 "github.com/benbjohnson/litestream/s3"
	"github.com/superfly/ltx"
)

// Each UI action performs one storage page, never an implicit full-history scan.
// At most 1 LIST + 20 identity GETs + 20 timestamp HEADs, under operationTimeout.
const recoveryObjectLimit = 20
const maxRecoveryCursorBytes = 16 << 10

type RecoveryPointPage struct {
	Points     []RecoveryPoint `json:"points"`
	NextCursor string          `json:"nextCursor"`
}
type recoveryObject struct {
	Key      string
	Modified time.Time
}
type recoveryObjectPage struct {
	Objects []recoveryObject
	Next    string
}
type recoveryCursor struct {
	Version int    `json:"v"`
	Scope   string `json:"scope"`
	Token   string `json:"token"`
}

func recoveryScope(c config) string {
	// No secret is exposed in the cursor. Scope includes credentials so replacing
	// them invalidates a pending traversal even when the bucket stays unchanged.
	data, _ := json.Marshal([]string{"recovery-v1", remotePrefix, targetKey(c), c.AccessKeyID, c.SecretAccessKey})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
func encodeRecoveryCursor(token string, c config) string {
	data, _ := json.Marshal(recoveryCursor{Version: 1, Scope: recoveryScope(c), Token: token})
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeRecoveryCursor(cursor string, c config) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > maxRecoveryCursorBytes {
		return "", ErrConfiguration
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrConfiguration
	}
	var value recoveryCursor
	if json.Unmarshal(data, &value) != nil || value.Version != 1 || value.Scope != recoveryScope(c) || value.Token == "" || len(value.Token) > maxRecoveryCursorBytes/2 {
		return "", ErrConfiguration
	}
	return value.Token, nil
}

func (b r2Backend) probe(ctx context.Context) error {
	_, err := b.sdk().ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(b.c.Bucket), Prefix: aws.String(remotePrefix), MaxKeys: aws.Int32(1)})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (b r2Backend) recoveryObjects(ctx context.Context, token string) (recoveryObjectPage, error) {
	return listRecoveryObjects(ctx, b.sdk(), b.c.Bucket, token)
}
func listRecoveryObjects(ctx context.Context, client objectLister, bucket, token string) (recoveryObjectPage, error) {
	input := &awss3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(remotePrefix), MaxKeys: aws.Int32(recoveryObjectLimit)}
	if token != "" {
		input.ContinuationToken = aws.String(token)
	}
	page, err := client.ListObjectsV2(ctx, input)
	if err != nil || page == nil || len(page.Contents) > recoveryObjectLimit {
		return recoveryObjectPage{}, ErrUnavailable
	}
	result := recoveryObjectPage{Objects: make([]recoveryObject, 0, len(page.Contents))}
	for _, o := range page.Contents {
		result.Objects = append(result.Objects, recoveryObject{Key: aws.ToString(o.Key), Modified: aws.ToTime(o.LastModified)})
	}
	if aws.ToBool(page.IsTruncated) {
		result.Next = aws.ToString(page.NextContinuationToken)
		if result.Next == "" || result.Next == token || len(result.Next) > maxRecoveryCursorBytes/2 {
			return recoveryObjectPage{}, ErrUnavailable
		}
	}
	return result, nil
}
func (b r2Backend) recoveryTime(ctx context.Context, o recoveryObject) (time.Time, error) {
	head, err := b.sdk().HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(b.c.Bucket), Key: aws.String(o.Key)})
	if err != nil {
		return time.Time{}, ErrUnavailable
	}
	if value := head.Metadata[lss3.MetadataKeyTimestamp]; value != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed, nil
		}
	}
	// Same fallback as Litestream for objects without its original timestamp.
	if o.Modified.IsZero() {
		return time.Time{}, ErrUnavailable
	}
	return o.Modified, nil
}

func recoveryObjectPoint(key string) (RecoveryPoint, bool) {
	if !strings.HasPrefix(key, remotePrefix) {
		return RecoveryPoint{}, false
	}
	parts := strings.Split(strings.TrimPrefix(key, remotePrefix), "/")
	if len(parts) != 4 || !validUUID(parts[0]) || !validUUID(parts[1]) || parts[2] != "0000" {
		return RecoveryPoint{}, false
	}
	min, max, err := ltx.ParseFilename(parts[3])
	if err != nil || min == 0 || max < min || ltx.FormatFilename(min, max) != parts[3] {
		return RecoveryPoint{}, false
	}
	return RecoveryPoint{StreamID: parts[0] + "/" + parts[1], TXID: max.String()}, true
}
func readRecoveryPage(ctx context.Context, b backend, token string, records map[string]streamRecord) (RecoveryPointPage, error) {
	objects, err := b.recoveryObjects(ctx, token)
	if err != nil {
		return RecoveryPointPage{}, err
	}
	if len(objects.Objects) > recoveryObjectLimit {
		return RecoveryPointPage{}, ErrUnavailable
	}
	result := RecoveryPointPage{Points: []RecoveryPoint{}, NextCursor: objects.Next}
	checked := map[string]bool{}
	missing := map[string]bool{}
	for _, object := range objects.Objects {
		point, ok := recoveryObjectPoint(object.Key)
		if !ok {
			continue
		}
		if r := records[point.StreamID]; r.State == "deleting" || r.State == "deleted" {
			continue
		}
		if !checked[point.StreamID] {
			_, err := b.identity(ctx, point.StreamID)
			if errors.Is(err, os.ErrNotExist) {
				missing[point.StreamID] = true
			} else if err != nil {
				return RecoveryPointPage{}, ErrUnavailable
			}
			checked[point.StreamID] = true
		}
		if missing[point.StreamID] {
			continue
		}
		captured, err := b.recoveryTime(ctx, object)
		if err != nil {
			return RecoveryPointPage{}, ErrUnavailable
		}
		point.CapturedAt = captured.UTC().Format(time.RFC3339Nano)
		result.Points = append(result.Points, point)
	}
	sortRecoveryPoints(result.Points)
	return result, nil
}
func sortRecoveryPoints(points []RecoveryPoint) {
	sort.Slice(points, func(i, j int) bool {
		a, b := points[i], points[j]
		at, _ := time.Parse(time.RFC3339Nano, a.CapturedAt)
		bt, _ := time.Parse(time.RFC3339Nano, b.CapturedAt)
		if at.Equal(bt) {
			if a.StreamID == b.StreamID {
				return a.TXID > b.TXID
			}
			return a.StreamID > b.StreamID
		}
		return at.After(bt)
	})
}

// The filesystem backend is an integration-test adapter only. Production has
// no setting that can redirect credentials or storage discovery to this root.
func (b fileBackend) probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := os.Stat(b.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (b fileBackend) recoveryObjects(ctx context.Context, token string) (recoveryObjectPage, error) {
	var objects []recoveryObject
	err := filepath.WalkDir(b.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.root, path)
		if err != nil {
			return err
		}
		key := remotePrefix + strings.Replace(filepath.ToSlash(rel), "/ltx/0/", "/0000/", 1)
		if key <= token {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		objects = append(objects, recoveryObject{Key: key, Modified: info.ModTime()})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return recoveryObjectPage{}, err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	result := recoveryObjectPage{Objects: objects}
	if len(objects) > recoveryObjectLimit {
		result.Objects = objects[:recoveryObjectLimit]
		result.Next = result.Objects[recoveryObjectLimit-1].Key
	}
	return result, nil
}
func (b fileBackend) recoveryTime(ctx context.Context, o recoveryObject) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	return o.Modified, nil
}
