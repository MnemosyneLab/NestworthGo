package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestGuardedHistoricalChangeRejectsStalePreviewAndReplaysAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guarded-change.db")
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(sqlite.NewRepository(database))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Guarded", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = start.Add(96 * time.Hour)
	removed, _ := domain.ParseMoney("100", "USD")
	later := domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: removed, Reason: domain.ReasonExpense, EffectiveAt: start.Add(72 * time.Hour)}
	if _, err := service.RecordChange(ctx, later); err != nil {
		t.Fatal(err)
	}
	added, _ := domain.ParseMoney("50", "USD")
	backdated := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: added, Reason: domain.ReasonIncome, EffectiveAt: start.Add(48 * time.Hour)}
	_, staleToken, err := service.PreviewChangeGuarded(ctx, backdated)
	if err != nil {
		t.Fatal(err)
	}
	// An ordinary write permit invalidates a reviewed state even if its caller
	// returns an error. This conservative rule covers all app-owned writes.
	_ = service.WithWrite(ctx, func(context.Context) error { return errors.New("failed write") })
	id := domain.NewMutationID().String()
	hash := strings.Repeat("ab", 32)
	if _, err := service.RecordChangeGuarded(ctx, backdated, id, hash, staleToken); !hasDomainCode(err, domain.ErrStalePreview) {
		t.Fatalf("stale guarded commit = %v, want stale_preview", err)
	}
	exclusivePreview, exclusiveToken, err := service.PreviewChangeGuarded(ctx, backdated)
	if err != nil || exclusivePreview.Activity.ID == "" {
		t.Fatalf("preview before backup = %+v, %v", exclusivePreview, err)
	}
	if err := service.WithExclusive(ctx, ExclusiveBackup, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChangeGuarded(ctx, backdated, id, hash, exclusiveToken); !hasDomainCode(err, domain.ErrStalePreview) {
		t.Fatalf("commit after backup = %v, want stale_preview", err)
	}
	_, token, err := service.PreviewChangeGuarded(ctx, backdated)
	if err != nil {
		t.Fatal(err)
	}
	_, secondToken, err := service.PreviewChangeGuarded(ctx, backdated)
	if err != nil || secondToken != token {
		t.Fatalf("second read changed token: %q, %q, %v", token, secondToken, err)
	}
	committed, err := service.RecordChangeGuarded(ctx, backdated, id, hash, token)
	if err != nil {
		t.Fatalf("guarded historical commit: %v", err)
	}
	if len(committed.Resulting) != 1 || committed.Resulting[0].Amount != "1050" {
		t.Fatalf("historical preview resulting = %+v, want 1050 before later expense", committed.Resulting)
	}
	if _, err := service.RecordChangeGuarded(ctx, backdated, domain.NewMutationID().String(), hash, secondToken); !hasDomainCode(err, domain.ErrStalePreview) {
		t.Fatalf("second commit with earlier token = %v, want stale_preview", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted := NewService(sqlite.NewRepository(reopened))
	restarted.setClock(func() time.Time { return now })
	replayed, err := restarted.RecordChangeGuarded(ctx, backdated, id, hash, token)
	if err != nil {
		t.Fatalf("replay after restart: %v", err)
	}
	if replayed.Activity.ID != committed.Activity.ID {
		t.Fatalf("replayed activity = %s, want %s", replayed.Activity.ID, committed.Activity.ID)
	}
	if _, err := restarted.RecordChangeGuarded(ctx, backdated, id, strings.Repeat("cd", 32), token); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("changed payload replay = %v, want conflict", err)
	}
	var activities int
	if err := reopened.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 2 {
		t.Fatalf("activities = %d, want two distinct entries", activities)
	}
}

func hasDomainCode(err error, code domain.ErrorCode) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr.Code == code
}

func TestGuardedBackdatedFirstBuyPreviewsWithoutPersistingHolding(t *testing.T) {
	ctx, service, _, _, bootstrap, setNow := historicalInsertionFixture(t)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "CNY", "2026-09-25"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "First ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Archived ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveInstrument(ctx, archived.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	command := domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "200", "CNY"), EffectiveAt: historicalInsertionTime(26)}
	archivedCommand := command
	archivedCommand.InstrumentID = archived.ID
	if _, err := service.PreviewChange(ctx, archivedCommand); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("first buy of archived instrument = %v, want conflict", err)
	}
	preview, token, err := service.PreviewChangeGuarded(ctx, command)
	if err != nil {
		t.Fatalf("backdated first-buy preview: %v", err)
	}
	if len(preview.Resulting) != 2 || preview.Resulting[0].Amount != "800" || preview.Resulting[1].Quantity != "2" {
		t.Fatalf("preview resulting = %+v, want cash 800 and quantity 2", preview.Resulting)
	}
	before, err := service.ListHoldings(ctx, account.Account.ID, false)
	if err != nil || len(before) != 0 {
		t.Fatalf("preview persisted holding: %+v, %v", before, err)
	}
	committed, err := service.RecordChangeGuarded(ctx, command, domain.NewMutationID().String(), strings.Repeat("ef", 32), token)
	if err != nil {
		t.Fatalf("backdated first-buy commit: %v", err)
	}
	after, err := service.ListHoldings(ctx, account.Account.ID, false)
	if err != nil || len(after) != 1 || after[0].Quantity.Canonical() != "2" {
		t.Fatalf("committed holding = %+v, %v", after, err)
	}
	if committed.Activity.TradeDetail == nil || committed.Activity.TradeDetail.HoldingID != after[0].ID {
		t.Fatalf("committed trade does not reference persisted holding: %+v", committed.Activity.TradeDetail)
	}
}

func TestConcurrentGuardedCommitsAllowOneMutationAndReplayItsID(t *testing.T) {
	ctx, service, repo, _, bootstrap, setNow := historicalInsertionFixture(t)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	command := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "10", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(26)}
	_, token, err := service.PreviewChangeGuarded(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		id      string
		preview domain.ChangePreview
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	hash := strings.Repeat("ba", 32)
	for i := 0; i < 2; i++ {
		id := domain.NewMutationID().String()
		go func() {
			<-start
			preview, err := service.RecordChangeGuarded(ctx, command, id, hash, token)
			results <- result{id: id, preview: preview, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	winner, loser := first, second
	if first.err != nil {
		winner, loser = second, first
	}
	if winner.err != nil || !hasDomainCode(loser.err, domain.ErrStalePreview) {
		t.Fatalf("concurrent commits = %+v / %+v, want one success and one stale_preview", first, second)
	}
	replayed, err := service.RecordChangeGuarded(ctx, command, winner.id, hash, token)
	if err != nil || replayed.Activity.ID != winner.preview.Activity.ID {
		t.Fatalf("same ID replay = %+v, %v; want %s", replayed, err, winner.preview.Activity.ID)
	}
	activities, err := repo.ListActivities(ctx, bootstrap.Household.ID, 10)
	if err != nil || len(activities) != 1 {
		t.Fatalf("activities after competing commits = %+v, %v", activities, err)
	}
}
