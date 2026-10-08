package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Real clocks and explicit RFC3339Nano observations must round-trip through
// SQLite's authoritative UTC millisecond timestamps and immutable receipts.
func TestProductHTTPDataTimestampRoundTrip(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 29, 12, 0, 0, 123456789, time.UTC)
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("UTC+08", 8*60*60)} {
		for _, kind := range []string{"terms", "valuation_default", "valuation_explicit_utc", "valuation_explicit_offset"} {
			t.Run(zone.String()+"/"+kind, func(t *testing.T) {
				now := base.In(zone)
				clock := func() time.Time { return now }
				fx, db, _ := newPersistentProductFixture(t, clock)
				c := ledgerSession(t, fx)
				id, _ := productDataSeed(t, fx, c, "locked_product")
				tool := "valuation"
				input := any(ProductValuationInput{id, "1100", ""})
				observed := base
				if kind == "terms" {
					tool = "terms"
					input = productTermsInput(id, 1, "locked_product")
				} else if kind != "valuation_default" {
					observed = base.Add(-time.Hour + 876543*time.Nanosecond)
					location := time.UTC
					if kind == "valuation_explicit_offset" {
						location = time.FixedZone("UTC+08", 8*60*60)
					}
					input = ProductValuationInput{id, "1100", observed.In(location).Format(time.RFC3339Nano)}
				}
				p := productDataPreviewHTTP(t, c, "preview_product_"+tool, input)
				plan := p["planId"].(string)
				now = now.Add(time.Minute + 987654*time.Nanosecond)
				op := uuid.NewString()
				first := productDataCommitHTTP(t, c, "commit_product_"+tool, plan, op)
				assertTime := func(raw any, want time.Time) {
					t.Helper()
					got, err := time.Parse(time.RFC3339Nano, raw.(string))
					if err != nil || !got.Equal(want.UTC().Truncate(time.Millisecond)) {
						t.Fatalf("time %v: %v; want persisted instant %v", raw, err, want)
					}
				}
				assertTime(first["recordedAt"], now)
				if tool == "valuation" {
					assertTime(first["observedAt"], observed)
				}
				verify := func() string {
					t.Helper()
					path := filepath.Join(t.TempDir(), "roundtrip.db")
					if err := db.SnapshotTo(t.Context(), path); err != nil {
						t.Fatal(err)
					}
					checked, err := sqlite.OpenReadOnlyForVerify(path)
					if err != nil {
						t.Fatal("legitimate timestamp rejected by backup verification:", err)
					}
					checked.Close()
					return path
				}
				verify() // Exact current revision before any subsequent mutation.
				var stored string
				if tool == "terms" {
					if err := db.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key LIKE ?`, "product.terms-mutation.%."+plan).Scan(&stored); err != nil {
						t.Fatal(err)
					}
					var m domain.ProductTermsMutation
					if err := json.Unmarshal([]byte(stored), &m); err != nil {
						t.Fatal(err)
					}
					for _, at := range []time.Time{m.CreatedAt, m.Receipt.RecordedAt, m.Receipt.Contract.UpdatedAt, m.Receipt.Policy.UpdatedAt} {
						if at.Location() != time.UTC || at.Nanosecond()%int(time.Millisecond) != 0 {
							t.Fatal("receipt retained an unpersisted timestamp:", at)
						}
					}
				}
				if tool == "valuation" {
					// The reviewed command itself, not only the display, must use the
					// same persisted precision; waiting must not move observation time.
					command := p["preview"].(map[string]any)["command"].(map[string]any)
					if command["observedAt"] != observed.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano) {
						t.Fatal("preview did not freeze the authoritative timestamp:", command)
					}
					if err := db.SQL.QueryRow(`SELECT result_json FROM product_operations WHERE id=?`, plan).Scan(&stored); err != nil {
						t.Fatal(err)
					}
					var result struct {
						Receipt domain.ProductValuationReceipt `json:"receipt"`
					}
					if err := json.Unmarshal([]byte(stored), &result); err != nil {
						t.Fatal(err)
					}
					for _, at := range []time.Time{result.Receipt.ObservedAt, result.Receipt.RecordedAt} {
						if at.Location() != time.UTC || at.Nanosecond()%int(time.Millisecond) != 0 {
							t.Fatal("valuation receipt retained an unpersisted timestamp:", at)
						}
					}
				}
				expected := make(map[string]any, len(first))
				for k, v := range first {
					expected[k] = v
				}
				expected["replayed"] = true
				productJSONEqual(t, productDataCommitHTTP(t, c, "commit_product_"+tool, plan, uuid.NewString()), expected)
				// A later revision / quote must not replace the original result.
				input = ProductValuationInput{id, "1200", ""}
				if tool == "terms" {
					in := productTermsInput(id, 2, "locked_product")
					in.Terms.Name = "Later timestamp revision"
					input = in
				}
				p2 := productDataPreviewHTTP(t, c, "preview_product_"+tool, input)
				now = now.Add(time.Minute + 567891*time.Nanosecond)
				op2 := uuid.NewString()
				fx.service.repository = &failProductSuccessReceipt{ConfigurationRepository: fx.service.repository, fail: true}
				if code := ledgerErrorCode(t, c, "commit_product_"+tool, map[string]any{"operationId": op2, "input": map[string]any{"planId": p2["planId"]}}); code != "operation_outcome_unknown" {
					t.Fatal(code)
				}
				path := verify()
				fx.service.Close()
				// Normal startup, fresh application coordinator, and fresh MCP
				// server recover the committed business evidence before stale plans.
				opened, err := sqlite.Open(path)
				if err != nil {
					t.Fatal("legitimate timestamp rejected by normal startup:", err)
				}
				defer opened.Close()
				repo := sqlite.NewRepository(opened)
				app := application.NewService(repo)
				appports.Wire(app)
				appports.AttachSQLiteHistory(app, repo)
				app.SetClock(clock)
				s := New(app, t.TempDir(), nil, sqlite.NewConfigurationRepository(opened))
				defer s.Close()
				if err := s.Resume(); err != nil {
					t.Fatal(err)
				}
				c = connect(t, s)
				productJSONEqual(t, productDataCommitHTTP(t, c, "commit_product_"+tool, plan, uuid.NewString()), expected)
				recovered := productDataCommitHTTP(t, c, "commit_product_"+tool, p2["planId"].(string), op2)
				assertTime(recovered["recordedAt"], now)
				if recovered["replayed"] != true {
					t.Fatal(recovered)
				}
				if err := app.WithExclusive(t.Context(), application.ExclusiveRestore, func(context.Context) error { return nil }); err != nil {
					t.Fatal(err)
				}
				productJSONEqual(t, productDataCommitHTTP(t, c, "commit_product_"+tool, p2["planId"].(string), uuid.NewString()), recovered)
			})
		}
	}
}

// Older guarded writes used raw clock/observation instants in JSON while SQL
// already stored milliseconds. Compatibility must preserve sealed raw evidence,
// enforce command/receipt binding, and still reject a changed persisted instant.
func TestProductHTTPDataLegacyTimestampEvidence(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"terms", "valuation"} {
		t.Run(tool, func(t *testing.T) {
			zone := time.FixedZone("UTC+08", 8*60*60)
			base := time.Date(2026, 9, 29, 20, 0, 0, 123456789, zone)
			now := base
			fx, db, _ := newPersistentProductFixture(t, func() time.Time { return now })
			c := ledgerSession(t, fx)
			id, _ := productDataSeed(t, fx, c, "locked_product")
			input := any(ProductValuationInput{id, "1100", ""})
			if tool == "terms" {
				input = productTermsInput(id, 1, "locked_product")
			}
			p := productDataPreviewHTTP(t, c, "preview_product_"+tool, input)
			plan := p["planId"].(string)
			now = now.Add(time.Minute + 987654*time.Nanosecond)
			first := productDataCommitHTTP(t, c, "commit_product_"+tool, plan, uuid.NewString())
			var raw, key string
			if tool == "terms" {
				if err := db.SQL.QueryRow(`SELECT key,value FROM app_configuration WHERE key LIKE ?`, "product.terms-mutation.%."+plan).Scan(&key, &raw); err != nil {
					t.Fatal(err)
				}
				var old domain.ProductTermsMutation
				if err := json.Unmarshal([]byte(raw), &old); err != nil {
					t.Fatal(err)
				}
				r := old.Receipt
				r.RecordedAt, r.Contract.UpdatedAt, r.Policy.UpdatedAt = now, now, now
				r.Contract.CreatedAt, r.Policy.CreatedAt = base, base
				if r.Policy.ConfirmedAt != nil {
					r.Policy.ConfirmedAt = &now
				}
				legacy, err := domain.NewProductTermsMutation(old.ID, old.HouseholdID, old.CommandJSON, r, now)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				raw = string(encoded)
				if _, err := db.SQL.Exec(`UPDATE app_configuration SET value=? WHERE key=?`, raw, key); err != nil {
					t.Fatal(err)
				}
			} else {
				var result string
				if err := db.SQL.QueryRow(`SELECT result_json FROM product_operations WHERE id=?`, plan).Scan(&result); err != nil {
					t.Fatal(err)
				}
				var res struct {
					QuoteID string                         `json:"quoteId"`
					Receipt domain.ProductValuationReceipt `json:"receipt"`
				}
				if err := json.Unmarshal([]byte(result), &res); err != nil {
					t.Fatal(err)
				}
				res.Receipt.ObservedAt, res.Receipt.RecordedAt = base, now
				command := application.ProductValuationCommand{ProductID: domain.ProductContractID(id), Amount: "1100", ObservedAt: base.UTC().Format(time.RFC3339Nano)}
				cmd, _ := json.Marshal(command)
				sum := sha256.Sum256(cmd)
				request, _ := json.Marshal(struct {
					Kind    string                              `json:"kind"`
					Command application.ProductValuationCommand `json:"command"`
				}{"value_observation", command})
				encoded, _ := json.Marshal(res)
				raw = string(encoded)
				if _, err := db.SQL.Exec(`UPDATE product_operations SET request_json=?,payload_sha256=?,result_json=? WHERE id=?`, string(request), hex.EncodeToString(sum[:]), raw, plan); err != nil {
					t.Fatal(err)
				}
				// Recover through the original legacy plan command, too.
				productEditPlan(t, fx.service, plan, func(p map[string]any) {
					p["command"] = command
				})
			}
			path := filepath.Join(t.TempDir(), "legacy.db")
			if err := db.SnapshotTo(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			checked, err := sqlite.OpenReadOnlyForVerify(path)
			if err != nil {
				t.Fatal("legacy valid evidence rejected:", err)
			}
			checked.Close()
			opened, err := sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			opened.Close()
			expected := make(map[string]any, len(first))
			for k, v := range first {
				expected[k] = v
			}
			expected["replayed"] = true
			productJSONEqual(t, productDataCommitHTTP(t, c, "commit_product_"+tool, plan, uuid.NewString()), expected)
			var retained string
			if tool == "terms" {
				err = db.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key=?`, key).Scan(&retained)
			} else {
				err = db.SQL.QueryRow(`SELECT result_json FROM product_operations WHERE id=?`, plan).Scan(&retained)
			}
			if err != nil || retained != raw {
				t.Fatal("legacy recovery rewrote immutable evidence:", err)
			}
			if tool == "terms" {
				_, err = db.SQL.Exec(`UPDATE product_contracts SET updated_at=? WHERE id=?`, now.UTC().Truncate(time.Millisecond).Add(time.Millisecond).Format(time.RFC3339Nano), id)
			} else {
				_, err = db.SQL.Exec(`UPDATE instrument_quotes SET quoted_at=? WHERE id=?`, base.UTC().Truncate(time.Millisecond).Add(time.Millisecond).Format(time.RFC3339Nano), first["quoteId"])
			}
			if err != nil {
				t.Fatal(err)
			}
			broken := filepath.Join(t.TempDir(), "changed-time.db")
			if err := db.SnapshotTo(t.Context(), broken); err != nil {
				t.Fatal(err)
			}
			if checked, err := sqlite.OpenReadOnlyForVerify(broken); err == nil {
				checked.Close()
				t.Fatal("changed persisted timestamp accepted")
			}
		})
	}
}

func TestProductHTTPValuationRejectsFutureSubmillisecond(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 29, 12, 0, 0, 123456789, time.UTC)
	fx, _, _ := newPersistentProductFixture(t, func() time.Time { return now })
	c := ledgerSession(t, fx)
	id, _ := productDataSeed(t, fx, c, "locked_product")
	input := ProductValuationInput{id, "1100", now.Add(time.Nanosecond).Format(time.RFC3339Nano)}
	if code := ledgerErrorCode(t, c, "preview_product_valuation", input); code != string(domain.ErrInvalidChangeTime) {
		t.Fatal("normalization admitted a future instant:", code)
	}
}
