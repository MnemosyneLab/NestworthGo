package domain

import "testing"

func TestLiquidityRegressionRequestProductIsRedeemable(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[2]
	p.Contract.MaturityOn = nil
	p.ExplicitPolicy.AccessKind = AccessOnRequest
	p.ExplicitPolicy.UnlockOn = nil
	got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources[0].DueUnconfirmed || got.Buckets[0].FullAvailable == nil || got.Buckets[0].FullAvailable.CanonicalAmount() != "4990" {
		t.Fatalf("request product due=%v available=%v; want redeemable / 4990", got.Sources[0].DueUnconfirmed, got.Buckets[0].FullAvailable)
	}
}

func TestLiquidityRegressionUnknownOnlyHasNoKnownZero(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[3]
	p.ExplicitPolicy.AccessKind = AccessUnknown
	got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	b := got.Buckets[0]
	if b.Status != StatusUnavailable || b.KnownAvailableSubtotal != nil {
		t.Fatalf("status=%s known=%v; want unavailable / nil", b.Status, b.KnownAvailableSubtotal)
	}
}

func TestLiquidityRegressionUnknownEarlyMustAffectToday(t *testing.T) {
	q, s, _ := fixtureA(t)
	q.IncludeEarlyWithdrawal = true
	p := s[1]
	p.ExplicitPolicy.EarlyKind = EarlyUnknown
	got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	b := got.Buckets[0]
	if b.Status == StatusComplete || b.FullAvailable != nil {
		t.Fatalf("status=%s full=%v row status=%s; unknown early route cannot yield complete zero", b.Status, b.FullAvailable, got.Sources[0].BucketResults[0].Status)
	}
}

func TestLiquidityRegressionScheduledReceiptDoesNotRollForward(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[1]
	p.Contract.MaturityOn = datePtr("2026-09-15")
	p.ExplicitPolicy.UnlockOn = datePtr("2026-09-15")
	p.ExplicitPolicy.SettlementDays = intPtr(2)
	got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Sources[0].DueUnconfirmed {
		t.Fatalf("due=%v receipt=%s; expected unconfirmed scheduled receipt on Sep 17", got.Sources[0].DueUnconfirmed, *got.Sources[0].NormalRoute.ReceiptOn)
	}
}

func TestLiquidityRegressionScheduledReceiptOverrideAndDetail(t *testing.T) {
	q, s, _ := fixtureA(t)
	p := s[1]
	p.Contract.MaturityOn = datePtr("2026-09-15")
	p.ExplicitPolicy.UnlockOn = datePtr("2026-09-15")
	for _, test := range []struct {
		receipt string
		due     bool
		amount  string
	}{{"2026-09-18", true, "0"}, {"2026-09-20", true, "0"}, {"2026-09-23", false, "20200"}} {
		t.Run(test.receipt, func(t *testing.T) {
			p.ExplicitPolicy.ReceiptOnOverride = datePtr(test.receipt)
			got, err := EvaluateLiquidity(q, []LiquiditySource{p}, nil, identityFX(q.BaseCurrency))
			if err != nil {
				t.Fatal(err)
			}
			if got.Sources[0].DueUnconfirmed != test.due {
				t.Fatalf("due=%v", got.Sources[0].DueUnconfirmed)
			}
			if b := got.Buckets[1]; b.FullAvailable == nil || b.FullAvailable.CanonicalAmount() != test.amount {
				t.Fatalf("available=%v want %s", b.FullAvailable, test.amount)
			}
			state, err := ProductAvailabilityState(*p.Contract, *p.ExplicitPolicy, got.LocalDate)
			if err != nil {
				t.Fatal(err)
			}
			if state != got.Sources[0].DisplayState {
				t.Fatalf("detail=%s overview=%s", state, got.Sources[0].DisplayState)
			}
		})
	}
}

func TestLiquidityRegressionKnownZeroAndUnknownAlternative(t *testing.T) {
	q, s, _ := fixtureA(t)
	q.IncludeEarlyWithdrawal = true
	knownZero := s[0]
	knownZero.CurrentNativeValue = moneyPtr(t, "0", "USD")
	unknown := s[1]
	unknown.ExplicitPolicy.EarlyKind = EarlyUnknown
	for _, convert := range []ConvertToBase{nil, identityFX(q.BaseCurrency)} {
		got, err := EvaluateLiquidity(q, []LiquiditySource{knownZero, unknown}, nil, convert)
		if err != nil {
			t.Fatal(err)
		}
		b := got.Buckets[0]
		if b.Status != StatusPartial || b.FullAvailable != nil || b.KnownAvailableSubtotal == nil || !b.KnownAvailableSubtotal.IsZero() {
			t.Fatalf("known zero plus unknown = %+v", b)
		}
		got, err = EvaluateLiquidity(q, []LiquiditySource{knownZero}, nil, convert)
		if err != nil {
			t.Fatal(err)
		}
		assertBucket(t, got.Buckets[0], "0", "0", "0")
	}
}
