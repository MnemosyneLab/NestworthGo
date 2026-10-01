package continuousbackup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/benbjohnson/litestream"
	"github.com/benbjohnson/litestream/file"
	lss3 "github.com/benbjohnson/litestream/s3"
	"github.com/google/uuid"
	"github.com/superfly/ltx"
)

const remotePrefix = "nestworth/v1/"
const operationTimeout = 10 * time.Second

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type backend interface {
	client(string) litestream.ReplicaClient
	streams(context.Context) ([]string, error)
	identity(context.Context, string) (streamIdentity, error)
	ensureIdentity(context.Context, streamIdentity) error
}
type r2Backend struct{ c config }

func (b r2Backend) client(stream string) litestream.ReplicaClient {
	c := lss3.NewReplicaClient()
	c.Region = "auto"
	c.Bucket = b.c.Bucket
	c.Path = remotePrefix + stream
	c.Endpoint = endpoint(b.c)
	c.ForcePathStyle = true
	c.AccessKeyID = b.c.AccessKeyID
	c.SecretAccessKey = b.c.SecretAccessKey
	c.SetLogger(quietLogger)
	return c
}
func (b r2Backend) sdk() *awss3.Client {
	return awss3.NewFromConfig(aws.Config{Region: "auto", RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired, Credentials: credentials.NewStaticCredentialsProvider(b.c.AccessKeyID, b.c.SecretAccessKey, ""), HTTPClient: &http.Client{Timeout: operationTimeout}, RetryMaxAttempts: 1}, func(o *awss3.Options) { o.BaseEndpoint = aws.String(endpoint(b.c)); o.UsePathStyle = true })
}
func (b r2Backend) streams(ctx context.Context) ([]string, error) {
	// Use explicit credentials, TLS verification and a bounded client. Never use
	// the user's AWS environment/profile as a fallback for missing R2 keys.
	c := b.sdk()
	list := func(prefix string) ([]string, error) {
		pager := awss3.NewListObjectsV2Paginator(c, &awss3.ListObjectsV2Input{Bucket: aws.String(b.c.Bucket), Prefix: aws.String(prefix), Delimiter: aws.String("/")})
		var out []string
		for pager.HasMorePages() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, ErrUnavailable
			}
			for _, p := range page.CommonPrefixes {
				out = append(out, aws.ToString(p.Prefix))
				if len(out) > 512 {
					return nil, ErrUnavailable
				}
			}
		}
		return out, nil
	}
	owners, err := list(remotePrefix)
	if err != nil {
		return nil, err
	}
	var streams []string
	for _, owner := range owners {
		if !validUUID(strings.TrimSuffix(strings.TrimPrefix(owner, remotePrefix), "/")) {
			continue
		}
		children, err := list(owner)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			stream := strings.TrimSuffix(strings.TrimPrefix(child, remotePrefix), "/")
			if validStream(stream) {
				streams = append(streams, stream)
			}
		}
		if len(streams) > 512 {
			return nil, ErrUnavailable
		}
	}
	sort.Strings(streams)
	return streams, nil
}

// fileBackend is used only by local integration tests; there is no UI/MCP
// switch that can redirect a credential-bearing production client to a URL.
type fileBackend struct{ root string }

func (b fileBackend) client(stream string) litestream.ReplicaClient {
	return file.NewReplicaClient(filepath.Join(b.root, filepath.FromSlash(stream)))
}
func (b fileBackend) streams(ctx context.Context) ([]string, error) {
	owners, err := os.ReadDir(b.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	var streams []string
	for _, owner := range owners {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !owner.IsDir() || !validUUID(owner.Name()) {
			continue
		}
		children, err := os.ReadDir(filepath.Join(b.root, owner.Name()))
		if err != nil {
			return nil, ErrUnavailable
		}
		for _, child := range children {
			stream := owner.Name() + "/" + child.Name()
			if child.IsDir() && validStream(stream) {
				streams = append(streams, stream)
			}
		}
	}
	sort.Strings(streams)
	return streams, nil
}
func validUUID(s string) bool { u, err := uuid.Parse(s); return err == nil && u.String() == s }
func validStream(s string) bool {
	parts := strings.Split(s, "/")
	return len(parts) == 2 && validUUID(parts[0]) && validUUID(parts[1])
}

type RecoveryPoint struct {
	StreamID   string `json:"streamID"`
	TXID       string `json:"txID"`
	CapturedAt string `json:"capturedAt"`
}

func recoveryPoints(ctx context.Context, b backend) ([]RecoveryPoint, error) {
	streams, err := b.streams(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := []RecoveryPoint{}
	for _, stream := range streams {
		if _, err := b.identity(ctx, stream); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, ErrUnavailable
		}
		c := b.client(stream)
		c.SetLogger(quietLogger)
		itr, err := c.LTXFiles(ctx, 0, 0, true)
		if err != nil {
			return nil, ErrUnavailable
		}
		// Preserve all remote history, but bound each stream's UI listing to the
		// latest 100 captured L0 recovery points. No compaction/retention in v1.
		points := []RecoveryPoint{}
		for itr.Next() {
			info := itr.Item()
			points = append(points, RecoveryPoint{StreamID: stream, TXID: info.MaxTXID.String(), CapturedAt: info.CreatedAt.UTC().Format(time.RFC3339Nano)})
			if len(points) > 100 {
				points = points[1:]
			}
		}
		if err := itr.Close(); err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, points...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.CapturedAt == b.CapturedAt && a.StreamID == b.StreamID {
			return a.TXID > b.TXID
		}
		return a.CapturedAt > b.CapturedAt
	})
	return out, nil
}
func restorePoint(ctx context.Context, b backend, p RecoveryPoint, destination string) error {
	if !validStream(p.StreamID) {
		return ErrConfiguration
	}
	if _, err := b.identity(ctx, p.StreamID); err != nil {
		return ErrUnavailable
	}
	txid, err := ltx.ParseTXID(p.TXID)
	if err != nil || txid == 0 {
		return ErrConfiguration
	}
	c := b.client(p.StreamID)
	c.SetLogger(quietLogger)
	replica := litestream.NewReplicaWithClient(nil, c)
	options := litestream.NewRestoreOptions()
	options.OutputPath = destination
	options.TXID = txid
	if err := replica.Restore(ctx, options); err != nil {
		return ErrUnavailable
	}
	return nil
}
