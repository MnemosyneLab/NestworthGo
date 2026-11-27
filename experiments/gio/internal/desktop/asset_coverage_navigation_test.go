package desktop

import (
	"context"
	"image"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type partialAssetCoverageReader struct {
	*application.Service
}

func (r partialAssetCoverageReader) AssetChange(context.Context, domain.AnalysisQuery) (application.AssetChangeResult, error) {
	return application.AssetChangeResult{
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessPartial},
	}, nil
}

func (r partialAssetCoverageReader) AssetTrend(_ context.Context, query domain.AnalysisQuery, _ application.AssetTrendGranularity, _ application.AssetTrendMetric) (application.AssetTrendResult, error) {
	return application.AssetTrendResult{
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessPartial},
		Points: []application.AssetTrendPoint{{
			Period:               query.From,
			AnalysisAvailability: application.AnalysisAvailability{Status: domain.CompletenessUnavailable},
		}},
	}, nil
}

func TestAssetCoverageLinkUsesEffectiveDateRangeAndRestoresTab(t *testing.T) {
	ctx := t.Context()
	service, _, _, _ := openExistingBuyFixture(t)
	assets := &assetChangesPage{ctx: ctx, reader: partialAssetCoverageReader{service}, clock: linkedContractClock, invalidate: func() {}}
	health := &dataHealthPage{ctx: ctx, invalidate: func() {}, reader: fixedDataHealthRead{Issues: []application.HealthIssue{
		{ID: "within", Kind: application.HealthKindSnapshotIncomplete, Label: "Synthetic within", RangeStart: "2026-11-22", RangeEnd: "2026-11-22"},
		{ID: "outside", Kind: application.HealthKindSnapshotIncomplete, Label: "Synthetic outside", RangeStart: "2026-11-24", RangeEnd: "2026-11-24"},
	}}}
	t.Cleanup(assets.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{assets: assets, dataHealth: health})
	h.size = image.Pt(1100, 980)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			assets.workers.Wait()
			health.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	settle()
	click("Asset changes")
	assets.filters.selection.from, assets.filters.selection.to = "2026-11-22", "2026-11-23"
	assets.change()
	settle()
	for _, tab := range []assetChangesTab{assetTrendTab, assetDriversTab} {
		if tab == assetDriversTab {
			click("Change drivers")
		}
		click("View period coverage gaps")
		if h.shell.activeRoute() != dataHealthRoute || health.focus.from != "2026-11-22" || health.focus.to != "2026-11-23" {
			t.Fatalf("Data Health opened at the wrong focus: %+v", health.focus)
		}
		assertPageLabel(t, &h.router, "Synthetic within")
		if findPageLabel(&h.router, "Synthetic outside") != nil {
			t.Fatal("the period coverage link included an out-of-window finding")
		}
		click("Back")
		if assets.state.tab != tab || assets.filters.selection.from != "2026-11-22" || assets.filters.selection.to != "2026-11-23" {
			t.Fatal("Back did not restore the originating analysis tab and dates")
		}
		h.focused(&assets.coverage)
	}
}
