package application

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAssetTrendPeriodResolverMatchesPeriod(t *testing.T) {
	for _, zone := range []string{"UTC", "America/New_York", "invalid/zone"} {
		for _, grain := range []AssetTrendGranularity{TrendDay, TrendWeek, TrendMonth} {
			t.Run(zone+"/"+string(grain), func(t *testing.T) {
				resolve := newAssetTrendPeriodResolver(zone, grain)
				// Unordered, duplicate, cross-year Monday, leap day and DST dates.
				for _, date := range []domain.LocalDate{"2024-03-11", "2021-01-01", "2024-02-29", "2024-03-10", "2021-01-01", "2024-11-03", "bad-date", "2024-03-10"} {
					want, wantErr := assetTrendPeriod(date, zone, grain)
					got, gotErr := resolve(date)
					if got != want || !reflect.DeepEqual(gotErr, wantErr) {
						t.Fatalf("%s: got %q, %v; want %q, %v", date, got, gotErr, want, wantErr)
					}
				}
			})
		}
	}
}

func TestAssetTrendPeriodLazyErrors(t *testing.T) {
	for _, grain := range []AssetTrendGranularity{TrendDay, TrendWeek, TrendMonth} {
		for _, metric := range []AssetTrendMetric{TrendNetWorth, TrendIncome, TrendReturnRate} {
			result := domain.PeriodAnalysisResult{AnalysisDayTimezone: "invalid/zone"}
			if _, err := foldAssetTrend(result, "", grain, metric); err != nil {
				t.Fatalf("empty %s/%s: %v", grain, metric, err)
			}
		}
		for _, display := range []bool{false, true} {
			result := domain.PeriodAnalysisResult{AnalysisDayTimezone: "invalid/zone", Days: []domain.ComponentDay{{Date: "bad-date"}}}
			if display {
				result.Days[0].DisplayEnding = &domain.ValuationAmount{Currency: "USD"}
			}
			_, err := foldAssetTrend(result, "", grain, TrendNetWorth)
			_, wantErr := assetTrendPeriod("bad-date", result.AnalysisDayTimezone, grain)
			if !reflect.DeepEqual(err, wantErr) {
				t.Fatalf("%s/display=%t: got %v want %v", grain, display, err, wantErr)
			}
		}
	}
}

func TestAssetTrendPeriodRequestsOwnResults(t *testing.T) {
	result := domain.PeriodAnalysisResult{AnalysisDayTimezone: "America/New_York", Status: domain.CompletenessOK, Days: []domain.ComponentDay{
		analysisTestDay(t, "2024-03-11", "asset", "100.123456789", map[domain.AttributionBucket]string{domain.BucketIncome: "3.5"}),
		analysisTestDay(t, "2024-03-10", "asset", "100.123456789", map[domain.AttributionBucket]string{domain.BucketIncome: "1.25"}),
		analysisTestDay(t, "2024-03-11", "other", "-40.123456789", nil),
	}}
	for _, display := range []bool{false, true} {
		if display {
			for i := range result.Days {
				day := &result.Days[i]
				day.DisplayLiability = i == 2
				value := &domain.MoneyView{Amount: day.EndingValue.Amount().Abs().String(), Currency: "USD"}
				day.DisplayEnding = &domain.ValuationAmount{Currency: "USD", Total: value, KnownSubtotal: value, Quality: domain.ValuationQuality{Availability: domain.ValuationFull, Coverage: domain.CoverageVerified, IncludedCount: 1, ValuedCount: 1}}
			}
		}
		for _, metric := range []AssetTrendMetric{TrendNetWorth, TrendLiabilities, TrendIncome} {
			first, err := foldAssetTrend(result, "", TrendMonth, metric)
			if err != nil {
				t.Fatal(err)
			}
			second, err := foldAssetTrend(result, "", TrendMonth, metric)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("display=%t/%s: repeated projection differs: %v", display, metric, err)
			}
			if len(first.Points) != 1 {
				t.Fatalf("points = %+v", first.Points)
			}
			first.Points[0].Period = "changed"
			if first.Points[0].Value != nil {
				*first.Points[0].Value = domain.SignedMoney{}
			}
			if first.Points[0].DisplayValuation != nil && first.Points[0].DisplayValuation.Total != nil {
				first.Points[0].DisplayValuation.Total.Amount = "changed"
			}
			third, err := foldAssetTrend(result, "", TrendMonth, metric)
			if err != nil || !reflect.DeepEqual(second, third) {
				t.Fatalf("display=%t/%s: result aliased across requests: %v", display, metric, err)
			}
		}
	}
}
