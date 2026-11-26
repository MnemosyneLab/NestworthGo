package desktop

import (
	"context"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func returnTrendChartFixture(t *testing.T, display application.ReturnTrendDisplay) application.ReturnTrendResult {
	t.Helper()
	money := func(text string) *domain.SignedMoney {
		amount, err := domain.NewSignedMoney(decimal.RequireFromString(text), "USD")
		if err != nil {
			t.Fatal(err)
		}
		return &amount
	}
	positive, negative := money("5.25"), money("-2.50")
	positiveRate, negativeRate := decimal.RequireFromString("0.0525"), decimal.RequireFromString("-0.025")
	rated := domain.RateCoverage{RatedDays: 1, TotalDays: 1}
	available := application.AnalysisAvailability{Available: true, Status: domain.CompletenessOK}
	points := []application.ReturnTrendPoint{
		{Date: "2026-08-01", Amount: positive, Rate: &positiveRate, Coverage: rated, AnalysisAvailability: available},
		{Date: "2026-08-02", ValuationMissing: true, Coverage: domain.RateCoverage{TotalDays: 1},
			AnalysisAvailability: application.AnalysisAvailability{Status: domain.CompletenessUnavailable, MissingReason: "daily return is incomplete"}},
		{Date: "2026-08-03", Amount: negative, Rate: &negativeRate, Coverage: rated, AnalysisAvailability: available},
	}
	switch display {
	case application.ReturnTrendCumulativeAmount:
		points[0].Value, points[1].Value, points[2].Value = positive, positive, money("2.75")
	case application.ReturnTrendPeriodReturnAmount:
		points[0].Value, points[2].Value = positive, negative
	}
	return application.ReturnTrendResult{Display: display, Points: points, Amount: money("2.75"),
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessPartial},
		ReturnDayCoverage:    application.ReturnDayCoverage{MissingValuationDays: 1}, Coverage: domain.RateCoverage{RatedDays: 2, TotalDays: 3}}
}

func TestReturnTrendChartProjectionKeepsCivilDatesAndGaps(t *testing.T) {
	view := ui.View{Language: i18n.English, DateFormat: settings.DateFormatISO, DecimalSeparator: ".", GroupingSeparator: ",", Colors: &lightPalette}
	for _, tc := range []struct {
		display                     application.ReturnTrendDisplay
		from, firstValue, lastValue string
		first, last                 float64
		baseline                    bool
	}{
		{application.ReturnTrendCumulativeAmount, "2026-07-31", "US$5.25", "US$2.75", 5.25, 2.75, true},
		{application.ReturnTrendLinkedRate, "2026-08-01", "+5.25%", "-2.5%", 0.0525, -0.025, false},
		{application.ReturnTrendPeriodReturnAmount, "2026-08-01", "US$5.25", "-US$2.50", 5.25, -2.5, false},
	} {
		t.Run(string(tc.display), func(t *testing.T) {
			spec := returnTrendLine(view, returnTrendChartFixture(t, tc.display), "2026-08-01", "2026-08-03")
			rows, points := spec.Rows(), spec.Series[0].Points
			if spec.From.Format(time.DateOnly) != tc.from || spec.To.Format(time.DateOnly) != "2026-08-03" || len(spec.Validate()) != 0 {
				t.Fatalf("chart bounds or IDs are invalid: %v, %v, %v", spec.From, spec.To, spec.Validate())
			}
			if tc.baseline {
				if len(points) != 4 || points[0].Time != spec.From || points[0].Value != charts.Num(0) || rows[0].Value != "US$0.00" {
					t.Fatalf("prior civil day is not the zero observation: %+v", points)
				}
				rows, points = rows[1:], points[1:]
			}
			if len(points) != 3 || points[0].Value != charts.Num(tc.first) || points[2].Value != charts.Num(tc.last) ||
				rows[0].Value != tc.firstValue || rows[2].Value != tc.lastValue {
				t.Fatalf("lost observed amounts or rates: %+v", rows)
			}
			gap := rows[1]
			if points[1].Value.Valid() || !gap.Missing || gap.ID != "2026-08-02" || !strings.Contains(analysisLineTableText(gap), "Missing valuation") {
				t.Fatalf("the dated missing valuation did not interrupt the chart: %+v", gap)
			}
		})
	}
}

type returnTrendChartReader struct {
	calendarFilterReader
	t *testing.T
}

func (r *returnTrendChartReader) ReturnTrend(_ context.Context, _ domain.AnalysisQuery, display application.ReturnTrendDisplay) (application.ReturnTrendResult, error) {
	return returnTrendChartFixture(r.t, display), nil
}

func TestReturnTrendChartAndTableThroughShell(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	p := &returnAnalysisPage{ctx: t.Context(), reader: &returnTrendChartReader{t: t}, clock: func() time.Time { return now }, invalidate: func() {}}
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	settle := func() {
		for range 3 {
			p.workers.Wait()
			h.frame()
		}
	}
	h.click("Return analysis", false)
	settle()
	h.click("Return trend", false)
	settle()
	assertPageLabel(t, &h.router, "2026-07-31 · US$0.00 · Starting value before the selected range")
	assertPageLabel(t, &h.router, "2026-08-02 · Unavailable · Rated days 0/1 · Missing valuation")
	h.router.Source().Execute(key.FocusCmd{Tag: &p.output.trendChart})
	h.frame()
	h.focused(&p.output.trendChart)
	h.press(key.NameHome)
	h.press(key.NameRightArrow)
	h.press(key.NameReturn)
	if selection := p.output.trendSelection; selection.Item != "2026-08-01" {
		t.Fatalf("keyboard selected %+v, want first recorded date", selection)
	}
	h.press(key.NameEscape)
	if !p.output.trendSelection.IsZero() {
		t.Fatal("Escape did not clear chart selection")
	}
	h.click("Display", true)
	h.click("Linked return rate", false)
	settle()
	assertPageLabel(t, &h.router, "2026-08-01 · +5.25% · Rated days 1/1")
	if findPageLabel(&h.router, "2026-07-31 · US$0.00 · Starting value before the selected range") != nil {
		t.Fatal("linked return displayed a cumulative baseline")
	}
	h.shell.prefs.saved.Appearance = settings.AppearanceDark
	h.size = image.Pt(700, 480)
	settle()
	scrollOverlayTestPage(h, 540)
	assertPageLabel(t, &h.router, "2026-08-02 · Unavailable · Rated days 0/1 · Missing valuation")
	h.router.Source().Execute(key.FocusCmd{Tag: &p.output.list})
	h.frame()
	h.focused(&p.output.list)
}
