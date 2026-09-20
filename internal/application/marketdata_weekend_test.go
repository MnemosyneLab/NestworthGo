package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestEquityWeekendDoesNotCreateHistoryWork(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for _, market := range []string{"US", "SG"} {
		t.Run(market, func(t *testing.T) {
			coverage := domain.InstrumentHistoryCoverage{InstrumentType: "etf", Market: market, ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "QQQM", CloseFetchedAt: map[string]time.Time{}}
			for _, date := range []string{"2026-09-11", "2026-09-14", "2026-09-15", "2026-09-16", "2026-09-17"} {
				coverage.CloseMarketDates = append(coverage.CloseMarketDates, date)
				coverage.CloseFetchedAt[date] = now
			}
			need, err := planInstrumentRepairNeed(coverage, "2026-09-18", "2026-09-17")
			if err != nil {
				t.Fatal(err)
			}
			if len(need.MissingRanges) != 0 {
				t.Fatalf("weekend counted as missing: %+v", need.MissingRanges)
			}
			planned, err := applyHistorySyncPolicy(need, coverage, "2026-09-17", now, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(planned.FetchRanges) != 0 {
				t.Fatalf("weekend fetch: %+v", planned.FetchRanges)
			}
			forced, err := applyHistorySyncPolicy(need, coverage, "2026-09-17", now, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, rng := range forced.FetchRanges {
				if instrumentKnownClosedDate(coverage, string(rng.Start)) || instrumentKnownClosedDate(coverage, string(rng.End)) {
					t.Fatal("force recheck starts or ends with a closed day")
				}
			}
			if len(forced.FetchRanges) != 1 {
				t.Fatalf("weekend split a continuous recheck: %+v", forced.FetchRanges)
			}

		})
	}
}

func TestCryptoWeekendStillRequiresHistory(t *testing.T) {
	coverage := domain.InstrumentHistoryCoverage{InstrumentType: "crypto", Market: "CRYPTO", ProviderKey: CoinGeckoProviderKey, ProviderSymbol: "bitcoin"}
	need := InstrumentRepairNeed{FetchRange: DateRange{Start: "2026-09-12", End: "2026-09-13"}}
	planned, err := applyHistorySyncPolicy(need, coverage, "2026-09-17", time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.FetchRanges) != 1 || planned.FetchRanges[0] != need.FetchRange {
		t.Fatalf("crypto weekend lost: %+v", planned)
	}
}

func TestLaterCloseSuppressesInteriorHolidayRepair(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	coverage := domain.InstrumentHistoryCoverage{InstrumentType: "etf", Market: "SG", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "ES3.SI", CloseMarketDates: []string{"2026-09-11", "2026-09-14", "2026-09-16", "2026-09-17"}, CloseFetchedAt: map[string]time.Time{}}
	for _, date := range coverage.CloseMarketDates {
		coverage.CloseFetchedAt[date] = now
	}
	// An expired provider no-observation must not cause another holiday query.
	coverage.NoObservationDates = []string{"2026-09-15"}
	coverage.NoObservationExpiresAt = map[string]time.Time{"2026-09-15": now.Add(-time.Hour)}
	need, err := planInstrumentRepairNeed(coverage, "2026-09-18", "2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	if len(need.MissingRanges) != 0 {
		t.Fatalf("holiday is a gap: %+v", need.MissingRanges)
	}
	planned, err := applyHistorySyncPolicy(need, coverage, "2026-09-17", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.FetchRanges) != 0 {
		t.Fatalf("holiday recheck: %+v", planned.FetchRanges)
	}
	forced, err := applyHistorySyncPolicy(need, coverage, "2026-09-17", now, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rng := range forced.FetchRanges {
		if rng.Start <= "2026-09-15" && rng.End >= "2026-09-15" {
			found = true
		}
	}
	if !found {
		t.Fatal("force recheck must include inferred holiday")
	}
	// The same rule applies even without a stored no-observation record.
	coverage.NoObservationDates = nil
	need, err = planInstrumentRepairNeed(coverage, "2026-09-18", "2026-09-17")
	if err != nil || len(need.MissingRanges) != 0 {
		t.Fatalf("inferred gap: %+v %v", need, err)
	}
}

func TestHistoricalValuationAcceptsInferredHolidayWithoutFuturePrice(t *testing.T) {
	provider := YahooFinanceProviderKey
	instrument := domain.Instrument{ID: domain.NewInstrumentID(), ProviderKey: &provider, ProviderBindingRevision: 1}
	quote := domain.InstrumentQuote{InstrumentID: instrument.ID, EffectiveDate: "2026-09-14", SourcePolicyVersion: "policy"}
	coverage := domain.InstrumentHistoryCoverage{InstrumentID: instrument.ID, InstrumentType: "etf", Market: "US", ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: "policy", CloseMarketDates: []string{"2026-09-14", "2026-09-16"}}
	if !historicalInstrumentCoverageCompleteAtMarketDate(instrument, quote, []domain.InstrumentHistoryCoverage{coverage}, "2026-09-15") {
		t.Fatal("holiday carry-forward remains incomplete")
	}
	coverage.BindingRevision = 2
	if historicalInstrumentCoverageCompleteAtMarketDate(instrument, quote, []domain.InstrumentHistoryCoverage{coverage}, "2026-09-15") {
		t.Fatal("another binding's later close must not close this gap")
	}
}

func TestMetalWeekendCarryAndRepairAgree(t *testing.T) {
	for _, symbol := range []string{"GC=F", "SI=F"} {
		t.Run(symbol, func(t *testing.T) {
			provider := YahooFinanceProviderKey
			inst := domain.Instrument{ID: domain.NewInstrumentID(), ProviderKey: &provider, ProviderBindingRevision: 1}
			quote := domain.InstrumentQuote{InstrumentID: inst.ID, EffectiveDate: "2026-09-18", SourcePolicyVersion: "policy"}
			now := time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC)
			coverage := domain.InstrumentHistoryCoverage{InstrumentID: inst.ID, InstrumentType: "precious_metal", Market: "COMEX", ProviderSymbol: symbol, ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: "policy", CloseMarketDates: []string{"2026-09-18"}, CloseFetchedAt: map[string]time.Time{"2026-09-18": now}}
			for _, date := range []string{"2026-09-19", "2026-09-20"} {
				if !historicalInstrumentCoverageCompleteAtMarketDate(inst, quote, []domain.InstrumentHistoryCoverage{coverage}, date) {
					t.Fatalf("%s: Friday reference should cover weekend", date)
				}
			}
			if historicalInstrumentCoverageCompleteAtMarketDate(inst, quote, []domain.InstrumentHistoryCoverage{coverage}, "2026-09-21") {
				t.Fatal("missing Monday close must remain incomplete")
			}
			for _, force := range []bool{false, true} {
				need := InstrumentRepairNeed{FetchRange: DateRange{Start: "2026-09-19", End: "2026-09-20"}}
				planned, err := applyHistorySyncPolicy(need, coverage, "2026-09-20", now, force)
				if err != nil || len(planned.FetchRanges) != 0 {
					t.Fatalf("weekend-only fetch: %+v %v", planned, err)
				}
			}
			coverage.UnverifiedDates = []string{"2026-09-19"}
			if historicalInstrumentCoverageCompleteAtMarketDate(inst, quote, []domain.InstrumentHistoryCoverage{coverage}, "2026-09-19") {
				t.Fatal("explicit unverified evidence must not be overridden")
			}
			coverage.UnverifiedDates = nil
			coverage.ProviderSymbol = "UNKNOWN"
			if historicalInstrumentCoverageCompleteAtMarketDate(inst, quote, []domain.InstrumentHistoryCoverage{coverage}, "2026-09-19") {
				t.Fatal("unknown futures must not inherit GC/SI calendar")
			}
		})
	}
}
