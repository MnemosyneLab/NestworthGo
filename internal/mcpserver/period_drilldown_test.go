package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestPeriodDrilldownStaysOutOfCaptureJSON(t *testing.T) {
	day := wire.ReturnDayDTO{
		Date: "2024-01-02", Valuation: "base", Currency: "USD", AmountStatus: "complete", Status: "ok",
		Composition:  []wire.ReturnComponentAmountDTO{},
		Contributors: []wire.ReturnContributorDTO{{Key: "synthetic", Label: "Synthetic stock", RatedDays: 1, TotalDays: 1}},
		Issues:       []wire.ReturnIssueDTO{{Date: "2024-01-02", Status: "partial", MissingReason: "synthetic-drilldown-marker"}},
	}
	driver := wire.AssetDriverDetailDTO{
		DriverKey: "income", Status: "ok",
		ByInstrument:    []wire.AnalysisDimensionAmountDTO{},
		ByAccount:       []wire.AnalysisDimensionAmountDTO{},
		ResidualDetails: []wire.AssetResidualDetailDTO{},
	}
	drill := newPeriodDrilldown("2024-01-01", "2024-01-03", []wire.ReturnDayDTO{day}, []wire.AssetDriverDetailDTO{driver})
	capture, err := periodReadCapture(AnalyzePeriodInput{Query: PeriodAnalysisQuery{From: "2024-01-01", To: "2024-01-03"}}, PeriodAnalysisDTO{}, drill)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic-drilldown-marker") || strings.Contains(string(raw), "returnDays") || strings.Contains(string(raw), "assetDrivers") {
		t.Fatalf("drill-down leaked into capture JSON: %s", raw)
	}
	if len(raw) > readCaptureLimit {
		t.Fatalf("empty period capture is %d bytes", len(raw))
	}

	cache := newBoundedReadCache()
	cache.activate(1)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	env, err := cache.store(1, capture, 25)
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	stored, ok := cache.entries[env.CaptureID]
	cache.mu.Unlock()
	if !ok || stored.drilldown == nil || stored.drilldown.days["2024-01-02"].Issues[0].MissingReason != "synthetic-drilldown-marker" {
		t.Fatal("store dropped the frozen return day")
	}
	if stored.size != len(frozenCaptureBytes(t, capture)) {
		t.Fatalf("charge %d normalized %d", stored.size, len(frozenCaptureBytes(t, capture)))
	}
	if stored.size > readCaptureLimit {
		t.Fatalf("charged %d", stored.size)
	}

	item, err := cache.periodItem(env.CaptureID, stored, ReadItemInput{Section: "returnDays", ID: "2024-01-02"})
	if err != nil {
		t.Fatal(err)
	}
	if item.CaptureID != env.CaptureID || item.ContentHash != env.ContentHash || item.ExpiresAt != env.ExpiresAt {
		t.Fatalf("item envelope changed identity or TTL: %+v", item)
	}
	section := item.Sections["returnDays/2024-01-02/issues"]
	if section.Page.Total != 1 || section.Page.NextCursor == nil {
		t.Fatalf("issues page: %+v", section.Page)
	}
	page, err := cache.page(1, ReadPageInput{CaptureID: env.CaptureID, Section: "returnDays/2024-01-02/issues", Cursor: *section.Page.NextCursor, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.ContentHash != env.ContentHash || page.ExpiresAt != env.ExpiresAt || len(page.Items) != 1 || !strings.Contains(string(page.Items[0]), "synthetic-drilldown-marker") {
		t.Fatalf("page: %+v", page)
	}

	_, err = cache.periodItem(env.CaptureID, stored, ReadItemInput{Section: "returnDays", ID: "2024-02-01"})
	if err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatal(err)
	}
	_, err = cache.periodItem(env.CaptureID, stored, ReadItemInput{Section: "assetDrivers", ID: "not-a-bucket"})
	if err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatal(err)
	}
	driverItem, err := cache.periodItem(env.CaptureID, stored, ReadItemInput{Section: "assetDrivers", ID: "income"})
	if err != nil || driverItem.Sections["assetDrivers/income/byInstrument"].Page.Total != 0 {
		t.Fatalf("driver item: %+v %v", driverItem, err)
	}
}
