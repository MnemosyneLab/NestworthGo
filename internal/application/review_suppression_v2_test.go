package application

import (
	"sync"
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

func TestReviewSuppressionV2CanBeSetAgainAfterExpiryConcurrently(t *testing.T) {
	s, _, _, now := overviewFixture(t)
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	first, err := s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{
		SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()),
		InstrumentID: instrument.ID.String(), Mode: "until_date", Days: 7,
	})
	if err != nil || first.UntilOn != "2026-10-08" || first.Revision != 1 {
		t.Fatalf("initial ignore: %+v %v", first, err)
	}
	*now = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if _, err := s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{
		SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()),
		InstrumentID: instrument.ID.String(), Mode: "until_date", Days: 30,
	}); err == nil {
		t.Fatal("new request silently extended an active ignore")
	}

	*now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	active, err := domain.ActiveReviewSuppression([]domain.ReviewSuppression{first}, instrument.ID, "", *now)
	if err != nil || active != nil {
		t.Fatalf("ignore should have expired on its resume date: %+v %v", active, err)
	}
	makeRequest := func() SetReviewSuppressionV2Input {
		return SetReviewSuppressionV2Input{
			SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()),
			InstrumentID: instrument.ID.String(), Mode: "until_date", Days: 7,
		}
	}
	requests := []SetReviewSuppressionV2Input{makeRequest(), makeRequest()}
	results := make([]domain.ReviewSuppression, len(requests))
	resultErrors := make([]error, len(requests))
	var wait sync.WaitGroup
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], resultErrors[index] = s.SetReviewSuppressionV2(t.Context(), requests[index])
		}(index)
	}
	wait.Wait()
	for index, err := range resultErrors {
		if err != nil {
			t.Fatalf("concurrent re-ignore %d: %v", index, err)
		}
	}
	for index, result := range results {
		if result.ID != first.ID || result.UntilOn != "2026-10-15" || result.Revision != 2 {
			t.Fatalf("re-ignore %d did not update the expired scope: %+v", index, result)
		}
	}
	entries, err := s.ListReviewSuppressionsV2(t.Context(), "", 10)
	if err != nil || len(entries) != 1 || entries[0].ID != first.ID || entries[0].UntilOn != "2026-10-15" || entries[0].Revision != 2 {
		t.Fatalf("concurrent re-ignore created or changed multiple rows: %+v %v", entries, err)
	}
}

func TestReviewSuppressionV2ExpiresAtHouseholdCivilDateWithFakeClock(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "Asia/Singapore", "2026-10-01", "1")
	f.day(t, "2026-10-02")
	spec := syntheticHistoricalReviewSpec()
	spec.Schedule.Timezone = "Asia/Singapore"
	rule := saveSyntheticReviewRule(t, f, "Synthetic suppression expiry", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "pending"}, "", 10)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Rules) != 1 || page.Items[0].Rules[0].RuleID != rule.ID {
		t.Fatalf("synthetic alert: %+v %v", page, err)
	}
	episode := page.Items[0].Rules[0]
	if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{
		SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()),
		InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-03",
	}); err != nil {
		t.Fatal(err)
	}

	// Singapore is UTC+8: the resume civil date starts at 16:00 UTC on Oct 2.
	f.s.SetClock(func() time.Time { return time.Date(2026, 10, 2, 15, 59, 59, 0, time.UTC) })
	ignored, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "ignored"}, "", 10)
	if err != nil || len(ignored.Items) != 1 || ignored.Items[0].Rules[0].SuppressionID == "" || ignored.Items[0].Rules[0].Actionable {
		t.Fatalf("still ignored immediately before local expiry: %+v %v", ignored, err)
	}

	f.s.SetClock(func() time.Time { return time.Date(2026, 10, 2, 16, 0, 0, 0, time.UTC) })
	pending, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "pending"}, "", 10)
	if err != nil || len(pending.Items) != 1 || pending.Items[0].Rules[0].EpisodeID != episode.EpisodeID || !pending.Items[0].Rules[0].Actionable {
		t.Fatalf("alert did not resume at Singapore midnight: %+v %v", pending, err)
	}
}
