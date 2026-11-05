package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// Opt-in parent-capture cost measurement for stream C2.
//
//	NESTWORTH_C2_MEASURE=1 go test ./internal/mcpserver -run TestMeasureC2ParentCaptureCost -count=1 -timeout 30m -v
func TestMeasureC2ParentCaptureCost(t *testing.T) {
	if os.Getenv("NESTWORTH_C2_MEASURE") != "1" {
		t.Skip("set NESTWORTH_C2_MEASURE=1")
	}
	ctx := context.Background()
	app := wailstest.NewService(t)
	// Measured ranges are closed through 2025-12-31. The clock is the next local day.
	app.SetClock(func() time.Time { return time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "SYNTHETIC c2", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"},
		Timezone: "UTC", HistoryStartDate: "2016-01-01",
	}); err != nil {
		t.Fatal(err)
	}
	accounts, instruments := seedC2Book(t, app)
	adapter := analysis.NewService(app)
	t.Logf("book accounts=%d instruments=%d", len(accounts), len(instruments))
	t.Logf("limits capture_bytes=%d cache_bytes=%d cache_count=%d ttl=%s build_timeout=%s", readCaptureLimit, readCacheLimit, readCacheCount, readTTL, readBuildTimeout)

	ranges := []struct{ name, from, to string }{
		{"month", "2025-12-01", "2025-12-31"},
		{"quarter", "2025-10-01", "2025-12-31"},
		{"year", "2025-01-01", "2025-12-31"},
		{"five_year", "2021-01-01", "2025-12-31"},
		{"ten_year", "2016-01-01", "2025-12-31"},
	}
	for _, window := range ranges {
		t.Run(window.name, func(t *testing.T) {
			measureC2Window(t, app, adapter, window.from, window.to)
		})
	}
	activateC2December(t, app, instruments)
	t.Run("active_month", func(t *testing.T) {
		measureC2Window(t, app, adapter, "2025-12-01", "2025-12-31")
	})
}

// activateC2December writes one manual quote per instrument per closed day so
// return-day drill-downs contain composition and contributors, unlike the
// carried-price book measured above.
func activateC2December(t *testing.T, app *application.Service, instruments []domain.Instrument) {
	t.Helper()
	ctx := context.Background()
	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	for day := 0; day < 31; day++ {
		at := start.AddDate(0, 0, day).Format("2006-01-02")
		price := fmt.Sprintf("%d", 40+day)
		for _, inst := range instruments {
			if _, err := app.AppendManualInstrumentQuote(ctx, inst.ID, price, at, false); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestMeasureC2ParentCaptureExtremes(t *testing.T) {
	if os.Getenv("NESTWORTH_C2_MEASURE") != "1" {
		t.Skip("set NESTWORTH_C2_MEASURE=1")
	}
	t.Run("active_year", func(t *testing.T) {
		app := newC2App(t, "2025-01-01")
		instruments := seedC2Positions(t, app, "2025-01-02")
		writeDailyQuotes(t, app, instruments, "2025-01-02", "2025-12-31")
		measureC2Window(t, app, analysis.NewService(app), "2025-01-02", "2025-12-31")
	})
	t.Run("quiet_14_year", func(t *testing.T) {
		app := newC2App(t, "2000-01-01")
		seedC2Positions(t, app, "2012-01-04")
		measureC2Window(t, app, analysis.NewService(app), "2012-01-01", "2025-12-31")
	})
	t.Run("quiet_18_year", func(t *testing.T) {
		app := newC2App(t, "2000-01-01")
		seedC2Positions(t, app, "2008-01-04")
		measureC2Window(t, app, analysis.NewService(app), "2008-01-01", "2025-12-31")
	})
}

func newC2App(t *testing.T, historyStart string) *application.Service {
	t.Helper()
	app := wailstest.NewService(t)
	app.SetClock(func() time.Time { return time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) })
	if err := app.CompleteOnboarding(context.Background(), application.OnboardingInput{
		HouseholdName: "SYNTHETIC c2", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"},
		Timezone: "UTC", HistoryStartDate: historyStart,
	}); err != nil {
		t.Fatal(err)
	}
	return app
}

func seedC2Book(t *testing.T, app *application.Service) (accounts []string, instruments []domain.Instrument) {
	t.Helper()
	ctx := context.Background()
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v %v", members, err)
	}
	owner := members[0].ID.String()
	for i := 0; i < 2; i++ {
		record, createErr := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
			Name: fmt.Sprintf("Synthetic brokerage %d", i+1), AccountType: "brokerage", BalanceSheetRole: "asset",
			TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
			OwnerIDs: []string{owner},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		accounts = append(accounts, record.Account.ID)
	}
	h := history.NewService(app)
	for _, accountID := range accounts {
		if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
			Kind: history.ChangeMoneyAdded, AccountID: accountID, Amount: "100000", Currency: "USD",
			Reason: "contribution", EffectiveAt: "2016-01-04T09:00:00Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	instSvc := instrument.NewService(app)
	for i := 0; i < 4; i++ {
		created, createErr := instSvc.CreateInstrument(ctx, instrument.InstrumentRequest{
			Name: fmt.Sprintf("Synthetic stock %d", i+1), Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		id, parseErr := domain.ParseInstrumentID(created.ID)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		instruments = append(instruments, domain.Instrument{ID: id})
		for _, quoted := range []struct{ price, at string }{{"40", "2016-01-04"}, {"55", "2025-12-31"}} {
			if _, err := app.AppendManualInstrumentQuote(ctx, id, quoted.price, quoted.at, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	trade := func(accountID, instrumentID, side, holdingID, quantity, gross, at string) string {
		t.Helper()
		result, recordErr := h.RecordChange(ctx, history.ChangeCommandRequest{
			Kind: history.ChangeTrade, SettlementAccountID: accountID, InstrumentID: instrumentID,
			Side: side, HoldingID: holdingID, Quantity: quantity, Gross: gross, GrossCurrency: "USD",
			Fee: "1", FeeCurrency: "USD", EffectiveAt: at,
		})
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		if result.Activity.TradeDetail == nil {
			t.Fatal("missing trade detail")
		}
		return result.Activity.TradeDetail.HoldingID
	}
	for ai, accountID := range accounts {
		for ii, inst := range instruments {
			holdingID := trade(accountID, inst.ID.String(), "buy", "", "10", "400", "2016-01-04T15:00:00Z")
			if ai == 0 && ii == 0 {
				trade(accountID, inst.ID.String(), "sell", holdingID, "4", "220", "2020-06-15T15:00:00Z")
				if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
					Kind: history.ChangeCashDividend, HoldingID: holdingID, Amount: "12", Currency: "USD",
					EffectiveAt: "2024-03-15T15:00:00Z",
				}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return accounts, instruments
}

func seedC2Positions(t *testing.T, app *application.Service, openDate string) []domain.Instrument {
	t.Helper()
	ctx := context.Background()
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v %v", members, err)
	}
	owner := members[0].ID.String()
	accounts := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		record, createErr := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
			Name: fmt.Sprintf("Synthetic brokerage %d", i+1), AccountType: "brokerage", BalanceSheetRole: "asset",
			TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
			OwnerIDs: []string{owner},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		accounts = append(accounts, record.Account.ID)
	}
	h := history.NewService(app)
	openAt := openDate + "T09:00:00Z"
	for _, accountID := range accounts {
		if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
			Kind: history.ChangeMoneyAdded, AccountID: accountID, Amount: "100000", Currency: "USD",
			Reason: "contribution", EffectiveAt: openAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	instSvc := instrument.NewService(app)
	instruments := make([]domain.Instrument, 0, 4)
	for i := 0; i < 4; i++ {
		created, createErr := instSvc.CreateInstrument(ctx, instrument.InstrumentRequest{
			Name: fmt.Sprintf("Synthetic stock %d", i+1), Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		id, parseErr := domain.ParseInstrumentID(created.ID)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		instruments = append(instruments, domain.Instrument{ID: id})
		for _, quoted := range []string{openDate, "2025-12-31"} {
			if _, err := app.AppendManualInstrumentQuote(ctx, id, "40", quoted, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, accountID := range accounts {
		for _, inst := range instruments {
			if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
				Kind: history.ChangeTrade, SettlementAccountID: accountID, InstrumentID: inst.ID.String(),
				Side: "buy", Quantity: "10", Gross: "400", GrossCurrency: "USD", Fee: "1", FeeCurrency: "USD",
				EffectiveAt: openDate + "T15:00:00Z",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return instruments
}

func writeDailyQuotes(t *testing.T, app *application.Service, instruments []domain.Instrument, from, to string) {
	t.Helper()
	ctx := context.Background()
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		t.Fatal(err)
	}
	for day, n := start, 0; !day.After(end); day, n = day.AddDate(0, 0, 1), n+1 {
		price := fmt.Sprintf("%d", 40+(n%50))
		at := day.Format("2006-01-02")
		for _, inst := range instruments {
			if _, err := app.AppendManualInstrumentQuote(ctx, inst.ID, price, at, false); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func measureC2Window(t *testing.T, app *application.Service, adapter *analysis.Service, from, to string) {
	t.Helper()
	ctx := context.Background()
	request := analysis.AnalysisQueryRequest{From: from, To: to, Valuation: "base", Basis: "investment", IncludeCash: true}
	query := PeriodAnalysisQuery{From: from, To: to, Valuation: "base", Basis: "investment", IncludeCash: true}

	var period PeriodAnalysisDTO
	periodStart := time.Now()
	err := app.WithWrite(ctx, func(ctx context.Context) error {
		var callErr error
		period.Income, callErr = adapter.Categories(ctx, request, "income")
		if callErr != nil {
			return callErr
		}
		period.Expenses, callErr = adapter.Categories(ctx, request, "spending")
		if callErr != nil {
			return callErr
		}
		period.AssetChange, callErr = adapter.AssetChange(ctx, request)
		if callErr != nil {
			return callErr
		}
		period.InvestmentReturns, callErr = adapter.ReturnTrend(ctx, request, "period_return_amount")
		return callErr
	})
	periodElapsed := time.Since(periodStart)
	if err != nil {
		t.Fatal(err)
	}
	parent := assemblePeriodCapture(t, AnalyzePeriodInput{Query: query}, period)
	parentSize := frozenCaptureSize(t, parent)

	var calendar wire.ReturnCalendarDTO
	calendarStart := time.Now()
	err = app.WithWrite(ctx, func(ctx context.Context) error {
		var callErr error
		calendar, callErr = adapter.ReturnCalendar(ctx, request, "", "day")
		return callErr
	})
	calendarElapsed := time.Since(calendarStart)
	if err != nil {
		t.Fatal(err)
	}

	driverKeys := []string{"external_flow", "income", "spending", "dividend_interest", "price_change", "fx_impact", "fx_conversion_spread", "fee", "liability_impact", "adjustment", "residual"}
	drivers := make([]wire.AssetDriverDetailDTO, 0, len(driverKeys))
	driverStart := time.Now()
	err = app.WithWrite(ctx, func(ctx context.Context) error {
		for _, key := range driverKeys {
			detail, callErr := adapter.AssetDriverDetail(ctx, request, key)
			if callErr != nil {
				return callErr
			}
			drivers = append(drivers, detail)
		}
		return nil
	})
	driverElapsed := time.Since(driverStart)
	if err != nil {
		t.Fatal(err)
	}

	folded := attachPeriodDrilldown(t, parent, calendar.Cells, drivers)
	foldedSize := frozenCaptureSize(t, folded)

	daySectionRows := 0
	for _, day := range calendar.Cells {
		for _, field := range []string{"composition", "contributors", "issues"} {
			daySectionRows += len(folded.Sections["returnDays/"+day.Date+"/"+field])
		}
	}
	driverSectionRows := 0
	residualRows := 0
	for _, key := range driverKeys {
		for _, field := range []string{"byInstrument", "byAccount", "residualDetails"} {
			n := len(folded.Sections["assetDrivers/"+key+"/"+field])
			driverSectionRows += n
			if field == "residualDetails" {
				residualRows += n
			}
		}
	}

	naiveDays := len(calendar.Cells)
	naiveLimit := naiveDays
	if naiveDays > 366 && os.Getenv("NESTWORTH_C2_MEASURE_NAIVE_ALL") != "1" {
		naiveLimit = 31
	}
	naiveStart := time.Now()
	err = app.WithWrite(ctx, func(ctx context.Context) error {
		for i := 0; i < naiveLimit; i++ {
			if _, callErr := adapter.ReturnDay(ctx, request, calendar.Cells[i].Date); callErr != nil {
				return callErr
			}
		}
		return nil
	})
	naiveElapsed := time.Since(naiveStart)
	if err != nil {
		t.Fatal(err)
	}

	contribInput := ListContributionsInput{Query: query, ReturnType: "total_return", GroupBy: "instrument", Ordering: "amount_desc", Limit: 25}
	var contrib wire.ContributionDTO
	var items []wire.ContributionItemDTO
	contribStart := time.Now()
	err = app.WithWrite(ctx, func(ctx context.Context) error {
		var callErr error
		contrib, items, callErr = analysis.CaptureContributions(ctx, app, request, "total_return", "instrument", "amount_desc")
		return callErr
	})
	contribElapsed := time.Since(contribStart)
	if err != nil {
		t.Fatal(err)
	}
	withDetail := assembleContributionCapture(t, contribInput, contrib, items)
	rowsOnly := rowsOnlyContribution(withDetail)
	contribSize := frozenCaptureSize(t, withDetail)
	rowsSize := frozenCaptureSize(t, rowsOnly)

	medianDay := 0
	maxDay := 0
	sampleContributors := 0
	sampleComposition := 0
	maxContributors := 0
	maxComposition := 0
	sampleStatus := ""
	if len(calendar.Cells) > 0 {
		sample := calendar.Cells[len(calendar.Cells)/2]
		sampleContributors = len(sample.Contributors)
		sampleComposition = len(sample.Composition)
		sampleStatus = sample.Status
		for _, day := range calendar.Cells {
			if len(day.Contributors) > maxContributors {
				maxContributors = len(day.Contributors)
			}
			if len(day.Composition) > maxComposition {
				maxComposition = len(day.Composition)
			}
			one, sectionErr := sectionedReadCapture("", nil, day, "composition", "contributors", "issues")
			if sectionErr != nil {
				t.Fatal(sectionErr)
			}
			size := frozenCaptureSize(t, one)
			if size > maxDay {
				maxDay = size
			}
		}
		one, sectionErr := sectionedReadCapture("", nil, sample, "composition", "contributors", "issues")
		if sectionErr != nil {
			t.Fatal(sectionErr)
		}
		medianDay = frozenCaptureSize(t, one)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	raw := frozenCaptureBytes(t, folded)
	var decoded readCapture
	if err := decodeReadValue(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(decoded)

	t.Logf("RANGE %s..%s calendar_days=%d calendar_status=%s available=%t", from, to, len(calendar.Cells), calendar.Status, calendar.Available)
	t.Logf("middle_day status=%s contributors=%d composition=%d middle_day_bytes=%d max_day_bytes=%d max_contributors=%d max_composition=%d", sampleStatus, sampleContributors, sampleComposition, medianDay, maxDay, maxContributors, maxComposition)
	t.Logf("period_now_bytes=%d period_build=%s return_points=%d asset_status=%s", parentSize, periodElapsed, len(period.InvestmentReturns.Points), period.AssetChange.Status)
	t.Logf("return_calendar_build=%s driver_build=%s drivers=%d", calendarElapsed, driverElapsed, len(drivers))
	t.Logf("folded_bytes=%d incremental_bytes=%d day_section_rows=%d driver_section_rows=%d residual_rows=%d", foldedSize, foldedSize-parentSize, daySectionRows, driverSectionRows, residualRows)
	t.Logf("folded_vs_capture_limit=%.6f folded_vs_cache_limit=%.6f over_capture_limit=%t", float64(foldedSize)/float64(readCaptureLimit), float64(foldedSize)/float64(readCacheLimit), foldedSize > readCaptureLimit)
	t.Logf("naive_return_day calls=%d of %d elapsed=%s complete=%t", naiveLimit, naiveDays, naiveElapsed, naiveLimit == naiveDays)
	t.Logf("contributions_bytes=%d rows_only_bytes=%d detail_incremental_bytes=%d items=%d contrib_build=%s contrib_status=%s", contribSize, rowsSize, contribSize-rowsSize, len(items), contribElapsed, contrib.Status)
	t.Logf("heap_alloc_before=%d heap_alloc_after_decode=%d heap_delta=%d vm_rss_kib=%d ttl=%s", before.HeapAlloc, after.HeapAlloc, int64(after.HeapAlloc)-int64(before.HeapAlloc), readVmRSS(), readTTL)
}

func assemblePeriodCapture(t *testing.T, in AnalyzePeriodInput, result PeriodAnalysisDTO) readCapture {
	t.Helper()
	capture := readCapture{Kind: "period_analysis", Query: in, Summary: map[string]any{}, Sections: map[string][]json.RawMessage{}}
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
			t.Fatal(err)
		}
		capture.Summary.(map[string]any)[projection.name] = part.Summary
		for name, rows := range part.Sections {
			capture.Sections[projection.name+"/"+name] = rows
		}
	}
	return capture
}

func attachPeriodDrilldown(t *testing.T, parent readCapture, days []wire.ReturnDayDTO, drivers []wire.AssetDriverDetailDTO) readCapture {
	t.Helper()
	names := make([]string, 0, len(parent.Sections))
	for name := range parent.Sections {
		names = append(names, name)
	}
	sort.Strings(names)
	parent.InitialSections = names
	parent.Details = map[string]any{}
	parent.DetailSections = map[string][]string{}
	for _, day := range days {
		detail, err := sectionedReadCapture("", nil, day, "composition", "contributors", "issues")
		if err != nil {
			t.Fatal(err)
		}
		key := "returnDays/" + day.Date
		parent.Details[key] = detail.Summary
		for _, field := range []string{"composition", "contributors", "issues"} {
			section := key + "/" + field
			parent.Sections[section] = detail.Sections[field]
			parent.DetailSections[key] = append(parent.DetailSections[key], section)
		}
	}
	for _, driver := range drivers {
		detail, err := sectionedReadCapture("", nil, driver, "byInstrument", "byAccount", "residualDetails")
		if err != nil {
			t.Fatal(err)
		}
		key := "assetDrivers/" + driver.DriverKey
		parent.Details[key] = detail.Summary
		for _, field := range []string{"byInstrument", "byAccount", "residualDetails"} {
			section := key + "/" + field
			parent.Sections[section] = detail.Sections[field]
			parent.DetailSections[key] = append(parent.DetailSections[key], section)
		}
	}
	return parent
}

func assembleContributionCapture(t *testing.T, in ListContributionsInput, value wire.ContributionDTO, items []wire.ContributionItemDTO) readCapture {
	t.Helper()
	capture, err := sectionedReadCapture("contributions", in, value, "rows")
	if err != nil {
		t.Fatal(err)
	}
	capture.InitialSections = []string{"rows"}
	capture.Details = map[string]any{}
	capture.DetailSections = map[string][]string{}
	for _, item := range items {
		detail, detailErr := sectionedReadCapture("", nil, item, "components", "byAccount")
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		key := "rows/" + item.Key
		capture.Details[key] = detail.Summary
		for _, field := range []string{"components", "byAccount"} {
			section := key + "/" + field
			capture.Sections[section] = detail.Sections[field]
			capture.DetailSections[key] = append(capture.DetailSections[key], section)
		}
	}
	return capture
}

func rowsOnlyContribution(full readCapture) readCapture {
	sections := map[string][]json.RawMessage{"rows": append([]json.RawMessage(nil), full.Sections["rows"]...)}
	return readCapture{
		Kind: full.Kind, Query: full.Query, Summary: full.Summary,
		Sections: sections, InitialSections: []string{"rows"},
	}
}

func frozenCaptureBytes(t *testing.T, e readCapture) []byte {
	t.Helper()
	e.Query = normalizeFrozenQuery(e.Query)
	raw, err := jsoncodec.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func frozenCaptureSize(t *testing.T, e readCapture) int {
	t.Helper()
	return len(frozenCaptureBytes(t, e))
}

func readVmRSS() int {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return -1
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var kib int
				if _, scanErr := fmt.Sscan(fields[1], &kib); scanErr == nil {
					return kib
				}
			}
		}
	}
	return -1
}
