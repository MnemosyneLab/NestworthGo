package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type mobileBlockingProvider struct {
	*refreshFakeProvider
	entered, release chan struct{}
	once             sync.Once
}

func (p *mobileBlockingProvider) LatestInstrument(ctx context.Context, id InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	p.once.Do(func() { close(p.entered) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return LatestInstrumentQuote{}, ctx.Err()
	}
	return p.refreshFakeProvider.LatestInstrument(ctx, id)
}

func TestMobileMarketJobsDedupeAcrossEntrypointsAndKeepPartialResults(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "mobile-market", []string{"Owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	fake := newRefreshFakeProvider()
	price, err := domain.ParseUnitPrice("12.3456789")
	if err != nil {
		t.Fatal(err)
	}
	fake.instruments["AAA"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "CNY", SourceKey: "fake", QuotedAt: now.Add(-24 * time.Hour)}}
	fake.instruments["BBB"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{err: &domain.Error{Code: domain.ErrProviderRateLimit}}
	provider := &mobileBlockingProvider{refreshFakeProvider: fake, entered: make(chan struct{}), release: make(chan struct{})}
	s.SetMarketDataRegistry(NewMarketDataRegistryWithDefault("fake", provider))
	var first domain.Instrument
	for _, symbol := range []string{"AAA", "BBB"} {
		key := "fake"
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic " + symbol, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: key, ProviderSymbol: symbol})
		if err != nil {
			t.Fatal(err)
		}
		if symbol == "AAA" {
			first = i
		}
	}
	request := MobileMarketRequest{Mode: "latest", Scope: "all"}
	job, attached, conflict, err := s.StartMobileMarket(ctx, request)
	if err != nil || attached || conflict {
		t.Fatalf("start %v %v %v", attached, conflict, err)
	}
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not entered")
	}
	duplicate, attached, conflict, err := s.StartMobileMarket(ctx, request)
	if err != nil || !attached || conflict || duplicate.ID != job.ID {
		t.Fatalf("attach %v %v %v", attached, conflict, err)
	}
	_, _, conflict, err = s.StartMobileMarket(ctx, MobileMarketRequest{Mode: "latest", Scope: "stale"})
	if err != nil || !conflict {
		t.Fatal("different scope was queued")
	}
	direct, err := s.RefreshInstrument(ctx, first.ID)
	if err != nil || len(direct.Items) != 1 || direct.Items[0].ErrorCode != domain.ErrConflict {
		t.Fatalf("desktop bypass %+v %v", direct, err)
	}
	started, err := s.StartMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: first.ID.String()})
	if err != nil || !started.Conflict {
		t.Fatalf("sync bypass %+v %v", started, err)
	}
	close(provider.release)
	s.mobileJobsWG.Wait()
	finished, ok := s.MobileMarketJob(job.ID)
	if !ok || finished.Outcome != SyncOutcomePartial {
		t.Fatalf("outcome %+v", finished)
	}
	quotes, err := s.repository.ListInstrumentQuotes(ctx, first.ID)
	if err != nil || len(quotes) != 1 || !quotes[0].QuotedAt.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("actual effective time lost: %v %+v", err, quotes)
	}
	fresh := NewService(s.repository)
	if _, ok := fresh.MobileMarketJob(job.ID); ok {
		t.Fatal("restart fabricated job status")
	}
}
