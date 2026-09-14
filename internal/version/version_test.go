package version

import "testing"

func TestReleaseMetadataIsV033(t *testing.T) {
	if Version != "v0.3.3" {
		t.Fatalf("Version = %q, want v0.3.3", Version)
	}
	if Build != "4" {
		t.Fatalf("Build = %q, want 4", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
