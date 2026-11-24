package main

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
)

func TestCatalogCoverage(t *testing.T) {
	for language, translations := range catalogs {
		for id := languageName; id < messageCount; id++ {
			if strings.TrimSpace(translations[id]) == "" {
				t.Errorf("%s is missing message %d", language, id)
			}
		}
		for _, sample := range []struct {
			id   messageID
			args []any
		}{
			{fieldByteCount, []any{24, 48}},
			{fieldByteExceeded, []any{60, 48}},
			{previewSubmissions, []any{1}},
			{previewSavedName, []any{"Example Bank"}},
			{previewSavedNotes, []any{"Sample notes"}},
			{previewQuery, []any{"Bank"}},
			{dateToday, []any{"Feb 28, 2024"}},
			{timeHourDisplay, []any{12, "AM"}},
			{timeTwelveDisplay, []any{12, 0, "AM"}},
			{previewHarborBranch, []any{1}},
			{previewAccountResult, []any{"Cash account"}},
			{previewInstitutionResult, []any{"Harbor Bank"}},
			{previewDateResult, []any{"2024-02-29"}},
			{previewTimeResult, []any{"00:00"}},
			{institutionsEditName, []any{"Sample Bank"}},
			{institutionsNameLength, []any{120}},
		} {
			if text := fmt.Sprintf(translations[sample.id], sample.args...); strings.Contains(text, "%!") {
				t.Errorf("%s message %d has incompatible format parameters: %q", language, sample.id, text)
			}
		}
	}
	if got := appLanguage("unknown").text(navComponentPreview); got != english.text(navComponentPreview) {
		t.Errorf("unknown language gave %q instead of the English preview label", got)
	}
}

func TestComponentPreviewKeepsDraftAcrossPreferenceChanges(t *testing.T) {
	h := newShellHarness(t)
	shell, router := h.shell, &h.router
	edit := func(text string, selection key.Range) {
		router.Queue(key.EditEvent{Text: text})
		h.frame()
		router.Queue(key.SelectionEvent(selection), key.SnippetEvent{End: utf8.RuneCountInString(text)})
		h.frame()
	}
	assertEditor := func(text string, selection key.Range) {
		t.Helper()
		state := router.EditorState()
		if state.Snippet.Text != text || state.Selection.Range != selection {
			t.Fatalf("editor has draft %q and selection %+v, want %q and %+v", state.Snippet.Text, state.Selection.Range, text, selection)
		}
	}

	h.frame()
	h.click("组件预览", false)
	if shell.activeRoute != componentPreviewRoute {
		t.Fatalf("navigated to %q, want %q", shell.activeRoute, componentPreviewRoute)
	}
	h.click("机构名称", true)
	const name = "示例 Bank"
	nameSelection := key.Range{Start: 2, End: 2}
	edit(name, nameSelection)
	assertEditor(name, nameSelection)

	h.click("备注", true)
	const notes = "草稿 Draft\nSecond line"
	selection := key.Range{Start: 8, End: 3}
	edit(notes, selection)
	assertEditor(notes, selection)

	for _, step := range []struct {
		button   string
		language appLanguage
		colors   *palette
	}{
		{"English", english, &lightPalette},
		{"Dark", english, &darkPalette},
		{"简体中文", simplifiedChinese, &darkPalette},
		{"浅色", simplifiedChinese, &lightPalette},
	} {
		h.click(step.button, false)
		if shell.activeRoute != componentPreviewRoute || shell.preferences.language != step.language {
			t.Fatalf("after clicking %q, got route %q and language %q", step.button, shell.activeRoute, shell.preferences.language)
		}
		if got := shell.theme.Palette; got.Bg != step.colors.pageBackground || got.Fg != step.colors.ink || got.ContrastBg != step.colors.accent {
			t.Fatalf("after clicking %q, unexpected material palette %+v", step.button, got)
		}
		semanticBounds(t, router, step.language.text(previewNotes), true)
		assertEditor(notes, selection)
	}

	h.click("机构名称", true)
	router.Queue(key.SelectionEvent(nameSelection), key.SnippetEvent{End: utf8.RuneCountInString(name)})
	h.frame()
	assertEditor(name, nameSelection)
}

func semanticBounds(t *testing.T, router *input.Router, text string, field bool) image.Rectangle {
	t.Helper()
	var hasLabel func(input.SemanticNode) bool
	hasLabel = func(node input.SemanticNode) bool {
		if node.Desc.Label == text {
			return true
		}
		for _, child := range node.Children {
			if hasLabel(child) {
				return true
			}
		}
		return false
	}
	var find func([]input.SemanticNode) image.Rectangle
	find = func(nodes []input.SemanticNode) image.Rectangle {
		for _, node := range nodes {
			if (field && node.Desc.Description == text) || (!field && node.Desc.Class == semantic.Button && hasLabel(node)) {
				return node.Desc.Bounds
			}
			if child := find(node.Children); !child.Empty() {
				return child
			}
		}
		return image.Rectangle{}
	}
	bounds := find(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatalf("could not find visible semantic node %q", text)
	}
	return bounds
}
