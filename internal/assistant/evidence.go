package assistant

import (
	"regexp"
	"time"
)

// ReadEvidence is a host trace, separate from model-authored prose. Missingness
// is deliberately conservative: absence of a summary never proves complete data.
// It contains no raw tool arguments, names, amounts, notes or provider internals.
type ReadEvidence struct {
	Source     string    `json:"source"`
	CapturedAt time.Time `json:"capturedAt"`
	HasMore    bool      `json:"hasMore"`
	Missing    string    `json:"missing"` // reported / not_summarized
}

var evidenceSource = regexp.MustCompile(`^[a-z_]{1,64}$`)

func validEvidence(items []ReadEvidence) bool {
	if len(items) > 16 {
		return false
	}
	for _, e := range items {
		if !evidenceSource.MatchString(e.Source) || e.CapturedAt.IsZero() || (e.Missing != "reported" && e.Missing != "not_summarized") {
			return false
		}
	}
	return true
}
