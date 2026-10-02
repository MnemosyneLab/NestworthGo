package continuousbackup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/superfly/ltx"
)

// Separate from UI discovery: every page and every object under this exact
// BackupID is inspected. There is no UI stream/point cap or recursive delete.
type retentionBackend interface {
	inventory(context.Context, string) ([]storedObject, error)
	deleteObject(context.Context, storedObject) error
}
type objectLister interface {
	ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
}

func inventoryPages(ctx context.Context, client objectLister, bucket, prefix string) ([]storedObject, error) {
	var token *string
	seen := map[string]bool{}
	objects := []storedObject{}
	for {
		page, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix), ContinuationToken: token})
		if err != nil || page == nil {
			return nil, ErrUnavailable
		}
		for _, o := range page.Contents {
			objects = append(objects, storedObject{Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size), ETag: aws.ToString(o.ETag)})
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		next := aws.ToString(page.NextContinuationToken)
		if next == "" || seen[next] {
			return nil, ErrUnavailable
		}
		seen[next] = true
		token = aws.String(next)
	}
	return objects, nil
}
func (b r2Backend) inventory(ctx context.Context, owner string) ([]storedObject, error) {
	if owner != b.c.BackupID || !validUUID(owner) {
		return nil, ErrConfiguration
	}
	return inventoryPages(ctx, b.sdk(), b.c.Bucket, remotePrefix+owner+"/")
}
func objectStream(owner, key string) (string, error) {
	prefix := remotePrefix + owner + "/"
	if !validUUID(owner) || !strings.HasPrefix(key, prefix) {
		return "", ErrUnavailable
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
	if len(parts) < 2 || !validUUID(parts[0]) {
		return "", ErrUnavailable
	}
	if len(parts) == 2 && parts[1] == identityFile {
		return owner + "/" + parts[0], nil
	}
	if len(parts) != 4 || parts[1] != "ltx" || parts[2] != "0" {
		return "", ErrUnavailable
	}
	min, max, err := ltx.ParseFilename(parts[3])
	if err != nil || min == 0 || max < min || ltx.FormatFilename(min, max) != parts[3] {
		return "", ErrUnavailable
	}
	return owner + "/" + parts[0], nil
}
func (b r2Backend) deleteObject(ctx context.Context, o storedObject) error {
	if _, err := objectStream(b.c.BackupID, o.Key); err != nil || o.ETag == "" {
		return ErrUnavailable
	}
	_, err := b.sdk().DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(b.c.Bucket), Key: aws.String(o.Key), IfMatch: aws.String(o.ETag)})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (b fileBackend) inventory(ctx context.Context, owner string) ([]storedObject, error) {
	if !validUUID(owner) {
		return nil, ErrConfiguration
	}
	var result []storedObject
	root := filepath.Join(b.root, owner)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrUnavailable
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return ErrUnavailable
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(b.root, path)
		if err != nil {
			return err
		}
		result = append(result, storedObject{Key: remotePrefix + filepath.ToSlash(rel), Size: int64(len(data)), ETag: fmt.Sprintf("%x", sha256.Sum256(data))})
		return nil
	})
	if err != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}
func (b fileBackend) deleteObject(ctx context.Context, o storedObject) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	path := filepath.Join(b.root, filepath.FromSlash(strings.TrimPrefix(o.Key, remotePrefix)))
	// Only synthetic tests use this backend; check the current content anyway.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != o.ETag {
		return ErrUnavailable
	}
	return os.Remove(path)
}
func groupInventory(owner string, objects []storedObject) (map[string][]storedObject, error) {
	streams := map[string][]storedObject{}
	seen := map[string]bool{}
	for _, o := range objects {
		stream, err := objectStream(owner, o.Key)
		if err != nil || seen[o.Key] || o.Size < 0 || o.ETag == "" {
			return nil, ErrUnavailable
		}
		seen[o.Key] = true
		streams[stream] = append(streams[stream], o)
	}
	for stream, items := range streams {
		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		streams[stream] = items
	}
	return streams, nil
}
