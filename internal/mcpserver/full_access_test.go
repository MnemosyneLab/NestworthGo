package mcpserver

import (
	"context"
	"sync"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func fullPreview(t *testing.T, fx hostFixture) HostChangeReview {
	t.Helper()
	r, e := fx.local.PreviewFullChange(t.Context(), hostCommand(fx))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestFullAccessAuthorityAndConcurrentReceipt(t *testing.T) {
	fx := newHostFixture(t)
	human := hostPreview(t, fx)
	_, err := fx.local.ExecuteFullChange(t.Context(), human.Binding.ID)
	requireLocalError(t, err, "permission_denied")
	r := fullPreview(t, fx)
	_, err = fx.local.ConfirmHostChange(t.Context(), r.Binding.ID, r.Digest)
	requireLocalError(t, err, "permission_denied")
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			got, e := fx.local.ExecuteFullChange(t.Context(), r.Binding.ID)
			if e != nil || got.State != ReviewSucceeded || got.Binding.Source != "ai_full" {
				t.Errorf("result=%+v err=%v", got, e)
			}
		})
	}
	wg.Wait()
	assertActivityCount(t, fx, 1)
	// Complete business commands must use shared validation, never a generic DB writer.
	for _, kind := range []history.ChangeCommandKind{"batch", "directory_patch", "trade", "position_import", "reconciliation"} {
		c := hostCommand(fx)
		c.Kind = kind
		if _, e := fx.local.PreviewFullChange(t.Context(), c); e == nil {
			t.Fatalf("allowed %s", kind)
		}
	}
	transfer := history.ChangeCommandRequest{Kind: history.ChangeCashTransfer, FromAccountID: fx.checking, ToAccountID: fx.savings, Sent: "2", SentCurrency: "USD", Received: "2", ReceivedCurrency: "USD", EffectiveAt: "2026-09-28T13:00:00Z"}
	r, err = fx.local.PreviewFullChange(t.Context(), transfer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.local.ExecuteFullChange(t.Context(), r.Binding.ID); err != nil {
		t.Fatal(err)
	}
	assertActivityCount(t, fx, 2)
}
func TestFullAccessFailuresNeverReplay(t *testing.T) {
	for _, phase := range []string{"admission_failed", "admission_uncertain", "result_failed", "revoked", "cancelled", "stale"} {
		t.Run(phase, func(t *testing.T) {
			fx := newHostFixture(t)
			repo := &failingReviewRepository{ConfigurationRepository: fx.local.service.repository}
			fx.local.service.repository = repo
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch phase {
			case "admission_failed":
				repo.failAt = 2
			case "admission_uncertain":
				repo.failAt = 2
				repo.saveThen = true
			case "result_failed":
				repo.failAt = 3
			case "revoked":
				repo.after = func() { _ = fx.local.RevokeForRestore(t.Context()) }
			case "cancelled":
				repo.after = cancel
			}
			r := fullPreview(t, fx)
			if phase == "stale" {
				h := hostPreview(t, fx)
				if _, e := fx.local.ConfirmHostChange(t.Context(), h.Binding.ID, h.Digest); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := fx.local.ExecuteFullChange(ctx, r.Binding.ID); e == nil {
				t.Fatal("expected failure")
			}
			count := 0
			if phase == "result_failed" || phase == "stale" {
				count = 1
			}
			assertActivityCount(t, fx, count)
			if phase == "revoked" {
				return
			}
			got, e := fx.local.InspectHostChange(t.Context(), r.Binding.ID)
			if e != nil {
				t.Fatal(e)
			}
			expected := map[string]string{"admission_failed": ReviewAwaitingConfirmation, "admission_uncertain": ReviewUnknown, "result_failed": ReviewSucceeded, "cancelled": ReviewCancelled, "stale": ReviewNeedsPreview}[phase]
			if got.State != expected {
				t.Fatalf("%s != %s", got.State, expected)
			}
			if phase == "admission_uncertain" {
				_, e = fx.local.ExecuteFullChange(t.Context(), r.Binding.ID)
				requireLocalError(t, e, "operation_outcome_unknown")
				requireLocalError(t, fx.local.CancelHostChange(t.Context(), r.Binding.ID), "operation_outcome_unknown")
			}
			assertActivityCount(t, fx, count)
		})
	}
}

func TestFullTransferSQLFailureRollsBackAllLegs(t *testing.T) {
	fx := newHostFixture(t)
	cash := fullPreview(t, fx)
	if _, err := fx.local.ExecuteFullChange(t.Context(), cash.Binding.ID); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := fx.db.SQL.QueryRow("SELECT count(*) FROM account_cash_values").Scan(&before); err != nil {
		t.Fatal(err)
	}
	command := history.ChangeCommandRequest{Kind: history.ChangeCashTransfer, FromAccountID: fx.checking, ToAccountID: fx.savings, Sent: "2", SentCurrency: "USD", Received: "2", ReceivedCurrency: "USD", EffectiveAt: "2026-09-28T13:00:00Z"}
	r, err := fx.local.PreviewFullChange(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.db.SQL.Exec(`CREATE TRIGGER fail_full_receipt BEFORE INSERT ON activity_mutation_keys BEGIN SELECT RAISE(ABORT,'synthetic Full receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	version := fx.app.MobileDataVersion()
	if _, err = fx.local.ExecuteFullChange(t.Context(), r.Binding.ID); err == nil {
		t.Fatal("expected SQL rollback")
	}
	assertActivityCount(t, fx, 1)
	var after int
	if err = fx.db.SQL.QueryRow("SELECT count(*) FROM account_cash_values").Scan(&after); err != nil || before != after || version != fx.app.MobileDataVersion() {
		t.Fatalf("partial write before=%d after=%d err=%v", before, after, err)
	}
	if _, err = fx.db.SQL.Exec("DROP TRIGGER fail_full_receipt"); err != nil {
		t.Fatal(err)
	}
	// Even after fixing the storage fault, inspecting/repeating the unresolved
	// identity cannot dispatch again. A write lock alone would not pass this test.
	_, err = fx.local.ExecuteFullChange(t.Context(), r.Binding.ID)
	requireLocalError(t, err, "operation_outcome_unknown")
	assertActivityCount(t, fx, 1)
}
