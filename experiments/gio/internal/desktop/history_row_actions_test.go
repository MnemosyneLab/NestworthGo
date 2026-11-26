package desktop

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHistoryRowActionsReuseDraftsAndCheckEligibilityAgain(t *testing.T) {
	ctx := t.Context()
	service, _, _, bank := seedBankBalanceFixture(t, true)
	adapter := historyapi.NewService(service)
	source, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: bank.String(), Amount: "25", Currency: "USD", Reason: "contribution",
		EffectiveAt: wire.FormatTime(accountTestClock.Add(-time.Hour)), MutationID: domain.NewMutationID().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	page, h, settle := repeatHarness(t, service, adapter, accountTestClock)
	t.Cleanup(page.reads.workers.Wait)
	activity := domain.ActivityID(source.Activity.ID)
	trigger := page.historyActionButton(activity)
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	cancel := func() {
		t.Helper()
		click("Cancel")
		if page.reads.detail != nil {
			t.Fatal("cancel did not return to the activity list")
		}
		h.focused(trigger)
		assertRepeatActivityCount(t, service, 1)
	}

	click("More actions")
	click("Repeat as new draft")
	if detail := page.reads.detail; detail == nil || detail.id != activity || detail.repeatDraft == nil || detail.repeatDraft.request().Amount != "25" {
		t.Fatal("row Repeat did not open the existing prefilled draft for the displayed activity")
	}
	cancel()

	click("More actions")
	click("Correct amount")
	if detail := page.reads.detail; detail == nil || detail.id != activity || detail.amendment == nil ||
		detail.amendment.action != history.CorrectActivity || detail.amendment.amount.Text() != "25" {
		t.Fatal("row correction did not open the existing amount editor for the displayed activity")
	}
	cancel()

	click("More actions")
	click("Reverse activity")
	assertPageLabel(t, &h.router, "Review reversal")
	assertPageLabel(t, &h.router, "Confirm reversal")
	cancel()

	// A lifecycle change can invalidate the action after the row menu loaded.
	click("More actions")
	if err := service.ArchiveAccount(ctx, bank, true); err != nil {
		t.Fatal(err)
	}
	click("Reverse activity")
	if detail := page.reads.detail; detail == nil || detail.amendment != nil || !detail.actionUnavailable {
		t.Fatal("the detail read reused stale row eligibility")
	}
	assertPageLabel(t, &h.router, "This activity is no longer eligible for this action.")
	assertRepeatActivityCount(t, service, 1)
	if err := service.ArchiveAccount(ctx, bank, false); err != nil {
		t.Fatal(err)
	}
	click("Back to ledger activity")

	click("More actions")
	click("Reverse activity")
	assertRepeatActivityCount(t, service, 1)
	click("Confirm reversal")
	assertRepeatActivityCount(t, service, 2)
	detail := page.reads.detail
	if detail == nil {
		t.Fatal("confirmation did not open the reversal receipt")
	}
	record := detail.record
	if record == nil || record.Activity.ReversesActivityID == nil || *record.Activity.ReversesActivityID != activity {
		t.Fatal("confirmation did not open the exact reversal receipt")
	}
}
