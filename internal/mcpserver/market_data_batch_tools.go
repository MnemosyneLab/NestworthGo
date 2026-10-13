package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type MarketBatchInput struct {
	Targets      []application.MarketReadTarget `json:"targets"`
	View         string                         `json:"view,omitempty"`
	Range        string                         `json:"range,omitempty"`
	SourceFilter string                         `json:"sourceFilter,omitempty"`
	Sort         *ReadSort                      `json:"sort,omitempty"`
	HistorySort  *ReadSort                      `json:"historySort,omitempty"`
	Limit        int                            `json:"limit,omitempty"`
}
type marketHistoryPoint struct {
	QuoteID         string  `json:"quoteId"`
	QuotedAt        string  `json:"quotedAt"`
	CreatedAt       string  `json:"createdAt"`
	EffectiveDate   *string `json:"effectiveDate"`
	Value           string  `json:"value"`
	ObservationKind string  `json:"observationKind"`
	PriceBasis      string  `json:"priceBasis"`
	TimestampBasis  string  `json:"timestampBasis"`
	SourceKind      string  `json:"sourceKind"`
	SourceKey       string  `json:"sourceKey"`
	Delayed         bool    `json:"delayed"`
	Revision        int     `json:"revision"`
	ConversionJSON  string  `json:"conversionJson,omitempty"`
}

func historyRows(points []domain.QuoteSeriesPoint, order ReadSort, defaultDirection string) ([]json.RawMessage, error) {
	if order.Direction == "" {
		order.Direction = defaultDirection
	}
	if order.Field == "" {
		order.Field = "quotedAt"
	}
	sort.SliceStable(points, func(i, j int) bool {
		a, b := points[i], points[j]
		cmp := 0
		if order.Field == "effectiveDate" {
			cmp = compareReadText(a.EffectiveDate, b.EffectiveDate, order.Direction)
		} else {
			if !a.QuotedAt.Equal(b.QuotedAt) {
				if order.Direction == "desc" {
					return a.QuotedAt.After(b.QuotedAt)
				}
				return a.QuotedAt.Before(b.QuotedAt)
			}
		}
		if cmp != 0 {
			return cmp < 0
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	rows := make([]marketHistoryPoint, 0, len(points))
	for _, p := range points {
		var date *string
		if p.EffectiveDate != "" {
			d := p.EffectiveDate
			date = &d
		}
		rows = append(rows, marketHistoryPoint{p.ID, p.QuotedAt.UTC().Format(time.RFC3339Nano), p.CreatedAt.UTC().Format(time.RFC3339Nano), date, p.Value, p.ObservationKind, p.PriceBasis, p.TimestampBasis, string(p.SourceKind), p.SourceKey, p.Delayed, p.Revision, p.ConversionJSON})
	}
	return readRows(rows)
}
func (s *Service) marketBatchTools(server *mcp.Server, g uint64) {
	readTool(server, "get_market_data_batch", "Capture 1 to 100 distinct instrument IDs or oriented FX pairs in one strictly local read-only transaction. current (default) omits history; coverage preserves counts and gapAssessment=not_evaluated; history separates selected points from raw observations per target. Follow every section cursor with get_read_page before claiming completeness. No provider refresh or writes.", func(ctx context.Context, in MarketBatchInput) (any, error) {
		request, err := application.NormalizeMarketRead(application.MarketReadRequest{Targets: in.Targets, View: in.View, Range: in.Range, SourceFilter: in.SourceFilter})
		if err != nil {
			return nil, err
		}
		order := ReadSort{}
		if in.Sort != nil {
			order = *in.Sort
		}
		if err := validateReadSort(&order, []string{"targetKey", "displayName"}, "targetKey", "asc"); err != nil {
			return nil, err
		}
		if order.HorizonOn != "" {
			return nil, fail("validation", "market sort does not accept horizonOn")
		}
		history := ReadSort{}
		if in.HistorySort != nil {
			history = *in.HistorySort
			if history.Field != "" && !hasString([]string{"quotedAt", "effectiveDate"}, history.Field) {
				return nil, fail("validation", "invalid history sort field")
			}
			if history.Direction != "" && history.Direction != "asc" && history.Direction != "desc" {
				return nil, fail("validation", "invalid history sort direction")
			}
			if history.HorizonOn != "" {
				return nil, fail("validation", "history sort does not accept horizonOn")
			}
		}
		return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
			r, err := s.app.ReadMarketDataBatch(ctx, request)
			if err != nil {
				return readCapture{}, err
			}
			sort.SliceStable(r.Targets, func(i, j int) bool {
				a, b := r.Targets[i], r.Targets[j]
				av, bv := a.Target.Key(), b.Target.Key()
				if order.Field == "displayName" {
					av, bv = normalizedReadName(a.DisplayName), normalizedReadName(b.DisplayName)
				}
				cmp := compareReadText(av, bv, order.Direction)
				if cmp == 0 {
					return a.Target.Key() < b.Target.Key()
				}
				return cmp < 0
			})
			targets := []map[string]any{}
			sections := map[string][]json.RawMessage{}
			for _, t := range r.Targets {
				v := map[string]any{"target": t.Target, "targetKey": t.Target.Key(), "displayName": t.DisplayName, "status": t.Status, "reasons": t.Reasons, "currentValue": t.CurrentValue, "currentInstrument": wire.FromInstrumentQuotePtr(t.CurrentInstrument), "currentFX": wire.FromFXQuotePtr(t.CurrentFX)}
				if request.View != "current" {
					first, last := (*string)(nil), (*string)(nil)
					for _, p := range t.Series.Observations {
						d := p.QuotedAt.UTC().Format(time.RFC3339Nano)
						if p.EffectiveDate != "" {
							d = p.EffectiveDate
						}
						if first == nil || d < *first {
							x := d
							first = &x
						}
						if last == nil || d > *last {
							x := d
							last = &x
						}
					}
					v["coverage"] = map[string]any{"range": request.Range, "firstAvailable": first, "lastAvailable": last, "observationsCount": len(t.Series.Observations), "selectedPointsCount": len(t.Series.Points), "outsideRange": t.Series.OutsideRange, "gapAssessment": "not_evaluated"}
					if request.View == "history" {
						pk, ok := t.Target.Key()+"/points", t.Target.Key()+"/observations"
						points, err := historyRows(t.Series.Points, history, "asc")
						if err != nil {
							return readCapture{}, err
						}
						observations, err := historyRows(t.Series.Observations, history, "desc")
						if err != nil {
							return readCapture{}, err
						}
						sections[pk], sections[ok] = points, observations
						v["pointsSection"], v["observationsSection"] = pk, ok
					}
				}
				targets = append(targets, v)
			}
			rows, err := readRows(targets)
			if err != nil {
				return readCapture{}, err
			}
			sections["targets"] = rows
			return readCapture{InitialSections: []string{"targets"}, Kind: "market_data", CapturedAt: r.CapturedAt, Query: map[string]any{"request": request, "sort": order, "historySort": history}, Summary: map[string]any{"view": request.View, "targetCount": len(targets), "sourceFilter": request.SourceFilter, "range": request.Range, "gapAssessment": "not_evaluated"}, Sections: sections}, nil
		})
	})
}
