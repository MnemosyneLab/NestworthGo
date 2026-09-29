package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestV13CostMigrationPreservesActivitiesEffectsAndReceipts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v13-cost.db")
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(schema)
	legacy = strings.Replace(legacy, "'position_transfer','cost_adjustment','buy'", "'position_transfer','buy'", 1)
	legacy = strings.Replace(legacy, "'holding_quantity','holding_cost'", "'holding_quantity'", 1)
	legacy = strings.Replace(legacy, " OR\n          (target = 'holding_cost' AND account_id IS NULL AND holding_id IS NOT NULL AND instrument_id IS NOT NULL AND quantity IS NULL AND amount IS NULL AND currency IS NULL AND cost_unit_price IS NOT NULL)", "", 1)
	legacy = strings.Replace(legacy, "PRAGMA user_version = 14", "PRAGMA user_version = 13", 1)
	seed, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	const h = "00000000-0000-4000-8000-000000000001"
	const account = "00000000-0000-4000-8000-000000000002"
	const instrument = "00000000-0000-4000-8000-000000000003"
	const holding = "00000000-0000-4000-8000-000000000004"
	const activity = "00000000-0000-4000-8000-000000000005"
	const effect = "00000000-0000-4000-8000-000000000006"
	const stamp = "2026-09-01T00:00:00Z"
	for _, statement := range []string{
		`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('` + h + `','Home','USD','` + stamp + `','` + stamp + `')`,
		`INSERT INTO members(id,household_id,name,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000011','` + h + `','Owner','person','` + stamp + `','` + stamp + `')`,
		`INSERT INTO accounts(id,household_id,name,account_type,balance_sheet_role,tracking_mode,default_currency,icon_key,created_at,updated_at) VALUES('` + account + `','` + h + `','Broker','brokerage','asset','holdings','USD','investment','` + stamp + `','` + stamp + `')`,
		`INSERT INTO account_ownership(account_id,member_id,share_bps) VALUES('` + account + `','00000000-0000-4000-8000-000000000011',10000)`,
		`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('` + instrument + `','` + h + `','Fund','etf','USD','investment','` + stamp + `','` + stamp + `')`,
		`INSERT INTO holdings(id,account_id,instrument_id,quantity,created_at,updated_at) VALUES('` + holding + `','` + account + `','` + instrument + `','10','` + stamp + `','` + stamp + `')`,
		`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES('` + activity + `','` + h + `','buy','principal','` + stamp + `','2026-09-01','` + stamp + `')`,
		`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,holding_id,instrument_id,quantity) VALUES('` + effect + `','` + activity + `',1,'quantity','added','holding_quantity','trade_principal','` + holding + `','` + instrument + `','10')`,
		`INSERT INTO holding_quantity_values(id,holding_id,quantity,effective_at,created_at,activity_effect_id,projection_kind) VALUES('00000000-0000-4000-8000-000000000007','` + holding + `','10','` + stamp + `','` + stamp + `','` + effect + `','event')`,
		`INSERT INTO activity_trade_details(activity_id,side,instrument_id,holding_id,quantity,gross_amount,gross_currency,unit_price) VALUES('` + activity + `','buy','` + instrument + `','` + holding + `','10','250','USD','25')`,
		`INSERT INTO activity_mutation_keys(household_id,mutation_id,payload_sha256,activity_id,created_at) VALUES('` + h + `','00000000-0000-4000-8000-000000000008','` + strings.Repeat("a", 64) + `','` + activity + `','` + stamp + `')`,
	} {
		if _, err := seed.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM activity_mutation_keys m JOIN activities a ON a.id = m.activity_id JOIN activity_effects e ON e.activity_id = a.id JOIN holding_quantity_values q ON q.activity_effect_id = e.id WHERE a.id = ?`, activity).Scan(&count); err != nil || count != 1 {
		t.Fatalf("v13 receipt and projection after migration: count=%d err=%v", count, err)
	}
	const correction = "00000000-0000-4000-8000-000000000009"
	if _, err := db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cost_adjustment','reconciliation',?,'2026-09-01',?)`, correction, h, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,holding_id,instrument_id,cost_unit_price) VALUES('00000000-0000-4000-8000-000000000010',?,1,'cost','added','holding_cost','remeasurement',?,?,'30')`, correction, holding, instrument); err != nil {
		t.Fatal(err)
	}
	events, err := NewRepository(db).ListCostBasisEvents(ctx, domain.HoldingID(holding), domain.CostBasisReadFilter{})
	if err != nil || len(events) != 2 || events[1].Kind != domain.CostBasisCostAdjustment || events[1].UnitCost.Canonical() != "30" {
		t.Fatalf("migrated cost events=%+v err=%v", events, err)
	}
	export, err := NewRepository(db).ReadExportSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range export.Facts.History["effects"] {
		if record["activityId"] == correction && record["target"] == "holding_cost" && record["costUnitPrice"] == "30" {
			found = true
		}
	}
	if !found {
		t.Fatal("JSON export omitted the cost correction effect")
	}
}
