package version

import "testing"

func TestReleaseMetadataIsV032(t *testing.T) {
	if Version != "v0.3.2" {
		t.Fatalf("Version = %q, want v0.3.2", Version)
	}
	if Build != "3" {
		t.Fatalf("Build = %q, want 3", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
