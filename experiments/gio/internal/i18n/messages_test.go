package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestCatalogCoverage(t *testing.T) {
	languages := slices.DeleteFunc(settings.AllLanguages(), func(lang settings.Language) bool {
		return lang == settings.LanguageSystem
	})
	if len(catalogs) != len(languages) {
		t.Errorf("registered %d catalogs, want %d", len(catalogs), len(languages))
	}
	for _, supported := range languages {
		language := Language(supported)
		translations := catalogs[language]
		if translations == nil {
			t.Errorf("missing catalog for %s", language)
			continue
		}
		for id := NoMessage + 1; id < messageCount; id++ {
			base, value := englishCatalog[id], translations[id]
			if strings.TrimSpace(value) == "" || language.Text(id) != value {
				t.Errorf("%s is missing key %d (%q)", language, id, base)
				continue
			}
			want, err := formatSignature(base)
			got, translationErr := formatSignature(value)
			if err != nil || translationErr != nil || !slices.Equal(want, got) {
				t.Errorf("%s key %d (%q): format %q (%v) in %q, want %q (%v)", language, id, base, got, translationErr, value, want, err)
			}
		}
	}
	if got := Language("unknown").Text(NavComponentPreview); got != English.Text(NavComponentPreview) {
		t.Errorf("unknown language gave %q instead of the English preview label", got)
	}
}

// Go's positional syntax lets translators move an argument while preserving
// its type and padding (for example, Japanese places the period before the hour).
var formatDirective = regexp.MustCompile(`^%([0-9]*)(?:\[([1-9][0-9]*)\])?([ds])`)

func formatSignature(value string) ([]string, error) {
	if strings.Contains(value, "{{") {
		return nil, fmt.Errorf("unconverted translation placeholder")
	}
	var signature []string
	next := 1
	for {
		_, rest, found := strings.Cut(value, "%")
		if !found {
			break
		}
		if strings.HasPrefix(rest, "%") {
			value = rest[1:]
			continue
		}
		// Plain UI copy may contain a percent sign, as in "100%" or "50% off".
		// Go's supported catalog directives begin with an index, width or verb.
		if rest == "" || strings.ContainsAny(rest[:1], " \n\t,.;:!?)") || rest[0] >= 0x80 {
			value = rest
			continue
		}
		m := formatDirective.FindStringSubmatch("%" + rest)
		if m == nil {
			return nil, fmt.Errorf("invalid format directive in %q", value)
		}
		if m[2] != "" {
			var err error
			next, err = strconv.Atoi(m[2])
			if err != nil {
				return nil, err
			}
		}
		signature = append(signature, fmt.Sprintf("%d:%s%s", next, m[1], m[3]))
		next++
		value = rest[len(m[0])-1:]
	}
	slices.Sort(signature)
	return signature, nil
}

func TestFormatSignatureRecognizesTranslatorMistakes(t *testing.T) {
	for _, plain := range []string{"100%", "50% off", "100%完成", "Already 100%!"} {
		if got, err := formatSignature(plain); len(got) != 0 || err != nil {
			t.Fatalf("plain copy %q incorrectly requires formatting: %v, %v", plain, got, err)
		}
	}
	want, err := formatSignature("%d:%02d %s")
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := formatSignature("%[3]s%[1]d:%02[2]d")
	if err != nil || !slices.Equal(ordered, want) {
		t.Fatalf("valid Japanese argument order: %v, %v", ordered, err)
	}
	for _, broken := range []string{"%[3]s%[1]d", "%[3]s%[1]d:%02[1]d", "%[3]d%[1]d:%02[2]d", "%[3]s%[1]d:%[2]d", "{{hour}}:%02d %s", "%[bad]s"} {
		got, err := formatSignature(broken)
		if err == nil && slices.Equal(got, want) {
			t.Errorf("accepted broken translation %q", broken)
		}
	}
}

// Exercise the argument types and counts supplied by the rendering call sites.
func TestFormattedMessagesAcceptCallerArguments(t *testing.T) {
	for language, translations := range catalogs {
		for _, sample := range []struct {
			id   MessageID
			args []any
		}{
			{FieldByteCount, []any{24, 48}},
			{FieldByteExceeded, []any{60, 48}},
			{PreviewSubmissions, []any{1}},
			{PreviewSavedName, []any{"Example Bank"}},
			{PreviewSavedNotes, []any{"Sample notes"}},
			{PreviewQuery, []any{"Bank"}},
			{DateToday, []any{"Feb 28, 2024"}},
			{TimeHourDisplay, []any{12, "AM"}},
			{TimeTwelveDisplay, []any{12, 0, "AM"}},
			{PreviewHarborBranch, []any{1}},
			{PreviewAccountResult, []any{"Cash account"}},
			{PreviewInstitutionResult, []any{"Harbor Bank"}},
			{PreviewDateResult, []any{"2024-02-29"}},
			{PreviewTimeResult, []any{"00:00"}},
			{InstitutionsEditName, []any{"Sample Bank"}},
			{InstitutionsNameLength, []any{120}},
			{HistoryCount, []any{6}},
			{SettingsTimezoneResolved, []any{"Asia/Tokyo"}},
			{SettingsTimezoneDiff, []any{"Asia/Singapore", "Asia/Tokyo"}},
		} {
			if text := fmt.Sprintf(translations[sample.id], sample.args...); strings.Contains(text, "%!") {
				t.Errorf("%s message %d has incompatible format parameters: %q", language, sample.id, text)
			}
		}
	}
}
