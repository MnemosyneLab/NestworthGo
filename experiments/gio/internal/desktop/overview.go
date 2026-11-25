package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type overviewReader interface {
	Overview(context.Context, domain.AccountFilter) (domain.OverviewResult, error)
}

type overviewRead struct {
	value  domain.OverviewResult
	readAt time.Time
	err    error
}

type overviewPage struct {
	ctx        context.Context
	reader     overviewReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	request    <-chan overviewRead
	cancel     context.CancelFunc

	current      *overviewRead
	loading      bool
	needsRefresh bool
	err          error
	reload       widget.Clickable
	list         widget.List
}

func newOverviewPage(ctx context.Context, reader overviewReader, clock func() time.Time, invalidate func()) *overviewPage {
	return &overviewPage{
		ctx: ctx, reader: reader, clock: clock, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
}

func (p *overviewPage) read() {
	if p.loading || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	result := make(chan overviewRead, 1)
	p.request, p.cancel, p.loading, p.err = result, cancel, true, nil
	p.workers.Go(func() {
		readAt := p.clock()
		value, err := p.reader.Overview(ctx, domain.AccountFilter{})
		if ctx.Err() != nil {
			return
		}
		result <- overviewRead{value: value, readAt: readAt, err: err}
		p.invalidate()
	})
}

func (p *overviewPage) retireRead() {
	if p.cancel != nil {
		p.cancel()
	}
	p.request, p.cancel, p.loading = nil, nil, false
}

func (p *overviewPage) refresh() {
	p.retireRead()
	p.needsRefresh = true
}

func (p *overviewPage) update() {
	select {
	case result := <-p.request:
		p.retireRead()
		p.err = result.err
		if p.err == nil {
			p.current = &result
			p.list.Position = restoredPosition(p.list.Position, p.rowCount())
		}
	default:
	}
	if !p.loading && (p.needsRefresh || p.current == nil && p.err == nil) {
		p.needsRefresh = false
		p.read()
	}
}

func (p *overviewPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	if p.reload.Clicked(gtx) {
		p.read()
	}
	return ui.Column{Gap: 12}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := i18n.HistoryRefresh
					if p.err != nil {
						label = i18n.HistoryRetry
					}
					if p.loading {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reload, view.Language.Text(label))(gtx)
				}),
			)
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return view.List(&p.list).Layout(gtx, p.rowCount(), func(gtx layout.Context, row int) layout.Dimensions {
				if row == 0 {
					if p.current.value.AccountCount == 0 {
						return view.Message(view.Language.Text(i18n.OverviewEmpty), view.Colors.SecondaryInk)(gtx)
					}
					return p.totals(gtx, view)
				}
				return layout.Inset{Top: 10, Left: 4, Right: 4}.Layout(gtx,
					view.Message(overviewIssue(view.Language, p.current.value.MissingInputs[row-1]), view.Colors.Ink))
			})
		}),
	)
}

func (p *overviewPage) rowCount() int {
	if p.current == nil {
		return 0
	}
	if p.current.value.AccountCount == 0 {
		return 1
	}
	return 1 + len(p.current.value.MissingInputs)
}

func (p *overviewPage) status(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	var rows []layout.FlexChild
	if p.current != nil {
		if p.current.value.Currency != "" {
			rows = append(rows, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewBaseCurrency), p.current.value.Currency), view.Colors.Ink)))
		}
		rows = append(rows, layout.Rigid(view.Message(valuationReadTime(view, p.current.readAt), view.Colors.SecondaryInk)))
	}
	switch {
	case p.loading:
		id := i18n.OverviewLoading
		if p.current != nil {
			id = i18n.OverviewRefreshing
		}
		rows = append(rows, layout.Rigid(view.Message(l.Text(id), view.Colors.SecondaryInk)))
	case p.err != nil:
		id := i18n.OverviewError
		if p.current != nil {
			id = i18n.OverviewStale
		}
		rows = append(rows, layout.Rigid(view.Message(l.Text(id), view.Colors.ErrorInk)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx, rows...)
}

func (p *overviewPage) totals(gtx layout.Context, view ui.View) layout.Dimensions {
	result := p.current.value
	values := result.DisplayValuation
	status := i18n.OverviewCurrentEstimate
	if !result.Complete || values.NetWorth.Quality.Availability != domain.ValuationFull {
		status = i18n.OverviewIncomplete
	}
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(18).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Rigid(view.Message(view.Language.Text(status), view.Colors.SecondaryInk)),
				layout.Rigid(overviewAmount(view, i18n.NetWorth, values.NetWorth, 32)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(12)}.Layout(gtx,
						layout.Flexed(1, overviewAmount(view, i18n.Assets, values.Assets, 20)),
						layout.Flexed(1, overviewAmount(view, i18n.Liabilities, values.Liabilities, 20)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return overviewEvidence(gtx, view, values.NetWorth.Quality)
				}),
			)
		})
	})
}

func overviewAmount(view ui.View, label i18n.MessageID, value domain.ValuationAmount, size unit.Sp) layout.Widget {
	caption := view.Language.Text(label)
	money := value.Total
	if money == nil && value.KnownSubtotal != nil {
		money = value.KnownSubtotal
		qualifier := i18n.OverviewKnownSubtotal
		if label == i18n.NetWorth {
			qualifier = i18n.OverviewKnownNet
		}
		caption += " · " + view.Language.Text(qualifier)
	}
	display := view.Language.Text(i18n.OverviewNoValue)
	if money != nil {
		display = valuationMoney(view, *money, label == i18n.Liabilities)
	}
	return func(gtx layout.Context) layout.Dimensions {
		label := material.Label(view.Theme, size, display)
		label.Color, label.Font.Weight = view.Colors.Ink, font.Medium
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
			layout.Rigid(view.Message(caption, view.Colors.SecondaryInk)),
			layout.Rigid(label.Layout),
		)
	}
}

func valuationReadTime(view ui.View, instant time.Time) string {
	stamp := format.LocalDateTime(instant.In(view.TimeLocation), settings.Settings{
		DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
	}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.OverviewReadAt), stamp, view.TimeLocation)
}

func valuationMoney(view ui.View, money domain.MoneyView, liability bool) string {
	amount := money.Amount
	// Debt magnitudes are unsigned in the service. Match Wails' visual sign.
	if liability && amount != "0" && !strings.HasPrefix(amount, "-") {
		amount = "-" + amount
	}
	return format.Money(amount, money.Currency.String(), settings.Settings{
		DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator,
	})
}

func overviewEvidence(gtx layout.Context, view ui.View, quality domain.ValuationQuality) layout.Dimensions {
	var rows []layout.FlexChild
	l := view.Language
	if quality.OldestMarketDate != nil {
		localDay := func(value string) string {
			if day, ok := ui.ParseCivilDay(value); ok {
				return ui.FormatDay(day, l, view.DateFormat)
			}
			return value
		}
		dates := localDay(*quality.OldestMarketDate)
		if quality.NewestMarketDate != nil && *quality.NewestMarketDate != *quality.OldestMarketDate {
			dates += " – " + localDay(*quality.NewestMarketDate)
		}
		rows = append(rows, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewQuoteDates), dates), view.Colors.SecondaryInk)))
	}
	if quality.CarriedCount > 0 || quality.StaleCount > 0 {
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.OverviewCarried), view.Colors.SecondaryInk)))
	}
	if slices.Contains(quality.Reasons, domain.ReasonHistoryUnverified) {
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.OverviewHistoryUnverified), view.Colors.SecondaryInk)))
	}
	if slices.Contains(quality.Reasons, domain.ReasonEvidenceUnavailable) {
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.OverviewEvidenceUnavailable), view.Colors.SecondaryInk)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, rows...)
}

var overviewMissing = map[domain.MissingInputKind]i18n.MessageID{
	domain.MissingInstrumentPrice: i18n.OverviewMissingPrice,
	domain.MissingInstrument:      i18n.OverviewMissingInstrument,
	domain.MissingFXRate:          i18n.OverviewMissingFX,
	domain.MissingAccountValue:    i18n.OverviewMissingAccount,
	domain.MissingHistoryCoverage: i18n.OverviewHistoryUnverified,
	domain.MissingHoldingAmount:   i18n.OverviewEvidenceUnavailable,
}

func overviewIssue(language i18n.Language, missing domain.MissingInputView) string {
	message, ok := overviewMissing[missing.Kind]
	if !ok {
		message = i18n.OverviewEvidenceUnavailable
	}
	parts := []string{language.Text(message)}
	if missing.Kind == domain.MissingFXRate && missing.BaseCurrency != "" && missing.QuoteCurrency != "" {
		parts = append(parts, missing.BaseCurrency.String()+"/"+missing.QuoteCurrency.String())
	}
	if name := domain.InstrumentDisplayLabel(missing.InstrumentName, missing.InstrumentSymbol, missing.InstrumentShortName); name != "" {
		parts = append(parts, name)
	}
	if missing.AccountName != "" {
		parts = append(parts, missing.AccountName)
	}
	return strings.Join(parts, " · ")
}
