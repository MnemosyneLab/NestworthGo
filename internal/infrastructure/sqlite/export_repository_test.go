package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestExportUsesOneReadSnapshotDuringConcurrentWrite(t *testing.T) {
	db, repo, _, account, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	writer, err := sql.Open("sqlite", db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := db.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var original string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id = ?`, account.ID.String()).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `UPDATE accounts SET name = 'Changed while exporting' WHERE id = ?`, account.ID.String()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readExportSnapshot(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Facts.Directory["accounts"][0]["name"] != original || snapshot.Portfolio.Accounts[0].Account.Name != original {
		t.Fatal("facts and summary inputs escaped the transaction")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	fresh, err := repo.ReadExportSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Facts.Directory["accounts"][0]["name"] != "Changed while exporting" {
		t.Fatal("fresh export did not see committed write")
	}
}

func TestExportAllowlistExcludesNewColumnsAndSanitizesConversion(t *testing.T) {
	db, repo, _, _, instrument := seedPortfolioRepository(t)
	if _, err := db.SQL.Exec(`ALTER TABLE accounts ADD COLUMN api_key TEXT DEFAULT 'secret';
 INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,conversion_json)
 VALUES('00000000-0000-4000-8000-000000000099',?,'1.12345678','USD','provider','test','2026-08-23T12:00:00Z','2026-08-23T12:00:00Z',?)`, instrument.ID.String(), `{"policy":"metal_usd_troy_ounce_v1","rawPrice":"31.1034768","apiKey":"secret"}`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.ReadExportSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot.Facts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "apiKey") || strings.Contains(string(data), "api_key") {
		t.Fatal("export leaked non-contract data")
	}
	quotes := snapshot.Facts.MarketData["instrumentQuotes"]
	if len(quotes) != 1 || quotes[0]["unitPrice"] != "1.12345678" || quotes[0]["conversion"] == nil {
		t.Fatalf("quote data = %+v", quotes)
	}
}
