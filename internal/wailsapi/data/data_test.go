package data

import (
	"context"
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/application"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type memoryDialogs struct {
	save, open string
	replace    bool
}

func (m memoryDialogs) SaveFile(string, string, string, string) (string, error) { return m.save, nil }
func (m memoryDialogs) OpenFile(string, string, string) (string, error)         { return m.open, nil }
func (m memoryDialogs) ConfirmReplace(string) (bool, error)                     { return m.replace, nil }

func TestCreateBackupCancel(t *testing.T) {
	app := wailstest.NewService(t)
	service := NewService(app, nil, memoryDialogs{}, native.NoopRefresh{})
	result, err := service.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled {
		t.Fatal("empty save path should cancel")
	}
}

func TestLastBackupStatusMissing(t *testing.T) {
	service := NewService(nil, nil, nil, nil)
	status, err := service.LastBackupStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Fatal("expected no backup status")
	}
}

func TestExportJSONCancelAndReplaceDeclined(t *testing.T) {
	app := wailstest.NewService(t)
	service := NewService(app, nil, memoryDialogs{}, nil)
	result, err := service.ExportJSON()
	if err != nil || !result.Cancelled {
		t.Fatalf("cancel = %+v, %v", result, err)
	}
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	service.dialogs = memoryDialogs{save: path}
	result, err = service.ExportJSON()
	if err != nil || !result.Cancelled {
		t.Fatalf("decline = %+v, %v", result, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep me" {
		t.Fatal("declined export replaced the existing file")
	}
}

func TestExportJSONWritesStructuredFile(t *testing.T) {
	app := wailstest.NewService(t)
	if err := app.CompleteOnboarding(context.Background(), application.OnboardingInput{HouseholdName: "Export", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export")
	service := NewService(app, nil, memoryDialogs{save: path}, nil)
	result, err := service.ExportJSON()
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.FileName != "export.nestworth.json" {
		t.Fatalf("result = %+v", result)
	}
	path += ".nestworth.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Format        string `json:"format"`
		FormatVersion int    `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != "com.nestworth.export" || doc.FormatVersion != 2 {
		t.Fatalf("document = %+v", doc)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %v", info.Mode())
	}
}
