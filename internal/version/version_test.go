package version

import "testing"

func TestReleaseMetadataIsV012(t *testing.T) {
	if Version != "v0.1.2" {
		t.Fatalf("Version = %q, want v0.1.2", Version)
	}
	if Build != "1" {
		t.Fatalf("Build = %q, want 1", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
