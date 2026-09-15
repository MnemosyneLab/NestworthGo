package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	workerProviderKey    = domain.WorkerProviderKey
	workerRequestTimeout = 8 * time.Second
	workerMaxBodyBytes   = int64(4 * 1024 * 1024)
)

var sharedWorkerSemaphore = make(chan struct{}, 2)

// WorkerConfig is the local configuration needed to call the authenticated
// Nestworth market Worker. The token is intentionally kept below the Wails
// boundary and is read lazily so settings changes apply without a restart.
type WorkerConfig struct {
	BaseURL  string
	APIToken string
}

type WorkerConfigSource func() (WorkerConfig, error)

type WorkerProviderOptions struct {
	Transport   http.RoundTripper
	Timeout     time.Duration
	MaxBodySize int64
	Semaphore   chan struct{}
	Config      WorkerConfigSource
	Now         func() time.Time
}

type WorkerProvider struct {
	conn   *providerHTTPClient
	config WorkerConfigSource
	now    func() time.Time
}

func NewWorkerProvider(config WorkerConfigSource, transport http.RoundTripper) *WorkerProvider {
	return NewWorkerProviderWithOptions(WorkerProviderOptions{Config: config, Transport: transport})
}

func NewWorkerProviderWithOptions(options WorkerProviderOptions) *WorkerProvider {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &WorkerProvider{
		conn: newProviderHTTPClient(providerHTTPOptions{
			Transport: options.Transport, Timeout: options.Timeout, MaxBodySize: options.MaxBodySize, Semaphore: options.Semaphore,
		}, workerRequestTimeout, workerMaxBodyBytes, sharedWorkerSemaphore),
		config: options.Config,
		now:    now,
	}
}

func (p *WorkerProvider) Key() string { return workerProviderKey }

func (p *WorkerProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, InstrumentDailyHistory: true}
}

func (p *WorkerProvider) LocalConfigStatus(context.Context) (code, reason string) {
	cfg, err := p.configured()
	if err != nil {
		return application.ProviderConfigUnavailable, "settings"
	}
	if cfg.BaseURL == "" {
		return application.ProviderConfigMissingKey, "worker_url_missing"
	}
	if cfg.APIToken == "" {
		return application.ProviderConfigMissingKey, "worker_token_missing"
	}
	return application.ProviderConfigOK, ""
}

func (p *WorkerProvider) LatestInstrument(ctx context.Context, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.LatestInstrumentQuote{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	currency, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return application.LatestInstrumentQuote{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	cfg, err := p.configured()
	if err != nil {
		return application.LatestInstrumentQuote{}, providerError(domain.ErrProviderUnavailable, "provider is unavailable")
	}
	if cfg.BaseURL == "" || cfg.APIToken == "" {
		return application.LatestInstrumentQuote{}, providerError(domain.ErrProviderUnavailable, "provider is unavailable")
	}
	body, err := p.fetch(ctx, cfg, workerQuoteURL(cfg.BaseURL, identity.ProviderSymbol))
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	response, err := decodeWorkerQuote(body)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	if !sameSymbol(response.Symbol, identity.ProviderSymbol) || !sameCurrency(response.Currency, currency.String()) {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	lexeme, err := jsonNumberLexeme(response.Price)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	price, err := domain.ParseUnitPrice(lexeme)
	if err != nil || strings.TrimSpace(response.AsOf) == "" {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	quotedAt, err := time.Parse(time.RFC3339Nano, response.AsOf)
	if err != nil || quotedAt.IsZero() {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	return application.LatestInstrumentQuote{
		Price: price, Currency: currency, SourceKey: p.Key(), QuotedAt: quotedAt.UTC(), Delayed: true,
	}, nil
}

func (p *WorkerProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrUnavailable, Message: "provider does not support FX refresh"}
}

func (p *WorkerProvider) InstrumentDailyHistory(ctx context.Context, identity application.InstrumentMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	if _, err := application.InclusiveMarketDates(rng); err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	cfg, err := p.configured()
	if err != nil || cfg.BaseURL == "" || cfg.APIToken == "" {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, providerError(domain.ErrProviderUnavailable, "provider is unavailable")
	}
	body, err := p.fetch(ctx, cfg, workerHistoryURL(cfg.BaseURL, identity.ProviderSymbol, rng))
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	response, err := decodeWorkerHistory(body)
	if err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	if !sameSymbol(response.Symbol, identity.ProviderSymbol) || !sameCurrency(response.Currency, identity.QuoteCurrency.String()) || (response.Interval != "" && response.Interval != "1d") {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "provider_identity_mismatch"), nil
	}
	if !domain.InstrumentUsesCryptoDailyBar(identity.InstrumentType, identity.Market) {
		if _, supported := domain.EquitySessionScheduleForMarket(identity.Market); !supported {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "session_policy_unverified"}, nil
		}
	}
	completeness, evidence, effectiveAt, err := p.historyPolicy(identity, rng)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	observations := make([]application.InstrumentDailyObservation, 0, len(response.Prices))
	seen := make(map[string]struct{}, len(response.Prices))
	for _, item := range response.Prices {
		marketDate, parseErr := domain.ParseMarketDate(strings.TrimSpace(item.Date))
		if parseErr != nil || marketDate < string(rng.Start) || marketDate > string(rng.End) {
			return invalidInstrumentOutcome(completeness, "provider_date_outside_requested_range"), nil
		}
		if _, exists := seen[marketDate]; exists {
			return invalidInstrumentOutcome(completeness, "duplicate_market_date"), nil
		}
		seen[marketDate] = struct{}{}
		if !dateInRanges(marketDate, completeness.VerifiedRanges) {
			continue
		}
		lexeme, numberErr := jsonNumberLexeme(item.Close)
		if numberErr != nil {
			return invalidInstrumentOutcome(completeness, "malformed_close"), nil
		}
		price, priceErr := domain.ParseUnitPrice(lexeme)
		if priceErr != nil {
			return invalidInstrumentOutcome(completeness, "malformed_close"), nil
		}
		valueEffectiveAt, effectiveErr := effectiveAt(marketDate)
		if effectiveErr != nil {
			return application.MappingOutcome[application.InstrumentDailyObservation]{}, effectiveErr
		}
		observations = append(observations, application.InstrumentDailyObservation{
			MarketDate: application.MarketDate(marketDate), Value: price.Canonical(), Currency: identity.QuoteCurrency.String(),
			ValueEffectiveAt: valueEffectiveAt, Kind: application.InstrumentObservationClose,
			PriceBasis: application.PriceBasisWorkerYahooClose, TimestampBasis: evidence.TimestampBasis,
		})
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: mappingStatusFor(completeness, application.MappingMapped), Reason: completeness.Reason,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			Observations: observations, VerifiedRanges: toAppRanges(completeness.VerifiedRanges), PendingRanges: toAppRanges(completeness.PendingRanges), UncertainRanges: toAppRanges(completeness.UncertainRanges), Evidence: evidence,
		},
	}, nil
}

type workerQuoteResponse struct {
	Symbol   string          `json:"symbol"`
	Currency string          `json:"currency"`
	Price    json.RawMessage `json:"price"`
	AsOf     string          `json:"asOf"`
}

type workerHistoryResponse struct {
	Symbol   string               `json:"symbol"`
	Currency string               `json:"currency"`
	Interval string               `json:"interval"`
	Prices   []workerHistoryPrice `json:"prices"`
}

type workerHistoryPrice struct {
	Date  string          `json:"date"`
	Close json.RawMessage `json:"close"`
}

func decodeWorkerQuote(body []byte) (workerQuoteResponse, error) {
	var value workerQuoteResponse
	if err := decodeJSON(body, &value); err != nil {
		return workerQuoteResponse{}, err
	}
	return value, nil
}

func decodeWorkerHistory(body []byte) (workerHistoryResponse, error) {
	var value workerHistoryResponse
	if err := decodeJSON(body, &value); err != nil {
		return workerHistoryResponse{}, err
	}
	return value, nil
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (p *WorkerProvider) configured() (WorkerConfig, error) {
	if p.config == nil {
		return WorkerConfig{}, nil
	}
	value, err := p.config()
	if err != nil {
		return WorkerConfig{}, err
	}
	value.BaseURL = strings.TrimRight(strings.TrimSpace(value.BaseURL), "/")
	value.APIToken = strings.TrimSpace(value.APIToken)
	if value.BaseURL == "" {
		return value, nil
	}
	parsed, err := url.Parse(value.BaseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return WorkerConfig{}, providerError(domain.ErrUnavailable, "provider is unavailable")
	}
	return value, nil
}

func (p *WorkerProvider) fetch(ctx context.Context, cfg WorkerConfig, requestURL *url.URL) ([]byte, error) {
	return p.conn.doFetch(ctx, requestURL, func(request *http.Request) {
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}, classifyWorkerResponse)
}

func workerQuoteURL(base, symbol string) *url.URL {
	return workerEndpointURL(base, "/v1/quote/", symbol, "")
}

func workerHistoryURL(base, symbol string, rng application.DateRange) *url.URL {
	query := url.Values{}
	query.Set("from", string(rng.Start))
	query.Set("to", string(rng.End))
	return workerEndpointURL(base, "/v1/history/", symbol, query.Encode())
}

func workerEndpointURL(base, prefix, symbol, rawQuery string) *url.URL {
	parsed, _ := url.Parse(strings.TrimRight(base, "/"))
	basePath := strings.TrimRight(parsed.Path, "/")
	escapedBasePath := strings.TrimRight(parsed.EscapedPath(), "/")
	escapedSymbol := url.PathEscape(symbol)
	parsed.Path = basePath + prefix + escapedSymbol
	parsed.RawPath = escapedBasePath + prefix + escapedSymbol
	parsed.RawQuery = rawQuery
	return parsed
}

func classifyWorkerResponse(response *http.Response) error {
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

func sameSymbol(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func sameCurrency(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func (p *WorkerProvider) historyPolicy(identity application.InstrumentMarketIdentity, rng application.DateRange) (domain.HistoryCompleteness, application.ResponseEvidence, func(string) (time.Time, error), error) {
	now := p.now()
	if now.IsZero() {
		return domain.HistoryCompleteness{}, application.ResponseEvidence{}, nil, providerValidation("now", "backend clock is required")
	}
	lastFinalized := ""
	evidence := application.ResponseEvidence{
		Adapter: "nestworth_market_worker", AdapterVersion: "v1", RequestIdentity: string(identity.ProviderSymbol),
		SourcePolicy: string(application.PriceBasisWorkerYahooClose), PriceBasis: application.PriceBasisWorkerYahooClose,
	}
	var effectiveAt func(string) (time.Time, error)
	if domain.InstrumentUsesCryptoDailyBar(identity.InstrumentType, identity.Market) {
		var err error
		lastFinalized, err = domain.LastFinalizedCryptoMarketDate(now)
		if err != nil {
			return domain.HistoryCompleteness{}, application.ResponseEvidence{}, nil, err
		}
		evidence.TimestampBasis = application.TimestampBasisPolicyDerived
		evidence.SessionPolicy = domain.YahooCryptoUTCDailyBarPolicy
		effectiveAt = domain.CryptoDailyBarEligibleAt
	} else {
		schedule, supported := domain.EquitySessionScheduleForMarket(identity.Market)
		if !supported {
			return domain.HistoryCompleteness{}, application.ResponseEvidence{}, nil, nil
		}
		var err error
		lastFinalized, err = domain.LastFinalizedEquityMarketDate(now, identity.Market)
		if err != nil {
			return domain.HistoryCompleteness{}, application.ResponseEvidence{}, nil, err
		}
		evidence.TimestampBasis = application.TimestampBasisSessionClose
		evidence.SessionPolicy = schedule.Policy
		effectiveAt = func(marketDate string) (time.Time, error) {
			resolution, err := domain.ResolveEquitySessionClose(marketDate, identity.Market, domain.SessionEvidence{
				Kind: domain.SessionKindRegular, Timezone: schedule.Timezone, CloseClock: schedule.CloseClock, Policy: schedule.Policy,
			})
			if err != nil {
				return time.Time{}, err
			}
			if resolution.Status != "mapped" {
				return time.Time{}, providerError(domain.ErrMalformedProviderResponse, resolution.Reason)
			}
			return resolution.CloseInstant, nil
		}
	}
	completeness, err := domain.ClassifyHistoryCompleteness(string(rng.Start), string(rng.End), lastFinalized, nil, false, false)
	if err != nil {
		return domain.HistoryCompleteness{}, application.ResponseEvidence{}, nil, err
	}
	return completeness, evidence, effectiveAt, nil
}

var _ application.MarketDataProvider = (*WorkerProvider)(nil)
var _ application.InstrumentHistoryProvider = (*WorkerProvider)(nil)
