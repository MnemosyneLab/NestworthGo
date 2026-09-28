package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestLiquidityOverviewAcceptsHighPrecisionHoldingValuations(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "100")
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Precision holding", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.String(), InstrumentID: instrument.ID.String(), Quantity: "3", UnitCost: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "305.26998901", "2026-09-20T04:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	saveExplicitAccessPolicy(t, service, ctx, domain.HoldingSourceRef(account, holding.ID), 0)
	snapshot, err := service.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	before, after, err := service.productPreviewValues(snapshot, productPlan{beforeContracts: []domain.ProductContract{{HoldingID: holding.ID, Currency: "USD"}}})
	if err != nil {
		t.Fatal(err)
	}
	if before == nil || after == nil || before.CanonicalAmount() != "915.81" || after.CanonicalAmount() != "915.81" {
		t.Fatalf("preview valuations = %v, %v", before, after)
	}
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range overview.Sources {
		if source.Ref.Key() == domain.HoldingSourceRef(account, holding.ID).Key() {
			if source.NormalRoute == nil || source.NormalRoute.NetNative == nil || source.NormalRoute.NetNative.CanonicalAmount() != "915.81" {
				t.Fatalf("route = %+v", source.NormalRoute)
			}
			for _, row := range source.BucketResults {
				if row.NetBase == nil || row.NetBase.String() != "915.80996703" {
					t.Fatalf("exact converted valuation = %v", row.NetBase)
				}
			}
			return
		}
	}
	t.Fatal("holding missing from overview")
}
