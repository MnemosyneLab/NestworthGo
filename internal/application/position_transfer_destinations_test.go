package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestPositionTransferRequiresExactlyOneDestination(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "position-transfer-destinations", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	fromAccount := createHoldingsAccount(t, service, ctx, owner, "From")
	toAccount := createHoldingsAccount(t, service, ctx, owner, "To")
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	from, err := service.CreateHolding(ctx, HoldingInput{AccountID: fromAccount.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := service.CreateHolding(ctx, HoldingInput{AccountID: toAccount.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)
	setClock(now)
	base := domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: from.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: now}
	byBoth := base
	byBoth.ToHoldingID, byBoth.ToAccountID = to.ID, &toAccount.Account.ID
	if _, err := service.PreviewChange(ctx, byBoth); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("both destinations = %v, want validation", err)
	}
	if _, err := service.RecordChange(ctx, byBoth); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("commit with both destinations = %v, want validation", err)
	}
	if _, err := service.PreviewChange(ctx, base); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("no destination = %v, want validation", err)
	}
	byHolding := base
	byHolding.ToHoldingID = to.ID
	if _, err := service.PreviewChange(ctx, byHolding); err != nil {
		t.Fatalf("holding destination: %v", err)
	}
	byAccount := base
	byAccount.ToAccountID = &toAccount.Account.ID
	if _, err := service.PreviewChange(ctx, byAccount); err != nil {
		t.Fatalf("account destination: %v", err)
	}
}
