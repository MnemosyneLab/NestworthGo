package testledger

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func HistoryStart() time.Time {
	return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
}

func seedHistory(ctx context.Context, service *application.Service, bankID domain.InstitutionID) error {
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		return err
	}
	account, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Harbor Checking", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "250",
		IncludeInNetWorth: true, IncludeInLiquidAssets: true, InstitutionID: bankID.String(),
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		return err
	}
	if _, err := service.CaptureSyntheticOrigin(ctx, "Asia/Singapore"); err != nil {
		return err
	}

	for _, entry := range []struct {
		day, hourUTC int
		amount, note string
		reason       domain.ActivityReason
	}{
		{2, 1, "1500", "November pay", domain.ReasonIncome},
		{3, 10, "49.75", "Groceries", domain.ReasonExpense},
		{5, 0, "24", "Transit pass", domain.ReasonExpense},
		{8, 8, "125", "Workshop honorarium", domain.ReasonIncome},
		{10, 6, "68.40", "Home utilities", domain.ReasonExpense},
		{11, 16, "1.28", "Bank interest", domain.ReasonInterest},
	} {
		instant := time.Date(2026, time.November, entry.day, entry.hourUTC, 30, 0, 0, time.UTC)
		service.SetClock(func() time.Time { return instant })
		money, err := domain.ParseMoney(entry.amount, "USD")
		if err != nil {
			return err
		}
		if entry.reason == domain.ReasonExpense {
			_, err = service.RecordChange(ctx, domain.MoneyRemovedInput{
				HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID,
				Amount: money, Reason: entry.reason, EffectiveAt: instant, Note: &entry.note,
			})
		} else {
			_, err = service.RecordChange(ctx, domain.MoneyAddedInput{
				HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID,
				Amount: money, Reason: entry.reason, EffectiveAt: instant, Note: &entry.note,
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}
