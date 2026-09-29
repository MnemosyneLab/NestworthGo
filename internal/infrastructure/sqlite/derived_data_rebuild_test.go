package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReplaceDerivedDataPreservesAbsoluteTargetAndRollsBackFailedPublication(t *testing.T) {
	db, repo, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	members, err := repo.ListMembers(ctx, true)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v", err)
	}
	initial, err := domain.ParseMoney("100", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Bank", AccountType: domain.TypeBankAccount,
		BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance,
		DefaultCurrency: "CNY", InitialAmount: "100", Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	initialValue, err := domain.NewAccountValue(account, initial, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAccount(ctx, account, ownership, &initialValue); err != nil {
		t.Fatal(err)
	}
	originID := domain.NewHistoryOriginID()
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO history_origins(id, household_id, timezone, started_at, created_at) VALUES(?, ?, 'UTC', ?, ?)`, originID.String(), household.ID.String(), formatTimestamp(now), formatTimestamp(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at, input_generation, resolver_policy_version) VALUES(?, NULL, NULL, ?, 3, ?)`, household.ID.String(), formatTimestamp(now), domain.MarketDataResolverPolicy); err != nil {
		t.Fatal(err)
	}
	insertEvent := func(kind domain.ActivityKind, amount, resulting string) (domain.Activity, domain.ActivityEffect) {
		t.Helper()
		activityID, effectID := domain.NewActivityID(), domain.NewActivityEffectID()
		if _, err := db.SQL.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at) VALUES(?, ?, ?, 'other', ?, '2026-09-25', ?)`, activityID.String(), household.ID.String(), string(kind), formatTimestamp(now), formatTimestamp(now)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SQL.ExecContext(ctx, `INSERT INTO activity_effects(id, activity_id, sequence, role, direction, target, classification, account_id, amount, currency) VALUES(?, ?, 1, 'amount', 'added', 'account_value', 'external_flow', ?, ?, 'CNY')`, effectID.String(), activityID.String(), account.ID.String(), amount); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SQL.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, activity_effect_id, projection_kind) VALUES(?, ?, 'balance', ?, 'CNY', ?, ?, ?, 'event')`, domain.NewAccountValueID().String(), account.ID.String(), resulting, formatTimestamp(now), formatTimestamp(now), effectID.String()); err != nil {
			t.Fatal(err)
		}
		money, err := domain.ParseMoney(amount, "CNY")
		if err != nil {
			t.Fatal(err)
		}
		effect := domain.ActivityEffect{ID: effectID, ActivityID: activityID, Sequence: 1, Target: domain.EffectTargetAccountValue, AccountID: &account.ID, Money: &money}
		activity := domain.Activity{ID: activityID, HouseholdID: household.ID, Kind: kind, EffectiveAt: now, CreatedAt: now, Effects: []domain.ActivityEffect{effect}, RecordedEffects: []domain.ActivityEffect{effect}}
		return activity, effect
	}
	flow, flowEffect := insertEvent(domain.ActivityCashIn, "25", "999")
	absolute, absoluteEffect := insertEvent(domain.ActivityValueUpdate, "25", "150")
	view := func(amount string) domain.EndpointView {
		return domain.EndpointView{Target: domain.EffectTargetAccountValue, AccountID: &account.ID, Amount: amount, Currency: "CNY"}
	}
	rebuild := domain.DerivedDataRebuild{
		HouseholdID: household.ID, InputGeneration: 3, AsOf: now,
		Projections: []domain.ActivityProjection{
			{Activity: flow, Resulting: []domain.EndpointView{view("125")}},
			{Activity: absolute, Resulting: []domain.EndpointView{view("777")}},
		},
		Current: []domain.EndpointView{view("150")},
	}
	rebuild.Current = append(rebuild.Current, view("150"))
	if err := repo.ReplaceDerivedData(ctx, rebuild); err == nil {
		t.Fatal("duplicate staged current target should fail")
	}
	readEvent := func(effectID domain.ActivityEffectID) string {
		t.Helper()
		var amount string
		if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_values WHERE activity_effect_id = ? AND projection_kind = 'event'`, effectID.String()).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		return amount
	}
	if got := readEvent(flowEffect.ID); got != "999" {
		t.Fatalf("failed transaction changed event projection: %s", got)
	}
	rebuild.Current = rebuild.Current[:1]
	if err := repo.ReplaceDerivedData(ctx, rebuild); err != nil {
		t.Fatal(err)
	}
	if got := readEvent(flowEffect.ID); got != "125" {
		t.Fatalf("event projection = %s", got)
	}
	if got := readEvent(absoluteEffect.ID); got != "150" {
		t.Fatalf("absolute target changed to %s", got)
	}
	current, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range current.Accounts {
		if record.Account.ID == account.ID {
			if record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "150" {
				t.Fatalf("current balance = %+v", record.LatestValue)
			}
		}
	}
	// A later same-millisecond write must supersede the rebuild's current row.
	insertEvent(domain.ActivityCashIn, "25", "175")
	current, err = repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range current.Accounts {
		if record.Account.ID == account.ID && (record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "175") {
			t.Fatalf("later event did not win timestamp tie: %+v", record.LatestValue)
		}
	}
	var baseline string
	if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_values WHERE account_id = ? AND projection_kind = 'baseline'`, account.ID.String()).Scan(&baseline); err != nil || baseline != "100" {
		t.Fatalf("baseline = %s, %v", baseline, err)
	}
}
