package settings

import "testing"

func TestResolveLanguage(t *testing.T) {
	for _, tt := range []struct {
		preference Language
		locale     string
		want       Language
	}{
		{LanguageSystem, "zh_HK.UTF-8", LanguageZhTW},
		{LanguageSystem, "zh-Hant-CN", LanguageZhTW},
		{LanguageSystem, "zh-Hans-TW", LanguageZhCN},
		{LanguageSystem, "zh-SG", LanguageZhCN},
		{LanguageSystem, "en-SG", LanguageEnglish},
		{LanguageSystem, "ja_JP.UTF-8", LanguageJa},
		{LanguageJa, "en-SG", LanguageJa},
		{LanguageZhCN, "zh-TW", LanguageZhCN},
		{LanguageEnglish, "zh-CN", LanguageEnglish},
	} {
		if got := ResolveLanguage(tt.preference, tt.locale); got != tt.want {
			t.Errorf("ResolveLanguage(%q, %q) = %q, want %q", tt.preference, tt.locale, got, tt.want)
		}
	}
}
