package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewMemoBudget(t *testing.T) {
	target := &reviewMemoReadTarget{entries: map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}}
	target.payload = reviewPrefixMemoMaxPayload
	target.add(reviewMemoEntry{Input: "x"})
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
