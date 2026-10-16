package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry(t *testing.T) {
	s, db, _, now := overviewFixture(t)
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	if _, err := s.StartHistory(t.Context(), "America/New_York"); err != nil {
		t.Fatal(err)
	}
	id := string(domain.NewReviewSuppressionID())
	mutation := string(domain.NewMutationID())
	in := SetReviewSuppressionV2Input{SuppressionID: id, MutationID: mutation, InstrumentID: instrument.ID.String(), Mode: "until_date", Days: 7, ExpectedRevision: 0}
	created, err := s.SetReviewSuppressionV2(t.Context(), in)
	if err != nil || created.UntilOn != "2026-11-07" || created.Revision != 1 || created.Timezone != "America/New_York" {
		t.Fatalf("create: %+v %v", created, err)
	}
	*now = time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC)
	retried, err := s.SetReviewSuppressionV2(t.Context(), in)
	if err != nil || retried.UntilOn != created.UntilOn || retried.Revision != 1 {
		t.Fatalf("retry extended expiry: %+v %v", retried, err)
	}
	in.MutationID = string(domain.NewMutationID())
	in.Days = 30
	in.ExpectedRevision = 1
	updated, err := s.SetReviewSuppressionV2(t.Context(), in)
	if err != nil || updated.UntilOn != "2026-12-08" || updated.Revision != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	list, err := s.ListReviewSuppressionsV2(t.Context(), "", 100)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("list: %+v %v", list, err)
	}
	in.MutationID = string(domain.NewMutationID())
	in.ExpectedRevision = 1
	if _, err := s.SetReviewSuppressionV2(t.Context(), in); err == nil {
		t.Fatal("stale revision overwrote ignore")
	}
	clearMutation := string(domain.NewMutationID())
	if err := s.ClearReviewSuppressionV2(t.Context(), id, clearMutation, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearReviewSuppressionV2(t.Context(), id, clearMutation, 2); err != nil {
		t.Fatalf("clear retry: %v", err)
	}
	list, err = s.ListReviewSuppressionsV2(t.Context(), "", 100)
	if err != nil || len(list) != 0 {
		t.Fatalf("clear: %+v %v", list, err)
	}
	var revision int
	if err := db.SQL.QueryRow(`SELECT review_state_revision FROM investment_review_control`).Scan(&revision); err != nil || revision != 3 {
		t.Fatalf("state revision=%d %v", revision, err)
	}
}

func TestReviewSuppressionV2CustomRetryAfterExpiry(t *testing.T) {
	s, _, _, now := overviewFixture(t)
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	in := SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-10"}
	first, err := s.SetReviewSuppressionV2(t.Context(), in)
	if err != nil || first.UntilOn != "2026-10-10" {
		t.Fatalf("custom: %+v %v", first, err)
	}
	*now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	again, err := s.SetReviewSuppressionV2(t.Context(), in)
	if err != nil || again.UntilOn != first.UntilOn {
		t.Fatalf("expired retry: %+v %v", again, err)
	}
	in.MutationID = string(domain.NewMutationID())
	in.SuppressionID = string(domain.NewReviewSuppressionID())
	if _, err := s.SetReviewSuppressionV2(t.Context(), in); err == nil {
		t.Fatal("new operation accepted a past date")
	}
}
