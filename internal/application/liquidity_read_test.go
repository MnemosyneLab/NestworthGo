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
