package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/data"
)

func TestSettingsDataRebuildMatchesWailsAndPreservesPreferences(t *testing.T) {
	ctx := t.Context()
	const closedDays = 3
	start := time.Date(2026, time.November, 6, 9, 0, 0, 0, time.UTC)
	now := start.AddDate(0, 0, closedDays)
	clock := func() time.Time { return now }
	seed, household := seedStage6Snapshots(t, start, closedDays, true)
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	_, repo, gio := openStage6Database(t, gioPath, clock)
	_, _, wails := openStage6Database(t, wailsPath, clock)
	before, err := gio.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h, page := generalSettingsHarness(t)
	page.edit(i18n.SettingsQuoteCacheTTL, settings.QuoteCacheTTL1h)
	cancelledLatest := false
	page.data.cancelLatest = func() { cancelledLatest = true }
	unblock := make(chan struct{}, 1)
	t.Cleanup(func() {
		close(unblock)
		page.prefs.wait()
	})
	page.data.run = func(ctx context.Context) (application.DerivedDataRebuildResult, error) {
		<-unblock
		return gio.RebuildDerivedData(ctx)
	}
	h.router.Source().Execute(key.FocusCmd{Tag: &page.tabs[settingsMarket]})
	h.frame()
	h.press(key.NameRightArrow)
	h.focused(&page.tabs[settingsData])
	assertPageLabel(t, &h.router, "Recalculate history and balances")
	h.click("Recalculate all data", false)
	if page.data.request == nil || !cancelledLatest {
		t.Fatal("recalculation did not start after fencing the latest refresh")
	}
	assertPageLabel(t, &h.router, "Recalculating your data. Keep this window open until it finishes.")
	h.click("Appearance", false)
	unblock <- struct{}{}
	page.prefs.wait()
	h.frame()
	h.click("Data", false)
	assertPageLabel(t, &h.router, "Recalculation complete.")
	assertPageLabel(t, &h.router, fmt.Sprintf("%d snapshot days · 0 incomplete days", closedDays))
	oracle, err := data.NewService(wails, nil, nil, nil).RebuildDerivedData(ctx)
	if err != nil || page.data.result == nil || *page.data.result != oracle ||
		oracle.From != start.Format(time.DateOnly) || oracle.To != now.Format(time.DateOnly) {
		t.Fatalf("recalculation outcome: Gio %+v, Wails %+v, %v", page.data.result, oracle, err)
	}
	assertPageLabel(t, &h.router, "Period: 2026-11-06 – 2026-11-09.")
	if page.draft().QuoteCacheTTL != settings.QuoteCacheTTL1h || page.prefs.saved.QuoteCacheTTL != settings.QuoteCacheTTL12h {
		t.Fatal("recalc changed the other tabs' preference draft or saved it")
	}
	after, err := gio.HistoryOrigin(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("History start changed after rebuild: %+v, %v", after, err)
	}
	snapshots, err := repo.ListDailyValuationSnapshots(ctx, household, time.Time{}, time.Time{})
	if err != nil || len(snapshots) != closedDays {
		t.Fatalf("recalculation did not publish all closed days: %d, %v", len(snapshots), err)
	}
	for _, row := range snapshots {
		if !row.Complete || row.ResolverPolicyVersion != domain.MarketDataResolverPolicy {
			t.Fatalf("stale derived evidence survived rebuild: %+v", row)
		}
	}
}

func TestSettingsDataShowsHistoryRequirementInSimplifiedChinese(t *testing.T) {
	_, _, app := openStage6Database(t, filepath.Join(t.TempDir(), "no-history.db"), func() time.Time { return accountTestClock })
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic no history", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	h, page := generalSettingsHarness(t)
	page.data.run = app.RebuildDerivedData
	page.prefs.saved.Language = settings.LanguageZhCN
	h.frame()
	h.click("数据", false)
	assertPageLabel(t, &h.router, "重算历史与余额")
	h.click("重算全部数据", false)
	page.prefs.wait()
	h.frame()
	assertPageLabel(t, &h.router, "请先开始历史记录，再重算数据。")
	origin, err := app.HistoryOrigin(t.Context())
	if err != nil || origin != nil || page.data.result != nil {
		t.Fatalf("a recalculation without history returned %+v, origin %+v, %v", page.data.result, origin, err)
	}
}
