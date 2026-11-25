package desktop

import (
	"errors"
	"image"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestSidebarLanguageAndAppearanceUseSavedPreferences(t *testing.T) {
	h := newShellHarness(t)
	p := h.shell.prefs
	p.saved.Language = settings.LanguageSystem
	p.saved.Appearance = settings.AppearanceSystem
	p.systemLanguage, p.systemDark = settings.LanguageJa, true
	h.frame()
	if h.shell.preferences.language != i18n.Japanese || !h.shell.preferences.dark {
		t.Fatal("system preferences were not resolved at the shell")
	}
	h.click(i18n.Japanese.Text(i18n.SettingsLanguage), false)
	h.press(key.NameReturn)
	if p.saved.Language != settings.LanguageSystem || p.busy {
		t.Fatal("confirming the effective language cleared the system preference")
	}
	h.click(i18n.Japanese.Text(i18n.SettingsLanguage), false)
	h.press(key.NameUpArrow)
	h.press(key.NameReturn)
	p.wait()
	h.frame()
	if p.saved.Language != settings.LanguageZhTW || h.shell.preferences.language != i18n.TraditionalChinese {
		t.Fatal("quick language selection did not update the shared setting")
	}

	for _, want := range []settings.Appearance{settings.AppearanceLight, settings.AppearanceDark, settings.AppearanceSystem} {
		h.cycleAppearance()
		if p.saved.Appearance != want {
			t.Fatalf("appearance after cycling = %q, want %q", p.saved.Appearance, want)
		}
	}
	store := p.store.(*testPreferenceStore)
	store.err = errors.New("failed test save")
	h.cycleAppearance()
	if p.saved.Appearance != settings.AppearanceSystem || !h.shell.preferences.dark {
		t.Fatal("a failed sidebar save changed the selected appearance")
	}
	assertPageLabel(t, &h.router, i18n.TraditionalChinese.Text(i18n.SidebarSaveError))
	store.err = nil
	h.cycleAppearance()
	if p.saved.Appearance != settings.AppearanceLight || h.shell.preferences.dark {
		t.Fatal("the sidebar could not retry the failed change")
	}
}

func TestNarrowSidebarKeepsItsControlsAndWidePreference(t *testing.T) {
	h := newShellHarness(t)
	h.size = image.Pt(959, 660)
	h.frame()
	if !h.shell.collapsed() {
		t.Fatal("narrow window did not open with the icon rail")
	}
	h.selectSidebarLanguage(i18n.SettingsLanguageEn)
	h.click("Toggle sidebar", false)
	if h.shell.collapsed() {
		t.Fatal("narrow sidebar did not expand")
	}
	h.click("Ledger activity", false)
	if h.shell.activeRoute() != historyRoute || !h.shell.collapsed() {
		t.Fatal("narrow navigation did not close the expanded sidebar")
	}
	h.size.X = 960
	h.frame()
	if h.shell.collapsed() {
		t.Fatal("narrow navigation changed the wide preference")
	}
	h.click("Toggle sidebar", false)
	if !h.shell.collapsed() {
		t.Fatal("wide sidebar did not collapse")
	}
	h.size.X = 820
	h.frame()
	h.click("Toggle sidebar", false)
	h.size.X = 1000
	h.frame()
	if !h.shell.collapsed() {
		t.Fatal("expanding a narrow sidebar overwrote the user's wide icon rail")
	}
}
