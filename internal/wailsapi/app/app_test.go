package app_test

import (
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/version"
	"github.com/waltwang/nestworth-go/internal/wailsapi/app"
)

func TestAppInfoMatchesVersionPackage(t *testing.T) {
	service := app.NewService()
	info := service.AppInfo()
	if info.Name != version.Name || info.AppID != version.AppID || info.Version != version.Version || info.Build != version.Build {
		t.Fatalf("AppInfo() = %+v, want it to mirror internal/version", info)
	}
}

func TestAppInfoRoundTripsAsJSON(t *testing.T) {
	service := app.NewService()
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
