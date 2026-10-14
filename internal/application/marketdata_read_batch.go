package application

import (
	"context"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type MarketReadTarget = domain.MarketReadTarget
type MarketReadRequest = domain.MarketReadRequest
type MarketReadSnapshot = domain.MarketReadSnapshot
type MarketReadRepository interface {
	ReadMarketDataSnapshot(context.Context, MarketReadRequest, time.Time) (MarketReadSnapshot, error)
}
type MarketReadResult struct {
	CapturedAt time.Time
	Targets    []MarketReadResultTarget
}
type MarketReadResultTarget struct {
	Target            MarketReadTarget
	DisplayName       string
	Status            string
	Reasons           []string
	CurrentInstrument *domain.InstrumentQuote
	CurrentFX         *domain.FXQuote
	CurrentValue      *string
	Series            domain.QuoteSeries
}

func NormalizeMarketRead(in MarketReadRequest) (MarketReadRequest, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 100 {
		return in, &domain.Error{Code: domain.ErrValidation, Message: "targets supports 1 to 100 items"}
	}
	if in.View == "" {
		in.View = "current"
	}
	if in.View != "current" && in.View != "history" && in.View != "coverage" {
		return in, &domain.Error{Code: domain.ErrValidation, Message: "view must be current, coverage or history"}
	}
	if in.Range == "" {
		in.Range = "30d"
	}
	if _, err := domain.ParseTrendRange(in.Range); err != nil {
		return in, err
	}
	if in.SourceFilter == "" {
		in.SourceFilter = "all"
	}
	if _, err := domain.ParseQuoteSourceFilter(in.SourceFilter); err != nil {
		return in, err
	}
	in.Targets = append([]MarketReadTarget{}, in.Targets...)
	seen := map[string]bool{}
	for n, t := range in.Targets {
		if t.InstrumentID != "" {
			parsed, parseErr := domain.ParseInstrumentID(t.InstrumentID)
			if parseErr != nil {
				return in, parseErr
			}
			t.InstrumentID = parsed.String()
			in.Targets[n] = t
			if t.BaseCurrency != "" || t.QuoteCurrency != "" {
				return in, &domain.Error{Code: domain.ErrValidation, Message: "choose instrumentId or oriented FX pair"}
			}
		} else {
			base, quote, err := parseOrientedPair(domain.CurrencyCode(t.BaseCurrency), domain.CurrencyCode(t.QuoteCurrency))
			if err != nil {
				return in, err
			}
			t.BaseCurrency, t.QuoteCurrency = base.String(), quote.String()
			in.Targets[n] = t
		}
		if seen[t.Key()] {
			return in, &domain.Error{Code: domain.ErrValidation, Message: "duplicate market target"}
		}
		seen[t.Key()] = true
	}
	sort.Slice(in.Targets, func(i, j int) bool { return in.Targets[i].Key() < in.Targets[j].Key() })
	return in, nil
}

func (s *Service) ReadMarketDataBatch(ctx context.Context, in MarketReadRequest) (MarketReadResult, error) {
	in, err := NormalizeMarketRead(in)
	if err != nil {
		return MarketReadResult{}, err
	}
	repo, ok := s.repository.(MarketReadRepository)
	if !ok {
		return MarketReadResult{}, &domain.Error{Code: domain.ErrUnavailable, Message: "bounded market read storage unavailable"}
	}
	now := s.clock()
	provider := s.FXProviderKey()
	snapshot, err := repo.ReadMarketDataSnapshot(ctx, in, now)
	if err != nil {
		return MarketReadResult{}, err
	}
	location := time.UTC
	if snapshot.Origin != nil {
		location, err = time.LoadLocation(snapshot.Origin.Timezone)
		if err != nil {
			return MarketReadResult{}, err
		}
	}
	result := MarketReadResult{CapturedAt: now, Targets: []MarketReadResultTarget{}}
	r, _ := domain.ParseTrendRange(in.Range)
	for _, t := range in.Targets {
		row := MarketReadResultTarget{Target: t, DisplayName: t.Key(), Status: "missing", Reasons: []string{}, Series: domain.QuoteSeries{Range: r, Points: []domain.QuoteSeriesPoint{}, Observations: []domain.QuoteSeriesPoint{}, OutsideRange: snapshot.OutsideRange[t.Key()]}}
		if t.InstrumentID != "" {
			var instrument *domain.Instrument
			for i := range snapshot.Instruments {
				if snapshot.Instruments[i].ID.String() == t.InstrumentID {
					instrument = &snapshot.Instruments[i]
					break
				}
			}
			if instrument == nil {
				row.Status = "not_found"
				row.Reasons = append(row.Reasons, "instrument_not_found")
				result.Targets = append(result.Targets, row)
				continue
			}
			row.DisplayName = instrument.DisplayLabel()
			row.CurrentInstrument = selectInstrumentQuote(*instrument, snapshot.CurrentInstrumentQuotes)
			if convertedMetalRequiresFX(*instrument, row.CurrentInstrument) {
				a, b, _ := domain.NormalizeFXPair("USD", instrument.QuoteCurrency)
				var fx *domain.FXQuote
				for _, p := range snapshot.FXPreferences {
					if p.CurrencyA == a && p.CurrencyB == b {
						fx = selectFXQuote(p, snapshot.CurrentFXQuotes, "USD", instrument.QuoteCurrency, provider, nil)
					}
				}
				if fx == nil {
					row.CurrentInstrument = nil
					row.Reasons = append(row.Reasons, "conversion_fx_missing")
				}
			}
			if row.CurrentInstrument != nil {
				v := row.CurrentInstrument.UnitPrice.Canonical()
				row.CurrentValue = &v
				row.Status = "available"
			}
			row.Series.DisplayCurrency = instrument.QuoteCurrency
			row.Series.Observations = instrumentSeriesObservations(snapshot.InstrumentHistory[t.Key()])
		} else {
			base, quote := domain.CurrencyCode(t.BaseCurrency), domain.CurrencyCode(t.QuoteCurrency)
			a, b, _ := domain.NormalizeFXPair(base, quote)
			for _, p := range snapshot.FXPreferences {
				if p.CurrencyA == a && p.CurrencyB == b {
					row.CurrentFX = selectFXQuote(p, snapshot.CurrentFXQuotes, base, quote, provider, nil)
					break
				}
			}
			if row.CurrentFX != nil {
				v, e := fxRateInDirection(*row.CurrentFX, base, quote)
				if e != nil {
					return MarketReadResult{}, e
				}
				row.CurrentValue = &v
				row.Status = "available"
			}
			row.Series.BaseCurrency, row.Series.QuoteCurrency = base, quote
			row.Series.Observations = fxSeriesObservations(snapshot.FXHistory[t.Key()], base, quote)
		}
		row.Series.Points = chartQuotePoints(row.Series.Observations, r, location)
		if row.Status == "missing" {
			row.Reasons = append(row.Reasons, "no_selected_quote")
		}
		result.Targets = append(result.Targets, row)
	}
	return result, nil
}
func instrumentSeriesObservations(quotes []domain.InstrumentQuote) []domain.QuoteSeriesPoint {
	out := make([]domain.QuoteSeriesPoint, 0, len(quotes))
	for _, q := range quotes {
		out = append(out, domain.QuoteSeriesPoint{Currency: q.Currency, ID: q.ID.String(), Revision: q.Revision, ConversionJSON: q.ConversionJSON, ObservationKind: q.ObservationKind, EffectiveDate: q.EffectiveDate, PriceBasis: q.PriceBasis, TimestampBasis: q.TimestampBasis, QuotedAt: q.QuotedAt, CreatedAt: q.CreatedAt, Value: q.UnitPrice.Canonical(), SourceKind: q.SourceKind, SourceKey: q.SourceKey, Delayed: q.Delayed})
	}
	return out
}
func fxSeriesObservations(quotes []domain.FXQuote, base, quote domain.CurrencyCode) []domain.QuoteSeriesPoint {
	out := make([]domain.QuoteSeriesPoint, 0, len(quotes))
	for _, q := range quotes {
		v, e := fxRateInDirection(q, base, quote)
		if e != nil {
			continue
		}
		out = append(out, domain.QuoteSeriesPoint{ID: q.ID.String(), Revision: q.Revision, ObservationKind: q.ObservationKind, EffectiveDate: q.EffectiveDate, TimestampBasis: q.TimestampBasis, QuotedAt: q.QuotedAt, CreatedAt: q.CreatedAt, Value: v, SourceKind: q.SourceKind, SourceKey: q.SourceKey, Delayed: q.Delayed})
	}
	sortQuoteSeriesNewestFirst(out)
	return out
}
