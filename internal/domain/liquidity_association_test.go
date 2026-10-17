package domain

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// All identities and amounts are synthetic and stable across runs.
func associationFixture(sourceCount, reservationCount int) (LiquidityQuery, []LiquiditySource, []LiquidityReservation) {
	query := LiquidityQuery{AsOf: time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC), Timezone: "UTC", BaseCurrency: "USD"}
	household, _ := ParseHouseholdID("00000000-0000-4000-8000-000000000001")
	amount, _ := ParseMoney("1", "USD")
	value, _ := ParseMoney("100", "USD")
	sources := make([]LiquiditySource, sourceCount)
	for i := range sources {
		account, _ := ParseAccountID(fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1))
		sources[i] = LiquiditySource{Ref: AccountValueSourceRef(account), AccountID: account, DisplayName: fmt.Sprintf("source %06d", i), NativeCurrency: "USD", CurrentNativeValue: &value, AccountType: TypeBankAccount}
	}
	reservations := make([]LiquidityReservation, reservationCount)
	for i := range reservations {
		// Half missing; matches cycle through S-2, S-4, ..., 0 for even S.
		// Missing keys scan all sources; matches scan about half on average.
		ref := sources[sourceCount-1-i%sourceCount].Ref
		if i%2 == 0 {
			account, _ := ParseAccountID("ffffffff-ffff-4fff-8fff-ffffffffffff")
			ref = AccountValueSourceRef(account)
		}
		reservations[i] = LiquidityReservation{HouseholdID: household, Source: ref, Label: fmt.Sprintf("reserve %d", i), Amount: amount, Currency: "USD", Revision: 1}
	}
	return query, sources, reservations
}

func TestLiquidityReservationAssociationParity(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*LiquidityQuery, *[]LiquiditySource, *[]LiquidityReservation)
	}{
		{"mixed missing and multiple", nil},
		{"archived", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) { (*s)[0].Archived = true }},
		{"liability", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) { (*s)[0].Liability = true }},
		{"quantity zero", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) { (*s)[0].QuantityZero = true }},
		{"unknown", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			(*s)[0].CurrentNativeValue = nil
		}},
		{"zero", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			v, _ := ParseMoney("0", "USD")
			(*s)[0].CurrentNativeValue = &v
		}},

		{"exact positive rounded zero", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			v, _ := ParseMoney("0", "USD")
			(*s)[0].CurrentNativeValue = &v
			(*s)[0].CurrentNativeAmount = "0.000001"
		}},
		{"exact zero overrides positive", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			(*s)[0].CurrentNativeAmount = "0"
		}},

		{"duplicate qualifying first", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			bad := (*s)[0]
			bad.Archived = true
			*s = append(*s, bad)
		}},
		{"duplicate qualifying last", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			bad := (*s)[0]
			bad.QuantityZero = true
			*s = append([]LiquiditySource{bad}, (*s)...)
		}},
		{"released", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) { (*r)[1].ReleasedAt = &q.AsOf }},
		{"invalid released", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			(*r)[1].ReleasedAt = &q.AsOf
			(*r)[1].Label = ""
		}},
		{"invalid active", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) { (*r)[1].Revision = 0 }},
		{"mixed currencies", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			v, _ := ParseMoney("2", "EUR")
			(*s)[0].NativeCurrency = "EUR"
			(*s)[0].CurrentNativeValue = &v
			for i := 1; i < len(*r); i += 2 {
				(*r)[i].Amount = v
				(*r)[i].Currency = "EUR"
			}
		}},
		{"aggregation currency error", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			v, _ := ParseMoney("2", "EUR")
			(*r)[1].Amount = v
			(*r)[1].Currency = "EUR"
		}},
		{"amount validation precedes reservation", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			(*s)[0].CurrentNativeAmount = "invalid"
			(*r)[0].Label = ""
		}},
		{"reservation validation precedes source", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			(*s)[0].Ref.Kind = "invalid"
			(*r)[0].Label = ""
		}},
		{"query validation precedes amount", func(q *LiquidityQuery, s *[]LiquiditySource, r *[]LiquidityReservation) {
			q.Timezone = "invalid"
			(*s)[0].CurrentNativeAmount = "invalid"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, s, r := associationFixture(2, 8)
			if tc.mutate != nil {
				tc.mutate(&q, &s, &r)
			}
			for _, convert := range []ConvertToBase{nil, func(amount decimal.Decimal, currency CurrencyCode) (*decimal.Decimal, bool, error) {
				rate := decimal.NewFromInt(1)
				if currency == "EUR" {
					rate = decimal.NewFromInt(2)
				}
				value := amount.Mul(rate)
				return &value, true, nil
			}} {
				got, err := EvaluateLiquidity(q, s, r, convert)
				want, werr := evaluateLiquidityLegacyAssociation(q, s, r, convert)
				if !reflect.DeepEqual(err, werr) || !reflect.DeepEqual(got, want) {
					t.Fatalf("complete evaluator mismatch\nerror: %v / %v\ngot: %#v\nwant: %#v", err, werr, got, want)
				}
			}
		})
	}
}

// Negative amounts fail domain parsing before reservation association.
func TestLiquidityReservationAssociationRejectsNegativeNativeAmount(t *testing.T) {
	if _, err := ParseMoney("-1", "USD"); err == nil {
		t.Fatal("negative Money must be rejected")
	}
	_, expectedErr := ParseNativeAmount("-0.000001")
	if expectedErr == nil {
		t.Fatal("negative native amount must be rejected")
	}
	q, sources, reservations := associationFixture(2, 8)
	sources[0].CurrentNativeAmount = "-0.000001"
	got, err := EvaluateLiquidity(q, sources, reservations, nil)
	want, legacyErr := evaluateLiquidityLegacyAssociation(q, sources, reservations, nil)
	if !reflect.DeepEqual(err, expectedErr) || !reflect.DeepEqual(legacyErr, expectedErr) ||
		!reflect.DeepEqual(got, LiquidityOverview{}) || !reflect.DeepEqual(got, want) {
		t.Fatalf("expected native parser error and empty overview: got %v, legacy %v, expected %v", err, legacyErr, expectedErr)
	}
}

func BenchmarkLiquidityReservationAssociation(b *testing.B) {
	for _, size := range [][2]int{{10, 20}, {100, 200}, {1000, 2000}} {
		q, s, r := associationFixture(size[0], size[1])
		for _, legacy := range []bool{true, false} {
			name := "indexed"
			evaluator := EvaluateLiquidity
			if legacy {
				name = "legacy"
				evaluator = evaluateLiquidityLegacyAssociation
			}
			b.Run(fmt.Sprintf("S%d_R%d/%s", size[0], size[1], name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := evaluator(q, s, r, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
