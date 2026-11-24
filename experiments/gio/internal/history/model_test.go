package history

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyReaderFunc func(context.Context, domain.ActivityQuery) (domain.ActivityPage, error)

func (f historyReaderFunc) ListActivityPage(ctx context.Context, q domain.ActivityQuery) (domain.ActivityPage, error) {
	return f(ctx, q)
}

func TestHistoryPaginationRetryAndReload(t *testing.T) {
	at := time.Date(2026, time.November, 2, 0, 15, 0, 0, time.UTC)
	note := "Local day before UTC midnight"
	first := domain.Activity{
		ID: domain.NewActivityID(), Kind: domain.ActivityCashIn, EffectiveAt: at, CreatedAt: at.Add(time.Hour),
		EffectiveLocalDate: "2026-11-01", Note: &note,
	}
	second := domain.Activity{ID: domain.NewActivityID()}
	cursor := domain.ActivityCursor{ID: first.ID, EffectiveAt: first.EffectiveAt, CreatedAt: first.CreatedAt}
	failure := errors.New("read failed")
	var queries []domain.ActivityQuery
	reader := historyReaderFunc(func(_ context.Context, q domain.ActivityQuery) (domain.ActivityPage, error) {
		queries = append(queries, q)
		switch len(queries) {
		case 1:
			return domain.ActivityPage{Activities: []domain.Activity{first}, Next: &cursor, HasMore: true}, nil
		case 2, 4:
			return domain.ActivityPage{}, failure
		case 3:
			return domain.ActivityPage{Activities: []domain.Activity{second}}, nil
		case 5:
			return domain.ActivityPage{}, nil
		default:
			return domain.ActivityPage{}, errors.New("unexpected request")
		}
	})
	invalidations := 0
	p := New(t.Context(), reader, func() { invalidations++ })
	if p.visit.started || p.Loading() {
		t.Fatal("constructing the page should not begin a read")
	}
	finish := func(wantReplaced bool) {
		t.Helper()
		if !p.Loading() {
			t.Fatal("expected a read in progress")
		}
		p.Wait()
		if replaced := p.Update(); replaced != wantReplaced {
			t.Fatalf("replaced = %v, want %v", replaced, wantReplaced)
		}
	}
	p.Update()
	p.Reload()
	finish(true)
	if len(queries) != 1 || p.Loading() || p.Err() != nil || !p.HasMore() || len(p.Rows()) != 1 {
		t.Fatalf("first page: calls %d, busy %v, error %v, more %v, rows %d", len(queries), p.Loading(), p.Err(), p.HasMore(), len(p.Rows()))
	}
	if row := p.Rows()[0]; row.ID != first.ID || row.Kind != first.Kind || row.EffectiveLocalDate != first.EffectiveLocalDate || row.Note == nil || *row.Note != note {
		t.Fatalf("first row lost its business date or fields: %+v", row)
	}

	p.LoadMore()
	p.LoadMore()
	finish(false)
	if !errors.Is(p.Err(), failure) || !p.HasMore() || len(p.Rows()) != 1 {
		t.Fatal("append failure lost the rows or cursor")
	}
	p.Retry()
	finish(false)
	if p.Err() != nil || p.HasMore() || len(p.Rows()) != 2 || p.Rows()[0].ID != first.ID || p.Rows()[1].ID != second.ID {
		t.Fatal("retry did not append the older row in service order")
	}
	p.LoadMore()
	if p.Loading() {
		t.Fatal("end of history started another read")
	}
	p.Reload()
	finish(false)
	if !errors.Is(p.Err(), failure) || len(p.Rows()) != 2 {
		t.Fatal("failed refresh discarded the previous list")
	}
	p.Retry()
	finish(true)
	if p.Loading() || p.Err() != nil || p.HasMore() || len(p.Rows()) != 0 {
		t.Fatal("successful empty refresh did not replace all pages")
	}
	want := []domain.ActivityQuery{
		{Limit: 50}, {Limit: 50, After: &cursor}, {Limit: 50, After: &cursor}, {Limit: 50}, {Limit: 50},
	}
	if !reflect.DeepEqual(queries, want) || invalidations != len(want) {
		t.Fatalf("requests %+v, invalidations %d", queries, invalidations)
	}
}

func TestHistoryCancellationReleasesWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{})
	reader := historyReaderFunc(func(ctx context.Context, _ domain.ActivityQuery) (domain.ActivityPage, error) {
		close(entered)
		<-ctx.Done()
		return domain.ActivityPage{}, ctx.Err()
	})
	p := New(ctx, reader, func() { t.Error("a cancelled read invalidated the window") })
	p.Update()
	<-entered
	cancel()
	p.Wait()
	if len(p.pending.result) != 0 {
		t.Fatal("a cancelled read published a result")
	}
}

func TestHistoryFiltersRetireOldPageAndCursor(t *testing.T) {
	type call struct {
		query domain.ActivityQuery
		done  chan pageResultForTest
	}
	calls := make(chan call, 3)
	reader := historyReaderFunc(func(_ context.Context, q domain.ActivityQuery) (domain.ActivityPage, error) {
		next := call{query: q, done: make(chan pageResultForTest, 1)}
		calls <- next
		// Deliberately ignore cancellation to model an already-completed read
		// whose response reaches the UI after a filter switch.
		result := <-next.done
		return result.page, result.err
	})
	updated := make(chan struct{}, 3)
	p := New(t.Context(), reader, func() { updated <- struct{}{} })
	firstID, oldTailID, newID := domain.NewActivityID(), domain.NewActivityID(), domain.NewActivityID()
	cursor := &domain.ActivityCursor{ID: firstID}
	p.Update()
	first := <-calls
	first.done <- pageResultForTest{page: domain.ActivityPage{Activities: []domain.Activity{{ID: firstID}}, HasMore: true, Next: cursor}}
	p.Wait()
	<-updated
	p.Update()
	p.LoadMore()
	old := <-calls
	account, instrument := domain.NewAccountID(), domain.NewInstrumentID()
	scope := Filters{Account: account, Instrument: instrument, Kind: domain.ActivityBuy, From: "2026-11-21", To: "2026-11-21"}
	p.SetFilters(scope)
	current := <-calls
	if old.query.After != cursor || len(p.Rows()) != 0 || p.HasMore() {
		t.Fatal("replacing the filter kept an old row or cursor")
	}
	if want := scope.query(nil); !reflect.DeepEqual(current.query, want) {
		t.Fatalf("new query = %+v, want %+v", current.query, want)
	}
	current.done <- pageResultForTest{page: domain.ActivityPage{Activities: []domain.Activity{{ID: newID}}}}
	<-updated
	if !p.Update() || len(p.Rows()) != 1 || p.Rows()[0].ID != newID {
		t.Fatal("the replacement query did not resolve before the older request")
	}
	old.done <- pageResultForTest{page: domain.ActivityPage{Activities: []domain.Activity{{ID: oldTailID}}}}
	p.Wait()
	if p.Update() || p.Loading() || len(p.Rows()) != 1 || p.Rows()[0].ID != newID || p.Err() != nil {
		t.Fatal("an old page changed the replacement")
	}
	saved := p.Visit()
	p.Restore(Visit{})
	p.Restore(saved)
	if p.Filters() != scope || p.Loading() || len(p.Rows()) != 1 || p.Rows()[0].ID != newID {
		t.Fatal("restoring a settled visit refetched it or lost its filter")
	}
}

type pageResultForTest struct {
	page domain.ActivityPage
	err  error
}
