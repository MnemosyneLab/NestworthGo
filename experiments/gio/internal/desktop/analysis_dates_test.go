package desktop

import "testing"

func TestAnalysisDateShortcutsMatchCivilRanges(t *testing.T) {
	week, month := 1, 2
	cases := []struct {
		name string
		got  analysisDateWindow
		want analysisDateWindow
	}{
		{"closed week", analysisPresetRange(week, "2026-10-07", "2026-01-01"), analysisDateWindow{"2026-10-01", "2026-10-07"}},
		{"month ending after leap day", analysisPresetRange(month, "2024-03-31", "2024-01-01"), analysisDateWindow{"2024-03-01", "2024-03-31"}},
		{"bounded to origin", analysisPresetRange(week, "2026-10-07", "2026-10-04"), analysisDateWindow{"2026-10-04", "2026-10-07"}},
		{"previous month", analysisShiftRange(analysisDateWindow{"2024-03-01", "2024-03-31"}, -1, month), analysisDateWindow{"2024-02-01", "2024-02-29"}},
		{"next month", analysisShiftRange(analysisDateWindow{"2024-02-01", "2024-02-29"}, 1, month), analysisDateWindow{"2024-03-01", "2024-03-31"}},
		{"previous custom range", analysisShiftRange(analysisDateWindow{"2026-10-03", "2026-10-07"}, -1, -1), analysisDateWindow{"2026-09-28", "2026-10-02"}},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s produced %+v, want %+v", tc.name, tc.got, tc.want)
		}
	}
}
