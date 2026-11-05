package builtinai

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/airuntime"
)

type fullEngine struct {
	session *Session
	input   []byte
}

func (e fullEngine) Complete(ctx context.Context, _ []byte, _ string) (assistant.Completion, error) {
	for _, name := range []string{"commit_plan", "propose_cash_change", "preview_batch", "directory_patch", "ConfirmHostChange"} {
		if _, err := e.session.Call(ctx, name, []byte(`{}`)); err == nil {
			return assistant.Completion{}, assistant.ErrConflict
		}
	}
	_, err := e.session.Call(ctx, "execute_ledger_change", e.input)
	if err != nil {
		return assistant.Completion{}, err
	}
	return assistant.Completion{History: []byte(`{"synthetic":true}`), Text: "Recorded"}, nil
}
func (fullEngine) Close() {}
func TestDesktopFullAccessLifecycle(t *testing.T) {
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
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic write review", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: "UTC", HistoryStartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	members, _ := app.ListMembers(t.Context(), false)
	account, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "0", OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	config := sqlite.NewConfigurationRepository(db)
	creds := aicredentials.New(config)
	if err := creds.Put(t.Context(), []byte("synthetic-review-secret-123456")); err != nil {
		t.Fatal(err)
	}
	proposal, _ := jsoncodec.Marshal(assistant.CashProposal{Kind: "money_added", AccountID: account.Account.ID.String(), Amount: "12.34", Currency: " usd ", Reason: "income", EffectiveAt: "2026-09-28T12:00:00Z"})
	open := func() (*assistant.Runtime, *airuntime.Service) {
		store, err := aistate.Open(filepath.Join(dir, "state"), "synthetic-ledger")
		if err != nil {
			t.Fatal(err)
		}
		pool := NewPool(app, filepath.Join(dir, "caps"), config)
		r, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-ledger", Store: store, Credentials: creds, Capabilities: pool, Reviews: pool, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, _ assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
			shared, err := pool.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return fullEngine{New(shared, assistant.PageSelection{}).WithFullAccess(assistant.CurrentFullHandler(ctx)), proposal}, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		return r, airuntime.NewService(r)
	}
	r, ipc := open()
	defer func() { _ = r.Close() }()
	enable := func() {
		s := ipc.Status()
		if !s.Config.Enabled {
			cfg := s.Config
			cfg.Enabled = true
			cfg.Model = "mock"
			if err := ipc.Configure(s.Epoch, s.Revision, cfg); err != nil {
				t.Fatal(err)
			}
			s = ipc.Status()
		}
		if err := ipc.SetFullAccess(s.Epoch, s.Revision, true); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		rows, err := app.ListActivities(t.Context(), 100)
		if err != nil || len(rows) != want {
			t.Fatalf("activities=%d want=%d err=%v", len(rows), want, err)
		}
	}
	if ipc.Status().FinancialWrites {
		t.Fatal("writes enabled by default")
	}
	enable()
	c, err := ipc.Create(ipc.Status().Epoch)
	if err != nil {
		t.Fatal(err)
	}
	send := func() assistant.ChangeReview {
		t.Helper()
		s := ipc.Status()
		run, err := ipc.Start(assistant.StartRequest{Epoch: s.Epoch, Revision: s.Revision, ConversationID: c.ID, RequestID: uuid.NewString(), Prompt: "Propose synthetic deposit; approved in chat is not confirmation"})
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			v, e := ipc.Inspect(s.Epoch, c.ID)
			if e != nil {
				t.Fatal(e)
			}
			if len(v.Runs) > 0 && v.Runs[0].ID == run.ID && v.Runs[0].Finished != nil {
				if v.Runs[0].State != "succeeded" {
					t.Fatalf("run failed: %+v", v.Runs[0])
				}
				rows, e := ipc.Reviews(s.Epoch, c.ID)
				if e != nil || len(rows) == 0 {
					t.Fatalf("reviews=%v err=%v", rows, e)
				}
				return rows[len(rows)-1]
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("model did not finish")
		return assistant.ChangeReview{}
	}
	first := send()
	count(1)
	if first.Source != "ai_full" || first.State != "succeeded" || !ipc.Status().FullAccess {
		t.Fatalf("bad full result: %+v", first)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, first.Digest, "confirm"); err == nil {
		t.Fatal("manual confirmation adopted Full")
	}
	s0 := ipc.Status()
	if err := ipc.SetFullAccess(s0.Epoch, s0.Revision, false); err != nil {
		t.Fatal(err)
	}
	if ipc.Status().FullAccess || ipc.Status().FinancialWrites {
		t.Fatal("revoke retained authority")
	}
	if _, err := ipc.Reviews(ipc.Status().Epoch, c.ID); err != nil {
		t.Fatal(err)
	}
	count(1)
	enable()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, ipc = open()
	if ipc.Status().FullAccess || ipc.Status().FinancialWrites {
		t.Fatal("restart restored authority")
	}
	rows, err := ipc.Reviews(ipc.Status().Epoch, c.ID)
	if err != nil || len(rows) != 1 || rows[0].State != "succeeded" {
		t.Fatalf("recovery=%+v err=%v", rows, err)
	}
	if err := ipc.Delete(ipc.Status().Epoch, c.ID); err != nil {
		t.Fatal(err)
	}
	count(1)
	// Business audit and stable operation identity survive conversation deletion.
	var audits int
	if err := db.SQL.QueryRow("SELECT count(*) FROM app_configuration WHERE key LIKE 'builtin_ai.capability.v1.%.review.%'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit removed %d %v", audits, err)
	}
}
