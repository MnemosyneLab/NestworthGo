package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/version"
	"github.com/waltwang/nestworth-go/internal/wailsapi/app"
)

func TestAppInfoMatchesVersionPackage(t *testing.T) {
	service := app.NewService(nil)
	info := service.AppInfo()
	if info.Name != version.Name || info.AppID != version.AppID || info.Version != version.Version || info.Build != version.Build {
		t.Fatalf("AppInfo() = %+v, want it to mirror internal/version", info)
	}
}

func TestAppInfoRoundTripsAsJSON(t *testing.T) {
	service := app.NewService(nil)
	encoded, err := json.Marshal(service.AppInfo())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded app.AppInfoDTO
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Name != version.Name {
		t.Fatalf("decoded.Name = %q, want %q", decoded.Name, version.Name)
	}
}

func TestStartupAvailableWhenDatabaseOpened(t *testing.T) {
	startup := app.NewService(nil).Startup()
	if !startup.Available {
		t.Fatalf("Startup() = %+v, want available", startup)
	}
	if startup.Code != "" || startup.Message != "" {
		t.Fatalf("available Startup must omit error fields, got %+v", startup)
	}
}

func TestStartupReportsUnavailableAsWireErrorShape(t *testing.T) {
	original := &domain.Error{Code: domain.ErrUnavailable, Field: "database", Message: "the local database could not be opened"}
	startup := app.NewService(original).Startup()
	if startup.Available {
		t.Fatal("Startup() available = true, want false")
	}
	if startup.Code != string(domain.ErrUnavailable) || startup.Field != "database" {
		t.Fatalf("Startup() = %+v, want unavailable/database", startup)
	}
	encoded, err := json.Marshal(startup)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"available":false`) {
		t.Fatalf("encoded startup = %s", encoded)
	}
}

func TestStartupGenericErrorNeverLeaksDetail(t *testing.T) {
	sensitive := fmt.Errorf("sqlite: open /Users/alice/Library/nestworth.db: permission denied")
	startup := app.NewService(sensitive).Startup()
	if startup.Available {
		t.Fatal("Startup() available = true, want false")
	}
	if startup.Code != "internal" {
		t.Fatalf("Code = %q, want internal", startup.Code)
	}
	if strings.Contains(startup.Message, "/Users/alice") || strings.Contains(startup.Message, "sqlite") {
		t.Fatalf("Startup leaked underlying detail: %+v", startup)
	}
}
