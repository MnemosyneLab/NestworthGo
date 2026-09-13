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

type tiingoEODRow struct {
	Date        string          `json:"date"`
	Close       json.RawMessage `json:"close"`
	AdjClose    json.RawMessage `json:"adjClose"`
	DivCash     json.RawMessage `json:"divCash"`
	SplitFactor json.RawMessage `json:"splitFactor"`
}

func QualifyTiingoHistory(meta vnextFixtureMeta, body []byte) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	if !domain.USListedEquityMarket(meta.Market) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "tiingo_us_listed_only",
		}, nil
	}
	clock, err := parseClock(meta.Clock)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	rows, err := decodeTiingoEOD(body)
	if err != nil {
		completeness, _ := completenessFor(meta, true)
		return invalidInstrumentOutcome(completeness, "malformed_response"), nil
	}
	observations := make([]application.InstrumentDailyObservation, 0, len(rows))
	for _, row := range rows {
		mapped, mapErr := mapTiingoRow(row, meta, clock)
		if mapErr != nil {
			return application.MappingOutcome[application.InstrumentDailyObservation]{}, mapErr
		}
		if mapped.Status != application.MappingMapped {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: mapped.Status, Reason: mapped.Reason}, nil
		}
		if len(mapped.Batch.Observations) == 1 {
			observations = append(observations, mapped.Batch.Observations[0])
		}
	}
	completeness, err := completenessFor(meta, false)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	if completeness.Status == "invalid" {
		return invalidInstrumentOutcome(completeness, completeness.Reason), nil
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: mappingStatusFor(completeness, application.MappingMapped),
		Reason: completeness.Reason,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			Observations:    observations,
			VerifiedRanges:  toAppRanges(completeness.VerifiedRanges),
			PendingRanges:   toAppRanges(completeness.PendingRanges),
			UncertainRanges: toAppRanges(completeness.UncertainRanges),
			Evidence: application.ResponseEvidence{
				Adapter:         "tiingo_eod",
				AdapterVersion:  "vnext-qualify-1",
				SourcePolicy:    meta.PriceBasis,
				RequestIdentity: meta.FixtureID,
				PriceBasis:      application.PriceBasisTiingoRawClose,
				TimestampBasis:  application.TimestampBasisSessionClose,
				SessionPolicy:   meta.SessionPolicy,
			},
		},
	}, nil
}

func mapTiingoRow(row tiingoEODRow, meta vnextFixtureMeta, clock time.Time) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	marketDate, err := tiingoMarketDate(row.Date)
	if err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_market_date"), nil
	}
	if len(row.Close) == 0 || isJSONNull(row.Close) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_price_basis",
		}, nil
	}
	lexeme, err := jsonNumberLexeme(row.Close)
	if err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	price, err := domain.ParseUnitPrice(lexeme)
	if err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	session, err := domain.ResolveEquitySessionClose(marketDate, meta.Market, sessionEvidence(meta))
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	if session.Status != "mapped" {
		status := application.MappingUncertain
		if session.Status == "unsupported" {
			status = application.MappingUnsupported
		}
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: status, Reason: session.Reason}, nil
	}
	effectiveAt, err := application.NormalizeHistoricalObservationTime(session.CloseInstant)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUncertain,
			Reason: "session_timestamp_outside_window",
		}, nil
	}
	labelAt := parseTiingoDateLabel(row.Date)
	if !labelAt.IsZero() && effectiveAt.Equal(labelAt.UTC()) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{
			Status: application.MappingUncertain,
			Reason: "utc_midnight_is_not_session_close",
		}, nil
	}
	split := optionalJSONNumber(row.SplitFactor)
	div := optionalJSONNumber(row.DivCash)
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: application.MappingMapped,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			Observations: []application.InstrumentDailyObservation{{
				MarketDate:        application.MarketDate(marketDate),
				Value:             price.Canonical(),
				Currency:          meta.QuoteCurrency,
				ValueEffectiveAt:  effectiveAt,
				ProviderTimestamp: labelAt.UTC(),
				Kind:              application.InstrumentObservationClose,
				PriceBasis:        application.PriceBasisTiingoRawClose,
				TimestampBasis:    application.TimestampBasisSessionClose,
				SplitFactor:       split,
				DividendCash:      div,
			}},
		},
	}, nil
}

func decodeTiingoEOD(body []byte) ([]tiingoEODRow, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var rows []tiingoEODRow
	if err := decoder.Decode(&rows); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, err
	}
	return rows, nil
}

func tiingoMarketDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 10 {
		return domain.ParseMarketDate(trimmed[:10])
	}
	return domain.ParseMarketDate(trimmed)
}

func parseTiingoDateLabel(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func optionalJSONNumber(raw json.RawMessage) string {
	if len(raw) == 0 || isJSONNull(raw) {
		return ""
	}
	lexeme, err := jsonNumberLexeme(raw)
	if err != nil {
		return ""
	}
	return lexeme
}

func invalidInstrumentOutcome(completeness domain.HistoryCompleteness, reason string) application.MappingOutcome[application.InstrumentDailyObservation] {
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: application.MappingInvalid,
		Reason: reason,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			UncertainRanges: toAppRanges(completeness.UncertainRanges),
		},
	}
}
