package marketdata

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type yahooHistoryEnvelope struct {
	Chart struct {
		Result []yahooHistoryResult `json:"result"`
		Error  *chartError          `json:"error"`
	} `json:"chart"`
}

type yahooHistoryResult struct {
	Meta       yahooHistoryMeta `json:"meta"`
	Timestamp  []int64          `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Close []json.RawMessage `json:"close"`
		} `json:"quote"`
		Adjclose []struct {
			Adjclose []json.RawMessage `json:"adjclose"`
		} `json:"adjclose"`
	} `json:"indicators"`
}

type yahooHistoryMeta struct {
	Currency             string `json:"currency"`
	Symbol               string `json:"symbol"`
	ExchangeTimezoneName string `json:"exchangeTimezoneName"`
}

func QualifyYahooHistory(meta vnextFixtureMeta, body []byte) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var envelope yahooHistoryEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	if envelope.Chart.Error != nil || len(envelope.Chart.Result) != 1 {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	result := envelope.Chart.Result[0]
	if stringsEmpty(result.Meta.ExchangeTimezoneName) && stringsEmpty(meta.SessionTimezone) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUncertain,
			Reason: "session_timezone_unknown",
		}, nil
	}
	if len(result.Indicators.Quote) != 1 {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	hasClose := false
	for _, raw := range result.Indicators.Quote[0].Close {
		if !isJSONNull(raw) && len(raw) > 0 {
			hasClose = true
			break
		}
	}
	if !hasClose {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_price_basis",
		}, nil
	}
	if !meta.PriceBasisVerified {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_price_basis",
		}, nil
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: application.MappingUnsupported,
		Reason: "yahoo_history_not_qualified",
	}, nil
}

func yahooHistoryRequestMeta(identity application.InstrumentMarketIdentity, rng application.DateRange) vnextFixtureMeta {
	meta := vnextFixtureMeta{
		FixtureID:          "yahoo-history-request",
		Provider:           yahooProviderKey,
		Capability:         "InstrumentDailyHistory",
		ProviderSymbol:     identity.ProviderSymbol,
		QuoteCurrency:      identity.QuoteCurrency.String(),
		Market:             identity.Market,
		PriceBasis:         string(application.PriceBasisYahooClose),
		PriceBasisVerified: false,
		Clock:              time.Now().UTC().Format(time.RFC3339),
	}
	meta.RequestedRange.Start = string(rng.Start)
	meta.RequestedRange.End = string(rng.End)
	if finalized, err := domain.LastFinalizedUSEquityMarketDate(time.Now().UTC()); err == nil {
		meta.LastFinalizedMarketDate = finalized
	}
	return meta
}

func stringsEmpty(value string) bool { return len(value) == 0 }
