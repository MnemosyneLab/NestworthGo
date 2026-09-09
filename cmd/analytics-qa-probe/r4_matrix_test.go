package main

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestR4SettingsMatrixCasesPass(t *testing.T) {
	cases := r4SettingsMatrixCases()
	if len(cases) != 8 {
		t.Fatalf("settings matrix cases = %d, want 8 (2 week + 3 locale + 3 window)", len(cases))
	}
	for _, c := range cases {
		if c.Status != "PASS" {
			t.Fatalf("%s status=%s detail=%s", c.ID, c.Status, c.Detail)
		}
	}
	s := settings.Default()
	s.WeekStart = settings.WeekStartSunday
	s.Language = settings.LanguageZhTW
	s.WindowWidth = 1440
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestR4HostMacOSIsBlockedOnLinux(t *testing.T) {
	for _, c := range r4HostCases() {
		if c.ID == "m_os_macos_apple_silicon" && c.Status != "BLOCKED" {
			t.Fatalf("macOS Apple Silicon must be BLOCKED on this host, got %s", c.Status)
		}
		if c.ID == "m_os_linux" && c.Status != "PASS" {
			t.Fatalf("linux host row: got %s", c.Status)
		}
	}
}
