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
		{locale: "zh_MO", want: settings.LanguageZhTW},
		{locale: "zh_Hant_TW", want: settings.LanguageZhTW},
		{locale: "zh-Hant", want: settings.LanguageZhTW},
		{locale: "zh_TW@pinyin", want: settings.LanguageZhTW},
		{locale: "zh_CN.UTF-8", want: settings.LanguageZhCN},
		{locale: "zh_SG", want: settings.LanguageZhCN},
		{locale: "zh-Hans-HK", want: settings.LanguageZhCN},
		{locale: "zh", want: settings.LanguageZhCN},
		{locale: "en_SG.UTF-8", want: settings.LanguageEnglish},
		{locale: "fr_FR", want: settings.LanguageEnglish},
	}
	for _, test := range tests {
		if got := languageFromLocale(test.locale); got != test.want {
			t.Errorf("languageFromLocale(%q) = %q, want %q", test.locale, got, test.want)
		}
	}
}

// The catalog is assembled from two source tables (translations and
// errorTranslations); this guards the assembly-time contract that no key may
// be written twice — the failure mode the removed v011/v012 patch files
// relied on.
func TestCatalogAssemblyRejectsDuplicateWrites(t *testing.T) {
	_, conflicts := buildCatalogs(translations, errorTranslations)
	if len(conflicts) != 0 {
		t.Fatalf("buildCatalogs reported duplicate key writes: %v", conflicts)
	}

	extra := map[string]translation{"common.add": {english: "clash", simplified: "冲突", traditional: "衝突"}}
	_, conflicts = buildCatalogs(translations, extra)
	if len(conflicts) != 1 || conflicts[0] != "common.add" {
		t.Fatalf("buildCatalogs(extra) conflicts = %v, want [common.add]", conflicts)
	}
}

// Keys that historically lived in version-patch files which overwrote the
// main catalog; pinned so a future merge cannot silently flip them back.
func TestPatchedCatalogValuesAreCanonical(t *testing.T) {
	want := map[settings.Language]map[string]string{
		settings.LanguageEnglish: {
			"settings.reset.body":       "All presentation choices and market-data routing will return to their defaults.",
			"settings.numbers.currency": "Sample display currency (preview only)",
			"common.pending":            "Saving…",
		},
		settings.LanguageZhCN: {
			"settings.numbers.currency": "示例展示货币（仅用于下方预览）",
			"accounts.datePlaceholder":  "选择日期",
		},
		settings.LanguageZhTW: {
			"nav.members":              "成員",
			"accounts.datePlaceholder": "選擇日期",
		},
	}
	for language, expectations := range want {
		for key, value := range expectations {
			if got := New(language).T(key); got != value {
				t.Errorf("T(%q, %q) = %q, want %q", language, key, got, value)
			}
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
