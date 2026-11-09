package mcpserver

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Portable reproduction: this test also runs unchanged on the pre-change main.
type pushdownRepositorySpy struct {
	*sqlite.Repository
	portfolioAccounts, portfolioHoldings, portfolioInstruments, audits int
}

func (r *pushdownRepositorySpy) ReadPortfolioSnapshot(ctx context.Context, f domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	p, err := r.Repository.ReadPortfolioSnapshot(ctx, f)
	r.portfolioAccounts += len(p.Accounts)
	r.portfolioHoldings += len(p.Holdings)
	r.portfolioInstruments += len(p.Instruments)
	return p, err
}
func (r *pushdownRepositorySpy) ListAgentQuoteRecords(ctx context.Context, h domain.HouseholdID) ([]domain.AgentQuoteRecord, error) {
	rows, err := r.Repository.ListAgentQuoteRecords(ctx, h)
	r.audits += len(rows)
	return rows, err
}
func TestPushdownRequestEvidence(t *testing.T) { testPushdownRequestEvidence(t, 0) }
func testPushdownRequestEvidence(t *testing.T, extra int) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &pushdownRepositorySpy{Repository: sqlite.NewRepository(db)}
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo.Repository)
	fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	members, err := app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	// 40 irrelevant accounts and 40 irrelevant quotes; target has one observation.
	for n := 0; n < 40; n++ {
		_, err := app.CreateAccount(t.Context(), application.AccountInput{Name: fmt.Sprintf("Synthetic irrelevant %d", n), AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	items := []application.AgentMarketDataItem{{InstrumentID: fx.instrument, Currency: "USD", Kind: "close", Date: "2026-09-28", Value: "1.12345678", SourceTitle: "Synthetic target"}}
	for n := 0; n < 40; n++ {
		i, err := app.CreateInstrument(t.Context(), application.InstrumentInput{Name: fmt.Sprintf("Synthetic irrelevant %d", n), Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, application.AgentMarketDataItem{InstrumentID: i.ID.String(), Currency: "USD", Kind: "close", Date: "2026-09-27", Value: "1", SourceTitle: "Synthetic unrelated"})
	}
	if _, err = app.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), application.AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	if extra > 0 {
		records, err := app.AgentMarketDataRecords(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var template domain.AgentQuoteRecord
		for _, record := range records {
			if record.InstrumentQuote != nil && record.InstrumentQuote.InstrumentID.String() == fx.instrument {
				template = record
				break
			}
		}
		err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
			accountStmt := syntheticCloneStatement(t, tx, "accounts", []string{"id"})
			defer accountStmt.Close()
			instrumentStmt := syntheticCloneStatement(t, tx, "instruments", []string{"id"})
			defer instrumentStmt.Close()
			quoteStmt := syntheticCloneStatement(t, tx, "instrument_quotes", []string{"id", "instrument_id"})
			defer quoteStmt.Close()
			auditStmt := syntheticCloneStatement(t, tx, "agent_quote_records", []string{"id", "quote_id"})
			defer auditStmt.Close()
			for range extra {
				iid, qid := domain.NewInstrumentID().String(), domain.NewInstrumentQuoteID().String()
				if _, err := accountStmt.ExecContext(t.Context(), domain.NewAccountID().String(), fx.checking); err != nil {
					return err
				}
				if _, err := instrumentStmt.ExecContext(t.Context(), iid, fx.instrument); err != nil {
					return err
				}
				if _, err := quoteStmt.ExecContext(t.Context(), qid, iid, template.InstrumentQuote.ID.String()); err != nil {
					return err
				}
				if _, err := auditStmt.ExecContext(t.Context(), domain.NewActivityID().String(), qid, template.RecordID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	c := ledgerSession(t, fx)
	repo.portfolioAccounts, repo.portfolioHoldings, repo.portfolioInstruments, repo.audits = 0, 0, 0, 0
	for _, request := range []struct {
		name string
		args any
	}{{"get_account_valuations", AccountValuationReadInput{AccountIDs: []string{fx.checking}}}, {"list_agent_market_data", map[string]any{"filter": map[string]any{"instrumentIds": []string{fx.instrument}, "fromDate": "2026-09-28", "toDate": "2026-09-28"}}}} {
		result, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: request.name, Arguments: request.args})
		if err != nil || result.IsError {
			t.Fatal(result, err)
		}
		raw, err := jsoncodec.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 64<<10 {
			t.Fatalf("wire budget=%d", len(raw))
		}
		t.Logf("%s complete MCP wire=%d bytes tool calls=1", request.name, len(raw))
	}
	t.Logf("repository expansion: portfolio accounts=%d holdings=%d instruments=%d legacy audit rows=%d", repo.portfolioAccounts, repo.portfolioHoldings, repo.portfolioInstruments, repo.audits)
}

func TestAgentAuditScopedCaptureRetainsOutsideInvalidators(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	first := mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "1.12567891", "kind": "close", "date": "2026-09-27", "sourceTitle": "Synthetic original"}}})
	original := first["quoteIds"].([]any)[0]
	args := map[string]any{"limit": 1, "filter": map[string]any{"instrumentIds": []string{fx.instrument}, "operations": []string{"append"}, "fromDate": "2026-09-27", "toDate": "2026-09-27", "createdTo": "2026-09-29T12:00:00.000000001Z"}}
	frozen := scaleCapture(t, c, "list_agent_market_data", args)
	fx.app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC) })
	fixed := mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "correct", "targetQuoteId": original, "instrumentId": fx.instrument, "currency": "USD", "value": "2", "kind": "close", "date": "2026-09-28", "sourceTitle": "Synthetic next-date correction"}}})
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": fixed["quoteIds"].([]any)[0], "sourceTitle": "Synthetic withdrawal"}}})
	after := scaleCapture(t, c, "list_agent_market_data", args)
	rows := scaleSection(t, c, after, "records")
	if len(rows) != 1 || rows[0].(map[string]any)["active"] != false {
		t.Fatal("out-of-filter invalidator lost", rows)
	}
	oldRows := scaleSection(t, c, frozen, "records")
	if len(oldRows) != 1 || oldRows[0].(map[string]any)["active"] != true {
		t.Fatal("old capture mutated", oldRows)
	}
	// Scope filters never expand credential permissions. A mode change expires
	// previously issued pages/captures rather than silently reusing them.
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_read_item", Arguments: ReadItemInput{CaptureID: after["captureId"].(string), Section: "records", ID: rows[0].(map[string]any)["recordId"].(string)}}); err == nil {
		t.Fatal("rotated credential reused capture")
	}
	readOnly := connect(t, fx.service)
	listed, err := listAllTools(t.Context(), readOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "import_market_data" {
			t.Fatal("read_only expanded write permission")
		}
	}
	current := scaleCapture(t, readOnly, "list_agent_market_data", args)
	if len(scaleSection(t, readOnly, current, "records")) != 1 {
		t.Fatal("read_only target read changed")
	}
}

// Clone only isolated synthetic rows. Deriving the projection from SQLite's
// columns keeps this perf fixture independent of optional/default column order.
func syntheticCloneStatement(t *testing.T, tx *sql.Tx, table string, replaced []string) *sql.Stmt {
	t.Helper()
	rows, err := tx.QueryContext(t.Context(), "SELECT name FROM pragma_table_info(?) ORDER BY cid", table)
	if err != nil {
		t.Fatal(err)
	}
	columns := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		column := `"` + name + `"`
		for _, replace := range replaced {
			if name == replace {
				column = "?"
			}
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	stmt, err := tx.PrepareContext(t.Context(), "INSERT INTO "+table+" SELECT "+strings.Join(columns, ",")+" FROM "+table+" WHERE id=?")
	if err != nil {
		t.Fatal(err)
	}
	return stmt
}
