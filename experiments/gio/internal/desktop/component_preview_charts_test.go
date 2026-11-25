package desktop

import (
	"slices"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type previewChartSpec interface {
	Validate() []charts.Problem
	Rows() []charts.TextRow
}

func TestPreviewChartExamplesAreValidInEveryLanguage(t *testing.T) {
	builders := map[string]func(ui.View, charts.Selection) previewChartSpec{
		"line standard":      func(v ui.View, s charts.Selection) previewChartSpec { return previewWealthLine(v, s) },
		"line edge":          func(v ui.View, s charts.Selection) previewChartSpec { return previewReturnLine(v, s) },
		"bars standard":      func(v ui.View, s charts.Selection) previewChartSpec { return previewStandardBars(v, s) },
		"bars edge":          func(v ui.View, s charts.Selection) previewChartSpec { return previewEdgeBars(v, s) },
		"waterfall standard": func(v ui.View, s charts.Selection) previewChartSpec { return previewStandardWaterfall(v, s) },
		"waterfall edge":     func(v ui.View, s charts.Selection) previewChartSpec { return previewEdgeWaterfall(v, s) },
		"donut standard":     func(v ui.View, s charts.Selection) previewChartSpec { return previewStandardDonut(v, s) },
		"donut edge":         func(v ui.View, s charts.Selection) previewChartSpec { return previewEdgeDonut(v, s) },
		"sankey standard":    func(v ui.View, s charts.Selection) previewChartSpec { return previewStandardSankey(v, s) },
		"sankey edge":        func(v ui.View, s charts.Selection) previewChartSpec { return previewEdgeSankey(v, s) },
	}
	for _, supported := range slices.DeleteFunc(settings.AllLanguages(), func(l settings.Language) bool { return l == settings.LanguageSystem }) {
		view := ui.View{Language: i18n.Language(supported), DecimalSeparator: ".", GroupingSeparator: ","}
		for name, build := range builders {
			spec := build(view, charts.Selection{})
			if problems := spec.Validate(); len(problems) != 0 {
				t.Fatalf("%s %s: %v", supported, name, problems)
			}
			rows := spec.Rows()
			if len(rows) == 0 {
				t.Fatalf("%s %s: no rows", supported, name)
			}
			for _, row := range rows {
				// A person reads labels, never identifiers.
				if row.Label == "" || row.Label == string(row.ID) {
					t.Fatalf("%s %s: row %q has label %q", supported, name, row.ID, row.Label)
				}
			}
		}
	}
}

func TestPreviewChartExamplesCoverEveryChartTypeTwice(t *testing.T) {
	groups := newPreviewChartGroups()
	want := []i18n.MessageID{i18n.ChartTitleLine, i18n.ChartTitleBars, i18n.ChartTitleWaterfall, i18n.ChartTitleDonut, i18n.ChartTitleSankey}
	if len(groups) != len(want) {
		t.Fatalf("%d chart groups, want %d", len(groups), len(want))
	}
	for i, group := range groups {
		if group.title != want[i] || len(group.examples) != 2 {
			t.Fatalf("group %d = %v with %d examples", i, group.title, len(group.examples))
		}
		if group.examples[0].caption != i18n.ChartExampleStandard || group.examples[1].caption != i18n.ChartExampleEdge {
			t.Fatalf("group %d captions %v %v", i, group.examples[0].caption, group.examples[1].caption)
		}
	}
}

func TestPreviewChartsRenderAtNarrowAndWideWidthsInEveryLanguage(t *testing.T) {
	for _, language := range []settings.Language{settings.LanguageEnglish, settings.LanguageZhTW, settings.LanguageJa} {
		for _, width := range []int{390, 760, 1120} {
			h := newPreviewHarness(t)
			h.shell.prefs.saved.Language = language
			h.size.X = width
			h.frame()
			if h.preview.showCharts {
				t.Fatalf("%s width %d: charts open before the tab was chosen", language, width)
			}
			h.click(i18n.Language(language).Text(i18n.ChartPreviewTitle), false)
			if !h.preview.showCharts {
				t.Fatalf("%s width %d: the chart tab did not open", language, width)
			}
			for range 30 {
				h.preview.scroll.ScrollBy(5)
				h.frame()
			}
			if h.preview.scroll.Position.First == 0 && h.preview.scroll.Position.Offset == 0 {
				t.Fatalf("%s width %d: page did not scroll", language, width)
			}
		}
	}
}
