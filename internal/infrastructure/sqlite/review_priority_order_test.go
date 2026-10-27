package sqlite

import "testing"

func TestReviewAlertPriorityOrder(t *testing.T) {
	for _, tc := range []struct {
		a, b   string
		ap, bp int
		ai, bi string
	}{
		{"pending", "pending", 0, 1, "z", "a"},
		{"pending", "pending", 1, 2, "z", "a"},
		{"pending", "acknowledged", 2, 0, "z", "a"},
		{"unknown", "ignored", 2, 0, "z", "a"},
		{"pending", "pending", 0, 0, "a", "b"},
	} {
		if !reviewAlertOrderLess(tc.a, tc.ap, tc.ai, tc.b, tc.bp, tc.bi) || reviewAlertOrderLess(tc.b, tc.bp, tc.bi, tc.a, tc.ap, tc.ai) {
			t.Fatalf("incorrect order %+v", tc)
		}
	}
}
