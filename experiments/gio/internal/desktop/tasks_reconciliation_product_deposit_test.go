package desktop

import (
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func seedLinkedTermDeposit(t *testing.T, app *application.Service, account domain.AccountID) domain.ProductContractID {
	t.Helper()
	app.SetClock(linkedContractClock)
	maturity := "2026-11-23"
	days, zero, basis := 0, "0", string(domain.DayBasisCalendar)
	command := application.ProductCommand{Kind: domain.ProductOpOpen, Open: &application.OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "50.1234", OpeningFee: productOptional("0.0123"),
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "09:00",
		Terms: application.ProductTermsInput{Kind: string(domain.ProductTermDeposit), Name: "Synthetic deposit checkpoint", StartOn: "2026-11-22", MaturityOn: &maturity,
			InterestMode: string(domain.InterestSimpleAct365), AnnualRatePercent: productOptional("2.5001")},
		Policy: application.ProductPolicyInput{AccessKind: string(domain.AccessOnDate), UnlockOn: &maturity, SettlementDays: &days,
			DayBasis: &basis, NormalExitFee: &zero, EarlyKind: string(domain.EarlyNotAllowed)},
	}}
	preview, err := app.PreviewProductOperation(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := app.RecordProductOperation(t.Context(), command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil || len(receipt.ProductIDs) != 1 {
		t.Fatalf("could not seed a term deposit: %v", err)
	}
	return receipt.ProductIDs[0]
}

func TestLinkedTermDepositMatchesWails(t *testing.T) {
	for _, closing := range []struct {
		name, label, endCash string
		effects              int
	}{{"settle", "Confirm actual receipt", "159.5555", 2}, {"renew", "Record actual renewal", "119.4444", 3}} {
		t.Run(closing.name, func(t *testing.T) {
			ctx := t.Context()
			app, storage, paths, ids := openExistingBuyFixture(t)
			ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
			id := seedLinkedTermDeposit(t, app, ids.account)
			refID := seedLinkedTermDeposit(t, ref, refIDs.account)
			api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
			session := startTestHoldingReconciliation(t, api, ids.account)
			refSession := startTestHoldingReconciliation(t, refAPI, refIDs.account)
			fault := &linkedProductCommitFault{holdingReconcileAPI: api}
			h := newLinkedProductHarness(t, app, fault)
			c := h.c
			itemID := "product:" + id.String()
			buttons := c.checkpointActions(ids.account.String(), itemID)
			if findPageLabel(&h.router, "Update contract valuation") != nil {
				t.Fatal("term deposit offered a locked-product valuation action")
			}
			refLink := func() *domain.ReconciliationLink {
				t.Helper()
				current, err := refAPI.Get(ctx, refSession.ID)
				if err != nil {
					t.Fatal(err)
				}
				return &domain.ReconciliationLink{SessionID: current.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
					ExpectedRevision: current.Revision, FinancialRevision: current.Accounts[0].FinancialRevision}
			}
			open := func(label string) *holdingReconDraft {
				t.Helper()
				h.activate(t, label)
				d := c.draft
				if d == nil || d.product.id != id || d.product.kind != domain.ProductTermDeposit || d.product.link().ItemID != itemID {
					t.Fatalf("wrong checkpoint in %s", label)
				}
				return d
			}
			confirm := func() { h.press(key.NameReturn); h.settle() }
			label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
			before, err := app.ListActivities(ctx, 100)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := app.Product(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			const rate = "3.1259"
			editTerms := func(d *holdingReconDraft) {
				h.focused(&d.product.terms.name.Editor)
				d.product.terms.terms.InterestMode = string(domain.InterestSimpleAct360)
				d.product.terms.deposit.rate.Editor.SetText(rate)
				h.settle()
				h.activate(t, "Review")
				label("Annual rate (%) · 2.5001% → 3.1259%")
			}
			d := open("Edit terms and rules")
			editTerms(d)
			h.activate(t, "Back to details")
			h.focused(&d.product.terms.name.Editor)
			if d.product.terms.deposit.rate.Text() != rate {
				t.Fatal("Back lost the deposit rate")
			}
			h.activate(t, "Cancel")
			h.focused(&buttons.terms)
			cancelled, err := app.Product(ctx, id)
			if err != nil || !reflect.DeepEqual(cancelled.Contract, initial.Contract) {
				t.Fatalf("terms cancellation changed deposit: %v", err)
			}
			d = open("Edit terms and rules")
			editTerms(d)
			termsPlan := d.review.planID
			refDetail, err := ref.Product(ctx, refID)
			if err != nil {
				t.Fatal(err)
			}
			terms, policy := productTermInputs(liquidity.FromProductDetail(refDetail).Product)
			terms.InterestMode, terms.AnnualRate, terms.AnnualRatePercent = "simple_act_360", nil, productOptional(rate)
			termsReview, err := refAPI.PreviewProductTerms(ctx, liquidity.UpdateProductTermsRequest{ProductID: refID.String(), ExpectedRevision: refDetail.Contract.Revision,
				ReconciliationLink: refLink(), Terms: terms, Policy: policy})
			if err != nil || termsReview.After == nil || !reflect.DeepEqual(termsReview.After.Product.AnnualRate, d.review.terms.After.Product.AnnualRate) {
				t.Fatalf("Wails terms differ: %v", err)
			}
			if err := refAPI.CommitProduct(ctx, termsReview.PlanID); err != nil {
				t.Fatal(err)
			}
			confirm()
			h.focused(&buttons.terms)
			const paid, day, minute, interest = "2026-11-23", "2026-11-23", "12:00", "2.3456"
			editInterest := func(d *holdingReconDraft) {
				h.focused(&d.total.Editor)
				d.total.Editor.SetText(interest)
				d.product.interest.day, d.product.interest.minute = day, minute
				h.settle()
			}
			d = open("Record interest received")
			editInterest(d)
			h.activate(t, "Review")
			label("Could not review this interest receipt. Check the amount and dates and try again.")
			d.product.interest.paidOn = paid
			h.activate(t, "Review")
			label("Interest paid through · 2026-11-23")
			label("Account cash after · 108.2099 USD")
			stale := d.review.planID
			current, err := api.Get(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			check, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: current.ID, ExpectedRevision: current.Revision,
				AccountID: ids.account.String(), ItemID: itemID, FinancialRevision: current.Accounts[0].FinancialRevision, Outcome: "skipped", Note: "Independent evidence pending."})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := api.Commit(ctx, check.PlanID); err != nil {
				t.Fatal(err)
			}
			confirm()
			if d.review != nil || d.total.Text() != interest || d.product.interest.paidOn != paid {
				t.Fatal("stale confirmation cleared the interest receipt")
			}
			h.focused(&d.total.Editor)
			h.activate(t, "Review")
			interestPlan := d.review.planID
			if interestPlan == stale {
				t.Fatal("stale plan reused for a new review")
			}
			compareAndPost := func(request liquidity.ProductCommandRequest) string {
				t.Helper()
				request.ReconciliationLink = refLink()
				refPreview, err := refAPI.PreviewProductOperation(ctx, request)
				if err != nil || refPreview.Operation == nil || c.draft.review == nil {
					t.Fatalf("shared receipt preview unavailable: %v", err)
				}
				got, want := c.draft.review.operation, refPreview.Operation
				if got == nil || got.Kind != want.Kind || got.EffectiveAt != want.EffectiveAt || len(got.Activities) != len(want.Activities) ||
					!reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) {
					t.Fatalf("receipt differs from Wails: %+v / %+v", got, want)
				}
				if err := refAPI.CommitProduct(ctx, refPreview.PlanID); err != nil {
					t.Fatal(err)
				}
				return refPreview.PlanID
			}
			refInterestPlan := compareAndPost(liquidity.ProductCommandRequest{Kind: "receive_interest", ReceiveInterest: &application.ReceiveInterestCommand{
				ProductID: refID.String(), Amount: interest, InterestPaidThroughOn: productOptional(paid), EffectiveLocalDate: day, EffectiveLocalTime: minute}})
			fault.loseNext = true
			confirm()
			if !c.unresolved() || c.draft.review.planID != interestPlan {
				t.Fatal("lost acknowledgement cleared the operation identity")
			}
			confirm()
			h.focused(&buttons.interest)
			const returned, extraInterest, fee = "50.1234", "1.2345", "0.0123"
			settle := application.SettleProductCommand{ProductID: refID.String(), ReturnedPrincipal: productOptional(returned), Interest: productOptional(extraInterest),
				Fee: productOptional(fee), EffectiveLocalDate: day, EffectiveLocalTime: "18:00"}
			request := liquidity.ProductCommandRequest{Kind: closing.name, Settle: &settle}
			button := &buttons.settle
			editClosing := func(d *holdingReconDraft) {
				h.focused(&d.total.Editor)
				d.total.Editor.SetText(returned)
				field := d.product.settlement
				if r := d.product.renewal; r != nil {
					button = &buttons.renew
					field = &r.settlement
					r.principal.Editor.SetText("40")
					r.openingFee.Editor.SetText("0.1111")
					r.terms.name.Editor.SetText("Synthetic successor deposit")
					r.terms.terms.StartOn, r.terms.terms.MaturityOn = day, productOptional("2026-12-23")
					r.terms.changeDepositMode(string(domain.InterestManualMaturityAmount))
					r.terms.deposit.maturityInterest.Editor.SetText("3.4567")
				}
				h.router.MoveFocus(key.FocusForward)
				h.focused(&field.interest.Editor)
				h.router.MoveFocus(key.FocusForward)
				h.focused(&field.fee.Editor)
				field.interest.Editor.SetText(extraInterest)
				field.fee.Editor.SetText(fee)
				field.day, field.minute = settle.EffectiveLocalDate, settle.EffectiveLocalTime
				h.settle()
				h.activate(t, "Review")
				label("Returned principal · 50.1234 USD")
				label("Interest received · 1.2345 USD")
				label("Account cash after · " + closing.endCash + " USD")
			}
			d = open(closing.label)
			editClosing(d)
			h.activate(t, "Cancel")
			h.focused(button)
			assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "108.2099"})
			d = open(closing.label)
			editClosing(d)
			closePlan := d.review.planID
			if closing.name == "renew" {
				terms.Name, terms.StartOn, terms.MaturityOn = "Synthetic successor deposit", day, productOptional("2026-12-23")
				terms.InterestMode, terms.AnnualRatePercent, terms.InterestPaidThroughOn = "manual_maturity_amount", nil, nil
				terms.MaturityInterest, policy.UnlockOn = productOptional("3.4567"), terms.MaturityOn
				request.Settle = nil
				request.Renew = &application.RenewProductCommand{Settle: settle, Principal: "40", OpeningFee: productOptional("0.1111"), Terms: terms, Policy: policy}
			}
			refClosePlan := compareAndPost(request)
			confirm()
			if !reflect.DeepEqual(fault.plans, []string{termsPlan, stale, interestPlan, interestPlan, closePlan}) {
				t.Fatalf("unexpected commit IDs: %v", fault.plans)
			}
			if err := storage.Close(); err != nil {
				t.Fatal(err)
			}
			if err := refStorage.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, _ := reopenOnboardingProfile(t, paths)
			reference, _ := reopenOnboardingProfile(t, refPaths)
			for _, copy := range []struct {
				app                               *application.Service
				account                           domain.AccountID
				id                                domain.ProductContractID
				session, terms, interest, closing string
			}{{reopened, ids.account, id, session.ID, termsPlan, interestPlan, closePlan},
				{reference, refIDs.account, refID, refSession.ID, termsReview.PlanID, refInterestPlan, refClosePlan}} {
				copy.app.SetClock(linkedContractClock)
				assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": closing.endCash})
				saved, err := copy.app.Product(ctx, copy.id)
				p := saved.Contract
				if err != nil || p.State != domain.ProductStateSettled || p.ClosedOperationID == nil || p.ClosedOperationID.String() != copy.closing ||
					p.Principal.CanonicalAmount() != returned || p.AnnualRate == nil || p.AnnualRate.Canonical() != "0.031259" ||
					p.InterestMode != domain.InterestSimpleAct360 || p.InterestPaidThroughOn == nil || *p.InterestPaidThroughOn != paid {
					t.Fatalf("unexpected reopened principal, terms or receipt: %+v %v", p, err)
				}
				if closing.name == "renew" {
					r, err := copy.app.ListProducts(ctx, &copy.account, false)
					if err != nil || len(r) != 1 || r[0].Contract.OpenedOperationID.String() != copy.closing || r[0].Contract.RenewedFromID == nil ||
						*r[0].Contract.RenewedFromID != copy.id || r[0].Contract.Principal.CanonicalAmount() != "40" ||
						r[0].Contract.MaturityInterest == nil || r[0].Contract.MaturityInterest.CanonicalAmount() != "3.4567" {
						t.Fatalf("renewal successor did not reopen: %+v %v", r, err)
					}
				}
				links, err := reconciliation.NewService(copy.app).Links(ctx, copy.session)
				if err != nil || len(links) != 4+closing.effects {
					t.Fatalf("wrong reopened link count: %+v %v", links, err)
				}
				operations := map[string]bool{copy.terms: false, copy.interest: false, copy.closing: false}
				activityIDs := make(map[string]bool)
				for _, link := range links {
					if link.ItemID != "product:"+copy.id.String() || link.AccountID != copy.account.String() {
						t.Fatalf("wrong receipt context: %+v", link)
					}
					if link.Family == "activity" {
						activityIDs[link.OperationID] = true
					} else {
						operations[link.OperationID] = true
					}
				}
				if len(operations) != 3 || !operations[copy.terms] || !operations[copy.interest] || !operations[copy.closing] {
					t.Fatalf("wrong receipt identities: %v", operations)
				}
				activities, err := copy.app.ListActivities(ctx, 100)
				count := 1 + closing.effects
				if err != nil || len(activities) != len(before)+count {
					t.Fatalf("unexpected change count: %v", err)
				}
				for _, activity := range activities[:count] {
					if !activityIDs[activity.ID.String()] || activity.EffectiveAt.In(time.FixedZone("SGT", 8*60*60)).Format(time.DateOnly) != day {
						t.Fatalf("activity linkage or date changed on reopen: %v", activity)
					}
				}
				if copy.app == reopened && !reflect.DeepEqual(activities[count:], before) {
					t.Fatal("unrelated financial activity changed")
				}
			}
		})
	}
}
