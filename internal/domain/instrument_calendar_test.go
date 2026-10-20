package domain

import "testing"

func TestInstrumentCalendarCoversEveryType(t *testing.T) {
	rules := map[InstrumentType]bool{InstrumentStock: true, InstrumentETF: true, InstrumentMutualFund: true, InstrumentBankInvestmentProduct: true, InstrumentBond: true, InstrumentPreciousMetal: true, InstrumentCrypto: false, InstrumentOther: false}
	if len(rules) != len(AllInstrumentTypes()) {
		t.Fatal("new type requires an explicit calendar rule")
	}
	for _, kind := range AllInstrumentTypes() {
		weekend, ok := rules[kind]
		if !ok {
			t.Fatalf("missing rule for %s", kind)
		}
		i := Instrument{ID: NewInstrumentID(), Type: kind}
		if DefaultInstrumentClosedDate(kind, "2026-10-18") != weekend || InstrumentClosedCarry(i, "2026-10-16", "2026-10-18", nil) != weekend {
			t.Fatalf("weekend: %s", kind)
		}
		if DefaultInstrumentClosedDate(kind, "invalid") || InstrumentClosedCarry(i, "2026-10-15", "2026-10-18", nil) {
			t.Fatalf("missing anchor trading day: %s", kind)
		}
		facts := []InstrumentClosure{{InstrumentID: i.ID, Date: "2026-10-19"}}
		if !InstrumentClosedCarry(i, "2026-10-18", "2026-10-19", facts) || InstrumentClosedCarry(i, "2026-10-18", "2026-10-20", facts) {
			t.Fatalf("explicit closure: %s", kind)
		}
	}
}

func TestReviewPerformanceClosedCarryPreservesDatesAndCalendarSpan(t *testing.T) {
	_, instrument := reviewFixture(t)
	instrument.Type = InstrumentBankInvestmentProduct
	instrument.QuoteSource = QuoteSourceAgent
	quotes := []InstrumentQuote{reviewQuote(t, instrument, "2026-09-17", "1"), reviewQuote(t, instrument, "2026-10-16", "1.01")}
	closure := InstrumentClosure{InstrumentID: instrument.ID, Date: "2026-10-19"}
	series := NewReviewDailyQuoteSeries(instrument.ID, quotes, closure)
	performance, reason := series.ResolveDailyPerformance(instrument, "2026-10-19", 30, 7)
	if performance == nil || reason != "" || performance.LatestOn != "2026-10-16" || performance.AsOfOn != "2026-10-19" || performance.BaselineOn != "2026-09-17" || performance.ElapsedDays != 32 || performance.AnnualizedPercent == nil {
		t.Fatalf("closed performance: %+v %s", performance, reason)
	}
	if performance.PeriodPercent != "1" || *performance.AnnualizedPercent != "11.40625" {
		t.Fatalf("annualization must use evaluation span: %+v", performance)
	}
	spec := syntheticReviewSpec()
	spec.Predicate.All = append(spec.Predicate.All[:1], ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "annualized", Operator: ">", Value: "10", LookbackDays: 30}})
	quantity := "1"
	input := ReviewDailyInput{On: "2026-10-19", InstrumentID: instrument.ID, Quantity: &quantity, InstrumentType: &instrument.Type, NativeCurrency: &instrument.QuoteCurrency, Performance: map[int]QuotePerformance{30: *performance}}
	result, err := EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewTrue {
		t.Fatalf("closed performance not admitted by review: %+v %v", result, err)
	}
	if q, reason := series.ResolveDailyClose(instrument, "2026-10-20"); q != nil || reason != "daily_close_missing" {
		t.Fatalf("missing trading day: %+v %s", q, reason)
	}
	closure.InstrumentID = NewInstrumentID()
	if q, _ := NewReviewDailyQuoteSeries(instrument.ID, quotes, closure).ResolveDailyClose(instrument, "2026-10-19"); q != nil {
		t.Fatal("another instrument's closure leaked")
	}
	instrument.QuoteSource = QuoteSourceManual
	if q, reason := series.ResolveDailyClose(instrument, "2026-10-20"); q == nil || reason != "" || q.EffectiveDate != "2026-10-16" {
		t.Fatalf("manual preference: %+v %s", q, reason)
	}
}
