package agentguidance

import (
	"os"
	"strings"
	"testing"
)

func TestRecipesSharePortableTextAndStayCompact(t *testing.T) {
	ids, tools := map[string]bool{}, map[string]bool{}
	total := 0
	for _, r := range recipes {
		if r.ID == "" || r.Tool == "" || r.Text == "" || ids[r.ID] || tools[r.Tool] {
			t.Fatal("duplicate/incomplete recipe")
		}
		ids[r.ID], tools[r.Tool] = true, true
		raw, err := os.ReadFile("../../skills/nestworth/references/" + r.Reference + ".md")
		if err != nil || !strings.Contains(string(raw), r.Text) {
			t.Fatalf("portable skill drift: %s, %v", r.ID, err)
		}
		body := Description(r.Tool, "")
		if len(body) > 2400 {
			t.Fatalf("recipe %s grew to %d bytes", r.ID, len(body))
		}
		total += len(body)
	}
	if total > 7500 {
		t.Fatalf("business recipes grew to %d bytes", total)
	}
	if Description("future_tool", "unchanged") != "unchanged" {
		t.Fatal("recipe added to unknown entry tool")
	}
	t.Logf("shared recipes=%d, total description bytes=%d", len(recipes), total)
}
