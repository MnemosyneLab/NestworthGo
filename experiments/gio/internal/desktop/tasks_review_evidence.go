package desktop

import (
	"fmt"
	"slices"

	"gioui.org/layout"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func taskReviewRule(view ui.View, rule domain.ReviewAlertRule, acknowledge layout.Widget, known taskReviewKnown, hasKnownRead bool) layout.Widget {
	l := view.Language
	fields := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		label, tone := taskReviewPriority(rule.Severity)
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(label)) }),
			layout.Flexed(1, view.Body(ui.TextLabel, rule.Name, view.Colors.Ink)),
		)
	})}
	if rule.TriggeredOn != "" {
		triggered := fmt.Sprintf(l.Text(i18n.TasksReviewTriggered), historyDay(view, rule.TriggeredOn))
		fields = append(fields, layout.Rigid(view.Message(triggered, view.Colors.SecondaryInk)))
	}
	if rule.Window.WindowStart != "" && rule.Window.WindowEnd != "" {
		window := fmt.Sprintf(l.Text(i18n.TasksReviewWindow), historyDay(view, rule.Window.WindowStart),
			historyDay(view, rule.Window.WindowEnd), rule.Window.HitCount)
		fields = append(fields, layout.Rigid(view.Message(window, view.Colors.SecondaryInk)))
	}
	if acknowledge != nil {
		fields = append(fields, layout.Rigid(acknowledge))
	}
	latest := taskReviewAssessment(view, taskReviewAssessmentKey{rule.EpisodeID, i18n.TasksReviewLatestAssessment},
		rule.LatestResult, i18n.TasksReviewAssessmentUnavailable)
	if hasKnownRead {
		missing := i18n.TasksReviewNoKnownAssessment
		if known.failed {
			missing = i18n.TasksReviewEvidenceLoadFailed
		}
		previous := taskReviewAssessment(view, taskReviewAssessmentKey{rule.EpisodeID, i18n.TasksReviewKnownAssessment}, known.result, missing)
		fields = append(fields, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			assessments := [...]layout.Widget{latest, previous}
			return (ui.Grid{MinColumn: 275, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, len(assessments),
				func(gtx layout.Context, _ ui.View, i int) layout.Dimensions { return assessments[i](gtx) })
		}), layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.TasksReviewUnknownDoesNotRecover), view.Colors.SecondaryInk)))
	} else {
		fields = append(fields, layout.Rigid(latest))
	}
	return func(gtx layout.Context) layout.Dimensions {
		return accessibility.Row(gtx, rule.EpisodeID, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, fields...)
		})
	}
}

func taskReviewPriority(severity string) (i18n.MessageID, ui.BadgeTone) {
	switch severity {
	case "p0":
		return i18n.TasksReviewPriorityHighest, ui.BadgeDestructive
	case "p1", "warning":
		return i18n.TasksReviewPriorityHigh, ui.BadgeWarning
	default:
		return i18n.TasksReviewPriorityNormal, ui.BadgeOutline
	}
}

type taskReviewAssessmentKey struct {
	episode domain.ReviewEpisodeID
	title   i18n.MessageID
}

type taskReviewConditionKey struct {
	assessment taskReviewAssessmentKey
	index      int
}

func taskReviewAssessment(view ui.View, key taskReviewAssessmentKey, result *domain.ReviewDailyResult, missing i18n.MessageID) layout.Widget {
	l := view.Language
	title := l.Text(key.title)
	return func(gtx layout.Context) layout.Dimensions {
		return accessibility.Section(gtx, key, title, func(gtx layout.Context) layout.Dimensions {
			return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(ui.Space12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					fields := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// AppKit already announces this title on the section.
						return accessibility.Hidden(gtx, view.Body(ui.TextCaptionStrong, title, view.Colors.SecondaryInk))
					})}
					if result == nil {
						fields = append(fields, layout.Rigid(view.Message(l.Text(missing), view.Colors.SecondaryInk)))
					} else {
						fields = append(fields, layout.Rigid(taskReviewResultDay(view, *result)))
						definitions := appendReviewDefinitions(nil, result.Predicate)
						conditions := 0
						for i, leaf := range result.Leaves {
							label := taskReviewMetric(leaf.Metric)
							if label == i18n.NoMessage {
								continue
							}
							// Definitions are from this exact published vote. If
							// their shape is incomplete, display observed facts only.
							var definition *domain.ReviewRuleLeaf
							if len(definitions) == len(result.Leaves) && definitions[i].Metric == leaf.Metric {
								definition = definitions[i]
							}
							fields = append(fields, layout.Rigid(taskReviewCondition(view, taskReviewConditionKey{key, i}, label, leaf, definition)))
							conditions++
						}
						if conditions == 0 {
							fields = append(fields, layout.Rigid(view.Message(l.Text(i18n.TasksReviewNoDailyEvidence), view.Colors.SecondaryInk)))
						}
						for _, reason := range taskReviewMissingReasons(result) {
							fields = append(fields, layout.Rigid(view.Body(ui.TextCaption, l.Text(reason), view.Colors.SecondaryInk)))
						}
					}
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, fields...)
				})
			})
		})
	}
}

func taskReviewMissingReasons(result *domain.ReviewDailyResult) []i18n.MessageID {
	if reviewAssessmentKnown(result) {
		return nil
	}
	var reasons []i18n.MessageID
	for _, leaf := range result.Leaves {
		var reason i18n.MessageID
		switch leaf.Reason {
		case "daily_performance_unknown":
			reason = i18n.TasksReviewPerformanceUnknown
		case "historical_amount_unknown":
			reason = i18n.TasksReviewHoldingAmountUnknown
		default:
			continue
		}
		if !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

// The review engine appends daily evidence depth-first in this same order.
func appendReviewDefinitions(out []*domain.ReviewRuleLeaf, node *domain.ReviewPredicateNode) []*domain.ReviewRuleLeaf {
	if node == nil {
		return out
	}
	if node.Leaf != nil {
		return append(out, node.Leaf)
	}
	children := node.All
	if children == nil {
		children = node.Any
	}
	for i := range children {
		out = appendReviewDefinitions(out, &children[i])
	}
	return out
}

func taskReviewResultDay(view ui.View, result domain.ReviewDailyResult) layout.Widget {
	label := i18n.TasksReviewUnknown
	switch result.Truth {
	case domain.ReviewTrue:
		label = i18n.TasksReviewHit
	case domain.ReviewFalse:
		label = i18n.TasksReviewNoHit
	}
	return func(gtx layout.Context) layout.Dimensions {
		var fields []layout.FlexChild
		if _, valid := ui.ParseCivilDay(result.On); valid {
			fields = append(fields, layout.Rigid(view.Body(ui.TextCaption, historyDay(view, result.On), view.Colors.Ink)))
		}
		fields = append(fields, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Badge(gtx, ui.BadgeNeutral, view.Language.Text(label))
		}))
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, fields...)
	}
}

func taskReviewMetric(metric string) i18n.MessageID {
	switch metric {
	case "amount":
		return i18n.TasksReviewAmount
	case "cost":
		return i18n.TasksReviewCost
	case "price_change":
		return i18n.TasksReviewPriceChange
	case "annualized":
		return i18n.TasksReviewAnnualized
	default:
		return i18n.NoMessage
	}
}

func taskReviewCondition(view ui.View, key taskReviewConditionKey, label i18n.MessageID, leaf domain.ReviewDailyLeafEvidence, definition *domain.ReviewRuleLeaf) layout.Widget {
	l := view.Language
	title := l.Text(label)
	if definition != nil {
		if definition.LookbackDays > 0 {
			title += " · " + fmt.Sprintf(l.Text(i18n.TasksReviewLookbackDays), definition.LookbackDays)
		}
		if definition.Operator != "" && definition.Value != "" {
			title += " · " + definition.Operator + " " + taskReviewMetricValue(view, definition.Metric, definition.Value, definition.Currency)
		}
	}
	observed := taskReviewMetricValue(view, leaf.Metric, leaf.Actual, leaf.Currency)
	fields := []layout.FlexChild{
		layout.Rigid(view.Body(ui.TextCaption, title, view.Colors.SecondaryInk)),
		layout.Rigid(view.Body(ui.TextCaptionStrong, observed, view.Colors.Ink)),
	}
	_, baseline := ui.ParseCivilDay(leaf.BaselineOn)
	_, effective := ui.ParseCivilDay(leaf.EffectiveOn)
	if baseline && effective {
		dates := historyDay(view, leaf.BaselineOn)
		if leaf.BaselineOn != leaf.EffectiveOn {
			dates = performanceDateRange(view, leaf.BaselineOn, leaf.EffectiveOn)
		}
		fields = append(fields, layout.Rigid(view.Body(ui.TextCaption, dates, view.Colors.SecondaryInk)))
	}
	return func(gtx layout.Context) layout.Dimensions {
		return accessibility.Row(gtx, key, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space2)}.Layout(gtx, fields...)
		})
	}
}

func taskReviewMetricValue(view ui.View, metric, value string, currency domain.CurrencyCode) string {
	exact, err := decimal.NewFromString(value)
	if err != nil {
		return view.Language.Text(i18n.TasksReviewUnknown)
	}
	prefs := settings.Settings{DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator}
	switch metric {
	case "price_change", "annualized":
		return format.ExactDecimal(exact.RoundBank(2).StringFixed(2), prefs) + "%"
	default:
		if currency == "" {
			return format.ExactDecimal(exact.RoundBank(2).StringFixed(2), prefs)
		}
		return format.Money(value, currency.String(), prefs)
	}
}
