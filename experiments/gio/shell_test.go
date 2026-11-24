package main

import (
	"image"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestCatalogCoverage(t *testing.T) {
	for language, translations := range catalogs {
		for id := languageName; id < messageCount; id++ {
			if strings.TrimSpace(translations[id]) == "" {
				t.Errorf("%s is missing message %d", language, id)
			}
		}
	}
	if got := appLanguage("unknown").text(navComponentPreview); got != english.text(navComponentPreview) {
		t.Errorf("unknown language gave %q instead of the English preview label", got)
	}
}

func TestComponentPreviewKeepsDraftAcrossPreferenceChanges(t *testing.T) {
	brand := image.NewRGBA(image.Rect(0, 0, 1, 1))
	shell := newShell(brand, brand)
	var router input.Router
	var ops op.Ops
	now := time.Now()
	frame := func() {
		ops.Reset()
		now = now.Add(time.Second / 60)
		shell.layout(layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1120, 840)),
		})
		router.Frame(&ops)
	}
	click := func(label string, field bool) {
		t.Helper()
		bounds := semanticBounds(t, &router, label, field)
		center := bounds.Min.Add(bounds.Max).Div(2)
		pos := f32.Pt(float32(center.X), float32(center.Y))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
	}
	edit := func(text string, selection key.Range) {
		router.Queue(key.EditEvent{Text: text})
		frame()
		router.Queue(key.SelectionEvent(selection), key.SnippetEvent{End: utf8.RuneCountInString(text)})
		frame()
	}
	assertEditor := func(text string, selection key.Range) {
		t.Helper()
		state := router.EditorState()
		if state.Snippet.Text != text || state.Selection.Range != selection {
			t.Fatalf("editor has draft %q and selection %+v, want %q and %+v", state.Snippet.Text, state.Selection.Range, text, selection)
		}
	}

	frame()
	click("组件预览", false)
	if shell.activeRoute != componentPreviewRoute {
		t.Fatalf("navigated to %q, want %q", shell.activeRoute, componentPreviewRoute)
	}
	click("机构名称", true)
	const name = "示例 Bank"
	nameSelection := key.Range{Start: 2, End: 2}
	edit(name, nameSelection)
	assertEditor(name, nameSelection)

	click("备注", true)
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
		click(step.button, false)
		if shell.activeRoute != componentPreviewRoute || shell.preferences.language != step.language {
			t.Fatalf("after clicking %q, got route %q and language %q", step.button, shell.activeRoute, shell.preferences.language)
		}
		if got := shell.theme.Palette; got.Bg != step.colors.pageBackground || got.Fg != step.colors.ink || got.ContrastBg != step.colors.accent {
			t.Fatalf("after clicking %q, unexpected material palette %+v", step.button, got)
		}
		semanticBounds(t, &router, step.language.text(previewNotes), true)
		assertEditor(notes, selection)
	}

	click("机构名称", true)
	router.Queue(key.SelectionEvent(nameSelection), key.SnippetEvent{End: utf8.RuneCountInString(name)})
	frame()
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
