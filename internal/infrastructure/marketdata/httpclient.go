package marketdata

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// providerHTTPClient bundles the connection settings shared by every native
// provider: a redirect-refusing http.Client, a response size cap, and a
// concurrency-limiting semaphore.
type providerHTTPClient struct {
	client      *http.Client
	maxBodySize int64
	semaphore   chan struct{}
}

type providerHTTPOptions struct {
	Transport   http.RoundTripper
	Timeout     time.Duration
	MaxBodySize int64
	Semaphore   chan struct{}
}

func newProviderHTTPClient(options providerHTTPOptions, defaultTimeout time.Duration, defaultMaxBodyBytes int64, defaultSemaphore chan struct{}) *providerHTTPClient {
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxBodySize := options.MaxBodySize
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxBodyBytes
	}
	semaphore := options.Semaphore
	if semaphore == nil {
		semaphore = defaultSemaphore
	}
	return &providerHTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxBodySize: maxBodySize,
		semaphore:   semaphore,
	}
}

// doFetch performs a GET against requestURL under the shared semaphore and
// returns the body bytes. Providers inject their differences: prepare
// decorates the outgoing request (e.g. browser headers) and classify maps
// response status codes and headers onto domain errors, returning nil for an
// acceptable response.
func (c *providerHTTPClient) doFetch(ctx context.Context, requestURL *url.URL, prepare func(*http.Request), classify func(*http.Response) error) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-ctx.Done():
		return nil, providerUnavailable("provider request was cancelled")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		logProviderRootCause(err)
		return nil, providerUnavailable("provider is unavailable")
	}
	if prepare != nil {
		prepare(request)
	}
	response, err := c.client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUnavailable("provider request was cancelled")
		}
		logProviderRootCause(err)
		return nil, providerUnavailable("provider is unavailable")
	}
	if response == nil || response.Body == nil {
		return nil, malformedProvider()
	}
	defer response.Body.Close()
	if err := classify(response); err != nil {
		return nil, err
	}
	if response.ContentLength > c.maxBodySize {
		return nil, providerResponseTooLarge()
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodySize+1))
	if err != nil {
		logProviderRootCause(err)
		return nil, providerUnavailable("provider is unavailable")
	}
	if int64(len(body)) > c.maxBodySize {
		return nil, providerResponseTooLarge()
	}
	return body, nil
}

// logProviderRootCause records the underlying transport failure at debug level
// before it is folded into the static "provider is unavailable" message shown
// to users.
func logProviderRootCause(err error) {
	slog.Debug("marketdata: provider transport failure", "kind", sanitizeProviderError(err))
}

func sanitizeProviderError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return "timeout"
		}
		return "transport"
	}
	return "unavailable"
}

func providerResponseTooLarge() error {
	return providerError(domain.ErrMarketDataResponseTooLarge, "provider response is too large")
}
