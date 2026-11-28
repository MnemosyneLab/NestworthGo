package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func seedInvestmentAmendmentRestrictions(t *testing.T, asOf time.Time) investmentAmendmentFixture {
	t.Helper()
	fixture := seedInvestmentAmendments(t, asOf)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "restricted"), fixture.seed)
	db, _, service := openStage6Database(t, path, func() time.Time { return asOf })
	wails := historyapi.NewService(service)
	effective := asOf.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1).Add(time.Hour).Format(time.RFC3339Nano)
	for _, row := range []struct {
		target                 investmentAmendmentTarget
		amount, currency, note string
	}{
		{fixture.sell, "889", "EUR", "Later withdrawal uses sale proceeds"},
		{fixture.dividend, "42", "USD", "Later withdrawal uses dividend cash"},
	} {
		if _, err := wails.RecordChange(t.Context(), historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyRemoved, AccountID: row.target.account.String(),
			Amount: row.amount, Currency: row.currency, Reason: "other", EffectiveAt: effective, Note: &row.note,
		}); err != nil {
			t.Fatalf("seed restricted %s cash: %v", row.target.accountName, err)
		}
	}
	fixture.seed = stage6SeedBytes(t, db)
	return fixture
}

func TestExportInvestmentAmendmentRestrictions(t *testing.T) {
	output := os.Getenv("NESTWORTH_INVESTMENT_RESTRICTION_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_INVESTMENT_RESTRICTION_QA_DIR to an existing disposable directory to export native QA fixtures")
	}
	root, err := os.MkdirTemp(output, "investment-restrictions-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedInvestmentAmendmentRestrictions(t, time.Now())
	for _, profile := range []string{"source", "gio", "wails"} {
		writeStage6Fixture(t, filepath.Join(root, profile), fixture.seed)
	}
	t.Logf("fixtures=%s household=%q dates=%s..%s", root, investmentAmendmentHousehold,
		fixture.sell.activity.EffectiveLocalDate, fixture.dividend.activity.EffectiveLocalDate)
}

func TestHistoryInvestmentAmendmentsRejectConsumedCash(t *testing.T) {
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	clock := func() time.Time { return now }
	fixture := seedInvestmentAmendmentRestrictions(t, now)
	for _, row := range []struct {
		correct  string
		kind     domain.ActivityKind
		target   investmentAmendmentTarget
		amount   string
		cash     map[domain.CurrencyCode]string
		quantity string
	}{
		{"Correct sell", domain.ActivitySell, fixture.sell, "89", map[domain.CurrencyCode]string{"EUR": "0.9167"}, "15.5"},
		{"Correct dividend", domain.ActivityCashDividend, fixture.dividend, "11", map[domain.CurrencyCode]string{"EUR": "900", "USD": "0.3456"}, "10"},
	} {
		t.Run(string(row.kind), func(t *testing.T) {
			ctx := t.Context()
			root := t.TempDir()
			gioPath := writeStage6Fixture(t, filepath.Join(root, "gio"), fixture.seed)
			wailsPath := writeStage6Fixture(t, filepath.Join(root, "wails"), fixture.seed)
			_, _, gio := openStage6Database(t, gioPath, clock)
			_, _, reference := openStage6Database(t, wailsPath, clock)
			wails := historyapi.NewService(reference)
			activities := func(app *application.Service) []domain.Activity {
				t.Helper()
				got, err := app.ListActivities(ctx, 50)
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			before := activities(gio)
			page, h, finish, click := investmentAmendmentHarness(t, gio, clock)
			page.model.SetFilters(history.Filters{Account: row.target.account, Kind: row.kind})
			finish()
			click("View")
			original := row.target.activity.ID
			if page.reads.detail.id.String() != original {
				t.Fatalf("opened the wrong restricted %s", row.kind)
			}
			rejected := func(confirm string, problem i18n.MessageID) {
				t.Helper()
				form := page.reads.detail.amendment
				if form == nil || form.review != nil || form.problem != problem {
					t.Fatalf("expected blocked %s with message %v, got %+v", row.kind, problem, form)
				}
				if findPageLabel(&h.router, confirm) != nil {
					t.Fatalf("blocked activity exposed %q", confirm)
				}
				click("Cancel")
			}
			click("Reverse activity")
			rejected("Confirm reversal", i18n.CashMovementInsufficient)
			click(row.correct)
			page.reads.detail.amendment.amount.Editor.SetText(row.amount)
			click("Preview correction")
			rejected("Confirm correction", i18n.HistoryAmendmentRefused)

			assertWireError := func(err error, code domain.ErrorCode) {
				t.Helper()
				var wire *apierror.WireError
				if !errors.As(err, &wire) || wire.Code != string(code) {
					t.Fatalf("Wails %s guard returned %v, want %s", row.kind, err, code)
				}
			}
			_, err := wails.UndoChange(ctx, original)
			assertWireError(err, domain.ErrInsufficientBalance)
			replacement := row.target.request
			replacement.Note = row.target.activity.Note
			replacement.EffectiveAt = row.target.activity.EffectiveAt
			if row.kind == domain.ActivitySell {
				replacement.Gross = row.amount
			} else {
				replacement.Amount = row.amount
			}
			_, err = wails.PreviewFixChange(ctx, original, replacement)
			assertWireError(err, domain.ErrCannotFixChange)
			_, err = wails.FixChange(ctx, original, replacement)
			assertWireError(err, domain.ErrCannotFixChange)
			for _, app := range []*application.Service{gio, reference} {
				if after := activities(app); !reflect.DeepEqual(after, before) {
					t.Fatalf("a rejected %s changed the audit history", row.kind)
				}
				assertNativeTradeCash(t, app, row.target.account, row.cash)
				holdings, err := app.HoldingsByAccounts(ctx, []domain.AccountID{row.target.account})
				if held := holdings[row.target.account]; err != nil || len(held) != 1 || held[0].ID != row.target.holding || held[0].Quantity.Canonical() != row.quantity {
					t.Fatalf("a rejected %s changed the holding: %+v (%v)", row.kind, held, err)
				}
			}
		})
	}
}
