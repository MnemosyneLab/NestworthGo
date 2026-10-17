package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewCashOnlyActivityFilteringPreservesDailyInputs(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	batch, err := sqlite.NewRepository(f.db).LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	cash, err := domain.ParseMoney("25", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	accountID := f.account.Account.ID
	activityAt := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)
	for index := 0; index < 100; index++ {
		activityID := domain.NewActivityID()
		batch.History.Activities = append(batch.History.Activities, domain.Activity{
			ID: activityID, HouseholdID: f.account.Account.HouseholdID, Kind: domain.ActivityCashDividend,
			EffectiveAt: activityAt.Add(time.Duration(index) * time.Minute), EffectiveLocalDate: "2026-08-01", CreatedAt: activityAt,
			Effects: []domain.ActivityEffect{{
				ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount,
				Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome,
				AccountID: &accountID, Money: &cash,
			}},
		})
	}
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	activityID := domain.NewActivityID()
	holdingID, instrumentID := f.holding.ID, f.instrument.ID
	batch.History.Activities = append(batch.History.Activities, domain.Activity{
		ID: activityID, HouseholdID: f.account.Account.HouseholdID, Kind: domain.ActivityPositionTransfer,
		EffectiveAt: activityAt, EffectiveLocalDate: "2026-08-01", CreatedAt: activityAt,
		Effects: []domain.ActivityEffect{{
			ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleQuantity,
			Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationRemeasurement,
			HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity,
		}},
	})

	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}}}}
	fullInput, fullCoverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", spec, f.instrument.ID, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	reviewBatch := batch
	reviewBatch.History.Activities = reviewSnapshotActivities(batch.History.Activities)
	filteredInput, filteredCoverage, err := ProjectReviewHistoricalDay(t.Context(), reviewBatch, "", spec, f.instrument.ID, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fullInput, filteredInput) || !reflect.DeepEqual(fullCoverage, filteredCoverage) {
		t.Fatalf("cash-only activity filtering changed review evidence: full=%+v/%+v filtered=%+v/%+v", fullInput, fullCoverage, filteredInput, filteredCoverage)
	}
	if filteredInput.Quantity == nil || *filteredInput.Quantity != "2" {
		t.Fatalf("quantity activity was filtered from review history: %+v", filteredInput)
	}
}

func TestReviewHistoricalCaptureRetainsPositionActivityAndSkipsCashOnlyActivity(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	unitCost, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	position, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{
		HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID,
		Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &unitCost, EffectiveAt: *f.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	cashActivityID := domain.NewActivityID()
	cashEffectID := domain.NewActivityEffectID()
	cashValueID := domain.NewAccountCashValueID()
	at := f.now.UTC().Format(time.RFC3339Nano)
	if _, err := f.db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`, cashActivityID.String(), f.account.Account.HouseholdID.String(), at, "2026-08-01", at); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'5','CNY')`, cashEffectID.String(), cashActivityID.String(), f.account.Account.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'5','CNY',?,?,?,'event')`, cashValueID.String(), f.account.Account.ID.String(), at, at, cashEffectID.String()); err != nil {
		t.Fatal(err)
	}
	capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Historical.History.Activities) != 1 || capture.Historical.History.Activities[0].ID != position.Activity.ID {
		t.Fatalf("review source should retain the position activity and omit cash-only activity: %+v", capture.Historical.History.Activities)
	}
	if len(capture.Historical.History.Activities[0].Effects) == 0 || capture.Historical.History.Activities[0].Effects[0].Target != domain.EffectTargetHoldingQuantity {
		t.Fatalf("position effect was not retained for review replay: %+v", capture.Historical.History.Activities[0].Effects)
	}
}
