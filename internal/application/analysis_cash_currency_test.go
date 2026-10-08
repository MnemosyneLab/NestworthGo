package application

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisCashContributionsSeparateNativeCurrencies(t *testing.T) {
	t.Parallel()
	first, second := domain.NewAccountID(), domain.NewAccountID()
	instrument := domain.NewInstrumentID()
	query := analysisBaseQuery(domain.ValuationBase)
	result := domain.PeriodAnalysisResult{Query: query, AnalysisDayTimezone: "UTC", Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}, Status: domain.CompletenessOK}
	money := func(value string) domain.SignedMoney {
		m, err := domain.NewSignedMoney(decimal.RequireFromString(value), "CNY")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, row := range []struct {
		account       domain.AccountID
		currency      domain.CurrencyCode
		capital, gain string
		instrument    *domain.InstrumentID
	}{
		{first, "USD", "300", "3", nil},
		{second, "USD", "100", "1", nil},
		{first, "SGD", "100", "1", nil},
		{first, "CNY", "500", "0", nil},
		{first, "USD", "100", "7", &instrument},
	} {
		capital, gain := money(row.capital), money(row.gain)
		ending, _ := domain.NewSignedMoney(capital.Amount().Add(gain.Amount()), "CNY")
		component := domain.ComponentID{AccountID: row.account, Currency: row.currency, Cash: row.instrument == nil, InstrumentID: row.instrument}
		bucket, returnComponent := domain.BucketFXImpact, domain.ReturnFXImpact
		if row.instrument != nil {
			bucket, returnComponent = domain.BucketPriceChange, domain.ReturnPriceChange
		}
		result.Days = append(result.Days, domain.ComponentDay{
			Date: query.From, Component: component, Status: domain.CompletenessOK,
			BeginningValue: capital, EndingValue: ending, InvestedCapital: &capital, ReturnAmount: &gain,
			ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{returnComponent: gain},
			AssetBuckets:     map[domain.AttributionBucket]domain.SignedMoney{bucket: gain},
		})
	}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"cash:USD": "4", "cash:SGD": "1", "cash:CNY": "0", instrument.String(): "7"}
	if len(groups) != len(want) {
		t.Fatalf("groups=%+v", groups)
	}
	for _, group := range groups {
		if group.ReturnAmount.String() != want[group.Key] {
			t.Fatalf("group %s amount=%s", group.Key, group.ReturnAmount)
		}
		if group.Key == "cash:USD" && (group.ReturnRate == nil || group.ReturnRate.String() != "0.01") {
			t.Fatalf("USD cash rate includes another currency or instrument: %+v", group)
		}
	}
	for key, accounts := range map[string]int{"cash:USD": 2, "cash:SGD": 1} {
		item, err := projectContributionItem(result, "", query, ContributionTotalReturn, ContributionGroupInstrument, key)
		if err != nil {
			t.Fatal(err)
		}
		if item.Amount.CanonicalAmount() != want[key] || item.Amount.Currency() != "CNY" || len(item.ByAccount) != accounts || len(item.Components) != 1 || item.Components[0].Amount.CanonicalAmount() != want[key] {
			t.Fatalf("cash detail=%+v", item)
		}
		if item.HistoryHint.InstrumentID != "" {
			t.Fatalf("cash key treated as an instrument: %+v", item.HistoryHint)
		}
	}
	detail := foldAssetDriverDetail(result, "", string(domain.BucketFXImpact))
	if len(detail.ByInstrument) != 2 {
		t.Fatalf("cash driver dimensions=%+v", detail.ByInstrument)
	}
	for _, row := range detail.ByInstrument {
		if row.InstrumentID != "" || row.Amount.Currency() != "CNY" || row.Amount.CanonicalAmount() != want[row.Key] {
			t.Fatalf("cash driver row=%+v", row)
		}
	}
	result.Query.IncludeCash = false
	groups, err = FoldReturnGroups(result, GroupByInstrument)
	if err != nil || len(groups) != 1 || groups[0].Key != instrument.String() {
		t.Fatalf("excluded cash leaked into groups: %+v, %v", groups, err)
	}
}
