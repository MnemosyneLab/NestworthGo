package domain

import (
	"github.com/shopspring/decimal"
	"testing"
)

func TestLiquidityRetainsValuationPrecisionThroughFeesReservationsAndFX(t *testing.T) {
	for _, raw := range []string{"1.00004", "0.00004", "915.80996703"} {
		t.Run(raw, func(t *testing.T) {
			q, fixture, _ := fixtureA(t)
			q.BaseCurrency = "CNY"
			exact := decimal.RequireFromString(raw)
			projection, err := NewMoney(exact, "USD")
			if err != nil {
				t.Fatal(err)
			}
			var sources []LiquiditySource
			var reservations []LiquidityReservation
			fee := mustTestMoney(t, "0.0001", "USD")
			if exact.LessThan(fee.Amount()) {
				fee = mustTestMoney(t, "0", "USD")
			}
			for range 2 {
				s := fixture[0]
				s.AccountID = NewAccountID()
				s.Ref = AccountCashSourceRef(s.AccountID, "USD")
				s.CurrentNativeAmount, s.CurrentNativeValue = raw, &projection
				policy := *s.ExplicitPolicy
				policy.Source, policy.NormalExitFee = s.Ref, &fee
				s.ExplicitPolicy = &policy
				sources = append(sources, s)
				reservations = append(reservations, LiquidityReservation{ID: NewLiquidityReservationID(), HouseholdID: policy.HouseholdID, Source: s.Ref, Label: "Reserve", Amount: mustTestMoney(t, "0.5", "USD"), Currency: "USD", Revision: 1, CreatedAt: q.AsOf, UpdatedAt: q.AsOf})
			}
			rate := decimal.RequireFromString("7.12345678")
			convert := func(m Money) (*decimal.Decimal, bool, error) { v := m.Amount().Mul(rate); return &v, true, nil }
			got, err := EvaluateLiquidity(q, sources, reservations, convert)
			if err != nil {
				t.Fatal(err)
			}
			net := exact.Sub(fee.Amount())
			total := net.Mul(decimal.NewFromInt(2))
			reserve := decimal.Min(net, decimal.RequireFromString("0.5")).Mul(decimal.NewFromInt(2))
			wantNative, _ := NewMoney(total, "USD")
			wantBase, _ := NewMoney(total.Mul(rate), "CNY")
			wantReserve, _ := NewMoney(reserve.Mul(rate), "CNY")
			for _, b := range got.Buckets {
				if b.FullAvailable == nil || !b.FullAvailable.Amount().Equal(wantBase.Amount()) {
					t.Fatalf("base total = %+v, want %s", b.FullAvailable, wantBase.CanonicalAmount())
				}
				if b.AppliedReserveSubtotal == nil || !b.AppliedReserveSubtotal.Amount().Equal(wantReserve.Amount()) {
					t.Fatalf("reserve = %+v, want %s", b.AppliedReserveSubtotal, wantReserve.CanonicalAmount())
				}
				if len(b.NativeCurrencyGroups) != 1 || b.NativeCurrencyGroups[0].FullAvailable == nil || !b.NativeCurrencyGroups[0].FullAvailable.Amount().Equal(wantNative.Amount()) {
					t.Fatalf("native groups = %+v, want %s", b.NativeCurrencyGroups, wantNative.CanonicalAmount())
				}
			}
		})
	}
}

func TestLiquidityExactValuationRespectsCapAndMissingFX(t *testing.T) {
	q, sources, _ := fixtureA(t)
	s := sources[0]
	s.CurrentNativeAmount = "1.00008"
	s.CurrentNativeValue = moneyPtr(t, "1.0001", "USD")
	s.ExplicitPolicy.AccessibleAmountCap = moneyPtr(t, "1", "USD")
	got, err := EvaluateLiquidity(q, []LiquiditySource{s}, nil, identityFX("USD"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Buckets[0].FullAvailable == nil || got.Buckets[0].FullAvailable.CanonicalAmount() != "1" {
		t.Fatal("exact valuation exceeded policy cap")
	}
	q.BaseCurrency = "CNY"
	s.CurrentNativeAmount = "0.00004"
	s.CurrentNativeValue = moneyPtr(t, "0", "USD")
	for _, convert := range []ConvertToBase{nil, identityFX("CNY")} {
		got, err := EvaluateLiquidity(q, []LiquiditySource{s}, nil, convert)
		if err != nil {
			t.Fatal(err)
		}
		if got.Buckets[0].FullAvailable != nil {
			t.Fatal("positive foreign valuation with missing FX must not become a complete zero")
		}
	}
}

func TestLiquidityUnknownAccessNeedsInformationRatherThanLocked(t *testing.T) {
	query, sources, _ := fixtureA(t)
	source := sources[3]
	source.ExplicitPolicy = nil
	overview, err := EvaluateLiquidity(query, []LiquiditySource{source}, nil, identityFX(query.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if overview.Sources[0].DisplayState != ProductDisplayNeedsInfo {
		t.Fatalf("state = %s", overview.Sources[0].DisplayState)
	}
	if overview.Buckets[0].FullAvailable != nil {
		t.Fatal("unknown access must not become a known amount")
	}
}
