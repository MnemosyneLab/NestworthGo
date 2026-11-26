package desktop

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type contributionDirection int

const (
	contributionLargest contributionDirection = iota
	contributionPositive
	contributionNegative
)

var contributionDirections = [...]i18n.MessageID{
	i18n.RankingLargest, i18n.RankingPositiveOnly, i18n.RankingNegativeOnly,
}

type contributionRanking struct {
	direction contributionDirection
	limit     int
	tabs      [3]widget.Clickable
	limits    [3]widget.Clickable
	search    ui.SearchField
	chart     charts.BarChart
}

func rankedContributionRows(rows []application.ContributionRow, direction contributionDirection, limit int) []application.ContributionRow {
	ranked := make([]application.ContributionRow, 0, len(rows))
	for _, row := range rows {
		if row.Amount == nil {
			continue
		}
		sign := row.Amount.Amount().Sign()
		if direction == contributionPositive && sign <= 0 || direction == contributionNegative && sign >= 0 {
			continue
		}
		ranked = append(ranked, row)
	}
	slices.SortFunc(ranked, func(a, b application.ContributionRow) int {
		if n := b.Amount.Amount().Abs().Cmp(a.Amount.Amount().Abs()); n != 0 {
			return n
		}
		return cmp.Compare(a.Key, b.Key)
	})
	if limit > 0 && limit < len(ranked) {
		ranked = ranked[:limit]
	}
	return ranked
}

func (p *returnAnalysisPage) contributionRows(view ui.View, data application.ContributionResult) []ui.ColumnChild {
	l := view.Language
	if !data.Available {
		return []ui.ColumnChild{ui.Rigid(emptyState(view, l.Text(i18n.RankingUnavailable)))}
	}
	basis := returnValuationContext{valuation: p.current.query.Valuation, forced: data.ValuationForced}
	if data.Summary.Total != nil {
		basis.currency = data.Summary.Total.Currency()
	} else {
		for _, row := range data.Rows {
			if row.Amount != nil {
				basis.currency = row.Amount.Currency()
				break
			}
		}
	}
	rows := []ui.ColumnChild{ui.Rigid(returnValuationLabel(view, i18n.RankingValuation, basis)), ui.Rigid(contributionSummary(view, data.Summary))}
	rows = append(rows,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutRankedContributions(gtx, view, data) }),
		ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.RankingDetails), view.Colors.Ink)),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			sortControl := p.contributionSortControl(l)
			return (ui.Grid{MinColumn: 170, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
				if index == 1 {
					return sortControl(gtx, view)
				}
				return p.output.ranking.search.Layout(gtx, view, i18n.RankingSearch, i18n.RankingSearch, func(string) {
					p.output.list.Position = layout.Position{}
					gtx.Execute(op.InvalidateCmd{})
				})
			})
		}),
		ui.Rigid(contributionHeadings(view, data)),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			search := strings.ToLower(strings.TrimSpace(p.output.ranking.search.Field.Text()))
			details := make([]application.ContributionRow, 0, len(data.Rows))
			for _, row := range data.Rows {
				if strings.Contains(strings.ToLower(p.contributionName(l, data.GroupBy, row)), search) {
					details = append(details, row)
				}
			}
			if len(details) == 0 {
				return view.Message(l.Text(i18n.RankingNoMatches), view.Colors.SecondaryInk)(gtx)
			}
			if p.output.rankButtons == nil {
				p.output.rankButtons = make(map[string]*widget.Clickable)
			}
			return analysisResultList(gtx, view, &p.output.list, l.Text(i18n.RankingDetails), len(details), func(gtx layout.Context, index int) layout.Dimensions {
				row := details[index]
				return p.contributionButton(gtx, view, data, row, portfolioButton(p.output.rankButtons, row.Key))
			})
		}))
	return rows
}

func contributionHeadings(view ui.View, data application.ContributionResult) layout.Widget {
	l := view.Language
	group := i18n.AnalysisInstrument
	switch data.GroupBy {
	case application.ContributionGroupAccount:
		group = i18n.AnalysisAccount
	case application.ContributionGroupCurrency:
		group = i18n.AnalysisCurrency
	case application.ContributionGroupAssetClass:
		group = i18n.AnalysisAssetClass
	}
	magnitude := l.Text(i18n.ReturnContribution)
	if data.ReturnType == application.ContributionTotalReturn {
		magnitude += " / " + l.Text(i18n.ReturnRate)
	}
	return labeledRow(view, l.Text(group)+" · "+magnitude, l.Text(group), magnitude, view.Colors.SecondaryInk)
}

func contributionSummary(view ui.View, data application.ContributionSummary) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		labels := [...]i18n.MessageID{i18n.RankingTotal, i18n.RankingPositive, i18n.RankingNegative}
		values := [...]*domain.SignedMoney{data.Total, data.Positive, data.Negative}
		return (ui.Grid{MinColumn: 150, Gap: 10, MaxColumns: len(labels)}).Layout(gtx, view, len(labels),
			func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
				return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return (ui.Column{Gap: 5}).Layout(gtx,
							ui.Rigid(view.Message(view.Language.Text(labels[index]), view.Colors.SecondaryInk)),
							ui.Rigid(view.Body(ui.TextLabel, returnAmountText(view, values[index], data.AmountStatus), view.Colors.Ink)))
					})
				})
			})
	}
}

func (p *returnAnalysisPage) layoutRankedContributions(gtx layout.Context, view ui.View, data application.ContributionResult) layout.Dimensions {
	ranking := &p.output.ranking
	l := view.Language
	for index := range ranking.tabs {
		if ranking.tabs[index].Clicked(gtx) {
			ranking.direction = contributionDirection(index)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	options := [...]int{10, 20, -1}
	for index := range ranking.limits {
		if ranking.limits[index].Clicked(gtx) {
			ranking.limit = options[index]
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	limit := ranking.limit
	if limit == 0 {
		limit = 10
	}
	controls := func(gtx layout.Context) layout.Dimensions {
		heading := view.Body(ui.TextHeading, l.Text(i18n.RankingMain), view.Colors.Ink)
		order := view.Body(ui.TextCaption, l.Text(i18n.RankingOrderHint), view.Colors.SecondaryInk)
		directions := func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(contributionDirections), func(gtx layout.Context, index int) layout.Dimensions {
				return analysisToggle(view, &ranking.tabs[index], l.Text(contributionDirections[index]), ranking.direction == contributionDirection(index))(gtx)
			})
		}
		more := func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(options), func(gtx layout.Context, index int) layout.Dimensions {
				count := options[index]
				text := l.Text(i18n.RankingAll)
				if count > 0 {
					text = fmt.Sprintf(l.Text(i18n.RankingShowCount), count)
				}
				return analysisToggle(view, &ranking.limits[index], text, count == limit)(gtx)
			})
		}
		return (ui.Column{Gap: 7}).Layout(gtx, ui.Rigid(heading), ui.Rigid(order), ui.Rigid(directions), ui.Rigid(more))
	}
	ranked := rankedContributionRows(data.Rows, ranking.direction, limit)
	count := fmt.Sprintf(l.Text(i18n.RankingVisible), len(ranked), len(data.Rows), len(data.Rows)-len(ranked))
	rows := []ui.ColumnChild{ui.Rigid(controls)}
	if len(ranked) > 0 {
		rows = append(rows, ui.Rigid(p.contributionRankingChart(view, data, ranked)))
	} else {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.RankingNoMatches), view.Colors.SecondaryInk)))
	}
	return (ui.Column{Gap: 8}).Layout(gtx, append(rows,
		ui.Rigid(view.Body(ui.TextCaption, count, view.Colors.SecondaryInk)),
		ui.Rigid(view.Body(ui.TextCaption, l.Text(i18n.RankingDisplayScope), view.Colors.SecondaryInk)))...)
}

func (p *returnAnalysisPage) contributionName(l i18n.Language, group application.ContributionGroupBy, row application.ContributionRow) string {
	if group == application.ContributionGroupCurrency {
		return row.Key
	}
	kind, currency, _ := strings.Cut(row.Key, ":")
	if kind == domain.BucketCash {
		name := l.Text(i18n.AccountCash)
		if currency != "" {
			name += " · " + currency
		}
		return name
	}
	if group == application.ContributionGroupAssetClass {
		if label := marketInstrumentTypes[domain.InstrumentType(row.Key)]; label != i18n.NoMessage {
			return l.Text(label)
		}
	}
	if name := p.refs.names[row.Key]; name != "" {
		return name
	}
	if row.Label != "" && row.Label != row.Key {
		return row.Label
	}
	return l.Text(i18n.OverviewEvidenceUnavailable)
}

func (p *returnAnalysisPage) contributionButton(gtx layout.Context, view ui.View, data application.ContributionResult, row application.ContributionRow, button *widget.Clickable) layout.Dimensions {
	name := p.contributionName(view.Language, data.GroupBy, row)
	amount := returnSignedMoney(view, row.Amount)
	label, rate := name+" · "+amount, ""
	if data.ReturnType == application.ContributionTotalReturn {
		rate = returnRate(view, row.Rate)
		label += " · " + rate
	}
	note := ""
	if row.Status == domain.CompletenessPartial || row.Status == domain.CompletenessUnavailable {
		note = view.Language.Text(contributionAvailabilityCopy(row.AnalysisAvailability))
		label += " · " + note
	}
	view.Focus.Watch(gtx, button)
	if gtx.Enabled() && p.returnFocus == button {
		p.returnFocus = nil
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	if button.Clicked(gtx) {
		p.openContribution(row.Key, name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.ButtonOf(button, ui.ButtonSpec{Name: label, Variant: ui.ButtonGhost, Toggle: true,
		Selected: p.output.selectedKey == row.Key, Fill: true, Content: func(gtx layout.Context) layout.Dimensions {
			identity := func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink))}
				if note != "" {
					rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, note, view.Colors.WarningInk)))
				}
				return (ui.Column{Gap: ui.Space2}).Layout(gtx, rows...)
			}
			columns := []layout.FlexChild{layout.Flexed(1, identity), layout.Rigid(view.Body(ui.TextLabel, amount, view.Colors.Ink))}
			if rate != "" {
				columns = append(columns, layout.Rigid(view.Body(ui.TextCaption, rate, view.Colors.SecondaryInk)))
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, columns...)
		}})(gtx)
}
