package application

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestSourceTrackingProviderRefreshEvidence(t *testing.T) {
	if os.Getenv("NW_SOURCE_BATCH_PERF") != "1" {
		t.Skip("synthetic end-to-end provider refresh; run alone")
	}
	db, service, fake, account, _ := newRefreshFixture(t)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	price, err := domain.ParseUnitPrice("100.0001")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 100; i++ {
		symbol := fmt.Sprintf("SYNTHETIC%d", i)
		instrument, err := service.CreateInstrument(t.Context(), InstrumentInput{Name: symbol, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: symbol})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.CreateHolding(t.Context(), HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
			t.Fatal(err)
		}
		fake.instruments[symbol] = struct {
			quote LatestInstrumentQuote
			err   error
		}{quote: LatestInstrumentQuote{Price: price, Currency: "CNY", SourceKey: "fake", QuotedAt: now}}
	}
	for repeat := 0; repeat < 2; repeat++ {
		if err = db.CheckpointWAL(t.Context()); err != nil {
			t.Fatal(err)
		}
		var beforeChanges, beforeRevision int64
		if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&beforeChanges); err != nil {
			t.Fatal(err)
		}
		if err = db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&beforeRevision); err != nil {
			t.Fatal(err)
		}
		beforeInfo, err := os.Stat(db.Path)
		if err != nil {
			t.Fatal(err)
		}
		began := time.Now()
		result, err := service.RefreshAll(t.Context())
		elapsed := time.Since(began)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus := RefreshFetched
		if repeat == 1 {
			wantStatus = RefreshCached
		}
		if len(result.Items) != 101 || result.RateLimited {
			t.Fatalf("incomplete refresh: %+v", result)
		}
		for _, item := range result.Items {
			if item.Status != wantStatus {
				t.Fatalf("unexpected refresh result: %+v", item)
			}
		}
		var changes, revision, floor, rows, quotes int64
		if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changes); err != nil {
			t.Fatal(err)
		}
		if err = db.SQL.QueryRow(`SELECT revision,journal_floor,(SELECT count(*) FROM financial_source_changes),(SELECT count(*) FROM instrument_quotes) FROM financial_source_revision`).Scan(&revision, &floor, &rows, &quotes); err != nil {
			t.Fatal(err)
		}
		wal := int64(0)
		if info, e := os.Stat(db.Path + "-wal"); e == nil {
			wal = info.Size()
		}
		if err = db.CheckpointWAL(t.Context()); err != nil {
			t.Fatal(err)
		}
		afterInfo, err := os.Stat(db.Path)
		if err != nil {
			t.Fatal(err)
		}
		if rows > 4351 || rows != revision-floor || quotes != 100 {
			t.Fatalf("refresh facts/tracking invalid: %+v quotes=%d retained=%d", result, quotes, rows)
		}
		if repeat == 1 && revision != beforeRevision {
			t.Fatal("cached refresh changed source revision")
		}
		if err = db.Verify(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Logf("batch=service-refresh-100-repeat-%d sample=single elapsed_ns=%d targets=100-instrument+1-FX providerCallsCumulative=%d totalChangedRows=%d revisionDelta=%d floor=%d retainedRows=%d quotes=%d WALBeforeCheckpoint=%d DBBefore=%d DBAfter=%d DBGrowth=%d items=%d status=%s", repeat, elapsed.Nanoseconds(), fake.callCount(), changes-beforeChanges, revision-beforeRevision, floor, rows, quotes, wal, beforeInfo.Size(), afterInfo.Size(), afterInfo.Size()-beforeInfo.Size(), len(result.Items), wantStatus)
	}
}
