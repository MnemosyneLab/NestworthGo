package preferences

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestWindowSizePersistsWithoutOverwritingPreferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	db, err := sqlite.Open(filepath.Join(dir, "household.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	app := application.NewService(sqlite.NewRepository(db))
	service, err := Open(path, db, app)
	if err != nil {
		t.Fatal(err)
	}
	defaults := image.Pt(settings.DefaultWindowWidth, settings.DefaultWindowHeight)
	if got := service.WindowSize(); got != defaults {
		t.Fatalf("initial window size %v, want %v", got, defaults)
	}
	before, err := service.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetCoinGeckoKey(ctx, "fixture-key"); err != nil {
		t.Fatal(err)
	}
	want := image.Pt(900, 590)
	if err := service.saveWindowSize(ctx, want); err != nil {
		t.Fatal(err)
	}
	after := before
	after.Accent = settings.AccentMint
	if _, err := service.Save(ctx, before, after); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, db, app)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.saveWindowSize(ctx, image.Pt(want.X, settings.MinWindowHeight-1)); err == nil {
		t.Fatal("out-of-bounds window size was accepted")
	}
	if got := reopened.WindowSize(); got != want {
		t.Fatalf("reopened window size %v, want %v", got, want)
	}
	if got, err := reopened.Load(ctx); err != nil || got.Accent != settings.AccentMint || !got.CoinGeckoConfigured {
		t.Fatalf("geometry did not preserve the latest preferences and credential: %+v, %v", got, err)
	}
	if _, err := reopened.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reopened.WindowSize(); got != defaults {
		t.Fatalf("reset window size %v, want %v", got, defaults)
	}
}
