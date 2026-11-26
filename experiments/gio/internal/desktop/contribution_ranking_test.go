package desktop

import (
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestContributionRankingUsesExactMagnitudeAndDirection(t *testing.T) {
	row := func(key, exact string) application.ContributionRow {
		t.Helper()
		money, err := domain.NewSignedMoney(decimal.RequireFromString(exact), "USD")
		if err != nil {
			t.Fatal(err)
		}
		return application.ContributionRow{Key: key, Amount: &money}
	}
	data := []application.ContributionRow{
		row("zero", "0"), row("positive-tie", "1"), {Key: "missing"}, row("negative-larger", "-1.00000001"),
		row("positive-first", "1"), row("negative-smaller", "-0.125"),
	}
	cases := []struct {
		dir   contributionDirection
		limit int
		keys  []string
	}{
		{contributionLargest, 3, []string{"negative-larger", "positive-first", "positive-tie"}},
		{contributionLargest, -1, []string{"negative-larger", "positive-first", "positive-tie", "negative-smaller", "zero"}},
		{contributionPositive, -1, []string{"positive-first", "positive-tie"}},
		{contributionNegative, -1, []string{"negative-larger", "negative-smaller"}},
	}
	for _, tc := range cases {
		var keys []string
		for _, row := range rankedContributionRows(data, tc.dir, tc.limit) {
			keys = append(keys, row.Key)
		}
		if !reflect.DeepEqual(keys, tc.keys) {
			t.Fatalf("direction %d, limit %d: got %v, want %v", tc.dir, tc.limit, keys, tc.keys)
		}
	}
	if data[0].Key != "zero" || data[1].Key != "positive-tie" {
		t.Fatal("ranking changed the order of the complete details list")
	}
}
