package version

import "testing"

func TestReleaseMetadataIsV037(t *testing.T) {
	if Version != "v0.3.7" {
		t.Fatalf("Version = %q, want v0.3.7", Version)
	}
	if Build != "8" {
		t.Fatalf("Build = %q, want 8", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
