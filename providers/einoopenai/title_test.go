package einoopenai

import (
	"net/http"
	"strings"
	"testing"
)

func TestTitleProviderHasNoToolsHistoryOrLedgerContext(t *testing.T) {
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		if round != 0 {
			t.Error("title retried")
		}
		if tools, ok := request["tools"].([]any); ok && len(tools) != 0 {
			t.Error("title exposed tools")
		}
		input := request["input"].([]any)
		if len(input) != 2 || !strings.Contains(encode(input[1]), "Synthetic title question") {
			t.Error("wrong title input")
		}
		writeMode(w, request, []any{textItem()})
	})
	title, err := titleWithModel(t.Context(), fake.model, "Synthetic title question")
	if err != nil || title != "Synthetic complete" {
		t.Fatal(title, err)
	}
}
func TestTitleProviderRejectsToolOutput(t *testing.T) {
	fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		writeMode(w, request, []any{callItem()})
	})
	if _, err := titleWithModel(t.Context(), fake.model, "Synthetic"); err == nil {
		t.Fatal("accepted tool output")
	}
}
