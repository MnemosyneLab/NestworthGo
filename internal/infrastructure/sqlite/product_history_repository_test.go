package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func seedProductHistory(t *testing.T) (*DB, *Repository, *application.Service, domain.ProductContractID) {
	t.Helper()
	db, repo, _, account, _ := seedPortfolioRepository(t)
	app := application.NewService(repo)
	now := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	if _, err := app.AppendAccountCashValue(t.Context(), account.ID, "1000", "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	maturity, zero, calendar := "2026-12-20", 0, "calendar"
	shortName := "History"
	cmd := application.ProductCommand{Kind: domain.ProductOpOpen, Open: &application.OpenProductCommand{AccountID: account.ID.String(), Currency: "USD", Principal: "100", Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic", ShortName: &shortName, StartOn: "2026-09-20", MaturityOn: &maturity, InterestMode: "none"}, Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, SettlementDays: &zero, DayBasis: &calendar, EarlyKind: "not_allowed"}}}
	preview, err := app.PreviewProductOperation(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := app.RecordProductOperation(t.Context(), cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	return db, repo, app, receipt.ProductIDs[0]
}

func appendHistoryValuations(t *testing.T, db *DB, householdID domain.HouseholdID, productID domain.ProductContractID, count int) []domain.ProductOperationID {
	t.Helper()
	ids := []domain.ProductOperationID{}
	stamp := formatTimestamp(time.Date(2026, 9, 21, 4, 0, 0, 0, time.UTC))
	for i := 0; i < count; i++ {
		id := domain.ProductOperationID(fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1))
		// Intentionally reverse the effective dates: only creation/UUID order owns paging.
		effective := fmt.Sprintf("2026-09-%02dT04:00:00Z", 20-i%10)
		if _, err := db.SQL.Exec(`INSERT INTO product_operations(id,household_id,kind,payload_sha256,request_version,request_json,result_json,effective_at,created_at) VALUES(?,?,'value_observation',?,1,'{}','{}',?,?)`, id.String(), householdID.String(), fmt.Sprintf("%064d", i), effective, stamp); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SQL.Exec(`INSERT INTO product_operation_products(operation_id,product_id,role) VALUES(?,?,'valued')`, id.String(), productID.String()); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

type productHistorySQLProbe struct {
	queryer
	calls      int
	beforePage func()
	failAt     int
}

func (p *productHistorySQLProbe) QueryContext(ctx context.Context, s string, args ...any) (*sql.Rows, error) {
	p.calls++
	if p.calls == p.failAt {
		return nil, errors.New("synthetic history failure")
	}
	if p.beforePage != nil {
		f := p.beforePage
		p.beforePage = nil
		f()
	}
	return p.queryer.QueryContext(ctx, s, args...)
}
func (p *productHistorySQLProbe) QueryRowContext(ctx context.Context, s string, args ...any) *sql.Row {
	p.calls++
	return p.queryer.QueryRowContext(ctx, s, args...)
}

func TestProductHistorySnapshotBoundedPagesOrderingAndGlobalMetadata(t *testing.T) {
	for _, count := range []int{0, 25} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, repo, _, product := seedProductHistory(t)
			contract, err := repo.Product(t.Context(), mustHistoryHousehold(t, repo), product)
			if err != nil {
				t.Fatal(err)
			}
			ids := appendHistoryValuations(t, db, contract.HouseholdID, product, count)
			cursor := ""
			seen := map[domain.ProductOperationID]bool{}
			var previous domain.ProductOperationID
			for {
				page, err := repo.ReadProductOperationHistorySnapshot(t.Context(), product, 26, cursor)
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Rows) > 26 {
					t.Fatal("unbounded page")
				}
				if count > 0 && (page.Candidate == nil || page.Candidate.ID != ids[len(ids)-1] || page.UndoEvidence != nil) {
					t.Fatal("global valuation candidate or hydration incorrect")
				}
				rows := page.Rows
				more := len(rows) > 25
				if more {
					rows = rows[:25]
				}
				for _, row := range rows {
					if seen[row.ID] {
						t.Fatal("duplicate")
					}
					seen[row.ID] = true
					if row.Kind == domain.ProductOpValueObservation {
						if previous != "" && row.ID >= previous {
							t.Fatal("equal-time UUID order changed")
						}
						previous = row.ID
					}
				}
				if !more {
					break
				}
				last := rows[len(rows)-1]
				cursor = last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID.String()
			}
			if len(seen) != count+1 {
				t.Fatalf("lost rows %d", len(seen))
			}
			if count == 0 {
				// Empty successful page remains a non-nil array, with an off-page global target.
				exhausted, err := repo.ReadProductOperationHistorySnapshot(t.Context(), product, 25, "2000-01-01T00:00:00Z|"+domain.NewProductOperationID().String())
				if err != nil {
					t.Fatal(err)
				}
				if exhausted.Rows == nil || len(exhausted.Rows) != 0 || exhausted.Candidate == nil || exhausted.UndoEvidence == nil {
					t.Fatal("exhausted page lost global metadata")
				}
				if exhausted.UndoEvidence.Contracts[product].ShortName != "History" {
					t.Fatal("short-name projection lost")
				}
			}
		})
	}
}
func mustHistoryHousehold(t *testing.T, repo *Repository) domain.HouseholdID {
	t.Helper()
	snapshot, err := repo.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Household.ID
}

func TestProductHistorySnapshotBatchCountFailureAndScope(t *testing.T) {
	db, repo, _, product := seedProductHistory(t)
	household := mustHistoryHousehold(t, repo)
	var expected int
	for _, n := range []int{1, 101} {
		tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		probe := &productHistorySQLProbe{queryer: tx}
		snapshot, err := readProductOperationHistoryQuery(t.Context(), probe, product, n, "")
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.UndoEvidence == nil || len(snapshot.UndoEvidence.Activities) != 1 {
			t.Fatal("activity evidence missing")
		}
		if expected == 0 {
			expected = probe.calls
		} else if probe.calls != expected {
			t.Fatal("SQL count scales with page size")
		}
		tx.Rollback()
	}
	t.Logf("lifecycle snapshot SQL statements: %d", expected)
	// More than a hundred reversed rows still require the same fixed statement count.
	ids := appendHistoryValuations(t, db, household, product, 105)
	for i, id := range ids {
		reverseID := domain.NewProductOperationID()
		if _, err := db.SQL.Exec(`INSERT INTO product_operations(id,household_id,kind,payload_sha256,request_version,request_json,result_json,effective_at,created_at,reverses_operation_id) VALUES(?,?,'undo',?,1,'{}','{}','2026-09-22T04:00:00.000Z','2026-09-22T04:00:00.000Z',?)`, reverseID.String(), household.String(), fmt.Sprintf("%064d", i), id.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SQL.Exec(`INSERT INTO product_operation_products(operation_id,product_id,role) VALUES(?,?,'valued')`, reverseID.String(), product.String()); err != nil {
			t.Fatal(err)
		}
	}
	longTx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	longProbe := &productHistorySQLProbe{queryer: longTx}
	longSnapshot, err := readProductOperationHistoryQuery(t.Context(), longProbe, product, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if longProbe.calls != expected || len(longSnapshot.UndoEvidence.Histories[product]) != 211 {
		t.Fatalf("long evidence: calls=%d rows=%d", longProbe.calls, len(longSnapshot.UndoEvidence.Histories[product]))
	}
	longTx.Rollback()

	for _, id := range []domain.ProductContractID{domain.NewProductContractID(), domain.ProductContractID("00000000-0000-4000-8000-000000000000")} {
		page, err := repo.ReadProductOperationHistorySnapshot(t.Context(), id, 25, "")
		var de *domain.Error
		if !errors.As(err, &de) || de.Code != domain.ErrNotFound || page.Candidate != nil {
			t.Fatalf("scope failure %+v %v", page, err)
		}
	}
	if _, err := repo.ReadProductUndoEvidence(t.Context(), domain.NewHouseholdID(), domain.NewProductOperationID()); err == nil {
		t.Fatal("foreign operation found")
	}
	if _, err := repo.ReadProductOperationHistorySnapshot(t.Context(), product, 25, "bad-cursor"); err == nil {
		t.Fatal("bad cursor accepted")
	}
	tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	probe := &productHistorySQLProbe{queryer: tx, failAt: 7}
	snapshot, err := readProductOperationHistoryQuery(t.Context(), probe, product, 25, "")
	if err == nil || snapshot.Candidate != nil || snapshot.Rows != nil {
		t.Fatalf("partial failed read: %+v %v", snapshot, err)
	}
	tx.Rollback()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := repo.ReadProductOperationHistorySnapshot(cancelled, product, 25, ""); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	if _, err := repo.ReadProductOperationHistorySnapshot(t.Context(), product, 25, ""); err != nil {
		t.Fatal("transaction leaked", err)
	}
	// Inspect the existing indexes; no schema change is required.
	rows, err := db.SQL.Query(`EXPLAIN QUERY PLAN SELECT o.id FROM product_operations o JOIN product_operation_products p ON p.operation_id=o.id WHERE o.household_id=? AND p.product_id=? AND NOT EXISTS (SELECT 1 FROM product_operations r WHERE r.reverses_operation_id=o.id AND r.household_id=o.household_id) ORDER BY o.created_at DESC,o.id DESC LIMIT 1`, household.String(), product.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		t.Log(detail)
	}
}

func TestProductHistorySnapshotInterleavingSeparateConnection(t *testing.T) {
	db, repo, app, product := seedProductHistory(t)
	// Open a second live connection to the same synthetic WAL database.
	var path string
	rows, err := db.SQL.Query(`PRAGMA database_list`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int
		var name, p string
		if err := rows.Scan(&n, &name, &p); err != nil {
			t.Fatal(err)
		}
		if name == "main" {
			path = p
		}
	}
	rows.Close()
	if !filepath.IsAbs(path) {
		t.Fatal(path)
	}
	writer, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1&_pragma=busy_timeout%3d5000")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	initial, err := repo.ReadProductOperationHistorySnapshot(t.Context(), product, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	contract := initial.UndoEvidence.Contracts[product]
	tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	reached, resume := make(chan struct{}), make(chan struct{})
	probe := &productHistorySQLProbe{queryer: tx, beforePage: func() { close(reached); <-resume }}
	type outcome struct {
		snapshot domain.ProductOperationHistorySnapshot
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		snap, err := readProductOperationHistoryQuery(t.Context(), probe, product, 26, "")
		done <- outcome{snap, err}
	}()
	<-reached // Household and product reads established the SQLite snapshot.
	newID := domain.NewProductOperationID()
	wtx, err := writer.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wtx.Exec(`INSERT INTO product_operations(id,household_id,kind,payload_sha256,request_version,request_json,result_json,effective_at,created_at) VALUES(?,?,'value_observation',?,1,'{}','{}','2026-09-21T04:00:00Z','2026-09-21T04:00:00Z')`, newID.String(), contract.HouseholdID.String(), fmt.Sprintf("%064d", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err = wtx.Exec(`INSERT INTO product_operation_products(operation_id,product_id,role) VALUES(?,?,'valued')`, newID.String(), product.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = wtx.Exec(`UPDATE product_contracts SET revision=revision+1 WHERE id=?`, product.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = wtx.Exec(`UPDATE account_cash_values SET amount='0' WHERE account_id=?`, contract.AccountID.String()); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(); err != nil {
		t.Fatal(err)
	}
	close(resume)
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.snapshot.Candidate.ID != initial.Candidate.ID || len(result.snapshot.Rows) != 1 || result.snapshot.UndoEvidence.Contracts[product].Revision != contract.Revision {
		t.Fatal("mixed snapshot")
	}
	state := result.snapshot.Portfolio.CashValues
	if len(state) == 0 || state[0].Amount.CanonicalAmount() == "0" {
		t.Fatal("financial state mixed with later writer")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	next, err := app.ListProductOperations(t.Context(), product, "", 25)
	if err != nil {
		t.Fatal(err)
	}
	if next.Undo.Allowed || next.Undo.Operation.ID != newID || *next.Undo.Reason != application.UndoValuationRequiresCorrection {
		t.Fatal("new snapshot missed writer")
	}
}
