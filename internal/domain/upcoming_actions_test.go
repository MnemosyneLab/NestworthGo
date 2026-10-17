package domain

import (
	"encoding/json"
	"github.com/shopspring/decimal"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func upcomingFixture(t *testing.T) (LiquidityQuery, LiquiditySource) {
	q, ss, _ := fixtureA(t)
	s := ss[2]
	q.AsOf = time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC)
	q.Timezone = "Asia/Shanghai"
	s.ExplicitPolicy.Source.Currency = nil
	s.Contract.MaturityOn = datePtr("2026-10-15")
	s.ExplicitPolicy.UnlockOn = datePtr("2026-10-15")
	return q, s
}
func projectUpcoming(t *testing.T, q LiquidityQuery, sources []LiquiditySource, rs []LiquidityReservation) UpcomingActions {
	t.Helper()
	q.IncludeEarlyWithdrawal = false
	o, e := EvaluateLiquidity(q, sources, rs, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	out, e := ProjectUpcomingActions(o, sources, LiquiditySnapshot{Reservations: rs})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestUpcomingSchedulePhases(t *testing.T) {
	cases := []struct {
		name    string
		m, u, r *string
		access  AccessKind
		days    *int
		today   string
		phase   string
		count   int
	}{
		{"today", datePtr("2026-10-11"), datePtr("2026-10-11"), nil, AccessOnDate, intPtr(2), "2026-10-11", "today", 1},
		{"end inclusive", datePtr("2026-11-10"), datePtr("2026-11-10"), nil, AccessOnDate, intPtr(2), "2026-10-11", "upcoming", 1},
		{"outside", datePtr("2026-11-11"), datePtr("2026-11-11"), nil, AccessOnDate, intPtr(2), "2026-10-11", "", 0},
		{"waiting", datePtr("2026-10-10"), datePtr("2026-10-10"), nil, AccessOnDate, intPtr(3), "2026-10-11", "awaiting_receipt", 1},
		{"waiting beyond E", datePtr("2026-10-10"), datePtr("2026-10-10"), datePtr("2027-01-10"), AccessOnDate, intPtr(3), "2026-10-11", "awaiting_receipt", 1},
		{"due today", datePtr("2026-10-09"), datePtr("2026-10-09"), nil, AccessOnDate, intPtr(2), "2026-10-11", "receipt_due", 1},
		{"overdue months", datePtr("2026-01-09"), datePtr("2026-01-09"), nil, AccessOnDate, intPtr(2), "2026-10-11", "receipt_due", 1},
		{"blocked overdue", datePtr("2026-01-09"), datePtr("2026-01-09"), nil, AccessOnDate, nil, "2026-10-11", "blocked", 1},
		{"blocked upcoming", datePtr("2026-10-15"), datePtr("2026-10-15"), nil, AccessUnknown, nil, "2026-10-11", "blocked", 1},
		{"unknown override", datePtr("2026-10-09"), nil, datePtr("2026-10-10"), AccessUnknown, intPtr(0), "2026-10-11", "blocked", 1},
		{"unknown no maturity override", nil, nil, datePtr("2026-10-10"), AccessUnknown, intPtr(0), "2026-10-11", "", 0},
		{"unlock rolling", nil, datePtr("2026-10-15"), nil, AccessOnDate, intPtr(2), "2026-10-11", "upcoming", 1},
		{"past unlock", nil, datePtr("2026-09-15"), nil, AccessOnDate, intPtr(2), "2026-10-11", "eligible", 1},
		{"unlock today override", nil, datePtr("2026-10-11"), datePtr("2026-10-19"), AccessOnDate, intPtr(2), "2026-10-11", "today", 1},
		{"past unlock override beyond E", nil, datePtr("2026-10-10"), datePtr("2027-01-10"), AccessOnDate, intPtr(2), "2026-10-11", "scheduled_receipt", 1},
		{"override only", nil, nil, datePtr("2026-11-10"), AccessOnRequest, intPtr(2), "2026-10-11", "scheduled_receipt", 1},
		{"override outside", nil, nil, datePtr("2026-11-11"), AccessOnRequest, intPtr(2), "2026-10-11", "", 0},
		{"override due", nil, nil, datePtr("2026-10-10"), AccessOnRequest, intPtr(2), "2026-10-11", "receipt_due", 1},
		{"maturity beats early unlock", datePtr("2026-11-10"), datePtr("2026-10-11"), nil, AccessOnRequest, intPtr(2), "2026-10-11", "upcoming", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, s := upcomingFixture(t)
			q.AsOf, _ = time.Parse(time.RFC3339, c.today+"T04:00:00Z")
			s.Contract.MaturityOn = c.m
			s.ExplicitPolicy.UnlockOn = c.u
			s.ExplicitPolicy.ReceiptOnOverride = c.r
			s.ExplicitPolicy.AccessKind = c.access
			s.ExplicitPolicy.SettlementDays = c.days
			got := projectUpcoming(t, q, []LiquiditySource{s}, nil)
			if len(got.Items) != c.count {
				t.Fatalf("items=%+v", got.Items)
			}
			if c.count == 1 && got.Items[0].Phase != c.phase {
				t.Fatalf("phase=%s want %s", got.Items[0].Phase, c.phase)
			}
			if got.Counts.Total != got.Counts.Attention+got.Counts.Scheduled {
				t.Fatal("attention and scheduled do not sum to total")
			}
			if c.name == "blocked overdue" && (got.Counts.Attention != 1 || got.Counts.Scheduled != 0) {
				t.Fatalf("past blocked item missing from attention: %+v", got.Counts)
			}
			if c.phase == "blocked" && got.Completeness != StatusPartial {
				t.Fatal("blocked data marked complete")
			}
		})
	}
}
func TestUpcomingScopeAndAmounts(t *testing.T) {
	for _, test := range []string{"excluded", "archived", "closed", "cancelled", "zero_quantity", "zero_amount", "unknown_amount", "unknown_fee", "missing_policy", "ordinary_default", "legacy_unlock", "not_net_worth"} {
		t.Run(test, func(t *testing.T) {
			q, s := upcomingFixture(t)
			want := 1
			switch test {
			case "excluded":
				s.ExplicitPolicy.AccessKind = AccessExcluded
				want = 0
			case "archived":
				s.Archived = true
				want = 0
			case "closed":
				s.Contract.State = ProductStateSettled
				want = 0
			case "cancelled":
				s.Contract.State = ProductStateCancelled
				want = 0
			case "zero_quantity":
				s.QuantityZero = true
				want = 0
			case "zero_amount":
				s.CurrentNativeValue = moneyPtr(t, "0", "USD")
			case "unknown_amount":
				s.CurrentNativeValue = nil
			case "unknown_fee":
				s.ExplicitPolicy.NormalExitFee = nil
			case "missing_policy":
				s.ExplicitPolicy = nil
			case "ordinary_default":
				s.Managed = false
				s.Contract = nil
				s.ProductID = nil
				s.ExplicitPolicy = nil
				want = 0
			case "legacy_unlock":
				s.Managed = false
				s.Contract = nil
				s.ProductID = nil
			case "not_net_worth":
				s.IncludeInNetWorth = false
			}
			got := projectUpcoming(t, q, []LiquiditySource{s}, nil)
			if len(got.Items) != want {
				t.Fatalf("items=%+v", got.Items)
			}
			if test == "excluded" && (got.Completeness != StatusComplete || len(got.DataChecks) != 1 || got.DataChecks[0].Category != "scope_note") {
				t.Fatalf("excluded=%+v", got)
			}
			if test == "unknown_amount" || test == "unknown_fee" {
				if got.Completeness != StatusPartial || got.Items[0].Amount != nil || got.Counts.DataChecks != 0 {
					t.Fatalf("unknown=%+v", got)
				}
			}
			if test == "zero_amount" && got.Items[0].Amount == nil {
				t.Fatal("known zero hidden")
			}
			if test == "legacy_unlock" && got.Items[0].PrimaryTarget.Kind != "source_policy" {
				t.Fatal("legacy product action")
			}
		})
	}
}
func TestUpcomingReservationCoverage(t *testing.T) {
	for _, test := range []struct {
		name, status, category string
		partial                bool
		short                  bool
	}{
		{"short", "short", "", false, true}, {"covered", "covered", "", false, false}, {"mismatch", "currency_mismatch", "data_issue", true, false},
		{"excluded", "excluded_source", "scope_note", false, false}, {"due", "due_route_suppressed", "information", false, false},
		{"late", "not_available_by_E", "information", false, false}, {"unknown", "amount_unknown", "data_issue", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			q, s := upcomingFixture(t)
			s.Managed = false
			s.Contract = nil
			s.ProductID = nil
			s.ExplicitPolicy.AccessKind = AccessOnRequest
			s.ExplicitPolicy.UnlockOn = nil
			s.ExplicitPolicy.NormalExitFee = moneyPtr(t, "0", "USD")
			rs := []LiquidityReservation{{ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "B", Amount: mustTestMoney(t, "4000", "USD"), Currency: "USD", Revision: 1}, {ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "A", Amount: mustTestMoney(t, "3000", "USD"), Currency: "USD", Revision: 1}}
			switch test.name {
			case "covered":
				s.CurrentNativeValue = moneyPtr(t, "9000", "USD")
			case "mismatch":
				for i := range rs {
					rs[i].Currency = "EUR"
					rs[i].Amount = mustTestMoney(t, "3000", "EUR")
				}
			case "excluded":
				s.ExplicitPolicy.AccessKind = AccessExcluded
			case "due":
				_, managed := upcomingFixture(t)
				s.Managed = true
				s.Contract = managed.Contract
				s.ProductID = managed.ProductID
				s.Contract.MaturityOn = datePtr("2026-10-09")
				s.ExplicitPolicy.UnlockOn = s.Contract.MaturityOn
			case "late":
				s.ExplicitPolicy.AccessKind = AccessOnDate
				s.ExplicitPolicy.UnlockOn = datePtr("2026-11-11")
			case "unknown":
				s.CurrentNativeValue = nil
			}
			got := projectUpcoming(t, q, []LiquiditySource{s}, rs)
			if (got.Completeness == StatusPartial) != test.partial {
				t.Fatalf("complete=%s checks=%+v items=%+v", got.Completeness, got.DataChecks, got.Items)
			}
			if test.short {
				if len(got.Items) != 1 || got.Items[0].CoverageStatus != test.status || got.Items[0].Amount.CanonicalAmount() != "2000" || len(got.Items[0].ReservationIDs) != 2 {
					t.Fatalf("short=%+v", got)
				}
			} else if test.status != "covered" {
				if len(got.DataChecks) != 1 || got.DataChecks[0].CoverageStatus != test.status || got.DataChecks[0].Category != test.category {
					t.Fatalf("checks=%+v", got.DataChecks)
				}
			} else if len(got.Items) != 0 || len(got.DataChecks) != 0 {
				t.Fatal("covered emitted an issue")
			}
		})
	}
}
func TestUpcomingMissingFXRetainsNativeShortfall(t *testing.T) {
	q, s := upcomingFixture(t)
	q.BaseCurrency = "EUR"
	s.Managed = false
	s.Contract = nil
	s.ProductID = nil
	s.ExplicitPolicy.UnlockOn = nil
	s.ExplicitPolicy.AccessKind = AccessOnRequest
	rs := []LiquidityReservation{{ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "USD reserve", Amount: mustTestMoney(t, "6000", "USD"), Currency: "USD", Revision: 1}}
	o, e := EvaluateLiquidity(q, []LiquiditySource{s}, rs, func(decimal.Decimal, CurrencyCode) (*decimal.Decimal, bool, error) { return nil, false, nil })
	if e != nil {
		t.Fatal(e)
	}
	got, e := ProjectUpcomingActions(o, []LiquiditySource{s}, LiquiditySnapshot{Reservations: rs})
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Items) != 1 || got.Items[0].Amount == nil || got.Items[0].Amount.Currency() != "USD" {
		t.Fatalf("native=%+v", got)
	}
}
func TestUpcomingDeterminismAndIdentity(t *testing.T) {
	q, ss, rs := fixtureA(t)
	for i := range ss {
		if ss[i].Ref.Kind == SourceHolding {
			ss[i].Ref.Currency = nil
		}
		if ss[i].ExplicitPolicy != nil && ss[i].ExplicitPolicy.Source.Kind == SourceHolding {
			ss[i].ExplicitPolicy.Source.Currency = nil
		}
	}
	o, e := EvaluateLiquidity(q, ss, rs, identityFX(q.BaseCurrency))
	if e != nil {
		t.Fatal(e)
	}
	snap := LiquiditySnapshot{Reservations: rs}
	before, _ := json.Marshal(struct {
		O    LiquidityOverview
		S    []LiquiditySource
		Snap LiquiditySnapshot
	}{o, ss, snap})
	a, e := ProjectUpcomingActions(o, ss, snap)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(struct {
		O    LiquidityOverview
		S    []LiquiditySource
		Snap LiquiditySnapshot
	}{o, ss, snap})
	if string(before) != string(after) {
		t.Fatal("mutated input")
	}
	rng := rand.New(rand.NewSource(7))
	for k := 0; k < 20; k++ {
		rng.Shuffle(len(o.Sources), func(i, j int) { o.Sources[i], o.Sources[j] = o.Sources[j], o.Sources[i] })
		rng.Shuffle(len(ss), func(i, j int) { ss[i], ss[j] = ss[j], ss[i] })
		rng.Shuffle(len(rs), func(i, j int) { rs[i], rs[j] = rs[j], rs[i] })
		for i := range o.Sources {
			rng.Shuffle(len(o.Sources[i].BucketResults), func(x, y int) {
				o.Sources[i].BucketResults[x], o.Sources[i].BucketResults[y] = o.Sources[i].BucketResults[y], o.Sources[i].BucketResults[x]
			})
			rng.Shuffle(len(o.Sources[i].Assumptions), func(x, y int) {
				o.Sources[i].Assumptions[x], o.Sources[i].Assumptions[y] = o.Sources[i].Assumptions[y], o.Sources[i].Assumptions[x]
			})
		}
		b, e := ProjectUpcomingActions(o, ss, snap)
		if e != nil || !reflect.DeepEqual(a, b) {
			t.Fatalf("unstable iteration %d: %v", k, e)
		}
	}
	q, s := upcomingFixture(t)
	first := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	s.DisplayName = "renamed"
	s.Contract.Name = "new"
	s.CurrentNativeValue = moneyPtr(t, "999", "USD")
	s.ExplicitPolicy.Revision++
	s.ExplicitPolicy.SettlementDays = intPtr(2)
	second := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if first.ID != second.ID || first.OccurrenceKey != second.OccurrenceKey {
		t.Fatal("non-date edit replaced identity")
	}
	s.Contract.MaturityOn = datePtr("2026-10-16")
	third := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if third.ID != first.ID || third.OccurrenceKey == first.OccurrenceKey {
		t.Fatal("date identity contract")
	}
	q.AsOf = time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
	fourth := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if fourth.ID != third.ID || fourth.OccurrenceKey != third.OccurrenceKey || fourth.Phase != "receipt_due" {
		t.Fatal("phase replaced occurrence")
	}
}
func TestUpcomingCivilWindow(t *testing.T) {
	for _, c := range []struct{ instant, zone, start, end string }{{"2026-12-31T20:00:00Z", "Asia/Shanghai", "2027-01-01", "2027-01-31"}, {"2028-02-28T23:00:00Z", "UTC", "2028-02-28", "2028-03-29"}, {"2026-03-08T06:59:00Z", "America/New_York", "2026-03-08", "2026-04-07"}} {
		q, s := upcomingFixture(t)
		q.AsOf, _ = time.Parse(time.RFC3339, c.instant)
		q.Timezone = c.zone
		s.Contract.MaturityOn = datePtr(c.end)
		s.ExplicitPolicy.UnlockOn = datePtr(c.end)
		got := projectUpcoming(t, q, []LiquiditySource{s}, nil)
		if got.WindowStartOn != c.start || got.WindowEndOn != c.end || len(got.Items) != 1 {
			t.Fatalf("window=%+v", got)
		}
	}
}

func TestUpcomingWeekdayAndRollingReceiptEvidence(t *testing.T) {
	q, s := upcomingFixture(t)
	s.Contract.MaturityOn = nil
	s.ExplicitPolicy.UnlockOn = datePtr("2026-10-16")
	s.ExplicitPolicy.SettlementDays = intPtr(2)
	s.ExplicitPolicy.DayBasis = dayBasisPtr(DayBasisWeekdays)
	first := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if *first.EstimatedReceiptOn != "2026-10-20" || first.ReceiptBasis != "request_on_eligible_date" {
		t.Fatalf("receipt=%+v", first)
	}
	q.AsOf = time.Date(2026, 10, 17, 4, 0, 0, 0, time.UTC)
	second := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if second.Phase != "eligible" || second.ReceiptBasis != "request_today" || first.OccurrenceKey != second.OccurrenceKey {
		t.Fatal("rolling estimate became scheduled receipt")
	}
	s.ExplicitPolicy.UnlockOn = datePtr("2026-10-17")
	s.ExplicitPolicy.SettlementDays = intPtr(0)
	third := projectUpcoming(t, q, []LiquiditySource{s}, nil).Items[0]
	if *third.EstimatedReceiptOn != "2026-10-17" || third.Phase != "today" {
		t.Fatal("T+0 weekend moved")
	}
}
func TestUpcomingUnresolvedReservationAndCountConsistency(t *testing.T) {
	q, s := upcomingFixture(t)
	s.QuantityZero = true
	r := LiquidityReservation{ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "Unresolved", Amount: mustTestMoney(t, "10", "USD"), Currency: "USD", Revision: 1}
	got := projectUpcoming(t, q, []LiquiditySource{s}, []LiquidityReservation{r})
	if got.Counts.ReservationIssues != 1 || got.Counts.Total != 1 || got.Items[0].Kind != "reservation_unresolved" || got.Items[0].PrimaryTarget.ReservationID == nil {
		t.Fatalf("unresolved=%+v", got)
	}
	if got.Counts.Total != got.Counts.Attention+got.Counts.Scheduled {
		t.Fatal("counts differ")
	}
}
func TestUpcomingExcludedDatesSurviveReservationNotes(t *testing.T) {
	q, s := upcomingFixture(t)
	s.ExplicitPolicy.AccessKind = AccessExcluded
	r := LiquidityReservation{ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "Excluded", Amount: mustTestMoney(t, "10", "USD"), Currency: "USD", Revision: 1}
	got := projectUpcoming(t, q, []LiquiditySource{s}, []LiquidityReservation{r})
	if len(got.DataChecks) != 1 || got.DataChecks[0].AnchorOn == nil || *got.DataChecks[0].AnchorOn != "2026-10-15" || got.DataChecks[0].ReferenceOn == nil || got.Completeness != StatusComplete {
		t.Fatalf("scope=%+v", got)
	}
	check := got.DataChecks[0]
	if check.Target.Kind != "product" || len(check.Targets) != 2 || check.Targets[0].Kind == check.Targets[1].Kind {
		t.Fatalf("merged terms and reservations targets lost: %+v", check)
	}
}
func TestUpcomingInvalidReservationErrors(t *testing.T) {
	q, s := upcomingFixture(t)
	a := LiquidityReservation{ID: NewLiquidityReservationID(), HouseholdID: s.ExplicitPolicy.HouseholdID, Source: s.Ref, Label: "A", Amount: mustTestMoney(t, "10", "USD"), Currency: "USD", Revision: 1}
	b := a
	b.ID = NewLiquidityReservationID()
	b.Amount = mustTestMoney(t, "10", "EUR")
	b.Currency = "EUR"
	if _, e := EvaluateLiquidity(q, []LiquiditySource{s}, []LiquidityReservation{a, b}, identityFX(q.BaseCurrency)); e == nil {
		t.Fatal("mixed currency accepted")
	}
	b.Currency = "USD"
	if _, e := EvaluateLiquidity(q, []LiquiditySource{s}, []LiquidityReservation{b}, identityFX(q.BaseCurrency)); e == nil {
		t.Fatal("invalid reservation silently dropped")
	}
}

func TestUpcomingOrdinaryUnlockWithReceiptOverride(t *testing.T) {
	for _, c := range []struct{ name, unlock, receipt, phase string }{
		{"past", "2026-10-01", "2026-10-05", "eligible"},
		{"future_receipt", "2026-10-01", "2026-10-20", "eligible"},
		{"today_unlock", "2026-10-11", "2026-10-15", "today"},
		{"future_unlock", "2026-10-15", "2026-10-20", "upcoming"},
	} {
		t.Run(c.name, func(t *testing.T) {
			q, s := upcomingFixture(t)
			s.Managed = false
			s.Contract = nil
			s.ProductID = nil
			s.ExplicitPolicy.UnlockOn = datePtr(c.unlock)
			s.ExplicitPolicy.ReceiptOnOverride = datePtr(c.receipt)
			overview, err := EvaluateLiquidity(q, []LiquiditySource{s}, nil, identityFX(q.BaseCurrency))
			if err != nil {
				t.Fatal(err)
			}
			if *overview.Sources[0].NormalRoute.ReceiptOn != c.receipt {
				t.Fatal("authoritative override changed")
			}
			got, err := ProjectUpcomingActions(overview, []LiquiditySource{s}, LiquiditySnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != 1 || got.Items[0].Phase != c.phase || got.Counts.ReceiptDue != 0 {
				t.Fatalf("ordinary lifecycle=%+v", got)
			}
			item := got.Items[0]
			if item.EstimatedReceiptOn != nil || item.ReceiptBasis != "unknown" || item.PrimaryTarget.Kind != "source_policy" {
				t.Fatalf("ordinary override must stay in Available Funds details: %+v", item)
			}
		})
	}
}

func TestUpcomingOrdinaryOverrideWithoutUnlockHasNoAction(t *testing.T) {
	q, s := upcomingFixture(t)
	s.Managed = false
	s.Contract = nil
	s.ProductID = nil
	s.ExplicitPolicy.AccessKind = AccessOnRequest
	s.ExplicitPolicy.UnlockOn = nil
	s.ExplicitPolicy.ReceiptOnOverride = datePtr("2026-10-05")
	got := projectUpcoming(t, q, []LiquiditySource{s}, nil)
	if len(got.Items) != 0 || got.Counts.ReceiptDue != 0 {
		t.Fatalf("ordinary override invented action: %+v", got)
	}
}
