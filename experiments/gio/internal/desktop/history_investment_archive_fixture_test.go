package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestExportInvestmentAmendmentArchivedHoldings(t *testing.T) {
	output := os.Getenv("NESTWORTH_INVESTMENT_ARCHIVE_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_INVESTMENT_ARCHIVE_QA_DIR to a disposable directory to export native QA fixtures")
	}
	root, err := os.MkdirTemp(output, "investment-archives-")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	now := time.Now().UTC()
	fixture := seedInvestmentAmendments(t, now)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "archive"), fixture.seed)
	db, _, service := openStage6Database(t, path, func() time.Time { return now })
	wails := historyapi.NewService(service)
	effective := now.Truncate(24*time.Hour).AddDate(0, 0, -1).Add(time.Hour)
	rows := []struct {
		target                            investmentAmendmentTarget
		instrument, quantity, gross, note string
	}{
		{fixture.sell, fixture.sell.activity.TradeDetail.InstrumentID, "15.5", "155", "Final sale before archiving PRSELL"},
		{fixture.dividend, fixture.dividend.activity.DividendDetail.InstrumentID, "10", "100", "Final sale before archiving PRDIV"},
	}
	holding := func(target investmentAmendmentTarget, amount string, archived bool) {
		t.Helper()
		byAccount, err := service.HoldingsByAccounts(ctx, []domain.AccountID{target.account})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range byAccount[target.account] {
			if h.ID == target.holding && h.Quantity.Canonical() == amount && (h.ArchivedAt != nil) == archived {
				return
			}
		}
		t.Fatalf("%s holding expected quantity %s, archived %t: %+v", target.accountName, amount, archived, byAccount[target.account])
	}
	for _, row := range rows {
		target := row.target
		holding(target, row.quantity, false)
		closeout, err := wails.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: "sell", SettlementAccountID: target.account.String(),
			HoldingID: target.holding.String(), InstrumentID: row.instrument, Quantity: row.quantity,
			Gross: row.gross, GrossCurrency: "EUR", EffectiveAt: effective.Format(time.RFC3339Nano), Note: &row.note,
		})
		if err != nil {
			t.Fatalf("liquidate %s: %v", target.accountName, err)
		}
		holding(target, "0", false)
		if err := service.ArchiveHolding(ctx, target.holding, true); err != nil {
			t.Fatalf("archive %s: %v", target.accountName, err)
		}
		holding(target, "0", true)
		t.Logf("account=%q original=%s final_sale=%s date=%s", target.accountName, target.activity.ID, closeout.Activity.ID, closeout.Activity.EffectiveLocalDate)
	}
	seed := stage6SeedBytes(t, db)
	for _, profile := range []string{"source", "wails", "gio"} {
		writeStage6Fixture(t, filepath.Join(root, profile), seed)
	}
	t.Logf("fixtures=%s household=%q", root, investmentAmendmentHousehold)
}
