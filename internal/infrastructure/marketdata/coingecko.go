package marketdata

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var coinGeckoIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var sharedCoinGeckoSemaphore = make(chan struct{}, 1)

type CoinGeckoProvider struct {
	conn        *providerHTTPClient
	apiKey      func() (string, error)
	now         func() time.Time
	mu          sync.Mutex
	cache       map[string]coinGeckoCachedResponse
	nextRequest time.Time
}
type coinGeckoCachedResponse struct {
	body    []byte
	expires time.Time
}

func NewCoinGeckoProvider(key func() (string, error), transport http.RoundTripper) *CoinGeckoProvider {
	return &CoinGeckoProvider{
		conn:   newProviderHTTPClient(providerHTTPOptions{Transport: transport}, 15*time.Second, 8*1024*1024, sharedCoinGeckoSemaphore),
		apiKey: key, now: time.Now, cache: map[string]coinGeckoCachedResponse{},
	}
}
func (p *CoinGeckoProvider) Key() string { return domain.CoinGeckoProviderKey }
func (p *CoinGeckoProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, InstrumentDailyHistory: true, InstrumentSearch: true}
}
func (p *CoinGeckoProvider) LocalConfigStatus(context.Context) (string, string) {
	if p.apiKey == nil {
		return application.ProviderConfigMissingKey, "coingecko_key_missing"
	}
	key, err := p.apiKey()
	if err != nil {
		return application.ProviderConfigUnavailable, "settings"
	}
	if strings.TrimSpace(key) == "" {
		return application.ProviderConfigMissingKey, "coingecko_key_missing"
	}
	return application.ProviderConfigOK, ""
}
func (p *CoinGeckoProvider) fetch(ctx context.Context, path string, params url.Values, ttl time.Duration) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.apiKey == nil {
		return nil, providerError(domain.ErrProviderAuthentication, "CoinGecko API key is required")
	}
	key, err := p.apiKey()
	if err != nil || strings.TrimSpace(key) == "" {
		return nil, providerError(domain.ErrProviderAuthentication, "CoinGecko API key is required")
	}
	u := &url.URL{Scheme: "https", Host: "api.coingecko.com", Path: "/api/v3" + path, RawQuery: params.Encode()}
	// Serialize cache misses and pace requests across search, latest and history.
	p.mu.Lock()
	defer p.mu.Unlock()
	cacheKey := fmt.Sprintf("%x:%s", sha256.Sum256([]byte(key)), u.String())
	if item, ok := p.cache[cacheKey]; ok && p.now().Before(item.expires) {
		return item.body, nil
	}
	delay := time.Until(p.nextRequest)
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	p.nextRequest = time.Now().Add(time.Second)
	body, err := p.conn.doFetch(ctx, u, func(r *http.Request) {
		r.Header.Set("x-cg-demo-api-key", strings.TrimSpace(key))
		r.Header.Set("Accept", "application/json")
	}, classifyTiingoResponse)
	if err != nil {
		return nil, err
	}
	if len(p.cache) >= 256 {
		p.cache = map[string]coinGeckoCachedResponse{}
	}
	p.cache[cacheKey] = coinGeckoCachedResponse{body: body, expires: p.now().Add(ttl)}
	return body, nil
}
func (p *CoinGeckoProvider) SearchInstruments(ctx context.Context, query, instrumentType string, limit int) ([]application.InstrumentSearchHit, error) {
	if instrumentType != "crypto" {
		return nil, unsupportedProviderSymbol()
	}
	body, err := p.fetch(ctx, "/search", url.Values{"query": {strings.TrimSpace(query)}}, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	var response struct {
		Coins []struct{ ID, Name, Symbol string } `json:"coins"`
	}
	if json.Unmarshal(body, &response) != nil || response.Coins == nil {
		return nil, malformedProvider()
	}
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	hits := make([]application.InstrumentSearchHit, 0)
	for _, coin := range response.Coins {
		if !coinGeckoIDPattern.MatchString(coin.ID) || coin.Name == "" || coin.Symbol == "" {
			continue
		}
		hits = append(hits, application.InstrumentSearchHit{ProviderKey: p.Key(), ProviderSymbol: coin.ID, Name: coin.Name, Symbol: strings.ToUpper(coin.Symbol), Type: "crypto", MarketCode: "CRYPTO", QuoteCurrency: "USD", Exchange: "CoinGecko"})
		if len(hits) == limit {
			break
		}
	}
	return hits, nil
}
func coinGeckoIdentity(identity application.InstrumentMarketIdentity) (string, error) {
	id := strings.ToLower(strings.TrimSpace(identity.ProviderSymbol))
	if !domain.InstrumentUsesCryptoDailyBar(identity.InstrumentType, identity.Market) || !coinGeckoIDPattern.MatchString(id) {
		return "", unsupportedProviderSymbol()
	}
	return id, nil
}
func coinGeckoPrice(value json.Number) (domain.UnitPrice, error) {
	d, err := decimal.NewFromString(value.String())
	if err != nil || !d.IsPositive() {
		return domain.UnitPrice{}, malformedProvider()
	}
	rounded := d.Round(8)
	if rounded.IsZero() {
		return domain.UnitPrice{}, providerError(domain.ErrUnavailable, "price is below supported precision")
	}
	return domain.NewUnitPrice(rounded)
}
func (p *CoinGeckoProvider) LatestInstrument(ctx context.Context, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	id, err := coinGeckoIdentity(identity)
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	body, err := p.fetch(ctx, "/simple/price", url.Values{"ids": {id}, "vs_currencies": {strings.ToLower(identity.QuoteCurrency.String())}, "include_last_updated_at": {"true"}, "precision": {"full"}}, 60*time.Second)
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	return p.decodeLatest(body, identity)
}
func (p *CoinGeckoProvider) decodeLatest(body []byte, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	var response map[string]map[string]json.Number
	if json.Unmarshal(body, &response) != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	item := response[strings.ToLower(strings.TrimSpace(identity.ProviderSymbol))]
	price, err := coinGeckoPrice(item[strings.ToLower(identity.QuoteCurrency.String())])
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	stamp, err := item["last_updated_at"].Int64()
	if err != nil || stamp <= 0 || time.Unix(stamp, 0).After(p.now().Add(time.Minute)) {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	return application.LatestInstrumentQuote{Price: price, Currency: identity.QuoteCurrency, SourceKey: p.Key(), QuotedAt: time.Unix(stamp, 0).UTC(), Delayed: true}, nil
}
func (p *CoinGeckoProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, providerError(domain.ErrUnavailable, "CoinGecko does not supply FX")
}

// Daily reference points retain their actual 00:00 UTC timestamp. They are
// not shifted to the previous date or labelled as an exchange session close.
func (p *CoinGeckoProvider) InstrumentDailyHistory(ctx context.Context, identity application.InstrumentMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	empty := application.MappingOutcome[application.InstrumentDailyObservation]{}
	id, err := coinGeckoIdentity(identity)
	if err != nil {
		return empty, err
	}
	dates, err := application.InclusiveMarketDates(rng)
	if err != nil {
		return empty, err
	}
	start, _ := time.Parse("2006-01-02", string(rng.Start))
	end, _ := time.Parse("2006-01-02", string(rng.End))
	if start.Before(p.now().UTC().AddDate(0, 0, -365)) {
		return empty, providerError(domain.ErrUnavailable, "coingecko_history_limit_365_days")
	}
	body, err := p.fetch(ctx, "/coins/"+id+"/market_chart/range", url.Values{
		"vs_currency": {strings.ToLower(identity.QuoteCurrency.String())}, "from": {start.Format("2006-01-02")}, "to": {end.AddDate(0, 0, 1).Format("2006-01-02")},
		"interval": {"daily"}, "precision": {"full"},
	}, 5*time.Minute)
	if err != nil {
		return empty, err
	}
	var response struct {
		Prices [][]json.Number `json:"prices"`
	}
	if json.Unmarshal(body, &response) != nil || response.Prices == nil {
		return empty, malformedProvider()
	}
	batch := application.HistoryBatch[application.InstrumentDailyObservation]{Evidence: application.ResponseEvidence{
		Adapter: p.Key(), AdapterVersion: "coingecko-demo-v1", SourcePolicy: domain.CoinGeckoDailyPriceBasis, PriceBasis: application.PriceBasis(domain.CoinGeckoDailyPriceBasis),
		TimestampBasis: application.TimestampBasisObservedPublication, SessionPolicy: "utc_daily_reference", RequestIdentity: id,
	}}
	seen := map[string]bool{}
	for _, point := range response.Prices {
		if len(point) != 2 {
			return empty, malformedProvider()
		}
		ms, e := point[0].Int64()
		if e != nil {
			return empty, malformedProvider()
		}
		stamp := time.UnixMilli(ms).UTC()
		date := stamp.Format("2006-01-02")
		if date < string(rng.Start) || date > string(rng.End) {
			continue
		}
		// Ignore the trailing live point and wait for the documented daily cache update.
		if stamp.Hour() != 0 || stamp.Minute() != 0 || stamp.Second() != 0 || stamp.Nanosecond() != 0 || p.now().Before(stamp.Add(40*time.Minute)) {
			continue
		}
		if seen[date] {
			return empty, malformedProvider()
		}
		price, e := coinGeckoPrice(point[1])
		if e != nil {
			return empty, e
		}
		seen[date] = true
		batch.Observations = append(batch.Observations, application.InstrumentDailyObservation{
			MarketDate: application.MarketDate(date), Value: price.Canonical(), Currency: identity.QuoteCurrency.String(),
			ValueEffectiveAt: stamp, ProviderTimestamp: stamp, Kind: application.InstrumentObservationClose,
			PriceBasis: application.PriceBasis(domain.CoinGeckoDailyPriceBasis), TimestampBasis: application.TimestampBasisObservedPublication,
		})
		batch.VerifiedRanges = append(batch.VerifiedRanges, application.DateRange{Start: application.MarketDate(date), End: application.MarketDate(date)})
	}
	for _, date := range dates {
		if !seen[string(date)] {
			batch.PendingRanges = append(batch.PendingRanges, application.DateRange{Start: date, End: date})
		}
	}
	status := application.MappingMapped
	reason := ""
	if len(batch.PendingRanges) > 0 {
		status = application.MappingPending
		reason = "coingecko_daily_reference_missing"
		next := p.now().Add(time.Hour)
		batch.NextCheckAt = &next
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{Status: status, Reason: reason, Batch: batch}, nil
}

// Batch by quote currency and coin ID; each item still carries an independent
// error so a missing token cannot poison the rest of the portfolio.
func (p *CoinGeckoProvider) LatestInstruments(ctx context.Context, identities []application.InstrumentMarketIdentity) map[string]application.LatestInstrumentResult {
	results := map[string]application.LatestInstrumentResult{}
	groups := map[string][]application.InstrumentMarketIdentity{}
	for _, identity := range identities {
		if _, err := coinGeckoIdentity(identity); err != nil {
			results[application.LatestInstrumentIdentityKey(identity)] = application.LatestInstrumentResult{Err: err}
			continue
		}
		groups[identity.QuoteCurrency.String()] = append(groups[identity.QuoteCurrency.String()], identity)
	}
	var stopped error
	for currency, group := range groups {
		if stopped != nil {
			for _, identity := range group {
				results[application.LatestInstrumentIdentityKey(identity)] = application.LatestInstrumentResult{Err: stopped}
			}
			continue
		}
		for start := 0; start < len(group); start += 100 {
			end := start + 100
			if end > len(group) {
				end = len(group)
			}
			chunk := group[start:end]
			ids := []string{}
			seen := map[string]bool{}
			for _, identity := range chunk {
				id := strings.ToLower(strings.TrimSpace(identity.ProviderSymbol))
				if !seen[id] {
					ids = append(ids, id)
					seen[id] = true
				}
			}
			body, err := p.fetch(ctx, "/simple/price", url.Values{"ids": {strings.Join(ids, ",")}, "vs_currencies": {strings.ToLower(currency)}, "include_last_updated_at": {"true"}, "precision": {"full"}}, 60*time.Second)
			for _, identity := range chunk {
				result := application.LatestInstrumentResult{Err: err}
				if err == nil {
					result.Quote, result.Err = p.decodeLatest(body, identity)
				}
				results[application.LatestInstrumentIdentityKey(identity)] = result
			}
			if err != nil {
				stopped = err
				for _, identity := range group[end:] {
					results[application.LatestInstrumentIdentityKey(identity)] = application.LatestInstrumentResult{Err: err}
				}
				break
			}
		}
	}
	return results
}

func (p *CoinGeckoProvider) SupportedQuoteCurrencies(ctx context.Context) ([]string, error) {
	body, err := p.fetch(ctx, "/simple/supported_vs_currencies", url.Values{}, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	var values []string
	if json.Unmarshal(body, &values) != nil || len(values) == 0 {
		return nil, malformedProvider()
	}
	result := []string{}
	for _, value := range values {
		if currency, err := domain.ParseSupportedCurrency(strings.ToUpper(value)); err == nil {
			result = append(result, currency.String())
		}
	}
	return result, nil
}
