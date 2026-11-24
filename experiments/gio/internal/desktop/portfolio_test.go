package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	historyui "github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	portapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

type portfolioFixture struct {
	account     domain.AccountID
	instruments map[string]domain.Instrument
}

func seedPortfolio(ctx context.Context, service *application.Service, clock time.Time, mode string) (ids portfolioFixture, err error) {
	ids.instruments = make(map[string]domain.Instrument)
	early := clock.Add(-48 * time.Hour)
	service.SetClock(func() time.Time { return early })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic holdings review", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		return ids, err
	}
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		return ids, err
	}
	createAccount := func(name string) (domain.AccountRecord, error) {
		return service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
			DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true,
		})
	}
	broker, err := createAccount("Harbor Brokerage")
	if err != nil {
		return ids, err
	}
	ids.account = broker.Account.ID
	if _, err = service.AppendAccountCashValue(ctx, ids.account, "100", "USD", ""); err != nil {
		return ids, err
	}
	if _, err = service.StartHistory(ctx, "Asia/Singapore"); err != nil {
		return ids, err
	}
	service.SetClock(func() time.Time { return clock })
	if mode == "empty" {
		return ids, nil
	}
	second, err := createAccount("Second Brokerage")
	if err != nil {
		return ids, err
	}
	quotedAt := clock.Add(-time.Hour).Format(time.RFC3339)
	if _, err = service.SetFXPreference(ctx, "USD", "EUR", "manual"); err != nil {
		return ids, err
	}
	if _, err = service.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", quotedAt); err != nil {
		return ids, err
	}
	scenarios := []struct {
		name, symbol, currency, quantity, unitCost, price string
	}{
		{"Complete Fund", "QA-FULL", "EUR", "2.5", "20.12", "30.1234"},
		{"Price Gap Fund", "QA-PX", "EUR", "2", "4", ""},
		{"FX Gap Fund", "QA-FX", "GBP", "1.25", "7.3", "10.4"},
		{"Both Gaps Fund", "QA-BOTH", "CHF", "1", "6.5", ""},
	}
	if mode == "complete" {
		scenarios = scenarios[:1]
	}
	for _, item := range scenarios {
		instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
			Name: item.name, Symbol: item.symbol, Type: "etf", QuoteCurrency: item.currency, QuoteSource: "manual",
		})
		if err != nil {
			return ids, err
		}
		ids.instruments[item.symbol] = instrument
		_, err = service.CreateHolding(ctx, application.HoldingInput{
			AccountID: ids.account.String(), InstrumentID: instrument.ID.String(), Quantity: item.quantity, UnitCost: item.unitCost,
		})
		if err != nil {
			return ids, err
		}
		if item.symbol == "QA-FULL" {
			if _, err := service.CreateHolding(ctx, application.HoldingInput{
				AccountID: second.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0.5", UnitCost: item.unitCost,
			}); err != nil {
				return ids, err
			}
		}
		if item.price != "" {
			if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, item.price, quotedAt, false); err != nil {
				return ids, err
			}
		}
	}
	return ids, nil
}

func openPortfolioFixture(t *testing.T, mode string) (*application.Service, *pilot.Session, pilot.Paths, portfolioFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	service, session := reopenOnboardingProfile(t, paths)
	ids, err := seedPortfolio(t.Context(), service, accountTestClock, mode)
	if err != nil {
		t.Fatal(err)
	}
	return service, session, paths, ids
}

func TestPortfolioAndInstrumentReadsMatchWailsGo(t *testing.T) {
	ctx := t.Context()
	for _, mode := range []string{"complete", "partial", "empty"} {
		t.Run(mode, func(t *testing.T) {
			gio, _, _, gioIDs := openPortfolioFixture(t, mode)
			wails, _, _, wailsIDs := openPortfolioFixture(t, mode)
			gl, err := readPortfolio(ctx, gio, "")
			if err != nil {
				t.Fatal(err)
			}
			adapter := portapi.NewService(wails)
			wp, err := adapter.Portfolio(ctx, account.AccountFilterRequest{})
			if err != nil {
				t.Fatal(err)
			}
			gover, err := gio.Overview(ctx, domain.AccountFilter{})
			if err != nil {
				t.Fatal(err)
			}
			wo, err := adapter.Overview(ctx, account.AccountFilterRequest{})
			if err != nil {
				t.Fatal(err)
			}
			quality := gl.summary.DisplayValuation.Quality
			net := gover.DisplayValuation.NetWorth
			value := gl.summary.ValuedSubtotal
			wantPortfolio, wantNet, wantQuality := "108.4442", "208.44424", domain.ValuationFull
			if mode == "partial" {
				wantQuality = domain.ValuationPartial
			}
			if mode == "empty" {
				wantPortfolio, wantNet = "0", "100"
			}
			if value == nil || wp.ValuedSubtotal == nil || value.Amount != wantPortfolio || wp.ValuedSubtotal.Amount != wantPortfolio ||
				quality.Availability != wantQuality || wantQuality != wp.DisplayValuation.Quality.Availability ||
				net.Quality.Availability != wantQuality || wantQuality != wo.DisplayValuation.NetWorth.Quality.Availability ||
				gover.NetWorth.String() != wo.NetWorth || gover.NetWorth.String() != wantNet {
				t.Fatalf("unexpected Portfolio/NetWorth amounts or coverage: %+v / %+v; %s / %s", value, wp.DisplayValuation, gover.NetWorth, wo.NetWorth)
			}
			if mode == "partial" {
				display := gl.summary.DisplayValuation
				if display.Total != nil || wp.DisplayValuation.Total != nil || net.Total != nil || wo.DisplayValuation.NetWorth.Total != nil ||
					display.KnownSubtotal == nil || display.KnownSubtotal.Amount != wantPortfolio || net.KnownSubtotal == nil ||
					wp.DisplayValuation.KnownSubtotal == nil || wp.DisplayValuation.KnownSubtotal.Amount != wantPortfolio ||
					wo.DisplayValuation.NetWorth.KnownSubtotal == nil || net.KnownSubtotal.Amount != wo.DisplayValuation.NetWorth.KnownSubtotal.Amount ||
					!slices.Equal(quality.Reasons, wp.DisplayValuation.Quality.Reasons) || !slices.Equal(net.Quality.Reasons, wo.DisplayValuation.NetWorth.Quality.Reasons) {
					t.Fatal("partial market inputs were not kept separate from available subtotals")
				}
			}
			wgroups, err := analytics.NewService(wails).InstrumentHoldingGroups(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wrows := make(map[string]analytics.InstrumentHoldingsDTO)
			for _, group := range wgroups {
				for _, row := range group.Instruments {
					wrows[row.Symbol] = row
				}
			}
			if len(wrows) != len(gl.positions) || len(gl.positions) != len(gioIDs.instruments) {
				t.Fatalf("unexpected portfolio instrument count: Gio %d, Wails %d", len(gl.positions), len(wrows))
			}
			for _, item := range []struct{ symbol, quantity, cost, native, price, fx string }{
				{"QA-FULL", "3", "60.36", "90.3702", "30.1234", "1.2"},
				{"QA-PX", "2", "8", "", "", "1.2"},
				{"QA-FX", "1.25", "9.125", "13", "10.4", ""},
				{"QA-BOTH", "1", "6.5", "", "", ""},
			} {
				i, ok := gioIDs.instruments[item.symbol]
				if !ok {
					continue
				}
				index := slices.IndexFunc(gl.positions, func(row domain.InstrumentHoldingsView) bool { return row.InstrumentID == i.ID })
				if index < 0 {
					t.Fatalf("Portfolio is missing instrument %s", item.symbol)
				}
				gr := gl.positions[index]
				wr := wrows[item.symbol]
				detail, err := readPortfolio(ctx, gio, i.ID)
				if err != nil {
					t.Fatal(err)
				}
				if detail.instrument == nil || detail.instrument.ID != i.ID || len(detail.positions) != 1 || detail.positions[0].InstrumentID != i.ID {
					t.Fatalf("instrument detail resolved the wrong object: %+v", detail)
				}
				if i.ID.String() == wr.InstrumentID {
					t.Fatal("the Wails check must use an independent synthetic profile")
				}
				got := detail.positions[0].Amounts
				amount := func(m *domain.MoneyView) string {
					if m == nil {
						return ""
					}
					if m.Currency != i.QuoteCurrency {
						t.Fatalf("wrong currency: %+v", m)
					}
					return m.Amount
				}
				if got.Quantity != item.quantity || gr.Amounts.Quantity != got.Quantity || wr.Amounts.Quantity != got.Quantity ||
					amount(got.TotalCost) != item.cost || amount(gr.Amounts.TotalCost) != item.cost || wr.Amounts.TotalCost == nil || wr.Amounts.TotalCost.Amount != item.cost ||
					amount(got.CurrentValue) != item.native || amount(gr.Amounts.CurrentValue) != item.native ||
					(item.native == "" && wr.Amounts.CurrentValue != nil) ||
					(item.native != "" && (wr.Amounts.CurrentValue == nil || wr.Amounts.CurrentValue.Amount != item.native || wr.Amounts.CurrentValue.Currency != wr.QuoteCurrency)) {
					t.Fatalf("%s holdings: %+v / %+v", item.symbol, got, wr.Amounts)
				}
				api := quoteapi.NewService(wails)
				price, pe := api.CurrentInstrumentQuote(ctx, wailsIDs.instruments[item.symbol].ID.String())
				fx, fe := api.CurrentFXQuote(ctx, string(i.QuoteCurrency), "USD")
				if pe != nil || fe != nil {
					t.Fatalf("Wails quote reads: %v / %v", pe, fe)
				}
				if item.price == "" {
					if price != nil || detail.price != nil {
						t.Fatal("missing price was replaced")
					}
				} else if price == nil || detail.price == nil || price.UnitPrice != item.price || detail.price.UnitPrice.Canonical() != item.price {
					t.Fatalf("wrong price: %+v / %+v", price, detail.price)
				}
				if item.fx == "" {
					if fx != nil || detail.fx != nil {
						t.Fatal("missing FX was replaced")
					}
				} else if fx == nil || detail.fx == nil || fx.Rate != item.fx || detail.fx.Rate.Canonical() != item.fx ||
					fx.BaseCurrency != "EUR" || detail.fx.BaseCurrency != "EUR" || fx.QuoteCurrency != "USD" || detail.fx.QuoteCurrency != "USD" {
					t.Fatalf("wrong FX: %+v / %+v", fx, detail.fx)
				}
			}
		})
	}
}

type portfolioReadProbe struct {
	*application.Service
	badPrice         atomic.Bool
	holdPrice        atomic.Bool
	started, release chan struct{}
}

func (p *portfolioReadProbe) CurrentInstrumentQuote(ctx context.Context, id domain.InstrumentID) (*domain.InstrumentQuote, error) {
	if p.badPrice.Swap(false) {
		return nil, errors.New("synthetic quote read error")
	}
	return p.Service.CurrentInstrumentQuote(ctx, id)
}

func (p *portfolioReadProbe) WithRead(ctx context.Context, read func(context.Context) error) error {
	err := p.Service.WithRead(ctx, read)
	if p.holdPrice.Swap(false) {
		close(p.started)
		<-p.release
	}
	return err
}

func TestPortfolioShellRoundTripsExactIDsAndReadStates(t *testing.T) {
	ctx := t.Context()
	service, session, paths, ids := openPortfolioFixture(t, "partial")
	probe := &portfolioReadProbe{Service: service, started: make(chan struct{}), release: make(chan struct{})}
	portfolio := newPortfolioPage(ctx, probe, func() {})
	release := sync.OnceFunc(func() { close(probe.release) })
	t.Cleanup(func() { release(); portfolio.workers.Wait() })
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, clock, func() {})
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, portfolio: portfolio, version: service.MobileDataVersion})
	h.size = image.Pt(950, 820)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			portfolio.workers.Wait()
			accounts.workers.Wait()
			ledger.reads.workers.Wait()
			ledger.model.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(label string) { t.Helper(); assertPageLabel(t, &h.router, label) }
	full := ids.instruments["QA-FULL"]
	other := ids.instruments["QA-FX"]
	h.frame()
	click("Investment assets")
	label("Portfolio value · Known subtotal")
	visit := h.shell.trail.current().id
	setBankField(h, "Search holdings", &portfolio.search.Field, "qa-full")
	settle()
	click(full.DisplayLabel())
	label("Current price · EUR 30.1234 · manual · 2026-11-21 16:00")
	label("Market FX · 1 EUR = 1.2 USD · manual · 2026-11-21 16:00")
	label("Cost basis · EUR € 60.36")
	label("Native market value · EUR € 90.37")
	click("Open account · Harbor Brokerage")
	if accounts.target.account != ids.account {
		t.Fatal("wrong account link destination")
	}
	h.size.Y = 1020
	h.frame()
	click("Open instrument · " + full.DisplayLabel())
	if portfolio.id != full.ID {
		t.Fatal("the account Holding did not open the exact instrument")
	}
	click("Back")
	h.focused(portfolioButton(accounts.instruments, full.ID))
	h.size.Y = 820
	h.frame()
	click("Back")
	h.focused(portfolioButton(portfolio.accounts, ids.account))
	click("View instrument history")
	filters := historyui.Filters{Instrument: full.ID}
	if ledger.model.Filters() != filters || len(ledger.model.Rows()) != 2 {
		t.Fatalf("History opened the wrong filtered scope: %+v / %d", ledger.model.Filters(), len(ledger.model.Rows()))
	}
	h.click("View", false)
	settle()
	activity := ledger.reads.detail.id
	click(fmt.Sprintf("Open instrument · %s", full.DisplayLabel()))
	if portfolio.id != full.ID {
		t.Fatal("History did not reopen the exact instrument")
	}
	click("Back")
	if ledger.reads.detail.id != activity {
		t.Fatal("Back lost activity selection")
	}
	h.focused(portfolioButton(ledger.reads.detail.instruments, full.ID))
	click("Back to ledger activity")
	if ledger.model.Filters() != filters {
		t.Fatal("Back lost instrument filter")
	}
	h.focused(ledger.detailButton(activity))
	click("Back")
	click("Back")
	if h.shell.trail.current().id != visit || portfolio.search.Field.Text() != "qa-full" {
		t.Fatal("Portfolio return discarded the filtered list visit")
	}
	h.focused(portfolioButton(portfolio.instruments, full.ID))

	probe.badPrice.Store(true)
	click(full.DisplayLabel())
	label("Could not load portfolio data.")
	click("Retry")
	label("Native market value · EUR € 90.37")
	probe.holdPrice.Store(true)
	h.click("Reload", false)
	select {
	case <-probe.started:
	case <-time.After(3 * time.Second):
		t.Fatal("old instrument read did not start")
	}
	portfolio.openInstrument(other.ID, other.DisplayLabel(), nil)
	deadline := time.Now().Add(3 * time.Second)
	for portfolio.current == nil || portfolio.id != other.ID {
		if time.Now().After(deadline) {
			t.Fatal("the next instrument waited for the old response")
		}
		h.frame()
		time.Sleep(time.Millisecond)
	}
	release()
	settle()
	label("Market FX unavailable · GBP/USD")
	label("Native market value · GBP £ 13.00")
	if portfolio.current.instrument == nil || portfolio.current.instrument.ID != other.ID {
		t.Fatal("a late quote replaced the instrument")
	}
	for _, item := range []struct{ symbol, fx string }{
		{"QA-PX", "Market FX · 1 EUR = 1.2 USD · manual · 2026-11-21 16:00"},
		{"QA-BOTH", "Market FX unavailable · CHF/USD"},
	} {
		i := ids.instruments[item.symbol]
		portfolio.openInstrument(i.ID, i.DisplayLabel(), nil)
		settle()
		label("Current price unavailable")
		label("Native market value · Unavailable")
		label(item.fx)
	}
	portfolio.openInstrument(domain.NewInstrumentID(), "", nil)
	settle()
	label("This instrument is unavailable.")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	facts, err := readPortfolio(ctx, reopened, full.ID)
	if err != nil || facts.instrument == nil || len(facts.positions) != 1 || facts.positions[0].Amounts.Quantity != "3" {
		t.Fatalf("could not read the same instrument after reopening: %+v / %v", facts, err)
	}
}

func TestPortfolioShellShowsEmptyHoldingsWithFundedAccount(t *testing.T) {
	service, _, _, ids := openPortfolioFixture(t, "empty")
	p := newPortfolioPage(t.Context(), service, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Investment assets", false)
	p.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No investment holdings.")
	assertPageLabel(t, &h.router, "$ 0.00")
	i, err := service.CreateInstrument(t.Context(), application.InstrumentInput{
		Name: "Closed Sample", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(t.Context(), application.HoldingInput{
		AccountID: ids.account.String(), InstrumentID: i.ID.String(), Quantity: "0",
	}); err != nil {
		t.Fatal(err)
	}
	h.click("Reload", false)
	p.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No investment holdings.")
}
