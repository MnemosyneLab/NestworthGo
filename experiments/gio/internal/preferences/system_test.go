package preferences

import (
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestSystemPreferenceSources(t *testing.T) {
	if language := SystemLanguage(); language == settings.LanguageSystem || !slices.Contains(settings.AllLanguages(), language) {
		t.Fatalf("system preference resolved to unsupported language %q", language)
	}
	if name := SystemTimezone(); name == "" {
		t.Fatal("empty system timezone")
	} else if _, err := time.LoadLocation(name); err != nil {
		t.Fatalf("unusable system timezone %q: %v", name, err)
	}
	if zones := Timezones(); !slices.IsSorted(zones) || !slices.Contains(zones, "UTC") || !slices.Contains(zones, "Asia/Singapore") {
		t.Fatalf("incomplete IANA zone list (%d options)", len(zones))
	}
	stop := WatchSystemChanges(func() {})
	_ = SystemDark()
	stop()
	stop()
}
