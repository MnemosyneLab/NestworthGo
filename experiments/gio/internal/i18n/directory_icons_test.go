package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestDirectoryIconsMatchWails(t *testing.T) {
	frontend := filepath.Join("..", "..", "..", "..", "frontend", "src")
	registry, err := os.ReadFile(filepath.Join(frontend, "lib", "iconCatalog.ts"))
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`\{ key: "([^"]+)", labelKey: "icons\.([^"]+)"`)
	choices := pattern.FindAllStringSubmatch(string(registry), -1)
	if len(choices) == 0 || len(choices) != len(directoryIcons) {
		t.Fatalf("Wails has %d selectable icons, Gio has %d", len(choices), len(directoryIcons))
	}
	for _, language := range []Language{English, SimplifiedChinese, TraditionalChinese, Japanese} {
		t.Run(string(language), func(t *testing.T) {
			bytes, err := os.ReadFile(filepath.Join(frontend, "i18n", "locales", string(language)+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var messages struct{ Icons map[string]any }
			if err := jsoncodec.Unmarshal(bytes, &messages); err != nil {
				t.Fatal(err)
			}
			for i, choice := range choices {
				key, labelKey := choice[1], choice[2]
				icon := directoryIcons[i]
				label, ok := messages.Icons[labelKey].(string)
				if !ok || label == "" || icon.ID != key || icon.Label(language) != label {
					t.Errorf("choice %d: Gio %s %q, Wails %s %q", i, icon.ID, icon.Label(language), key, label)
				}
				if err := domain.ValidateIconKey(icon.ID); err != nil {
					t.Errorf("icon %q: %v", icon.ID, err)
				}
			}
		})
	}
}
