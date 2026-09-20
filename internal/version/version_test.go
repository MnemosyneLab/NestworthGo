package version

import "testing"

func TestReleaseMetadataIsV035(t *testing.T) {
	if Version != "v0.3.5" {
		t.Fatalf("Version = %q, want v0.3.5", Version)
	}
	if Build != "6" {
		t.Fatalf("Build = %q, want 6", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
