package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wails "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type taskReviewResponseLoss struct {
	*application.Service
	called   atomic.Bool
	requests chan wails.AcknowledgeReviewEpisodeV2Request
	release  <-chan struct{}
}

func (s *taskReviewResponseLoss) AcknowledgeReviewEpisodeV2(ctx context.Context, episode, mutation string, revision int) (domain.ReviewAcknowledgement, error) {
	first := !s.called.Swap(true)
	s.requests <- wails.AcknowledgeReviewEpisodeV2Request{EpisodeID: episode, MutationID: mutation, ExpectedRevision: revision}
	if first {
		select {
		case <-s.release:
		case <-ctx.Done():
			return domain.ReviewAcknowledgement{}, ctx.Err()
		}
	}
	receipt, err := s.Service.AcknowledgeReviewEpisodeV2(ctx, episode, mutation, revision)
	if first && err == nil {
		return domain.ReviewAcknowledgement{}, errors.New("synthetic lost acknowledgement response")
	}
	return receipt, err
}

func TestTaskReviewAcknowledgementMatchesWails(t *testing.T) {
	const action = "Acknowledge · Review Stock · Original cost under 100 USD"
	app, storage, _, term := openManagedProductFixture(t)
	if _, err := seedTaskChecks(t.Context(), app, term, accountTestClock); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(storage.DB.Path)
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	open := func(t *testing.T, name string) *application.Service {
		t.Helper()
		path := writeStage6Fixture(t, filepath.Join(t.TempDir(), name), seed)
		_, _, service := openStage6Database(t, path, clock)
		return service
	}
	pending := func(t *testing.T, api *wails.Service) domain.ReviewAlertPage {
		t.Helper()
		page, err := api.ListReviewAlertsV2(t.Context(), wails.ListReviewAlertsV2Request{
			Filter: domain.ReviewAlertFilter{Status: "pending"}, PageSize: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	rule := func(t *testing.T, page domain.ReviewAlertPage) domain.ReviewAlertRule {
		t.Helper()
		if page.PublishedOn == "" || page.Summary.ActionableRules != 1 ||
			len(page.Items) != 1 || len(page.Items[0].Rules) != 1 || !page.Items[0].Rules[0].Actionable {
			t.Fatalf("expected one published actionable rule, got %+v", page)
		}
		return page.Items[0].Rules[0]
	}
	setup := func(t *testing.T, service *application.Service, reader taskChecksReader) (*previewHarness, *tasksPage, *overviewPage, func()) {
		t.Helper()
		tasks := newTasksPage(t.Context(), service, func() {})
		tasks.checks = reader
		overview := newOverviewPage(t.Context(), service, clock, func() {})
		// Test direct invalidation and a concurrent writer independently of the
		// in-process ledger version watcher.
		h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, overview: overview})
		h.shell.prefs.saved.Language = settings.LanguageEnglish
		h.size = image.Pt(1120, 1000)
		t.Cleanup(tasks.workers.Wait)
		t.Cleanup(overview.workers.Wait)
		settle := func() {
			for range 4 {
				h.frame()
				tasks.workers.Wait()
				overview.workers.Wait()
			}
			h.frame()
		}
		settle()
		overview.list.ScrollTo(2)
		settle()
		h.click("Investment review · 1", false)
		settle()
		assertPageLabel(t, &h.router, action)
		return h, tasks, overview, settle
	}
	ackEvent := func(t *testing.T, api *wails.Service, episode domain.ReviewEpisodeID) domain.ReviewEpisodeEvent {
		t.Helper()
		result, err := api.ListReviewEpisodeEventsV2(t.Context(), wails.ListReviewEpisodeEventsV2Request{
			EpisodeID: string(episode), PageSize: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		acks := slices.DeleteFunc(result.Items, func(event domain.ReviewEpisodeEvent) bool { return event.Kind != "acknowledged" })
		if len(acks) != 1 {
			t.Fatalf("expected exactly one acknowledgement event, got %+v", acks)
		}
		return acks[0]
	}

	t.Run("retry recovers the same receipt and both views reread", func(t *testing.T) {
		ctx := t.Context()
		gio := open(t, "gio")
		ref := wails.NewService(open(t, "wails"))
		want := rule(t, pending(t, ref))
		release := make(chan struct{})
		writer := &taskReviewResponseLoss{Service: gio, requests: make(chan wails.AcknowledgeReviewEpisodeV2Request, 2), release: release}
		h, tasks, overview, settle := setup(t, gio, writer)
		got := rule(t, *tasks.review)
		if got.EpisodeID != want.EpisodeID || got.EpisodeRevision != want.EpisodeRevision {
			t.Fatalf("Gio and Wails opened different episodes: %+v / %+v", got, want)
		}
		h.router.Source().Execute(key.FocusCmd{Tag: tasks.reviewAck.buttons[got.EpisodeID]})
		h.focused(tasks.reviewAck.buttons[got.EpisodeID])
		h.press(key.NameReturn)
		h.frame()
		if node := findPageLabel(&h.router, action); tasks.reviewAck.request == nil || node != nil && !node.Desc.Disabled {
			t.Fatal("the acknowledgement could be submitted again while the first request was pending")
		}
		h.focused(&tasks.tabs[tasksInvestment])
		assertPageLabel(t, &h.router, "Saving acknowledgement…")
		h.click("Back", false)
		if h.shell.activeRoute() != overviewRoute {
			t.Fatal("the user could not return to Overview while acknowledgement was pending")
		}
		close(release)
		settle()
		if overview.upcoming.summary.review == nil || *overview.upcoming.summary.review != 0 {
			t.Fatal("Overview's pending count did not reread independently of the ledger version")
		}
		h.click("Investment review · 0", false)
		settle()
		assertPageLabel(t, &h.router, "The acknowledgement could not be verified.")
		if findPageLabel(&h.router, action) != nil {
			t.Fatal("authoritative pending rows still contained the acknowledged episode")
		}
		h.click("Retry acknowledgement", false)
		settle()
		assertPageLabel(t, &h.router, "Acknowledgement saved.")
		if findPageLabel(&h.router, "Retry acknowledgement") != nil || tasks.reviewAck.intent != nil || len(writer.requests) != 2 {
			t.Fatal("a successful retry must finish exactly one pending acknowledgement")
		}
		first, retry := <-writer.requests, <-writer.requests
		if first != retry || first.EpisodeID != string(want.EpisodeID) || first.ExpectedRevision != want.EpisodeRevision {
			t.Fatalf("acknowledgement was retried with a different request: %+v / %+v", first, retry)
		}
		if _, err := domain.ParseMutationID(first.MutationID); err != nil {
			t.Fatal(err)
		}
		receipt, err := ref.AcknowledgeReviewEpisodeV2(ctx, first)
		if err != nil || receipt.EpisodeID != want.EpisodeID || receipt.Revision != want.EpisodeRevision+1 {
			t.Fatalf("Wails acknowledgement did not use the same identity and revision: %+v / %v", receipt, err)
		}
		wantPage := pending(t, ref)
		if tasks.review == nil || tasks.review.Summary != wantPage.Summary || len(tasks.review.Items) != 0 || len(wantPage.Items) != 0 {
			t.Fatalf("Gio and Wails pending review differ: %+v / %+v", tasks.review, wantPage)
		}
		event, reference := ackEvent(t, wails.NewService(gio), got.EpisodeID), ackEvent(t, ref, want.EpisodeID)
		if event.EpisodeID != reference.EpisodeID || event.PayloadJSON != reference.PayloadJSON || event.CreatedAt != reference.CreatedAt {
			t.Fatalf("Gio and Wails stored different acknowledgement evidence: %+v / %+v", event, reference)
		}
	})

	t.Run("a concurrent acknowledgement rejects the older visible revision", func(t *testing.T) {
		gio := open(t, "gio")
		api := wails.NewService(gio)
		h, tasks, _, settle := setup(t, gio, gio)
		before := rule(t, *tasks.review)
		if _, err := api.AcknowledgeReviewEpisodeV2(t.Context(), wails.AcknowledgeReviewEpisodeV2Request{
			EpisodeID: string(before.EpisodeID), ExpectedRevision: before.EpisodeRevision, MutationID: domain.NewMutationID().String(),
		}); err != nil {
			t.Fatal(err)
		}
		h.click(action, false)
		settle()
		assertPageLabel(t, &h.router, "The review alert changed. Check the latest results.")
		if tasks.review == nil || tasks.review.Summary.ActionableRules != 0 || tasks.reviewAck.intent != nil ||
			findPageLabel(&h.router, action) != nil || findPageLabel(&h.router, "Retry acknowledgement") != nil {
			t.Fatal("a revision conflict did not clear the action and reload current results")
		}
		ackEvent(t, api, before.EpisodeID)
	})
}
