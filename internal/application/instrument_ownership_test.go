package application

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"reflect"
	"testing"
	"time"
)

func TestInstrumentOwnershipIncludesClosedProductsAndPreservesOrdinarySameType(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	ordinary, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Ordinary manual NAV", Type: "bank_investment_product", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	preview, err := service.PreviewProductOperation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	assert := func() {
		t.Helper()
		owners, err := service.InstrumentProductOwnership(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if owners[detail.Contract.InstrumentID.String()] != detail.Contract.ID.String() || owners[ordinary.ID.String()] != "" {
			t.Fatalf("ownership %+v", owners)
		}
		bootstrap, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		quantity, _ := domain.ParseQuantity("1")
		gross, _ := domain.ParseMoney("1000", "USD")
		when := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
		buy := domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account, InstrumentID: detail.Contract.InstrumentID, Quantity: quantity, Gross: gross, EffectiveAt: when}
		heldBuy := buy
		heldBuy.HoldingID = detail.Contract.HoldingID
		sell := heldBuy
		sell.Side = domain.TradeSell
		commands := []any{buy, heldBuy, sell,
			domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: detail.Contract.HoldingID, Amount: gross, EffectiveAt: when},
			domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: detail.Contract.HoldingID, Quantity: quantity, EffectiveAt: when},
			domain.PositionAdjustmentInput{HouseholdID: bootstrap.Household.ID, HoldingID: detail.Contract.HoldingID, Quantity: quantity, Added: true, EffectiveAt: when},
			domain.PositionImportInput{HouseholdID: bootstrap.Household.ID, AccountID: account, InstrumentID: detail.Contract.InstrumentID, Quantity: quantity, Currency: "USD", EffectiveAt: when},
		}
		before, err := service.repository.ReadGainSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		refused := func(err error) {
			t.Helper()
			var domainErr *domain.Error
			if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrManagedPosition {
				t.Fatalf("managed guard: %v", err)
			}
		}
		for _, command := range commands {
			_, err := service.PreviewChange(ctx, command)
			refused(err)
			_, err = service.RecordChange(ctx, command)
			refused(err)
		}
		_, err = service.CreateHolding(ctx, HoldingInput{AccountID: account.String(), InstrumentID: detail.Contract.InstrumentID.String(), Quantity: "1"})
		refused(err)
		_, err = service.UpdateInstrument(ctx, detail.Contract.InstrumentID, InstrumentInput{Name: "Forbidden generic edit"})
		refused(err)
		refused(service.ArchiveInstrument(ctx, detail.Contract.InstrumentID, true))
		_, err = service.AppendManualInstrumentQuote(ctx, detail.Contract.InstrumentID, "999", when.Format(time.RFC3339), false)
		refused(err)
		after, err := service.repository.ReadGainSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatal("rejected generic commands changed financial facts")
		}
		instruments, err := service.ListInstruments(ctx, true)
		if err != nil || len(instruments) != 2 {
			t.Fatalf("identity inventory %v %v", instruments, err)
		}
	}
	assert()
	settle := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: detail.Contract.ID.String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("0"), Fee: strPtr("0"), EffectiveAt: "2026-09-20T04:00:00Z"}}
	preview, err = service.PreviewProductOperation(ctx, settle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RecordProductOperation(ctx, settle, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	assert()
}
