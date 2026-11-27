package storage

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

type currentQuoteOrder struct {
	source              domain.QuoteSourceKind
	quotedAt, createdAt time.Time
	id, date            string
	rawAt               time.Time
	revision            int
}

type currentQuoteSelection[T any] struct {
	values [3]T // realtime, daily, converted metal
	orders [3]currentQuoteOrder
	set    [3]bool
}

func (s *currentQuoteSelection[T]) add(value T, order currentQuoteOrder) {
	kind := 0
	if order.date != "" {
		kind = 1
	} else if !order.rawAt.IsZero() {
		kind = 2
	}
	previous := s.orders[kind]
	later := false
	switch {
	case kind == 1 && order.date != previous.date:
		later = order.date > previous.date
	case kind == 2 && !order.rawAt.Equal(previous.rawAt):
		later = order.rawAt.After(previous.rawAt)
	case kind == 2 && order.revision != previous.revision:
		later = order.revision > previous.revision
	case kind == 2:
		later = order.createdAt.After(previous.createdAt) || (order.createdAt.Equal(previous.createdAt) && order.id > previous.id)
	default:
		later = currentQuoteLater(order.source, order.quotedAt, order.createdAt, order.id, previous.source, previous.quotedAt, previous.createdAt, previous.id)
		if kind == 1 && order.source != previous.source && (order.source == domain.QuoteSourceAgent || previous.source == domain.QuoteSourceAgent) {
			later = order.source == domain.QuoteSourceAgent
		}
	}
	if !s.set[kind] || later {
		s.values[kind], s.orders[kind], s.set[kind] = value, order, true
	}
}

func (s *currentQuoteSelection[T]) selected() T {
	var value T
	var best currentQuoteOrder
	found := false
	for kind, exists := range s.set {
		if !exists {
			continue
		}
		order := s.orders[kind]
		if !found || currentQuoteLater(order.source, order.quotedAt, order.createdAt, order.id, best.source, best.quotedAt, best.createdAt, best.id) {
			value, best, found = s.values[kind], order, true
		}
	}
	return value
}

func instrumentQuoteOrder(q domain.InstrumentQuote) currentQuoteOrder {
	order := currentQuoteOrder{source: q.SourceKind, quotedAt: q.QuotedAt, createdAt: q.CreatedAt, id: q.ID.String(), revision: q.Revision}
	if q.ObservationKind == "close" {
		order.date = q.EffectiveDate
	} else if q.SourceKind == domain.QuoteSourceProvider {
		order.rawAt, _ = domain.MetalConversionRawQuotedAt(q.ConversionJSON)
	}
	return order
}

func SelectQuote(instrument domain.Instrument, quotes []domain.InstrumentQuote) *domain.InstrumentQuote {
	var selection currentQuoteSelection[*domain.InstrumentQuote]
	for index := range quotes {
		quote := &quotes[index]
		if quote.InstrumentID != instrument.ID || !quoteSourceAllowed(instrument.QuoteSource, quote.SourceKind) || quote.Currency != instrument.QuoteCurrency {
			continue
		}
		selection.add(quote, instrumentQuoteOrder(*quote))
	}
	return selection.selected()
}

// Agent observations can supplement a saved provider or manual preference.
// Selecting Agent as the preference makes it the only eligible source.
func quoteSourceAllowed(preference, source domain.QuoteSourceKind) bool {
	if source == domain.QuoteSourceAgent {
		return true
	}
	return preference == source
}

func currentQuoteLater(source domain.QuoteSourceKind, quotedAt, createdAt time.Time, id string, otherSource domain.QuoteSourceKind, otherQuotedAt, otherCreatedAt time.Time, otherID string) bool {
	if !quotedAt.Equal(otherQuotedAt) {
		return quotedAt.After(otherQuotedAt)
	}
	if source != otherSource && (source == domain.QuoteSourceAgent || otherSource == domain.QuoteSourceAgent) {
		return source == domain.QuoteSourceAgent
	}
	return quoteLater(quotedAt, createdAt, id, otherQuotedAt, otherCreatedAt, otherID)
}

func quoteLater(quotedAt, createdAt time.Time, id string, otherQuotedAt, otherCreatedAt time.Time, otherID string) bool {
	if !quotedAt.Equal(otherQuotedAt) {
		return quotedAt.After(otherQuotedAt)
	}
	if !createdAt.Equal(otherCreatedAt) {
		return createdAt.After(otherCreatedAt)
	}
	return id > otherID
}
