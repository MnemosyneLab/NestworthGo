package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// Drill-down values are frozen as privately owned JSON, outside the parent
// capture JSON. Their bytes and a conservative map/key allowance are charged
// to the capture's retained budget and the shared cache before admission.
// This is a retained-data budget, not a bound on builder temporaries or RSS.
const readRetainedCaptureLimit = 8 << 20
const readDrillEntryLimit = 32768
const drillEntryOverhead = 128
const drillBaseOverhead = 256

var assetDriverKeys = []string{
	"external_flow", "income", "spending", "dividend_interest", "price_change",
	"fx_impact", "fx_conversion_spread", "fee", "liability_impact", "adjustment", "residual",
}

var assetDriverKeyIndex = func() map[string]struct{} {
	index := make(map[string]struct{}, len(assetDriverKeys))
	for _, key := range assetDriverKeys {
		index[key] = struct{}{}
	}
	return index
}()

var returnDayFields = []string{"composition", "contributors", "issues"}
var assetDriverFields = []string{"byInstrument", "byAccount", "residualDetails"}

type periodDrilldown struct {
	from, to string
	days     map[string]json.RawMessage
	drivers  map[string]json.RawMessage
	size     int
}

type drillItem struct {
	summary  any
	children []string
	sections map[string][]json.RawMessage
}

func newPeriodDrilldown(from, to string, days []wire.ReturnDayDTO, drivers []wire.AssetDriverDetailDTO) (*periodDrilldown, error) {
	if len(days) > readDrillEntryLimit || len(drivers) > readDrillEntryLimit-len(days) {
		return nil, fail("too_large", "period details exceed retained entry budget; narrow query")
	}
	out := &periodDrilldown{from: strings.Clone(from), to: strings.Clone(to),
		days: make(map[string]json.RawMessage, len(days)), drivers: make(map[string]json.RawMessage, len(drivers)),
		size: drillBaseOverhead + len(from) + len(to)}
	add := func(dst map[string]json.RawMessage, key string, value any) error {
		raw, err := jsoncodec.Marshal(value)
		if err != nil {
			return err
		}
		charge := len(raw) + len(key) + drillEntryOverhead
		if charge > readRetainedCaptureLimit-out.size {
			return fail("too_large", "period details exceed retained byte budget; narrow query")
		}
		// Exact-capacity storage does not retain encoder spare capacity or DTO aliases.
		frozen := make([]byte, len(raw))
		copy(frozen, raw)
		dst[strings.Clone(key)] = frozen
		out.size += charge
		return nil
	}
	for _, day := range days {
		if err := add(out.days, day.Date, day); err != nil {
			return nil, err
		}
	}
	for _, driver := range drivers {
		if err := add(out.drivers, driver.DriverKey, driver); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func loadPeriodAnalysis(ctx context.Context, app *application.Service, adapter *analysis.Service, query PeriodAnalysisQuery) (PeriodAnalysisDTO, *periodDrilldown, error) {
	request := query.request()
	domainQuery, err := request.ToDomain()
	if err != nil {
		return PeriodAnalysisDTO{}, nil, err
	}
	var result PeriodAnalysisDTO
	var calendar wire.ReturnCalendarDTO
	drivers := make([]wire.AssetDriverDetailDTO, 0, len(assetDriverKeys))
	// One write fence covers the period projections and the drill-down that
	// get_read_item will serve. Later item reads do not refresh snapshots.
	err = app.WithWrite(ctx, func(ctx context.Context) error {
		var err error
		result.Income, err = adapter.Categories(ctx, request, "income")
		if err != nil {
			return err
		}
		result.Expenses, err = adapter.Categories(ctx, request, "spending")
		if err != nil {
			return err
		}
		result.AssetChange, err = adapter.AssetChange(ctx, request)
		if err != nil {
			return err
		}
		result.InvestmentReturns, err = adapter.ReturnTrend(ctx, request, "period_return_amount")
		if err != nil {
			return err
		}
		calendar, err = adapter.ReturnCalendar(ctx, request, "", "day")
		if err != nil {
			return err
		}
		for _, key := range assetDriverKeys {
			detail, err := adapter.AssetDriverDetail(ctx, request, key)
			if err != nil {
				return err
			}
			drivers = append(drivers, detail)
		}
		return nil
	})
	if err != nil {
		return PeriodAnalysisDTO{}, nil, err
	}
	drill, err := newPeriodDrilldown(string(domainQuery.From), string(domainQuery.To), calendar.Cells, drivers)
	return result, drill, err
}

func periodReadCapture(in AnalyzePeriodInput, result PeriodAnalysisDTO, drill *periodDrilldown) (readCapture, error) {
	capture := readCapture{Kind: "period_analysis", Query: in, Summary: map[string]any{}, Sections: map[string][]json.RawMessage{}, drilldown: drill}
	projections := []struct {
		name   string
		value  any
		fields []string
	}{
		{"income", result.Income, []string{"rows"}},
		{"expenses", result.Expenses, []string{"rows"}},
		{"assetChange", result.AssetChange, []string{"waterfall", "groups"}},
		{"investmentReturns", result.InvestmentReturns, []string{"points", "sources"}},
	}
	for _, projection := range projections {
		part, err := sectionedReadCapture("period_analysis", in, projection.value, projection.fields...)
		if err != nil {
			return capture, err
		}
		capture.Summary.(map[string]any)[projection.name] = part.Summary
		for name, rows := range part.Sections {
			capture.Sections[projection.name+"/"+name] = rows
		}
	}
	return capture, nil
}

func (c *boundedReadCache) periodItem(id string, e readCapture, in ReadItemInput) (ReadEnvelope, error) {
	summary, children, err := e.drilldown.item(in.Section, in.ID)
	if err != nil {
		return ReadEnvelope{}, err
	}
	r := c.base(id, e)
	r.Section, r.Summary = in.Section, summary
	r.Sections = map[string]ReadSection{}
	for _, section := range children {
		r.Sections[section] = ReadSection{Items: []json.RawMessage{}, Page: c.pageInfo(id, section, e, 0, 0)}
	}
	if ok, err := readFits(r); err != nil {
		return ReadEnvelope{}, err
	} else if !ok {
		return ReadEnvelope{}, fail("too_large", "single_item detail exceeds read wire budget; narrow the source query")
	}
	return r, nil
}

func (d *periodDrilldown) item(section, id string) (any, []string, error) {
	if d == nil {
		return nil, nil, fail("validation", "period capture has no drill-down")
	}
	id = strings.TrimSpace(id)
	switch section {
	case "returnDays":
		parsed, err := time.Parse("2006-01-02", id)
		if err != nil || parsed.Format("2006-01-02") != id || id < d.from || id > d.to {
			return nil, nil, fail("validation", "date must be YYYY-MM-DD within query.from and query.to")
		}
	case "assetDrivers":
		if _, ok := assetDriverKeyIndex[id]; !ok {
			return nil, nil, fail("validation", "driverKey must be a non-empty asset-change bucket")
		}
	default:
		return nil, nil, fail("validation", "period item section must be returnDays or assetDrivers")
	}
	prepared, err := d.materialize(section, id)
	if err != nil {
		return nil, nil, err
	}
	return prepared.summary, prepared.children, nil
}

func (d *periodDrilldown) sectionRows(section string) ([]json.RawMessage, bool, error) {
	kind, id, field, ok := splitDrillSection(section)
	if !ok || d == nil {
		return nil, false, nil
	}
	prepared, err := d.materialize(kind, id)
	if err != nil {
		return nil, false, err
	}
	rows, ok := prepared.sections[field]
	return rows, ok, nil
}

func splitDrillSection(section string) (kind, id, field string, ok bool) {
	parts := strings.Split(section, "/")
	if len(parts) != 3 || (parts[0] != "returnDays" && parts[0] != "assetDrivers") {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func (d *periodDrilldown) materialize(kind, id string) (drillItem, error) {
	switch kind {
	case "returnDays":
		day, ok := d.days[id]
		if !ok {
			return drillItem{}, fail("not_found", "item not found in capture section")
		}
		var value wire.ReturnDayDTO
		if err := jsoncodec.Unmarshal(day, &value); err != nil {
			return drillItem{}, err
		}
		return sectionItem("returnDays/"+id, value, returnDayFields)
	case "assetDrivers":
		driver, ok := d.drivers[id]
		if !ok {
			return drillItem{}, fail("not_found", "item not found in capture section")
		}
		var value wire.AssetDriverDetailDTO
		if err := jsoncodec.Unmarshal(driver, &value); err != nil {
			return drillItem{}, err
		}
		return sectionItem("assetDrivers/"+id, value, assetDriverFields)
	default:
		return drillItem{}, fail("not_found", "item not found in capture section")
	}
}

func sectionItem(key string, value any, fields []string) (drillItem, error) {
	detail, err := sectionedReadCapture("", nil, value, fields...)
	if err != nil {
		return drillItem{}, err
	}
	item := drillItem{
		summary:  detail.Summary,
		children: make([]string, 0, len(fields)),
		sections: make(map[string][]json.RawMessage, len(fields)),
	}
	for _, field := range fields {
		item.children = append(item.children, key+"/"+field)
		item.sections[field] = detail.Sections[field]
	}
	return item, nil
}
