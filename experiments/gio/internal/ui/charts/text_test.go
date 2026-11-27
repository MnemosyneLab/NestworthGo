package charts

import (
	"testing"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func TestDonutCenterCompanionAmountsShareSizeAndWeight(t *testing.T) {
	view := ui.View{Theme: ui.NewTheme()}
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}}
	totals := []string{"CN¥2,667,614.06", "-CN¥29,713.70"}
	long := textBlock{style: ui.TextMoneyMD, content: totals[0]}
	short := textBlock{style: ui.TextMoneyMD, content: totals[1]}
	long.maxWidth = short.natural(gtx, view)
	short.maxWidth = long.maxWidth
	longStyle := donutCenterStyle(gtx, view, long, totals)
	shortStyle := donutCenterStyle(gtx, view, short, totals)
	if longStyle != shortStyle || longStyle.Size >= ui.TextMoneyMD.Size ||
		longStyle.Weight != ui.TextMoneyMD.Weight || !longStyle.Serif {
		t.Fatalf("companion styles = %+v and %+v, want a shared smaller serif semibold style", longStyle, shortStyle)
	}
	long.style = longStyle
	if width := long.natural(gtx, view); width > long.maxWidth {
		t.Fatalf("long total still needs %d pixels, want at most %d", width, long.maxWidth)
	}
}

func TestChartFiguresUsePageTypefaces(t *testing.T) {
	view := ui.View{Theme: ui.NewTheme()}
	sans := view.Theme.Face
	serif := view.TextLabelStyle(ui.TextHero, "US$1,234.50", view.Theme.Palette.Fg).Font.Typeface
	if serif == sans {
		t.Fatal("the page's hero amount uses the body typeface")
	}
	for _, test := range []struct {
		name, value string
		style       ui.TextStyle
		face        font.Typeface
	}{
		{"summary amount", "US$1,234.50", ui.TextMoneyMD, serif},
		{"large amount", "-US$1,234.50", ui.TextMoneyLG, serif},
		{"rate", "-12.34%", ui.TextMetricMD, sans},
		{"large rate", "+12.34%", ui.TextMetricLG, sans},
		{"legend amount", "US$1,234.50", ui.TextLabel, sans},
	} {
		t.Run(test.name, func(t *testing.T) {
			label := (textBlock{style: test.style, content: test.value}).label(view)
			if label.Font.Typeface != test.face || label.TextSize != test.style.Size {
				t.Fatalf("got %q at %v sp, want %q at %v sp", label.Font.Typeface, label.TextSize, test.face, test.style.Size)
			}
		})
	}
}
