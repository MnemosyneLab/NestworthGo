package application

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMobileLocalTimePreservesSecondsAndRejectsDSTGuessing(t *testing.T) {
	at, err := mobileLocalTime("2026-10-30", "12:30:42", "Asia/Shanghai")
	if err != nil || at.UTC().Format(time.RFC3339) != "2026-10-30T04:30:42Z" {
		t.Fatalf("seconds: %v %s", err, at)
	}
	for _, clock := range []string{"02:30", "02:30:15"} {
		if _, err := mobileLocalTime("2026-03-08", clock, "America/New_York"); err == nil {
			t.Fatal("nonexistent DST time accepted")
		}
	}
	if _, err := mobileLocalTime("2026-11-01", "01:30:15", "America/New_York"); err == nil {
		t.Fatal("ambiguous DST time guessed")
	}
}

func TestMobileDailyRecordsKeepSemanticsAndDurableReceipts(t *testing.T) {
	s, ctx, b, setClock := newOnboardedService(t, "mobile-synthetic", []string{"Owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	create := func(name, kind, role, mode, value string) domain.AccountID {
		t.Helper()
		a, err := s.CreateAccount(ctx, AccountInput{Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: "CNY", InitialAmount: value, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return a.Account.ID
	}
	cash := create("Cash", "bank_account", "asset", "balance", "1000")
	card := create("Card", "credit_card", "liability", "balance", "100")
	other := create("Other", "bank_account", "asset", "balance", "0")
	broker := create("Broker", "brokerage", "asset", "holdings", "")
	if _, err := s.AppendAccountCashValue(ctx, broker, "10", "CNY", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	setClock(now.Add(time.Minute))
	cases := []struct {
		kind                  string
		account, other        domain.AccountID
		amount, received, fee string
	}{
		{"income", cash, "", "10.1234", "", ""}, {"expense", cash, "", "1.0001", "", ""}, {"transfer", cash, other, "20", "20", "1"}, {"card_purchase", card, "", "5", "", ""}, {"card_refund", card, "", "2", "", ""}, {"repayment", card, cash, "10", "", "1"}, {"balance", cash, "", "900", "", ""}, {"balance", broker, "", "12", "", ""},
	}
	for _, c := range cases {
		t.Run(c.kind+"-"+c.account.String(), func(t *testing.T) {
			id := uuid.NewString()
			p, err := s.PreviewMobileRecord(ctx, MobileRecordInput{OperationID: id, Kind: c.kind, AccountID: c.account.String(), OtherAccountID: c.other.String(), Amount: c.amount, Currency: "CNY", Received: c.received, ReceivedCurrency: "CNY", Fee: c.fee})
			if err != nil {
				t.Fatal(err)
			}
			first, err := s.ConfirmMobileRecord(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			again, err := s.ConfirmMobileRecord(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if again.Activity.ID != first.Activity.ID {
				t.Fatal("duplicate activity")
			}
			recovered, err := s.LookupMobileRecord(ctx, id)
			if err != nil || recovered == nil || recovered.Activity.ID != first.Activity.ID {
				t.Fatalf("receipt: %v %+v", err, recovered)
			}
			if c.kind == "card_purchase" && first.Effects[0].Classification != domain.ClassificationExternalOutflow {
				t.Fatalf("card classification %+v", first.Effects)
			}
			if c.kind == "balance" {
				for _, effect := range first.Effects {
					if effect.Classification == domain.ClassificationIncome {
						t.Fatal("calibration became income")
					}
				}
			}
		})
	}
	accounts, err := s.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts {
		if a.Account.ID == card && a.LatestValue.Amount.CanonicalAmount() != "93" {
			t.Fatalf("card debt=%s", a.LatestValue.Amount.CanonicalAmount())
		}
	}
}

func TestMobilePreviewCASAndConcurrentReplay(t *testing.T) {
	s, ctx, b, setClock := newOnboardedService(t, "mobile-cas", []string{"Owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setClock(now.Add(time.Minute))
	in := MobileRecordInput{OperationID: uuid.NewString(), Kind: "expense", AccountID: a.Account.ID.String(), Amount: "1", Currency: "CNY"}
	p, err := s.PreviewMobileRecord(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	ids := make(chan domain.ActivityID, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.ConfirmMobileRecord(ctx, p); errs <- e; ids <- r.Activity.ID }()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first domain.ActivityID
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate")
		}
	}
	in.OperationID = uuid.NewString()
	stale, err := s.PreviewMobileRecord(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateMember(ctx, "Concurrent desktop write"); err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfirmMobileRecord(ctx, stale)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.ErrStalePreview || de.Field != "previewToken" {
		t.Fatalf("stale = %v", err)
	}
	if r, err := s.LookupMobileRecord(ctx, in.OperationID); err != nil || r != nil {
		t.Fatalf("stale committed: %v %+v", err, r)
	}
	in.Kind = "trade"
	if _, err = s.PreviewMobileRecord(ctx, in); err == nil {
		t.Fatal("generic trade admitted")
	}
}

func TestMobileManualQuoteAtomicReceiptAndSourceFence(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "mobile-quotes", []string{"Owner"})
	setClock(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	input := MobileQuoteInput{OperationID: uuid.NewString(), InstrumentID: i.ID.String(), Value: "1.23456789", QuotedAt: "2026-08-23"}
	p, err := s.PreviewMobileQuote(ctx, input, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.ConfirmMobileQuote(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.ConfirmMobileQuote(ctx, p)
	if err != nil || one.FactID != two.FactID {
		t.Fatalf("replay %v %+v", err, two)
	}
	fresh := NewService(s.repository)
	recovered, err := fresh.LookupMobileQuote(ctx, input.OperationID)
	if err != nil || recovered == nil || recovered.Value != "1.23456789" {
		t.Fatalf("restart receipt %v %+v", err, recovered)
	}
	input.Value = "2"
	changed, err := s.PreviewMobileQuote(ctx, input, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmMobileQuote(ctx, changed); err == nil {
		t.Fatal("operation ID changed payload")
	}
	if _, err = s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	fx, err := s.PreviewMobileQuote(ctx, MobileQuoteInput{OperationID: uuid.NewString(), Currency: "USD", QuoteCurrency: "CNY", Value: "7.12345678", QuotedAt: "2026-08-23"}, "synthetic-device")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConfirmMobileQuote(ctx, fx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewMobileQuote(ctx, MobileQuoteInput{OperationID: uuid.NewString(), Currency: "EUR", QuoteCurrency: "CNY", Value: "8", QuotedAt: "2026-08-23"}, "synthetic-device"); err == nil {
		t.Fatal("source selection silently changed")
	}
}
