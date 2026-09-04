package version

import "testing"

func TestReleaseMetadataIsV031(t *testing.T) {
	if Version != "v0.3.1" {
		t.Fatalf("Version = %q, want v0.3.1", Version)
	}
	if Build != "2" {
		t.Fatalf("Build = %q, want 2", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
