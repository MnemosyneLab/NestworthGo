package version

import "testing"

func TestReleaseMetadataIsV013(t *testing.T) {
	if Version != "v0.1.3" {
		t.Fatalf("Version = %q, want v0.1.3", Version)
	}
	if Build != "1" {
		t.Fatalf("Build = %q, want 1", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
