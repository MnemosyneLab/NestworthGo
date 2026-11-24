package preferences

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func SystemLanguage() settings.Language {
	locale := preferredLocale()
	if locale == "" {
		for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
			if locale = strings.TrimSpace(os.Getenv(key)); locale != "" {
				break
			}
		}
	}
	return settings.ResolveLanguage(settings.LanguageSystem, locale)
}

var zoneCatalog = sync.OnceValue(func() []string {
	// The tzdb zone table lists selectable geographic names without the
	// duplicate aliases in the full zoneinfo directory. UTC is a separate
	// non-geographic option in the Wails picker.
	zones := []string{"UTC"}
	file, err := os.Open("/usr/share/zoneinfo/zone.tab")
	if err != nil {
		return zones
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := lines.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if columns := strings.Split(line, "\t"); len(columns) >= 3 {
			zones = append(zones, columns[2])
		}
	}
	slices.Sort(zones)
	return slices.Compact(zones)
})

func Timezones() []string { return slices.Clone(zoneCatalog()) }
