package desktop

import (
	"fmt"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
)

func TestSettingsTabsSharePreferenceDraft(t *testing.T) {
	h, p := generalSettingsHarness(t)
	p.prefs.saved.LogPath = "/synthetic/logs/nestworth.log"
	h.frame()
	h.click("Date format", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.click("Market data", false)
	h.click("Quote cache duration", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.click("Diagnostics", false)
	assertPageLabel(t, &h.router, p.prefs.saved.LogPath)
	h.click("Application file logging", true)
	for range 3 {
		h.press(key.NameDownArrow)
	}
	h.press(key.NameReturn)
	h.router.Source().Execute(key.FocusCmd{Tag: &p.tabs[settingsDiagnostics]})
	h.frame()
	h.press(key.NameRightArrow)
	h.focused(&p.tabs[settingsAbout])
	assertPageLabel(t, &h.router, pilot.DisplayName)
	assertPageLabel(t, &h.router, fmt.Sprintf(i18n.English.Text(i18n.SettingsAboutVersion), version.Version, version.Build))
	assertPageLabel(t, &h.router, "MIT License")
	assertPageLabel(t, &h.router, pilot.AppID)
	if p.prefs.saved.DateFormat != settings.DateFormatISO || p.prefs.saved.QuoteCacheTTL != settings.QuoteCacheTTL12h || p.prefs.saved.LogLevel != "off" {
		t.Fatal("changing tabs applied unsaved preferences")
	}
	h.click("Save changes", false)
	p.prefs.wait()
	h.frame()
	if got := p.prefs.saved; got.DateFormat != settings.DateFormatDayFirst || got.QuoteCacheTTL != settings.QuoteCacheTTL24h || got.LogLevel != "info" || len(p.edits) != 0 {
		t.Fatalf("saving from About did not apply the shared draft: %+v", got)
	}
	h.click("Overview", false)
	h.click("Back", false)
	if p.active != settingsAbout {
		t.Fatal("Back did not restore the Settings tab")
	}
}

func TestSettingsCoinGeckoKeyUsesItsOwnSave(t *testing.T) {
	h, p := generalSettingsHarness(t)
	p.edit(i18n.SettingsQuoteCacheTTL, settings.QuoteCacheTTL1h)
	h.click("Market data", false)
	assertPageLabel(t, &h.router, "No CoinGecko key is configured.")
	h.click("CoinGecko API key", true)
	h.router.Queue(key.EditEvent{Text: "  fixture-key  "})
	h.frame()
	if p.credential.input.Editor.Mask == 0 {
		t.Fatal("the unsaved key was visible by default")
	}
	h.click("Show new input", false)
	if p.credential.input.Editor.Mask != 0 {
		t.Fatal("the visibility toggle did not reveal the new input")
	}
	h.click("Save key", false)
	p.prefs.wait()
	h.frame()
	if !p.prefs.saved.CoinGeckoConfigured || p.prefs.saved.QuoteCacheTTL != settings.QuoteCacheTTL12h || p.draft().QuoteCacheTTL != settings.QuoteCacheTTL1h || p.credential.input.Text() != "" {
		t.Fatal("saving the key changed the preferences draft or retained plaintext")
	}
	h.click("Replace", false)
	h.focused(&p.credential.input.Editor)
	if p.credential.input.Text() != "" || p.credential.input.Editor.Mask == 0 {
		t.Fatal("replacement exposed the stored key")
	}
	h.click("Cancel", false)
	h.click("Remove key", false)
	p.prefs.wait()
	h.frame()
	if p.prefs.saved.CoinGeckoConfigured || p.draft().QuoteCacheTTL != settings.QuoteCacheTTL1h {
		t.Fatal("removing the key did not preserve the preference draft")
	}
}
