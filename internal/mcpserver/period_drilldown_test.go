package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	drill, err := newPeriodDrilldown("2024-01-01", "2024-01-03", []wire.ReturnDayDTO{day}, []wire.AssetDriverDetailDTO{driver})
	if err != nil {
		t.Fatal(err)
	}
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
	if !ok || stored.drilldown == nil || !strings.Contains(string(stored.drilldown.days["2024-01-02"]), "synthetic-drilldown-marker") {
		t.Fatal("store dropped the frozen return day")
	}
	if stored.size != len(frozenCaptureBytes(t, capture))+drill.size {
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

// Synthetic days × instruments exercise the retained graph rather than a large
// parent summary. No ledger or provider access is needed for cache boundaries.
func syntheticPeriodDetails(t *testing.T, days, instruments, labelBytes int) *periodDrilldown {
	t.Helper()
	values := make([]wire.ReturnDayDTO, days)
	for i := range values {
		values[i] = wire.ReturnDayDTO{Date: time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Status: "ok"}
		for j := 0; j < instruments; j++ {
			values[i].Contributors = append(values[i].Contributors, wire.ReturnContributorDTO{Key: fmt.Sprint(j), Label: strings.Repeat("x", labelBytes)})
		}
	}
	drill, err := newPeriodDrilldown("2024-01-01", "2040-12-31", values, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The original projection may be reused or changed after freezing.
	if days > 0 && instruments > 0 {
		values[0].Contributors[0].Label = "mutated"
	}
	return drill
}

func TestPeriodDrilldownRetainedBudgets(t *testing.T) {
	small := syntheticPeriodDetails(t, 2, 3, 100)
	moreDays := syntheticPeriodDetails(t, 4, 3, 100)
	moreInstruments := syntheticPeriodDetails(t, 4, 6, 100)
	if small.size >= moreDays.size || moreDays.size >= moreInstruments.size {
		t.Fatal("details not charged by days and instruments")
	}
	if strings.Contains(string(small.days["2024-01-01"]), "mutated") {
		t.Fatal("retained caller-owned data")
	}
	if _, err := newPeriodDrilldown("", "", make([]wire.ReturnDayDTO, readDrillEntryLimit+1), nil); err == nil {
		t.Fatal("entry limit not enforced")
	}
	if _, err := newPeriodDrilldown("", "", []wire.ReturnDayDTO{{Date: "2024-01-01", MissingReason: strings.Repeat("x", readRetainedCaptureLimit)}}, nil); err == nil {
		t.Fatal("detail byte limit not enforced")
	}

	cache := newBoundedReadCache()
	cache.activate(1)
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	drill := syntheticPeriodDetails(t, 32, 32, 4096)
	capture := readCapture{Kind: "period_analysis", CapturedAt: now, Summary: "", Sections: map[string][]json.RawMessage{"padding": {json.RawMessage(`{}`), json.RawMessage(`""`)}}, drilldown: drill}
	raw := frozenCaptureBytes(t, capture)
	// Exact combined boundary is accepted even when neither part alone exceeds
	// its limit; one more byte is rejected without evicting any admitted data.
	padding := strings.Repeat("p", readRetainedCaptureLimit-drill.size-len(raw))
	capture.Sections["padding"][1] = json.RawMessage(`"` + padding + `"`)
	env, err := cache.store(1, capture, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cache.bytes != readRetainedCaptureLimit {
		t.Fatalf("retained charge = %d", cache.bytes)
	}
	capture.Sections["padding"][1] = json.RawMessage(`"` + padding + `p"`)
	if _, err := cache.store(1, capture, 1); err == nil || !strings.Contains(err.Error(), "too_large") {
		t.Fatalf("combined overflow: %v", err)
	}
	if len(cache.entries) != 1 || cache.entries[env.CaptureID].size != readRetainedCaptureLimit {
		t.Fatal("rejected admission evicted data")
	}
	capture.Sections["padding"][1] = json.RawMessage(`"` + padding + `"`)
	now = now.Add(time.Second)
	second, err := cache.store(1, capture, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cache.bytes != readCacheLimit || len(cache.entries) != 2 {
		t.Fatalf("exact cache boundary: %d %d", cache.bytes, len(cache.entries))
	}
	now = now.Add(time.Second)
	third, err := cache.store(1, readCapture{Kind: "period_analysis", Summary: "", drilldown: small}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.entries[env.CaptureID]; ok {
		t.Fatal("oldest capture not evicted by byte budget")
	}
	if _, ok := cache.entries[second.CaptureID]; !ok {
		t.Fatal("wrong capture evicted")
	}
	if cache.bytes > readCacheLimit || len(cache.entries) != 2 {
		t.Fatal("cache budget exceeded")
	}
	stored := cache.entries[third.CaptureID]
	item, err := cache.periodItem(third.CaptureID, stored, ReadItemInput{Section: "returnDays", ID: "2024-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	cursor := *item.Sections["returnDays/2024-01-01/contributors"].Page.NextCursor
	page, err := cache.page(1, ReadPageInput{CaptureID: third.CaptureID, Section: "returnDays/2024-01-01/contributors", Cursor: cursor, Limit: 1})
	if err != nil || page.ExpiresAt != third.ExpiresAt || page.ContentHash != third.ContentHash || len(page.Items) != 1 {
		t.Fatalf("frozen page: %+v %v", page, err)
	}
	now = now.Add(readTTL)
	if _, err := cache.page(1, ReadPageInput{CaptureID: third.CaptureID, Section: "returnDays/2024-01-01/contributors", Cursor: cursor}); err == nil {
		t.Fatal("expired cursor accepted")
	}
	if cache.bytes != 0 || len(cache.entries) != 0 {
		t.Fatal("expiration failed to release charge")
	}
}

func TestPeriodDrilldownConcurrentAdmission(t *testing.T) {
	cache := newBoundedReadCache()
	cache.activate(1)
	drill := syntheticPeriodDetails(t, 16, 32, 4096)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			env, err := cache.store(1, readCapture{Kind: "period_analysis", drilldown: drill}, 1)
			if err != nil {
				t.Error(err)
				return
			}
			// Eviction may win this race; an accepted read must remain frozen.
			cache.mu.Lock()
			entry, err := cache.entry(1, env.CaptureID)
			var item ReadEnvelope
			if err == nil {
				item, err = cache.periodItem(env.CaptureID, entry, ReadItemInput{Section: "returnDays", ID: "2024-01-01"})
			}
			cache.mu.Unlock()
			if err == nil && (item.ContentHash != env.ContentHash || item.ExpiresAt != env.ExpiresAt) {
				t.Error("identity changed")
			}
		}()
	}
	wg.Wait()
	cache.mu.Lock()
	total := 0
	for _, e := range cache.entries {
		total += e.size
	}
	if cache.bytes != total || total > readCacheLimit || len(cache.entries) > readCacheCount {
		t.Errorf("bad accounting: %d %d %d", cache.bytes, total, len(cache.entries))
	}
	cache.mu.Unlock()
	cache.revoke()
	if cache.bytes != 0 || len(cache.entries) != 0 {
		t.Fatal("revoke retained charges")
	}
}
