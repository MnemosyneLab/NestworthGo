package main

import (
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestDesktopRuntimeStartsClosedInAnIsolatedHousehold(t *testing.T) {
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
	if r, e := openAssistantRuntime(app, path, filepath.Join(dir, "settings.json"), db); e == nil || r != nil {
		t.Fatal("runtime opened without household")
	}
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic runtime", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: "UTC", HistoryStartDate: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	r, err := openAssistantRuntime(app, path, filepath.Join(dir, "settings.json"), db)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Snapshot()
	if s.Available || s.FullAccess || s.Config.Enabled || s.Credentials != "missing" {
		t.Fatal("test/ad-hoc runtime gained authority")
	}
	if err := r.SetCredential(t.Context(), s.Epoch, []byte("synthetic-desktop-key")); err != nil {
		t.Fatal(err)
	}
	c, e := r.Create(t.Context(), s.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	if e := r.FenceForRestore(); e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	r, e = openAssistantRuntime(app, path, filepath.Join(dir, "settings.json"), db)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	newStatus := r.Snapshot()
	if newStatus.Credentials != "present" || newStatus.Available || newStatus.FullAccess {
		t.Fatal("credential restart/closed gate regression")
	}
	if newStatus.Epoch == s.Epoch {
		t.Fatal("process identity reused")
	}
	if _, e := r.Inspect(t.Context(), newStatus.Epoch, c.ID); e == nil {
		t.Fatal("restored namespace reused old conversation")
	}
}
