package marketdata

import (
	"bytes"
	"encoding/json"
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
	clock, err := parseClock(meta.Clock)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response frankfurterHistoryResponse
	if err := decoder.Decode(&response); err != nil {
		return invalidFXOutcome("malformed_response"), nil
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalidFXOutcome("malformed_response"), nil
	}
	responseBase, err := domain.ParseCurrency(response.Base)
	if err != nil || responseBase != base {
		return invalidFXOutcome("malformed_response"), nil
	}
	observations := make([]application.FXDailyObservation, 0, len(response.Rates))
	dates := make([]string, 0, len(response.Rates))
	for date := range response.Rates {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	for _, date := range dates {
		quotes := response.Rates[date]
		raw, ok := quotes[quote.String()]
		if !ok {
			return application.MappingOutcome[application.FXDailyObservation]{
				Status: application.MappingUnsupported,
				Reason: "unsupported_currency_pair",
			}, nil
		}
		lexeme, lexemeErr := jsonNumberLexeme(raw)
		if lexemeErr != nil {
			return invalidFXOutcome("malformed_response"), nil
		}
		rate, parseErr := domain.ParseFxRate(lexeme)
		if parseErr != nil {
			return invalidFXOutcome("malformed_response"), nil
		}
		eligibleAt, eligibleErr := domain.FrankfurterReferenceEligibleAt(date)
		if eligibleErr != nil {
			return invalidFXOutcome("malformed_market_date"), nil
		}
		effectiveAt, timeErr := application.NormalizeProviderObservationTime(eligibleAt, clock)
		if timeErr != nil {
			return application.MappingOutcome[application.FXDailyObservation]{
				Status: application.MappingUncertain,
				Reason: "policy_timestamp_outside_window",
			}, nil
		}
		observations = append(observations, application.FXDailyObservation{
			MarketDate:       application.MarketDate(date),
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

func invalidFXOutcome(reason string) application.MappingOutcome[application.FXDailyObservation] {
	return application.MappingOutcome[application.FXDailyObservation]{
		Status: application.MappingInvalid,
		Reason: reason,
	}
}
