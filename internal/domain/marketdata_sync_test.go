package domain

import (
	"testing"
	"time"
)

func TestResolveInstrumentRouteUSSelectableAndNoFallback(t *testing.T) {
	usYahoo := ResolveInstrumentRoute("US", YahooFinanceProviderKey, "AAPL")
	if usYahoo.Status != InstrumentRouteOK || usYahoo.ProviderKey != YahooFinanceProviderKey {
		t.Fatalf("US Yahoo = %+v", usYahoo)
	}
	usTiingo := ResolveInstrumentRoute("XNAS", TiingoProviderKey, "AAPL")
	if usTiingo.Status != InstrumentRouteOK || usTiingo.ProviderKey != TiingoProviderKey {
		t.Fatalf("US Tiingo = %+v", usTiingo)
	}
	usDefault := ResolveInstrumentRoute("US", "", "AAPL")
	if usDefault.Status != InstrumentRouteOK || usDefault.ProviderKey != YahooFinanceProviderKey {
		t.Fatalf("US default = %+v, want Yahoo upgrade default", usDefault)
	}
	missing := ResolveInstrumentRoute("US", TiingoProviderKey, "")
	if missing.Status != InstrumentRouteBindingMissing {
		t.Fatalf("missing US binding = %+v", missing)
	}
	cn := ResolveInstrumentRoute("CN", YahooFinanceProviderKey, "0700.HK")
	if cn.Status != InstrumentRouteOK || cn.ProviderKey != YahooFinanceProviderKey {
		t.Fatalf("CN Yahoo = %+v", cn)
	}
	cnTiingo := ResolveInstrumentRoute("CN", TiingoProviderKey, "000001.SS")
	if cnTiingo.Status != InstrumentRouteUnsupported || cnTiingo.Reason != "tiingo_us_listed_only" || cnTiingo.ProviderKey != "" {
		t.Fatalf("CN Tiingo silently fell back: %+v", cnTiingo)
	}
	other := ResolveInstrumentRoute("SG", "unknown", "ES3")
	if other.Status != InstrumentRouteUnsupported {
		t.Fatalf("other unknown provider = %+v", other)
	}
	fx := ResolveInstrumentRoute("US", FrankfurterProviderKey, "AAPL")
	if fx.Status != InstrumentRouteUnsupported || fx.Reason != "frankfurter_is_fx_only" {
		t.Fatalf("Frankfurter instrument route = %+v", fx)
	}
}

func TestNoObservationExpiryRecentVsOlder(t *testing.T) {
	checked := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	// Last finalized at this clock is 2026-09-09; last 7 dates include 2026-09-03..09.
	recent, err := NoObservationExpiresAt("2026-09-06", checked)
	if err != nil {
		t.Fatal(err)
	}
	if !recent.Equal(checked.Add(RecentNoObservationTTL)) {
		t.Fatalf("recent expiry = %s, want +24h", recent)
	}
	older, err := NoObservationExpiresAt("2026-08-01", checked)
	if err != nil {
		t.Fatal(err)
	}
	if !older.Equal(checked.Add(OlderNoObservationTTL)) {
		t.Fatalf("older expiry = %s, want +30d", older)
	}
}

func TestDecideHistoryFetchSkipForceAndCorrection(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	lastFinalized := "2026-09-08"
	closeFetched := now.Add(-2 * time.Hour)
	skipClose := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-09-04", LastFinalized: lastFinalized, Now: now,
		HasClose: true, CloseFetchedAt: closeFetched,
	})
	if skipClose.Action != HistorySkipExistingClose {
		t.Fatalf("old close = %+v", skipClose)
	}
	force := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-09-04", LastFinalized: lastFinalized, Now: now,
		HasClose: true, CloseFetchedAt: closeFetched, ForceRecheck: true,
	})
	if force.Action != HistoryFetch || force.Reason != "force_recheck" {
		t.Fatalf("force = %+v", force)
	}
	staleCorrection := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-09-08", LastFinalized: lastFinalized, Now: now,
		HasClose: true, CloseFetchedAt: now.Add(-25 * time.Hour),
	})
	if staleCorrection.Action != HistoryFetch || staleCorrection.Reason != "recent_correction" {
		t.Fatalf("stale last-3 close = %+v", staleCorrection)
	}
	freshCorrection := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-09-08", LastFinalized: lastFinalized, Now: now,
		HasClose: true, CloseFetchedAt: now.Add(-time.Hour),
	})
	if freshCorrection.Action != HistorySkipExistingClose {
		t.Fatalf("fresh last-3 close = %+v", freshCorrection)
	}
	unexpired := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-09-05", LastFinalized: lastFinalized, Now: now,
		HasNoObservation: true, NoObservationHasExpiry: true,
		NoObservationExpiresAt: now.Add(time.Hour), NoObservationCheckedAt: now.Add(-time.Hour),
	})
	if unexpired.Action != HistorySkipNoObservation {
		t.Fatalf("unexpired no-obs = %+v", unexpired)
	}
	expired := DecideHistoryFetch(HistoryFetchInput{
		Date: "2026-08-01", LastFinalized: lastFinalized, Now: now,
		HasNoObservation: true, NoObservationHasExpiry: true,
		NoObservationExpiresAt: now.Add(-time.Minute),
	})
	if expired.Action != HistoryFetch || expired.Reason != "no_observation_expired" {
		t.Fatalf("expired no-obs = %+v", expired)
	}
	gap := DecideHistoryFetch(HistoryFetchInput{Date: "2026-09-07", LastFinalized: lastFinalized, Now: now})
	if gap.Action != HistoryFetch || gap.Reason != "coverage_gap" {
		t.Fatalf("gap = %+v", gap)
	}
}

func TestLatestRequestDueIndependentOfQuoteAge(t *testing.T) {
	sunday := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	checked := sunday.Add(-45 * time.Minute)
	if LatestRequestDue(checked, sunday, 3*time.Hour, false) {
		t.Fatal("successful check 45 minutes ago was treated as due")
	}
	if !LatestRequestDue(checked, sunday.Add(3*time.Hour), 3*time.Hour, false) {
		t.Fatal("expired request TTL was not due")
	}
	if !LatestRequestDue(checked, sunday, 3*time.Hour, true) {
		t.Fatal("force did not bypass request TTL")
	}
	if !LatestRequestDue(time.Time{}, sunday, 3*time.Hour, false) {
		t.Fatal("missing last-successful-check must fetch")
	}
}
