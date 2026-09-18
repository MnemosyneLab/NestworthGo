package marketdata

import (
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	yfinancemodels "github.com/wnjoon/go-yfinance/pkg/models"
	"strings"
	"time"
)

// These are futures daily reference closes, never spot prices or settlements.
func qualifyYahooMetalHistory(identity application.InstrumentMarketIdentity, rng application.DateRange, bars []yfinancemodels.Bar, metadata *yfinancemodels.ChartMeta, now time.Time) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	invalid := func(reason string) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingInvalid, Reason: reason}, nil
	}
	if identity.ProviderSymbol != "GC=F" && identity.ProviderSymbol != "SI=F" {
		return invalid("unsupported_metal_symbol")
	}
	if metadata == nil || metadata.Currency != "USD" || identity.QuoteCurrency != "USD" {
		return invalid("quote_currency_mismatch")
	}
	if !strings.EqualFold(metadata.Symbol, identity.ProviderSymbol) {
		return invalid("provider_symbol_mismatch")
	}
	if metadata.ExchangeTimezoneName != "America/New_York" {
		return invalid("session_timezone_unknown")
	}
	loc, err := time.LoadLocation(metadata.ExchangeTimezoneName)
	if err != nil {
		return invalid("session_timezone_unknown")
	}
	finalized, err := domain.LastFinalizedMetalMarketDate(now)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	end := string(rng.End)
	if end > finalized {
		end = finalized
	}
	batch := application.HistoryBatch[application.InstrumentDailyObservation]{Evidence: application.ResponseEvidence{Adapter: "go-yfinance", AdapterVersion: yahooAdapterVersion, SourcePolicy: domain.YahooRawClosePriceBasis, PriceBasis: application.PriceBasisYahooClose, TimestampBasis: application.TimestampBasisPolicyDerived, SessionPolicy: "yahoo_comex_daily_reference_v1", RequestIdentity: identity.ProviderSymbol}}
	seen := map[string]bool{}
	for _, bar := range bars {
		if bar.Date.IsZero() || !isUsableYahooPrice(bar.Close) {
			return invalid("malformed_close")
		}
		date := bar.Date.In(loc).Format("2006-01-02")
		if date < string(rng.Start) || date > end {
			continue
		}
		if seen[date] {
			return invalid("duplicate_market_date")
		}
		seen[date] = true
		price, err := domain.ParseUnitPrice(yahooPriceLexeme(bar.Close))
		if err != nil {
			return invalid("malformed_close")
		}
		eligible, err := domain.MetalDailyBarEligibleAt(date)
		if err != nil {
			return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
		}
		batch.Observations = append(batch.Observations, application.InstrumentDailyObservation{MarketDate: application.MarketDate(date), Value: price.Canonical(), Currency: "USD", ProviderTimestamp: bar.Date.UTC(), ValueEffectiveAt: eligible, Kind: application.InstrumentObservationClose, PriceBasis: application.PriceBasisYahooClose, TimestampBasis: application.TimestampBasisPolicyDerived})
	}
	if len(batch.Observations) == 0 {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUncertain, Reason: "no_metal_daily_reference"}, nil
	}
	if end >= string(rng.Start) {
		batch.VerifiedRanges = []application.DateRange{{Start: rng.Start, End: application.MarketDate(end)}}
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingMapped, Batch: batch}, nil
}
