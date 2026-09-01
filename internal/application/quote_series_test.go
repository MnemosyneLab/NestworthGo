package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestInstrumentQuoteSeriesClipsRangeAndKeepsDeterministicPoints(t *testing.T) {
	service, ctx, _, setClock := newOnboardedService(t, "quote-series-range", []string{"Owner"})
	setClock(time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "400", "2026-07-01T00:00:00.000Z", false); err != nil {
		t.Fatalf("old quote: %v", err)
	}
	first, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "410", "2026-08-20T10:00:00.000Z", false)
	if err != nil {
		t.Fatalf("first quote: %v", err)
	}
	second, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "411", "2026-08-20T10:00:00.000Z", false)
	if err != nil {
		t.Fatalf("same-time quote: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "420", "2026-08-21T15:00:00.000Z", false); err != nil {
		t.Fatalf("next-day quote: %v", err)
	}

	series, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err != nil {
		t.Fatalf("InstrumentQuoteSeries: %v", err)
	}
	if series.DisplayCurrency != domain.CurrencyCode("USD") {
		t.Fatalf("DisplayCurrency = %q, want USD", series.DisplayCurrency)
	}
	if len(series.Observations) != 3 {
		t.Fatalf("30d observations = %d, want 3 (July quote is outside the window)", len(series.Observations))
	}
	if len(series.Points) != 2 {
		t.Fatalf("30d points = %+v, want one point per quotedAt", series.Points)
	}
	if series.Points[0].QuotedAt.Format(time.RFC3339) != first.QuotedAt.Format(time.RFC3339) {
		t.Fatalf("first point quotedAt = %s", series.Points[0].QuotedAt)
	}
	if series.Points[0].Value != "411" || series.Points[0].ID != second.ID.String() {
		t.Fatalf("same-time chart point = %+v, want latest created quote 411", series.Points[0])
	}
	if series.Observations[0].Value != "420" || series.Observations[1].Value != "411" || series.Observations[2].Value != "410" {
		t.Fatalf("observations should keep every same-time fact, newest first: %+v", series.Observations)
	}

	yearSeries, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.TrendOneYear, domain.QuoteSourceFilterAll)
	if err != nil {
		t.Fatalf("1y series: %v", err)
	}
	if len(yearSeries.Observations) != 4 {
		t.Fatalf("1y observations = %d, want all four facts", len(yearSeries.Observations))
	}
	if len(yearSeries.Points) != 3 {
		t.Fatalf("1y points = %+v, want one point per local date", yearSeries.Points)
	}
	if yearSeries.Points[1].Value != "411" {
		t.Fatalf("daily last on 2026-08-20 = %+v, want 411", yearSeries.Points[1])
	}
}

func TestInstrumentQuoteSeriesSourceFilterAndEmptyRange(t *testing.T) {
	service, ctx, _, setClock := newOnboardedService(t, "quote-series-source", []string{"Owner"})
	setClock(time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: "yahoo_finance", ProviderSymbol: "AAPL"})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-08-20T00:00:00.000Z", false); err != nil {
		t.Fatalf("manual quote: %v", err)
	}
	providerQuote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{
		UnitPrice: quoteSeriesUnitPrice(t, "101"), Currency: domain.CurrencyCode("USD"),
		SourceKind: domain.QuoteSourceProvider, SourceKey: "yahoo_finance", QuotedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
	}, time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("provider quote: %v", err)
	}
	if _, err := service.repository.AppendProviderInstrumentQuoteIfChanged(ctx, providerQuote); err != nil {
		t.Fatalf("append provider: %v", err)
	}

	manual, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.Trend30Days, domain.QuoteSourceFilterManual)
	if err != nil || len(manual.Points) != 1 || manual.Points[0].Value != "100" || manual.Points[0].SourceKind != domain.QuoteSourceManual {
		t.Fatalf("manual series = %+v err=%v", manual, err)
	}
	provider, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.Trend30Days, domain.QuoteSourceFilterProvider)
	if err != nil || len(provider.Points) != 1 || provider.Points[0].Value != "101" {
		t.Fatalf("provider series = %+v err=%v", provider, err)
	}

	setClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	empty, err := service.InstrumentQuoteSeries(ctx, instrument.ID, domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err != nil || len(empty.Points) != 0 || len(empty.Observations) != 0 {
		t.Fatalf("out-of-range series = %+v err=%v", empty, err)
	}
	if !empty.OutsideRange {
		t.Fatal("OutsideRange should be true when local facts exist outside 30d")
	}

	fresh, err := service.CreateInstrument(ctx, InstrumentInput{Name: "NOHIST", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("fresh instrument: %v", err)
	}
	never, err := service.InstrumentQuoteSeries(ctx, fresh.ID, domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err != nil || never.OutsideRange {
		t.Fatalf("never-quoted series = %+v err=%v, want OutsideRange false", never, err)
	}
}

func TestFXQuoteSeriesInvertsStoredOrientation(t *testing.T) {
	service, ctx, _, setClock := newOnboardedService(t, "fx-series", []string{"Owner"})
	setClock(time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-08-20T00:00:00.000Z"); err != nil {
		t.Fatalf("fx quote: %v", err)
	}

	direct, err := service.FXQuoteSeries(ctx, domain.CurrencyCode("USD"), domain.CurrencyCode("CNY"), domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err != nil {
		t.Fatalf("direct series: %v", err)
	}
	if direct.BaseCurrency != "USD" || direct.QuoteCurrency != "CNY" {
		t.Fatalf("direction = %s/%s", direct.BaseCurrency, direct.QuoteCurrency)
	}
	if len(direct.Points) != 1 || direct.Points[0].Value != "7" {
		t.Fatalf("direct points = %+v, want 7", direct.Points)
	}

	inverted, err := service.FXQuoteSeries(ctx, domain.CurrencyCode("CNY"), domain.CurrencyCode("USD"), domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err != nil {
		t.Fatalf("inverted series: %v", err)
	}
	if inverted.BaseCurrency != "CNY" || inverted.QuoteCurrency != "USD" {
		t.Fatalf("inverted direction = %s/%s", inverted.BaseCurrency, inverted.QuoteCurrency)
	}
	if len(inverted.Points) != 1 || inverted.Points[0].Value != "0.142857142857" {
		t.Fatalf("inverted value = %q, want Go reciprocal of 7", inverted.Points[0].Value)
	}
	if inverted.Observations[0].Value != inverted.Points[0].Value {
		t.Fatalf("table must use the same Go reciprocal as the chart")
	}
}

func TestFXQuoteSeriesRejectsSameCurrency(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "fx-series-same", []string{"Owner"})
	_, err := service.FXQuoteSeries(ctx, domain.CurrencyCode("USD"), domain.CurrencyCode("USD"), domain.Trend30Days, domain.QuoteSourceFilterAll)
	if err == nil {
		t.Fatal("FXQuoteSeries accepted USD/USD")
	}
}

func TestPortfolioTrendSumsInstrumentItemsAndExcludesCash(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "portfolio-trend", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "800",
	}); err != nil {
		t.Fatalf("bank: %v", err)
	}
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("brokerage: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerage.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatalf("holding: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "100", "2026-08-01", false); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "100"}); err != nil {
		t.Fatalf("StartHistoryWithCosts: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))

	wealth, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if len(wealth.Points) < 2 {
		t.Fatalf("wealth points = %+v", wealth.Points)
	}
	if wealth.Points[0].Assets == nil || wealth.Points[0].Liabilities == nil || wealth.Points[0].NetWorth == nil {
		t.Fatalf("closed-day wealth point missing series: %+v", wealth.Points[0])
	}
	if wealth.Start == nil || wealth.End == nil || wealth.Change == nil {
		t.Fatalf("wealth summary missing: %+v", wealth)
	}

	portfolio, err := service.PortfolioTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("PortfolioTrend: %v", err)
	}
	if len(portfolio.Points) < 2 {
		t.Fatalf("portfolio points = %+v", portfolio.Points)
	}
	if portfolio.Points[0].ValuedSubtotal == nil || portfolio.Points[0].ValuedSubtotal.CanonicalAmount() != "200" {
		t.Fatalf("portfolio closed day = %+v, want holdings-only 200 (cash 800 excluded)", portfolio.Points[0])
	}
	if wealth.Points[0].NetWorth != nil && wealth.Points[0].NetWorth.CanonicalAmount() == portfolio.Points[0].ValuedSubtotal.CanonicalAmount() {
		t.Fatal("portfolio trend must not equal net worth when cash exists")
	}
}

func quoteSeriesUnitPrice(t *testing.T, value string) domain.UnitPrice {
	t.Helper()
	price, err := domain.ParseUnitPrice(value)
	if err != nil {
		t.Fatalf("ParseUnitPrice(%s): %v", value, err)
	}
	return price
}
