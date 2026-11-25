package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type fundsSetupFault struct {
	*application.Service
	loseNext bool
	plans    []string
	replayed bool
}

func (f *fundsSetupFault) RecordProductOperation(ctx context.Context, command application.ProductCommand, id, hash string) (application.ProductOperationReceipt, error) {
	f.plans = append(f.plans, id)
	receipt, err := f.Service.RecordProductOperation(ctx, command, id, hash)
	if err == nil && f.loseNext {
		f.loseNext = false
		return application.ProductOperationReceipt{}, errors.New("synthetic lost product confirmation")
	}
	f.replayed = receipt.Replayed
	return receipt, err
}

func TestAvailableFundsProductSetupMatchesWails(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "opening"
		if existing {
			name = "record existing"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			app, session, paths, ids := openExistingBuyFixture(t)
			ref, refSession, refPaths, refIDs := openExistingBuyFixture(t)
			now := linkedContractClock()
			clock := func() time.Time { return now }
			app.SetClock(clock)
			ref.SetClock(clock)
			if existing {
				bootstrap, err := app.Bootstrap(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range []struct{ name, kind string }{{"Alternate Brokerage", "brokerage"}, {"Archived Brokerage", "brokerage"}, {"Physical wallet", "cash_on_hand"}} {
					a, err := app.CreateAccount(ctx, application.AccountInput{Name: row.name, AccountType: row.kind, TrackingMode: "holdings", BalanceSheetRole: "asset",
						DefaultCurrency: "EUR", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, IncludeInNetWorth: true})
					if err != nil {
						t.Fatal(err)
					}
					if row.name == "Archived Brokerage" {
						if err := app.ArchiveAccount(ctx, a.Account.ID, true); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			before, err := app.ListActivities(ctx, 100)
			if err != nil {
				t.Fatal(err)
			}
			fault := &fundsSetupFault{Service: app}
			funds := newAvailableFundsPage(ctx, app, func() {})
			funds.productLedger, funds.productClock = fault, clock
			funds.productPage = newProductDetailPage(ctx, app, func() {})
			h := newShellHarnessWithPages(t, ledgerPages{funds: funds, version: app.MobileDataVersion})
			h.size = image.Pt(665, 660)
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			settle := func() {
				for range 4 {
					h.frame()
					funds.workers.Wait()
					funds.productPage.workers.Wait()
				}
			}
			click := func(label string) {
				t.Helper()
				bounds := semanticBounds(t, &h.router, label, false)
				if y := bounds.Min.Add(bounds.Max).Y / 2; y < 70 || y >= h.size.Y-20 {
					h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+y-260)
					settle()
				}
				h.click(label, false)
				settle()
			}
			label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
			settle()
			click("Available funds")
			const principal, start, maturity, fee = "50.1234", "2026-11-23", "2026-12-23", "0.0123"
			const title = "Synthetic available funds product"
			fill := func() *fundsProductSetup {
				t.Helper()
				click("Add product")
				d := funds.setup
				if existing {
					label("Choose account")
					if len(d.choices) != 2 || findPageLabel(&h.router, "Physical wallet") != nil || findPageLabel(&h.router, "Archived Brokerage") != nil {
						t.Fatalf("incorrect account choices: %+v", d.choices)
					}
					h.focused(d.accountButtons[d.choices[0].ID])
					click("Harbor Brokerage")
				}
				if d.account.ID != ids.account || d.currency != "USD" || d.form == nil {
					t.Fatal("wrong setup account or currency")
				}
				h.focused(&d.form.principal.Editor)
				label("Account cash available · 156 USD")
				d.form.principal.Editor.SetText(principal)
				if existing {
					d.currency, d.form.mode = "EUR", domain.ProductOpRecordExisting
					d.form.pickKind(string(domain.ProductLockedProduct))
					d.form.currentValue.Editor.SetText("52.0123")
					d.form.cost.Editor.SetText("49.7891")
				} else {
					d.form.terms.terms.InterestMode = string(domain.InterestSimpleAct360)
					d.form.terms.deposit.rate.Editor.SetText("3.1259")
					d.form.openingFee.Editor.SetText(fee)
					d.form.time.day, d.form.time.minute = start, "18:00"
				}
				d.form.terms.name.Editor.SetText(title)
				d.form.terms.terms.StartOn = start
				d.form.terms.terms.MaturityOn = productOptional(maturity)
				d.form.terms.fee.Editor.SetText("0.0021")
				settle()
				if existing {
					click("Review")
					label("Confirm that account cash excludes this product before recording it.")
					d.form.excludesCash.Value = true
					settle()
				}
				return d
			}
			noWrite := func() {
				t.Helper()
				products, err := app.ListProducts(ctx, nil, true)
				activities, activitiesErr := app.ListActivities(ctx, 100)
				if err != nil || activitiesErr != nil || len(products) != 0 || !reflect.DeepEqual(activities, before) {
					t.Fatalf("unconfirmed setup changed the ledger: %v, %v", err, activitiesErr)
				}
			}
			d := fill()
			click("Review")
			click("Back to details")
			h.focused(&d.form.principal.Editor)
			if d.form.principal.Text() != principal {
				t.Fatal("back discarded product input")
			}
			click("Review")
			click("Cancel")
			h.focused(&funds.addProduct)
			noWrite()

			d = fill()
			click("Review")
			stale := d.plan.mutation.String()
			now = now.Add(24 * time.Hour)
			h.press(key.NameReturn)
			settle()
			noWrite()
			if d.plan != nil || d.form.principal.Text() != principal || funds.productSetupUnresolved() {
				t.Fatal("stale product review lost the draft")
			}
			h.focused(&d.form.principal.Editor)
			click("Review")
			plan := d.plan.mutation.String()
			wantCash, wantValue := "105.8643", principal
			terms := application.ProductTermsInput{Kind: "term_deposit", Name: title, StartOn: start, MaturityOn: productOptional(maturity),
				InterestMode: "simple_act_360", AnnualRatePercent: productOptional("3.1259")}
			zero := 0
			policy := application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: terms.MaturityOn, SettlementDays: &zero, DayBasis: productOptional("calendar"),
				NormalExitFee: productOptional("0.0021"), EarlyKind: "not_allowed"}
			command := liquidity.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{AccountID: refIDs.account.String(), Currency: "USD", Principal: principal,
				OpeningFee: productOptional(fee), EffectiveLocalDate: start, EffectiveLocalTime: "18:00", Terms: terms, Policy: policy}}
			if existing {
				wantCash, wantValue = "156", "52.0123"
				terms.Kind, terms.InterestMode, terms.AnnualRatePercent = "locked_product", "none", nil
				policy.AccessKind, policy.UnlockOn = "on_request", nil
				command = liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{AccountID: refIDs.account.String(), Currency: "EUR", Principal: principal,
					CurrentValue: wantValue, TotalCostBasis: "49.7891", CashExcludesProduct: true, Terms: terms, Policy: policy}}
			}
			adapter := liquidity.NewService(ref)
			want, err := adapter.PreviewProductOperation(ctx, command)
			got := liquidity.FromProductOperationPreview(d.plan.review)
			if err != nil || got.EffectiveAt != want.EffectiveAt || got.Timezone != "Asia/Singapore" || !reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) {
				t.Fatalf("Wails preview differs: %+v / %+v %v", got, want, err)
			}
			refPlan := domain.NewProductOperationID().String()
			if _, err := adapter.RecordProductOperation(ctx, liquidity.RecordProductOperationRequest{Command: command, MutationID: refPlan, ReviewedStateHash: want.ReviewedStateHash}); err != nil {
				t.Fatal(err)
			}
			fault.loseNext = true
			h.press(key.NameReturn)
			settle()
			if !funds.productSetupUnresolved() || d.plan.mutation.String() != plan {
				t.Fatal("lost acknowledgement did not retain the posted plan")
			}
			h.press(key.NameReturn)
			settle()
			if !fault.replayed || !reflect.DeepEqual(fault.plans, []string{stale, plan, plan}) || funds.setup != nil {
				t.Fatalf("product did not replay exactly once: %+v", fault.plans)
			}
			h.focused(&funds.addProduct)
			label("Product recorded.")
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
			if err := refSession.Close(); err != nil {
				t.Fatal(err)
			}
			actual, _ := reopenOnboardingProfile(t, paths)
			reference, _ := reopenOnboardingProfile(t, refPaths)
			for _, copy := range []struct {
				app     *application.Service
				account domain.AccountID
				plan    string
				at      string
				before  []domain.Activity
			}{{actual, ids.account, plan, want.EffectiveAt, before}, {reference, refIDs.account, refPlan, want.EffectiveAt, nil}} {
				copy.app.SetClock(clock)
				assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": wantCash})
				products, err := copy.app.ListProducts(ctx, nil, true)
				if err != nil || len(products) != 1 {
					t.Fatalf("unexpected reopened products: %+v %v", products, err)
				}
				p := products[0]
				if p.Contract.Name != title || p.Contract.AccountID != copy.account || p.Contract.OpenedOperationID.String() != copy.plan ||
					p.Contract.Principal.CanonicalAmount() != principal || p.Contract.MaturityOn == nil || *p.Contract.MaturityOn != maturity ||
					p.CurrentValue == nil || p.CurrentValue.CanonicalAmount() != wantValue || p.Policy.NormalExitFee == nil || p.Policy.NormalExitFee.CanonicalAmount() != "0.0021" {
					t.Fatalf("unexpected reopened product terms: %+v", p)
				}
				if !existing && (p.Contract.AnnualRate == nil || p.Contract.AnnualRate.Canonical() != "0.031259" || p.Contract.InterestMode != domain.InterestSimpleAct360 || p.Policy.UnlockOn == nil || *p.Policy.UnlockOn != maturity) {
					t.Fatalf("deposit rate or availability rule changed: %+v", p)
				}
				operations, err := copy.app.ListProductOperations(ctx, p.Contract.ID, "", 10)
				if err != nil || len(operations.Operations) != 1 || operations.Operations[0].ID.String() != copy.plan {
					t.Fatalf("operation identity did not reopen: %+v %v", operations, err)
				}
				activities, err := copy.app.ListActivities(ctx, 100)
				at, parseErr := time.Parse(time.RFC3339Nano, copy.at)
				if err != nil || parseErr != nil || len(activities) != len(before)+1 || !activities[0].EffectiveAt.Equal(at) || activities[0].ProductContext == nil || activities[0].ProductContext.ProductID != p.Contract.ID {
					t.Fatalf("unexpected journal after reopen: %+v %v", activities, err)
				}
				if copy.before != nil && !reflect.DeepEqual(activities[1:], copy.before) {
					t.Fatal("standalone setup changed prior activities")
				}
				if existing {
					costFound := false
					for _, effect := range activities[0].Effects {
						costFound = costFound || effect.CostUnitPrice != nil && effect.HoldingID != nil && *effect.HoldingID == p.Contract.HoldingID && effect.CostUnitPrice.Canonical() == "49.7891"
					}
					if !costFound || p.Contract.Currency != "EUR" {
						t.Fatal("existing product cost or currency did not reopen")
					}
					if read, err := readProduct(ctx, copy.app, p.Contract.ID); err != nil || read.detail == nil || read.canConfirmReceipt() {
						t.Fatalf("locked product detail or receipt eligibility incorrect: %+v %v", read, err)
					}
				}
			}
		})
	}
}
