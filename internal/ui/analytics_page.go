package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

func NewAnalyticsPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	if c.service == nil {
		return NewComingSoonPage(c, "page.analyticsTitle", "page.analyticsDescription", "storage")
	}
	selected := domain.TrendRange(c.trendRange)
	if selected == "" {
		selected = domain.Trend30Days
	}
	trend, err := c.service.NetWorthTrend(context.Background(), selected)
	if err != nil {
		return errorPanel(c, t.T("page.analyticsTitle"), err)
	}
	rangeSelect := widget.NewSelect([]string{t.T("analytics.range30"), t.T("analytics.range1year"), t.T("analytics.rangeAll")}, nil)
	selectedLabel := t.T("analytics.range30")
	if selected == domain.TrendOneYear {
		selectedLabel = t.T("analytics.range1year")
	} else if selected == domain.TrendAllTime {
		selectedLabel = t.T("analytics.rangeAll")
	}
	rangeSelect.SetSelected(selectedLabel)
	rows := []fyne.CanvasObject{}
	if feedback := snapshotFeedback(c); feedback != nil {
		rows = append(rows, feedback)
	}
	rows = append(rows, widget.NewLabelWithStyle(t.T("analytics.trend"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	for _, point := range trend.Points {
		value := t.T("portfolio.unavailable")
		if point.Value != nil {
			value = format.Money(point.Value.CanonicalAmount(), point.Value.Currency().String(), c.preference)
		}
		status := t.T("analytics.statusPartial")
		if point.Complete {
			status = t.T("analytics.statusComplete")
		}
		rows = append(rows, widget.NewLabel(fmt.Sprintf("%s    %s    %s", point.LocalDate, value, status)))
	}
	if len(trend.Points) == 0 {
		rows = append(rows, widget.NewLabel(t.T("analytics.empty")))
	}
	rangeSelect.OnChanged = func(value string) {
		switch value {
		case t.T("analytics.range1year"):
			c.trendRange = string(domain.TrendOneYear)
		case t.T("analytics.rangeAll"):
			c.trendRange = string(domain.TrendAllTime)
		default:
			c.trendRange = string(domain.Trend30Days)
		}
		c.RefreshContent()
	}
	return surface(container.NewVBox(container.NewHBox(widget.NewLabel(t.T("analytics.range")), rangeSelect), container.NewVBox(rows...)), fyne.NewSize(640, 520))
}
