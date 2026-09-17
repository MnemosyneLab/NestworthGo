package application

import (
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestInstrumentCurrencyImmutablePreservesHistoricalCost(t *testing.T) {
	s, ctx, b, tick := newOnboardedService(t, "currency-immutable", []string{"Owner"})
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10", UnitCost: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "120", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	tick(time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC))
	if _, err = s.AppendAccountCashValue(ctx, a.Account.ID, "1000", "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(ctx, domain.TradeInput{HouseholdID: b.Household.ID, Side: domain.TradeBuy, SettlementAccountID: a.Account.ID, HoldingID: h.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "100", "USD"), EffectiveAt: s.clock()}); err != nil {
		t.Fatal(err)
	}
	before, err := s.HoldingGain(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, replace := range []bool{false, true} {
		_, err = s.UpdateInstrument(ctx, i.ID, InstrumentInput{Replace: replace, Name: "Must not persist", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		var validation *domain.Error
		if !errors.As(err, &validation) || validation.Code != domain.ErrValidation || validation.Field != "quoteCurrency" {
			t.Fatalf("replace=%v: expected currency validation, got %v", replace, err)
		}
		persisted, err := s.repository.Instrument(ctx, b.Household.ID, i.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Name != i.Name || persisted.QuoteCurrency != i.QuoteCurrency {
			t.Fatalf("rejected edit wrote instrument: %+v", persisted)
		}
	}
	after, err := s.HoldingGain(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.CurrentValue == nil || after.CurrentValue == nil {
		t.Fatal("expected priced holding")
	}
	if after.TotalCost.Currency != before.TotalCost.Currency || after.TotalCost.Amount != before.TotalCost.Amount || after.CurrentValue.Currency != before.CurrentValue.Currency || after.CurrentValue.Amount != before.CurrentValue.Amount {
		t.Fatalf("rejected edit changed gain: before=%+v after=%+v", before, after)
	}
	if _, err = s.UpdateInstrument(ctx, i.ID, InstrumentInput{Name: "Renamed", QuoteCurrency: "USD"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateInstrument(ctx, i.ID, InstrumentInput{Name: "Partial rename"}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountSettingsSavePreservesRequestedIcon(t *testing.T) {
	s, ctx, b, _ := newOnboardedService(t, "account-icon", []string{"Owner"})
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{b.Members[0].ID}, IconKey: "bank"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []AccountInput{{IconKey: "wallet", IconKeySet: true}, {Name: "Renamed"}} {
		updated, err := s.UpdateAccount(ctx, a.Account.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Account.IconKey == nil || *updated.Account.IconKey != "wallet" {
			t.Fatalf("icon not saved/preserved: %+v", updated.Account)
		}
		persisted, err := s.accountRecord(ctx, a.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Account.IconKey == nil || *persisted.Account.IconKey != "wallet" {
			t.Fatalf("persisted icon = %+v", persisted.Account.IconKey)
		}
	}
	cleared, err := s.UpdateAccount(ctx, a.Account.ID, AccountInput{IconKeySet: true})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Account.IconKey != nil {
		t.Fatalf("icon not cleared: %v", *cleared.Account.IconKey)
	}
	renamed, err := s.UpdateAccount(ctx, a.Account.ID, AccountInput{Name: "After clearing"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Account.IconKey != nil {
		t.Fatalf("name-only edit restored a cleared icon: %v", *renamed.Account.IconKey)
	}
	persisted, err := s.accountRecord(ctx, a.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Account.IconKey != nil {
		t.Fatalf("cleared icon was not preserved in storage: %v", *persisted.Account.IconKey)
	}
}

func TestSnapshotHealthAndPreviewOnlyIncludeClosedDays(t *testing.T) {
	ptr := func(v string) *string { return &v }
	plan := HistoryRepairPlan{OriginLocalDate: "2026-09-14", YesterdayLocal: "2026-09-16"}
	for _, tc := range []struct {
		name     string
		state    domain.DailySnapshotState
		days     int
		from, to string
	}{
		{"yesterday", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-16")}, 1, "2026-09-16", "2026-09-16"},
		{"today", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-17"), LastCompletedClosedOn: ptr("2026-09-16")}, 0, "", ""},
		{"future", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-18")}, 0, "", ""},
		{"clamped", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-13"), DirtyTo: ptr("2026-09-20")}, 3, "2026-09-14", "2026-09-16"},
		{"bounded", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-14"), DirtyTo: ptr("2026-09-15")}, 2, "2026-09-14", "2026-09-15"},
		{"reversed", domain.DailySnapshotState{DirtyFrom: ptr("2026-09-16"), DirtyTo: ptr("2026-09-15")}, 0, "", ""},
		{"missing", domain.DailySnapshotState{}, 3, "2026-09-14", "2026-09-16"},
		{"complete", domain.DailySnapshotState{LastCompletedClosedOn: ptr("2026-09-16")}, 0, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if days := estimateDirtyDays(tc.state, plan); days != tc.days {
				t.Fatalf("preview days=%d want=%d", days, tc.days)
			}
			issues := scanSnapshotHealth(tc.state, plan, false)
			if tc.days == 0 {
				if len(issues) != 0 {
					t.Fatalf("unexpected repair: %+v", issues)
				}
				return
			}
			if len(issues) != 1 || issues[0].RangeStart != tc.from || issues[0].RangeEnd != tc.to || !issues[0].Executable {
				t.Fatalf("health range: %+v", issues)
			}
			blocked := scanSnapshotHealth(tc.state, plan, true)
			if blocked[0].Executable || !blocked[0].Collapsed {
				t.Fatalf("root cause not respected: %+v", blocked)
			}
		})
	}
	for _, origin := range []string{"", "2026-09-17", "2026-09-18"} {
		p := HistoryRepairPlan{OriginLocalDate: origin, YesterdayLocal: "2026-09-16"}
		if len(scanSnapshotHealth(domain.DailySnapshotState{}, p, false)) != 0 || estimateDirtyDays(domain.DailySnapshotState{}, p) != 0 {
			t.Fatalf("invalid missing range for origin %q", origin)
		}
	}
}
