package assistant_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
)

func TestNavigationRequiresGrantAndFencesPageExpiryConfigAndReplay(t *testing.T) {
	for _, target := range []ai.ModelNavigationTarget{
		{Kind: "overview"},
		{Kind: "return-trend", Trend: &ai.ReturnTrendAction{From: "2026-01-01", To: "2026-02-28", Valuation: "base", IncludeCash: true, Display: "linked_rate"}},
	} {
		t.Run(target.Kind, func(t *testing.T) { navigationFences(t, target) })
	}
}
func navigationFences(t *testing.T, requested ai.ModelNavigationTarget) {
	now := time.Now().UTC()
	c := &credentials{available: true}
	dir := filepath.Join(t.TempDir(), "private")
	var enabled bool
	var duringValidation func()
	factory := func(ctx context.Context, _ ai.ModelConfig, _ []byte, _ string) (ai.Engine, error) {
		enabled = ai.ModelNavigationEnabled(ctx)
		return &engine{fn: func(context.Context, []byte, string) (ai.Completion, error) {
			return ai.Completion{Text: "Navigation requested", History: []byte("synthetic-native"), Navigation: &requested}, nil
		}}, nil
	}
	openRuntime := func() *ai.Runtime {
		store, err := aistate.Open(dir, "synthetic-nav")
		if err != nil {
			t.Fatal(err)
		}
		r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-nav", Store: store, Credentials: c, Factory: factory, Validate: func(context.Context) bool { return true }, NavigationClock: func() time.Time { return now }, NavigationValidator: func(_ context.Context, target ai.ModelNavigationTarget) bool {
			if duringValidation != nil {
				duringValidation()
			}
			return target.Valid()
		}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := openRuntime()
	enable(t, r)
	q := request(t, r)
	q.Page = ai.PageSelection{Kind: "overview"}
	publish := func() ai.DesktopPageLease {
		status := r.Snapshot()
		lease, err := r.PublishPage(t.Context(), status.Epoch, status.Revision, q.Page)
		if err != nil {
			t.Fatal(err)
		}
		return lease
	}
	lease := publish()
	q.PageLeaseID = lease.ID
	// A lease/selector alone gives no model navigation authority, even if an
	// adversarial engine returns a typed target anyway.
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	waitRun(t, r, q)
	v, _ := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
	if enabled || v.Navigation != nil {
		t.Fatal("client page lease granted navigation")
	}
	startGranted := func() ai.ModelNavigationIntent {
		q.RequestID = uuid.NewString()
		q.Revision = r.Snapshot().Revision
		lease = publish()
		q.PageLeaseID = lease.ID
		if err := r.GrantNavigation(t.Context(), q.Epoch, q.Revision, q.ConversationID, q.RequestID, lease.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Start(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		waitRun(t, r, q)
		view, err := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
		if err != nil || view.Navigation == nil || !enabled {
			t.Fatal("explicit grant did not offer navigation")
		}
		return *view.Navigation
	}
	intent := startGranted()
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, "wrong-lease"); err == nil {
		t.Fatal("wrong page lease accepted")
	}
	target, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID)
	if err != nil || target.Kind != requested.Kind {
		t.Fatal("valid intent failed", err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("intent replayed")
	}
	intent = startGranted()
	if err := r.Cancel(t.Context(), q.Epoch, q.ConversationID, intent.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("Stop left a completed navigation intent executable")
	}
	intent = startGranted()
	publish()
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("old page navigated after refresh")
	}
	intent = startGranted()
	now = now.Add(3 * time.Minute)
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("expired page navigated")
	}
	intent = startGranted()
	status := r.Snapshot()
	cfg := status.Config
	cfg.Enabled = false
	if err := r.Configure(t.Context(), status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("disabled config navigated")
	}
	status = r.Snapshot()
	cfg.Enabled = true
	if err := r.Configure(t.Context(), status.Epoch, status.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	intent = startGranted()
	if err := r.GrantNavigation(t.Context(), q.Epoch, r.Snapshot().Revision, q.ConversationID, uuid.NewString(), intent.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("new authorization retained old intent")
	}
	intent = startGranted()
	duringValidation = func() {
		if err := r.GrantNavigation(t.Context(), q.Epoch, r.Snapshot().Revision, q.ConversationID, uuid.NewString(), intent.LeaseID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("new grant during validation retained in-flight intent")
	}
	duringValidation = nil
	intent = startGranted()
	q.RequestID = uuid.NewString()
	q.PageLeaseID = ""
	q.Revision = r.Snapshot().Revision
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	waitRun(t, r, q)
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("new ordinary turn retained old intent")
	}
	intent = startGranted()
	if err := r.Delete(t.Context(), q.Epoch, q.ConversationID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("deleted conversation retained intent")
	}
	conversation, err := r.Create(t.Context(), q.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	q.ConversationID = conversation.ID
	intent = startGranted()
	duringValidation = func() {
		if err := r.Delete(t.Context(), q.Epoch, q.ConversationID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("conversation deletion during validation retained intent")
	}
	duringValidation = nil
	conversation, err = r.Create(t.Context(), q.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	q.ConversationID = conversation.ID
	intent = startGranted()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r = openRuntime()
	defer r.Close()
	status = r.Snapshot()
	v, err = r.Inspect(t.Context(), status.Epoch, q.ConversationID)
	if err != nil || v.Navigation != nil {
		t.Fatal("restart resurrected navigation authority")
	}
	if _, err := r.ConsumeNavigation(t.Context(), status.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("historical intent executed")
	}
}

func TestPageChangeDuringModelRunSuppressesNavigation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	c := &credentials{available: true}
	store, err := aistate.Open(privateTestDirectory(t), "synthetic-nav-stale")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-nav-stale", Store: store, Credentials: c, Validate: func(context.Context) bool { return true }, NavigationValidator: func(context.Context, ai.ModelNavigationTarget) bool { return true }, Factory: factory(func(context.Context, []byte, string) (ai.Completion, error) {
		close(started)
		<-release
		return ai.Completion{Text: "requested", History: []byte("native"), Navigation: &ai.ModelNavigationTarget{Kind: "accounts"}}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	enable(t, r)
	q := request(t, r)
	q.Page = ai.PageSelection{Kind: "overview"}
	lease, err := r.PublishPage(t.Context(), q.Epoch, q.Revision, q.Page)
	if err != nil {
		t.Fatal(err)
	}
	q.PageLeaseID = lease.ID
	if err := r.GrantNavigation(t.Context(), q.Epoch, q.Revision, q.ConversationID, q.RequestID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := r.PublishPage(t.Context(), q.Epoch, q.Revision, ai.PageSelection{Kind: "accounts"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitRun(t, r, q)
	v, _ := r.Inspect(t.Context(), q.Epoch, q.ConversationID)
	if v.Navigation != nil {
		t.Fatal("late completion navigated old page")
	}
}

// A typed page action from the same successful run cannot dismiss a financial
// review restored from durable state, even after the conversation is renamed.
func TestReturnTrendBlockedByRestoredReviewAfterRename(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	store, err := aistate.Open(dir, "synthetic-nav-review")
	if err != nil {
		t.Fatal(err)
	}
	state := ai.InitialState("synthetic-nav-review")
	id := uuid.NewString()
	state.Conversations = []ai.ConversationState{{ID: id, Updated: time.Now(), Reviews: []ai.ReviewRef{{ID: uuid.NewString()}}}}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	target := ai.ModelNavigationTarget{Kind: "return-trend", Trend: &ai.ReturnTrendAction{From: "2026-01-01", To: "2026-02-28", Valuation: "base", IncludeCash: true, Display: "linked_rate"}}
	validated := false
	r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-nav-review", Store: store, Credentials: &credentials{available: true}, Validate: func(context.Context) bool { return true }, NavigationValidator: func(context.Context, ai.ModelNavigationTarget) bool { validated = true; return true }, Factory: factory(func(context.Context, []byte, string) (ai.Completion, error) {
		return ai.Completion{Text: "Requested trend", History: []byte("synthetic"), Navigation: &target}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	enable(t, r)
	status := r.Snapshot()
	if err := r.Rename(t.Context(), status.Epoch, id, "Pending cash review"); err != nil {
		t.Fatal(err)
	}
	lease, err := r.PublishPage(t.Context(), status.Epoch, status.Revision, ai.PageSelection{Kind: "overview"})
	if err != nil {
		t.Fatal(err)
	}
	q := ai.StartRequest{Epoch: status.Epoch, Revision: status.Revision, ConversationID: id, RequestID: uuid.NewString(), Prompt: "Open return trend", Page: ai.PageSelection{Kind: "overview"}, PageLeaseID: lease.ID}
	if err := r.GrantNavigation(t.Context(), q.Epoch, q.Revision, id, q.RequestID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	waitRun(t, r, q)
	view, err := r.Inspect(t.Context(), q.Epoch, id)
	if err != nil || view.Navigation == nil || view.Title != "Pending cash review" {
		t.Fatal("missing combined conversation state", err)
	}
	intent := view.Navigation
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, id, intent.RunID, intent.ID, lease.ID); err == nil || validated {
		t.Fatal("pending review allowed return trend execution")
	}
}
