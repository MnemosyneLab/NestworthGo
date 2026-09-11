package marketdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type frankfurterHistoryResponse struct {
	Base      string                                `json:"base"`
	StartDate string                                `json:"start_date"`
	EndDate   string                                `json:"end_date"`
	Rates     map[string]map[string]json.RawMessage `json:"rates"`
}

func QualifyFrankfurterHistory(meta vnextFixtureMeta, body []byte) (application.MappingOutcome[application.FXDailyObservation], error) {
	base, err := domain.ParseSupportedCurrency(meta.BaseCurrency)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_currency_pair",
		}, nil
	}
	quote, err := domain.ParseSupportedCurrency(meta.QuoteCurrency)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "unsupported_currency_pair",
		}, nil
	}
	if base == quote {
		return application.MappingOutcome[application.FXDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "identity_pair_is_not_a_raw_observation",
		}, nil
	}
	_, err = parseClock(meta.Clock)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	rows, decodeErr := decodeFrankfurterHistory(body, base, quote)
	if decodeErr != nil {
		if errors.Is(decodeErr, errUnsupportedFXPair) {
			return application.MappingOutcome[application.FXDailyObservation]{
				Status: application.MappingUnsupported,
				Reason: "unsupported_currency_pair",
			}, nil
		}
		return invalidFXOutcome("malformed_response"), nil
	}
	observations := make([]application.FXDailyObservation, 0, len(rows))
	for _, row := range rows {
		rate, parseErr := domain.ParseFxRate(row.lexeme)
		if parseErr != nil {
			return invalidFXOutcome("malformed_response"), nil
		}
		eligibleAt, eligibleErr := domain.FrankfurterReferenceEligibleAt(row.date)
		if eligibleErr != nil {
			return invalidFXOutcome("malformed_market_date"), nil
		}
		effectiveAt, timeErr := application.NormalizeHistoricalObservationTime(eligibleAt)
		if timeErr != nil {
			return application.MappingOutcome[application.FXDailyObservation]{
				Status: application.MappingUncertain,
				Reason: "policy_timestamp_outside_window",
			}, nil
		}
		observations = append(observations, application.FXDailyObservation{
			MarketDate:       application.MarketDate(row.date),
			Rate:             rate.Canonical(),
			BaseCurrency:     base.String(),
			QuoteCurrency:    quote.String(),
			ValueEffectiveAt: effectiveAt,
			Kind:             application.FXObservationDailyReference,
			TimestampBasis:   application.TimestampBasisPolicyDerived,
			Derived:          false,
			SourcePolicy:     domain.FrankfurterV2BlendedPolicy,
		})
	}
	completeness, err := completenessFor(meta, false)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	return application.MappingOutcome[application.FXDailyObservation]{
		Status: mappingStatusFor(completeness, application.MappingMapped),
		Reason: completeness.Reason,
		Batch: application.HistoryBatch[application.FXDailyObservation]{
			Observations:    observations,
			VerifiedRanges:  toAppRanges(completeness.VerifiedRanges),
			PendingRanges:   toAppRanges(completeness.PendingRanges),
			UncertainRanges: toAppRanges(completeness.UncertainRanges),
			Evidence: application.ResponseEvidence{
				Adapter:         "frankfurter_v2",
				AdapterVersion:  "vnext-qualify-1",
				SourcePolicy:    domain.FrankfurterV2BlendedPolicy,
				RequestIdentity: meta.FixtureID,
				TimestampBasis:  application.TimestampBasisPolicyDerived,
			},
		},
	}, nil
}

var errUnsupportedFXPair = errors.New("unsupported_currency_pair")

type frankfurterHistoryRow struct {
	date   string
	lexeme string
}

func decodeFrankfurterHistory(body []byte, base, quote domain.CurrencyCode) ([]frankfurterHistoryRow, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("malformed_response")
	}
	if trimmed[0] == '[' {
		return decodeFrankfurterV2Rates(trimmed, base, quote)
	}
	return decodeFrankfurterTimeseries(trimmed, base, quote)
}

func decodeFrankfurterTimeseries(body []byte, base, quote domain.CurrencyCode) ([]frankfurterHistoryRow, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response frankfurterHistoryResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("malformed_response")
	}
	responseBase, err := domain.ParseCurrency(response.Base)
	if err != nil || responseBase != base {
		return nil, errors.New("malformed_response")
	}
	dates := make([]string, 0, len(response.Rates))
	for date := range response.Rates {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	rows := make([]frankfurterHistoryRow, 0, len(dates))
	for _, date := range dates {
		quotes := response.Rates[date]
		raw, ok := quotes[quote.String()]
		if !ok {
			return nil, errUnsupportedFXPair
		}
		lexeme, lexemeErr := jsonNumberLexeme(raw)
		if lexemeErr != nil {
			return nil, errors.New("malformed_response")
		}
		rows = append(rows, frankfurterHistoryRow{date: date, lexeme: lexeme})
	}
	return rows, nil
}

func decodeFrankfurterV2Rates(body []byte, base, quote domain.CurrencyCode) ([]frankfurterHistoryRow, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var items []frankfurterRateResponse
	if err := decoder.Decode(&items); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("malformed_response")
	}
	rows := make([]frankfurterHistoryRow, 0, len(items))
	for _, item := range items {
		itemBase, err := domain.ParseCurrency(item.Base)
		if err != nil || itemBase != base {
			return nil, errors.New("malformed_response")
		}
		itemQuote, err := domain.ParseCurrency(item.Quote)
		if err != nil || itemQuote != quote {
			return nil, errUnsupportedFXPair
		}
		date, err := domain.ParseMarketDate(item.Date)
		if err != nil {
			return nil, errors.New("malformed_response")
		}
		lexeme, err := jsonNumberLexeme(item.Rate)
		if err != nil {
			return nil, errors.New("malformed_response")
		}
		rows = append(rows, frankfurterHistoryRow{date: date, lexeme: lexeme})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].date < rows[j].date })
	return rows, nil
}

func invalidFXOutcome(reason string) application.MappingOutcome[application.FXDailyObservation] {
	return application.MappingOutcome[application.FXDailyObservation]{
		Status: application.MappingInvalid,
		Reason: reason,
	}
}
