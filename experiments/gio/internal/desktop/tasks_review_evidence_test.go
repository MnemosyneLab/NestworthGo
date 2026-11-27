package desktop

import (
	"context"
	"slices"
	"strings"
	"testing"

	"gioui.org/io/input"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wails "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestTaskReviewEvidenceShowsPublishedPriorityAndAssessment(t *testing.T) {
	rule := domain.ReviewAlertRule{
		Name: "Synthetic momentum and cost", Severity: "p0", Actionable: true, TriggeredOn: "2026-11-18",
		Window:        domain.ReviewPolicyWindow{WindowStart: "2026-11-16", WindowEnd: "2026-11-22", HitCount: 3},
		TriggerWindow: &domain.ReviewPolicyWindow{WindowStart: "2026-11-12", WindowEnd: "2026-11-18", HitCount: 2},
		LatestResult: &domain.ReviewDailyResult{
			On: "2026-11-22", Truth: domain.ReviewTrue,
			Predicate: &domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
				{Any: []domain.ReviewPredicateNode{
					{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: "-1", LookbackDays: 7}},
					{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 30}},
				}},
				{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: ">", Value: "100", Currency: "USD"}},
			}},
			Leaves: []domain.ReviewDailyLeafEvidence{
				{Metric: "price_change", Truth: domain.ReviewTrue, Actual: "-2.005", Threshold: "-1", BaselineOn: "2026-11-15", EffectiveOn: "2026-11-22"},
				{Metric: "price_change", Truth: domain.ReviewFalse, Actual: "9.125", Threshold: "10", BaselineOn: "2026-10-23", EffectiveOn: "2026-11-22"},
				{Metric: "cost", Truth: domain.ReviewUnknown, Threshold: "100", Currency: "USD", Reason: "historical_amount_unknown"},
			},
		},
	}
	report := taskReviewEvidenceReport(rule,
		domain.ReviewAlertRule{Name: "Synthetic missing assessment", Severity: "warning", Actionable: true},
		domain.ReviewAlertRule{Name: "Synthetic quiet assessment", Severity: "notice", Actionable: true, LatestResult: &domain.ReviewDailyResult{
			On: "2026-11-21", Truth: domain.ReviewFalse,
		}},
	)
	for _, tc := range []struct {
		lang   i18n.Language
		labels []string
		hit    string
		noHit  string
		seven  string
		thirty string
		cost   string
	}{
		{i18n.English, []string{
			"P0 · Highest", "P1 · High", "P2 · Normal", "Latest assessment", "Assessment unavailable.",
			"Review window · 2026-11-16 to 2026-11-22 · Matches 3",
		}, "Hit", "No hit", "Price change · 7 days · < -1.00%", "Price change · 30 days · > 10.00%", "Cost · > US$100.00"},
		{i18n.SimplifiedChinese, []string{
			"P0 · 最高", "P1 · 高", "P2 · 普通", "最新评估", "评估暂不可用。",
			"核查窗口 · 2026-11-16 至 2026-11-22 · 命中 3 天",
		}, "命中", "未命中", "近期涨跌 · 7 天 · < -1.00%", "近期涨跌 · 30 天 · > 10.00%", "成本 · > US$100.00"},
	} {
		t.Run(string(tc.lang), func(t *testing.T) {
			router := taskReviewEvidenceFrame(report, tc.lang)
			for _, label := range append(tc.labels, "Synthetic reviewed investment", rule.Name, "Synthetic missing assessment", "Synthetic quiet assessment") {
				assertPageLabel(t, router, label)
			}
			assertTaskReviewEvidenceSequence(t, router, tc.seven, "-2.00%", "2026-11-15 – 2026-11-22")
			assertTaskReviewEvidenceSequence(t, router, tc.thirty, "9.12%", "2026-10-23 – 2026-11-22")
			unknown := "Unknown"
			if tc.lang == i18n.SimplifiedChinese {
				unknown = "未知"
			}
			assertTaskReviewEvidenceSequence(t, router, tc.cost, unknown)
			assertTaskReviewDateTruth(t, router, "2026-11-22", tc.hit)
			assertTaskReviewDateTruth(t, router, "2026-11-21", tc.noHit)
			assertTaskReviewEvidenceOmits(t, router, "US$0.00")
		})
	}
}

func TestTaskReviewEvidenceKeepsOnlySupportedPredicateContext(t *testing.T) {
	for _, tc := range []struct {
		name      string
		predicate *domain.ReviewPredicateNode
	}{
		{name: "unavailable condition"},
		{name: "mismatched metric", predicate: &domain.ReviewPredicateNode{
			Leaf: &domain.ReviewRuleLeaf{Metric: "annualized", LookbackDays: 45, Operator: ">", Value: "77"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := domain.ReviewAlertRule{
				Name: "Synthetic older result", Actionable: true, Severity: "p2",
				LatestResult: &domain.ReviewDailyResult{
					On: "2026-11-21", Truth: domain.ReviewUnknown, Predicate: tc.predicate,
					Leaves: []domain.ReviewDailyLeafEvidence{{Metric: "price_change", Truth: domain.ReviewTrue, Actual: "12", Threshold: "69"}},
				},
			}
			router := taskReviewEvidenceFrame(taskReviewEvidenceReport(rule), i18n.English)
			assertPageLabel(t, router, "Price change")
			assertPageLabel(t, router, "12.00%")
			assertTaskReviewDateTruth(t, router, "2026-11-21", "Unknown")
			assertTaskReviewEvidenceOmits(t, router, "45 days", "77.00%", "69.00%", ">")
		})
	}
}

func TestTaskReviewEvidenceUsesHistoricalRuleAfterRuleChange(t *testing.T) {
	ctx := t.Context()
	app, _, _, term := openManagedProductFixture(t)
	if _, err := seedTaskChecks(ctx, app, term, accountTestClock); err != nil {
		t.Fatal(err)
	}
	adapter := wails.NewService(app)
	rules, err := adapter.ListReviewRulesV2(ctx, wails.ListReviewRulesV2Request{PageSize: 10})
	if err != nil || len(rules) != 1 {
		t.Fatalf("expected one published fixture rule: %v / %v", rules, err)
	}
	rule := rules[0]
	spec := rule.Spec
	spec.Predicate.All[0].Leaf.Value = "777.75"
	current, err := adapter.UpdateReviewRuleV2(ctx, application.SaveReviewRuleV2Input{
		RuleID: string(rule.ID), MutationID: domain.NewMutationID().String(), ExpectedRevision: rule.RowRevision,
		Name: rule.Name, Enabled: rule.Enabled, Severity: rule.Severity, Spec: spec,
	})
	if err != nil || current.CurrentConditionVersion <= rule.CurrentConditionVersion {
		t.Fatalf("editing the rule did not produce a distinct current condition: %+v / %v", current, err)
	}
	report, err := adapter.ListReviewAlertsV2(ctx, wails.ListReviewAlertsV2Request{
		Filter: domain.ReviewAlertFilter{Status: "pending"}, PageSize: 10,
	})
	if err != nil || len(report.Items) != 1 || len(report.Items[0].Rules) != 1 {
		t.Fatalf("expected one retained published assessment: %+v / %v", report, err)
	}
	saved := report.Items[0].Rules[0].LatestResult
	if saved == nil || saved.Predicate == nil || len(saved.Predicate.All) != 1 || saved.Predicate.All[0].Leaf == nil ||
		saved.Predicate.All[0].Leaf.Value != "100" || len(saved.Leaves) != 1 || saved.Leaves[0].Actual != "45.0000" {
		t.Fatalf("Wails did not retain the historical condition and vote: %+v", saved)
	}
	router := taskReviewEvidenceFrame(report, i18n.English)
	assertPageLabel(t, router, "Cost · < US$100.00")
	assertPageLabel(t, router, "US$45.00")
	assertTaskReviewEvidenceOmits(t, router, "777.75")
}

func TestTaskReviewEvidenceKeepsLatestAndKnownAssessmentsSeparate(t *testing.T) {
	known := domain.ReviewDailyResult{
		On: "2026-11-25", Truth: domain.ReviewTrue,
		Predicate: &domain.ReviewPredicateNode{
			Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: "-1", LookbackDays: 7},
		},
		Leaves: []domain.ReviewDailyLeafEvidence{{Metric: "price_change", Truth: domain.ReviewTrue, Actual: "-8"}},
	}
	for _, tc := range []struct {
		name             string
		latest           domain.ReviewTruth
		fallback         taskReviewKnown
		labels, omitted  []string
		latestTruthLabel string
	}{
		{
			name: "unknown with prior valid date", latest: domain.ReviewUnknown, fallback: taskReviewKnown{result: &known},
			labels:           []string{"Most recent valid assessment", "Price change · 7 days · < -1.00%", "-8.00%"},
			latestTruthLabel: "Unknown",
		},
		{
			name: "failed fallback", latest: domain.ReviewUnknown, fallback: taskReviewKnown{failed: true},
			labels:           []string{"Most recent valid assessment", "Unable to load this publication’s daily evidence. Refresh to try again."},
			omitted:          []string{"2026-11-25", "-8.00%"},
			latestTruthLabel: "Unknown",
		},
		{
			name: "valid latest", latest: domain.ReviewFalse,
			omitted:          []string{"Most recent valid assessment", "2026-11-25", "-8.00%"},
			latestTruthLabel: "No hit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			latest := &domain.ReviewDailyResult{
				On: "2026-11-26", Truth: tc.latest,
				Leaves: []domain.ReviewDailyLeafEvidence{{Metric: "price_change", Truth: tc.latest}},
			}
			if tc.latest != domain.ReviewUnknown {
				latest.Leaves[0].Actual = "1"
			}
			rule := domain.ReviewAlertRule{Actionable: true, Name: "Synthetic assessment", LatestResult: latest}
			if tc.latest == domain.ReviewUnknown {
				rule.Actionable = false
				rule.Status = "unknown"
			}
			report := taskReviewEvidenceReport(rule)
			page := tasksPage{review: &report}
			if tc.latest == domain.ReviewUnknown {
				page.reviewKnown = map[domain.ReviewEpisodeID]taskReviewKnown{report.Items[0].Rules[0].EpisodeID: tc.fallback}
			}
			router := renderTaskReviewEvidence(&page, i18n.English)
			assertPageLabel(t, router, "Latest assessment")
			assertTaskReviewDateTruth(t, router, latest.On, tc.latestTruthLabel)
			for _, label := range tc.labels {
				assertPageLabel(t, router, label)
			}
			if tc.latest == domain.ReviewUnknown && !tc.fallback.failed {
				assertTaskReviewDateTruth(t, router, known.On, "Hit")
				assertTaskReviewEvidenceSequence(t, router, "Price change", "Unknown")
			}
			assertTaskReviewEvidenceOmits(t, router, tc.omitted...)
		})
	}
}

type taskReviewDailyEvidenceFixture struct {
	taskChecksReader
	page  domain.ReviewDailyEvidencePage
	calls int
}

func (f *taskReviewDailyEvidenceFixture) ListReviewDailyEvidenceV2(context.Context, string, string, int) (domain.ReviewDailyEvidencePage, error) {
	f.calls++
	return f.page, nil
}

func TestTaskReviewKnownEvidenceReadChecksPublishedEpisode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*domain.ReviewDailyEvidencePage)
		wantRead bool
		failed   bool
	}{
		{name: "matching published episode", wantRead: true},
		{name: "older episode revision", change: func(p *domain.ReviewDailyEvidencePage) { p.EpisodeRevision-- }, wantRead: true, failed: true},
		{name: "another episode identity", change: func(p *domain.ReviewDailyEvidencePage) { p.EpisodeID = domain.NewReviewEpisodeID() }, wantRead: true, failed: true},
		{name: "another latest day", change: func(p *domain.ReviewDailyEvidencePage) { p.Items[0].On = "2026-11-27" }, wantRead: true, failed: true},
		{name: "future known result", change: func(p *domain.ReviewDailyEvidencePage) { p.LatestKnownResult.On = "2026-11-27" }, wantRead: true, failed: true},
		{name: "no read for valid latest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			latest := domain.ReviewDailyResult{On: "2026-11-26", Truth: domain.ReviewUnknown}
			if !tc.wantRead {
				latest.Truth = domain.ReviewFalse
			}
			known := &domain.ReviewDailyResult{On: "2026-11-25", Truth: domain.ReviewTrue}
			report := taskReviewEvidenceReport(domain.ReviewAlertRule{Actionable: true, LatestResult: &latest})
			rule := report.Items[0].Rules[0]
			service := &taskReviewDailyEvidenceFixture{page: domain.ReviewDailyEvidencePage{
				EpisodeID: rule.EpisodeID, EpisodeRevision: rule.EpisodeRevision, Items: []domain.ReviewDailyResult{latest},
				LatestKnownResult: known,
			}}
			if tc.change != nil {
				tc.change(&service.page)
			}
			result, read := readTaskReviewKnown(t.Context(), service, report)[rule.EpisodeID]
			if read != tc.wantRead || result.failed != tc.failed || service.calls > 1 {
				t.Fatalf("fallback read = %+v (published %v, calls %d)", result, read, service.calls)
			}
			want := known
			if !tc.wantRead || tc.failed {
				want = nil
			}
			if result.result != want || (service.calls == 1) != tc.wantRead {
				t.Fatalf("fallback evidence = %+v, want %+v", result.result, want)
			}
		})
	}
}

func taskReviewEvidenceReport(rules ...domain.ReviewAlertRule) domain.ReviewAlertPage {
	for i := range rules {
		rules[i].RuleID, rules[i].EpisodeID = domain.NewReviewRuleID(), domain.NewReviewEpisodeID()
		rules[i].EpisodeRevision = 1
	}
	return domain.ReviewAlertPage{
		ProjectionStatus: "complete", PublishedOn: "2026-11-22",
		Items: []domain.ReviewAlertGroup{{InstrumentID: domain.NewInstrumentID(), DisplayName: "Synthetic reviewed investment", Rules: rules}},
	}
}

func taskReviewEvidenceFrame(report domain.ReviewAlertPage, language i18n.Language) *input.Router {
	return renderTaskReviewEvidence(&tasksPage{review: &report}, language)
}

func renderTaskReviewEvidence(page *tasksPage, language i18n.Language) *input.Router {
	view := ui.View{Theme: ui.NewTheme(), Language: language, Colors: &lightPalette,
		DateFormat: settings.DateFormatISO, DecimalSeparator: ".", GroupingSeparator: ","}
	gtx := chartTableContext(960)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = 0, 1800
	var children []ui.ColumnChild
	for _, row := range page.reviewRows(view) {
		children = append(children, ui.Rigid(row))
	}
	(ui.Column{Gap: ui.Space8}).Layout(gtx, children...)
	router := new(input.Router)
	router.Frame(gtx.Ops)
	return router
}

func assertTaskReviewDateTruth(t *testing.T, router *input.Router, date, truth string) {
	t.Helper()
	labels := taskReviewEvidenceLabels(router)
	for i, label := range labels {
		if label == date && i+1 < len(labels) && labels[i+1] == truth {
			return
		}
		parts := strings.Split(label, " · ")
		if slices.Contains(parts, date) && slices.Contains(parts, truth) {
			return
		}
	}
	t.Fatalf("assessment did not expose %s and %s: %v", date, truth, labels)
}

func assertTaskReviewEvidenceSequence(t *testing.T, router *input.Router, want ...string) {
	t.Helper()
	labels := taskReviewEvidenceLabels(router)
	for i := 0; i+len(want) <= len(labels); i++ {
		if slices.Equal(labels[i:i+len(want)], want) {
			return
		}
	}
	t.Fatalf("assessment did not keep its evidence together: %v in %v", want, labels)
}

func assertTaskReviewEvidenceOmits(t *testing.T, router *input.Router, unwanted ...string) {
	t.Helper()
	for _, label := range taskReviewEvidenceLabels(router) {
		for _, value := range unwanted {
			if strings.Contains(label, value) {
				t.Fatalf("unexpected value %q in assessment label %q", value, label)
			}
		}
	}
}

func taskReviewEvidenceLabels(router *input.Router) []string {
	var labels []string
	var collect func([]input.SemanticNode)
	collect = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Label != "" {
				labels = append(labels, node.Desc.Label)
			}
			collect(node.Children)
		}
	}
	collect(router.AppendSemantics(nil))
	return labels
}
