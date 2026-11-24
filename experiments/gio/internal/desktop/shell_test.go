package desktop

import (
	"image"
	"testing"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func TestComponentPreviewKeepsDraftAcrossPreferenceChanges(t *testing.T) {
	h := newShellHarness(t)
	shell, router := h.shell, &h.router
	preview := newComponentPreview()
	shell.routes[componentPreviewRoute].body = preview.layout
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
		button      string
		language    i18n.Language
		dark        bool
		accentIndex int
	}{
		{"English", i18n.English, false, 0},
		{"Dark", i18n.English, true, 0},
		{"Color theme: Indigo", i18n.English, true, 1},
		{"繁體中文", i18n.TraditionalChinese, true, 1},
		{"顏色主題：薰衣草", i18n.TraditionalChinese, true, 2},
		{"淺色", i18n.TraditionalChinese, false, 2},
		{"顏色主題：薄荷", i18n.TraditionalChinese, false, 3},
		{"顏色主題：天空", i18n.TraditionalChinese, false, 4},
		{"顏色主題：蜜桃", i18n.TraditionalChinese, false, 0},
		{"简体中文", i18n.SimplifiedChinese, false, 0},
	} {
		h.click(step.button, false)
		want := sessionPreferences{language: step.language, dark: step.dark, accentIndex: step.accentIndex}
		if shell.activeRoute != componentPreviewRoute || shell.preferences != want {
			t.Fatalf("after clicking %q, got route %q and preferences %+v, want preview and %+v", step.button, shell.activeRoute, shell.preferences, want)
		}
		colors := want.colors()
		if got := shell.theme.Palette; got.Bg != colors.pageBackground || got.Fg != colors.ink || got.ContrastBg != colors.accent || got.ContrastFg != colors.onAccent {
			t.Fatalf("after clicking %q, unexpected material palette %+v", step.button, got)
		}
		semanticBounds(t, router, step.language.Text(i18n.PreviewNotes), true)
		if preview.readOnly.text() != step.language.Text(i18n.PreviewHarbor) || preview.disabled.text() != step.language.Text(i18n.PreviewEvergreen) {
			t.Fatalf("after clicking %q, sample values were not localized", step.button)
		}
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
