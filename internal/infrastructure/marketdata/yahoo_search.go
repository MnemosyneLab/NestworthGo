package marketdata

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	yfinanceclient "github.com/wnjoon/go-yfinance/pkg/client"
	yfinancemodels "github.com/wnjoon/go-yfinance/pkg/models"
	yfinancesearch "github.com/wnjoon/go-yfinance/pkg/search"
)

const (
	yahooSearchLimit    = 8
	yahooSearchFetchMax = 12
)

type yahooListing struct {
	Market   string
	Country  string
	Currency string
}

type managedYahooSearch struct {
	search *yfinancesearch.Search
	client *yfinanceclient.Client
}

func (s *managedYahooSearch) Quotes(query string, maxResults int) ([]yfinancemodels.SearchQuote, error) {
	return s.search.Quotes(query, maxResults)
}

func (s *managedYahooSearch) Close() {
	if s.search != nil {
		s.search.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
}

func defaultYahooSearchFactory() (YahooSearch, error) {
	client, err := yfinanceclient.New(yfinanceclient.WithTimeout(int(yahooRequestTimeout / time.Second)))
	if err != nil {
		return nil, err
	}
	searcher, err := yfinancesearch.New(yfinancesearch.WithClient(client))
	if err != nil {
		client.Close()
		return nil, err
	}
	return &managedYahooSearch{search: searcher, client: client}, nil
}

func (p *YahooChartProvider) SearchInstruments(ctx context.Context, query, _ string, limit int) ([]application.InstrumentSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, providerValidation("query", "search query is required")
	}
	if limit <= 0 {
		limit = yahooSearchLimit
	}
	if limit > yahooSearchFetchMax {
		limit = yahooSearchFetchMax
	}
	fetchLimit := yahooSearchFetchMax
	if fetchLimit < limit {
		fetchLimit = limit
	}

	var quotes []yfinancemodels.SearchQuote
	err := p.withSearch(ctx, func(searcher YahooSearch) error {
		var searchErr error
		quotes, searchErr = searcher.Quotes(query, fetchLimit)
		return searchErr
	})
	if err != nil {
		return nil, err
	}
	return mapYahooSearchQuotes(quotes, limit), nil
}

func (p *YahooChartProvider) withSearch(ctx context.Context, operation func(YahooSearch) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.semaphore != nil {
		select {
		case p.semaphore <- struct{}{}:
			defer func() { <-p.semaphore }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	searcher, err := p.searchFactory()
	if err != nil {
		return mapYahooLibraryError(err)
	}
	if searcher == nil {
		return malformedProvider()
	}
	defer searcher.Close()
	err = operation(searcher)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return mapYahooLibraryError(err)
}

func mapYahooSearchQuotes(quotes []yfinancemodels.SearchQuote, limit int) []application.InstrumentSearchHit {
	if limit <= 0 {
		limit = yahooSearchLimit
	}
	hits := make([]application.InstrumentSearchHit, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, quote := range quotes {
		hit, ok := mapYahooSearchQuote(quote)
		if !ok {
			continue
		}
		if _, exists := seen[hit.ProviderSymbol]; exists {
			continue
		}
		seen[hit.ProviderSymbol] = struct{}{}
		hits = append(hits, hit)
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

func mapYahooSearchQuote(quote yfinancemodels.SearchQuote) (application.InstrumentSearchHit, bool) {
	providerSymbol := strings.ToUpper(strings.TrimSpace(quote.Symbol))
	if providerSymbol == "" {
		return application.InstrumentSearchHit{}, false
	}
	instrumentType, ok := yahooInstrumentType(quote.QuoteType)
	if !ok {
		return application.InstrumentSearchHit{}, false
	}
	listing := resolveYahooListing(quote.Exchange, quote.ExchangeDisp, providerSymbol)
	hit := application.InstrumentSearchHit{
		ProviderKey:    yahooProviderKey,
		ProviderSymbol: providerSymbol,
		Name:           yahooSearchName(quote),
		Symbol:         yahooLocalSymbol(providerSymbol),
		Type:           string(instrumentType),
		Exchange:       firstNonEmpty(strings.TrimSpace(quote.ExchangeDisp), strings.TrimSpace(quote.Exchange)),
	}
	if listing.Market != "" {
		hit.MarketCode = listing.Market
	}
	if listing.Country != "" {
		hit.CountryCode = listing.Country
	}
	if listing.Currency != "" {
		if currency, err := domain.ParseSupportedCurrency(listing.Currency); err == nil {
			hit.QuoteCurrency = currency.String()
		}
	}
	return hit, true
}

func yahooInstrumentType(quoteType string) (domain.InstrumentType, bool) {
	switch strings.ToUpper(strings.TrimSpace(quoteType)) {
	case "EQUITY":
		return domain.InstrumentStock, true
	case "ETF":
		return domain.InstrumentETF, true
	default:
		return "", false
	}
}

func yahooSearchName(quote yfinancemodels.SearchQuote) string {
	if name := strings.TrimSpace(quote.LongName); name != "" {
		return name
	}
	if name := strings.TrimSpace(quote.ShortName); name != "" {
		return name
	}
	return strings.ToUpper(strings.TrimSpace(quote.Symbol))
}

func yahooLocalSymbol(providerSymbol string) string {
	symbol := strings.ToUpper(strings.TrimSpace(providerSymbol))
	if index := strings.LastIndex(symbol, "."); index > 0 {
		suffix := symbol[index+1:]
		if _, ok := yahooSuffixListings[suffix]; ok {
			return symbol[:index]
		}
	}
	return symbol
}

func resolveYahooListing(exchange, exchangeDisp, providerSymbol string) yahooListing {
	if listing, ok := lookupYahooListing(exchange); ok {
		return listing
	}
	if listing, ok := lookupYahooListing(exchangeDisp); ok {
		return listing
	}
	if index := strings.LastIndex(providerSymbol, "."); index > 0 {
		if listing, ok := yahooSuffixListings[strings.ToUpper(providerSymbol[index+1:])]; ok {
			return listing
		}
	}
	return yahooListing{}
}

func lookupYahooListing(value string) (yahooListing, bool) {
	key := strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "").Replace(strings.TrimSpace(value)))
	if key == "" {
		return yahooListing{}, false
	}
	listing, ok := yahooExchangeListings[key]
	return listing, ok
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var yahooSuffixListings = map[string]yahooListing{
	"SS":  {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SZ":  {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"BJ":  {Market: "BSE", Country: "CN", Currency: "CNY"},
	"HK":  {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"T":   {Market: "TSE", Country: "JP", Currency: "JPY"},
	"SI":  {Market: "SGX", Country: "SG", Currency: "SGD"},
	"TW":  {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"TWO": {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"AX":  {Market: "ASX", Country: "AU", Currency: "AUD"},
	"L":   {Market: "LSE", Country: "GB", Currency: "GBP"},
	"DE":  {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"F":   {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"PA":  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"AS":  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"BR":  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"KS":  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"KQ":  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"SW":  {Market: "SIX", Country: "CH", Currency: "CHF"},
}

var yahooExchangeListings = map[string]yahooListing{
	"NMS":                  {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NCM":                  {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NGM":                  {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NIM":                  {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NAS":                  {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NASDAQ":               {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NASDAQGS":             {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NASDAQGM":             {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NASDAQCM":             {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"XNAS":                 {Market: "NASDAQ", Country: "US", Currency: "USD"},
	"NYQ":                  {Market: "NYSE", Country: "US", Currency: "USD"},
	"NYE":                  {Market: "NYSE", Country: "US", Currency: "USD"},
	"NYSE":                 {Market: "NYSE", Country: "US", Currency: "USD"},
	"XNYS":                 {Market: "NYSE", Country: "US", Currency: "USD"},
	"NEWYORKSTOCKEXCHANGE": {Market: "NYSE", Country: "US", Currency: "USD"},
	"ASE":                  {Market: "AMEX", Country: "US", Currency: "USD"},
	"AMX":                  {Market: "AMEX", Country: "US", Currency: "USD"},
	"AMEX":                 {Market: "AMEX", Country: "US", Currency: "USD"},
	"XASE":                 {Market: "AMEX", Country: "US", Currency: "USD"},
	"NYSEAMERICAN":         {Market: "AMEX", Country: "US", Currency: "USD"},
	"NYSEMKT":              {Market: "AMEX", Country: "US", Currency: "USD"},
	"NYSEAMEX":             {Market: "AMEX", Country: "US", Currency: "USD"},
	"PCX":                  {Market: "ARCA", Country: "US", Currency: "USD"},
	"PSE":                  {Market: "ARCA", Country: "US", Currency: "USD"},
	"ARCA":                 {Market: "ARCA", Country: "US", Currency: "USD"},
	"ARCX":                 {Market: "ARCA", Country: "US", Currency: "USD"},
	"NYA":                  {Market: "ARCA", Country: "US", Currency: "USD"},
	"NYSEARCA":             {Market: "ARCA", Country: "US", Currency: "USD"},
	"BTS":                  {Market: "BZX", Country: "US", Currency: "USD"},
	"BATS":                 {Market: "BZX", Country: "US", Currency: "USD"},
	"BZX":                  {Market: "BZX", Country: "US", Currency: "USD"},
	"CBOEBZX":              {Market: "BZX", Country: "US", Currency: "USD"},
	"CBOEBZXEXCHANGE":      {Market: "BZX", Country: "US", Currency: "USD"},
	"EDGX":                 {Market: "EDGX", Country: "US", Currency: "USD"},
	"CBOEEDGX":             {Market: "EDGX", Country: "US", Currency: "USD"},
	"IEX":                  {Market: "IEX", Country: "US", Currency: "USD"},
	"IEXG":                 {Market: "IEX", Country: "US", Currency: "USD"},
	"INVESTORSEXCHANGE":    {Market: "IEX", Country: "US", Currency: "USD"},
	"SHH":                  {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SHA":                  {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SHG":                  {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SSE":                  {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SHANGHAI":             {Market: "SSE", Country: "CN", Currency: "CNY"},
	"SHZ":                  {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"SHE":                  {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"SZE":                  {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"SZSE":                 {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"SHENZHEN":             {Market: "SZSE", Country: "CN", Currency: "CNY"},
	"BJS":                  {Market: "BSE", Country: "CN", Currency: "CNY"},
	"BJSE":                 {Market: "BSE", Country: "CN", Currency: "CNY"},
	"BEIJING":              {Market: "BSE", Country: "CN", Currency: "CNY"},
	"HKG":                  {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"HKX":                  {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"HKSE":                 {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"HKEX":                 {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"HONGKONG":             {Market: "HKEX", Country: "HK", Currency: "HKD"},
	"TYO":                  {Market: "TSE", Country: "JP", Currency: "JPY"},
	"JPX":                  {Market: "TSE", Country: "JP", Currency: "JPY"},
	"TSE":                  {Market: "TSE", Country: "JP", Currency: "JPY"},
	"TKS":                  {Market: "TSE", Country: "JP", Currency: "JPY"},
	"OSA":                  {Market: "TSE", Country: "JP", Currency: "JPY"},
	"TOKYO":                {Market: "TSE", Country: "JP", Currency: "JPY"},
	"SES":                  {Market: "SGX", Country: "SG", Currency: "SGD"},
	"SGX":                  {Market: "SGX", Country: "SG", Currency: "SGD"},
	"SIN":                  {Market: "SGX", Country: "SG", Currency: "SGD"},
	"SINGAPORE":            {Market: "SGX", Country: "SG", Currency: "SGD"},
	"TAI":                  {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"TPE":                  {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"TWSE":                 {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"TWO":                  {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"TAIWAN":               {Market: "TWSE", Country: "TW", Currency: "TWD"},
	"ASX":                  {Market: "ASX", Country: "AU", Currency: "AUD"},
	"AUSTRALIA":            {Market: "ASX", Country: "AU", Currency: "AUD"},
	"LSE":                  {Market: "LSE", Country: "GB", Currency: "GBP"},
	"LON":                  {Market: "LSE", Country: "GB", Currency: "GBP"},
	"LONDON":               {Market: "LSE", Country: "GB", Currency: "GBP"},
	"GER":                  {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"FRA":                  {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"ETR":                  {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"XETRA":                {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"FRANKFURT":            {Market: "XETRA", Country: "EU", Currency: "EUR"},
	"PAR":                  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"AMS":                  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"BRU":                  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"LIS":                  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"EPA":                  {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"EURONEXT":             {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"PARIS":                {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"AMSTERDAM":            {Market: "EURONEXT", Country: "EU", Currency: "EUR"},
	"KSC":                  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"KOE":                  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"KOS":                  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"KRX":                  {Market: "KRX", Country: "KR", Currency: "KRW"},
	"KOREA":                {Market: "KRX", Country: "KR", Currency: "KRW"},
	"EBS":                  {Market: "SIX", Country: "CH", Currency: "CHF"},
	"SWX":                  {Market: "SIX", Country: "CH", Currency: "CHF"},
	"SIX":                  {Market: "SIX", Country: "CH", Currency: "CHF"},
	"VTX":                  {Market: "SIX", Country: "CH", Currency: "CHF"},
	"SWISS":                {Market: "SIX", Country: "CH", Currency: "CHF"},
	"SIXSWISS":             {Market: "SIX", Country: "CH", Currency: "CHF"},
}
