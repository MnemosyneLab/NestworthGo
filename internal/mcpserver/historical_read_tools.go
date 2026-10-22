package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// These select endpoint rows, not the household's monetary/trend scope. The
// distinction is explicit in the frozen query and summary, including no matches.
type HistoricalReadFilter struct {
	AccountIDs    []string `json:"accountIds,omitempty"`
	InstrumentIDs []string `json:"instrumentIds,omitempty"`
	Kinds         []string `json:"kinds,omitempty" jsonschema:"Default excludes account rollups; explicit account includes them"`
	Included      *bool    `json:"included,omitempty"`
	Quality       string   `json:"quality,omitempty" jsonschema:"pending, missing, provisional or verified"`
	Reasons       []string `json:"reasons,omitempty"`
	Side          string   `json:"side,omitempty" jsonschema:"left, right or either (default); right requires a comparison"`
}
type HistoricalReadOptions struct {
	View   string                `json:"view,omitempty" jsonschema:"compact (default) or evidence; full row evidence is also available via get_read_item"`
	Filter *HistoricalReadFilter `json:"filter,omitempty" jsonschema:"Select endpoint rows only; household monetary totals and series points retain full scope"`
	Limit  int                   `json:"limit,omitempty" jsonschema:"Initial default 5 per section, maximum 100; continuations default 25; byte-aware pages may contain fewer rows"`
}

var historicalReasons = []string{"carried_nav", "carried_price", "carried_fx", "history_unverified", "price_missing", "fx_missing", "account_value_missing", "evidence_unavailable", "instrument_price", "missing_instrument", "fx_rate", "account_value", "history_coverage", "holding_amount", "funding_cash_after_product_start"}

func normalizeHistoricalRead(in HistoricalReadOptions) (HistoricalReadOptions, HistoricalReadFilter, error) {
	f := HistoricalReadFilter{}
	if in.Filter != nil {
		f = *in.Filter
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "evidence" {
		return in, f, fail("validation", "view must be compact or evidence")
	}
	for _, ids := range [][]string{f.AccountIDs, f.InstrumentIDs} {
		if err := validateReadValues(ids, nil, true); err != nil {
			return in, f, err
		}
	}
	if err := validateReadValues(f.Kinds, []string{"account", "holding", "balance", "cash"}, false); err != nil {
		return in, f, err
	}
	if err := validateReadValues(f.Reasons, historicalReasons, false); err != nil {
		return in, f, err
	}
	if f.Quality != "" && !hasString([]string{"pending", "missing", "provisional", "verified"}, f.Quality) {
		return in, f, fail("validation", "unknown quality filter")
	}
	if f.Side == "" {
		f.Side = "either"
	}
	if !hasString([]string{"left", "right", "either"}, f.Side) {
		return in, f, fail("validation", "side must be left, right or either")
	}
	if _, err := readLimit(in.Limit); err != nil {
		return in, f, err
	}
	in.Filter = &f
	return in, f, nil
}
func historicalCellMatches(c *application.HistoricalOverviewCell, f HistoricalReadFilter) bool {
	if c == nil {
		return false
	}
	if f.Included != nil && c.Included != *f.Included {
		return false
	}
	q := c.DisplayValuation.Quality
	switch f.Quality {
	case "pending":
		if q.CarriedCount == 0 {
			return false
		}
	case "missing":
		if c.Complete && len(c.Missing) == 0 && q.UnpricedCount == 0 {
			return false
		}
	case "provisional":
		if q.Coverage != domain.CoverageProvisional {
			return false
		}
	case "verified":
		if q.Coverage != domain.CoverageVerified {
			return false
		}
	}
	if len(f.Reasons) > 0 {
		found := false
		for _, reason := range q.Reasons {
			if hasString(f.Reasons, string(reason)) {
				found = true
				break
			}
		}
		if !found {
			for _, reason := range c.Missing {
				if hasString(f.Reasons, reason) {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func historicalRowMatches(r application.HistoricalOverviewRow, f HistoricalReadFilter) bool {
	if len(f.Kinds) == 0 && r.Kind == "account" {
		return false
	}
	if !hasString(f.Kinds, r.Kind) || !hasString(f.AccountIDs, r.AccountID) || !hasString(f.InstrumentIDs, r.InstrumentID) {
		return false
	}
	return (f.Side != "right" && historicalCellMatches(r.Left, f)) || (f.Side != "left" && historicalCellMatches(r.Right, f))
}

type historicalMatchCounts struct {
	IncludedLeafCount int `json:"includedLeafCount"`
	PendingCount      int `json:"pendingCount"`
}

func (c *historicalMatchCounts) add(kind string, cell *application.HistoricalOverviewCell) {
	if kind != "account" && cell != nil && cell.Included {
		c.IncludedLeafCount++
		c.PendingCount += cell.DisplayValuation.Quality.CarriedCount
	}
}
func historicalRowProjection(row application.HistoricalOverviewRow, compact bool) (map[string]any, error) {
	out, err := readSummary(row)
	if err != nil {
		return nil, err
	}
	out["id"] = row.Key
	out["aggregation"] = "leaf"
	if row.Kind == "account" {
		out["aggregation"] = "rollup"
	}
	for _, side := range []string{"left", "right"} {
		cell, ok := out[side].(map[string]any)
		if !ok {
			continue
		}
		original := row.Left
		if side == "right" {
			original = row.Right
		}
		counted := row.Kind != "account" && original.Included
		pending := 0
		if counted {
			pending = original.DisplayValuation.Quality.CarriedCount
		}
		cell["countedLeaf"], cell["pendingContribution"] = counted, pending
		if !compact {
			continue
		}
		delete(cell, "missingDetails")
		for _, kind := range []string{"price", "fx"} {
			evidence, ok := cell[kind].(map[string]any)
			if !ok {
				continue
			}
			small := map[string]any{}
			for _, key := range []string{"id", "source", "freshness", "effectiveAt", "marketDate", "closedCarry"} {
				if v, ok := evidence[key]; ok {
					small[key] = v
				}
			}
			cell[kind] = small
		}
	}
	if compact {
		for _, key := range []string{"holdingId", "instrumentId", "parentKey"} {
			if out[key] == "" {
				delete(out, key)
			}
		}
	}
	return out, nil
}

func historicalReadCapture(overview application.HistoricalOverviewResult, points []application.HistoricalSeriesPoint, in HistoricalReadOptions, f HistoricalReadFilter) (readCapture, error) {
	if f.Side == "right" && overview.Right == nil {
		return readCapture{}, fail("validation", "right side requires compareTo or a series")
	}
	summary, err := readSummary(overview, "rows", "categories", "reconciliation", "capturedAt")
	if err != nil {
		return readCapture{}, err
	}
	c := readCapture{Kind: "historical_overview", Query: map[string]any{"date": overview.Left.Date, "view": in.View, "filter": f}, Summary: summary, Sections: map[string][]json.RawMessage{}, Details: map[string]any{}, DetailSections: map[string][]string{}, InitialSections: []string{"rows"}}
	if overview.Right != nil {
		c.Query.(map[string]any)["compareTo"] = overview.Right.Date
	}
	c.CapturedAt, _ = time.Parse(time.RFC3339Nano, overview.CapturedAt)
	summary["responseVersion"] = "historical-read/2"
	summary["totalsScope"], summary["filterScope"] = "household", "endpoint_rows"
	summary["detailRef"] = map[string]string{"section": "overview", "id": "endpoints"}
	// Allocations remain complete but are fetched separately from the header.
	for _, side := range []string{"left", "right"} {
		state, ok := summary[side].(map[string]any)
		if !ok {
			continue
		}
		c.Details["states/"+side] = state
		header := map[string]any{}
		for key, v := range state {
			if key != "byClass" && key != "byCurrency" {
				header[key] = v
			}
		}
		summary[side] = header
	}
	if overview.Reconciliation != nil {
		header, e := readSummary(overview.Reconciliation, "contributions")
		if e != nil {
			return c, e
		}
		summary["reconciliation"] = header
		c.Sections["contributions"], err = readRows(overview.Reconciliation.Contributions)
		if err != nil {
			return c, err
		}
	}
	categories := []map[string]any{}
	for _, category := range overview.Categories {
		row, e := readSummary(category)
		if e != nil {
			return c, e
		}
		id := category.Role + ":" + category.Dimension + ":" + category.Key
		row["id"] = id
		for _, side := range []string{"left", "right"} {
			cell := row[side].(map[string]any)
			keys, ok := cell["missingRowKeys"].([]any)
			if !ok {
				keys = []any{}
			}
			section := "categoryMissing/" + id + "/" + side
			c.Sections[section], err = readRows(keys)
			if err != nil {
				return c, err
			}
			delete(cell, "missingRowKeys")
			cell["missingRowCount"] = len(keys)
			c.DetailSections["categories/"+id] = append(c.DetailSections["categories/"+id], section)
		}
		c.Details["categories/"+id] = row
		categories = append(categories, row)
	}
	c.Sections["categories"], err = readRows(categories)
	if err != nil {
		return c, err
	}
	c.DetailSections["overview/endpoints"] = []string{"categories"}
	if overview.Reconciliation != nil {
		c.DetailSections["overview/endpoints"] = append(c.DetailSections["overview/endpoints"], "contributions")
	}
	c.Details["overview/endpoints"] = map[string]any{"totalsScope": "household", "stateIds": []string{"left"}, "reconciliation": summary["reconciliation"]}
	if overview.Right != nil {
		c.Details["overview/endpoints"].(map[string]any)["stateIds"] = []string{"left", "right"}
	}
	rows := append([]application.HistoricalOverviewRow{}, overview.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	selected := []map[string]any{}
	left, right := historicalMatchCounts{}, historicalMatchCounts{}
	totalLeaves := 0
	for _, r := range rows {
		if r.Kind != "account" {
			totalLeaves++
		}
		if !historicalRowMatches(r, f) {
			continue
		}
		detail, e := historicalRowProjection(r, false)
		if e != nil {
			return c, e
		}
		c.Details["rows/"+r.Key] = detail
		row := detail
		if in.View == "compact" {
			row, e = historicalRowProjection(r, true)
			if e != nil {
				return c, e
			}
		}
		selected = append(selected, row)
		left.add(r.Kind, r.Left)
		right.add(r.Kind, r.Right)
	}
	summary["totalRowCount"], summary["totalLeafCount"], summary["matchedRowCount"] = len(rows), totalLeaves, len(selected)
	summary["matchedLeft"], summary["matchedRight"] = left, right
	c.Sections["rows"], err = readRows(selected)
	if err != nil {
		return c, err
	}
	if points != nil {
		c.Kind = "historical_overview_series"
		c.InitialSections = append(c.InitialSections, "points")
		c.Query.(map[string]any)["from"], c.Query.(map[string]any)["to"] = points[0].Date, points[len(points)-1].Date
		delete(c.Query.(map[string]any), "date")
		delete(c.Query.(map[string]any), "compareTo")
		summary["pointsScope"], summary["pointCount"] = "household", len(points)
		compact := []map[string]any{}
		for _, p := range points {
			row, e := readSummary(p, "missing")
			if e != nil {
				return c, e
			}
			row["id"], row["missingCount"] = p.Date, len(p.Missing)
			section := "missing/" + p.Date
			c.Sections[section], err = readRows(p.Missing)
			if err != nil {
				return c, err
			}
			c.Details["points/"+p.Date] = row
			c.DetailSections["points/"+p.Date] = []string{section}
			compact = append(compact, row)
		}
		c.Sections["points"], err = readRows(compact)
		if err != nil {
			return c, err
		}
	}
	return c, nil
}
func (s *Service) pagedHistoricalOverview(ctx context.Context, g uint64, in HistoricalOverviewInput) (ReadEnvelope, error) {
	options, f, err := normalizeHistoricalRead(in.HistoricalReadOptions)
	if err != nil {
		return ReadEnvelope{}, err
	}
	return s.buildRead(ctx, g, options.Limit, func(ctx context.Context) (readCapture, error) {
		result, e := s.app.HistoricalOverview(ctx, in.Date, in.CompareTo)
		if e != nil {
			return readCapture{}, e
		}
		return historicalReadCapture(result, nil, options, f)
	})
}
func (s *Service) pagedHistoricalSeries(ctx context.Context, g uint64, in HistoricalOverviewSeriesInput) (ReadEnvelope, error) {
	options, f, err := normalizeHistoricalRead(in.HistoricalReadOptions)
	if err != nil {
		return ReadEnvelope{}, err
	}
	return s.buildRead(ctx, g, options.Limit, func(ctx context.Context) (readCapture, error) {
		result, e := s.app.HistoricalOverviewSeries(ctx, in.From, in.To)
		if e != nil {
			return readCapture{}, e
		}
		return historicalReadCapture(result.Overview, result.Points, options, f)
	})
}
