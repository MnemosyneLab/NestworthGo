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
	now := time.Now().UTC()
	c := &credentials{available: true}
	dir := filepath.Join(t.TempDir(), "private")
	var enabled bool
	factory := func(ctx context.Context, _ ai.ModelConfig, _ []byte, _ string) (ai.Engine, error) {
		enabled = ai.ModelNavigationEnabled(ctx)
		return &engine{fn: func(context.Context, []byte, string) (ai.Completion, error) {
			return ai.Completion{Text: "Navigation requested", History: []byte("synthetic-native"), Navigation: &ai.ModelNavigationTarget{Kind: "overview"}}, nil
		}}, nil
	}
	openRuntime := func() *ai.Runtime {
		store, err := aistate.Open(dir, "synthetic-nav")
		if err != nil {
			t.Fatal(err)
		}
		r, err := ai.NewRuntime(ai.Options{Ledger: "synthetic-nav", Store: store, Credentials: c, Factory: factory, Validate: func(context.Context) bool { return true }, NavigationClock: func() time.Time { return now }, NavigationValidator: func(_ context.Context, target ai.ModelNavigationTarget) bool { return target.Valid() }})
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
	if err != nil || target.Kind != "overview" {
		t.Fatal("valid intent failed", err)
	}
	if _, err := r.ConsumeNavigation(t.Context(), q.Epoch, q.ConversationID, intent.RunID, intent.ID, intent.LeaseID); err == nil {
		t.Fatal("intent replayed")
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
