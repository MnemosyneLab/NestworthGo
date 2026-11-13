package settings

import "strings"

// ResolveLanguage returns the language used for display, without changing the
// stored preference. System locales can be BCP 47 tags or POSIX locale names.
func ResolveLanguage(preference Language, systemLocale string) Language {
	switch preference {
	case LanguageEnglish, LanguageZhCN, LanguageZhTW, LanguageJa:
		return preference
	case LanguageSystem:
		locale, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(systemLocale)), ".")
		locale, _, _ = strings.Cut(locale, "@")
		parts := strings.Split(strings.ReplaceAll(locale, "_", "-"), "-")
		if parts[0] == "ja" {
			return LanguageJa
		}
		if parts[0] != "zh" {
			return LanguageEnglish
		}
		for _, part := range parts[1:] {
			switch part {
			case "hant":
				return LanguageZhTW
			case "hans":
				return LanguageZhCN
			}
		}
		for _, part := range parts[1:] {
			switch part {
			case "tw", "hk", "mo":
				return LanguageZhTW
			}
		}
		return LanguageZhCN
	default:
		return LanguageEnglish
	}
}
