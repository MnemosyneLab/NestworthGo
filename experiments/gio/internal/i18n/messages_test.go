package i18n

import (
	"fmt"
	"strings"
	"testing"
)

func TestCatalogCoverage(t *testing.T) {
	for language, translations := range catalogs {
		for id := LanguageName; id < messageCount; id++ {
			if strings.TrimSpace(translations[id]) == "" {
				t.Errorf("%s is missing message %d", language, id)
			}
		}
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
			{ColorThemeLabel, []any{"Indigo"}},
			{HistoryCount, []any{6}},
		} {
			if text := fmt.Sprintf(translations[sample.id], sample.args...); strings.Contains(text, "%!") {
				t.Errorf("%s message %d has incompatible format parameters: %q", language, sample.id, text)
			}
		}
	}
	if got := Language("unknown").Text(NavComponentPreview); got != English.Text(NavComponentPreview) {
		t.Errorf("unknown language gave %q instead of the English preview label", got)
	}
}
