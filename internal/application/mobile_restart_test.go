package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestMobileReceiptsSurviveVerifiedSQLiteReopen(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	ctx := t.Context()
	account := overviewAccount(t, s, owner, "Synthetic restart cash", "bank_account", "asset", "balance", "CNY", "100")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic restart NAV", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	recordID := uuid.NewString()
	plan, err := s.PreviewMobileRecord(ctx, MobileRecordInput{OperationID: recordID, Kind: "expense", AccountID: account.Account.ID.String(), Amount: "1.2345", Currency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.ConfirmMobileRecord(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	quoteID := uuid.NewString()
	quotePlan, err := s.PreviewMobileQuote(ctx, MobileQuoteInput{OperationID: quoteID, InstrumentID: instrument.ID.String(), Value: "1.23456789", QuotedAt: "2026-08-01"}, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := s.ConfirmMobileQuote(ctx, quotePlan)
	if err != nil {
		t.Fatal(err)
	}
	path := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("verified reopen: %v", err)
	}
	defer reopened.Close()
	fresh := NewService(sqlite.NewRepository(reopened))
	got, err := fresh.LookupMobileRecord(ctx, recordID)
	if err != nil || got == nil || got.Activity.ID != record.Activity.ID {
		t.Fatalf("ledger receipt after reopen %v %+v", err, got)
	}
	gotQuote, err := fresh.LookupMobileQuote(ctx, quoteID)
	if err != nil || gotQuote == nil || gotQuote.FactID != quote.FactID || gotQuote.Value != "1.23456789" {
		t.Fatalf("quote receipt after reopen %v %+v", err, gotQuote)
	}
	if _, err := reopened.SQL.Exec(`UPDATE app_configuration SET value=json_set(value,'$.value','9') WHERE key LIKE 'mobile.quote-receipt.v1.%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.LookupMobileQuote(ctx, quoteID); err == nil {
		t.Fatal("inconsistent financial receipt was accepted")
	}
}
