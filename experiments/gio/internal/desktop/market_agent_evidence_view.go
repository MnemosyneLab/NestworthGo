package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *marketDataPage) agentEvidenceAction(gtx layout.Context, view ui.View, target marketQuoteHistoryTarget, from *widget.Clickable, ready bool) layout.Widget {
	if ready && from.Clicked(gtx) {
		p.openAgentEvidence(target, from)
		gtx.Execute(op.InvalidateCmd{})
	}
	return returnMonthButton(view, from, i18n.AgentEvidenceOpen, nil, ready)
}

func (p *marketDataPage) marketAgentAction(gtx layout.Context, view ui.View, key string, target marketQuoteHistoryTarget) []layout.Widget {
	if p.agentReader == nil {
		return nil
	}
	if p.agentActions == nil {
		p.agentActions = make(map[string]*widget.Clickable)
	}
	return []layout.Widget{p.agentEvidenceAction(gtx, view, target,
		portfolioButton(p.agentActions, key), p.request == nil && p.err == nil)}
}

func (p *marketDataPage) layoutAgentEvidence(gtx layout.Context, view ui.View) (event.Tag, bool) {
	d, l := p.agentEvidence, view.Language
	saved := false
	select {
	case response := <-d.request:
		d.stop()
		d.request, d.stop, d.err = nil, nil, response.err
		if response.err == nil {
			d.result = &response.value
			d.filter()
		}
	case response := <-d.saving:
		d.saving, d.saveErr = nil, response.err
		if response.err == nil {
			d.receipt = &response.value
			saved = true
			p.readAgentEvidence()
		}
	default:
	}
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)
	if d.overlay.Trigger == nil && d.saving == nil && d.sheet.dismissed(gtx, &d.picker.Trigger, &d.all, &d.dated) {
		if d.stop != nil {
			d.stop()
		}
		p.agentEvidence = nil
		gtx.Execute(op.InvalidateCmd{})
		return d.from, saved
	}
	if d.result != nil {
		for i, button := range []*widget.Clickable{&d.all, &d.dated} {
			if button.Clicked(gtx) {
				d.selectScope(i == 1)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	title := fmt.Sprintf(l.Text(i18n.AgentEvidenceTitle), d.name())
	var footer layout.Widget
	switch {
	case d.err != nil:
		if d.sheet.action.Clicked(gtx) {
			p.readAgentEvidence()
			gtx.Execute(op.InvalidateCmd{})
		}
		footer = view.PrimaryButton(&d.sheet.action, l.Text(i18n.HistoryRetry))
	case d.result != nil && d.result.canDeclare:
		_, dateOK := ui.ParseCivilDay(d.date)
		ready := dateOK && d.date <= d.result.today && d.request == nil && d.saving == nil &&
			(d.receipt == nil || d.date != d.attemptDate)
		if ready && d.sheet.action.Clicked(gtx) {
			p.saveAgentClosedDate()
			ready = false
			gtx.Execute(op.InvalidateCmd{})
		}
		label := i18n.AgentEvidenceDeclare
		if d.saving != nil {
			label = i18n.SettingsPending
		}
		footer = func(gtx layout.Context) layout.Dimensions {
			if !ready {
				gtx = gtx.Disabled()
			}
			return view.PrimaryButton(&d.sheet.action, l.Text(label))(gtx)
		}
	}
	d.sheet.layoutPanel(gtx, view, title, func(gtx layout.Context, view ui.View) layout.Dimensions {
		if d.err != nil || d.result == nil {
			status := i18n.MarketLoading
			if d.err != nil {
				status = i18n.AgentEvidenceReadFailed
			}
			return view.Message(l.Text(status), view.Colors.SecondaryInk)(gtx)
		}
		bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
		view = view.WithScrollAnchors(&d.sheet.list)
		d.overlay.ScrollOutsidePanel(gtx, &d.sheet.list, bounds)
		dims := p.layoutAgentEvidenceList(gtx, view, d, title)
		d.overlay.OffsetScrollAnchor(&d.sheet.list, bounds)
		return dims
	}, footer, d.saving == nil)
	d.overlay.Layout(gtx, view)
	return nil, saved
}

func (p *marketDataPage) layoutAgentEvidenceList(gtx layout.Context, view ui.View, d *marketAgentEvidence, title string) layout.Dimensions {
	l := view.Language
	return analysisResultList(gtx, view, &d.sheet.list, title, 2+len(d.findings)+len(d.rows), func(gtx layout.Context, i int) layout.Dimensions {
		switch {
		case i == 0:
			view.Origin = view.Origin.Add(image.Pt(gtx.Dp(10), gtx.Dp(5)))
			return d.layoutControls(gtx, view)
		case i <= len(d.findings):
			issue := d.findings[i-1]
			label := healthKinds[issue.Kind]
			if label == i18n.NoMessage {
				label = i18n.HealthFinding
			}
			return healthRepairLines(gtx, view, []string{l.Text(label), healthIssueEvidence(view, issue)})
		case i == len(d.findings)+1:
			if len(d.rows) == 0 {
				return view.Message(l.Text(i18n.AgentEvidenceEmpty), view.Colors.SecondaryInk)(gtx)
			}
			return view.Body(ui.TextHeading, l.Text(i18n.AgentEvidenceRecords), view.Colors.Ink)(gtx)
		default:
			return healthRepairLines(gtx, view, agentEvidenceLines(view, d.rows[i-len(d.findings)-2]))
		}
	})
}

func (d *marketAgentEvidence) layoutControls(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	width, height, gap := gtx.Constraints.Max.X, 0, gtx.Dp(8)
	place := func(body func(layout.Context, ui.View) layout.Dimensions) {
		dims := ui.Place(gtx, view, image.Pt(0, height), width, body)
		height += dims.Size.Y + gap
	}
	message := func(text string, messageID i18n.MessageID) {
		place(func(gtx layout.Context, view ui.View) layout.Dimensions {
			ink := view.Colors.SecondaryInk
			if messageID == i18n.AgentEvidenceSaveFailed {
				ink = view.Colors.ErrorInk
			}
			return view.Message(text, ink)(gtx)
		})
	}
	if window := marketQuoteFindingWindow(d.target.finding); window.from != "" {
		place(func(gtx layout.Context, view ui.View) layout.Dimensions {
			return marketActionRow(view, []layout.Widget{
				analysisToggle(view, &d.all, l.Text(i18n.QuoteHistoryShowAll), !d.scope),
				analysisToggle(view, &d.dated, healthDateRange(view, window.from, window.to), d.scope),
			})(gtx)
		})
	}
	if d.result.canDeclare {
		message(l.Text(i18n.AgentEvidenceClosureHelp), i18n.NoMessage)
		place(func(gtx layout.Context, view ui.View) layout.Dimensions {
			week := time.Monday
			if view.WeekStart == settings.WeekStartSunday {
				week = time.Sunday
			}
			dateOptions := ui.DateOptions{Today: d.result.today, Max: d.result.today, Format: view.DateFormat, WeekStart: week, Clearable: true}
			field := ui.PickerField{Label: i18n.AgentEvidenceClosedDate, Origin: view.Origin, Disabled: d.saving != nil}
			d.picker.Layout(gtx, view, field, dateOptions, d.date, func(day string) { d.date, d.saveErr = day, nil })
			return manualPickerSize(gtx)
		})
	}
	if d.saveErr != nil {
		message(l.Text(i18n.AgentEvidenceSaveFailed), i18n.AgentEvidenceSaveFailed)
	}
	if d.receipt != nil {
		status := i18n.AgentEvidenceSaved
		if d.receipt.SnapshotStatus == "pending" {
			status = i18n.AgentEvidenceSavedPending
		}
		message(fmt.Sprintf(l.Text(status), wealthDay(view, d.attemptDate)), i18n.NoMessage)
	}
	status := i18n.AgentEvidenceFindings
	if len(d.findings) == 0 {
		status = i18n.AgentEvidenceNoFindings
	}
	if d.request != nil {
		status = i18n.MarketLoading
	}
	place(func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Body(ui.TextLabel, l.Text(status), view.Colors.Ink)(gtx)
	})
	return layout.Dimensions{Size: image.Pt(width, height-gap)}
}

func agentEvidenceLines(view ui.View, row marketAgentEvidenceRow) []string {
	r, l := row.record, view.Language
	label := i18n.AgentEvidenceObservation
	if r.Operation == domain.AgentQuoteCorrect {
		label = i18n.AgentEvidenceCorrection
	}
	switch {
	case r.Operation == domain.AgentQuoteRetract:
		label = i18n.AgentEvidenceWithdrawal
	case r.Closure != nil && r.Operation != domain.AgentQuoteCorrect:
		label = i18n.AgentEvidenceClosure
	}
	title := l.Text(label)
	if row.day != "" {
		title += " · " + wealthDay(view, row.day)
	}
	if row.inactive {
		title += " · " + l.Text(i18n.AgentEvidenceSuperseded)
	}
	lines := []string{title}
	if q := r.InstrumentQuote; q != nil {
		point := domain.QuoteSeriesPoint{Value: q.UnitPrice.Canonical(), Currency: q.Currency, SourceKind: q.SourceKind, SourceKey: q.SourceKey,
			ObservationKind: q.ObservationKind, EffectiveDate: q.EffectiveDate, PriceBasis: q.PriceBasis, TimestampBasis: q.TimestampBasis, QuotedAt: q.QuotedAt, Delayed: q.Delayed}
		lines = append(lines, quoteHistoryAmount(view, point, marketFXPair{}, nil))
		lines = append(lines, quoteHistoryEvidence(view, point)...)
	}
	if q := r.FXQuote; q != nil {
		point := domain.QuoteSeriesPoint{Value: q.Rate.Canonical(), SourceKind: q.SourceKind, SourceKey: q.SourceKey, ObservationKind: q.ObservationKind,
			EffectiveDate: q.EffectiveDate, TimestampBasis: q.TimestampBasis, QuotedAt: q.QuotedAt, Delayed: q.Delayed}
		lines = append(lines, quoteHistoryAmount(view, point, marketFXPair{q.BaseCurrency, q.QuoteCurrency}, nil))
		lines = append(lines, quoteHistoryEvidence(view, point)...)
	}
	if r.Closure != nil {
		lines = append(lines, l.Text(i18n.AgentEvidenceNoPrice))
	}
	if r.SourceTitle != "" {
		lines = append(lines, fmt.Sprintf(l.Text(i18n.AgentEvidenceSource), r.SourceTitle))
	}
	if r.SourceURL != "" {
		lines = append(lines, r.SourceURL)
	}
	return lines
}
