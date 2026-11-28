package desktop

import (
	"fmt"
	"image"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *historyPage) layoutInvestmentAmendment(gtx layout.Context, view ui.View, fix, busy bool, action, prepare, reviewTitle, confirm i18n.MessageID) layout.Dimensions {
	gtx, view = view.Measure(gtx, 640)
	detail, l := p.reads.detail, view.Language
	record, form := detail.record, detail.amendment
	investment := form.investment
	view = view.WithScrollAnchors(&form.scroll)
	origin := view.Origin
	title := action
	if form.review != nil {
		title = reviewTitle
	}
	account := record.Accounts[investment.Account]
	if account == "" {
		account = l.Text(i18n.AccountReferenceUnavailable)
	}
	instrument := record.Instruments[investment.Instrument]
	if instrument == "" {
		instrument = l.Text(i18n.HistoryInstrumentUnavailable)
	}
	rows := []layout.Widget{
		tradeTitle(view, title),
		view.Message(account+" · "+instrument, view.Colors.Ink),
		view.Message(l.Text(i18n.HistoryOriginalMovement), view.Colors.SecondaryInk),
		view.Message(tradeTimestamp(view, record.Activity.EffectiveAt, record.Location), view.Colors.SecondaryInk),
	}
	rows = append(rows, investmentAmendmentFacts(view, record, record.Activity, record.Activity.Effects, history.InvestmentReplacementSources{})...)
	description := i18n.HistoryReverseDescription
	if fix {
		description = i18n.HistoryCorrectDescription
	}
	rows = append(rows, view.Message(l.Text(description), view.Colors.SecondaryInk))
	if form.review != nil {
		preview := form.review.Preview
		heading := i18n.HistoryReversalMovement
		if fix {
			heading = i18n.HistoryReplacementMovement
		}
		rows = append(rows, view.Message(l.Text(heading), view.Colors.SecondaryInk))
		if !fix {
			rows = append(rows, view.Message(tradeTimestamp(view, preview.Activity.EffectiveAt, record.Location), view.Colors.SecondaryInk))
		}
		rows = append(rows, investmentAmendmentFacts(view, record, preview.Activity, preview.Effects, investment.sources)...)
		rows = append(rows, investmentAmendmentResults(view, preview, fix)...)
	} else if fix {
		rows = append(rows, p.replacementFields(view, form, &origin, busy)...)
	}
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, row)
		}))
	}
	panel := []ui.ColumnChild{
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
			if view.Overlay != nil {
				view.Overlay.ScrollOutsidePanel(gtx, &form.scroll, bounds)
			}
			dims := view.List(&form.scroll).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				origin = view.Origin
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
			})
			if view.Overlay != nil {
				view.Overlay.OffsetScrollAnchor(&form.scroll, bounds)
			}
			return dims
		}),
	}
	problem := form.problem
	if form.uncertain {
		problem = i18n.ContributionUncertain
	}
	if problem != i18n.NoMessage {
		panel = append(panel, ui.Rigid(view.Message(l.Text(problem), view.Colors.ErrorInk)))
	}
	panel = append(panel, ui.Rigid(amendmentActions(view, form, fix, prepare, confirm)))
	return (ui.Column{Gap: 8}).Layout(gtx, panel...)
}

func investmentAmendmentFacts(view ui.View, record *history.Detail, activity domain.Activity, effects []domain.ActivityEffect, sources history.InvestmentReplacementSources) []layout.Widget {
	l := view.Language
	account := func(id *domain.AccountID) string {
		if id != nil {
			if name := record.Accounts[*id]; name != "" {
				return name
			}
			for _, account := range sources.Accounts {
				if account.ID == *id {
					return account.Name
				}
			}
		}
		return l.Text(i18n.AccountReferenceUnavailable)
	}
	instrument := func(id domain.InstrumentID) string {
		if name := record.Instruments[id]; name != "" {
			return name
		}
		for _, holding := range sources.Holdings {
			if holding.Instrument.ID == id {
				return holding.Instrument.DisplayLabel()
			}
		}
		return l.Text(i18n.HistoryInstrumentUnavailable)
	}
	var rows []layout.Widget
	for _, effect := range effects {
		caption := effectCaption(view, effect, false, false, false, 0, account, instrument)
		if caption == "" {
			continue
		}
		if effect.Role == domain.EffectRoleFee {
			caption += " · " + l.Text(i18n.HistoryFee)
		}
		rows = append(rows, view.Message(caption, view.Colors.Ink))
	}
	if trade := activity.TradeDetail; trade != nil {
		price := portfolioDecimal(view, trade.UnitPrice.Canonical()) + " " + trade.Gross.Currency().String()
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.BuyExecutionPrice), price), view.Colors.Ink))
	}
	if activity.Note != nil && *activity.Note != "" {
		rows = append(rows, view.Message(*activity.Note, view.Colors.SecondaryInk))
	}
	return rows
}

func investmentAmendmentResults(view ui.View, preview domain.ChangePreview, fix bool) []layout.Widget {
	l := view.Language
	fact := func(label i18n.MessageID, values ...any) layout.Widget {
		return view.Message(fmt.Sprintf(l.Text(label), values...), view.Colors.Ink)
	}
	balanceLabel, quantityLabel := i18n.HistoryBalanceAfterReversal, i18n.CustodyAmendmentAfterReversal
	if fix {
		balanceLabel, quantityLabel = i18n.HistoryBalanceAfterCorrection, i18n.QuantityAdjustAfter
	}
	var rows []layout.Widget
	if len(preview.Review) != 0 {
		for _, endpoint := range preview.Review {
			switch endpoint.Target {
			case domain.EffectTargetAccountCash, domain.EffectTargetAccountValue:
				currency := " " + endpoint.Currency.String()
				rows = append(rows, view.Message(endpoint.AccountName, view.Colors.SecondaryInk))
				rows = append(rows,
					fact(i18n.ContributionBefore, endpoint.Before+currency),
					fact(i18n.HistoryChangeDelta, endpoint.Delta+currency),
					fact(balanceLabel, endpoint.After+currency),
				)
			case domain.EffectTargetHoldingQuantity:
				if endpoint.AccountName != "" && endpoint.Instrument != "" {
					rows = append(rows, view.Message(endpoint.AccountName+" · "+endpoint.Instrument, view.Colors.SecondaryInk))
				}
				rows = append(rows, fact(i18n.CustodyAmendmentBeforeAfter, portfolioDecimal(view, endpoint.Before), portfolioDecimal(view, endpoint.After)))
			}
		}
		return rows
	}
	for _, endpoint := range preview.Resulting {
		switch endpoint.Target {
		case domain.EffectTargetAccountCash:
			rows = append(rows, fact(balanceLabel, endpoint.Amount+" "+endpoint.Currency.String()))
		case domain.EffectTargetHoldingQuantity:
			rows = append(rows, fact(quantityLabel, portfolioDecimal(view, endpoint.Quantity)))
		}
	}
	return rows
}
