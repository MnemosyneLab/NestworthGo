package sqlite

import "github.com/waltwang/nestworth-go/internal/domain"

// reviewEpisodeEvidence stores the current rolling window alongside the
// immutable opening window. The latter anchors first-hit metadata and dated
// evidence pagination as the current window advances.
type reviewEpisodeEvidence struct {
	Window          domain.ReviewPolicyWindow
	TriggerWindow   domain.ReviewPolicyWindow
	FirstHitOn      string
	TriggerResult   *domain.ReviewDailyResult
	TriggerCoverage []string
	LatestResult    *domain.ReviewDailyResult
	Coverage        []string
}
