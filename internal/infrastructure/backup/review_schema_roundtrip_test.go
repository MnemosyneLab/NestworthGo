package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestV19ReviewStateSurvivesCompleteBackup(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "synthetic-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const household = "00000000-0000-4000-8000-000000000001"
	const instrument = "00000000-0000-4000-8000-000000000002"
	const rule = "00000000-0000-4000-8000-000000000003"
	const epoch = "00000000-0000-4000-8000-000000000004"
	const episode = "00000000-0000-4000-8000-000000000005"
	if _, err := db.SQL.Exec(`
INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');
INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic stock','stock','CNY','chart-line','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');
INSERT INTO investment_review_control(household_id,projection_status) VALUES('00000000-0000-4000-8000-000000000001','complete');
INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001','Price','1','notice','daily_predicate_v1',1,1,'2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');
INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES('00000000-0000-4000-8000-000000000003',1,1,'{}','synthetic-hash','2026-10-01T00:00:00Z');
INSERT INTO investment_review_position_epochs(id,household_id,instrument_id,opened_at,start_unknown,source_fingerprint) VALUES('00000000-0000-4000-8000-000000000004','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000002','2026-10-01T00:00:00Z',0,'synthetic-source');
INSERT INTO investment_review_daily_inputs(household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint,computed_at) VALUES('00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000002','UTC/v1','2026-10-01','daily_metrics_v1','1','CNY','100','90','{}','{}','synthetic-input','2026-10-02T00:00:00Z');
INSERT INTO investment_review_daily_results(rule_id,condition_version,instrument_id,timezone_version,on_date,predicate_truth,evidence_json,input_fingerprint,evaluated_at) VALUES('00000000-0000-4000-8000-000000000003',1,'00000000-0000-4000-8000-000000000002','UTC/v1','2026-10-01','true','{}','synthetic-input','2026-10-02T00:00:00Z');
INSERT INTO investment_review_episodes(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,evidence_json,evidence_fingerprint,acknowledged_at,revision) VALUES('00000000-0000-4000-8000-000000000005','00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000004',1,'2026-10-01','2026-10-01','2026-10-02T00:00:00Z','2026-10-01','2026-10-01','2026-10-01',1,0,0,1,'{}','synthetic-evidence','2026-10-02T01:00:00Z',2);
INSERT INTO investment_review_suppressions(id,household_id,instrument_id,mode,until_on,timezone,revision,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000006','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000002','until_date','2026-10-30','UTC',1,'2026-10-02T00:00:00Z','2026-10-02T00:00:00Z');
INSERT INTO investment_review_mutation_receipts(household_id,mutation_id,request_hash,result_json,created_at) VALUES('00000000-0000-4000-8000-000000000001','synthetic-mutation','synthetic-request','{}','2026-10-02T00:00:00Z');
`); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "snapshot.db")
	if err := db.SnapshotTo(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	settings := []byte(`{"schema_version":1}`)
	pkg := Package{Manifest: NewManifest(time.Now().UTC(), payload, settings, sqlite.EntityCounts{Households: 1, Instruments: 1}), Database: payload, Settings: settings}
	archive := filepath.Join(dir, "synthetic.nestworth-backup")
	if err := WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadPackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	restoredPath := filepath.Join(dir, "restored.db")
	if err := ExtractDatabase(loaded, restoredPath); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.OpenReadOnlyForVerify(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	for _, table := range []string{"investment_review_control", "investment_review_rules", "investment_review_rule_versions", "investment_review_position_epochs", "investment_review_daily_inputs", "investment_review_daily_results", "investment_review_episodes", "investment_review_suppressions", "investment_review_mutation_receipts"} {
		var count int
		if err := restored.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var ack string
	if err := restored.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, episode).Scan(&ack); err != nil || ack != "2026-10-02T01:00:00Z" {
		t.Fatalf("episode ack lost: %q %v", ack, err)
	}
	var until string
	if err := restored.SQL.QueryRow(`SELECT until_on FROM investment_review_suppressions WHERE instrument_id=?`, instrument).Scan(&until); err != nil || until != "2026-10-30" {
		t.Fatalf("suppression moved: %q %v", until, err)
	}
	_ = household
	_ = rule
	_ = epoch
}
