package domain

import (
	"reflect"
	"testing"
	"time"
)

func reviewSyntheticDays(sequence string) map[string]ReviewTruth {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	days := map[string]ReviewTruth{}
	for i, v := range sequence {
		truth := ReviewUnknown
		if v == 'T' {
			truth = ReviewTrue
		} else if v == 'F' {
			truth = ReviewFalse
		}
		days[start.AddDate(0, 0, i).Format(time.DateOnly)] = truth
	}
	return days
}

func TestReviewDailyGoldenWindows(t *testing.T) {
	for _, tc := range []struct {
		name, sequence                   string
		policy                           ReviewTriggerPolicy
		want                             ReviewTruth
		hits, falseDays, unknown, suffix int
	}{
		{"D01_single", "T", ReviewTriggerPolicy{Kind: "single"}, ReviewTrue, 1, 0, 0, 1},
		{"single_unknown", "U", ReviewTriggerPolicy{Kind: "single"}, ReviewUnknown, 0, 0, 1, 0},
		{"D02_three_dates", "TTT", ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, ReviewTrue, 3, 0, 0, 3},
		{"D04_gap", "TUT", ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, ReviewUnknown, 2, 0, 1, 1},
		{"D04_after_gap", "TUTTT", ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, ReviewTrue, 3, 0, 0, 3},
		{"D05_false_gap", "TFT", ReviewTriggerPolicy{Kind: "consecutive", Count: 3}, ReviewFalse, 2, 1, 0, 1},
		{"D06_sufficient_known", "TTTTTUUUUU", ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5}, ReviewTrue, 5, 0, 5, 0},
		{"D07_impossible", "TTTTFFFFFF", ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5}, ReviewFalse, 4, 6, 0, 0},
		{"D07_uncertain", "TTTTUFFFFF", ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5}, ReviewUnknown, 4, 5, 1, 0},
		{"D28_false_does_not_cancel_hits", "TTTTTFFFFF", ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5}, ReviewTrue, 5, 5, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end := time.Date(2026, 10, len(tc.sequence), 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
			got, err := EvaluateReviewPolicy(tc.policy, end, reviewSyntheticDays(tc.sequence))
			if err != nil {
				t.Fatal(err)
			}
			if got.Truth != tc.want || got.HitCount != tc.hits || got.FalseCount != tc.falseDays || got.UnknownCount != tc.unknown || got.ConsecutiveCount != tc.suffix {
				t.Fatalf("got %+v; want truth=%q H/F/U=%d/%d/%d suffix=%d", got, tc.want, tc.hits, tc.falseDays, tc.unknown, tc.suffix)
			}
		})
	}
}

func TestReviewPolicyTimelineMatchesDateKeyedEvaluation(t *testing.T) {
	for _, policy := range []ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 3}, {Kind: "rolling", WindowDays: 10, MinHits: 5}} {
		for _, sequence := range []string{"TTTTTFFFFF", "TUTTTFTTTU", "UUUTTTTTTT", "TFTFTFTFTF"} {
			truths := reviewSyntheticDays(sequence)
			if sequence == "TUTTTFTTTU" {
				delete(truths, "2026-10-04") // a missing slot is U, not a skipped day
			}
			days := make([]ReviewDailyResult, 0, len(truths))
			for index := 0; index < len(sequence); index++ {
				on := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index).Format(time.DateOnly)
				truth, present := truths[on]
				if present {
					days = append(days, ReviewDailyResult{On: on, Truth: truth})
				}
			}
			got, err := EvaluateReviewPolicyTimeline(policy, days)
			if err != nil {
				t.Fatalf("timeline %+v %s: %v", policy, sequence, err)
			}
			for _, day := range days {
				want, err := EvaluateReviewPolicy(policy, day.On, truths)
				if err != nil || got[day.On] != want {
					t.Fatalf("timeline %+v %s %s: %+v, want %+v (%v)", policy, sequence, day.On, got[day.On], want, err)
				}
			}
		}
	}
}

func TestReviewPolicyTimelineHandlesLongDailySuffixInOnePass(t *testing.T) {
	days := make([]ReviewDailyResult, 3650)
	start := time.Date(2016, 10, 1, 0, 0, 0, 0, time.UTC)
	for index := range days {
		days[index] = ReviewDailyResult{On: start.AddDate(0, 0, index).Format(time.DateOnly), Truth: ReviewTrue}
	}
	windows, err := EvaluateReviewPolicyTimeline(ReviewTriggerPolicy{Kind: "rolling", WindowDays: 365, MinHits: 365}, days)
	if err != nil {
		t.Fatal(err)
	}
	latest := windows[days[len(days)-1].On]
	if latest.Truth != ReviewTrue || latest.HitCount != 365 || latest.ConsecutiveCount != len(days) {
		t.Fatalf("long true suffix was truncated: %+v", latest)
	}
}

var reviewPolicyBenchmarkSink map[string]ReviewPolicyWindow

func BenchmarkReviewPolicyTimeline365CalendarDays(b *testing.B) {
	days := make([]ReviewDailyResult, 365)
	start := time.Date(2025, 10, 16, 0, 0, 0, 0, time.UTC)
	for index := range days {
		truth := ReviewTrue
		if index%7 == 0 {
			truth = ReviewUnknown
		} else if index%5 == 0 {
			truth = ReviewFalse
		}
		days[index] = ReviewDailyResult{On: start.AddDate(0, 0, index).Format(time.DateOnly), Truth: truth}
	}
	policy := ReviewTriggerPolicy{Kind: "rolling", WindowDays: 30, MinHits: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		windows, err := EvaluateReviewPolicyTimeline(policy, days)
		if err != nil {
			b.Fatal(err)
		}
		reviewPolicyBenchmarkSink = windows
	}
}

func TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes(t *testing.T) {
	p := ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5}
	days := reviewSyntheticDays("TTTTTFFFFF")
	first, err := EvaluateReviewPolicy(p, "2026-10-10", days)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		days["2026-10-01"] = ReviewTrue // another refresh/revision of the same date
		got, err := EvaluateReviewPolicy(p, "2026-10-10", days)
		if err != nil || !reflect.DeepEqual(first, got) {
			t.Fatalf("refresh %d: %+v, %v", i, got, err)
		}
	}
	got, err := EvaluateReviewPolicy(p, "2026-10-11", days)
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowStart != "2026-10-02" || got.WindowEnd != "2026-10-11" || got.HitCount != 4 || got.UnknownCount != 1 || got.Truth != ReviewUnknown {
		t.Fatalf("D08: expired hit must leave window even without a new quote: %+v", got)
	}
	days["2026-10-11"] = ReviewTrue // late economic-day evidence replaces unknown
	got, _ = EvaluateReviewPolicy(p, "2026-10-11", days)
	if got.HitCount != 5 || got.UnknownCount != 0 || got.Truth != ReviewTrue {
		t.Fatalf("D24: %+v", got)
	}
	days["2026-10-11"] = ReviewFalse // correction replaces a vote rather than adding one
	got, _ = EvaluateReviewPolicy(p, "2026-10-11", days)
	if got.HitCount != 4 || got.FalseCount != 6 || got.Truth != ReviewFalse {
		t.Fatalf("D24 correction: %+v", got)
	}
}

func TestReviewConsecutiveCountIsFullTrailingSuffix(t *testing.T) {
	policy := ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
	for _, tc := range []struct {
		name, sequence string
		want           int
	}{
		{"six_hits", "TTTTTT", 6},
		{"false_then_six", "FTTTTTT", 6},
		{"unknown_then_six", "UTTTTTT", 6},
		{"later_unknown_breaks", "TTUTTT", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end := time.Date(2026, 10, len(tc.sequence), 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
			got, err := EvaluateReviewPolicy(policy, end, reviewSyntheticDays(tc.sequence))
			if err != nil || got.Truth != ReviewTrue || got.ConsecutiveCount != tc.want || got.HitCount != 3 || got.FalseCount != 0 || got.UnknownCount != 0 {
				t.Fatalf("full suffix and window counts: %+v %v", got, err)
			}
		})
	}
	missing := reviewSyntheticDays("TTTTTT")
	delete(missing, "2026-10-03")
	got, err := EvaluateReviewPolicy(policy, "2026-10-06", missing)
	if err != nil || got.ConsecutiveCount != 3 || got.Truth != ReviewTrue {
		t.Fatalf("missing day must interrupt suffix: %+v %v", got, err)
	}
	// A correction replaces one economic day's vote; it cannot add a second.
	missing["2026-10-03"] = ReviewTrue
	got, err = EvaluateReviewPolicy(policy, "2026-10-06", missing)
	if err != nil || got.ConsecutiveCount != 6 || got.HitCount != 3 {
		t.Fatalf("backfilled date must restore six-day suffix without an extra vote: %+v %v", got, err)
	}
}

func TestReviewDailyBooleanGate(t *testing.T) {
	values := []ReviewTruth{ReviewTrue, ReviewFalse, ReviewUnknown}
	wantAll := [][]ReviewTruth{{ReviewTrue, ReviewFalse, ReviewUnknown}, {ReviewFalse, ReviewFalse, ReviewFalse}, {ReviewUnknown, ReviewFalse, ReviewUnknown}}
	wantAny := [][]ReviewTruth{{ReviewTrue, ReviewTrue, ReviewTrue}, {ReviewTrue, ReviewFalse, ReviewUnknown}, {ReviewTrue, ReviewUnknown, ReviewUnknown}}
	for i, a := range values {
		for j, b := range values {
			if ReviewAll(a, b) != wantAll[i][j] || ReviewAny(a, b) != wantAny[i][j] {
				t.Fatalf("truth table %q,%q", a, b)
			}
		}
	}
	// D10/D11/D13: performance alone cannot bypass the day's quantity or amount.
	if ReviewAll(ReviewFalse, ReviewAny(ReviewTrue, ReviewUnknown)) != ReviewFalse {
		t.Fatal("zero historical quantity must veto a matching OR")
	}
	if ReviewAll(ReviewUnknown, ReviewTrue) != ReviewUnknown {
		t.Fatal("unknown historical quantity must not become a hit")
	}
	if ReviewAll(ReviewTrue, ReviewFalse, ReviewTrue) != ReviewFalse {
		t.Fatal("amount must match on the same day as performance")
	}
}

func TestReviewPolicyValidationAndNormalization(t *testing.T) {
	for _, p := range []ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 1}, {Kind: "rolling", WindowDays: 1, MinHits: 1}} {
		got, err := p.Normalize()
		if err != nil || got != (ReviewTriggerPolicy{Kind: "single"}) {
			t.Fatalf("normalize %+v: %+v %v", p, got, err)
		}
	}
	for _, p := range []ReviewTriggerPolicy{{}, {Kind: "single", Count: 1}, {Kind: "consecutive"}, {Kind: "consecutive", Count: 366}, {Kind: "consecutive", Count: 2, MinHits: 1}, {Kind: "rolling", WindowDays: 10, MinHits: 11}, {Kind: "rolling", WindowDays: 10}, {Kind: "rolling", WindowDays: 366, MinHits: 1}} {
		if _, err := p.Normalize(); err == nil {
			t.Fatalf("accepted invalid %+v", p)
		}
	}
	p := ReviewTriggerPolicy{Kind: "single"}
	if _, err := EvaluateReviewPolicy(p, "2026-02-30", nil); err == nil {
		t.Fatal("accepted invalid date")
	}
	if _, err := EvaluateReviewPolicy(p, "2026-10-01", map[string]ReviewTruth{"2026-10-01": "truthy"}); err == nil {
		t.Fatal("accepted invalid truth")
	}
	// A preview outside the window cannot be counted or invalidate the result.
	got, err := EvaluateReviewPolicy(p, "2026-10-01", map[string]ReviewTruth{"2026-10-02": ReviewTrue})
	if err != nil || got.Truth != ReviewUnknown {
		t.Fatalf("preview leaked: %+v %v", got, err)
	}
}

// Exhaust every three-day evidence sequence and its possible completions.
// A policy is true iff all completions trigger, false iff none trigger.
func TestReviewThreeValuedPoliciesAgreeWithAllCompletions(t *testing.T) {
	for _, p := range []ReviewTriggerPolicy{{Kind: "consecutive", Count: 3}, {Kind: "rolling", WindowDays: 3, MinHits: 2}} {
		for code := 0; code < 27; code++ {
			n := code
			sequence := make([]byte, 3)
			for i := range sequence {
				sequence[i] = "TFU"[n%3]
				n /= 3
			}
			anyTrue, anyFalse := false, false
			for completion := 0; completion < 8; completion++ {
				hits := 0
				for i, v := range sequence {
					if v == 'T' || (v == 'U' && completion&(1<<i) != 0) {
						hits++
					}
				}
				required := 2
				if p.Kind == "consecutive" {
					required = 3
				}
				if hits >= required {
					anyTrue = true
				} else {
					anyFalse = true
				}
			}
			want := ReviewUnknown
			if !anyFalse {
				want = ReviewTrue
			} else if !anyTrue {
				want = ReviewFalse
			}
			got, err := EvaluateReviewPolicy(p, "2026-10-03", reviewSyntheticDays(string(sequence)))
			if err != nil || got.Truth != want {
				t.Fatalf("%+v %s: %+v %v, want %q", p, sequence, got, err, want)
			}
		}
	}
}
