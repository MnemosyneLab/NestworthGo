package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCertifiedFutureActivationAndPublicationFence(t *testing.T) {
	for _, kind := range []string{"exit", "entry", "clear-rebuy", "transfer"} {
		t.Run(kind, func(t *testing.T) {
			quantity := "1"
			if kind == "entry" {
				quantity = "0"
			}
			f, r := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), quantity)
			before, at := *f.now, f.now.Add(time.Hour)
			if kind == "transfer" {
				var owner domain.MemberID
				if err := f.db.SQL.QueryRow(`SELECT member_id FROM account_ownership WHERE account_id=?`, f.account.Account.ID.String()).Scan(&owner); err != nil {
					t.Fatal(err)
				}
				account := overviewAccount(t, f.s, owner, "Synthetic certified receiver", "brokerage", "asset", "holdings", "CNY", "")
				target, err := f.s.CreateHolding(t.Context(), HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: f.instrument.ID.String(), Quantity: "0"})
				if err != nil {
					t.Fatal(err)
				}
				*f.now = at
				if _, err := f.s.RecordChange(t.Context(), domain.PositionTransferInput{HouseholdID: f.account.Account.HouseholdID, FromHoldingID: f.holding.ID, ToHoldingID: target.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: at}); err != nil {
					t.Fatal(err)
				}
			} else {
				*f.now = at
				f.quantity(t, kind == "entry")
				if kind == "clear-rebuy" {
					*f.now = at.Add(time.Hour)
					f.quantity(t, true)
				}
			}
			*f.now = before
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			initial := certifiedStage(t, f, r)
			if !initial.Capture.Temporal.NextTransitionAt.Equal(at) {
				t.Fatal("missing future frontier")
			}
			*f.now = at.Add(-time.Nanosecond)
			result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil || result.DayCount != 0 {
				t.Fatal("T-1ns failed certified no-op", result, err)
			}
			stale := certifiedStage(t, f, r)
			rows := noChangeProjectionRows(t, f.db)
			if err := certifiedPublish(t.Context(), f, r, stale, func() time.Time { return at }); err == nil {
				t.Fatal("old stage crossed activation fence")
			}
			if !reflect.DeepEqual(rows, noChangeProjectionRows(t, f.db)) {
				t.Fatal("fence failure partially wrote")
			}
			points := []time.Time{at, at.Add(time.Nanosecond)}
			if kind == "clear-rebuy" {
				points = append(points, at.Add(time.Hour), at.Add(time.Hour+time.Nanosecond))
			}
			for _, now := range points {
				*f.now = now
				p := certifiedStage(t, f, r)
				if p.Capture.SourceRevision.Token != initial.Capture.SourceRevision.Token {
					t.Fatal("activation fixture changed source token")
				}
				assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, p)
				if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				noChangeFullOracle(t, f)
			}
		})
	}
}

func TestReviewCertifiedPersistsRecheckedTemporalMinimum(t *testing.T) {
	f, r := certifiedFixture(t)
	before := *f.now
	t1, t2 := before.Add(time.Hour), before.Add(2*time.Hour)
	id := domain.NewActivityID().String()
	if _, err := f.db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cash_in','reconciliation',?,?,?)`, id, f.account.Account.HouseholdID.String(), t2.Format(time.RFC3339Nano), t2.Format(time.DateOnly), before.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'cash','added','account_cash','remeasurement',?,'1','CNY')`, domain.NewActivityID().String(), id, f.account.Account.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := certifiedStage(t, f, r)
	if !p.Capture.Temporal.NextTransitionAt.Equal(t2) {
		t.Fatal("missing captured frontier")
	}
	if _, err := f.db.SQL.Exec(`DROP TRIGGER financial_source_activities_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE activities SET effective_at=? WHERE id=?`, t1.Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return before }); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := f.db.SQL.QueryRow(`SELECT next_transition_at FROM review_temporal_checkpoints`).Scan(&saved); err != nil || saved != t1.Format(time.RFC3339Nano) {
		t.Fatal("PR121 conservative intersection was lost", saved, err)
	}
	assertNoCertifiedMAC(t, f) // Changed tracking cannot mint new authority.
}
