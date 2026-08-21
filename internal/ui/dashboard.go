package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type dashboardDemo struct {
	netWorth     string
	assets       string
	liabilities  string
	liquidAssets string
	accounts     string
	allocation   []allocationDemo
	trend        []float32
}

type allocationDemo struct {
	label   string
	percent int
	color   color.Color
}

func defaultDashboardDemo(accent settings.Accent) dashboardDemo {
	palette := PaletteFor(accent)
	return dashboardDemo{
		netWorth:     "1284560.00",
		assets:       "1612400.00",
		liabilities:  "327840.00",
		liquidAssets: "856200.00",
		accounts:     "8",
		allocation: []allocationDemo{
			{label: "dashboard.cash", percent: 38, color: palette.Primary},
			{label: "dashboard.investments", percent: 44, color: palette.Selection},
			{label: "dashboard.property", percent: 18, color: palette.Soft},
		},
		trend: []float32{38, 42, 41, 47, 50, 49, 57, 61, 66, 64, 71, 76},
	}
}

// NewDashboard builds the visual overview using explicitly marked demo data.
// No value in this function is an authoritative financial total.
func NewDashboard(controller *Controller) fyne.CanvasObject {
	t := controller.translator
	demo := defaultDashboardDemo(controller.preference.Accent)
	preference := controller.preference
	palette := PaletteFor(preference.Accent)

	eyebrow := canvas.NewText(t.T("dashboard.eyebrow"), palette.Primary)
	eyebrow.TextSize = 11
	eyebrow.TextStyle = fyne.TextStyle{Bold: true}
	title := widget.NewLabelWithStyle(t.T("dashboard.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel(t.T("dashboard.subtitle"))
	subtitle.Wrapping = fyne.TextWrapWord
	heroCopy := container.NewVBox(eyebrow, title, subtitle)
	preview := badge(t.T("common.preview"), palette.Soft, palette.Primary)
	hero := surface(container.NewBorder(nil, nil, nil, preview, heroCopy), fyne.NewSize(1, 132))

	netWorth := format.Money(demo.netWorth, preference.Currency, preference)
	date := formatLocalTime(preference, controller.translator.Language())
	netWorthText := canvas.NewText(netWorth, palette.Primary)
	netWorthText.TextSize = 34
	netWorthText.TextStyle = fyne.TextStyle{Bold: true}
	netWorthCopy := container.NewVBox(
		mutedLabel(t.T("dashboard.currentTotal")),
		netWorthText,
		mutedLabel(date),
	)
	changeText := canvas.NewText(t.T("dashboard.changeValue"), palette.Primary)
	changeText.TextSize = 20
	changeText.TextStyle = fyne.TextStyle{Bold: true}
	change := container.NewVBox(
		badge(t.T("dashboard.change"), palette.Soft, palette.Primary),
		changeText,
	)
	netWorthCard := surface(container.NewBorder(nil, nil, nil, change, netWorthCopy), fyne.NewSize(1, 154))

	metrics := container.NewGridWithColumns(
		3,
		metricCard(t.T("dashboard.assets"), format.Money(demo.assets, preference.Currency, preference), fmt.Sprintf(t.T("dashboard.accountCount"), 5), palette.Primary),
		metricCard(t.T("dashboard.liabilities"), format.Money(demo.liabilities, preference.Currency, preference), fmt.Sprintf(t.T("dashboard.accountCount"), 2), color.NRGBA{R: 184, G: 66, B: 99, A: 255}),
		metricCard(t.T("dashboard.liquidAssets"), format.Money(demo.liquidAssets, preference.Currency, preference), demo.accounts+" "+t.T("dashboard.accountsTracked"), palette.Primary),
	)

	trend := surface(container.NewVBox(
		sectionTitle(t.T("dashboard.netWorthTrend"), t.T("dashboard.previewTrend")),
		trendPlot(demo.trend, palette.Primary),
	), fyne.NewSize(1, 280))

	allocationRows := make([]fyne.CanvasObject, 0, len(demo.allocation))
	for _, item := range demo.allocation {
		allocationRows = append(allocationRows, allocationRow(t.T(item.label), item.percent, item.color, palette.Soft))
	}
	allocation := surface(container.NewVBox(
		sectionTitle(t.T("dashboard.allocation"), t.T("common.previewNotice")),
		layout.NewSpacer(),
		allocationRows[0],
		allocationRows[1],
		allocationRows[2],
	), fyne.NewSize(1, 280))

	secondary := container.NewGridWithColumns(2, trend, allocation)

	recent := surface(container.NewVBox(
		sectionTitle(t.T("dashboard.recentActivity"), t.T("dashboard.activityDescription")),
		layout.NewSpacer(),
		container.NewBorder(nil, nil, nil, badge(t.T("common.previewData"), palette.Soft, palette.Primary), widget.NewLabel(t.T("dashboard.noActivity"))),
	), fyne.NewSize(1, 132))

	accountsReview := surface(container.NewVBox(
		sectionTitle(t.T("dashboard.accountsToReview"), t.T("dashboard.reviewDescription")),
		layout.NewSpacer(),
		keyValueRow("•", t.T("dashboard.reviewItemOne")),
		keyValueRow("•", t.T("dashboard.reviewItemTwo")),
	), fyne.NewSize(1, 132))

	footer := container.NewGridWithColumns(2, recent, accountsReview)

	return container.NewVBox(hero, netWorthCard, metrics, secondary, footer)
}

func trendPlot(values []float32, accent color.Color) fyne.CanvasObject {
	const (
		width  = float32(500)
		height = float32(170)
		left   = float32(8)
		right  = float32(492)
		top    = float32(10)
		bottom = float32(156)
	)
	background := canvas.NewRectangle(color.Transparent)
	background.SetMinSize(fyne.NewSize(width, height))
	objects := []fyne.CanvasObject{background}
	gridColor := currentColor(fyneTheme.ColorNameSeparator)
	for index := 0; index < 4; index++ {
		y := top + float32(index)*(bottom-top)/3
		line := canvas.NewLine(gridColor)
		line.Position1 = fyne.NewPos(left, y)
		line.Position2 = fyne.NewPos(right, y)
		line.StrokeWidth = 1
		objects = append(objects, line)
	}
	if len(values) > 1 {
		minValue, maxValue := values[0], values[0]
		for _, value := range values[1:] {
			if value < minValue {
				minValue = value
			}
			if value > maxValue {
				maxValue = value
			}
		}
		span := maxValue - minValue
		if span == 0 {
			span = 1
		}
		for index := 1; index < len(values); index++ {
			x1 := left + float32(index-1)*(right-left)/float32(len(values)-1)
			x2 := left + float32(index)*(right-left)/float32(len(values)-1)
			y1 := bottom - (values[index-1]-minValue)/span*(bottom-top)
			y2 := bottom - (values[index]-minValue)/span*(bottom-top)
			line := canvas.NewLine(accent)
			line.Position1 = fyne.NewPos(x1, y1)
			line.Position2 = fyne.NewPos(x2, y2)
			line.StrokeWidth = 3
			objects = append(objects, line)
		}
	}
	return container.NewWithoutLayout(objects...)
}

func allocationRow(label string, percent int, accent, backgroundColor color.Color) fyne.CanvasObject {
	name := widget.NewLabel(label)
	value := widget.NewLabelWithStyle(formatPercent(percent), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	bar := allocationBar(percent, accent, backgroundColor)
	return container.NewVBox(container.NewBorder(nil, nil, name, value), bar)
}

func allocationBar(percent int, accent, backgroundColor color.Color) fyne.CanvasObject {
	const width = float32(360)
	const height = float32(10)
	background := canvas.NewRectangle(backgroundColor)
	background.Resize(fyne.NewSize(width, height))
	foreground := canvas.NewRectangle(accent)
	foreground.Resize(fyne.NewSize(width*float32(percent)/100, height))
	return container.NewWithoutLayout(background, foreground)
}

func formatPercent(value int) string {
	return fmt.Sprintf("%d%%", value)
}
