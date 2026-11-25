package desktop

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketDataReader interface {
	overviewReader
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
	CurrentInstrumentQuote(context.Context, domain.InstrumentID) (*domain.InstrumentQuote, error)
	ListFXPreferences(context.Context) ([]domain.FXPreference, error)
	CurrentFXQuote(context.Context, domain.CurrencyCode, domain.CurrencyCode) (*domain.FXQuote, error)
}

type marketTab int

const (
	marketInstruments marketTab = iota
	marketFX
)

type marketDataVisit struct {
	tab    marketTab
	search string
	scroll [2]layout.Position
}

type marketInstrumentRow struct {
	instrument domain.Instrument
	quote      *domain.InstrumentQuote
}

type marketFXPair struct {
	first, second domain.CurrencyCode
}

type marketFXRow struct {
	pair   marketFXPair
	source domain.QuoteSourceKind
	quote  *domain.FXQuote
}

type marketDataResult struct {
	instruments []marketInstrumentRow
	fx          []marketFXRow
}

type marketDataPage struct {
	ctx        context.Context
	reader     marketDataReader
	invalidate func()
	workers    sync.WaitGroup
	cancel     context.CancelFunc
	request    <-chan historyResult[marketDataResult]
	current    [2]*marketDataResult
	err        error
	tab        marketTab
	tabs       [2]widget.Clickable
	search     ui.SearchField
	reload     widget.Clickable
	list       [2]widget.List
}

func (p *marketDataPage) enter(visit marketDataVisit) {
	p.tab = visit.tab
	if p.search.Field.Text() != visit.search {
		p.search.Field.Editor.SetText(visit.search)
	}
	for index := range p.list {
		p.list[index].Position = visit.scroll[index]
	}
	p.read()
}

func (p *marketDataPage) visit() marketDataVisit {
	state := marketDataVisit{tab: p.tab, search: p.search.Field.Text()}
	for index := range p.list {
		state.scroll[index] = p.list[index].Position
	}
	return state
}

func (p *marketDataPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *marketDataPage) read() {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err = cancel, nil
	tab := p.tab
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (marketDataResult, error) {
		if tab == marketFX {
			rows, err := readMarketFX(ctx, p.reader)
			return marketDataResult{fx: rows}, err
		}
		rows, err := readMarketInstruments(ctx, p.reader)
		return marketDataResult{instruments: rows}, err
	})
}

func (p *marketDataPage) update() {
	select {
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err == nil {
			p.current[p.tab] = &result.value
		}
	default:
	}
}

func readMarketInstruments(ctx context.Context, reader marketDataReader) ([]marketInstrumentRow, error) {
	all, err := reader.ListInstruments(ctx, true)
	if err != nil {
		return nil, err
	}
	owned, err := reader.InstrumentProductOwnership(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]marketInstrumentRow, 0, len(all))
	for _, instrument := range all {
		if owned[instrument.ID.String()] != "" {
			continue
		}
		quote, err := reader.CurrentInstrumentQuote(ctx, instrument.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, marketInstrumentRow{instrument: instrument, quote: quote})
	}
	order := domain.AllInstrumentTypes()
	slices.SortStableFunc(rows, func(a, b marketInstrumentRow) int {
		left, right := a.instrument, b.instrument
		leftType, rightType := slices.Index(order, left.Type), slices.Index(order, right.Type)
		if group := cmp.Compare(leftType, rightType); group != 0 {
			return group
		}
		if currency := cmp.Compare(left.QuoteCurrency, right.QuoteCurrency); currency != 0 {
			return currency
		}
		return cmp.Compare(strings.ToLower(left.DisplayLabel()), strings.ToLower(right.DisplayLabel()))
	})
	return rows, nil
}

func readMarketFX(ctx context.Context, reader marketDataReader) ([]marketFXRow, error) {
	preferences, err := reader.ListFXPreferences(ctx)
	if err != nil {
		return nil, err
	}
	overview, err := reader.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		return nil, err
	}
	sources := make(map[marketFXPair]domain.QuoteSourceKind, len(preferences))
	for _, preference := range preferences {
		sources[marketFXPair{preference.CurrencyA, preference.CurrencyB}] = preference.SourceKind
	}
	for _, missing := range overview.MissingInputs {
		if missing.Kind != domain.MissingFXRate {
			continue
		}
		first, second, err := domain.NormalizeFXPair(missing.BaseCurrency, missing.QuoteCurrency)
		if err != nil {
			continue
		}
		pair := marketFXPair{first, second}
		if _, exists := sources[pair]; !exists {
			sources[pair] = ""
		}
	}
	pairs := slices.SortedFunc(maps.Keys(sources), func(a, b marketFXPair) int {
		if first := cmp.Compare(a.first, b.first); first != 0 {
			return first
		}
		return cmp.Compare(a.second, b.second)
	})
	rows := make([]marketFXRow, 0, len(pairs))
	for _, pair := range pairs {
		row := marketFXRow{pair: pair, source: sources[pair]}
		if row.source != "" {
			row.quote, err = reader.CurrentFXQuote(ctx, pair.first, pair.second)
			if err != nil {
				return nil, err
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func marketInstrumentMatches(i domain.Instrument, query string) bool {
	fields := []string{i.Name, i.ShortName, i.QuoteCurrency.String()}
	for _, field := range []*string{i.Symbol, i.ISIN, i.MarketCode, i.ProviderSymbol} {
		if field != nil {
			fields = append(fields, *field)
		}
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), query)
}
