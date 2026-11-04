package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type MarketBatchInput struct {
	Targets       []application.MarketReadTarget `json:"targets"`
	View          string                         `json:"view,omitempty"`
	Range         string                         `json:"range,omitempty"`
	FromDate      *string                        `json:"fromDate,omitempty"`
	ToDate        *string                        `json:"toDate,omitempty"`
	HistoryLayout string                         `json:"historyLayout,omitempty"`
	SourceFilter  string                         `json:"sourceFilter,omitempty"`
	Sort          *ReadSort                      `json:"sort,omitempty"`
	HistorySort   *ReadSort                      `json:"historySort,omitempty"`
	Limit         int                            `json:"limit,omitempty"`
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
func (s *Service) marketBatchTools(server *toolSet, g uint64) {
	readTool(server, "get_market_data_batch", "Capture 1 to 100 distinct instrument IDs or oriented FX pairs in one strictly local read-only transaction. current (default) omits history; coverage preserves counts and gapAssessment=not_evaluated; history separates selected points from raw observations per target. Follow every section cursor with get_read_page before claiming completeness. Use paired inclusive fromDate/toDate instead of range. Opt in to historyLayout=records for one shared active-observation section with chartSelected flags; FX value preserves original direction and displayValue is the oriented chart projection. Follow all cursors before claiming absence. No provider refresh or writes.", func(ctx context.Context, in MarketBatchInput) (any, error) {
		request, err := application.NormalizeMarketRead(application.MarketReadRequest{Targets: in.Targets, View: in.View, Range: in.Range, FromDate: in.FromDate, ToDate: in.ToDate, SourceFilter: in.SourceFilter})
		if err != nil {
			return nil, err
		}
		if in.HistoryLayout != "" && in.HistoryLayout != "per_target" && in.HistoryLayout != "records" {
			return nil, fail("validation", "historyLayout must be per_target or records")
		}
		unified := in.HistoryLayout == "records"
		if unified && request.View != "history" {
			return nil, fail("validation", "records layout requires view=history")
		}
		if unified && in.Limit == 0 {
			in.Limit = 100
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
			location, err := time.LoadLocation(r.Timezone)
			if err != nil {
				return readCapture{}, err
			}
			records := []marketHistoryRecord{}
			for _, t := range r.Targets {
				v := map[string]any{"target": t.Target, "targetKey": t.Target.Key(), "displayName": t.DisplayName, "status": t.Status, "reasons": t.Reasons, "currentValue": t.CurrentValue, "currentInstrument": wire.FromInstrumentQuotePtr(t.CurrentInstrument), "currentFX": wire.FromFXQuotePtr(t.CurrentFX)}
				if request.View != "current" {
					first, last := (*string)(nil), (*string)(nil)
					for _, p := range t.Series.Observations {
						d := p.QuotedAt.UTC().Format(time.RFC3339Nano)
						if unified || request.FromDate != nil {
							d = p.QuotedAt.In(location).Format(time.DateOnly)
						}
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
					if request.FromDate != nil {
						v["coverage"].(map[string]any)["fromDate"] = *request.FromDate
						v["coverage"].(map[string]any)["toDate"] = *request.ToDate
					}
					if request.View == "history" && unified {
						targetRecords, err := unifiedHistoryRows(t, history, r.Timezone)
						if err != nil {
							return readCapture{}, err
						}
						records = append(records, targetRecords...)
						v["historyStatus"] = "available"
						if len(t.Series.Observations) == 0 {
							v["historyStatus"] = "no_quotes_in_range"
						}
						if t.Status == "not_found" {
							v["historyStatus"] = "not_found"
						}
						v["recordsSection"] = "records"
					}
					if request.View == "history" && !unified {
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
			initial := []string{"targets"}
			if unified {
				sections["records"], err = readRows(records)
				if err != nil {
					return readCapture{}, err
				}
				initial = []string{"records", "targets"}
			}
			query := map[string]any{"request": request, "sort": order, "historySort": history}
			summary := map[string]any{"view": request.View, "targetCount": len(targets), "sourceFilter": request.SourceFilter, "range": request.Range, "gapAssessment": "not_evaluated"}
			if unified {
				query["historyLayout"] = "records"
				summary["recordCount"], summary["timezone"] = len(records), r.Timezone
				summary["evidenceScope"] = "active_observations_only"
			}
			if request.FromDate != nil {
				summary["fromDate"], summary["toDate"], summary["timezone"] = *request.FromDate, *request.ToDate, r.Timezone
			}
			return readCapture{InitialSections: initial, Kind: "market_data", CapturedAt: r.CapturedAt, Query: query, Summary: summary, Sections: sections}, nil
		})
	})
}

// Unified history stores each active observation once. chartSelected identifies
// chart membership, not current source-policy selection or import eligibility.
type marketHistoryRecord struct {
	marketHistoryPoint
	Target        application.MarketReadTarget `json:"target"`
	TargetKey     string                       `json:"targetKey"`
	Date          string                       `json:"date"`
	DateBasis     string                       `json:"dateBasis"`
	Currency      string                       `json:"currency"`
	BaseCurrency  string                       `json:"baseCurrency,omitempty"`
	QuoteCurrency string                       `json:"quoteCurrency,omitempty"`
	DisplayValue  string                       `json:"displayValue,omitempty"`
	ChartSelected bool                         `json:"chartSelected"`
	EvidenceKind  string                       `json:"evidenceKind"`
}

func unifiedHistoryRows(t application.MarketReadResultTarget, order ReadSort, timezone string) ([]marketHistoryRecord, error) {
	selected := map[string]bool{}
	for _, p := range t.Series.Points {
		selected[p.ID] = true
	}
	rawFX := map[string]domain.FXQuote{}
	for _, q := range t.RawFXQuotes {
		rawFX[q.ID.String()] = q
	}
	rows, err := historyRows(t.Series.Observations, order, "asc")
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.QuoteSeriesPoint{}
	for _, p := range t.Series.Observations {
		byID[p.ID] = p
	}
	records := make([]marketHistoryRecord, 0, len(rows))
	for _, raw := range rows {
		var point marketHistoryPoint
		if err := decodeReadValue(raw, &point); err != nil {
			return nil, err
		}
		p := byID[point.QuoteID]
		date, basis := p.EffectiveDate, "effective_date"
		if date == "" {
			date, basis = p.QuotedAt.In(location).Format(time.DateOnly), "quoted_at_in_capture_timezone"
		}
		record := marketHistoryRecord{marketHistoryPoint: point, Target: t.Target, TargetKey: t.Target.Key(), Date: date, DateBasis: basis, Currency: t.Series.DisplayCurrency.String(), ChartSelected: selected[p.ID], EvidenceKind: "active_observation"}
		if q, ok := rawFX[p.ID]; ok {
			record.DisplayValue = point.Value
			record.Value = q.Rate.Canonical()
			record.BaseCurrency, record.QuoteCurrency = q.BaseCurrency.String(), q.QuoteCurrency.String()
			record.Currency = q.QuoteCurrency.String()
		} else if t.Target.InstrumentID == "" {
			return nil, fail("unavailable", "raw FX evidence unavailable; cannot return a display projection as an original rate")
		}
		records = append(records, record)
	}
	return records, nil
}
