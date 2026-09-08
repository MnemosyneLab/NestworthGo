package analysis

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReturnDTOUsesCanonicalRatesCoverageAndIssues(t *testing.T) {
	amount, err := domain.ParseSignedMoney("-12.5", "USD")
	if err != nil {
		t.Fatal(err)
	}
	rate := decimal.NewFromInt(1).Div(decimal.NewFromInt(8))
	forced := "base"
	value := application.ReturnCalendarResult{
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessPartial, MissingReason: "missing quote", ValuationForced: &forced},
		Summary:              application.ReturnCalendarSummary{ReturnAmount: &amount, ReturnRate: &rate, Coverage: domain.RateCoverage{RatedDays: 2, TotalDays: 3}},
		Days: []application.ReturnDayResult{{
			Date:         "2026-08-01",
			ReturnAmount: &amount,
			ReturnRate:   &rate,
			Coverage:     domain.RateCoverage{RatedDays: 2, TotalDays: 3},
			Issues:       []application.ReturnIssue{{Date: "2026-08-02", Status: domain.CompletenessPartial, MissingReason: "missing quote"}},
		}},
	}
	dto := fromReturnCalendar(value)
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	if shape["valuationForced"] != "base" || shape["status"] != "partial" || shape["missingReason"] != "missing quote" {
		t.Fatalf("calendar metadata = %s", encoded)
	}
	summary := shape["summary"].(map[string]any)
	if summary["returnRate"] != "0.125" || summary["ratedDays"] != float64(2) || summary["totalDays"] != float64(3) {
		t.Fatalf("calendar summary = %s", encoded)
	}
	cells := shape["cells"].([]any)
	cell := cells[0].(map[string]any)
	issues := cell["issues"].([]any)
	if len(issues) != 1 || issues[0].(map[string]any)["date"] != "2026-08-02" {
		t.Fatalf("cell issues = %s", encoded)
	}
}

func TestReturnTrendDTOKeepsUnforcedRateNullAndMapsSources(t *testing.T) {
	amount, err := domain.ParseSignedMoney("7.5", "USD")
	if err != nil {
		t.Fatal(err)
	}
	rate := decimal.NewFromInt(3).Div(decimal.NewFromInt(4))
	dto := fromReturnTrend(application.ReturnTrendResult{
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessOK},
		Display:              application.ReturnTrendLinkedRate,
		Points:               []application.ReturnTrendPoint{{Date: "2026-08-01", Amount: &amount, Rate: &rate, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}},
		Sources:              []application.ReturnSource{{Key: "price_change", Label: "price_change", Amount: &amount, Share: &rate}},
		Coverage:             domain.RateCoverage{RatedDays: 1, TotalDays: 1},
	})
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	if value, exists := shape["valuationForced"]; !exists || value != nil {
		t.Fatalf("unforced trend valuation = %s", encoded)
	}
	if shape["display"] != string(application.ReturnTrendLinkedRate) || shape["ratedDays"] != float64(1) {
		t.Fatalf("trend metadata = %s", encoded)
	}
	if len(shape["sources"].([]any)) != 1 {
		t.Fatalf("trend sources = %s", encoded)
	}
	source := shape["sources"].([]any)[0].(map[string]any)
	if source["key"] != "price_change" || source["share"] != "0.75" {
		t.Fatalf("source shape = %s", encoded)
	}
	point := shape["points"].([]any)[0].(map[string]any)
	if point["rate"] != "0.75" || point["ratedDays"] != float64(1) || point["totalDays"] != float64(1) || point["value"] != nil {
		t.Fatalf("trend point rate = %s", encoded)
	}
}
