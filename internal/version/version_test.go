package version

import "testing"

func TestReleaseMetadataIsV0311(t *testing.T) {
	if Version != "v0.3.11" {
		t.Fatalf("Version = %q, want v0.3.11", Version)
	}
	if Build != "12" {
		t.Fatalf("Build = %q, want 12", Build)
	}
	if AppID != "com.nestworth.app" {
		t.Fatalf("AppID = %q, want com.nestworth.app", AppID)
	}
}
