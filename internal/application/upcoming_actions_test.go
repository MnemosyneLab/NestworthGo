package application

import (
	"context"
	"errors"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// An unimplemented repository call panics: the endpoint can only use this read.
type upcomingSnapshotSpy struct {
	Repository
	snapshot domain.LiquiditySnapshot
	reads    atomic.Int64
	err      error
}

func (r *upcomingSnapshotSpy) ReadLiquiditySnapshot(context.Context) (domain.LiquiditySnapshot, error) {
	r.reads.Add(1)
	return r.snapshot, r.err
}
func TestUpcomingActionsOneSnapshotAndFrozenClock(t *testing.T) {
	base, ctx, _, _ := openRegressionProduct(t)
	snapshot, err := base.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	spy := &upcomingSnapshotSpy{snapshot: snapshot}
	s := NewService(spy)
	first := time.Date(2026, 12, 19, 23, 59, 59, 0, time.UTC)
	var calls atomic.Int64
	s.SetClock(func() time.Time { return first.Add(time.Duration(calls.Add(1)-1) * 24 * time.Hour) })
	got, err := s.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || spy.reads.Load() != 1 || got.AsOf != first || got.LocalDate != "2026-12-19" {
		t.Fatalf("clock=%d reads=%d result=%+v", calls.Load(), spy.reads.Load(), got)
	}
	if s.valuation.now().Equal(first) {
		t.Fatal("shared valuation clock was frozen")
	}
	s.SetClock(func() time.Time { return first })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := s.UpcomingActions(ctx)
			if e != nil || !reflect.DeepEqual(a, got) {
				t.Errorf("concurrent projection differs: %v", e)
			}
		}()
	}
	wg.Wait()
	sentinel := errors.New("read failed")
	spy.err = sentinel
	if _, err := s.UpcomingActions(ctx); !errors.Is(err, sentinel) {
		t.Fatal("swallowed snapshot error")
	}
}
func TestUpcomingOverviewRegressionCompleteOutput(t *testing.T) {
	s, ctx, _, _ := openRegressionProduct(t)
	snapshot, e := s.repository.ReadLiquiditySnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	sources, e := s.liquiditySources(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	old, e := domain.EvaluateLiquidity(domain.LiquidityQuery{AsOf: s.clock(), Timezone: timezoneOrUTC(snapshot.Portfolio.Origin), BaseCurrency: snapshot.Portfolio.Household.BaseCurrency}, sources, snapshot.Reservations, func(amount decimal.Decimal, currency domain.CurrencyCode) (*decimal.Decimal, bool, error) {
		return s.valuation.ConvertExactAmount(snapshot.Portfolio, amount, currency)
	})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if e != nil {
		t.Fatal(e)
	}
	a, _ := jsoncodec.Marshal(old)
	b, _ := jsoncodec.Marshal(got)
	if string(a) != string(b) {
		t.Fatal("existing Overview output changed")
	}
}
func TestUpcomingActionsLifecycle(t *testing.T) {
	s, ctx, _, receipt := openRegressionProduct(t)
	s.SetClock(func() time.Time { return time.Date(2026, 12, 21, 5, 0, 0, 0, time.UTC) })
	read := func() domain.UpcomingActions {
		t.Helper()
		out, e := s.UpcomingActions(ctx)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	first := read()
	if first.Counts.ReceiptDue != 1 {
		t.Fatalf("due=%+v", first)
	}
	identity := first.Items[0].ID
	product := receipt.ProductIDs[0].String()
	settle := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: product, ReturnedPrincipal: strPtr("90000"), Interest: strPtr("0"), Fee: strPtr("0")}}
	if _, e := s.PreviewProductOperation(ctx, settle); e != nil {
		t.Fatal(e)
	}
	if read().Counts.ReceiptDue != 1 {
		t.Fatal("preview completed item")
	}
	if _, e := s.RecordProductOperation(ctx, settle, domain.NewProductOperationID().String(), "invalid-review"); e == nil {
		t.Fatal("expected guarded refusal")
	}
	if read().Counts.ReceiptDue != 1 {
		t.Fatal("failed write completed item")
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: product, Amount: "100"}})
	interest := read()
	if interest.Counts.ReceiptDue != 1 || interest.Items[0].ID != identity {
		t.Fatal("interest completed item")
	}
	settled := recordRegressionProduct(t, s, ctx, settle)
	if read().Counts.ReceiptDue != 0 {
		t.Fatal("settlement did not remove item")
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settled.OperationID.String()}})
	undone := read()
	if undone.Counts.ReceiptDue != 1 || undone.Items[0].ID != identity {
		t.Fatal("undo did not restore identity")
	}
}

func TestUpcomingActionsRenewalUndo(t *testing.T) {
	s, ctx, _, opened := openRegressionProduct(t)
	s.SetClock(func() time.Time { return time.Date(2026, 12, 21, 5, 0, 0, 0, time.UTC) })
	before, err := s.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cmd := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{
		Settle:    SettleProductCommand{ProductID: opened.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("0")},
		Principal: "100000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic successor", StartOn: "2026-12-21", MaturityOn: strPtr("2027-01-10"), InterestMode: "none"}, Policy: depositPolicyForMaturity("2027-01-10"),
	}}
	renewed := recordRegressionProduct(t, s, ctx, cmd)
	after, err := s.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 1 || after.Items[0].ID == before.Items[0].ID || after.Items[0].Phase != "upcoming" {
		t.Fatalf("renewal=%+v", after)
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: renewed.OperationID.String()}})
	undone, err := s.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(undone.Items) != 1 || undone.Items[0].ID != before.Items[0].ID || undone.Items[0].Phase != "receipt_due" {
		t.Fatalf("undo=%+v", undone)
	}
}

func TestUpcomingActionsArchivedInstrumentRetainsHeldProduct(t *testing.T) {
	base, ctx, _, _ := openRegressionProduct(t)
	snapshot, err := base.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 12, 19, 4, 0, 0, 0, time.UTC)
	for i := range snapshot.Portfolio.Instruments {
		snapshot.Portfolio.Instruments[i].ArchivedAt = &now
	}
	s := NewService(&upcomingSnapshotSpy{snapshot: snapshot})
	s.SetClock(func() time.Time { return now })
	got, err := s.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.Scheduled != 1 {
		t.Fatalf("held archived instrument omitted: %+v", got)
	}
}

func TestUpcomingActionsCapturesValuationConfigurationOnce(t *testing.T) {
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	var providers, ttls int
	s.valuation.SetFXProviderKey(func() string { providers++; return "frankfurter" })
	s.valuation.SetQuoteCacheTTL(func() time.Duration { ttls++; return 12 * time.Hour })
	if _, err := s.UpcomingActions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if providers != 1 || ttls != 1 {
		t.Fatalf("provider=%d ttl=%d", providers, ttls)
	}
}

func TestUpcomingActionsFreshnessUsesCapturedInstant(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	snapshot, err := fixture.repository.ReadLiquiditySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instant := fixture.now
	for i := range snapshot.Portfolio.FXPreferences {
		snapshot.Portfolio.FXPreferences[i].SourceKind = domain.QuoteSourceProvider
	}
	for i := range snapshot.Portfolio.FXQuotes {
		q := &snapshot.Portfolio.FXQuotes[i]
		q.SourceKind = domain.QuoteSourceProvider
		q.SourceKey = "frankfurter"
		q.QuotedAt = instant.Add(-12*time.Hour + time.Nanosecond)
	}
	s := NewService(&upcomingSnapshotSpy{snapshot: snapshot})
	var clocks int
	s.SetClock(func() time.Time { clocks++; return instant.Add(time.Duration(clocks-1) * time.Second) })
	s.valuation.SetFXProviderKey(func() string { return "frankfurter" })
	s.valuation.SetQuoteCacheTTL(func() time.Duration { return 12 * time.Hour })
	overview, _, sources, err := s.prepareLiquidity(context.Background(), LiquidityOverviewQuery{}, s.clock())
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, source := range sources {
		if source.FXEvidence != nil {
			seen++
			if source.FXEvidence.Freshness != domain.FreshnessFresh {
				t.Fatalf("mixed valuation instant: %+v", source.FXEvidence)
			}
		}
	}
	if clocks != 1 || seen == 0 || !overview.AsOf.Equal(instant) {
		t.Fatalf("clock=%d evidence=%d asOf=%v", clocks, seen, overview.AsOf)
	}
}
