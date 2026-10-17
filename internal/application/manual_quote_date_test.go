package application

import (
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestManualQuoteDatesUseHistoryTimezone(t *testing.T) {
	t.Parallel()
	for _, zone := range []string{"Pacific/Kiritimati", "America/Los_Angeles", "Asia/Singapore"} {
		t.Run(zone, func(t *testing.T) {
			service, ctx, _, setClock := newOnboardedService(t, "manual-date", nil)
			setClock(time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC))
			instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Manual", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.StartHistory(ctx, zone); err != nil {
				t.Fatal(err)
			}
			location, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			for _, date := range []string{"2026-03-08", "2026-11-01"} {
				want, err := time.ParseInLocation("2006-01-02", date, location)
				if err != nil {
					t.Fatal(err)
				}
				quote, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "12", date, false)
				if err != nil || !quote.QuotedAt.Equal(want) {
					t.Fatalf("instrument: got %v want %v err %v", quote.QuotedAt, want, err)
				}
				fx, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", date)
				if err != nil || !fx.QuotedAt.Equal(want) {
					t.Fatalf("FX: got %v want %v err %v", fx.QuotedAt, want, err)
				}
			}
			explicit := "2026-09-01T12:00:00+08:00"
			want, _ := time.Parse(time.RFC3339, explicit)
			got, err := service.parseManualQuoteTimestamp(ctx, explicit)
			if err != nil || !got.Equal(want) {
				t.Fatalf("explicit instant changed: %v %v", got, err)
			}
			if _, err := service.parseManualQuoteTimestamp(ctx, "2026-02-30"); err == nil {
				t.Fatal("accepted invalid date")
			}
		})
	}
}

func TestManualFutureQuotesCannotEnterCurrentValuation(t *testing.T) {
	t.Parallel()
	service, ctx, bootstrap, setClock := newOnboardedService(t, "manual-future-date", nil)
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
	setClock(now)
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Synthetic Holdings", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic USD Instrument", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-10-17T11:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-10-17T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	before, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil || before.Assets.String() != "70" {
		t.Fatalf("starting current valuation = %+v err=%v", before, err)
	}
	setClock(now)
	for _, appendQuote := range []struct {
		name string
		err  error
	}{
		{"instrument date-only", func() error {
			_, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "99", "2026-10-18", false)
			return err
		}()},
		{"instrument timestamp", func() error {
			_, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "99", "2026-10-17T12:00:01Z", false)
			return err
		}()},
		{"FX date-only", func() error { _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "99", "2026-10-18"); return err }()},
		{"FX timestamp", func() error {
			_, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "99", "2026-10-17T12:00:01Z")
			return err
		}()},
	} {
		var problem *domain.Error
		if !errors.As(appendQuote.err, &problem) || problem.Code != domain.ErrInvalidChangeTime {
			t.Errorf("%s error = %v, want ErrInvalidChangeTime", appendQuote.name, appendQuote.err)
		}
	}
	after, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil || after.Assets.String() != "70" {
		t.Fatalf("future quote changed current valuation: %+v err=%v", after, err)
	}
	instrumentQuotes, err := service.InstrumentQuoteHistory(ctx, instrument.ID)
	if err != nil || len(instrumentQuotes) != 1 {
		t.Fatalf("instrument quotes after rejected future writes = %d err=%v", len(instrumentQuotes), err)
	}
	fxQuotes, err := service.FXQuoteHistory(ctx)
	if err != nil || len(fxQuotes) != 1 {
		t.Fatalf("FX quotes after rejected future writes = %d err=%v", len(fxQuotes), err)
	}
}
