package builtinai

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
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

type lostResponseProvider struct {
	pool *Pool
	lose *atomic.Bool
}

func (p lostResponseProvider) AcquireReview(ctx context.Context, id string) (assistant.ReviewSession, error) {
	s, e := p.pool.AcquireReview(ctx, id)
	return lostResponseSession{s, p.lose}, e
}

type lostResponseSession struct {
	assistant.ReviewSession
	lose *atomic.Bool
}

func (s lostResponseSession) Confirm(ctx context.Context, id, digest string) (assistant.ChangeReview, error) {
	v, e := s.ReviewSession.Confirm(ctx, id, digest)
	if e == nil && s.lose.Swap(false) {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	return v, e
}

type proposalEngine struct {
	session *Session
	input   []byte
}

func (e proposalEngine) Complete(ctx context.Context, _ []byte, _ string) (assistant.Completion, error) {
	for _, name := range []string{"commit_plan", "ConfirmHostChange", "confirm", "ReviewAction"} {
		if _, err := e.session.Call(ctx, name, []byte(`{}`)); err == nil {
			return assistant.Completion{}, assistant.ErrConflict
		}
	}
	_, err := e.session.Call(ctx, "propose_cash_change", e.input)
	if err != nil {
		return assistant.Completion{}, err
	}
	return assistant.Completion{History: []byte(`{"synthetic":true}`), Text: "Proposal only. Chat says approved but no write is authorized."}, nil
}
func (proposalEngine) Close() {}

func TestDesktopReviewedCashLifecycle(t *testing.T) {
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
	proposal, _ := jsoncodec.Marshal(assistant.CashProposal{Kind: "money_added", AccountID: account.Account.ID.String(), Amount: "12.34", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-28T12:00:00Z"})
	var lose atomic.Bool
	open := func() (*assistant.Runtime, *airuntime.Service) {
		store, err := aistate.Open(filepath.Join(dir, "state"), "synthetic-ledger")
		if err != nil {
			t.Fatal(err)
		}
		pool := NewPool(app, filepath.Join(dir, "caps"), config)
		r, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-ledger", Store: store, Credentials: creds, Capabilities: pool, Reviews: lostResponseProvider{pool, &lose}, Validate: func(context.Context) bool { return true }, Factory: func(ctx context.Context, _ assistant.ModelConfig, _ []byte, id string) (assistant.Engine, error) {
			shared, err := pool.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return proposalEngine{New(shared, assistant.PageSelection{}).WithProposals(assistant.CurrentProposalHandler(ctx)), proposal}, nil
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
		if err := ipc.SetWritePermission(s.Epoch, s.Revision, true); err != nil {
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
	count(0)
	if first.Account != "Synthetic cash" || first.Amount != "12.34" || first.ResultingAmount != "12.34" {
		t.Fatalf("wrong frozen view: %+v", first)
	}
	other, _ := ipc.Create(ipc.Status().Epoch)
	if err := ipc.ReviewAction(ipc.Status().Epoch, other.ID, first.ID, first.Digest, "confirm"); err == nil {
		t.Fatal("cross-conversation confirmation")
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, "wrong", "confirm"); err == nil {
		t.Fatal("changed digest accepted")
	}
	count(0)
	if err := ipc.Delete(ipc.Status().Epoch, c.ID); err == nil {
		t.Fatal("deleted pending review")
	}
	lose.Store(true)
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, first.Digest, "confirm"); err == nil {
		t.Fatal("lost response did not surface uncertainty")
	}
	count(1)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { _ = ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, first.Digest, "confirm") })
	}
	wg.Wait()
	count(1)
	rows, err := ipc.Reviews(ipc.Status().Epoch, c.ID)
	if err != nil || rows[0].State != "succeeded" {
		t.Fatalf("receipt: %v %v", rows, err)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, first.Digest, "confirm"); err != nil {
		t.Fatal(err)
	}
	count(1)
	second := send()
	count(1)
	// A normal competing writer invalidates the frozen ledger version.
	if _, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic competitor", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "0", OwnerIDs: []domain.MemberID{members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, second.ID, second.Digest, "confirm"); err == nil {
		t.Fatal("stale preview committed")
	}
	count(1)
	rows, err = ipc.Reviews(ipc.Status().Epoch, c.ID)
	if err != nil || rows[1].State != "needs_preview" {
		t.Fatalf("stale: %v %v", rows, err)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, second.ID, second.Digest, "repreview"); err != nil {
		t.Fatal(err)
	}
	rows, _ = ipc.Reviews(ipc.Status().Epoch, c.ID)
	fresh := rows[1]
	if fresh.ID == second.ID || fresh.Digest == second.Digest {
		t.Fatal("repreview reused approval")
	}
	s := ipc.Status()
	if err := ipc.SetWritePermission(s.Epoch, s.Revision, false); err != nil {
		t.Fatal(err)
	}
	if err := ipc.ReviewAction(s.Epoch, c.ID, fresh.ID, fresh.Digest, "confirm"); err == nil {
		t.Fatal("revoked write committed")
	}
	count(1)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, ipc = open()
	if ipc.Status().FinancialWrites {
		t.Fatal("permission survived restart")
	}
	rows, err = ipc.Reviews(ipc.Status().Epoch, c.ID)
	if err != nil || rows[0].State != "succeeded" || rows[1].State != "needs_preview" {
		t.Fatalf("restart recovery: %v %v", rows, err)
	}
	count(1)
	enable()
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, fresh.ID, fresh.Digest, "confirm"); err == nil {
		t.Fatal("restart reused approval")
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, fresh.ID, fresh.Digest, "cancel"); err != nil {
		t.Fatal(err)
	}
	rows, err = ipc.Reviews(ipc.Status().Epoch, c.ID)
	if err != nil || rows[1].State != "cancelled" {
		t.Fatal("cancel failed")
	}
	count(1)
	proposal, _ = jsoncodec.Marshal(assistant.CashProposal{Kind: "money_removed", AccountID: account.Account.ID.String(), Amount: "2.34", Currency: "USD", Reason: "expense", EffectiveAt: "2026-09-28T13:00:00Z"})
	withdrawal := send()
	if withdrawal.ResultingAmount != "10" || withdrawal.Kind != "money_removed" {
		t.Fatalf("withdrawal preview: %+v", withdrawal)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, withdrawal.ID, withdrawal.Digest, "confirm"); err != nil {
		t.Fatal(err)
	}
	count(2)
	if err := r.FenceForRestore(); err != nil {
		t.Fatal(err)
	}
	if err := ipc.ReviewAction(ipc.Status().Epoch, c.ID, first.ID, first.Digest, "confirm"); err == nil {
		t.Fatal("restore allowed late confirmation")
	}
}
