package desktop

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestProductGroupedUndoRestoresReservationAndCash(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	app.SetClock(linkedContractClock)
	reservation := reserveTermDeposit(t, app, id, "Synthetic reserve")

	fault := &standaloneLifecycleFault{Service: app}
	h := newStandaloneLifecycleHarness(t, app, fault, linkedContractClock)
	p := h.page
	if p.canStartGroupedUndo() {
		t.Fatal("an active reservation should block undo of the opening")
	}
	if reason := p.history.undo.Reason; reason == nil || *reason != application.UndoActiveReservation {
		t.Fatalf("missing active reservation reason: %v", reason)
	}
	settle := func(service *application.Service, id domain.ProductContractID, reservation domain.LiquidityReservationID) string {
		t.Helper()
		api := liquidityapi.NewService(service)
		cmd := liquidityapi.ProductCommandRequest{Kind: string(domain.ProductOpSettle), Settle: &application.SettleProductCommand{
			ProductID: id.String(), GrossProceeds: productOptional("132.1234"), Fee: productOptional("1.2222"),
			EffectiveLocalDate: "2026-11-23", EffectiveLocalTime: "18:00", ReleaseReservationIDs: []string{reservation.String()},
		}}
		preview, err := api.PreviewProductOperation(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
			Command: cmd, ReviewedStateHash: preview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.OperationID
	}
	settlement := settle(app, id, reservation.ID)
	if err := storage.DB.CheckpointWAL(ctx); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(storage.DB.Path)
	if err != nil {
		t.Fatal(err)
	}
	refPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	refDB, _, reference := openStage6Database(t, refPath, linkedContractClock)
	before := managedOperations(t, app, id)
	assertReadOnly := func(service *application.Service) {
		t.Helper()
		if operations := managedOperations(t, service, id); !reflect.DeepEqual(operations, before) {
			t.Fatalf("review changed product operations: %+v", operations)
		}
		product, err := service.Product(ctx, id)
		if err != nil || product.Contract.State != domain.ProductStateSettled || len(product.Reservations) != 0 {
			t.Fatalf("review changed the settled product or showed an active reservation: %+v / %v", product, err)
		}
		rows, err := service.ListLiquidityReservations(ctx)
		i := slices.IndexFunc(rows, func(row domain.LiquidityReservation) bool { return row.ID == reservation.ID })
		if err != nil || i < 0 || rows[i].Active() || rows[i].Revision != reservation.Revision+1 {
			t.Fatalf("review changed the released reservation: %+v / %v", rows, err)
		}
		assertNativeTradeCash(t, service, ids.account, map[domain.CurrencyCode]string{"USD": "286.9012"})
	}
	h.settle()
	h.scrollClick("Grouped undo")
	if p.receipt == nil || p.receipt.undo.ID.String() != settlement {
		t.Fatal("opening undo did not use the latest eligible settlement")
	}
	h.clickButton("Preview undo")
	assertReadOnly(app)
	h.clickButton("Cancel")
	h.focused(&p.startGroupedUndo)
	if len(fault.receipts) != 0 {
		t.Fatal("cancelling a reviewed undo posted it")
	}

	h.clickButton("Grouped undo")
	h.size = image.Pt(665, 480)
	h.settle()
	h.clickButton("Preview undo")
	d := p.receipt
	if d.plan == nil {
		t.Fatal("undo preview is missing")
	}
	assertPageLabel(t, &h.router, "Account cash after · 156 USD")
	api := liquidityapi.NewService(reference)
	cmd := liquidityapi.ProductCommandRequest{Kind: string(domain.ProductOpUndo), Undo: &application.UndoProductCommand{OperationID: settlement}}
	want, err := api.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	got := liquidityapi.FromProductOperationPreview(d.plan.review)
	if !reflect.DeepEqual(got.CashBefore, want.CashBefore) || !reflect.DeepEqual(got.CashAfter, want.CashAfter) ||
		!reflect.DeepEqual(got.ProductAfter, want.ProductAfter) || len(got.Activities) != len(want.Activities) {
		t.Fatalf("undo preview differs from Wails: %+v / %+v", got, want)
	}
	assertReadOnly(app)
	assertReadOnly(reference)
	staleMutation := d.plan.mutation
	fault.revisionConflict = true
	h.clickButton("Confirm undo")
	if p.receipt != d || d.plan != nil || p.unresolved() || d.problem != i18n.ProductUndoStale {
		t.Fatal("a definite revision rejection did not discard the stale preview")
	}
	assertPageLabel(t, &h.router, "The undo preview is stale. Preview the reversal again.")
	assertReadOnly(app)
	h.clickButton("Preview undo")
	if d.plan == nil || d.plan.mutation == staleMutation {
		t.Fatal("retry reused the rejected undo preview")
	}

	refUndo, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
		Command: cmd, ReviewedStateHash: want.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
	})
	if err != nil {
		t.Fatal(err)
	}

	fault.loseReceipt = true
	h.clickButton("Confirm undo")
	if !d.uncertain || !p.unresolved() {
		t.Fatal("a lost undo acknowledgement did not retain the exact reviewed command")
	}
	h.focused(&d.replay)
	h.press(key.NameReturn)
	h.settle()
	if p.receipt != nil || p.saved != i18n.ProductUndoSaved || len(fault.receipts) != 3 || !reflect.DeepEqual(fault.receipts[1], fault.receipts[2]) {
		t.Fatal("undo retry failed to replay the same request once")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	_, _, refReopened := openStage6Database(t, refPath, linkedContractClock)
	rows := managedOperations(t, reopened, id)
	if len(rows) != 3 || rows[0].Kind != domain.ProductOpUndo || rows[1].ID.String() != settlement ||
		rows[1].ReversedByOperationID == nil || *rows[1].ReversedByOperationID != rows[0].ID {
		t.Fatalf("grouped undo evidence is missing after reopen: %+v", rows)
	}
	detail, err := reopened.Product(ctx, id)
	if err != nil || detail.Contract.State != domain.ProductStateOpen || len(detail.Reservations) != 1 ||
		!detail.Reservations[0].Active() || detail.Reservations[0].Revision != reservation.Revision+2 || detail.CurrentValue == nil ||
		detail.CurrentValue.CanonicalAmount() != "123.4567" {
		t.Fatalf("undo did not restore the contract and reservation: %+v / %v", detail, err)
	}
	refDetail, err := refReopened.Product(ctx, id)
	if err != nil || refDetail.Contract.State != detail.Contract.State || refDetail.CurrentValue == nil ||
		refDetail.CurrentValue.CanonicalAmount() != detail.CurrentValue.CanonicalAmount() || len(refDetail.Reservations) != 1 ||
		!refDetail.Reservations[0].Active() || refDetail.Reservations[0].Revision != detail.Reservations[0].Revision {
		t.Fatalf("Wails did not restore the same contract value and reservation: %+v / %v", refDetail, err)
	}
	undo, err := reopened.ReadProductOperationReceipt(ctx, rows[0].ID)
	if err != nil || len(undo.ActivityIDs) != len(refUndo.ActivityIDs) {
		t.Fatalf("reopened evidence differs from Wails: %+v / %+v / %v", undo, refUndo, err)
	}
	// Legacy operation receipts do not persist decimal cash values. Check the
	// exact resulting cash in each reopened ledger.
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
	assertNativeTradeCash(t, refReopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
