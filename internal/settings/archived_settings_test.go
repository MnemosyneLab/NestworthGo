package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	api "github.com/waltwang/nestworth-go/internal/wailsapi/settings"
)

func TestArchivedCredentialsSurviveLegacyImportSaveResetAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	value := settings.Default()
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := jsoncodec.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["tiingo_api_key"] = "synthetic-retired-secret"
	raw, err = jsoncodec.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewConfigurationRepository(db)
	store := settings.NewStore(path)
	if err := store.Attach(repo); err != nil {
		t.Fatal(err)
	}
	service := api.NewService(store, nil)
	dto, err := service.Load()
	if err != nil {
		t.Fatal(err)
	}
	dto.Appearance = settings.AppearanceDark
	if err := service.Save(dto); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reopened := settings.NewStore(path)
	if err := reopened.Attach(repo); err != nil {
		t.Fatal(err)
	}
	persisted, found, err := repo.LoadConfiguration("settings")
	if err != nil || !found {
		t.Fatalf("settings missing: %v", err)
	}
	var stored map[string]any
	if err := jsoncodec.Unmarshal(persisted, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["tiingo_api_key"] != "synthetic-retired-secret" {
		t.Fatal("opaque credential was lost")
	}
	public, err := jsoncodec.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	presentation, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{public, presentation} {
		var exposed map[string]any
		if err := jsoncodec.Unmarshal(payload, &exposed); err != nil {
			t.Fatal(err)
		}
		if _, ok := exposed["tiingo_api_key"]; ok {
			t.Fatal("retired credential exposed")
		}
	}
}
