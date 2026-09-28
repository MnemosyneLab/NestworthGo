package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestLiquidityReviewFutureUnlockUnknownLagKnownZero(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[3]
	p.Contract = nil
	p.Managed = false
	p.ExplicitPolicy.AccessKind = AccessOnDate
	p.ExplicitPolicy.UnlockOn = datePtr("2026-10-20")
	p.ExplicitPolicy.SettlementDays = nil
	p.ExplicitPolicy.DayBasis = nil
	got, e := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	if got.Buckets[0].Status != StatusComplete || got.Buckets[0].FullAvailable == nil || !got.Buckets[0].FullAvailable.IsZero() {
		t.Fatalf("known future lock gives %s / %v today", got.Buckets[0].Status, got.Buckets[0].FullAvailable)
	}
}
func TestLiquidityReviewMaturedUnknownEarlyIgnored(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[1]
	q.IncludeEarlyWithdrawal = true
	p.Contract.MaturityOn = datePtr("2026-09-15")
	p.ExplicitPolicy.UnlockOn = datePtr("2026-09-15")
	p.ExplicitPolicy.EarlyKind = EarlyUnknown
	got, e := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	if got.Sources[0].EarlyRoute != nil || got.Buckets[0].Status != StatusComplete {
		t.Fatalf("matured deposit has early candidate and status %s", got.Buckets[0].Status)
	}
}
func TestLiquidityReviewOverrideWithoutMaturityOverdue(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[2]
	p.Contract.MaturityOn = nil
	p.ExplicitPolicy.AccessKind = AccessOnRequest
	p.ExplicitPolicy.UnlockOn = nil
	p.ExplicitPolicy.ReceiptOnOverride = datePtr("2026-09-19")
	got, e := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	if !got.Sources[0].DueUnconfirmed {
		t.Fatalf("past revised receipt becomes state %s and status %s", got.Sources[0].DisplayState, got.Buckets[0].Status)
	}
}
func TestLiquidityReviewNativeFullValueIndependentOfFX(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[0]
	q.BaseCurrency = "EUR"
	got, e := EvaluateLiquidity(q, []LiquiditySource{p}, nil, func(Money) (*decimal.Decimal, bool, error) { return nil, false, nil })
	if e != nil {
		t.Fatal(e)
	}
	g := got.Buckets[0].NativeCurrencyGroups[0]
	if g.Status != StatusComplete || g.FullAvailable == nil {
		t.Fatalf("native group status=%s full=%v known=%v despite known native route", g.Status, g.FullAvailable, g.KnownAvailableSubtotal)
	}
}
func TestLiquidityReviewZeroCashReservationUnresolved(t *testing.T) {
	q, s, rs := fixtureA(t)
	p := s[0]
	zero, _ := ParseMoney("0", p.NativeCurrency)
	p.CurrentNativeValue = &zero
	var r LiquidityReservation
	for _, v := range rs {
		if v.Source.Key() == p.Ref.Key() {
			r = v
		}
	}
	if r.ID == "" {
		t.Fatal("fixture has no cash reservation")
	}
	got, e := EvaluateLiquidity(q, []LiquiditySource{p}, []LiquidityReservation{r}, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	if len(got.UnresolvedReservations) != 1 {
		t.Fatalf("zero cash reservation not unresolved: %d", len(got.UnresolvedReservations))
	}
}

func TestLiquidityReviewUnknownEarlyCannotBypassKnownLock(t *testing.T) {
	q, sources, _ := fixtureA(t)
	q.IncludeEarlyWithdrawal = true
	p := sources[2]
	p.ExplicitPolicy.UnlockOn = datePtr("2026-10-20")
	p.ExplicitPolicy.EarlyKind = EarlyUnknown
	got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if got.Buckets[0].Status != StatusComplete || got.Buckets[0].FullAvailable == nil || !got.Buckets[0].FullAvailable.IsZero() {
		t.Fatal("unknown early access ignored a known mandatory lock")
	}
}
