package webassets

import "testing"

func TestDistEmbedsFrontendDirectory(t *testing.T) {
	entries, err := Dist.ReadDir("frontend/dist")
	if err != nil {
		t.Fatalf("embedded frontend/dist is unreadable: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded frontend/dist is empty; keep frontend/dist/.gitkeep so go:embed succeeds on a clean checkout")
	}
}
