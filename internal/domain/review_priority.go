package domain

// NormalizeReviewPriority accepts the two historical API aliases. Persistence
// keeps those columns intact and stores explicit three-level priority separately.
func NormalizeReviewPriority(value string) (string, bool) {
	switch value {
	case "p0":
		return "p0", true
	case "p1", "warning":
		return "p1", true
	case "p2", "notice":
		return "p2", true
	default:
		return "", false
	}
}

func ReviewPriorityRank(value string) int {
	priority, _ := NormalizeReviewPriority(value)
	switch priority {
	case "p0":
		return 0
	case "p1":
		return 1
	default:
		return 2
	}
}

func ReviewLegacySeverity(value string) string {
	if ReviewPriorityRank(value) < 2 {
		return "warning"
	}
	return "notice"
}
