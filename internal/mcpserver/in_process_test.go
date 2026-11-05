package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

func localRaw(t *testing.T, s *InProcessSession, name string, arguments any, trusted bool) json.RawMessage {
	t.Helper()
	raw, err := jsoncodec.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.call(t.Context(), name, raw, trusted)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func localData(t *testing.T, s *InProcessSession, name string, arguments any, trusted bool) map[string]any {
	t.Helper()
	return jsonObject(t, localRaw(t, s, name, arguments, trusted))["data"].(map[string]any)
}

func requireLocalError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", code)
	}
	value, ok := apierror.Parse(err.Error())
	if !ok || value.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func newLocal(t *testing.T, fx ledgerFixture, mode string) *InProcessSession {
	t.Helper()
	local, err := NewInProcessSession(t.Context(), fx.app, t.TempDir(), uuid.NewString(), mode, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	return local
}

func TestInProcessModesHaveNoTransportOrModelWriteAuthority(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	for _, test := range []struct {
		mode  string
		count int
	}{{ReadOnly, 51}, {DirectoryWrite, 64}, {LedgerWrite, 85}} {
		local := newLocal(t, fx, test.mode)
		if local.service.listener != nil || local.service.server != nil || local.service.endpoint != "" || local.service.config.Token != "" {
			t.Fatal("local session created external transport/credentials")
		}
		if len(local.registry.Definitions()) != test.count {
			t.Fatalf("%s lost shared capabilities", test.mode)
		}
		public, err := local.Definitions()
		if err != nil {
			t.Fatal(err)
		}
		for _, definition := range public {
			if definition.Effect == agentcapability.Write || !modelCallable(definition) {
				t.Fatalf("model discovered restricted capability %s", definition.Name)
			}
		}
		for _, definition := range local.registry.Definitions() {
			if definition.Effect == agentcapability.Write || definition.Name == "search_market_instruments" {
				_, err := local.Call(t.Context(), definition.Name, json.RawMessage(`{}`))
				requireLocalError(t, err, "approval_required")
			}
		}
		_, err = local.Call(t.Context(), "commit_plan", json.RawMessage(`{}`))
		if test.mode == LedgerWrite {
			requireLocalError(t, err, "approval_required")
		} else {
			requireLocalError(t, err, "not_found")
		}
		localRaw(t, local, "get_catalog", map[string]any{}, false)
	}
	for _, id := range []string{"", uuid.Nil.String(), "../escape"} {
		_, err := NewInProcessSession(t.Context(), fx.app, t.TempDir(), id, LedgerWrite, nil)
		requireLocalError(t, err, "validation")
	}
}

func TestInProcessFrozenPaginationAndRevocationIsolation(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	external := ledgerSession(t, fx)
	local := newLocal(t, fx, ReadOnly)
	other := newLocal(t, fx, ReadOnly)
	var capture struct {
		Data ReadEnvelope `json:"data"`
	}
	if err := jsoncodec.Unmarshal(localRaw(t, local, "list_accounts", map[string]any{"limit": 1}, false), &capture); err != nil {
		t.Fatal(err)
	}
	section := capture.Data.Sections["accounts"]
	if !section.Page.HasMore || section.Page.Total != 4 {
		t.Fatalf("not paged: %+v", section.Page)
	}
	args := ReadPageInput{capture.Data.CaptureID, "accounts", *section.Page.NextCursor, 1}
	argsRaw, _ := jsoncodec.Marshal(args)
	_, err := other.Call(t.Context(), "get_read_page", argsRaw)
	if err == nil {
		t.Fatal("different local session read a frozen capture")
	}
	call(t, external, "get_read_page", args, true)
	if _, err := fx.service.Disable(); err != nil {
		t.Fatal(err)
	}
	seen := len(section.Items)
	for section.Page.HasMore {
		pageRaw := localRaw(t, local, "get_read_page", args, false)
		if string(pageRaw) != string(localRaw(t, local, "get_read_page", args, false)) {
			t.Fatal("retry changed frozen page")
		}
		var page struct {
			Data ReadEnvelope `json:"data"`
		}
		if err := jsoncodec.Unmarshal(pageRaw, &page); err != nil {
			t.Fatal(err)
		}
		if page.Data.ContentHash != capture.Data.ContentHash || page.Data.ExpiresAt != capture.Data.ExpiresAt || page.Data.CapturedAt != capture.Data.CapturedAt {
			t.Fatal("page changed capture identity, evidence time or TTL")
		}
		seen += len(page.Data.Items)
		section.Page = *page.Data.Page
		if section.Page.HasMore {
			args.Cursor = *section.Page.NextCursor
		}
	}
	if seen != 4 {
		t.Fatal("pagination lost rows", seen)
	}
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	external = connect(t, fx.service)
	if err := local.RevokeForRestore(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = local.Call(t.Context(), "get_catalog", json.RawMessage(`{}`))
	requireLocalError(t, err, "session_revoked")
	call(t, external, "get_catalog", Empty{}, false)
	localRaw(t, other, "get_catalog", Empty{}, false)
}

func TestInProcessLedgerParityConcurrentRetriesAndReceiptIsolation(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	external := ledgerSession(t, fx)
	local := newLocal(t, fx, LedgerWrite)
	command := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}
	preview := localData(t, local, "preview_change", command, false)
	externalPreview := ledgerPreview(t, external, command)
	if !strings.Contains(string(localRaw(t, local, "get_account_snapshot", IDInput{fx.checking}, false)), "Checking") {
		t.Fatal("account read unavailable")
	}
	// Resulting endpoint values are authoritative; provisional activity IDs are
	// intentionally different. Compare full commands and resulting endpoints.
	localPreview := preview["preview"].(map[string]any)
	remotePreview := externalPreview["preview"].(map[string]any)
	if !reflect.DeepEqual(preview["command"], externalPreview["command"]) || !reflect.DeepEqual(localPreview["resulting"], remotePreview["resulting"]) {
		t.Fatal("adapters produced different commands or resulting endpoint values")
	}
	operationID := uuid.NewString()
	input := map[string]any{"operationId": operationID, "input": map[string]any{"planId": preview["planId"]}}
	rawInput := mustJSON(t, input)
	var wg sync.WaitGroup
	results := make(chan string, 4)
	for range 4 {
		wg.Go(func() {
			result, err := local.call(t.Context(), "commit_plan", rawInput, true)
			if err != nil {
				t.Errorf("commit retry: %v", err)
			}
			results <- string(result)
		})
	}
	wg.Wait()
	close(results)
	var first string
	for result := range results {
		if first == "" {
			first = result
		} else if result != first {
			t.Fatal("duplicate confirmation did not recover exact receipt")
		}
	}
	if !strings.Contains(first, `"status":"succeeded"`) || !strings.Contains(first, `"12.34"`) {
		t.Fatal("successful exact write missing", first)
	}
	activities, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil || len(activities) != 1 {
		t.Fatalf("duplicate ledger posting: %d, %v", len(activities), err)
	}
	if amount := ledgerAmount(t, external, fx.checking, "USD"); amount != "12.34" {
		t.Fatalf("authoritative postcommit cash is %s, expected exact 12.34", amount)
	}
	call(t, external, "get_operation", IDInput{operationID}, true)
	other := newLocal(t, fx, LedgerWrite)
	_, err = other.Call(t.Context(), "get_operation", mustJSON(t, IDInput{operationID}))
	requireLocalError(t, err, "not_found")
	localData(t, local, "get_operation", IDInput{operationID}, false)
}

func TestInProcessRestoreFenceCancelsInflightReadWithoutDraining(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	local := newLocal(t, fx, ReadOnly)
	started := make(chan struct{})
	closed := false
	local.registry = agentcapability.New("")
	agentcapability.Add(local.registry, &agentcapability.Definition{Name: "synthetic_slow_read", Effect: agentcapability.Read, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}}, func(ctx context.Context, _ Empty) (Response, error) {
		close(started)
		<-ctx.Done()
		return Response{Data: "old private evidence"}, nil
	})
	result := make(chan error, 1)
	go func() {
		raw, err := local.Call(t.Context(), "synthetic_slow_read", json.RawMessage(`{}`))
		if len(raw) != 0 {
			t.Error("revoked read delivered old private evidence")
		}
		result <- err
	}()
	<-started
	// Restore's fence must not wait on a writer that could be waiting on the
	// application's exclusive permit. Holding this mutex detects lock coupling.
	local.service.operationMu.Lock()
	if err := local.RevokeForRestore(t.Context()); err != nil {
		t.Fatal(err)
	}
	local.service.operationMu.Unlock()
	requireLocalError(t, <-result, "session_revoked")
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInProcessCancellationDoesNotUndoCommittedReceipt(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	var changed atomic.Int32
	local, err := NewInProcessSession(t.Context(), fx.app, t.TempDir(), uuid.NewString(), LedgerWrite, func() { changed.Add(1); cancel() })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	command := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}
	preview := localData(t, local, "preview_change", command, false)
	input := mustJSON(t, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": preview["planId"]}})
	early, earlyCancel := context.WithCancel(t.Context())
	earlyCancel()
	_, err = local.call(early, "commit_plan", input, true)
	requireLocalError(t, err, "cancelled")
	result, err := local.call(ctx, "commit_plan", input, true)
	if err != nil || !strings.Contains(string(result), `"status":"succeeded"`) || changed.Load() != 1 || ctx.Err() == nil {
		t.Fatalf("late cancellation lost committed receipt: %s, %v", result, err)
	}
	retry, err := local.call(t.Context(), "commit_plan", input, true)
	if err != nil || string(retry) != string(result) || changed.Load() != 1 {
		t.Fatalf("retry changed committed operation: %s, %v", retry, err)
	}
}

func openLocalApp(t *testing.T, path string) (*application.Service, *sqlite.DB) {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := sqlite.NewRepository(db)
	app := application.NewService(repository)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repository)
	app.SetLiveDatabasePath(path)
	app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	return app, db
}

func TestInProcessSQLiteNamespaceRestartAndUnknownDirectoryWrite(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "synthetic.db")
	app, db := openLocalApp(t, path)
	fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	repository := sqlite.NewConfigurationRepository(db)
	id := uuid.NewString()
	local, err := NewInProcessSession(t.Context(), app, directory, id, LedgerWrite, nil, repository)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	_, err = NewInProcessSession(t.Context(), app, t.TempDir(), id, LedgerWrite, nil, repository)
	requireLocalError(t, err, "conflict")
	command := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "12.34", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-28T12:00:00Z"}
	preview := localData(t, local, "preview_change", command, false)
	uncommitted := localData(t, local, "preview_change", command, false)
	input := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": preview["planId"]}}
	committed := localRaw(t, local, "commit_plan", input, true)
	// A directory pending receipt has no transactional recovery proof.
	pendingID := uuid.NewString()
	directoryInput := CreateDirectoryInput{Name: "Synthetic pending member"}
	digest := sha256.Sum256(append([]byte("create_member\n"), mustJSON(t, directoryInput)...))
	pendingPath, _ := local.service.operationPath(pendingID)
	if err := local.service.writePrivate(pendingPath, Operation{ID: pendingID, Tool: "create_member", Hash: hex.EncodeToString(digest[:]), Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	values, err := repository.ListConfiguration("mcp.")
	if err != nil || len(values) != 0 {
		t.Fatalf("built-in state leaked into external namespace: %v, %v", values, err)
	}
	local.Close()
	fx.service.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	app, db = openLocalApp(t, path)
	reopened, err := NewInProcessSession(t.Context(), app, directory, id, LedgerWrite, nil, sqlite.NewConfigurationRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if result := localRaw(t, reopened, "commit_plan", input, true); string(result) != string(committed) {
		t.Fatal("reopened ledger retry lost original receipt")
	}
	_, err = reopened.call(t.Context(), "commit_plan", mustJSON(t, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": uncommitted["planId"]}}), true)
	requireLocalError(t, err, "stale_preview")
	_, err = reopened.call(t.Context(), "create_member", mustJSON(t, Mutation[CreateDirectoryInput]{OperationID: pendingID, Input: directoryInput}), true)
	requireLocalError(t, err, "operation_outcome_unknown")
	members, err := app.ListMembers(t.Context(), false)
	if err != nil || len(members) != 1 {
		t.Fatalf("unknown directory operation replayed: %d, %v", len(members), err)
	}
}
