package version

import "testing"

func TestReleaseMetadataIsV036(t *testing.T) {
	if Version != "v0.3.6" {
		t.Fatalf("Version = %q, want v0.3.6", Version)
	}
	if Build != "7" {
		t.Fatalf("Build = %q, want 7", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
