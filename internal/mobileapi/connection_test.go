package mobileapi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestSafeJobDetails(t *testing.T) {
	job := application.MobileMarketJob{Items: []application.SyncItemProgress{{Label: "Synthetic target", Status: "failed", Detail: "provider_authentication"}, {Label: "Synthetic target 2", Status: "failed", Detail: "secret-path-provider-message"}}, Blockers: []application.SyncBlocker{{Code: "provider_rate_limit", Reason: "raw-provider-secret"}, {Code: "raw-secret"}}}
	raw, err := jsoncodec.Marshal(mobileJobDTO(job, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret")) || !bytes.Contains(raw, []byte("provider_authentication")) || !bytes.Contains(raw, []byte("provider_rate_limit")) || bytes.Contains(raw, []byte("target 2")) {
		t.Fatalf("unsafe or unbounded details %s", raw)
	}
}

func TestReadOnlySessionCannotCancelProviderWork(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	response, _ := f.request(t, http.MethodPost, "/market-jobs/synthetic/cancel", struct{}{}, nil)
	if response.StatusCode != 403 {
		t.Fatalf("cancellation accepted: %d", response.StatusCode)
	}
}

func TestJobCursorFromCurrentPinsResolvedJobAcrossNavigation(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Market: true})
	for _, name := range []string{"Synthetic A", "Synthetic B"} {
		if _, err := f.s.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: name, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	start := func() string {
		t.Helper()
		job, _, conflict, err := f.s.app.StartMobileMarket(t.Context(), application.MobileMarketRequest{Mode: "latest", Scope: "all"})
		if err != nil || conflict {
			t.Fatalf("start: %v %v", conflict, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			current, _ := f.s.app.MobileMarketJob(job.ID)
			if current.Outcome != application.SyncOutcomeRunning {
				return job.ID
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("synthetic local task did not finish")
		return ""
	}
	id := start()
	response, raw := f.request(t, "GET", "/market-jobs/current?limit=1", nil, nil)
	var page struct {
		Next string `json:"next"`
	}
	if err := jsoncodec.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || page.Next == "" {
		t.Fatalf("first page: %d %s", response.StatusCode, raw)
	}
	path := "/market-jobs/" + id + "?limit=1&cursor=" + url.QueryEscape(page.Next)
	response, raw = f.request(t, "GET", path, nil, nil)
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte("Synthetic B")) {
		t.Fatalf("pinned page: %d %s", response.StatusCode, raw)
	}
	start()
	response, _ = f.request(t, "GET", path, nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("navigation changed the pinned task")
	}
	response, _ = f.request(t, "GET", "/market-jobs/current?limit=1&cursor="+url.QueryEscape(page.Next), nil, nil)
	if response.StatusCode != 409 {
		t.Fatal("old cursor attached to another current task")
	}
}

func TestJobDetailsPageEachCollectionFromTheOriginalOffset(t *testing.T) {
	for _, counts := range [][3]int{{0, 0, 65}, {0, 65, 65}, {5, 65, 40}, {65, 5, 0}} {
		t.Run(fmt.Sprint(counts), func(t *testing.T) {
			var job application.MobileMarketJob
			for i := range counts[0] {
				job.Items = append(job.Items, application.SyncItemProgress{Label: fmt.Sprintf("item-%02d", i)})
			}
			for i := range counts[1] {
				job.Result.Items = append(job.Result.Items, application.RefreshTargetResult{TargetKey: fmt.Sprintf("result-%02d", i)})
			}
			for i := range counts[2] {
				code := "provider_authentication"
				if i >= 30 {
					code = "provider_rate_limit"
				}
				if i >= 60 {
					code = "unavailable"
				}
				job.Blockers = append(job.Blockers, application.SyncBlocker{Code: code})
			}
			for _, offset := range []int{0, 30, 60, 90} {
				page := mobileJobDTO(job, offset, 30)
				items := page["items"].([]map[string]string)
				results := page["result"].(application.RefreshResult).Items
				blockers := page["blockers"].([]string)
				if len(items) != min(30, max(0, counts[0]-offset)) || len(results) != min(30, max(0, counts[1]-offset)) || len(blockers) != min(30, max(0, counts[2]-offset)) {
					t.Fatalf("offset %d repeated or lost rows: items=%d results=%d blockers=%d", offset, len(items), len(results), len(blockers))
				}
				if len(items) > 0 && items[0]["label"] != fmt.Sprintf("item-%02d", offset) {
					t.Fatalf("wrong item page %d", offset)
				}
				if len(results) > 0 && results[0].TargetKey != fmt.Sprintf("result-%02d", offset) {
					t.Fatalf("wrong result page %d: %+v", offset, results)
				}
				if len(blockers) > 0 && blockers[0] != job.Blockers[offset].Code {
					t.Fatalf("wrong blocker page %d: %v", offset, blockers)
				}
			}
		})
	}
}
