package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func seedPagedProductHistory(t *testing.T, app *application.Service, account domain.AccountID) domain.ProductContractID {
	t.Helper()
	ctx := t.Context()
	id := seedTestLockedProductWithUnlock(t, app, account, nil, "2026-11-22")
	for i := range 26 {
		at := linkedContractClock().Add(time.Duration(i) * time.Minute)
		app.SetClock(func() time.Time { return at })
		if _, err := app.AppendProductValuation(ctx, application.AppendProductValuationInput{ProductID: id, Amount: "123.4567", MutationID: domain.NewProductOperationID().String()}); err != nil {
			t.Fatal(err)
		}
	}
	post := func(command application.ProductCommand, at time.Time) domain.ProductOperationID {
		t.Helper()
		app.SetClock(func() time.Time { return at })
		preview, err := app.PreviewProductOperation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := app.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
		if err != nil {
			t.Fatal(err)
		}
		return receipt.OperationID
	}
	interest := post(application.ProductCommand{Kind: domain.ProductOpReceiveInterest,
		ReceiveInterest: &application.ReceiveInterestCommand{ProductID: id.String(), Amount: "1.0123"}}, linkedContractClock().Add(30*time.Minute))
	post(application.ProductCommand{Kind: domain.ProductOpUndo, Undo: &application.UndoProductCommand{OperationID: interest.String()}}, linkedContractClock().Add(31*time.Minute))
	return id
}

type productHistoryFault struct {
	*application.Service
	first, later bool
}

func (s *productHistoryFault) ListProductOperations(ctx context.Context, id domain.ProductContractID, cursor string, limit int) (application.ProductOperationPage, error) {
	if s.first && cursor == "" {
		s.first = false
		return application.ProductOperationPage{}, errors.New("synthetic first-page read failure")
	}
	if s.later && cursor != "" {
		s.later = false
		return application.ProductOperationPage{}, errors.New("synthetic next-page read failure")
	}
	return s.Service.ListProductOperations(ctx, id, cursor, limit)
}

func TestProductOperationHistoryMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	id := seedPagedProductHistory(t, app, ids.account)
	service := &productHistoryFault{Service: app, first: true, later: true}
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, service, func() {})
	p := funds.productPage
	p.receipts, p.clock = app, linkedContractClock
	// Exercise the change between cursor reads even when the shell has not
	// yet observed a ledger-version update and refreshed the current product.
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds})
	h.size = image.Pt(665, 590)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
			funds.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	scrollClick := func(label string) { t.Helper(); scrollProductControl(t, h, label); click(label) }
	settle()
	click("Available funds")
	scrollClick("Open product · Synthetic locked product")
	if p.history.problem != i18n.ProductHistoryFailed || p.err != nil {
		t.Fatal("history failure incorrectly blocked product detail")
	}
	scrollClick("Retry")
	if len(p.history.rows) != productHistoryPageSize || p.history.next == nil || p.history.interest != "" {
		t.Fatal("first page or reversed-interest shortcut is incorrect")
	}
	h.focused(&p.history.refresh)
	scrollClick("Load older operations")
	if len(p.history.rows) != productHistoryPageSize || p.history.problem != i18n.ProductHistoryFailed {
		t.Fatal("a failed page discarded loaded operations")
	}
	h.focused(&p.history.more)
	app.SetClock(func() time.Time { return linkedContractClock().Add(time.Hour) })
	if _, err := app.AppendProductValuation(ctx, application.AppendProductValuationInput{ProductID: id, Amount: "124.0012", MutationID: domain.NewProductOperationID().String()}); err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	settle()
	if len(p.history.rows) != productHistoryPageSize || p.history.problem != i18n.ProductHistoryChanged {
		t.Fatal("paging mixed old and new product history")
	}
	h.focused(&p.history.refresh)
	h.press(key.NameReturn)
	settle()
	scrollClick("Load older operations")
	if len(p.history.rows) != 30 || p.history.next != nil {
		t.Fatalf("wrong result count at the end: %d", len(p.history.rows))
	}
	h.focused(&p.history.refresh)
	assertPageLabel(t, &h.router, "All 30 operations loaded.")
	seen := make(map[domain.ProductOperationID]bool)
	for _, row := range p.history.rows {
		if seen[row.ID] {
			t.Fatalf("duplicate product operation %s", row.ID)
		}
		seen[row.ID] = true
	}
	if got := p.history.rows[2]; got.Kind != domain.ProductOpReceiveInterest || got.ReversedByOperationID == nil || *got.ReversedByOperationID != p.history.rows[1].ID {
		t.Fatal("missing grouped reversal evidence")
	}
	if p.history.zone == nil || p.history.zone.String() != "Asia/Singapore" {
		t.Fatal("history did not use the ledger timezone")
	}
	loaded := slices.Clone(p.history.rows)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	api := liquidity.NewService(reopened)
	for offset, cursor := 0, ""; ; {
		page, err := api.ListOperations(ctx, liquidity.ListOperationsRequest{ProductID: id.String(), Cursor: cursor, Limit: productHistoryPageSize})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Operations {
			got := loaded[offset]
			if row.ID != got.ID.String() || row.Kind != string(got.Kind) || row.EffectiveAt != wire.FormatTime(got.EffectiveAt) || (row.ReversedByOperationID != nil) != (got.ReversedByOperationID != nil) {
				t.Fatalf("reopened Wails row differs from Gio: %+v", row)
			}
			offset++
		}
		if page.Next == nil {
			if offset != len(loaded) {
				t.Fatalf("Wails returned %d of %d operations", offset, len(loaded))
			}
			break
		}
		cursor = *page.Next
	}
}
