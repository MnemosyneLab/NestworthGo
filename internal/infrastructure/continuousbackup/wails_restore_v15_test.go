package continuousbackup_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/superfly/ltx"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	infra "github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/version"
	wailsbackup "github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup"
	"github.com/waltwang/nestworth-go/internal/wailsapi/recovery"
)

// Real Manager.Stage, local replica decoding, Wails inspect/confirm, packaging,
// restore journal and upgrade run against disposable synthetic cloud objects.
func snapshotLTX(t *testing.T, data []byte) []byte {
	t.Helper()
	size := int(binary.BigEndian.Uint16(data[16:18]))
	if size == 1 {
		size = 65536
	}
	var out bytes.Buffer
	enc, err := ltx.NewEncoder(&out)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.EncodeHeader(ltx.Header{Version: ltx.Version, PageSize: uint32(size), Flags: ltx.HeaderFlagNoChecksum, Commit: uint32(len(data) / size), MinTXID: 1, MaxTXID: 1, Timestamp: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).UnixMilli()}); err != nil {
		t.Fatal(err)
	}

	for offset := 0; offset < len(data); offset += size {
		pgno := uint32(offset/size + 1)
		page := data[offset : offset+size]
		if err := enc.EncodePage(ltx.PageHeader{Pgno: pgno}, page); err != nil {
			t.Fatal(err)
		}

	}

	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func cloudSourceV15(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-v15.sqlite")
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../sqlite/testdata/schema_v15.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
 INSERT INTO households VALUES('00000000-0000-4000-8000-000000000001',1,'Synthetic cloud','USD','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO members(id,household_id,name,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000007','00000000-0000-4000-8000-000000000001','Owner','user','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO accounts(id,household_id,name,account_type,balance_sheet_role,tracking_mode,default_currency,icon_key,created_at,updated_at)
 VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Brokerage','brokerage','asset','holdings','USD','chart-line','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO account_ownership VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000007',10000);
 INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,symbol,isin,icon_key,created_at,updated_at)
 VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001','Fund class A','mutual_fund','USD','000001','US0000000001','chart-line','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO holdings(id,account_id,instrument_id,quantity,created_at,updated_at)
 VALUES('00000000-0000-4000-8000-000000000004','00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000003','7.1256789','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at)
 VALUES('00000000-0000-4000-8000-000000000005','00000000-0000-4000-8000-000000000002','98765.4321','USD','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at)
 VALUES('00000000-0000-4000-8000-000000000006','00000000-0000-4000-8000-000000000003','123.45678901','USD','manual','manual','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	verified.Close()
	return path
}

func cloudRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'reconciliation_%' AND name NOT LIKE 'investment_review_%' AND name != 'account_financial_revisions' AND name NOT IN ('financial_source_revision','financial_source_changes') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	out := map[string][]string{}
	for _, name := range names {
		info, err := db.Query(`PRAGMA table_info("` + name + `")`)
		if err != nil {
			t.Fatal(err)
		}
		var columns []string
		for info.Next() {
			var cid, nn, pk int
			var n, typ string
			var d sql.NullString
			if err := info.Scan(&cid, &n, &typ, &nn, &d, &pk); err != nil {
				t.Fatal(err)
			}
			if n != "short_name" {
				columns = append(columns, `"`+n+`"`)
			}
		}
		info.Close()
		list := strings.Join(columns, ",")
		rows, err := db.Query(`SELECT ` + list + ` FROM "` + name + `" ORDER BY ` + list)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = []string{}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			out[name] = append(out[name], string(raw))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return out
}

func TestPopulatedV15CloudInspectConfirmAndUpgrade(t *testing.T) {
	for _, scenario := range []string{"restore", "quiesce-failure", "mismatched-schema", "future-schema", "corrupt-ltx"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			source := cloudSourceV15(t)
			read, err := sqlite.OpenReadOnlyForVerify(source)
			if err != nil {
				t.Fatal(err)
			}
			expected := cloudRows(t, read.SQL)
			read.Close()
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			stream := "00000000-0000-4000-8000-000000000010/00000000-0000-4000-8000-000000000011"
			schema := 15
			if scenario == "mismatched-schema" {
				schema = 16
			}
			if scenario == "future-schema" {
				schema = 17
			}
			identity, err := json.Marshal(map[string]any{"format": 1, "appID": version.AppID, "appVersion": "0.3.7", "appBuild": "synthetic-v15", "schema": schema, "streamID": stream, "startedAt": "2026-09-01T00:00:00Z"})
			if err != nil {
				t.Fatal(err)
			}
			ltxData := snapshotLTX(t, data)
			if scenario == "corrupt-ltx" {
				ltxData = []byte("corrupt synthetic LTX")
			}
			root := t.TempDir()
			streamDir := filepath.Join(root, filepath.FromSlash(stream))
			if err := os.MkdirAll(filepath.Join(streamDir, "ltx", "0"), 0700); err != nil {
				t.Fatal(err)
			}
			identityPath := filepath.Join(streamDir, "stream.json")
			if err := os.WriteFile(identityPath, identity, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(streamDir, "ltx", "0", ltx.FormatFilename(1, 1)), ltxData, 0600); err != nil {
				t.Fatal(err)
			}

			live := filepath.Join(t.TempDir(), "live.sqlite")
			db, err := sqlite.Open(live)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			app := application.NewService(sqlite.NewRepository(db))
			appports.Wire(app)
			app.SetLiveDatabasePath(live)
			if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Current household", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec(`INSERT INTO app_configuration VALUES('live-only','must survive failure')`); err != nil {
				t.Fatal(err)
			}
			current := cloudRows(t, db.SQL)
			manager, err := infra.NewSyntheticCloudManager(live, db, root)
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close(ctx)
			if err := manager.Configure(ctx, infra.Update{Enabled: false, AccountID: strings.Repeat("a", 32), Bucket: "test-bucket", AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-secret"}); err != nil {
				t.Fatal(err)
			}
			inspector := application.NewRecovery(live, backup.NewRuntime(), app)
			defer inspector.Shutdown()
			quiesced := 0
			inspector.SetBeforeRestore(func(ctx context.Context) error {
				quiesced++
				if scenario == "quiesce-failure" {
					return errors.New("synthetic quiesce failure")
				}
				return manager.PauseForRestore(ctx)
			})
			service := wailsbackup.NewService(manager, app, inspector, recovery.NewService(inspector, nil, nil, nil))
			point := infra.RecoveryPoint{StreamID: stream, TXID: ltx.TXID(1).String(), CapturedAt: "2026-09-01T00:00:00Z"}
			preview, err := service.InspectRestore(ctx, point)
			if scenario == "mismatched-schema" || scenario == "future-schema" || scenario == "corrupt-ltx" {
				if err == nil {
					t.Fatal("bad cloud candidate was accepted")
				}
				if quiesced != 0 || db.SQL == nil || !reflect.DeepEqual(current, cloudRows(t, db.SQL)) {
					t.Fatal("failed inspect changed live session")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if preview.SchemaVersion != 15 || preview.BackupHousehold != "Synthetic cloud" || preview.BackupAccounts != 1 || preview.BackupHoldings != 1 || preview.AppBuild != "synthetic-v15" || !preview.HasOpenSession {
				t.Fatalf("untruthful preview %+v", preview)
			}
			if quiesced != 0 || !reflect.DeepEqual(current, cloudRows(t, db.SQL)) {
				t.Fatal("inspect modified live records")
			}
			if _, err := service.ConfirmRestore(recovery.RestoreConfirmRequest{Token: preview.Token, Confirmation: "WRONG", Acknowledged: true}); err == nil {
				t.Fatal("restored without confirmation")
			}
			if quiesced != 0 || db.SQL == nil || !reflect.DeepEqual(current, cloudRows(t, db.SQL)) {
				t.Fatal("invalid confirmation changed live session")
			}
			result, err := service.ConfirmRestore(recovery.RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
			if scenario == "quiesce-failure" {
				if err == nil || db.SQL == nil || !reflect.DeepEqual(current, cloudRows(t, db.SQL)) {
					t.Fatal("failed install changed live session")
				}
				return
			}
			if err != nil || !result.RestartRequired || quiesced != 1 {
				t.Fatalf("confirm %+v %v", result, err)
			}
			journal, err := backup.ReadJournal(live)
			if err != nil || journal.SchemaVersion != 15 {
				t.Fatalf("journal %+v %v", journal, err)
			}
			safety, err := sqlite.OpenReadOnlyForVerify(filepath.Join(filepath.Dir(live), journal.SafetyName))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current, cloudRows(t, safety.SQL)) {
				t.Fatal("safety copy lost original records")
			}
			safety.Close()
			if err := backup.ReconcileOnStartup(live, nil); err != nil {
				t.Fatal(err)
			}
			restored, err := sqlite.Open(live)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			if err := restored.Verify(ctx); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, cloudRows(t, restored.SQL)) {
				t.Fatal("cloud restore/upgrade changed records, values or IDs")
			}
			var alias string
			if err := restored.SQL.QueryRow(`SELECT short_name FROM instruments`).Scan(&alias); err != nil || alias != "" {
				t.Fatalf("alias default %q %v", alias, err)
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("inspection/restore modified source v15 bytes")
			}
			originalIdentity, readErr := os.ReadFile(identityPath)
			if readErr != nil || !bytes.Equal(identity, originalIdentity) {
				t.Fatal("source schema metadata was overwritten")
			}
		})
	}
}
