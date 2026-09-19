package version

import "testing"

func TestReleaseMetadataIsV034(t *testing.T) {
	if Version != "v0.3.4" {
		t.Fatalf("Version = %q, want v0.3.4", Version)
	}
	if Build != "5" {
		t.Fatalf("Build = %q, want 5", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
