package version

import "testing"

func TestReleaseMetadataIsV0310(t *testing.T) {
	if Version != "v0.3.10" {
		t.Fatalf("Version = %q, want v0.3.10", Version)
	}
	if Build != "11" {
		t.Fatalf("Build = %q, want 11", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
