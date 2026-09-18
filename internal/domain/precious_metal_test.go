package domain

import (
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func TestMetalPriceUsesTroyOuncesAndTargetCurrency(t *testing.T) {
	raw, err := ParseUnitPrice(TroyOunceGrams)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ unit, rate, want string }{
		{"g", "1", "1"}, {"g", "6.9", "6.9"}, {"troy_oz", "1", TroyOunceGrams}, {"troy_oz", "6.9", "214.61398992"},
	} {
		got, err := ConvertMetalPrice(raw, tc.unit, decimal.RequireFromString(tc.rate))
		if err != nil || got.Canonical() != tc.want {
			t.Fatalf("%+v: got %s, %v", tc, got.Canonical(), err)
		}
	}
	if _, err := ConvertMetalPrice(raw, "oz", decimal.NewFromInt(1)); err == nil {
		t.Fatal("ordinary ounces must be refused")
	}
	if _, err := ConvertMetalPrice(raw, "g", decimal.Zero); err == nil {
		t.Fatal("missing FX must not become zero")
	}
}

func TestMetalDailyReferenceDoesNotUseEquityClose(t *testing.T) {
	got, err := MetalDailyBarEligibleAt("2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 18, 3, 59, 59, 999000000, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if ResolveInstrumentHistorySupport("precious_metal", "US", YahooFinanceProviderKey).Status != InstrumentRouteUnsupported {
		t.Fatal("legacy unqualified metals must stay unsupported")
	}
	if ResolveInstrumentHistorySupport("precious_metal", MetalFuturesMarket, YahooFinanceProviderKey).Status != InstrumentRouteOK {
		t.Fatal("COMEX reference history must be available")
	}
}
