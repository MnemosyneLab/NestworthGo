package assistant

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// Keep the private runtime vocabulary aligned with the desktop's exact-code
// allowlist. Localized guidance is separately covered by locale/UI tests.
func TestDesktopBudgetVocabularyMatchesRuntime(t *testing.T) {
	raw, err := os.ReadFile("../../frontend/src/features/assistant/ConversationTimeline.tsx")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`"assistant_budget_exceeded\.([a-z_]+)":`).FindAllStringSubmatch(string(raw), -1)
	got := []string{}
	for _, match := range matches {
		got = append(got, match[1])
	}
	want := []string{}
	for rule := range budgetRules {
		want = append(want, rule)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("desktop budget vocabulary %v differs from runtime %v", got, want)
	}
}
