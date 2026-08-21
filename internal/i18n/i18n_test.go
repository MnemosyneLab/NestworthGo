package i18n

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestCatalogsHaveTheSameVisibleKeys(t *testing.T) {
	for _, language := range []settings.Language{settings.LanguageEnglish, settings.LanguageZhCN, settings.LanguageZhTW} {
		if missing := MissingKeys(language); len(missing) != 0 {
			t.Fatalf("MissingKeys(%q) = %v", language, missing)
		}
	}
}

func TestSystemLanguageMapping(t *testing.T) {
	tests := []struct {
		locale string
		want   settings.Language
	}{
		{locale: "zh_TW.UTF-8", want: settings.LanguageZhTW},
		{locale: "zh_HK", want: settings.LanguageZhTW},
		{locale: "zh_CN.UTF-8", want: settings.LanguageZhCN},
		{locale: "zh_SG", want: settings.LanguageZhCN},
		{locale: "en_SG.UTF-8", want: settings.LanguageEnglish},
		{locale: "fr_FR", want: settings.LanguageEnglish},
	}
	for _, test := range tests {
		if got := languageFromLocale(test.locale); got != test.want {
			t.Errorf("languageFromLocale(%q) = %q, want %q", test.locale, got, test.want)
		}
	}
}

func TestTraditionalChineseUsesCorrectLanguageName(t *testing.T) {
	translator := New(settings.LanguageZhTW)
	if got := translator.T("option.language.zhTW"); got != "正體中文" {
		t.Fatalf("traditional language label = %q, want 正體中文", got)
	}
	if got := translator.T("option.language.zhTW"); got == "繁體中文" {
		t.Fatal("traditional language label still uses 繁體中文")
	}
}
