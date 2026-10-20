//go:build perf

package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"os"
	"strings"
	"testing"
)

func TestReviewMemoBudgetLocal(t *testing.T) {
	if os.Getenv("NW_D2_MEMO_GUARDS") != "1" {
		t.Skip("opt-in memo memory bounds")
	}
	target := &reviewMemoReadTarget{entries: map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}}
	target.add(reviewMemoEntry{Input: strings.Repeat("x", reviewPrefixMemoMaxPayload+1)})
	if !target.disabled || target.entries != nil {
		t.Fatal("payload retention limit not enforced")
	}
	target = &reviewMemoReadTarget{entries: map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}}
	for n := 0; n <= reviewPrefixMemoMaxEntries; n++ {
		target.add(reviewMemoEntry{Key: domain.ReviewDailyEvidenceKey{ConditionVersion: n}})
	}
	if !target.disabled || target.entries != nil {
		t.Fatal("entry retention limit not enforced")
	}
}
