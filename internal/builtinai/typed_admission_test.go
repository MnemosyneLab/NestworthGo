package builtinai

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

type typedCorrectionEngine func(context.Context) (assistant.Completion, error)

func (e typedCorrectionEngine) Complete(ctx context.Context, _ []byte, _ string) (assistant.Completion, error) {
	return e(ctx)
}
func (typedCorrectionEngine) Close() {}

func TestFullRuntimeTypedRejectionDoesNotCreateUnknownReview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	app := application.NewService(repo)
	appports.Wire(app)
	appports.AttachSQLiteHistory(app, repo)
	app.SetLiveDatabasePath(path)
	app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic typed admission", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: "UTC", HistoryStartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	instrument, err := app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic instrument", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	config := sqlite.NewConfigurationRepository(db)
	creds := aicredentials.New(config)
	if err := creds.Put(t.Context(), []byte("synthetic-typed-admission-secret")); err != nil {
		t.Fatal(err)
	}
	store, err := aistate.Open(filepath.Join(dir, "state"), "synthetic-ledger")
	if err != nil {
		t.Fatal(err)
	}
	pool := NewPool(app, filepath.Join(dir, "caps"), config)
	var ipc *airuntime.Service
	var conversationID string
	operationID := uuid.NewString()
	r, err := assistant.NewRuntime(assistant.Options{
		Ledger: "synthetic-ledger", Store: store, Credentials: creds,
		Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true },
		Factory: func(ctx context.Context, _ assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
			shared, err := pool.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			session := New(shared, assistant.PageSelection{}).WithFullAccess(assistant.CurrentFullHandler(ctx))
			return typedCorrectionEngine(func(ctx context.Context) (assistant.Completion, error) {
				args := func(number string) []byte {
					return []byte(`{"operationId":"` + operationID + `","input":{"id":"` + instrument.ID.String() + `","changes":{"sortOrder":` + number + `}}}`)
				}
				for _, literal := range []string{"1.0", "1e0", "9223372036854775808"} {
					_, err := session.Call(ctx, "update_instrument", args(literal))
					var marker interface{ NotDispatched() bool }
					if !errors.Is(err, agentcapability.ErrInvalidArguments) || !errors.As(err, &marker) || !marker.NotDispatched() {
						return assistant.Completion{}, fmt.Errorf("typed rejection lacks proof: %s: %w", literal, err)
					}
					refs, err := ipc.Reviews(ipc.Status().Epoch, conversationID)
					if err != nil || len(refs) != 0 {
						return assistant.Completion{}, fmt.Errorf("typed rejection created recovery refs: %+v: %v", refs, err)
					}
					if _, err := shared.InspectFullOperation(operationID); err == nil {
						return assistant.Completion{}, errors.New("typed rejection created a receipt")
					}
				}
				if _, err := session.Call(ctx, "update_instrument", args("7")); err != nil {
					return assistant.Completion{}, fmt.Errorf("corrected write was fenced: %w", err)
				}
				return assistant.Completion{History: []byte(`{"synthetic":true}`), Text: "Corrected and recorded"}, nil
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ipc = airuntime.NewService(r)
	status := ipc.Status()
	cfg := status.Config
	cfg.Enabled, cfg.Model = true, "mock"
	if err := ipc.Configure(status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	status = ipc.Status()
	if err := ipc.SetFullAccess(status.Epoch, status.Revision, true); err != nil {
		t.Fatal(err)
	}
	conversation, err := ipc.Create(ipc.Status().Epoch)
	if err != nil {
		t.Fatal(err)
	}
	conversationID = conversation.ID
	status = ipc.Status()
	run, err := ipc.Start(assistant.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: conversationID, RequestID: uuid.NewString(), Prompt: "Update the synthetic instrument order"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	finished := false
	for time.Now().Before(deadline) {
		state, err := ipc.Inspect(status.Epoch, conversationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Runs) > 0 && state.Runs[0].ID == run.ID && state.Runs[0].Finished != nil {
			if state.Runs[0].State != "succeeded" {
				t.Fatalf("correction run failed: %+v", state.Runs[0])
			}
			finished = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !finished {
		t.Fatal("correction run did not finish")
	}
	refs, err := ipc.Reviews(status.Epoch, conversationID)
	if err != nil || len(refs) != 1 || refs[0].ID != operationID || refs[0].State != "succeeded" {
		t.Fatalf("unexpected recovery refs after correction: %+v, %v", refs, err)
	}
	instruments, err := app.ListInstruments(t.Context(), false)
	if err != nil || len(instruments) != 1 || instruments[0].SortOrder != 7 {
		t.Fatalf("corrected write did not execute: %+v, %v", instruments, err)
	}
}
