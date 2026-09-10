package marketdata

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	tiingoProviderKey    = "tiingo"
	tiingoHost           = "api.tiingo.com"
	tiingoRequestTimeout = 8 * time.Second
	tiingoMaxBodyBytes   = int64(4 * 1024 * 1024)
)

var sharedTiingoSemaphore = make(chan struct{}, 2)

type TiingoProviderOptions struct {
	Transport   http.RoundTripper
	Timeout     time.Duration
	MaxBodySize int64
	Semaphore   chan struct{}
	Secrets     application.SecretStore
	Now         func() time.Time
}

type TiingoProvider struct {
	conn    *providerHTTPClient
	secrets application.SecretStore
	now     func() time.Time
}

func NewTiingoProvider(secrets application.SecretStore, transport http.RoundTripper) *TiingoProvider {
	return NewTiingoProviderWithOptions(TiingoProviderOptions{Secrets: secrets, Transport: transport})
}

func NewTiingoProviderWithOptions(options TiingoProviderOptions) *TiingoProvider {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &TiingoProvider{
		conn:    newProviderHTTPClient(providerHTTPOptions{Transport: options.Transport, Timeout: options.Timeout, MaxBodySize: options.MaxBodySize, Semaphore: options.Semaphore}, tiingoRequestTimeout, tiingoMaxBodyBytes, sharedTiingoSemaphore),
		secrets: options.Secrets,
		now:     now,
	}
}

func (p *TiingoProvider) Key() string { return tiingoProviderKey }

func (p *TiingoProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, InstrumentDailyHistory: true}
}

// LocalConfigStatus inspects the secret store only. It never opens an HTTP
// connection, including when the key is missing.
func (p *TiingoProvider) LocalConfigStatus(ctx context.Context) (code, reason string) {
	if p.secrets == nil {
		return application.ProviderConfigMissingKey, "tiingo_key_missing"
	}
	status, err := p.secrets.Status(ctx, application.TiingoSecretRef())
	if err != nil {
		return application.ProviderConfigUnavailable, "secret_store"
	}
	if !application.TiingoKeyConfigured(status) {
		return application.ProviderConfigMissingKey, string(status)
	}
	return application.ProviderConfigOK, ""
}

func (p *TiingoProvider) LatestInstrument(ctx context.Context, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.LatestInstrumentQuote{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	if !domain.USListedEquityMarket(identity.Market) {
		return application.LatestInstrumentQuote{}, providerError(domain.ErrUnsupportedProviderSymbol, "provider symbol is unsupported")
	}
	token, err := p.apiToken(ctx)
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	body, err := p.fetch(ctx, tiingoIEXURL(identity.ProviderSymbol, token))
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	return QualifyTiingoLatest(tiingoLatestRequestMeta(identity, p.clock()), body)
}

func (p *TiingoProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrUnavailable, Message: "provider does not support FX refresh"}
}

func (p *TiingoProvider) InstrumentDailyHistory(ctx context.Context, identity application.InstrumentMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	if !domain.USListedEquityMarket(identity.Market) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "tiingo_us_listed_only",
		}, nil
	}
	if _, err := application.InclusiveMarketDates(rng); err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	token, err := p.apiToken(ctx)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	body, err := p.fetch(ctx, tiingoEODURL(identity.ProviderSymbol, rng, token))
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	return QualifyTiingoHistory(tiingoHistoryRequestMeta(identity, rng, p.clock()), body)
}

func (p *TiingoProvider) fetch(ctx context.Context, requestURL *url.URL) ([]byte, error) {
	return p.conn.doFetch(ctx, requestURL, func(request *http.Request) {
		request.Header.Set("Accept", "application/json")
	}, classifyTiingoResponse)
}

func (p *TiingoProvider) apiToken(ctx context.Context) (string, error) {
	if p.secrets == nil {
		return "", providerError(domain.ErrUnavailable, "provider is unavailable")
	}
	value, status, err := p.secrets.Get(ctx, application.TiingoSecretRef())
	if err != nil {
		return "", err
	}
	if !application.TiingoKeyConfigured(status) || len(strings.TrimSpace(string(value))) == 0 {
		return "", providerError(domain.ErrUnavailable, "provider is unavailable")
	}
	return strings.TrimSpace(string(value)), nil
}

func (p *TiingoProvider) clock() time.Time {
	if p.now == nil {
		return time.Now().UTC()
	}
	return p.now()
}

func classifyTiingoResponse(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return providerError(domain.ErrProviderAuthentication, "provider authentication failed")
	case http.StatusTooManyRequests:
		return providerError(domain.ErrProviderRateLimit, "provider rate limit reached")
	case http.StatusNotFound:
		return unsupportedProviderSymbol()
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return malformedProvider()
	}
	if response.StatusCode >= 500 {
		return providerUnavailable("provider is unavailable")
	}
	if response.StatusCode != http.StatusOK {
		return malformedProvider()
	}
	return nil
}

func tiingoIEXURL(symbol, token string) *url.URL {
	query := url.Values{}
	query.Set("token", token)
	return &url.URL{
		Scheme:   "https",
		Host:     tiingoHost,
		Path:     "/iex/" + url.PathEscape(symbol),
		RawQuery: query.Encode(),
	}
}

func tiingoEODURL(symbol string, rng application.DateRange, token string) *url.URL {
	query := url.Values{}
	query.Set("startDate", string(rng.Start))
	query.Set("endDate", string(rng.End))
	query.Set("token", token)
	return &url.URL{
		Scheme:   "https",
		Host:     tiingoHost,
		Path:     "/tiingo/daily/" + url.PathEscape(symbol) + "/prices",
		RawQuery: query.Encode(),
	}
}

func tiingoLatestRequestMeta(identity application.InstrumentMarketIdentity, now time.Time) vnextFixtureMeta {
	return vnextFixtureMeta{
		FixtureID:      "tiingo-iex-latest-request",
		Provider:       tiingoProviderKey,
		Capability:     "LatestInstrument",
		ProviderSymbol: identity.ProviderSymbol,
		QuoteCurrency:  identity.QuoteCurrency.String(),
		Market:         identity.Market,
		Clock:          now.UTC().Format(time.RFC3339),
	}
}

func tiingoHistoryRequestMeta(identity application.InstrumentMarketIdentity, rng application.DateRange, now time.Time) vnextFixtureMeta {
	meta := vnextFixtureMeta{
		FixtureID:       "tiingo-eod-history-request",
		Provider:        tiingoProviderKey,
		Capability:      "InstrumentDailyHistory",
		ProviderSymbol:  identity.ProviderSymbol,
		QuoteCurrency:   identity.QuoteCurrency.String(),
		Market:          identity.Market,
		SessionPolicy:   domain.USEquityRegularClosePolicy,
		SessionKind:     string(domain.SessionKindRegular),
		SessionTimezone: domain.USEquitySessionTimezone,
		CloseClock:      domain.USEquityRegularCloseClock,
		PriceBasis:      string(application.PriceBasisTiingoRawClose),
		Clock:           now.UTC().Format(time.RFC3339),
	}
	meta.RequestedRange.Start = string(rng.Start)
	meta.RequestedRange.End = string(rng.End)
	if finalized, err := domain.LastFinalizedUSEquityMarketDate(now); err == nil {
		meta.LastFinalizedMarketDate = finalized
	}
	return meta
}

var _ application.MarketDataProvider = (*TiingoProvider)(nil)
var _ application.InstrumentHistoryProvider = (*TiingoProvider)(nil)
