package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// This opt-in test accepts a standalone SQLite backup, never the application's
// live database. Every write targets a fresh temporary copy. No private labels,
// identifiers, or balances are included in test output.
//
// NESTWORTH_QA_DATABASE=/path/to/backup.db go test ./internal/application -run TestRealDataLiquiditySmoke -count=1 -v
func TestRealDataLiquiditySmoke(t *testing.T) {
	backupPath := os.Getenv("NESTWORTH_QA_DATABASE")
	if backupPath == "" {
		t.Skip("set NESTWORTH_QA_DATABASE to an existing standalone backup")
	}
	if info, err := os.Stat(backupPath + "-wal"); err == nil && info.Size() > 0 {
		t.Fatal("a standalone backup is required; the supplied file has a WAL")
	}
	backup, err := os.ReadFile(backupPath)
	realDataSmokeRequire(t, "read supplied backup", err)
	digest := sha256.Sum256(backup)
	t.Cleanup(func() {
		after, err := os.ReadFile(backupPath)
		realDataSmokeRequire(t, "recheck supplied backup", err)
		if sha256.Sum256(after) != digest {
			t.Error("supplied backup changed during the smoke test")
		}
	})
	copyPath := filepath.Join(t.TempDir(), "smoke.db")
	realDataSmokeRequire(t, "copy backup", os.WriteFile(copyPath, backup, 0600))
	db, err := sqlite.Open(copyPath)
	realDataSmokeRequire(t, "open copied database", err)
	t.Cleanup(func() { realDataSmokeRequire(t, "close copied database", db.Close()) })
	ctx := context.Background()
	repository := sqlite.NewRepository(db)
	service := NewService(repository)
	now := time.Now().UTC().Truncate(time.Second)
	service.setClock(func() time.Time { return now })
	bootstrap, err := service.Bootstrap(ctx)
	realDataSmokeRequire(t, "bootstrap copied household", err)
	if bootstrap.Household == nil {
		t.Skip("backup does not contain a household")
	}
	snapshot := func(t *testing.T) domain.LiquiditySnapshot {
		t.Helper()
		value, err := repository.ReadLiquiditySnapshot(ctx)
		realDataSmokeRequire(t, "read consistent snapshot", err)
		return value
	}
	readOverview := func(t *testing.T) domain.OverviewResult {
		t.Helper()
		value, err := service.Overview(ctx, domain.AccountFilter{})
		realDataSmokeRequire(t, "read net worth", err)
		if !value.NetWorth.Equal(value.Assets.Sub(value.Liabilities)) {
			t.Fatal("net worth does not equal assets less liabilities")
		}
		return value
	}
	readLiquidity := func(t *testing.T) domain.LiquidityOverview {
		t.Helper()
		value, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
		realDataSmokeRequire(t, "read availability", err)
		if len(value.Buckets) == 0 {
			t.Fatal("availability has no horizons")
		}
		for _, bucket := range value.Buckets {
			if bucket.Status == domain.StatusComplete && (bucket.FullAvailable == nil || bucket.FullUnreserved == nil) {
				t.Fatal("complete availability is missing its total")
			}
			if bucket.Status == domain.StatusUnavailable && (bucket.FullAvailable != nil || bucket.FullUnreserved != nil) {
				t.Fatal("unknown availability exposes a full total")
			}
		}
		return value
	}
	initialSnapshot := snapshot(t)
	initialOverview := readOverview(t)
	initialLiquidity := readLiquidity(t)
	t.Logf("copied data loaded: netWorthComplete=%t missingInputs=%d availabilityStatus=%s", initialOverview.Complete, len(initialOverview.MissingInputs), initialLiquidity.Buckets[0].Status)
	if initialSnapshot.Portfolio.Origin == nil {
		t.Skip("backup has no history starting point")
	}
	if initialSnapshot.Portfolio.Origin.StartedAt.After(now) {
		t.Skip("backup starting point is in the future")
	}

	t.Run("existing_deposit_preview_is_read_only", func(t *testing.T) {
		for _, contract := range initialSnapshot.Contracts {
			if contract.Kind != domain.ProductTermDeposit || contract.State != domain.ProductStateOpen {
				continue
			}
			before := snapshot(t)
			var beforeChanges, afterChanges int
			realDataSmokeRequire(t, "read SQLite change counter", db.SQL.QueryRowContext(ctx, "SELECT total_changes()").Scan(&beforeChanges))
			principal := contract.Principal.CanonicalAmount()
			command := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: contract.ID.String(), ReturnedPrincipal: &principal}}
			for _, reservation := range before.Reservations {
				if reservation.Active() && reservation.Source.Key() == domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key() {
					command.Settle.ReleaseReservationIDs = append(command.Settle.ReleaseReservationIDs, reservation.ID.String())
				}
			}
			preview, err := service.PreviewProductOperation(ctx, command)
			realDataSmokeRequire(t, "preview existing deposit receipt", err)
			if preview.ReviewedStateHash == "" || len(preview.Activities) == 0 {
				t.Fatal("existing deposit preview is incomplete")
			}
			realDataSmokeRequire(t, "recheck SQLite change counter", db.SQL.QueryRowContext(ctx, "SELECT total_changes()").Scan(&afterChanges))
			if beforeChanges != afterChanges || !reflect.DeepEqual(before, snapshot(t)) {
				t.Fatal("preview changed copied financial data")
			}
			return
		}
		t.Skip("backup has no open term deposit")
	})

	accounts := make(map[domain.AccountID]domain.Account)
	for _, record := range initialSnapshot.Portfolio.Accounts {
		accounts[record.Account.ID] = record.Account
	}
	var source *domain.LiquiditySourceResult
	var unitBase decimal.Decimal
	for _, candidate := range initialLiquidity.Sources {
		account := accounts[candidate.AccountID]
		if candidate.Ref.Kind != domain.SourceAccountCash || candidate.Excluded || domain.ProductAccountEligible(account) != nil || !account.IncludeInNetWorth || len(candidate.BucketResults) == 0 {
			continue
		}
		row := candidate.BucketResults[0]
		if row.Status != domain.StatusComplete || row.UnreservedNative == nil || row.UnreservedNative.Amount().LessThan(decimal.NewFromInt(20)) {
			continue
		}
		unit, err := domain.ParseMoney("1", candidate.NativeCurrency)
		realDataSmokeRequire(t, "prepare local test amount", err)
		converted, complete, err := service.valuation.ConvertAmount(initialSnapshot.Portfolio, unit)
		realDataSmokeRequire(t, "check existing FX coverage", err)
		if !complete || converted == nil {
			continue
		}
		chosen := candidate
		source, unitBase = &chosen, *converted
		break
	}
	if source == nil {
		t.Skip("no eligible account has sufficient known unreserved cash and existing FX coverage")
	}
	readSource := func(t *testing.T) domain.LiquiditySourceResult {
		t.Helper()
		for _, current := range readLiquidity(t).Sources {
			if current.Ref.Key() == source.Ref.Key() {
				return current
			}
		}
		t.Fatal("selected source disappeared")
		return domain.LiquiditySourceResult{}
	}
	assertCash := func(t *testing.T, expected decimal.Decimal) {
		t.Helper()
		for _, value := range latestCashValues(snapshot(t).Portfolio.CashValues) {
			if value.AccountID == source.AccountID && value.Amount.Currency() == source.NativeCurrency {
				if !value.Amount.Amount().Equal(expected) {
					t.Fatal("cash movement does not match recorded principal, interest and fees")
				}
				return
			}
		}
		t.Fatal("selected account cash is missing")
	}
	baselineCash := source.CurrentNativeValue.Amount()
	baselineNetWorth := initialOverview.NetWorth
	assertNetWorth := func(t *testing.T, nativeDelta decimal.Decimal) {
		t.Helper()
		current := readOverview(t)
		if current.Complete != initialOverview.Complete || len(current.MissingInputs) != len(initialOverview.MissingInputs) {
			t.Fatal("a local product operation changed valuation completeness")
		}
		// Inverse FX division is finite-precision decimal arithmetic. The 1e-12
		// tolerance is far below the smallest supported monetary precision.
		if current.NetWorth.Sub(baselineNetWorth).Sub(nativeDelta.Mul(unitBase)).Abs().GreaterThan(decimal.RequireFromString("0.000000000001")) {
			t.Fatal("net worth change does not match actual interest less fees")
		}
	}

	t.Run("reservation_create_edit_release", func(t *testing.T) {
		before := snapshot(t)
		reservation, err := service.SaveLiquidityReservation(ctx, SaveReservationInput{Source: source.Ref, Label: "Local smoke reserve", Amount: "1"})
		realDataSmokeRequire(t, "create reserve", err)
		assertReserve := func(increase int64) {
			t.Helper()
			row := readSource(t).BucketResults[0]
			original := source.BucketResults[0]
			if row.NetNative == nil || row.AppliedReserveNative == nil || row.UnreservedNative == nil ||
				!row.NetNative.Amount().Equal(original.NetNative.Amount()) ||
				!row.AppliedReserveNative.Amount().Equal(original.AppliedReserveNative.Amount().Add(decimal.NewFromInt(increase))) ||
				!row.UnreservedNative.Amount().Equal(original.UnreservedNative.Amount().Sub(decimal.NewFromInt(increase))) {
				t.Fatal("reserve did not change unreserved availability by the expected amount")
			}
			if !reflect.DeepEqual(before.Portfolio, snapshot(t).Portfolio) {
				t.Fatal("reservation changed balances, holdings or ledger observations")
			}
			assertNetWorth(t, decimal.Zero)
		}
		assertReserve(1)
		reservation, err = service.SaveLiquidityReservation(ctx, SaveReservationInput{ID: &reservation.ID, ExpectedRevision: reservation.Revision, Source: source.Ref, Label: "Updated smoke reserve", Amount: "2"})
		realDataSmokeRequire(t, "edit reserve", err)
		assertReserve(2)
		_, err = service.ReleaseLiquidityReservation(ctx, reservation.ID, reservation.Revision)
		realDataSmokeRequire(t, "release reserve", err)
		assertReserve(0)
	})

	t.Run("deposit_open_receipt_and_grouped_undo", func(t *testing.T) {
		record := func(command ProductCommand) ProductOperationReceipt {
			t.Helper()
			now = now.Add(time.Second)
			preview, err := service.PreviewProductOperation(ctx, command)
			realDataSmokeRequire(t, "preview product operation", err)
			id := domain.NewProductOperationID().String()
			receipt, err := service.RecordProductOperation(ctx, command, id, preview.ReviewedStateHash)
			realDataSmokeRequire(t, "record product operation", err)
			beforeReplay := snapshot(t)
			replayed, err := service.RecordProductOperation(ctx, command, id, preview.ReviewedStateHash)
			realDataSmokeRequire(t, "replay product operation", err)
			if !replayed.Replayed || replayed.OperationID != receipt.OperationID || !reflect.DeepEqual(beforeReplay, snapshot(t)) {
				t.Fatal("retry duplicated a product operation")
			}
			return receipt
		}
		start, _, err := domain.LocalCivilDate(now, initialSnapshot.Portfolio.Origin.Timezone)
		realDataSmokeRequire(t, "resolve household date", err)
		startDate, err := time.Parse("2006-01-02", start)
		realDataSmokeRequire(t, "parse household date", err)
		maturity := startDate.AddDate(0, 0, 1).Format("2006-01-02")
		opening := record(ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
			AccountID: source.AccountID.String(), Currency: source.NativeCurrency.String(), Principal: "10", OpeningFee: strPtr("1"),
			Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Local smoke deposit", StartOn: start, MaturityOn: &maturity, InterestMode: "none"},
			Policy: depositPolicyForMaturity(maturity),
		}})
		assertProduct := func(state domain.ProductContractState, quantity string) {
			t.Helper()
			detail, err := service.Product(ctx, opening.ProductIDs[0])
			realDataSmokeRequire(t, "read product state", err)
			holding, err := repository.Holding(ctx, detail.Contract.HoldingID)
			realDataSmokeRequire(t, "read managed holding", err)
			if detail.Contract.State != state || holding.Quantity.Canonical() != quantity {
				t.Fatal("product lifecycle and holding quantity disagree")
			}
		}
		assertCash(t, baselineCash.Sub(decimal.NewFromInt(11)))
		assertNetWorth(t, decimal.NewFromInt(-1))
		assertProduct(domain.ProductStateOpen, "1")
		now = now.Add(48 * time.Hour)
		settled := record(ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{
			ProductID: opening.ProductIDs[0].String(), ReturnedPrincipal: strPtr("10"), Interest: strPtr("3"), Fee: strPtr("1"),
		}})
		assertCash(t, baselineCash.Add(decimal.NewFromInt(1)))
		assertNetWorth(t, decimal.NewFromInt(1))
		assertProduct(domain.ProductStateSettled, "0")
		record(ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settled.OperationID.String()}})
		assertCash(t, baselineCash.Sub(decimal.NewFromInt(11)))
		assertNetWorth(t, decimal.NewFromInt(-1))
		assertProduct(domain.ProductStateOpen, "1")
		record(ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: opening.OperationID.String()}})
		assertCash(t, baselineCash)
		assertNetWorth(t, decimal.Zero)
		assertProduct(domain.ProductStateCancelled, "0")
	})
	finalSnapshot := snapshot(t)
	for _, original := range initialSnapshot.Portfolio.Holdings {
		found := false
		for _, current := range finalSnapshot.Portfolio.Holdings {
			if current.ID == original.ID {
				found = true
				if !reflect.DeepEqual(current, original) {
					t.Fatal("product round trip changed a pre-existing holding")
				}
			}
		}
		if !found {
			t.Fatal("product round trip removed a pre-existing holding")
		}
	}

	verified, err := sqlite.OpenReadOnlyForVerify(copyPath)
	realDataSmokeRequire(t, "verify copied database after round trip", err)
	realDataSmokeRequire(t, "close copied database verification", verified.Close())
}

func realDataSmokeRequire(t *testing.T, stage string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		t.Fatalf("%s failed: code=%s field=%s", stage, domainError.Code, domainError.Field)
	}
	t.Fatalf("%s failed (%T)", stage, err)
}
