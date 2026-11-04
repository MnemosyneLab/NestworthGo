package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func appendAuditQuote(t *testing.T, r *Repository, h domain.Household, i domain.Instrument, at time.Time, on, op, target string) domain.InstrumentQuote {
	t.Helper()
	price, _ := domain.ParseUnitPrice("100.1256789")
	sourceDay, err := time.Parse(time.DateOnly, on)
	if err != nil {
		t.Fatal(err)
	}
	q, err := domain.NewInstrumentQuote(i, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: sourceDay.Add(12 * time.Hour)}, at)
	if err != nil {
		t.Fatal(err)
	}
	q.ObservationKind = "close"
	q.EffectiveDate = on
	q.PriceBasis = "agent_raw_close_v1"
	q.ValueEffectiveAt = q.QuotedAt
	q.TimestampBasis = "source_timestamp"
	q.SourcePolicyVersion = "agent_supplied_v1"
	_, err = r.ImportAgentQuoteBatch(t.Context(), domain.AgentQuoteBatch{HouseholdID: h.ID, RequestKey: q.ID.String(), CreatedAt: at, Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteOperation(op), TargetQuoteID: target, InstrumentQuote: &q, SourceTitle: "Synthetic audit"}}})
	if err != nil {
		t.Fatal(err)
	}
	return q
}
func TestAgentAuditPushdownFullHistoryInvalidationAndTimeBounds(t *testing.T) {
	db, r, h, _, i := seedPortfolioRepository(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	a := appendAuditQuote(t, r, h, i, at, "2026-09-26", "append", "")
	b := appendAuditQuote(t, r, h, i, at.Add(time.Hour), "2026-09-27", "correct", a.ID.String())
	c := appendAuditQuote(t, r, h, i, at.Add(2*time.Hour), "2026-09-28", "correct", b.ID.String())
	_, err := r.ImportAgentQuoteBatch(t.Context(), domain.AgentQuoteBatch{HouseholdID: h.ID, RequestKey: "retraction", CreatedAt: at.Add(3 * time.Hour), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteRetract, TargetQuoteID: c.ID.String(), SourceTitle: "Synthetic later withdrawal"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		on     string
		count  int
		target string
	}{{"2026-09-26", 1, a.ID.String()}, {"2026-09-27", 1, b.ID.String()}, {"2026-09-28", 2, c.ID.String()}} {
		out, err := r.ReadAgentAudit(domain.WithBoundedRead(t.Context()), h.ID, domain.AgentAuditFilter{InstrumentIDs: []string{i.ID.String()}, FromDate: tc.on, ToDate: tc.on})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Records) != tc.count || !out.Inactive[tc.target] {
			t.Fatalf("date=%s out=%+v", tc.on, out)
		}
		if tc.count == 2 && (len(out.Identities) != 1 || out.Identities[0].InstrumentQuote.ID != c.ID) {
			t.Fatal("withdrawal identity is not immediate target")
		}
	}
	// Bounds are instants, not time-zone strings or truncated lower bounds.
	offset := at.In(time.FixedZone("offset", -7*3600))
	after := at.Add(time.Nanosecond)
	for _, tc := range []struct {
		from, to *time.Time
		count    int
	}{{&offset, &offset, 1}, {&after, &after, 0}, {nil, &after, 1}, {&after, nil, 3}} {
		out, err := r.ReadAgentAudit(t.Context(), h.ID, domain.AgentAuditFilter{CreatedFrom: tc.from, CreatedTo: tc.to})
		if err != nil || len(out.Records) != tc.count {
			t.Fatalf("instant bounds: count=%d want=%d err=%v", len(out.Records), tc.count, err)
		}
	}
	wrong := domain.AgentAuditFilter{InstrumentIDs: []string{domain.NewInstrumentID().String()}}
	out, err := r.ReadAgentAudit(t.Context(), h.ID, wrong)
	if err != nil || len(out.Records) != 0 {
		t.Fatal(out, err)
	}
	out, err = r.ReadAgentAudit(t.Context(), domain.NewHouseholdID(), domain.AgentAuditFilter{})
	if err != nil || len(out.Records) != 0 {
		t.Fatal("cross household audit", err)
	}
	// Enforce read-only at SQLite, and cancellation must never publish a result.
	if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ReadAgentAudit(domain.WithBoundedRead(t.Context()), h.ID, domain.AgentAuditFilter{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = r.ReadAgentAudit(ctx, h.ID, domain.AgentAuditFilter{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled read", err)
	}
}

// The meter executes diagnostic COUNT subqueries before each production SQL
// call. Reported calls exclude those diagnostics; rows are actual SQL result
// rows, not estimates, VM visits, storage pages, or wire bytes.
type pushdownMeter struct {
	queryer
	calls, rows int
	plans       []string
	t           *testing.T
}

func (q *pushdownMeter) measure(ctx context.Context, s string, args []any) {
	q.calls++
	var n int
	if err := q.queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+s+")", args...).Scan(&n); err != nil {
		q.t.Fatal(err)
	}
	q.rows += n
	if strings.Contains(s, "WITH matched") {
		rows, err := q.queryer.QueryContext(ctx, "EXPLAIN QUERY PLAN "+s, args...)
		if err != nil {
			q.t.Fatal(err)
		}
		for rows.Next() {
			var a, b, c int
			var d string
			if err := rows.Scan(&a, &b, &c, &d); err != nil {
				q.t.Fatal(err)
			}
			q.plans = append(q.plans, d)
		}
		rows.Close()
	}
}
func (q *pushdownMeter) QueryContext(ctx context.Context, s string, args ...any) (*sql.Rows, error) {
	q.measure(ctx, s, args)
	return q.queryer.QueryContext(ctx, s, args...)
}
func (q *pushdownMeter) QueryRowContext(ctx context.Context, s string, args ...any) *sql.Row {
	q.measure(ctx, s, args)
	return q.queryer.QueryRowContext(ctx, s, args...)
}

func testPushdownScale(t *testing.T, n int) {
	db, r, h, a, i := seedPortfolioRepository(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	selected := appendAuditQuote(t, r, h, i, at, "2026-09-28", "append", "")
	other := i
	other.ID = domain.NewInstrumentID()
	other.Name = "Unrelated synthetic instrument"
	if err := r.CreateInstrument(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	otherQuote := appendAuditQuote(t, r, h, other, at, "2026-09-27", "append", "")
	// Direct synthetic setup avoids measuring application write/rebuild cost.
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		for k := 0; k < n; k++ {
			unrelatedAccount := a
			unrelatedAccount.ID = domain.NewAccountID()
			unrelatedAccount.Name = fmt.Sprintf("Synthetic unrelated %d", k)
			_, err := tx.ExecContext(t.Context(), `INSERT INTO accounts(id,household_id,institution_id,group_id,name,account_type,balance_sheet_role,tracking_mode,default_currency,note,icon_key,include_in_net_worth,include_in_portfolio,include_in_liquid_assets,opened_on,closed_on,sort_order,created_at,updated_at,archived_at) SELECT ?,household_id,institution_id,group_id,?,account_type,balance_sheet_role,tracking_mode,default_currency,note,icon_key,include_in_net_worth,include_in_portfolio,include_in_liquid_assets,opened_on,closed_on,sort_order,created_at,updated_at,archived_at FROM accounts WHERE id=?`, unrelatedAccount.ID.String(), unrelatedAccount.Name, a.ID.String())
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(t.Context(), `INSERT INTO account_ownership SELECT ?,member_id,share_bps FROM account_ownership WHERE account_id=?`, unrelatedAccount.ID.String(), a.ID.String()); err != nil {
				return err
			}
			unrelatedInstrument := other
			unrelatedInstrument.ID = domain.NewInstrumentID()
			unrelatedInstrument.Name = fmt.Sprintf("Synthetic unrelated %d", k)
			if err = insertInstrumentTx(t.Context(), tx, unrelatedInstrument); err != nil {
				return err
			}
			quantity, _ := domain.ParseQuantity("2")
			holding, err := domain.NewHoldingForAccount(unrelatedAccount, unrelatedInstrument, quantity, nil, 0, at)
			if err != nil {
				return err
			}
			if err = insertHolding(t.Context(), tx, holding); err != nil {
				return err
			}

			q := otherQuote
			q.InstrumentID = unrelatedInstrument.ID
			q.ID = domain.NewInstrumentQuoteID()
			if err := appendInstrumentQuoteTx(t.Context(), tx, q); err != nil {
				return err
			}
			_, err = tx.ExecContext(t.Context(), `INSERT INTO agent_quote_records SELECT ?,request_key,household_id,target_type,operation,?,NULL,source_title,source_url,created_at FROM agent_quote_records WHERE quote_id=?`, fmt.Sprintf("00000000-0000-4000-a000-%012d", k), q.ID.String(), otherQuote.ID.String())
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	old := &pushdownMeter{queryer: db.SQL, t: t}
	all, err := listAgentQuoteRecordsQuery(t.Context(), old, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	meter := &pushdownMeter{queryer: db.SQL, t: t}
	out, err := readAgentAuditQuery(domain.WithBoundedRead(t.Context()), meter, h.ID, domain.AgentAuditFilter{InstrumentIDs: []string{i.ID.String()}, FromDate: "2026-09-28", ToDate: "2026-09-28"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n+2 || len(out.Records) != 1 || out.Records[0].InstrumentQuote.ID != selected.ID {
		t.Fatal("selected audit changed")
	}
	plan := strings.Join(meter.plans, "\n")
	if !strings.Contains(plan, "ux_agent_quote_records_quote") || !strings.Contains(plan, "idx_instrument_quotes_latest") {
		t.Fatal("target index path not activated", plan)
	}
	if meter.rows > 3 || meter.calls > 4 {
		t.Fatal("scoped audit expanded unrelated rows", meter.rows, meter.calls)
	}
	t.Logf("audit unrelated=%d legacy result rows=%d calls=%d; scoped result rows=%d calls=%d", n, old.rows, old.calls, meter.rows, meter.calls)
	// A scoped portfolio must match the same account in the full input snapshot.
	quantity, _ := domain.ParseQuantity("2")
	holding, err := domain.NewHoldingForAccount(a, i, quantity, nil, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.CreateHolding(t.Context(), holding); err != nil {
		t.Fatal(err)
	}
	fullMeter := &pushdownMeter{queryer: db.SQL, t: t}
	full, err := readPortfolioSnapshotQuery(t.Context(), fullMeter, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	narrowMeter := &pushdownMeter{queryer: db.SQL, t: t}
	narrow, err := readPortfolioSnapshotQuery(domain.WithBoundedRead(t.Context()), narrowMeter, domain.AccountFilter{AccountIDs: []domain.AccountID{a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	v := application.NewValuationService(r, func() time.Time { return at })
	want, _, err := v.ValueAccountCompositions(full)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := v.ValueAccountCompositions(narrow)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range want {
		if value.Account.ID == a.ID {
			want = []domain.AccountValuation{value}
			break
		}
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("target valuation differs from full snapshot")
	}
	if narrowMeter.rows >= fullMeter.rows || narrowMeter.calls > 25 {
		t.Fatal("scoped portfolio expanded unrelated inputs", narrowMeter.rows, narrowMeter.calls)
	}
	t.Logf("account unrelated=%d legacy result rows=%d calls=%d; scoped result rows=%d calls=%d", n, fullMeter.rows, fullMeter.calls, narrowMeter.rows, narrowMeter.calls)

	if len(narrow.Instruments) != 1 || len(narrow.InstrumentQuotes) != 1 {
		t.Fatal("unrelated instrument inputs decoded")
	}
}
func TestTargetReadPushdownSmallScale(t *testing.T) { testPushdownScale(t, 30) }

func TestAgentAuditScopedFXClosureAndBudget(t *testing.T) {
	db, r, h, _, i := seedPortfolioRepository(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	// A closure date is a source civil label, even where UTC is the prior day.
	loc, _ := time.LoadLocation("Pacific/Kiritimati")
	day := time.Date(2026, 9, 28, 0, 0, 0, 0, loc)
	closure := domain.InstrumentClosure{ID: domain.NewInstrumentQuoteID().String(), InstrumentID: i.ID, Date: "2026-09-28", EffectiveAt: day.UTC(), Timezone: loc.String(), CreatedAt: at}
	if _, err := r.ImportAgentQuoteBatch(t.Context(), domain.AgentQuoteBatch{HouseholdID: h.ID, RequestKey: "closure", CreatedAt: at, Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteAppend, Closure: &closure, SourceTitle: "Synthetic closure"}}}); err != nil {
		t.Fatal(err)
	}
	rate, _ := domain.ParseFxRate("7.12345678")
	q, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: h.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at.Add(-24 * time.Hour)}, at)
	if err != nil {
		t.Fatal(err)
	}
	q.EffectiveDate = "2026-09-28"
	q.ObservationKind = "daily_reference"
	if _, err = r.ImportAgentQuoteBatch(t.Context(), domain.AgentQuoteBatch{HouseholdID: h.ID, RequestKey: "fx", CreatedAt: at, Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteAppend, FXQuote: &q, SourceTitle: "Synthetic FX"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ImportAgentQuoteBatch(t.Context(), domain.AgentQuoteBatch{HouseholdID: h.ID, RequestKey: "fx-withdraw", CreatedAt: at.Add(time.Hour), Records: []domain.AgentQuoteRecord{{Operation: domain.AgentQuoteRetract, TargetQuoteID: q.ID.String(), SourceTitle: "Synthetic FX withdrawal"}}}); err != nil {
		t.Fatal(err)
	}
	f := domain.AgentAuditFilter{FXPairs: []domain.MarketReadTarget{{BaseCurrency: "USD", QuoteCurrency: "CNY"}}, FromDate: "2026-09-28", ToDate: "2026-09-28"}
	out, err := r.ReadAgentAudit(domain.WithBoundedRead(t.Context()), h.ID, f)
	if err != nil || len(out.Records) != 2 || !out.Inactive[q.ID.String()] || len(out.Identities) != 1 {
		t.Fatal(out, err)
	}
	f.FXPairs = []domain.MarketReadTarget{{BaseCurrency: "CNY", QuoteCurrency: "USD"}}
	out, err = r.ReadAgentAudit(t.Context(), h.ID, f)
	if err != nil || len(out.Records) != 0 {
		t.Fatal("audit FX orientation changed", out, err)
	}
	f.FXPairs = nil
	f.InstrumentIDs = []string{i.ID.String()}
	out, err = r.ReadAgentAudit(t.Context(), h.ID, f)
	if err != nil || len(out.Records) != 1 || out.Records[0].Closure.Date != "2026-09-28" {
		t.Fatal("closure source date changed", out, err)
	}
	// Oversized irrelevant provenance must not consume a target's budget; it must
	// still fail before expansion when that provenance is selected.
	if _, err = db.SQL.ExecContext(t.Context(), `UPDATE agent_quote_records SET source_title=printf('%.*c',?,'x') WHERE quote_id=?`, 4<<20, q.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ReadAgentAudit(domain.WithBoundedRead(t.Context()), h.ID, f); err != nil {
		t.Fatal("unrelated provenance consumed scoped budget", err)
	}
	_, err = r.ReadAgentAudit(domain.WithBoundedRead(t.Context()), h.ID, domain.AgentAuditFilter{})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "too_large" {
		t.Fatal("oversized audit did not fail closed", err)
	}
}
