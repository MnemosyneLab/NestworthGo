package marketdata

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type tiingoIEXRow struct {
	Ticker            string          `json:"ticker"`
	Timestamp         string          `json:"timestamp"`
	QuoteTimestamp    string          `json:"quoteTimestamp"`
	LastSaleTimestamp string          `json:"lastSaleTimestamp"`
	Last              json.RawMessage `json:"last"`
	TngoLast          json.RawMessage `json:"tngoLast"`
}

func QualifyTiingoLatest(meta vnextFixtureMeta, body []byte) (application.LatestInstrumentQuote, error) {
	if !domain.USListedEquityMarket(meta.Market) {
		return application.LatestInstrumentQuote{}, providerError(domain.ErrUnsupportedProviderSymbol, "provider symbol is unsupported")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var rows []tiingoIEXRow
	if err := decoder.Decode(&rows); err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	wanted := strings.ToUpper(strings.TrimSpace(meta.ProviderSymbol))
	var selected *tiingoIEXRow
	for index := range rows {
		if strings.ToUpper(strings.TrimSpace(rows[index].Ticker)) == wanted {
			selected = &rows[index]
			break
		}
	}
	if selected == nil && len(rows) == 1 && wanted == "" {
		selected = &rows[0]
	}
	if selected == nil {
		return application.LatestInstrumentQuote{}, unsupportedProviderSymbol()
	}
	raw := selected.Last
	if len(raw) == 0 || isJSONNull(raw) {
		raw = selected.TngoLast
	}
	if len(raw) == 0 || isJSONNull(raw) {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	lexeme, err := jsonNumberLexeme(raw)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	price, err := domain.ParseUnitPrice(lexeme)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	currency, err := domain.ParseCurrency(meta.QuoteCurrency)
	if err != nil {
		return application.LatestInstrumentQuote{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	clock, err := parseClock(meta.Clock)
	if err != nil {
		clock = time.Now().UTC()
	}
	quotedAt, err := parseTiingoLatestTimestamp(*selected, clock)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	return application.LatestInstrumentQuote{
		Price:     price,
		Currency:  currency,
		SourceKey: application.TiingoProviderKey,
		QuotedAt:  quotedAt,
		Delayed:   true,
	}, nil
}

func parseTiingoLatestTimestamp(row tiingoIEXRow, clock time.Time) (time.Time, error) {
	for _, value := range []string{row.Timestamp, row.QuoteTimestamp, row.LastSaleTimestamp} {
		parsed := parseTiingoDateLabel(value)
		if parsed.IsZero() {
			continue
		}
		return application.NormalizeProviderObservationTime(parsed.UTC(), clock)
	}
	return time.Time{}, malformedProvider()
}
