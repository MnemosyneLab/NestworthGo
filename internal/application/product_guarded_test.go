package application

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestProductGuardedReviewedTimeIsPrivateAndDoesNotMutateCaller(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	s, ctx := newProductTestService(t, now)
	account := seedHoldingsCash(t, s, ctx, "100")
	s.SetClock(func() time.Time { return now })
	command := ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{AccountID: account.String(), Currency: "USD", Principal: "1000", TotalCostBasis: "950", CurrentValue: "1000", CashExcludesProduct: true, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Existing", StartOn: "2026-09-01", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	preview, token, err := s.PreviewProductOperationGuarded(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if command.ReviewedAt != "" || command.RecordExisting.EffectiveAt != "" {
		t.Fatal("caller command was modified", command)
	}
	var frozen ProductCommand
	if err := json.Unmarshal([]byte(preview.NormalizedJSON), &frozen); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewProductOperation(ctx, frozen); err == nil {
		t.Fatal("ordinary preview accepted server-owned time")
	}
	if _, err := s.RecordProductOperation(ctx, frozen, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err == nil {
		t.Fatal("ordinary commit accepted server-owned time")
	}
	now = now.Add(time.Minute)
	id := domain.NewProductOperationID().String()
	receipt, err := s.RecordProductOperationGuarded(ctx, frozen, id, preview.ReviewedStateHash, token)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := s.Activity(ctx, receipt.ActivityIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !activity.EffectiveAt.Equal(preview.EffectiveAt) || !activity.CreatedAt.Equal(now) {
		t.Fatal(activity)
	}
	replay, err := s.RecordProductOperationGuarded(ctx, frozen, id, preview.ReviewedStateHash, "expired")
	if err != nil || !replay.Replayed || replay.ActivityIDs[0] != receipt.ActivityIDs[0] {
		t.Fatal(replay, err)
	}
}
