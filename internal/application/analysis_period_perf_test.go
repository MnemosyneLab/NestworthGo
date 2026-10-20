//go:build perf

package application

import (
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Isolate repeated period work; this does not measure the whole service or memo.
func BenchmarkAssetTrendPeriodReuse(b *testing.B) {
	dates := make([]domain.LocalDate, 31)
	for i := range dates {
		dates[i] = time.Date(2024, 3, i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	for _, zone := range []string{"UTC", "America/New_York"} {
		for _, components := range []int{1, 32} {
			for _, cached := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/components=%d/reuse=%t", zone, components, cached), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						resolve := newAssetTrendPeriodResolver(zone, TrendWeek)
						for component := 0; component < components; component++ {
							for _, date := range dates {
								var err error
								if cached {
									_, err = resolve(date)
								} else {
									_, err = assetTrendPeriod(date, zone, TrendWeek)
								}
								if err != nil {
									b.Fatal(err)
								}
							}
						}
					}
				})
			}
		}
	}
}
