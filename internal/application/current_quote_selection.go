package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Resolve daily overlays before comparing them with realtime observations.
// Pairwise daily-source overrides mixed with timestamp comparisons form a cycle.
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
	if q.ObservationKind == string(InstrumentObservationClose) {
		order.date = q.EffectiveDate
	} else if q.SourceKind == domain.QuoteSourceProvider {
		order.rawAt, _ = domain.MetalConversionRawQuotedAt(q.ConversionJSON)
	}
	return order
}

func fxQuoteOrder(q domain.FXQuote) currentQuoteOrder {
	order := currentQuoteOrder{source: q.SourceKind, quotedAt: q.QuotedAt, createdAt: q.CreatedAt, id: q.ID.String()}
	if q.ObservationKind == string(FXObservationDailyReference) {
		order.date = q.EffectiveDate
	}
	return order
}

func chartQuoteOrder(q domain.QuoteSeriesPoint) currentQuoteOrder {
	order := currentQuoteOrder{source: q.SourceKind, quotedAt: q.QuotedAt, createdAt: q.CreatedAt, id: q.ID, revision: q.Revision}
	if q.ObservationKind == string(InstrumentObservationClose) || q.ObservationKind == string(FXObservationDailyReference) {
		order.date = q.EffectiveDate
	} else if q.SourceKind == domain.QuoteSourceProvider {
		order.rawAt, _ = domain.MetalConversionRawQuotedAt(q.ConversionJSON)
	}
	return order
}
