package desktop

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func seedCashReceiptProfile(t *testing.T) pilot.Paths {
	t.Helper()
	ctx := t.Context()
	paths := prepareOnboardingProfile(t)
	app, session := reopenBaselineProfile(t, paths)
	testfixture.SetClock(app, func() time.Time { return accountTestClock.Add(-48 * time.Hour) })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic receipts", BaseCurrency: "USD", MemberNames: []string{"Example owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ name, typ, mode, initial string }{
		{"Harbor Checking", "bank_account", "balance", "100"},
		{"Cash Studio", "brokerage", "holdings", ""},
	} {
		row, err := testfixture.CreateAccount(app, ctx, application.AccountInput{
			Name: item.name, AccountType: item.typ, TrackingMode: item.mode, BalanceSheetRole: "asset",
			DefaultCurrency: "USD", InitialAmount: item.initial, OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if item.mode == "holdings" {
			for currency, amount := range map[string]string{"USD": "200", "EUR": "15"} {
				if _, err := testfixture.AppendAccountCashValue(app, ctx, row.Account.ID, amount, currency, ""); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if _, err := testfixture.CaptureSyntheticOrigin(app, ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func prepareCashReceiptReview(t *testing.T, page *historyCashMovements, movement history.CashMovement) {
	t.Helper()
	page.start(movement)
	page.workers.Wait()
	page.update()
	draft := page.draft
	if draft.sources == nil {
		t.Fatalf("could not load cash sources: %v", draft.sourceError)
	}
	for _, account := range draft.sources.Accounts {
		switch account.Name {
		case "Harbor Checking":
			if movement == history.CashTransfer {
				draft.chooseSource(account.ID)
			}
		case "Cash Studio":
			if movement == history.CashTransfer {
				draft.chooseDestination(account.ID)
			} else {
				draft.chooseSource(account.ID)
			}
		}
	}
	draft.chooseCurrency("USD")
	if movement == history.CashConversion {
		draft.chooseToCurrency("EUR")
		draft.received.Editor.SetText("20")
		draft.fee.Editor.SetText("1")
	}
	draft.amount.Editor.SetText("25")
	page.preview()
	page.workers.Wait()
	page.update()
	if draft.review == nil {
		t.Fatalf("could not preview synthetic cash movement: %v", draft.problem)
	}
}

type cashReceiptCrashLedger struct {
	*application.Service
	afterCommit bool
	stop        chan<- string
}

func (l cashReceiptCrashLedger) RecordChangeGuarded(ctx context.Context, cmd any, mutation, digest, token string) (domain.ChangePreview, error) {
	var id domain.ActivityID
	if l.afterCommit {
		receipt, err := l.Service.RecordChangeGuarded(ctx, cmd, mutation, digest, token)
		if err != nil {
			l.stop <- "error " + err.Error()
			return receipt, err
		}
		id = receipt.Activity.ID
	}
	l.stop <- "cash-receipt-boundary " + mutation + " " + id.String()
	// No acknowledgement reaches the result channel. The parent sends SIGKILL
	// only after receiving the boundary, without running Session.Close.
	<-ctx.Done()
	return domain.ChangePreview{}, ctx.Err()
}

func TestCashReceiptProcessBoundary(t *testing.T) {
	if mode := os.Getenv("GIO_TEST_CASH_RECEIPT_BOUNDARY"); mode != "" {
		movement, err := strconv.Atoi(os.Getenv("GIO_TEST_CASH_RECEIPT_MOVEMENT"))
		if err != nil {
			t.Fatal(err)
		}
		paths, err := pilot.PathsFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		app, session := reopenBaselineProfile(t, paths)
		stop := make(chan string, 1)
		page := newHistoryCashMovements(t.Context(), cashReceiptCrashLedger{app, mode == "after", stop},
			func() time.Time { return accountTestClock }, func() {})
		page.recovery = &cashReceiptRecovery{store: session.CashReceipts}
		prepareCashReceiptReview(t, page, history.CashMovement(movement))
		page.post()
		select {
		case notice := <-stop:
			fmt.Println(notice)
		case <-time.After(10 * time.Second):
			page.update()
			t.Fatalf("never reached dispatch: %+v", page.draft)
		}
		<-t.Context().Done()
		return
	}

	for _, movement := range []struct {
		name string
		kind history.CashMovement
	}{{"transfer", history.CashTransfer}, {"conversion", history.CashConversion}} {
		for _, boundary := range []string{"before", "after"} {
			t.Run(movement.name+"/"+boundary, func(t *testing.T) {
				paths := seedCashReceiptProfile(t)
				mutation, activity := runCashReceiptCrash(t, movement.kind, boundary)
				app, session := reopenBaselineProfile(t, paths)
				pending, err := session.CashReceipts.Pending(t.Context())
				if err != nil || len(pending) != 1 || !pending[0].MatchesLedger || pending[0].MutationID != mutation {
					t.Fatalf("lost the frozen identity at %s: %+v / %v", boundary, pending, err)
				}

				page, timeline, h, finish := cashMovementHarness(t, app)
				recovery := &cashReceiptRecovery{ctx: t.Context(), store: session.CashReceipts, reader: app, invalidate: func() {}}
				page.recovery = recovery
				recovery.read()
				recovery.workers.Wait()
				h.click("Overview", false)
				finish()

				if boundary == "before" {
					assertPageLabel(t, &h.router, i18n.English.Text(i18n.CashReceiptAbsent))
					if findPageLabel(&h.router, i18n.English.Text(i18n.CashReceiptOpen)) != nil {
						t.Fatal("a missing receipt offered an activity link")
					}
					assertCashReceiptBalances(t, app, movement.kind, 0)
					return
				}

				assertPageLabel(t, &h.router, i18n.English.Text(i18n.CashReceiptRecorded))
				assertCashReceiptBalances(t, app, movement.kind, 1)
				h.click(i18n.English.Text(i18n.CashReceiptOpen), false)
				finish()
				detail := timeline.reads.detail
				if detail == nil || detail.id != activity || detail.record == nil {
					t.Fatalf("recovered receipt did not open the original activity %s: %+v", activity, detail)
				}
				h.click(i18n.English.Text(i18n.CashReceiptDismiss), false)
				recovery.workers.Wait()
				finish()
				pending, err = session.CashReceipts.Pending(t.Context())
				if err != nil || len(pending) != 0 {
					t.Fatalf("dismiss did not clear the private identity: %+v / %v", pending, err)
				}
				assertCashReceiptBalances(t, app, movement.kind, 1)

				h.click("Ledger activity", false)
				finish()
				prepareCashReceiptReview(t, page, movement.kind)
				newID, digest := page.draft.review.ReceiptIdentity()
				// Identical form data, including effective time, can generate
				// the same hash. Only a fresh mutation ID denotes new intent.
				first, err := app.InspectChangeReceipt(t.Context(), mutation, digest)
				if err != nil || first == nil || first.Activity.ID != activity || newID == mutation {
					t.Fatalf("independent identical intent failed: %v / %v / %s", first, err, newID)
				}
				page.post()
				finish()
				assertCashReceiptBalances(t, app, movement.kind, 2)
			})
		}
	}
}

func runCashReceiptCrash(t *testing.T, movement history.CashMovement, boundary string) (string, domain.ActivityID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCashReceiptProcessBoundary$")
	cmd.Env = append(os.Environ(), "GIO_TEST_CASH_RECEIPT_BOUNDARY="+boundary, "GIO_TEST_CASH_RECEIPT_MOVEMENT="+strconv.Itoa(int(movement)))
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	scanner := bufio.NewScanner(stdout)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if result, ok := strings.CutPrefix(line, "cash-receipt-boundary "); ok {
			id, activity, _ := strings.Cut(result, " ")
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			var killed *exec.ExitError
			if err := cmd.Wait(); !errors.As(err, &killed) {
				t.Fatalf("child did not exit by signal: %v", err)
			}
			return id, domain.ActivityID(activity)
		}
	}
	t.Fatalf("child did not reach %s boundary: %v / %v", boundary, lines, scanner.Err())
	return "", ""
}

func assertCashReceiptBalances(t *testing.T, app *application.Service, movement history.CashMovement, count int) {
	t.Helper()
	ctx := t.Context()
	activities, err := app.ListActivities(ctx, 10)
	if err != nil || len(activities) != count {
		t.Fatalf("activities = %d, want %d: %v", len(activities), count, err)
	}
	rows, err := app.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	bank, usd, eur := 100, 200, 15
	switch movement {
	case history.CashTransfer:
		bank -= 25 * count
		usd += 25 * count
	case history.CashConversion:
		usd -= 26 * count
		eur += 20 * count
	}
	for _, row := range rows {
		switch row.Account.Name {
		case "Harbor Checking":
			if got := row.LatestValue.Amount.CanonicalAmount(); got != strconv.Itoa(bank) {
				t.Fatalf("bank balance = %s, want %d", got, bank)
			}
		case "Cash Studio":
			cash, err := app.ListAccountCashValues(ctx, row.Account.ID)
			if err != nil {
				t.Fatal(err)
			}
			latest := make(map[domain.CurrencyCode]string)
			for _, value := range cash {
				currency := value.Amount.Currency()
				if _, exists := latest[currency]; !exists {
					latest[currency] = value.Amount.CanonicalAmount()
				}
			}
			if latest["USD"] != strconv.Itoa(usd) || latest["EUR"] != strconv.Itoa(eur) {
				t.Fatalf("cash balances = %v, want USD %d and EUR %d", latest, usd, eur)
			}
		}
	}
}

type receiptReadFailure struct {
	cashReceiptReader
}

func (receiptReadFailure) InspectChangeReceipt(context.Context, string, string) (*domain.ChangePreview, error) {
	return nil, errors.New("synthetic receipt read failure")
}

func TestCashReceiptConflictAndReadFailureOnlyInspect(t *testing.T) {
	for _, conflict := range []bool{true, false} {
		name := "read failure"
		if conflict {
			name = "different digest"
		}
		t.Run(name, func(t *testing.T) {
			paths := seedCashReceiptProfile(t)
			app, session := reopenBaselineProfile(t, paths)
			page, _, h, finish := cashMovementHarness(t, app)
			prepareCashReceiptReview(t, page, history.CashTransfer)
			review := page.draft.review
			page.discard()
			mutation, digest := review.ReceiptIdentity()
			if conflict {
				digest = strings.Repeat("a", 64)
			}
			if err := session.CashReceipts.Remember(t.Context(), mutation, digest); err != nil {
				t.Fatal(err)
			}
			posted, err := review.Commit(t.Context(), app)
			if err != nil {
				t.Fatal(err)
			}
			recovery := &cashReceiptRecovery{ctx: t.Context(), store: session.CashReceipts, reader: app, invalidate: func() {}}
			if !conflict {
				recovery.reader = receiptReadFailure{app}
			}
			page.recovery = recovery
			recovery.read()
			recovery.workers.Wait()
			finish()
			assertPageLabel(t, &h.router, i18n.English.Text(i18n.CashReceiptUnconfirmed))
			if findPageLabel(&h.router, i18n.English.Text(i18n.CashReceiptOpen)) != nil {
				t.Fatal("unconfirmed receipt offered a history link")
			}
			assertCashReceiptBalances(t, app, history.CashTransfer, 1)
			if !conflict {
				recovery.reader = app
				h.click(i18n.English.Text(i18n.CashReceiptRetry), false)
				recovery.workers.Wait()
				finish()
				assertPageLabel(t, &h.router, i18n.English.Text(i18n.CashReceiptRecorded))
				if recovery.notices[0].activity != posted.Activity.ID {
					t.Fatal("retry did not read the original activity")
				}
				assertCashReceiptBalances(t, app, history.CashTransfer, 1)
			}
		})
	}
}

type cashReceiptSaveFailure struct {
	cashReceiptStore
}

func (cashReceiptSaveFailure) Remember(context.Context, string, string) error {
	return errors.New("synthetic receipt file flush failure")
}

func TestCashReceiptWriteErrorDoesNotDispatchAndSameProcessRetryKeepsIdentity(t *testing.T) {
	paths := seedCashReceiptProfile(t)
	app, session := reopenBaselineProfile(t, paths)
	fault := &cashMovementFault{Service: app, loseAttempt: 1, started: make(chan struct{}), release: make(chan struct{})}
	close(fault.release)
	page, timeline, h, finish := cashMovementHarness(t, fault)
	recovery := &cashReceiptRecovery{ctx: t.Context(), store: cashReceiptSaveFailure{session.CashReceipts}, reader: app, invalidate: func() {}}
	page.recovery = recovery
	h.click("Record change", false)
	h.click("Record transfer", false)
	finish()
	prepareCashReceiptReview(t, page, history.CashTransfer)
	id, digest := page.draft.review.ReceiptIdentity()
	h.frame()
	h.click("Confirm transfer", false)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.CashReceiptSaveFailed))
	if len(fault.attempts) != 0 {
		t.Fatal("failed sidecar write dispatched a ledger mutation")
	}
	assertCashReceiptBalances(t, app, history.CashTransfer, 0)

	recovery.store = session.CashReceipts
	h.click("Confirm transfer", false)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.ContributionUncertain))
	if len(fault.attempts) != 1 {
		t.Fatal("expected one recorded attempt with a lost response")
	}
	assertCashReceiptBalances(t, app, history.CashTransfer, 1)
	h.click("Retry same submission", false)
	finish()
	if len(fault.attempts) != 2 || timeline.reads.detail.id != fault.firstActivity {
		t.Fatal("same-process retry did not resolve the original activity")
	}
	for _, attempt := range fault.attempts {
		if attempt.mutation != id || attempt.hash != digest {
			t.Fatal("same-process retry changed the frozen receipt identity")
		}
	}
	pending, err := session.CashReceipts.Pending(t.Context())
	if err != nil || len(pending) != 1 || pending[0].MutationID != id || pending[0].SHA256 != digest {
		t.Fatalf("saved identity was lost or duplicated: %+v / %v", pending, err)
	}
	assertCashReceiptBalances(t, app, history.CashTransfer, 1)
}
