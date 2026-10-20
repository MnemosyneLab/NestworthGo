package version

import "testing"

func TestReleaseMetadataIsV039(t *testing.T) {
	if Version != "v0.3.9" {
		t.Fatalf("Version = %q, want v0.3.9", Version)
	}
	if Build != "10" {
		t.Fatalf("Build = %q, want 10", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
