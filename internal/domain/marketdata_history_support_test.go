package domain

import (
	"testing"
	"time"
)

func TestResolveInstrumentHistorySupportUsesTypeAndMarketNotProviderName(t *testing.T) {
	usYahooStock := ResolveInstrumentHistorySupport("stock", "US", YahooFinanceProviderKey)
	if usYahooStock.Status != InstrumentRouteOK {
		t.Fatalf("US Yahoo stock = %+v", usYahooStock)
	}
	usEquity := ResolveInstrumentHistorySupport("stock", "US", YahooFinanceProviderKey)
	if usEquity.Status != InstrumentRouteOK {
		t.Fatalf("US Equity stock = %+v", usEquity)
	}
	cnYahoo := ResolveInstrumentHistorySupport("stock", "CN", YahooFinanceProviderKey)
	if cnYahoo.Status != InstrumentRouteOK {
		t.Fatalf("CN Yahoo stock = %+v", cnYahoo)
	}
	cryptoYahoo := ResolveInstrumentHistorySupport("crypto", "US", YahooFinanceProviderKey)
	if cryptoYahoo.Status != InstrumentRouteOK {
		t.Fatalf("Yahoo crypto should use UTC daily bars even when market is US: %+v", cryptoYahoo)
	}
	cryptoEquity := ResolveInstrumentHistorySupport("crypto", "US", "retired_provider")
	if cryptoEquity.Status != InstrumentRouteUnsupported || cryptoEquity.Reason != "provider_not_selectable" {
		t.Fatalf("Equity crypto = %+v", cryptoEquity)
	}
	goldMetal := ResolveInstrumentHistorySupport("precious_metal", "US", YahooFinanceProviderKey)
	if goldMetal.Status != InstrumentRouteUnsupported || goldMetal.Reason != InstrumentTypeUnsupported {
		t.Fatalf("precious metal display name must not inherit equity repair: %+v", goldMetal)
	}
	unknownMarket := ResolveInstrumentHistorySupport("stock", "ZZ", YahooFinanceProviderKey)
	if unknownMarket.Status != InstrumentRouteUnsupported || unknownMarket.Reason != MarketSessionUnsupported {
		t.Fatalf("unknown equity market = %+v", unknownMarket)
	}
}

func TestLastFinalizedCryptoMarketDateIsUTCDailyBarNotEquityClose(t *testing.T) {
	sgt, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 0, 5, 0, 0, sgt)
	got, err := LastFinalizedCryptoMarketDate(now)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-09-08" {
		t.Fatalf("crypto last finalized = %q, want 2026-09-08 (UTC 2026-09-09 bar is still open)", got)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	afterUSClose := time.Date(2026, 9, 9, 16, 5, 0, 0, ny)
	cryptoDate, err := LastFinalizedCryptoMarketDate(afterUSClose)
	if err != nil {
		t.Fatal(err)
	}
	if cryptoDate != "2026-09-08" {
		t.Fatalf("crypto after US close = %q, want 2026-09-08, not the US session date", cryptoDate)
	}
	eligible, err := CryptoDailyBarEligibleAt("2026-09-04")
	if err != nil {
		t.Fatal(err)
	}
	if eligible.UTC().Format(time.RFC3339Nano) != "2026-09-04T23:59:59.999Z" {
		t.Fatalf("crypto eligibility presented as an exchange close: %s", eligible.UTC())
	}
}
