package version

import "testing"

func TestReleaseMetadataIsV038(t *testing.T) {
	if Version != "v0.3.8" {
		t.Fatalf("Version = %q, want v0.3.8", Version)
	}
	if Build != "9" {
		t.Fatalf("Build = %q, want 9", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
