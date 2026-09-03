package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestOpenReadOnlyForVerifyRejectsMissingFile(t *testing.T) {
	_, err := OpenReadOnlyForVerify(filepath.Join(t.TempDir(), "missing.db"))
	if err == nil {
		t.Fatal("expected missing file to fail")
	}
}

func TestOpenReadOnlyForVerifyDoesNotCreateOrRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.db")
	live, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	verified, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	defer verified.Close()
	counts, err := verified.EntityCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Households != 0 {
		t.Fatalf("households = %d", counts.Households)
	}
}

func TestSnapshotToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	live, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	dest := filepath.Join(dir, "snap.db")
	if err := live.SnapshotTo(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("snapshot unexpectedly has wal: %v", err)
	}
	verified, err := OpenReadOnlyForVerify(dest)
	if err != nil {
		t.Fatal(err)
	}
	_ = verified.Close()
}

func TestOpenReadOnlyForVerifyDoesNotCreateParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "db.sqlite")
	_, err := OpenReadOnlyForVerify(path)
	if err == nil {
		t.Fatal("expected failure")
	}
	if _, statErr := os.Stat(filepath.Dir(path)); !os.IsNotExist(statErr) {
		t.Fatal("OpenReadOnlyForVerify created a parent directory")
	}
}

func TestOpenReadOnlyForVerifyAcceptsBothV9CashOnHandChecks(t *testing.T) {
	database, _, _, _, _ := seedPortfolioRepository(t)
	path := database.Path
	ctx := context.Background()

	widened, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("widened v9 check rejected: %v", err)
	}
	if err := widened.Close(); err != nil {
		t.Fatal(err)
	}

	if err := rewriteAccountsCheckFragment(ctx, database.SQL, cashOnHandBalanceOrHoldingsCheck, cashOnHandBalanceOnlyCheck); err != nil {
		t.Fatalf("install original v9 check: %v", err)
	}
	if got := accountsCreateSQL(t, database.SQL); !schemaSQLContains(got, cashOnHandBalanceOnlyCheck) {
		t.Fatalf("original v9 check missing: %s", got)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	original, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("original v9 check rejected: %v", err)
	}
	counts, err := original.EntityCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Households != 1 || counts.Accounts != 1 || counts.Instruments != 1 || counts.Members != 1 {
		t.Fatalf("linked row counts = %+v", counts)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only verify changed the original v9 database bytes")
	}
}

func TestOpenReadOnlyForVerifyRejectsUnknownCashOnHandCheck(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "unknown-check.db"))
	if err != nil {
		t.Fatal(err)
	}
	path := database.Path
	unknown := "account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode = 'manual_value'"
	if err := rewriteAccountsCheckFragment(context.Background(), database.SQL, cashOnHandBalanceOrHoldingsCheck, unknown); err != nil {
		t.Fatalf("install unknown check: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = OpenReadOnlyForVerify(path)
	if err == nil {
		t.Fatal("unknown cash_on_hand check was accepted")
	}
	appErr, ok := err.(*domain.Error)
	if !ok || appErr.Code != domain.ErrBackupIntegrityFailed {
		t.Fatalf("unknown check error = %v, want backup_integrity_failed", err)
	}
}
