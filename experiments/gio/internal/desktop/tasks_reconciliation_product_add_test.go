package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type linkedProductHarness struct {
	*previewHarness
	c     *holdingReconciliation
	tasks *tasksPage
}

func newLinkedProductHarness(t *testing.T, app *application.Service, api holdingReconcileAPI) *linkedProductHarness {
	t.Helper()
	c := &holdingReconciliation{service: api, ledger: app, clock: linkedContractClock}
	tasks := newTasksPage(t.Context(), app, func() {})
	tasks.checks, tasks.holdingRecon = app, c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(665, 1000)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	out := &linkedProductHarness{previewHarness: h, c: c, tasks: tasks}
	out.settle()
	out.activate(t, "Tasks & checks")
	out.activate(t, "Reconciliation")
	out.activate(t, "Open holdings reconciliation · Harbor Brokerage")
	return out
}

func (h *linkedProductHarness) settle() {
	for range 5 {
		h.frame()
		h.tasks.workers.Wait()
		if f := h.c.cashDated; f != nil {
			f.workers.Wait()
		}
	}
}

func (h *linkedProductHarness) activate(t *testing.T, label string) {
	t.Helper()
	bounds := semanticBounds(t, &h.router, label, false)
	if y := bounds.Min.Add(bounds.Max).Y / 2; y < 70 || y >= h.size.Y-20 {
		h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+y-245)
		h.settle()
	}
	h.click(label, false)
	h.settle()
}

func TestLinkedAddProductMatchesWails(t *testing.T) {
	for _, ct := range []struct {
		name, currency, endCash string
		record                  bool
	}{
		{"open deposit", "USD", "105.8643", false}, {"record existing locked product", "EUR", "156", true},
	} {
		t.Run(ct.name, func(t *testing.T) {
			ctx := t.Context()
			app, storage, paths, ids := openExistingBuyFixture(t)
			ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
			app.SetClock(linkedContractClock)
			ref.SetClock(linkedContractClock)
			api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
			session := startTestHoldingReconciliation(t, api, ids.account)
			refSession := startTestHoldingReconciliation(t, refAPI, refIDs.account)
			before, err := app.ListActivities(ctx, 100)
			if err != nil {
				t.Fatal(err)
			}
			fault := &linkedProductCommitFault{holdingReconcileAPI: api}
			h := newLinkedProductHarness(t, app, fault)
			c := h.c
			const start, maturity, minute = "2026-11-23", "2026-12-23", "18:00"
			name, principal := "Synthetic new term deposit", "50.1234"
			if ct.record {
				name, principal = "Synthetic recorded contract", "40.1234"
			}
			entry := &c.checkpointActions(ids.account.String(), "scope").addProduct
			const action = "Add product"
			choose := func(label, option string) { h.click(label, true); h.activate(t, option) }
			edit := func() *holdingReconDraft {
				h.activate(t, action)
				d := c.draft
				f := d.product.add
				h.focused(&f.principal.Editor)
				if link := d.product.link(); link.AccountID != ids.account.String() || link.ItemID != "scope" {
					t.Fatal("wrong account completeness target")
				}
				if ct.record {
					choose("Entry method", "Record existing product")
					choose("Product type", "Locked product")
					h.click("Contract currency", true)
					h.router.Queue(key.EditEvent{Text: "EUR"})
					h.frame()
					h.activate(t, "EUR")
					f.currentValue.Editor.SetText("41.3456")
					f.cost.Editor.SetText("39.2345")
				} else {
					f.openingFee.Editor.SetText("0.0123")
					f.time.day, f.time.minute = start, minute
				}
				f.principal.Editor.SetText(principal)
				f.terms.name.Editor.SetText(name)
				f.terms.terms.StartOn, f.terms.terms.MaturityOn = start, productOptional(maturity)
				h.settle()
				return d
			}
			noWrite := func() {
				t.Helper()
				products, err := app.ListProducts(ctx, &ids.account, true)
				activities, aerr := app.ListActivities(ctx, 100)
				links, lerr := api.Links(ctx, session.ID)
				if err != nil || aerr != nil || lerr != nil || len(products) != 0 || !reflect.DeepEqual(activities, before) || len(links) != 0 {
					t.Fatalf("unconfirmed Add product changed ledger: products=%d activities=%d links=%d", len(products), len(activities), len(links))
				}
				assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
			}
			d := edit()
			if ct.record {
				h.activate(t, "Review")
				assertPageLabel(t, &h.router, "Confirm that account cash excludes this product before recording it.")
				d.product.add.excludesCash.Value = true
				h.settle()
			}
			h.activate(t, "Review")
			if d.review == nil || d.review.operation == nil {
				t.Fatal("missing product review")
			}
			h.activate(t, "Back to details")
			h.focused(&d.product.add.principal.Editor)
			if d.product.add.principal.Text() != principal {
				t.Fatal("Back lost the principal")
			}
			h.activate(t, "Review")
			h.activate(t, "Cancel")
			h.focused(entry)
			noWrite()

			d = edit()
			if ct.record {
				d.product.add.excludesCash.Value = true
				h.settle()
			}
			h.activate(t, "Review")
			stale := d.review.planID
			check, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
				AccountID: ids.account.String(), ItemID: "scope", FinancialRevision: session.Accounts[0].FinancialRevision, Outcome: "skipped", Note: "Another reviewer updated completeness."})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := api.Commit(ctx, check.PlanID); err != nil {
				t.Fatal(err)
			}
			h.press(key.NameReturn)
			h.settle()
			noWrite()
			if d.review != nil || d.product.add.principal.Text() != principal || d.product.add.terms.name.Text() != name || d.product.currency != ct.currency {
				t.Fatal("stale confirmation lost the Add product draft")
			}
			h.focused(&d.product.add.principal.Editor)
			h.activate(t, "Review")
			refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "scope",
				ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
			days, basis, zero := 0, "calendar", "0"
			terms := application.ProductTermsInput{Kind: "term_deposit", Name: name, StartOn: start, MaturityOn: productOptional(maturity), InterestMode: "none"}
			policy := application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: terms.MaturityOn, SettlementDays: &days, DayBasis: &basis, NormalExitFee: &zero, EarlyKind: "not_allowed"}
			request := liquidity.ProductCommandRequest{ReconciliationLink: &refLink, Kind: "open"}
			if ct.record {
				terms.Kind = "locked_product"
				policy.AccessKind, policy.UnlockOn = "on_request", nil
				request.Kind = "record_existing"
				request.RecordExisting = &application.RecordExistingProductCommand{AccountID: refIDs.account.String(), Currency: ct.currency, Principal: principal,
					CurrentValue: "41.3456", TotalCostBasis: "39.2345", CashExcludesProduct: true, Terms: terms, Policy: policy}
			} else {
				request.Open = &application.OpenProductCommand{AccountID: refIDs.account.String(), Currency: ct.currency, Principal: principal, OpeningFee: productOptional("0.0123"),
					EffectiveLocalDate: start, EffectiveLocalTime: minute, Terms: terms, Policy: policy}
			}
			expected, err := refAPI.PreviewProductOperation(ctx, request)
			if err != nil || expected.Operation == nil || d.review == nil || d.review.planID == stale {
				t.Fatalf("could not compare Wails and refreshed Gio preview: %v", err)
			}
			got, want := d.review.operation, expected.Operation
			if got.Kind != want.Kind || got.EffectiveAt != want.EffectiveAt || len(got.Activities) != 1 ||
				!reflect.DeepEqual(got.CashBefore, want.CashBefore) || !reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) {
				t.Fatalf("Gio Add product preview differs: %+v / %+v", got, want)
			}
			if err := refAPI.CommitProduct(ctx, expected.PlanID); err != nil {
				t.Fatal(err)
			}
			plan := d.review.planID
			fault.loseNext = true
			h.press(key.NameReturn)
			h.settle()
			if !c.unresolved() {
				t.Fatal("lost acknowledgement lost the reviewed plan")
			}
			h.press(key.NameReturn)
			h.settle()
			h.focused(entry)
			if !reflect.DeepEqual(fault.plans, []string{stale, plan, plan}) || len(c.links) != 2 {
				t.Fatalf("incorrect idempotent retry or link count: %v / %d", fault.plans, len(c.links))
			}
			saved := append([]domain.ReconciliationLinkedChange(nil), c.links...)
			if err := storage.Close(); err != nil {
				t.Fatal(err)
			}
			if err := refStorage.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, _ := reopenBaselineProfile(t, paths)
			reference, _ := reopenBaselineProfile(t, refPaths)
			reopened.SetClock(linkedContractClock)
			reference.SetClock(linkedContractClock)
			for _, copy := range []struct {
				app       *application.Service
				account   domain.AccountID
				operation string
			}{{reopened, ids.account, plan}, {reference, refIDs.account, expected.PlanID}} {
				products, err := copy.app.ListProducts(ctx, &copy.account, true)
				if err != nil || len(products) != 1 {
					t.Fatalf("expected one reopened holding: %v", err)
				}
				p := products[0]
				value := principal
				if ct.record {
					value = "41.3456"
				}
				if p.Contract.OpenedOperationID.String() != copy.operation || p.Contract.AccountID != copy.account || p.Contract.Name != name || p.Contract.Principal.CanonicalAmount() != principal ||
					p.Contract.Currency.String() != ct.currency || p.Contract.StartOn != start || p.Contract.MaturityOn == nil || *p.Contract.MaturityOn != maturity ||
					p.CurrentValue == nil || p.CurrentValue.CanonicalAmount() != value {
					t.Fatalf("wrong reopened product: %+v", p)
				}
				if ct.record {
					entries, err := copy.app.ListActivities(ctx, 100)
					if err != nil || len(entries) != len(before)+1 {
						t.Fatalf("record-existing activity did not reopen: %v", err)
					}
					var foundCost bool
					for _, effect := range entries[0].Effects {
						if effect.CostUnitPrice != nil && effect.HoldingID != nil && *effect.HoldingID == p.Contract.HoldingID {
							foundCost = effect.CostUnitPrice.Canonical() == "39.2345"
						}
					}
					if !foundCost {
						t.Fatalf("record-existing basis did not reopen: %+v", entries[0].Effects)
					}
				}
				assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": ct.endCash})
			}
			activities, err := reopened.ListActivities(ctx, 100)
			if err != nil || len(activities) != len(before)+1 || !reflect.DeepEqual(activities[1:], before) {
				t.Fatalf("wrong number of new activities: %d %v", len(activities), err)
			}
			if !ct.record && !activities[0].EffectiveAt.Equal(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)) {
				t.Fatal("explicit opening date did not reopen")
			}
			links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
			if err != nil || !reflect.DeepEqual(links, saved) {
				t.Fatalf("links did not reopen: %v", err)
			}
			for _, link := range links {
				if link.AccountID != ids.account.String() || link.ItemID != "scope" ||
					link.Family == "activity" && link.OperationID != activities[0].ID.String() ||
					link.Family == "product_operation" && link.OperationID != plan {
					t.Fatalf("invalid completeness link: %+v", link)
				}
			}
		})
	}
}
