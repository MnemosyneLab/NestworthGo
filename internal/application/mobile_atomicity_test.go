package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMobileQuoteReceiptFailureRollsBackObservation(t *testing.T) {
	s, db, _, _ := overviewFixture(t)
	ctx := t.Context()
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic atomic fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	p, err := s.PreviewMobileQuote(ctx, MobileQuoteInput{OperationID: id, InstrumentID: instrument.ID.String(), Value: "1.12345678", QuotedAt: "2026-08-01"}, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	// Prove these exact quote facts can commit before injecting receipt failure.
	if _, err := s.ConfirmMobileQuote(ctx, p); err != nil {
		t.Fatal(err)
	}
	id = uuid.NewString()
	p, err = s.PreviewMobileQuote(ctx, MobileQuoteInput{OperationID: id, InstrumentID: instrument.ID.String(), Value: "1.12345678", QuotedAt: "2026-08-01"}, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`CREATE TEMP TRIGGER reject_mobile_receipt BEFORE INSERT ON app_configuration WHEN substr(NEW.key,1,length('mobile.quote-receipt.v1.'))='mobile.quote-receipt.v1.' BEGIN SELECT RAISE(ABORT,'synthetic receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmMobileQuote(ctx, p); err == nil {
		t.Fatal("receipt failure was ignored")
	}
	var quotes, receipts int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM instrument_quotes WHERE instrument_id=?`, instrument.ID.String()).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRow(`SELECT count(*) FROM app_configuration WHERE key LIKE 'mobile.quote-receipt.v1.%'`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if quotes != 1 || receipts != 1 {
		t.Fatalf("atomic rollback failed quotes=%d receipts=%d", quotes, receipts)
	}
}

func TestMobileManualHealthFixesLeaveDerivedIssuesVisible(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	ctx := t.Context()
	account := overviewAccount(t, s, owner, "Synthetic funds", "brokerage", "asset", "holdings", "CNY", "")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic NAV", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "1", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	overviewAccount(t, s, owner, "Synthetic USD", "bank_account", "asset", "balance", "USD", "100")
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if _, err := db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=?`, instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.BuildDailyValuationSnapshot(ctx, "2026-08-02"); err != nil {
		t.Fatal(err)
	}
	before, err := s.ScanMarketDataHealth(ctx)
	if err != nil || !hasKind(before, HealthKindMissingManualPrice) || !hasKind(before, HealthKindMissingManualFX) {
		t.Fatalf("fixture health %v %+v", err, before)
	}
	for _, in := range []MobileQuoteInput{{OperationID: uuid.NewString(), InstrumentID: instrument.ID.String(), Value: "1.25", QuotedAt: "2026-08-01"}, {OperationID: uuid.NewString(), Currency: "USD", QuoteCurrency: "CNY", Value: "7.12345678", QuotedAt: "2026-08-01"}} {
		p, err := s.PreviewMobileQuote(ctx, in, "synthetic-device")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConfirmMobileQuote(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.ScanMarketDataHealth(ctx)
	if err != nil || hasKind(after, HealthKindMissingManualPrice) || hasKind(after, HealthKindMissingManualFX) || after.Healthy || !hasKind(after, HealthKindSnapshotOutdated) {
		t.Fatalf("health falsely cleared %v %+v", err, after)
	}
}
