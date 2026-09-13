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
	if !meta.PriceBasisVerified || strings.TrimSpace(meta.PriceBasis) != string(application.PriceBasisYahooClose) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_price_basis",
		}, nil
	}
	schedule, supported := domain.EquitySessionScheduleForMarket(meta.Market)
	if !supported || strings.TrimSpace(meta.SessionPolicy) != schedule.Policy || strings.TrimSpace(meta.SessionKind) != string(domain.SessionKindRegular) || strings.TrimSpace(meta.CloseClock) != schedule.CloseClock {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "session_policy_unverified",
		}, nil
	}
	if strings.TrimSpace(result.Meta.Symbol) != "" && !strings.EqualFold(strings.TrimSpace(result.Meta.Symbol), strings.TrimSpace(meta.ProviderSymbol)) {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "provider_symbol_mismatch"), nil
	}
	if strings.TrimSpace(result.Meta.Currency) != "" && !strings.EqualFold(strings.TrimSpace(result.Meta.Currency), strings.TrimSpace(meta.QuoteCurrency)) {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "quote_currency_mismatch"), nil
	}
	if len(result.Timestamp) != len(result.Indicators.Quote[0].Close) {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "misaligned_history"), nil
	}
	timezone := strings.TrimSpace(result.Meta.ExchangeTimezoneName)
	if timezone == "" {
		timezone = strings.TrimSpace(meta.SessionTimezone)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUncertain, Reason: "session_timezone_unknown"}, nil
	}
	start, startErr := domain.ParseMarketDate(meta.RequestedRange.Start)
	end, endErr := domain.ParseMarketDate(meta.RequestedRange.End)
	if startErr != nil || endErr != nil || start > end {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_requested_range"), nil
	}
	completeness, err := completenessFor(meta, false)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	observations := make([]application.InstrumentDailyObservation, 0, len(result.Timestamp))
	seenDates := map[string]struct{}{}
	for index, timestamp := range result.Timestamp {
		raw := result.Indicators.Quote[0].Close[index]
		if isJSONNull(raw) || len(raw) == 0 {
			continue
		}
		marketDate := time.Unix(timestamp, 0).In(location).Format("2006-01-02")
		if marketDate < string(start) || marketDate > string(end) {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "provider_date_outside_requested_range"}, nil
		}
		if _, duplicate := seenDates[marketDate]; duplicate {
			return invalidInstrumentOutcome(completeness, "duplicate_market_date"), nil
		}
		seenDates[marketDate] = struct{}{}
		if !dateInRanges(marketDate, completeness.VerifiedRanges) {
			continue
		}
		lexeme, numberErr := jsonNumberLexeme(raw)
		if numberErr != nil {
			return invalidInstrumentOutcome(completeness, "malformed_close"), nil
		}
		price, priceErr := domain.ParseUnitPrice(lexeme)
		if priceErr != nil {
			return invalidInstrumentOutcome(completeness, "malformed_close"), nil
		}
		evidence := sessionEvidence(meta)
		evidence.Timezone = timezone
		if evidence.Kind == "" {
			evidence.Kind = domain.SessionKindRegular
		}
		session, sessionErr := domain.ResolveEquitySessionClose(marketDate, meta.Market, evidence)
		if sessionErr != nil {
			return application.MappingOutcome[application.InstrumentDailyObservation]{}, sessionErr
		}
		if session.Status != "mapped" {
			status := application.MappingUncertain
			if session.Status == "unsupported" {
				status = application.MappingUnsupported
			}
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: status, Reason: session.Reason}, nil
		}
		observations = append(observations, application.InstrumentDailyObservation{
			MarketDate: application.MarketDate(marketDate), Value: price.Canonical(), Currency: meta.QuoteCurrency,
			ValueEffectiveAt: session.CloseInstant, ProviderTimestamp: time.Unix(timestamp, 0).UTC(),
			Kind: application.InstrumentObservationClose, PriceBasis: application.PriceBasisYahooClose,
			TimestampBasis: application.TimestampBasisSessionClose,
		})
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: mappingStatusFor(completeness, application.MappingMapped), Reason: completeness.Reason,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			Observations: observations, VerifiedRanges: toAppRanges(completeness.VerifiedRanges), PendingRanges: toAppRanges(completeness.PendingRanges), UncertainRanges: toAppRanges(completeness.UncertainRanges),
			Evidence: application.ResponseEvidence{Adapter: "yahoo_chart", AdapterVersion: "vnext-qualify-2", SourcePolicy: string(application.PriceBasisYahooClose), RequestIdentity: meta.FixtureID, PriceBasis: application.PriceBasisYahooClose, TimestampBasis: application.TimestampBasisSessionClose, SessionPolicy: meta.SessionPolicy},
		},
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
		PriceBasisVerified: true,
		SessionPolicy:      "",
		SessionKind:        string(domain.SessionKindRegular),
		SessionTimezone:    "",
		CloseClock:         "",
		Clock:              time.Now().UTC().Format(time.RFC3339),
	}
	meta.RequestedRange.Start = string(rng.Start)
	meta.RequestedRange.End = string(rng.End)
	if schedule, supported := domain.EquitySessionScheduleForMarket(identity.Market); supported {
		meta.SessionPolicy = schedule.Policy
		meta.SessionTimezone = schedule.Timezone
		meta.CloseClock = schedule.CloseClock
	}
	if finalized, err := domain.LastFinalizedEquityMarketDate(time.Now().UTC(), identity.Market); err == nil {
		meta.LastFinalizedMarketDate = finalized
	}
	return meta
}

func stringsEmpty(value string) bool { return len(value) == 0 }

func dateInRanges(date string, ranges []domain.InclusiveDateRange) bool {
	for _, rng := range ranges {
		if date >= rng.Start && date <= rng.End {
			return true
		}
	}
	return false
}
