package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type reversalQueryProbe struct {
	queryer
	calls, maxArgs int
	cancel         context.CancelFunc
	failAt         int
}

func (p *reversalQueryProbe) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	p.calls++
	p.maxArgs = max(p.maxArgs, len(args))
	if p.calls == p.failAt {
		return nil, errors.New("synthetic query failure")
	}
	rows, err := p.queryer.QueryContext(ctx, statement, args...)
	if p.cancel != nil {
		p.cancel()
	}
	return rows, err
}

func TestActivitiesWithReversalsSQLCounts(t *testing.T) {
	db, _, household, _, _ := seedPortfolioRepository(t)
	ids := make([]domain.ActivityID, reversalReadChunkSize+1)
	for i := range ids {
		ids[i] = domain.NewActivityID()
	}
	// Reversals outside any visible page/date filter, including the second
	// chunk.
	for _, source := range []domain.ActivityID{ids[0], ids[500]} {
		if _, err := db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cash_in','other','2019-01-01T00:00:00Z','2019-01-01','2019-01-01T00:00:00Z')`, source.String(), household.ID.String()); err != nil {
			t.Fatal(err)
		}
		_, err := db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at,reverses_activity_id) VALUES(?,?,'cash_out','other','2020-01-01T00:00:00Z','2020-01-01','2020-01-01T00:00:00Z',?)`, domain.NewActivityID().String(), household.ID.String(), source.String())
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []int{0, reversalReadChunkSize, reversalReadChunkSize + 1} {
		p := &reversalQueryProbe{queryer: db.SQL}
		input := append([]domain.ActivityID{}, ids[:n]...)
		if n > 0 {
			input = append(input, ids[0])
		}
		got, err := activitiesWithReversals(t.Context(), p, household.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		wantCalls := (n + 499) / 500
		if p.calls != wantCalls || p.maxArgs > 501 {
			t.Fatalf("n=%d queries=%d args=%d", n, p.calls, p.maxArgs)
		}
		wantResults := 0
		if n > 0 {
			wantResults = 1
		}
		if n > 500 {
			wantResults = 2
		}
		if len(got) != wantResults || (n > 0 && !got[ids[0]]) || (n > 500 && !got[ids[500]]) {
			t.Fatalf("reversals=%v", got)
		}
		t.Logf("%d distinct IDs: %d actual SQLite SELECTs", n, p.calls)
	}
	p := &reversalQueryProbe{queryer: db.SQL}
	got, err := activitiesWithReversals(t.Context(), p, domain.NewHouseholdID(), ids[:100])
	if err != nil || len(got) != 0 {
		t.Fatalf("household isolation: %v %v", got, err)
	}
	for _, failAt := range []int{1, 2} {
		p := &reversalQueryProbe{queryer: db.SQL, failAt: failAt}
		got, err := activitiesWithReversals(t.Context(), p, household.ID, ids)
		if err == nil || got != nil {
			t.Fatalf("failure returned optimistic result: %v %v", got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	p = &reversalQueryProbe{queryer: db.SQL, cancel: cancel}
	got, err = activitiesWithReversals(ctx, p, household.ID, ids)
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("cancel: %v %v", got, err)
	}
}
