package application

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestLiquidityReadSortMatrixExactDecimalAndNullLast(t *testing.T) {
	a, b := decimal.RequireFromString("9007199254740992.00000000000000000001"), decimal.RequireFromString("9007199254740992.00000000000000000002")
	kind := "mutual_fund"
	receipt := "2026-11-01"
	rows := []LiquidityReadSource{{Result: domain.LiquiditySourceResult{SourceKey: "b", DisplayName: " Same ", PolicyOrigin: domain.PolicyOriginExplicit, Policy: &domain.LiquidityPolicy{AccessKind: domain.AccessOnRequest}, NormalRoute: &domain.LiquidityRoute{ReceiptOn: &receipt}, BucketResults: []domain.LiquidityBucketResult{{HorizonOn: receipt, NetBase: &b}}}, InstrumentType: &kind, CurrentValueBase: &b}, {Result: domain.LiquiditySourceResult{SourceKey: "a", DisplayName: "same", PolicyOrigin: domain.PolicyOriginExplicit, Policy: &domain.LiquidityPolicy{AccessKind: domain.AccessOnRequest}, NormalRoute: &domain.LiquidityRoute{ReceiptOn: &receipt}, BucketResults: []domain.LiquidityBucketResult{{HorizonOn: receipt, NetBase: &a}}}, InstrumentType: &kind, CurrentValueBase: &a}, {Result: domain.LiquiditySourceResult{SourceKey: "z"}}}
	for _, field := range []string{"displayName", "instrumentType", "accessKind", "policyOrigin", "normalReceiptOn", "currentValueBase", "availableBase"} {
		for _, direction := range []string{"asc", "desc"} {
			copy := append([]LiquidityReadSource{}, rows...)
			SortLiquidityReadSources(copy, field, direction, receipt)
			if copy[2].Result.SourceKey != "z" {
				t.Fatalf("%s %s null first", field, direction)
			}
			expected := "a"
			if (field == "currentValueBase" || field == "availableBase") && direction == "desc" {
				expected = "b"
			}
			if copy[0].Result.SourceKey != expected {
				t.Fatalf("%s %s first=%s", field, direction, copy[0].Result.SourceKey)
			}
		}
	}
}

func TestLiquidityNativeSourceAmountsKeepsExactSubtotalsAndUnknownGroups(t *testing.T) {
	const day = "2026-11-26"
	source := func(kind domain.LiquiditySourceKind, currency domain.CurrencyCode, amount string, excluded bool) LiquidityReadSource {
		t.Helper()
		result := domain.LiquidityBucketResult{HorizonOn: day, Status: domain.StatusComplete}
		if amount != "" {
			value, err := domain.ParseMoney(amount, currency)
			if err != nil {
				t.Fatal(err)
			}
			result.NetNative = &value
			result.SelectedRoute = &domain.LiquidityRoute{Kind: domain.RouteNormal}
		} else {
			result.Status = domain.StatusUnavailable
		}
		return LiquidityReadSource{Result: domain.LiquiditySourceResult{
			Ref: domain.LiquiditySourceRef{Kind: kind}, NativeCurrency: currency,
			Excluded: excluded, BucketResults: []domain.LiquidityBucketResult{result},
		}}
	}
	read := LiquidityReadResult{Sources: []LiquidityReadSource{
		source(domain.SourceAccountValue, "USD", "12.3456", false),
		source(domain.SourceAccountCash, "USD", "0.0005", false),
		source(domain.SourceHolding, "USD", "2.0001", false),
		source(domain.SourceHolding, "USD", "", true),
		source(domain.SourceAccountCash, "EUR", "", false),
	}}
	got := read.NativeSourceAmounts(day)
	if group := got["USD"]; group.Cash == nil || group.Proceeds == nil || group.Cash.String() != "12.3461" || group.Proceeds.String() != "2.0001" {
		t.Fatalf("USD = %+v", group)
	}
	if group := got["EUR"]; group.Cash != nil || group.Proceeds == nil || !group.Proceeds.IsZero() {
		t.Fatalf("EUR = %+v, want unknown cash and known zero proceeds", group)
	}
	if group := read.NativeSourceAmounts("2026-11-27")["USD"]; group.Cash != nil || group.Proceeds != nil {
		t.Fatalf("missing horizon supplied amounts: %+v", group)
	}
}
