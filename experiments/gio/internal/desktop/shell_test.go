package desktop

import (
	"image"
	"testing"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"github.com/waltwang/nestworth-go/internal/settings"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func TestManagementGroupFollowsSidebarFooter(t *testing.T) {
	h := newShellHarness(t)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1280, 960)
	h.frame()
	heading := assertPageLabel(t, &h.router, "Manage")
	for _, removed := range []string{"Development", "Input method test"} {
		if findPageLabel(&h.router, removed) != nil {
			t.Fatalf("unexpected sidebar label %q", removed)
		}
	}
	if _, exists := h.shell.routes["input-method-test"]; exists {
		t.Fatal("standalone input method page is still registered")
	}
	// A taller window leaves more space between Insights and Manage. Every
	// management destination, including the preview, moves with the footer.
	lastInsight := semanticBounds(t, &h.router, "Asset changes", false)
	management := []string{"Directory", "Data maintenance", "Settings", "Component Preview"}
	positions := make([]image.Rectangle, len(management))
	prev := lastInsight
	for i, name := range management {
		positions[i] = semanticBounds(t, &h.router, name, false)
		if positions[i].Min.Y < prev.Max.Y || (i > 0 && positions[i].Min.Y-prev.Max.Y >= prev.Dy()) {
			t.Fatalf("%s is not the next row of Manage: %v after %v", name, positions[i], prev)
		}
		prev = positions[i]
	}
	footer := semanticBounds(t, &h.router, "Language", false)
	if gap := footer.Min.Y - prev.Max.Y; gap < 0 || gap > prev.Dy() {
		t.Fatalf("last management row %v is not next to the footer %v", prev, footer)
	}
	const increase = 180
	h.size.Y += increase
	h.frame()
	if got := assertPageLabel(t, &h.router, "Manage"); got != heading.Add(image.Pt(0, increase)) {
		t.Fatalf("management heading did not move with its rows: %v -> %v", heading, got)
	}
	if got := semanticBounds(t, &h.router, "Asset changes", false); got != lastInsight {
		t.Fatalf("resizing moved everyday navigation: %v -> %v", lastInsight, got)
	}
	for i, name := range management {
		if got := semanticBounds(t, &h.router, name, false); got != positions[i].Add(image.Pt(0, increase)) {
			t.Fatalf("resizing by %d did not move %s with the footer: %v -> %v", increase, name, positions[i], got)
		}
	}
}

func TestShortSidebarScrollsToManagementPreview(t *testing.T) {
	h := newShellHarness(t)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1120, 400)
	h.frame()
	footer := semanticBounds(t, &h.router, "Language", false)
	top := semanticBounds(t, &h.router, "Overview", false)
	pos := f32.Pt(float32(top.Min.X+10), float32(top.Min.Y+10))
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: pos, Scroll: f32.Pt(0, 2000)})
	h.frame()
	preview := semanticBounds(t, &h.router, "Component Preview", false)
	if preview.Min.Y <= 0 || preview.Max.Y > footer.Min.Y {
		t.Fatalf("scroll did not expose the component preview above the footer: %v, footer %v", preview, footer)
	}
	if got := semanticBounds(t, &h.router, "Language", false); got != footer {
		t.Fatalf("scrolling sidebar navigation moved the footer: %v -> %v", footer, got)
	}
	h.click("Component Preview", false)
	if h.shell.activeRoute() != componentPreviewRoute {
		t.Fatalf("clicking the scrolled preview row opened %q", h.shell.activeRoute())
	}
	semanticBounds(t, &h.router, i18n.English.Text(i18n.PreviewInstitutionName), true)
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: pos, Scroll: f32.Pt(0, -2000)})
	h.frame()
	if got := semanticBounds(t, &h.router, "Overview", false); got != top {
		t.Fatalf("scrolling back did not restore the top navigation row: %v -> %v", top, got)
	}
	h.click("Overview", false)
	if h.shell.activeRoute() != overviewRoute {
		t.Fatal("top navigation row was no longer reachable")
	}
}

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
	if shell.activeRoute() != componentPreviewRoute {
		t.Fatalf("navigated to %q, want %q", shell.activeRoute(), componentPreviewRoute)
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

	check := func(language i18n.Language, dark bool, accent int) {
		t.Helper()
		want := sessionPreferences{language: language, dark: dark, accentIndex: accent}
		if shell.activeRoute() != componentPreviewRoute || shell.preferences != want {
			t.Fatalf("got route %q and preferences %+v, want preview and %+v", shell.activeRoute(), shell.preferences, want)
		}
		colors := want.colors()
		if got := shell.theme.Palette; got.Bg != colors.PageBackground || got.Fg != colors.Ink || got.ContrastBg != colors.Accent || got.ContrastFg != colors.OnAccent {
			t.Fatalf("unexpected material palette %+v", got)
		}
		semanticBounds(t, router, language.Text(i18n.PreviewNotes), true)
		if preview.readOnly.Text() != language.Text(i18n.PreviewHarbor) || preview.disabled.Text() != language.Text(i18n.PreviewEvergreen) {
			t.Fatal("sample values were not localized")
		}
		start, end := preview.notes.Editor.Selection()
		if preview.notes.Text() != notes || start != selection.Start || end != selection.End {
			t.Fatalf("preference change lost the notes draft or selection: %q / %d..%d", preview.notes.Text(), start, end)
		}
	}
	h.selectSidebarLanguage(i18n.SettingsLanguageEn)
	check(i18n.English, false, 0)
	h.cycleAppearance()
	check(i18n.English, true, 0)
	h.selectSidebarLanguage(i18n.SettingsLanguageJa)
	check(i18n.Japanese, true, 0)
	h.selectSidebarLanguage(i18n.SettingsLanguageZhTW)
	check(i18n.TraditionalChinese, true, 0)
	// Accent is edited on the settings page. Exercise the same confirmed
	// preference publication while the preview owns a selected draft.
	for _, index := range []int{1, 2, 3, 4, 0} {
		before := shell.prefs.saved
		after := before
		after.Accent = settings.AllAccents()[index]
		shell.prefs.save(before, after, nil)
		shell.prefs.wait()
		h.frame()
		check(i18n.TraditionalChinese, true, index)
	}
	h.cycleAppearance()
	check(i18n.TraditionalChinese, false, 0)
	h.selectSidebarLanguage(i18n.SettingsLanguageZhCN)
	check(i18n.SimplifiedChinese, false, 0)

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
			action := node.Desc.Class == semantic.Button || node.Desc.Class == semantic.CheckBox || node.Desc.Class == semantic.RadioButton
			if (field && node.Desc.Description == text) || (!field && action && hasLabel(node)) {
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

func (h *previewHarness) selectSidebarLanguage(option i18n.MessageID) {
	h.t.Helper()
	lang := h.shell.preferences.language
	h.click(lang.Text(i18n.SettingsLanguage), false)
	h.click(lang.Text(option), false)
	h.shell.prefs.wait()
	h.frame()
}

func (h *previewHarness) cycleAppearance() {
	h.t.Helper()
	h.click(h.shell.preferences.language.Text(i18n.SidebarToggleAppearance), false)
	h.shell.prefs.wait()
	h.frame()
}
