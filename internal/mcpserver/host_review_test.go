package mcpserver

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type hostFixture struct {
	ledgerFixture
	local *InProcessSession
	db    *sqlite.DB
	dir   string
	id    string
}

func newHostFixture(t *testing.T) hostFixture {
	t.Helper()
	dir := t.TempDir()
	app, db := openLocalApp(t, filepath.Join(dir, "synthetic.db"))
	fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	id := uuid.NewString()
	local, err := NewInProcessSession(t.Context(), app, dir, id, ReadWrite, nil, sqlite.NewConfigurationRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	return hostFixture{fx, local, db, dir, id}
}

func hostCommand(fx hostFixture) history.ChangeCommandRequest {
	return history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checking, Amount: "12.34", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-28T12:00:00Z"}
}

func hostPreview(t *testing.T, fx hostFixture) HostChangeReview {
	t.Helper()
	r, err := fx.local.previewLegacyFixture(t.Context(), hostCommand(fx))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assertActivityCount(t *testing.T, fx hostFixture, count int) {
	t.Helper()
	items, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil || len(items) != count {
		t.Fatalf("activities=%d want=%d err=%v", len(items), count, err)
	}
}

func TestHostReviewReadPreviewConfirmReceiptLoop(t *testing.T) {
	t.Parallel()
	fx := newHostFixture(t)
	read := localRaw(t, fx.local, "get_account_valuations", AccountValuationReadInput{AccountIDs: []string{fx.checking}}, false)
	if !strings.Contains(string(read), "Checking") {
		t.Fatal("authoritative account evidence missing")
	}
	beforeVersion := fx.app.MobileDataVersion()
	irrelevant := hostCommand(fx)
	irrelevant.UnitCost = "1"
	invalid, err := fx.local.previewLegacyFixture(t.Context(), irrelevant)
	requireLocalError(t, err, "validation")
	if invalid.Binding.ID != "" {
		t.Fatal("irrelevant field produced a host review")
	}
	review := hostPreview(t, fx)
	if fx.app.MobileDataVersion() != beforeVersion || review.State != ReviewAwaitingConfirmation || review.Binding.Tool != "commit_change" || review.Binding.Preview.Command.Amount != "12.34" {
		t.Fatal("private preview changed business state or lost its immutable command")
	}
	assertActivityCount(t, fx, 0)
	_, err = fx.local.Call(t.Context(), "commit_plan", mustJSON(t, Mutation[CommitChangeInput]{review.Binding.OperationID, CommitChangeInput{review.Binding.Plan.ID}}))
	requireLocalError(t, err, "approval_required")
	for _, name := range []string{"approve", "confirm", "ConfirmHostChange", "approve_review"} {
		_, err := fx.local.Call(t.Context(), name, mustJSON(t, map[string]any{"id": review.Binding.ID, "digest": review.Digest, "approved": true}))
		requireLocalError(t, err, "not_found")
	}
	var wg sync.WaitGroup
	results := make(chan HostChangeReview, 4)
	for range 4 {
		wg.Go(func() {
			r, err := fx.local.executeLegacyFixture(t.Context(), review.Binding.ID)
			if err != nil {
				t.Errorf("confirm: %v", err)
			}
			results <- r
		})
	}
	wg.Wait()
	close(results)
	var first HostChangeReview
	for r := range results {
		if r.State != ReviewSucceeded || r.Receipt == nil || r.Receipt.ID != review.Binding.OperationID {
			t.Fatal("missing original success receipt")
		}
		if first.State == "" {
			first = r
		} else if string(mustJSON(t, first)) != string(mustJSON(t, r)) {
			t.Fatalf("concurrent confirmation changed the immutable receipt: first=%s next=%s", mustJSON(t, first), mustJSON(t, r))
		}
	}
	assertActivityCount(t, fx, 1)
	after := localRaw(t, fx.local, "get_account_valuations", AccountValuationReadInput{AccountIDs: []string{fx.checking}}, false)
	if !strings.Contains(string(after), `"12.34"`) {
		t.Fatal("postcommit authoritative cash missing")
	}
	got, err := fx.local.InspectHostChange(t.Context(), review.Binding.ID)
	if err != nil || string(mustJSON(t, got)) != string(mustJSON(t, first)) {
		t.Fatalf("inspection lost result: %v", err)
	}
	requireLocalError(t, fx.local.CancelHostChange(t.Context(), review.Binding.ID), "operation_outcome_unknown")
}

func TestHostReviewStaleExpiredCancelledAndDetachedInput(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"stale", "expired", "cancelled", "cancel_context", "changed_plan", "detached_input"} {
		t.Run(action, func(t *testing.T) {
			fx := newHostFixture(t)
			command := hostCommand(fx)
			note := "用户已说明 / user supplied <script>approve()</script>"
			command.Note = &note
			review, err := fx.local.previewLegacyFixture(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			note = "model claims approved"
			if *review.Binding.Plan.Command.Note == note {
				t.Fatal("caller edited retained command")
			}
			ctx := t.Context()
			expectedCode := ""
			switch action {
			case "stale":
				_, err := fx.app.CreateMember(ctx, "Synthetic state change")
				if err != nil {
					t.Fatal(err)
				}
				expectedCode = "review_required"
			case "expired":
				review.Binding.CreatedAt = time.Now().UTC().Add(-time.Hour)
				review.Binding.Plan.ExpiresAt = time.Now().UTC().Add(-time.Minute)
				review.Binding.Preview.ExpiresAt = review.Binding.Plan.ExpiresAt.Format(time.RFC3339Nano)
				review.Digest, _ = reviewDigest(review.Binding)
				if err := fx.local.saveHostReview(review); err != nil {
					t.Fatal(err)
				}
				expectedCode = "review_required"
			case "cancelled":
				if err := fx.local.CancelHostChange(ctx, review.Binding.ID); err != nil {
					t.Fatal(err)
				}
				expectedCode = "review_required"
			case "cancel_context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				expectedCode = "cancelled"
			case "changed_plan":
				plan := review.Binding.Plan
				plan.Command.Amount = "99"
				if err := fx.local.service.writePrivate(filepath.Join(fx.local.service.dir, "plans", plan.ID+".json"), plan); err != nil {
					t.Fatal(err)
				}
				expectedCode = "review_changed"
			}
			_, err = fx.local.executeLegacyFixture(ctx, review.Binding.ID)
			if expectedCode != "" {
				requireLocalError(t, err, expectedCode)
				assertActivityCount(t, fx, 0)
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertActivityCount(t, fx, 1)
			}
			if action == "stale" || action == "expired" {
				got, err := fx.local.InspectHostChange(t.Context(), review.Binding.ID)
				if err != nil || got.State != ReviewNeedsPreview {
					t.Fatalf("stale review not invalidated: %s %v", got.State, err)
				}
			}
		})
	}
}

func TestHostReviewRestartReceiptRecoveryAndNoReplay(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"unconfirmed", "admitted", "committed", "business_receipt_only"} {
		t.Run(phase, func(t *testing.T) {
			fx := newHostFixture(t)
			review := hostPreview(t, fx)
			if phase == "committed" || phase == "business_receipt_only" {
				if _, err := fx.local.executeLegacyFixture(t.Context(), review.Binding.ID); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "admitted" || phase == "business_receipt_only" {
				review.State = ReviewCommitting
				if err := fx.local.saveHostReview(review); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "business_receipt_only" {
				// Simulate loss of the outer receipt after the atomic ledger write.
				key := "builtin_ai.capability.v1." + fx.local.householdID + "." + fx.id + ".operation." + review.Binding.OperationID
				if _, err := fx.db.SQL.Exec("DELETE FROM app_configuration WHERE key=?", key); err != nil {
					t.Fatal(err)
				}
			}
			fx.local.Close()
			fx.service.Close()
			if err := fx.db.Close(); err != nil {
				t.Fatal(err)
			}
			app, db := openLocalApp(t, filepath.Join(fx.dir, "synthetic.db"))
			local, err := NewInProcessSession(t.Context(), app, fx.dir, fx.id, ReadWrite, nil, sqlite.NewConfigurationRepository(db))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(local.Close)
			version := app.MobileDataVersion()
			got, err := local.InspectHostChange(t.Context(), review.Binding.ID)
			if err != nil || version != app.MobileDataVersion() {
				t.Fatalf("receipt inspection attempted a financial write: %v", err)
			}
			expected := map[string]string{"unconfirmed": ReviewNeedsPreview, "admitted": ReviewUnknown, "committed": ReviewSucceeded, "business_receipt_only": ReviewSucceeded}[phase]
			if got.State != expected {
				t.Fatalf("%s state=%s want=%s", phase, got.State, expected)
			}
			_, err = local.executeLegacyFixture(t.Context(), review.Binding.ID)
			if expected == ReviewSucceeded {
				if err != nil || got.Receipt == nil || !strings.Contains(string(got.Receipt.Result), `"12.34"`) {
					t.Fatalf("success not recovered: %v", err)
				}
			} else {
				code := "review_required"
				if phase == "admitted" {
					code = "operation_outcome_unknown"
					requireLocalError(t, local.CancelHostChange(t.Context(), review.Binding.ID), code)
				}
				requireLocalError(t, err, code)
			}
			activities, err := app.ListActivities(t.Context(), 100)
			count := 0
			if expected == ReviewSucceeded {
				count = 1
			}
			if err != nil || len(activities) != count {
				t.Fatalf("restart replayed a submission: count=%d err=%v", len(activities), err)
			}
		})
	}
}

type failingReviewRepository struct {
	settings.ConfigurationRepository
	n        int
	failAt   int
	saveThen bool
	after    func()
}

func (r *failingReviewRepository) SaveConfiguration(key string, value []byte) error {
	if strings.Contains(key, ".review.") {
		r.n++
		if r.n == r.failAt {
			if r.saveThen {
				if err := r.ConfigurationRepository.SaveConfiguration(key, value); err != nil {
					return err
				}
			}
			return errors.New("synthetic persistence interruption")
		}
		if r.after != nil && r.n == 2 {
			r.after()
		}
	}
	return r.ConfigurationRepository.SaveConfiguration(key, value)
}

func TestHostReviewPersistenceFailuresAndRevocation(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"admission_failed", "admission_uncertain", "result_failed", "request_cancelled", "revoked"} {
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
				repo.failAt, repo.saveThen = 2, true
			case "result_failed":
				repo.failAt = 3
			case "request_cancelled":
				repo.after = cancel
			case "revoked":
				repo.after = func() { _ = fx.local.RevokeForRestore(t.Context()) }
			}
			review := hostPreview(t, fx)
			_, err := fx.local.executeLegacyFixture(ctx, review.Binding.ID)
			code := "review_unavailable"
			if phase == "result_failed" {
				code = "operation_outcome_unknown"
			} else if phase == "request_cancelled" {
				code = "cancelled"
			} else if phase == "revoked" {
				code = "session_revoked"
			}
			requireLocalError(t, err, code)
			expectedCount := 0
			if phase == "result_failed" {
				expectedCount = 1
			}
			assertActivityCount(t, fx, expectedCount)
			if phase == "revoked" {
				return
			}
			got, err := fx.local.InspectHostChange(t.Context(), review.Binding.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{"admission_failed": ReviewAwaitingConfirmation, "admission_uncertain": ReviewUnknown, "result_failed": ReviewSucceeded, "request_cancelled": ReviewCancelled}[phase]
			if got.State != expected {
				t.Fatalf("state=%s want=%s", got.State, expected)
			}
		})
	}
}

func TestHostReviewIsolationCorruptionAndPermissions(t *testing.T) {
	t.Parallel()
	fx := newHostFixture(t)
	review := hostPreview(t, fx)
	other, err := NewInProcessSession(t.Context(), fx.app, fx.dir, uuid.NewString(), ReadWrite, nil, sqlite.NewConfigurationRepository(fx.db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Close)
	_, err = other.InspectHostChange(t.Context(), review.Binding.ID)
	requireLocalError(t, err, "not_found")
	limited := newLocal(t, fx.ledgerFixture, ReadOnly)
	_, err = limited.previewLegacyFixture(t.Context(), hostCommand(fx))
	requireLocalError(t, err, "review_unavailable")
	fileOnly := newLocal(t, fx.ledgerFixture, ReadWrite)
	_, err = fileOnly.previewLegacyFixture(t.Context(), hostCommand(fx))
	requireLocalError(t, err, "review_unavailable")
	path, _ := fx.local.hostReviewPath(review.Binding.ID)
	for _, kind := range []string{"future", "digest", "unknown_field", "duplicate_name", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			value := review
			switch kind {
			case "future":
				value.Binding.Version = 2
				value.Digest, _ = reviewDigest(value.Binding)
			case "digest":
				value.Binding.Plan.Command.Amount = "99"
			}
			raw := mustJSON(t, value)
			if kind == "unknown_field" {
				raw = append([]byte(`{"future":1,`), raw[1:]...)
			} else if kind == "duplicate_name" {
				raw = append([]byte(`{"state":"succeeded",`), raw[1:]...)
			} else if kind == "malformed" {
				raw = []byte(`{"binding":`)
			}
			key := configurationKey(path)
			if err := fx.local.service.repository.SaveConfiguration(key, raw); err != nil {
				t.Fatal(err)
			}
			_, err := fx.local.executeLegacyFixture(t.Context(), review.Binding.ID)
			requireLocalError(t, err, "review_unavailable")
			assertActivityCount(t, fx, 0)
		})
	}
	values, err := sqlite.NewConfigurationRepository(fx.db).ListConfiguration("mcp.")
	if err != nil || len(values) != 0 {
		t.Fatalf("host review leaked to external MCP: %v", err)
	}
}

func TestInProcessMarketDataBusinessReceiptsArePrincipalBound(t *testing.T) {
	t.Parallel()
	fx := newHostFixture(t)
	external := ledgerSession(t, fx.ledgerFixture)
	other := newLocal(t, fx.ledgerFixture, ReadWrite)
	operationID := uuid.NewString()
	input := application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{InstrumentID: fx.instrument, Currency: "USD", Value: "1.01", Kind: "close", Date: "2026-09-28", SourceTitle: "Synthetic source"}}}
	args := Mutation[application.AgentMarketDataInput]{OperationID: operationID, Input: input}
	remote := call(t, external, "import_market_data", args, false)["data"]
	local := localData(t, fx.local, "import_market_data", args, true)
	// Same key, identical input must create an independent business receipt.
	if reflect.DeepEqual(remote, local) {
		t.Fatal("local session replayed the external import")
	}
	args.Input.Items = append([]application.AgentMarketDataItem{}, input.Items...)
	args.Input.Items[0].Value = "2.02"
	second := localData(t, other, "import_market_data", args, true)
	if reflect.DeepEqual(second, local) {
		t.Fatal("other local session reused a business receipt")
	}
	args.Input.Items[0].Value = "1.01"
	if again := localData(t, fx.local, "import_market_data", args, true); !reflect.DeepEqual(local, again) {
		t.Fatal("same-session retry lost its receipt")
	}
	args.Input.Items[0].Value = "3.03"
	_, err := fx.local.call(t.Context(), "import_market_data", mustJSON(t, args), true)
	requireLocalError(t, err, "conflict")
	key := "builtin_ai.v1:" + fx.local.householdID + ":" + fx.id + ":" + operationID
	otherKey := "builtin_ai.v1:" + other.householdID + ":" + other.owner.sessionID + ":" + operationID
	for _, storedKey := range []string{operationID, key, otherKey} {
		var count int
		if err := fx.db.SQL.QueryRow("SELECT COUNT(*) FROM agent_quote_batches WHERE request_key=?", storedKey).Scan(&count); err != nil || count != 1 {
			t.Fatalf("business receipt missing from expected keyspace: %v", err)
		}
	}
	if len(key) != 124 {
		t.Fatal("business key exceeds its specified encoding")
	}
	// An external client that knows the exact private key still cannot select
	// it: its unchanged operation-ID contract accepts UUIDs only.
	call(t, external, "import_market_data", Mutation[application.AgentMarketDataInput]{OperationID: key, Input: input}, true)
	if _, err := fx.app.ImportAgentMarketData(t.Context(), key, input); err == nil {
		t.Fatal("external application path accepted a reserved local key")
	}
	if _, err := fx.app.ImportBuiltinAgentMarketData(t.Context(), fx.id, operationID, args.Input); err == nil {
		t.Fatal("different input bypassed the transaction-backed receipt conflict")
	}
	canonicalReplay, err := fx.app.ImportBuiltinAgentMarketData(t.Context(), strings.ToUpper(fx.id), strings.ToUpper(operationID), input)
	if err != nil || !canonicalReplay.Replayed || canonicalReplay.RequestKey != operationID {
		t.Fatalf("local UUID case aliases changed business receipt: %+v %v", canonicalReplay, err)
	}
	// Lose the local outer receipt, retain the transactional business receipt.
	path, _ := fx.local.service.operationPath(operationID)
	op, err := fx.local.service.GetOperation(operationID)
	if err != nil {
		t.Fatal(err)
	}
	op.Status, op.Result = "pending", nil
	if err := fx.local.service.writePrivate(path, op); err != nil {
		t.Fatal(err)
	}
	fx.local.Close()
	reopened, err := NewInProcessSession(t.Context(), fx.app, fx.dir, fx.id, ReadWrite, nil, sqlite.NewConfigurationRepository(fx.db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	args.Input.Items[0].Value = "1.01"
	recovered := localData(t, reopened, "import_market_data", args, true)
	var imported application.AgentMarketDataResult
	if err := jsoncodec.Unmarshal(mustJSON(t, recovered["result"]), &imported); err != nil || !imported.Replayed || imported.RequestKey != operationID {
		t.Fatalf("transaction receipt not recovered: %+v %v", imported, err)
	}
	records, err := fx.app.AgentMarketDataRecords(t.Context())
	if err != nil || len(records) != 3 {
		t.Fatalf("cross-principal import lost/duplicated writes: %d %v", len(records), err)
	}
}

func TestHostReviewRestoreFenceAndLateRequestCancellation(t *testing.T) {
	t.Parallel()
	t.Run("exclusive_fence", func(t *testing.T) {
		fx := newHostFixture(t)
		err := fx.app.WithExclusive(t.Context(), application.ExclusiveBackup, func(context.Context) error {
			_, err := fx.local.previewLegacyFixture(t.Context(), hostCommand(fx))
			return err
		})
		requireLocalError(t, err, "backup_restore_busy")
		review := hostPreview(t, fx)
		if err := fx.app.WithExclusive(t.Context(), application.ExclusiveRestore, func(ctx context.Context) error {
			return fx.local.RevokeForRestore(ctx)
		}); err != nil {
			t.Fatal(err)
		}
		_, err = fx.local.executeLegacyFixture(t.Context(), review.Binding.ID)
		requireLocalError(t, err, "session_revoked")
		assertActivityCount(t, fx, 0)
	})
	t.Run("cancel_after_known_commit", func(t *testing.T) {
		fx := newHostFixture(t)
		review := hostPreview(t, fx)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		fx.local.service.changed = cancel
		got, err := fx.local.executeLegacyFixture(ctx, review.Binding.ID)
		if err != nil || got.State != ReviewSucceeded || ctx.Err() == nil {
			t.Fatalf("late request cancellation lost success: state=%s err=%v", got.State, err)
		}
		assertActivityCount(t, fx, 1)
	})
}

func TestRetiredHumanProposalsOnlyRecoverOrRetire(t *testing.T) {
	for _, state := range []string{ReviewAwaitingConfirmation, ReviewNeedsPreview, ReviewUnknown, ReviewSucceeded} {
		t.Run(state, func(t *testing.T) {
			fx := newHostFixture(t)
			review := fullPreview(t, fx)
			if state == ReviewSucceeded {
				var err error
				review, err = fx.local.executeLegacyFixture(t.Context(), review.Binding.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			// A saved pre-upgrade record, not an available permission or writer.
			review.Binding.Source = "human_review"
			review.State = state
			review.Digest, _ = reviewDigest(review.Binding)
			if err := fx.local.saveHostReview(review); err != nil {
				t.Fatal(err)
			}
			got, err := fx.local.InspectHostChange(t.Context(), review.Binding.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := state
			if state == ReviewAwaitingConfirmation || state == ReviewNeedsPreview {
				want = ReviewCancelled
			}
			if got.State != want {
				t.Fatalf("state=%s want=%s", got.State, want)
			}
			_, err = fx.local.executeLegacyFixture(t.Context(), review.Binding.ID)
			requireLocalError(t, err, "permission_denied")
			count := 0
			if state == ReviewSucceeded {
				count = 1
			}
			assertActivityCount(t, fx, count)
		})
	}
}
