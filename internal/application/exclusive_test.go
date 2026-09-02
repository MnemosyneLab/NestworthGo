package application

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func isBackupRestoreBusy(err error) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr != nil && domainErr.Code == domain.ErrBackupRestoreBusy
}

func holdExclusive(t *testing.T, service *Service, kind ExclusiveKind) (release func()) {
	t.Helper()
	started := make(chan struct{})
	done := make(chan struct{})
	releaseCh := make(chan struct{})
	go func() {
		_ = service.WithExclusive(context.Background(), kind, func(context.Context) error {
			close(started)
			<-releaseCh
			return nil
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("exclusive operation did not start")
	}
	return func() {
		close(releaseCh)
		<-done
	}
}

func holdWrite(t *testing.T, service *Service) (release func()) {
	t.Helper()
	started := make(chan struct{})
	done := make(chan struct{})
	releaseCh := make(chan struct{})
	go func() {
		_ = service.WithWrite(context.Background(), func(context.Context) error {
			close(started)
			<-releaseCh
			return nil
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("ordinary write did not start")
	}
	return func() {
		close(releaseCh)
		<-done
	}
}

func TestWithExclusiveRejectsOverlap(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "exclusive", []string{"Alice"})
	release := holdExclusive(t, service, ExclusiveBackup)
	if err := service.WithExclusive(ctx, ExclusiveCSV, func(context.Context) error { return nil }); !isBackupRestoreBusy(err) {
		t.Fatalf("overlapping exclusive = %v, want backup_restore_busy", err)
	}
	release()
	if err := service.WithExclusive(ctx, ExclusiveBackup, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveBackupRejectsActivityDirectoryIconAndSettingsWriters(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "exclusive-writers", []string{"Alice"})
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, service, ExclusiveBackup)
	defer release()

	amount, _ := domain.ParseMoney("10", "CNY")
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount,
		Reason: domain.ReasonIncome, EffectiveAt: service.clock(),
	}); !isBackupRestoreBusy(err) {
		t.Fatalf("activity write = %v, want backup_restore_busy", err)
	}
	if _, err := service.CreateInstitution(ctx, "Broker", domain.InstitutionBrokerage); !isBackupRestoreBusy(err) {
		t.Fatalf("directory write = %v, want backup_restore_busy", err)
	}
	if err := service.SetMemberIcon(ctx, bootstrap.Members[0].ID, "user"); !isBackupRestoreBusy(err) {
		t.Fatalf("icon write = %v, want backup_restore_busy", err)
	}
	if err := service.WithWrite(ctx, func(context.Context) error { return nil }); !isBackupRestoreBusy(err) {
		t.Fatalf("settings write = %v, want backup_restore_busy", err)
	}
}

func TestExclusiveRestoreRejectsMutationBeforeDatabaseClose(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "exclusive-restore", []string{"Alice"})
	started := make(chan struct{})
	closed := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- service.WithExclusiveKeep(ctx, ExclusiveRestore, func(context.Context) (bool, error) {
			close(started)
			<-release
			if err := service.CloseDatabase(); err != nil {
				return false, err
			}
			close(closed)
			return true, nil
		})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("restore exclusive did not start")
	}
	if _, err := service.CreateMember(ctx, "Bob"); !isBackupRestoreBusy(err) {
		t.Fatalf("mutation during restore = %v, want backup_restore_busy", err)
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("restore did not close the database")
	}
	if _, err := service.CreateMember(ctx, "Carol"); !isBackupRestoreBusy(err) {
		t.Fatalf("mutation after restore close = %v, want backup_restore_busy", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCSVCommitAndOrdinaryWriteAreMutuallyExclusive(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "exclusive-csv", []string{"Alice"})
	table := parseCSVTable(t, "account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,Alice:100%\n")
	plan, err := service.BuildCSVImportPlan(ctx, CSVProfileAccounts, table, nil, CSVParseOptions{DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none"}, nil)
	if err != nil || len(plan.Errors) != 0 {
		t.Fatalf("plan err=%v errors=%+v", err, plan.Errors)
	}

	releaseWrite := holdWrite(t, service)
	csvDone := make(chan error, 1)
	go func() {
		_, commitErr := service.CommitCSVImport(ctx, plan)
		csvDone <- commitErr
	}()
	select {
	case commitErr := <-csvDone:
		t.Fatalf("CSV commit finished while an ordinary write was held: %v", commitErr)
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := service.CreateGroup(ctx, "Blocked"); !isBackupRestoreBusy(err) {
		t.Fatalf("overlapping ordinary write = %v, want backup_restore_busy", err)
	}
	releaseWrite()
	select {
	case commitErr := <-csvDone:
		if commitErr != nil {
			t.Fatalf("CSV commit: %v", commitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CSV commit did not obtain the exclusive gate")
	}
	records, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("imported accounts = %+v, err=%v", records, err)
	}
}

func TestOrdinaryWritesRemainSerializedUnderRace(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "exclusive-race", []string{"Alice"})
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 4; i++ {
		group.Add(2)
		go func(i int) {
			defer group.Done()
			_, err := service.CreateInstitution(ctx, "Institution "+string(rune('A'+i)), domain.InstitutionOther)
			errs <- err
		}(i)
		go func(i int) {
			defer group.Done()
			_, err := service.CreateAccount(ctx, AccountInput{
				Name: "Cash " + string(rune('A'+i)), AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
				DefaultCurrency: "CNY", InitialAmount: "1", IncludeInNetWorth: true,
				OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
			})
			errs <- err
		}(i)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestExclusiveBackupRejectsInFlightRefreshPersist(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "refresh-exclusive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	fake := newRefreshFakeProvider()
	blocking := &blockingMarketDataRegistry{delegate: NewMarketDataRegistry(fake), entered: make(chan struct{}), release: make(chan struct{})}
	service := NewService(sqlite.NewRepository(database), blocking)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Refresh exclusive", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: "QQQ"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	price, _ := domain.ParseUnitPrice("700")
	fake.instruments["QQQ"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: "fake", QuotedAt: now}}
	rate, _ := domain.ParseFxRate("7")
	fake.fx["USD/CNY"] = struct {
		quote LatestFXQuote
		err   error
	}{quote: LatestFXQuote{Rate: rate, BaseCurrency: "USD", QuoteCurrency: "CNY", SourceKey: "fake", QuotedAt: now}}

	refreshDone := make(chan RefreshResult, 1)
	go func() {
		result, _ := service.RefreshAll(ctx)
		refreshDone <- result
	}()
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach provider I/O")
	}

	snapPath := filepath.Join(t.TempDir(), "backup.sqlite")
	exclusiveDone := make(chan error, 1)
	exclusiveStarted := make(chan struct{})
	exclusiveRelease := make(chan struct{})
	go func() {
		exclusiveDone <- service.WithExclusive(ctx, ExclusiveBackup, func(ctx context.Context) error {
			close(exclusiveStarted)
			service.LockWrites()
			snapErr := service.SnapshotTo(ctx, snapPath)
			service.UnlockWrites()
			<-exclusiveRelease
			return snapErr
		})
	}()
	select {
	case <-exclusiveStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("backup exclusive did not start during in-flight refresh")
	}
	close(blocking.release)
	var result RefreshResult
	select {
	case result = <-refreshDone:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not finish after exclusive started")
	}
	close(exclusiveRelease)
	if err := <-exclusiveDone; err != nil {
		t.Fatal(err)
	}
	rejected := false
	for _, item := range result.Items {
		if item.ErrorCode == domain.ErrBackupRestoreBusy {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatalf("refresh persist was not fenced: %+v", result.Items)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verified.EntityCounts(ctx); err != nil {
		t.Fatal(err)
	}
	_ = verified.Close()
}
