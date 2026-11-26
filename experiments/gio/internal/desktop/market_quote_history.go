package desktop

import (
	"context"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const marketQuoteEarliest = "1970-01-01"

var marketQuoteSources = [...]domain.QuoteSourceFilter{
	domain.QuoteSourceFilterAll, domain.QuoteSourceFilterManual,
	domain.QuoteSourceFilterProvider, domain.QuoteSourceFilterAgent,
}

type marketQuoteHistoryReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	InstrumentQuoteSeries(context.Context, domain.InstrumentID, domain.TrendRange, domain.QuoteSourceFilter) (domain.QuoteSeries, error)
	FXQuoteSeries(context.Context, domain.CurrencyCode, domain.CurrencyCode, domain.TrendRange, domain.QuoteSourceFilter) (domain.QuoteSeries, error)
	CurrentInstrumentQuote(context.Context, domain.InstrumentID) (*domain.InstrumentQuote, error)
	CurrentFXQuote(context.Context, domain.CurrencyCode, domain.CurrencyCode) (*domain.FXQuote, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	ListFXPreferences(context.Context) ([]domain.FXPreference, error)
}

type marketQuoteHistoryTarget struct {
	instrument domain.InstrumentID
	name       string
	pair       marketFXPair
	finding    healthFindingFocus
}

type marketQuoteHistoryVisit struct {
	target  marketQuoteHistoryTarget
	source  event.Tag
	window  analysisDateWindow
	filter  int
	reverse bool
	scroll  layout.Position
}

type marketQuoteHistoryRead struct {
	series       domain.QuoteSeries
	instrument   *domain.Instrument
	current      *domain.QuoteSeriesPoint
	storedPair   marketFXPair
	fxSource     domain.QuoteSourceKind
	location     *time.Location
	latest       string
	window       analysisDateWindow
	observedDays int
}

type marketQuoteHistory struct {
	visit                                     marketQuoteHistoryVisit
	sheet                                     analysisSheet
	dates                                     analysisDateShortcuts
	request                                   <-chan historyResult[marketQuoteHistoryRead]
	cancel                                    context.CancelFunc
	result                                    *marketQuoteHistoryRead
	err                                       error
	filters                                   [len(marketQuoteSources)]widget.Clickable
	swap, showAll, table, health, attribution widget.Clickable
	attributionCopied                         bool
	chart                                     charts.LineChart
	selected                                  charts.Selection
}

func (p *marketDataPage) openQuoteHistory(visit marketQuoteHistoryVisit) {
	p.closeQuoteHistory()
	p.quoteHistory = &marketQuoteHistory{visit: visit, sheet: newAnalysisSheet()}
	p.quoteHistory.sheet.list.Position = visit.scroll
	p.readQuoteHistory()
}

func (p *marketDataPage) closeQuoteHistory() {
	if d := p.quoteHistory; d != nil && d.cancel != nil {
		d.cancel()
	}
	p.quoteHistory = nil
}

func (p *marketDataPage) readQuoteHistory() {
	d := p.quoteHistory
	if d.cancel != nil {
		d.cancel()
	}
	ctx, cancel := context.WithCancel(p.ctx)
	d.cancel, d.err = cancel, nil
	visit := d.visit
	var previous *marketQuoteHistoryRead
	if d.result != nil {
		previous = d.result
	}
	now := p.quoteHistoryClock()
	d.request = runHistoryRead(&p.workers, p.invalidate, func() (marketQuoteHistoryRead, error) {
		return readMarketQuoteHistory(ctx, p.quoteHistoryReader, now, visit, previous)
	})
}

func (d *marketQuoteHistory) update() {
	select {
	case response := <-d.request:
		d.cancel()
		d.request, d.cancel, d.err = nil, nil, response.err
		if response.err == nil {
			d.result = &response.value
			d.visit.window = response.value.window
		}
	default:
	}
}

func (v marketQuoteHistoryVisit) direction() marketFXPair {
	if v.reverse {
		return marketFXPair{v.target.pair.second, v.target.pair.first}
	}
	return v.target.pair
}

func readMarketQuoteHistory(ctx context.Context, reader marketQuoteHistoryReader, now time.Time, visit marketQuoteHistoryVisit, previous *marketQuoteHistoryRead) (marketQuoteHistoryRead, error) {
	var result marketQuoteHistoryRead
	if previous != nil {
		result.location, result.instrument = previous.location, previous.instrument
		result.current, result.storedPair = previous.current, previous.storedPair
		result.fxSource = previous.fxSource
	} else {
		var err error
		result, err = readMarketQuoteHistoryTarget(ctx, reader, visit.target)
		if err != nil {
			return result, err
		}
	}
	result.latest = max(marketQuoteEarliest, now.In(result.location).Format(time.DateOnly))
	result.window = visit.window
	if result.window.from == "" {
		result.window = analysisPresetRange(2, result.latest, marketQuoteEarliest)
	}
	result.window.from = max(marketQuoteEarliest, min(result.latest, result.window.from))
	result.window.to = max(result.window.from, min(result.window.to, result.latest))
	period := domain.TrendRange(result.window.from + ":" + result.window.to)
	if result.window.from == marketQuoteEarliest && result.window.to == result.latest {
		period = domain.TrendAllTime
	}
	filter := marketQuoteSources[visit.filter]
	var err error
	if visit.target.instrument != "" {
		result.series, err = reader.InstrumentQuoteSeries(ctx, visit.target.instrument, period, filter)
	} else {
		pair := visit.direction()
		result.series, err = reader.FXQuoteSeries(ctx, pair.first, pair.second, period, filter)
	}
	dates := make(map[string]struct{})
	for _, observation := range result.series.Observations {
		date := observation.EffectiveDate
		if date == "" && !observation.QuotedAt.IsZero() {
			date = observation.QuotedAt.In(result.location).Format(time.DateOnly)
		}
		if date != "" {
			dates[date] = struct{}{}
		}
	}
	result.observedDays = len(dates)
	return result, err
}

func readMarketQuoteHistoryTarget(ctx context.Context, reader marketQuoteHistoryReader, target marketQuoteHistoryTarget) (marketQuoteHistoryRead, error) {
	result := marketQuoteHistoryRead{location: time.UTC}
	origin, err := reader.HistoryOrigin(ctx)
	if err != nil {
		return result, err
	}
	if origin != nil {
		if loc, err := time.LoadLocation(origin.Timezone); err == nil {
			result.location = loc
		}
	}
	if target.instrument != "" {
		instruments, err := reader.ListInstruments(ctx, true)
		if err != nil {
			return result, err
		}
		for _, instrument := range instruments {
			if instrument.ID == target.instrument {
				result.instrument = &instrument
				break
			}
		}
		quote, err := reader.CurrentInstrumentQuote(ctx, target.instrument)
		if quote != nil {
			result.current = &domain.QuoteSeriesPoint{
				ID: quote.ID.String(), SourceKind: quote.SourceKind, SourceKey: quote.SourceKey, Value: quote.UnitPrice.Canonical(), Currency: quote.Currency,
				QuotedAt: quote.QuotedAt, EffectiveDate: quote.EffectiveDate, ObservationKind: quote.ObservationKind, PriceBasis: quote.PriceBasis,
				TimestampBasis: quote.TimestampBasis, Delayed: quote.Delayed,
			}
		}
		return result, err
	}
	preferences, err := reader.ListFXPreferences(ctx)
	if err != nil {
		return result, err
	}
	a, b, err := domain.NormalizeFXPair(target.pair.first, target.pair.second)
	if err != nil {
		return result, err
	}
	for _, pref := range preferences {
		if a == pref.CurrencyA && b == pref.CurrencyB {
			result.fxSource = pref.SourceKind
			break
		}
	}
	q, err := reader.CurrentFXQuote(ctx, a, b)
	if q != nil {
		result.storedPair = marketFXPair{q.BaseCurrency, q.QuoteCurrency}
		result.current = &domain.QuoteSeriesPoint{
			ID: q.ID.String(), SourceKind: q.SourceKind, SourceKey: q.SourceKey, Value: q.Rate.Canonical(), QuotedAt: q.QuotedAt,
			EffectiveDate: q.EffectiveDate, ObservationKind: q.ObservationKind, TimestampBasis: q.TimestampBasis, Delayed: q.Delayed,
		}
	}
	return result, err
}

func marketQuoteFindingWindow(f healthFindingFocus) analysisDateWindow {
	if _, fromOK := ui.ParseCivilDay(f.from); !fromOK {
		return analysisDateWindow{}
	}
	to := f.to
	if _, toOK := ui.ParseCivilDay(to); !toOK || to < f.from {
		to = f.from
	}
	return analysisDateWindow{from: f.from, to: to}
}
