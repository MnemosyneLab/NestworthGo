package continuousbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/version"
)

const identityFile = "stream.json"
const maxIdentityBytes = 16 << 10

type streamIdentity struct {
	Format     int    `json:"format"`
	AppID      string `json:"appID"`
	AppVersion string `json:"appVersion"`
	AppBuild   string `json:"appBuild"`
	Schema     int    `json:"schema"`
	StreamID   string `json:"streamID"`
	StartedAt  string `json:"startedAt"`
}

func newIdentity(stream string) streamIdentity {
	return streamIdentity{Format: 1, AppID: version.AppID, AppVersion: strings.TrimPrefix(version.Version, "v"), AppBuild: version.Build, Schema: sqlite.CurrentSchemaVersion, StreamID: stream, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}
func readIdentity(r io.Reader, stream string) (streamIdentity, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxIdentityBytes+1))
	var info streamIdentity
	if err != nil || len(data) > maxIdentityBytes || json.Unmarshal(data, &info) != nil {
		return info, ErrUnavailable
	}
	if info.Format != 1 || info.AppID != version.AppID || info.Schema != sqlite.CurrentSchemaVersion || info.StreamID != stream || !validStream(stream) || info.AppVersion == "" || info.AppBuild == "" {
		return info, ErrUnavailable
	}
	if _, err := time.Parse(time.RFC3339Nano, info.StartedAt); err != nil {
		return info, ErrUnavailable
	}
	return info, nil
}
func (b r2Backend) identity(ctx context.Context, stream string) (streamIdentity, error) {
	if !validStream(stream) {
		return streamIdentity{}, ErrConfiguration
	}
	result, err := b.sdk().GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(b.c.Bucket), Key: aws.String(remotePrefix + stream + "/" + identityFile)})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return streamIdentity{}, os.ErrNotExist
		}
		return streamIdentity{}, ErrUnavailable
	}
	defer result.Body.Close()
	return readIdentity(result.Body, stream)
}
func (b r2Backend) ensureIdentity(ctx context.Context, info streamIdentity) error {
	existing, err := b.identity(ctx, info.StreamID)
	if err == nil {
		if existing != info {
			return ErrUnavailable
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ErrUnavailable
	}
	data, _ := json.Marshal(info)
	// Conditional creation protects a fresh UUID path from accidental reuse;
	// this is stream ownership, not cross-machine election or continuation.
	_, err = b.sdk().PutObject(ctx, &awss3.PutObjectInput{Bucket: aws.String(b.c.Bucket), Key: aws.String(remotePrefix + info.StreamID + "/" + identityFile), Body: bytes.NewReader(data), ContentType: aws.String("application/json"), IfNoneMatch: aws.String("*")})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (b fileBackend) identity(ctx context.Context, stream string) (streamIdentity, error) {
	if err := ctx.Err(); err != nil {
		return streamIdentity{}, err
	}
	if !validStream(stream) {
		return streamIdentity{}, ErrConfiguration
	}
	f, err := os.Open(filepath.Join(b.root, filepath.FromSlash(stream), identityFile))
	if err != nil {
		return streamIdentity{}, err
	}
	defer f.Close()
	return readIdentity(f, stream)
}
func (b fileBackend) ensureIdentity(ctx context.Context, info streamIdentity) error {
	existing, err := b.identity(ctx, info.StreamID)
	if err == nil {
		if existing != info {
			return ErrUnavailable
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Join(b.root, filepath.FromSlash(info.StreamID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, identityFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(info)
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		return ErrUnavailable
	}
	return nil
}

// Candidate remains inside Go infrastructure/controller code; its local path
// is never returned as a Wails DTO. Identity is checked before downloading LTX.
type Candidate struct {
	Path       string
	AppVersion string
	AppBuild   string
}
