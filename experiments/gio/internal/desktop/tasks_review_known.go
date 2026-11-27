package desktop

import (
	"context"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type taskReviewEvidenceReader interface {
	ListReviewDailyEvidenceV2(context.Context, string, string, int) (domain.ReviewDailyEvidencePage, error)
}

type taskReviewKnown struct {
	result *domain.ReviewDailyResult
	failed bool
}

func readTaskReviewKnown(ctx context.Context, service taskChecksReader, page domain.ReviewAlertPage) map[domain.ReviewEpisodeID]taskReviewKnown {
	reader, ok := service.(taskReviewEvidenceReader)
	if !ok {
		return nil
	}
	var results map[domain.ReviewEpisodeID]taskReviewKnown
	for _, group := range page.Items {
		for _, rule := range group.Rules {
			if ctx.Err() != nil {
				return results
			}
			if !taskReviewVisible(rule) || reviewAssessmentKnown(rule.LatestResult) {
				continue
			}
			evidence, err := reader.ListReviewDailyEvidenceV2(ctx, string(rule.EpisodeID), "", 1)
			read := taskReviewKnown{failed: err != nil || !taskReviewEvidenceMatches(rule, evidence)}
			if !read.failed {
				read.result = evidence.LatestKnownResult
			}
			if results == nil {
				results = make(map[domain.ReviewEpisodeID]taskReviewKnown)
			}
			results[rule.EpisodeID] = read
		}
	}
	return results
}

// A pending investment can also have an unresolved rule waiting for evidence.
// Keep that rule visible without making it acknowledgeable.
func taskReviewVisible(rule domain.ReviewAlertRule) bool {
	return rule.Actionable || rule.Status == "unknown" && rule.AcknowledgedAt == nil
}

func reviewAssessmentKnown(result *domain.ReviewDailyResult) bool {
	return result != nil && (result.Truth == domain.ReviewTrue || result.Truth == domain.ReviewFalse)
}

func taskReviewEvidenceMatches(rule domain.ReviewAlertRule, page domain.ReviewDailyEvidencePage) bool {
	if page.EpisodeID != rule.EpisodeID || page.EpisodeRevision != rule.EpisodeRevision {
		return false
	}
	latest := rule.LatestClosedOn
	if result := rule.LatestResult; result != nil {
		// Separate reads must still describe the same latest vote. The two
		// APIs' report revisions are minted for different scopes.
		if len(page.Items) == 0 || page.Items[0].On != result.On || page.Items[0].Truth != result.Truth {
			return false
		}
		latest = result.On
	}
	if known := page.LatestKnownResult; known != nil {
		_, valid := ui.ParseCivilDay(known.On)
		return valid && reviewAssessmentKnown(known) && known.On <= latest
	}
	return true
}
