package desktop

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const positionReplacementHousehold = "Synthetic PR302 position replacements"

type positionReplacementFixture struct {
	seed                []byte
	original, laterSale wire.ActivityDTO
	source, destination investmentAmendmentTarget
	empty               domain.AccountID
}

func seedPositionReplacements(t *testing.T, asOf time.Time) positionReplacementFixture {
	t.Helper()
	ctx := t.Context()
	origin := asOf.UTC().Truncate(24*time.Hour).AddDate(0, 0, -4).Add(time.Hour)
	now := origin
	db, _, app := openStage6Database(t, filepath.Join(t.TempDir(), "position-replacements.db"), func() time.Time { return now },
		application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: positionReplacementHousehold, BaseCurrency: "EUR", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account := func(name string) investmentAmendmentTarget {
		t.Helper()
		row, err := app.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "EUR", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return investmentAmendmentTarget{accountName: name, account: row.Account.ID}
	}
	fixture := positionReplacementFixture{
		source:      account("Sample Position Source"),
		destination: account("Sample Position Destination"),
		empty:       account("Sample Position Empty").account,
	}
	instrument, err := app.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Sample Position Fund", Symbol: "PRPOS", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	now = asOf
	wails := historyapi.NewService(app)
	record := func(request historyapi.ChangeCommandRequest, offset time.Duration, note string) wire.ActivityDTO {
		t.Helper()
		request.Note = &note
		request.EffectiveAt = origin.Add(offset).Format(time.RFC3339Nano)
		posted, err := wails.RecordChange(ctx, request)
		if err != nil {
			t.Fatalf("seed %q: %v", note, err)
		}
		return posted.Activity
	}
	fund := func(target investmentAmendmentTarget, amount string, offset time.Duration) {
		t.Helper()
		record(historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: target.account.String(), Amount: amount, Currency: "EUR", Reason: "contribution",
		}, offset, target.accountName+" funding")
	}
	trade := func(target investmentAmendmentTarget, side, quantity, gross string, offset time.Duration, note string) wire.ActivityDTO {
		t.Helper()
		return record(historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, SettlementAccountID: target.account.String(), InstrumentID: instrument.ID.String(),
			HoldingID: target.holding.String(), Side: side, Quantity: quantity, Gross: gross, GrossCurrency: "EUR",
		}, offset, note)
	}
	fund(fixture.source, "1000", time.Hour)
	fund(fixture.destination, "200", 2*time.Hour)
	source := trade(fixture.source, "buy", "12", "120", 3*time.Hour, "Position source opening buy")
	destination := trade(fixture.destination, "buy", "1", "11", 4*time.Hour, "Existing matching destination holding")
	fixture.source.holding = domain.HoldingID(source.TradeDetail.HoldingID)
	fixture.destination.holding = domain.HoldingID(destination.TradeDetail.HoldingID)
	fixture.original = trade(fixture.source, "buy", "4", "40", 48*time.Hour+17*time.Second+123*time.Millisecond, "Replace this synthetic position buy")
	fixture.laterSale = trade(fixture.source, "sell", "8", "96", 72*time.Hour, "Later sale needs eight units")
	fixture.seed = stage6SeedBytes(t, db)
	return fixture
}

func TestExportInvestmentPositionReplacementFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_POSITION_REPLACEMENT_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_POSITION_REPLACEMENT_QA_DIR to an existing disposable directory to export native QA fixtures")
	}
	root, err := os.MkdirTemp(output, "position-replacements-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedPositionReplacements(t, time.Now())
	for _, profile := range []string{"source", "gio-transfer", "wails-transfer", "gio-adjustment", "wails-adjustment"} {
		writeStage6Fixture(t, filepath.Join(root, profile), fixture.seed)
	}
	t.Logf("fixtures=%s household=%q target=%q effective=%s", root, positionReplacementHousehold, "Replace this synthetic position buy", fixture.original.EffectiveAt)
}

func TestHistoryInvestmentReplacementDefaultsToAddingUnits(t *testing.T) {
	buy := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeTrade, Side: string(domain.TradeBuy)}
	form := historyAmendmentDraft{investment: &investmentAmendmentDraft{command: buy, initial: buy}}

	form.selectReplacement(historyapi.ChangePositionAdjustment)
	if !form.investment.command.Added {
		t.Fatal("a new investment quantity adjustment should default to adding units")
	}

	form.investment.command.Added = false
	form.selectReplacement(historyapi.ChangePositionAdjustment)
	if form.investment.command.Added {
		t.Fatal("reselecting an adjustment should preserve the selected removal direction")
	}
}

func TestHistoryInvestmentPositionReplacementsMatchWails(t *testing.T) {
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	fixture := seedPositionReplacements(t, now)
	note := "Confirmed historical position replacement"
	transfer := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionTransfer, FromHoldingID: fixture.source.holding.String(), ToHoldingID: fixture.destination.holding.String(),
		Quantity: "2.5", Note: &note,
	}
	adjustment := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionAdjustment, HoldingID: fixture.source.holding.String(), Added: true,
		Quantity: "1.25", UnitCost: "10.125", Note: &note,
	}
	assertState := func(t *testing.T, service *application.Service, cash, source, destination string) {
		t.Helper()
		assertNativeTradeCash(t, service, fixture.source.account, map[domain.CurrencyCode]string{"EUR": cash})
		assertNativeTradeCash(t, service, fixture.destination.account, map[domain.CurrencyCode]string{"EUR": "189"})
		assertReplacementHolding(t, service, fixture.source, source, false)
		assertReplacementHolding(t, service, fixture.destination, destination, false)
		rows, err := service.HoldingsByAccounts(t.Context(), []domain.AccountID{fixture.empty})
		if err != nil || len(rows[fixture.empty]) != 0 {
			t.Fatalf("empty account gained a holding: %+v (%v)", rows, err)
		}
	}
	for _, tc := range []struct {
		name, source, destination string
		request                   historyapi.ChangeCommandRequest
	}{
		{"transfer to existing matching holding", "1.5", "3.5", transfer},
		{"add units with exact original unit cost", "5.25", "1", adjustment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gio, wails := openInvestmentReplacementPair(t, fixture.seed, now)
			assertInvestmentReplacementMatches(t, gio, wails, fixture.original, tc.request, now)
			for _, app := range []*application.Service{gio, wails} {
				assertState(t, app, "976", tc.source, tc.destination)
				laterSale, err := app.Activity(t.Context(), domain.ActivityID(fixture.laterSale.ID))
				if err != nil || laterSale.TradeDetail == nil || laterSale.TradeDetail.Quantity.Canonical() != "8" {
					t.Fatalf("historical replacement lost later sale: %+v (%v)", laterSale.TradeDetail, err)
				}
			}
		})
	}
	missingDestination, spentTransfer, spentAdjustment := transfer, transfer, adjustment
	missingDestination.ToHoldingID, missingDestination.ToAccountID = "", fixture.empty.String()
	spentTransfer.Quantity = "5"
	spentAdjustment.Added, spentAdjustment.Quantity, spentAdjustment.UnitCost = false, "5", ""
	for _, tc := range []struct {
		name    string
		request historyapi.ChangeCommandRequest
	}{
		{"transfer requires a matching destination holding", missingDestination},
		{"transfer cannot invalidate later sale", spentTransfer},
		{"quantity removal cannot invalidate later sale", spentAdjustment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			gio, reference := openInvestmentReplacementPair(t, fixture.seed, now)
			wails := historyapi.NewService(reference)
			before := investmentReplacementActivities(t, gio)
			original := fixture.original.ID
			_, err := history.ReviewAmendment(ctx, gio, history.CorrectActivity, domain.ActivityID(original), history.AmendmentInput{Replacement: &tc.request}, now)
			assertReplacementDomainError(t, err, domain.ErrCannotFixChange)
			_, err = wails.PreviewFixChange(ctx, original, tc.request)
			assertReplacementWailsError(t, err, domain.ErrCannotFixChange)
			_, err = wails.FixChange(ctx, original, tc.request)
			assertReplacementWailsError(t, err, domain.ErrCannotFixChange)
			for _, app := range []*application.Service{gio, reference} {
				if got := investmentReplacementActivities(t, app); !reflect.DeepEqual(got, before) {
					t.Fatal("refused position replacement wrote to the ledger")
				}
				assertState(t, app, "936", "8", "1")
			}
		})
	}
}
