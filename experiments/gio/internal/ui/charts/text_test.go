package charts

import (
	"testing"

	"gioui.org/font"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

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
