package desktop

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHistoryInvestmentReplacementsMatchWails(t *testing.T) {
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	fixture := seedInvestmentAmendments(t, now)
	note := "Synthetic historical replacement"
	type projection struct {
		target   investmentAmendmentTarget
		cash     map[domain.CurrencyCode]string
		quantity string
	}
	trade := fixture.sell.request
	trade.Side, trade.Quantity, trade.Gross, trade.Fee = "buy", "2", "20", ""
	cases := []struct {
		name        string
		original    wire.ActivityDTO
		request     historyapi.ChangeCommandRequest
		projections []projection
	}{
		{
			name: "buy existing holding in another account", original: fixture.buy.activity, request: trade,
			projections: []projection{
				{fixture.buy, map[domain.CurrencyCode]string{"EUR": "1000"}, "0"},
				{fixture.sell, map[domain.CurrencyCode]string{"EUR": "869.9167"}, "17.5"},
			},
		},
		{
			name: "sell becomes cash added elsewhere", original: fixture.sell.activity,
			request: historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangeMoneyAdded, AccountID: fixture.buy.account.String(), Amount: "25.125", Currency: "EUR", Reason: "income",
			},
			projections: []projection{
				{fixture.sell, map[domain.CurrencyCode]string{"EUR": "800"}, "20"},
				{fixture.buy, map[domain.CurrencyCode]string{"EUR": "942.4995"}, "8.25"},
			},
		},
		{
			name: "dividend changes payment currency", original: fixture.dividend.activity,
			request: historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangeCashDividend, HoldingID: fixture.dividend.holding.String(), Amount: "12.3456", Currency: "EUR",
			},
			projections: []projection{
				{fixture.dividend, map[domain.CurrencyCode]string{"EUR": "912.3456", "USD": "30"}, "10"},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gio, reference := openInvestmentReplacementPair(t, fixture.seed, now)
			request := test.request
			request.Note = &note
			assertInvestmentReplacementMatches(t, gio, reference, test.original, request, now)
			for _, service := range []*application.Service{gio, reference} {
				for _, expected := range test.projections {
					assertNativeTradeCash(t, service, expected.target.account, expected.cash)
					assertReplacementHolding(t, service, expected.target, expected.quantity, false)
				}
			}
		})
	}
	t.Run("missing and dependent holdings", func(t *testing.T) {
		ctx := t.Context()
		gio, reference := openInvestmentReplacementPair(t, fixture.seed, now)
		wails := historyapi.NewService(reference)
		before := investmentReplacementActivities(t, gio)
		for _, test := range []struct {
			target investmentAmendmentTarget
			modify func(*historyapi.ChangeCommandRequest)
		}{
			{fixture.buy, func(request *historyapi.ChangeCommandRequest) { request.HoldingID = "" }},
			{fixture.dependent, func(request *historyapi.ChangeCommandRequest) { request.Quantity = "6" }},
		} {
			request := test.target.request
			test.modify(&request)
			id := test.target.activity.ID
			_, err := history.ReviewAmendment(ctx, gio, history.CorrectActivity, domain.ActivityID(id), history.AmendmentInput{Replacement: &request}, now)
			assertReplacementDomainError(t, err, domain.ErrCannotFixChange)
			request.EffectiveAt = test.target.activity.EffectiveAt
			_, err = wails.FixChange(ctx, id, request)
			assertReplacementWailsError(t, err, domain.ErrCannotFixChange)
		}
		for _, service := range []*application.Service{gio, reference} {
			if after := investmentReplacementActivities(t, service); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected historical replacement changed the ledger")
			}
		}
	})
}

func TestHistoryArchivedSaleReplacementPreservesZeroHolding(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	fixture := seedInvestmentAmendments(t, now)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "archive"), fixture.seed)
	db, _, service := openStage6Database(t, path, func() time.Time { return now })
	target := fixture.sell
	request := target.request
	request.Quantity, request.Gross, request.Fee = "15.5", "155", ""
	request.EffectiveAt = now.AddDate(0, 0, -1).Format(time.RFC3339Nano)
	closeout, err := historyapi.NewService(service).RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertReplacementHolding(t, service, target, "0", false)
	if err := service.ArchiveHolding(ctx, target.holding, true); err != nil {
		t.Fatal(err)
	}
	gio, reference := openInvestmentReplacementPair(t, stage6SeedBytes(t, db), now)
	id := domain.ActivityID(closeout.Activity.ID)
	detail, err := history.LoadDetail(ctx, gio, id)
	if err != nil || !detail.Amendable || detail.Reversible {
		t.Fatalf("archived sale correction/undo eligibility = %t / %t (%v)", detail.Amendable, detail.Reversible, err)
	}
	before := investmentReplacementActivities(t, gio)
	request.Gross = "156.25"
	incompatible := request
	incompatible.Quantity = "14"
	_, err = history.ReviewAmendment(ctx, gio, history.CorrectActivity, id, history.AmendmentInput{Replacement: &incompatible}, now)
	assertReplacementDomainError(t, err, domain.ErrConflict)
	_, err = history.ReviewAmendment(ctx, gio, history.ReverseActivity, id, history.AmendmentInput{}, now)
	assertReplacementDomainError(t, err, domain.ErrConflict)
	_, err = historyapi.NewService(reference).UndoChange(ctx, id.String())
	assertReplacementWailsError(t, err, domain.ErrConflict)
	for _, app := range []*application.Service{gio, reference} {
		if after := investmentReplacementActivities(t, app); !reflect.DeepEqual(before, after) {
			t.Fatal("rejected archived activity changed the ledger")
		}
		assertReplacementHolding(t, app, target, "0", true)
	}
	assertInvestmentReplacementMatches(t, gio, reference, closeout.Activity, request, now)
	for _, app := range []*application.Service{gio, reference} {
		assertNativeTradeCash(t, app, target.account, map[domain.CurrencyCode]string{"EUR": "1046.1667"})
		assertReplacementHolding(t, app, target, "0", true)
	}
}

func openInvestmentReplacementPair(t *testing.T, seed []byte, now time.Time) (*application.Service, *application.Service) {
	t.Helper()
	root := t.TempDir()
	clock := func() time.Time { return now }
	_, _, gio := openStage6Database(t, writeStage6Fixture(t, filepath.Join(root, "gio"), seed), clock)
	_, _, reference := openStage6Database(t, writeStage6Fixture(t, filepath.Join(root, "wails"), seed), clock)
	return gio, reference
}

func assertInvestmentReplacementMatches(t *testing.T, gio, reference *application.Service, original wire.ActivityDTO, request historyapi.ChangeCommandRequest, now time.Time) {
	t.Helper()
	ctx := t.Context()
	id := domain.ActivityID(original.ID)
	wails := historyapi.NewService(reference)
	before := investmentReplacementActivities(t, gio)
	gioReview, err := history.ReviewAmendment(ctx, gio, history.CorrectActivity, id, history.AmendmentInput{Replacement: &request}, now)
	if err != nil {
		t.Fatal(err)
	}
	request.EffectiveAt = original.EffectiveAt
	wailsReview, err := wails.PreviewFixChange(ctx, original.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	assertInvestmentAmendmentParity(t, wire.FromReviewedChangePreview(gioReview.Preview), wailsReview)
	if got := investmentReplacementActivities(t, gio); !reflect.DeepEqual(got, before) {
		t.Fatal("historical replacement preview wrote to the ledger")
	}
	gioSaved, err := gioReview.Commit(ctx, gio)
	if err != nil {
		t.Fatal(err)
	}
	wailsSaved, err := wails.FixChange(ctx, original.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	assertInvestmentAmendmentParity(t, wire.FromChangePreview(gioSaved), wailsSaved)
	for _, client := range []struct {
		app *application.Service
		id  domain.ActivityID
	}{
		{gio, gioSaved.Activity.ID},
		{reference, domain.ActivityID(wailsSaved.Activity.ID)},
	} {
		persisted, err := history.LoadDetail(ctx, client.app, client.id)
		if err != nil || persisted.Links.Original != id || persisted.Links.CorrectionReversal == "" ||
			wire.FromActivity(persisted.Activity).EffectiveAt != original.EffectiveAt || !persisted.Activity.CreatedAt.Equal(now) {
			t.Fatalf("bad persisted replacement: %+v (%v)", persisted, err)
		}
		if count := len(investmentReplacementActivities(t, client.app)); count != len(before)+2 {
			t.Fatalf("got %d activities after one correction, want %d", count, len(before)+2)
		}
	}
}

func investmentReplacementActivities(t *testing.T, service *application.Service) []domain.Activity {
	t.Helper()
	rows, err := service.ListActivities(t.Context(), 50)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertReplacementHolding(t *testing.T, service *application.Service, target investmentAmendmentTarget, quantity string, archived bool) {
	t.Helper()
	rows, err := service.HoldingsByAccounts(t.Context(), []domain.AccountID{target.account})
	if err != nil {
		t.Fatal(err)
	}
	for _, holding := range rows[target.account] {
		if holding.ID == target.holding && holding.Quantity.Canonical() == quantity && (holding.ArchivedAt != nil) == archived {
			return
		}
	}
	t.Fatalf("%s: expected quantity %s with archived %t, got %+v", target.accountName, quantity, archived, rows[target.account])
}

func assertReplacementDomainError(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != code {
		t.Fatalf("Gio guard returned %v, want %s", err, code)
	}
}

func assertReplacementWailsError(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	var wireErr *apierror.WireError
	if !errors.As(err, &wireErr) || wireErr.Code != string(code) {
		t.Fatalf("Wails guard returned %v, want %s", err, code)
	}
}
