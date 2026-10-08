package application

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func assertAttributionPrecisionIdentity(t *testing.T, r FinancialComparisonResult) {
	t.Helper()
	link := r.Content.Attribution
	amount := func(a FinancialContextAmount) decimal.Decimal {
		t.Helper()
		if a.Value == nil {
			t.Fatal("missing identity amount", a)
		}
		value, err := decimal.NewFromString(*a.Value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !amount(link.EndingValue).Sub(amount(link.BeginningValue)).Equal(amount(link.AnalysisDelta)) ||
		!amount(link.ExplainedDelta).Add(amount(link.Residual)).Add(amount(link.Precision.DriverAdjustment)).Equal(amount(link.AnalysisDelta)) ||
		!amount(link.AnalysisDelta).Add(amount(link.Precision.BoundaryAdjustment)).Equal(amount(r.Content.Change.NetWorth)) ||
		!amount(link.Precision.BoundaryAdjustment).Add(amount(link.Precision.DriverAdjustment)).Equal(amount(link.PrecisionAdjustment)) ||
		!amount(link.ExplainedDelta).Add(amount(link.Residual)).Add(amount(link.PrecisionAdjustment)).Equal(amount(r.Content.Change.NetWorth)) {
		t.Fatal("precision identity failed", link, r.Content.Change)
	}
	if link.Precision.AmountScale != 4 || link.Precision.Rounding != "half_even" {
		t.Fatal("missing precision contract", link.Precision)
	}
}

func TestFinancialAttributionScopeDescribesAccountEnteringPeriod(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Private existing", "bank_account", "asset", "balance", "CNY", "40")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	b := overviewAccount(t, s, owner, "Private entering", "bank_account", "asset", "balance", "CNY", "15")
	*now = now.AddDate(0, 0, 1)
	r := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, r, "compatible", "")
	link := r.Content.Attribution
	wantOverviewAmount(t, r.Content.Left.Summary.NetWorth.Value, "40")
	wantOverviewAmount(t, r.Content.Right.Summary.NetWorth.Value, "55")
	wantOverviewAmount(t, attributionDriver(link, "external_flow"), "15")
	if link.LeftScope.AccountCount != 1 || link.LeftScope.IncludedAccountCount != 1 || link.RightScope.AccountCount != 2 || link.RightScope.IncludedAccountCount != 2 || link.Scope.AccountCount != 2 || link.Scope.IncludedAccountCount != 2 || len(link.Scope.AccountRefs) != 2 || !strings.Contains(link.ScopeBasis, "endpoint_account_union") {
		t.Fatal("misleading attribution scope", link)
	}
	assertAttributionPrecisionIdentity(t, r)
	raw, _ := json.Marshal(r.Content)
	for _, secret := range []string{"Private", a.Account.ID.String(), b.Account.ID.String()} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("scope description leaked identity", secret)
		}
	}
	scoped := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
	wantAttributionStatus(t, scoped, "compatible", "")
	if scoped.Content.Attribution.Scope.AccountCount != 1 || scoped.Content.Attribution.LeftScope.AccountCount != 1 || scoped.Content.Attribution.RightScope.AccountCount != 1 {
		t.Fatal("scope widened", scoped)
	}
	wantOverviewAmount(t, scoped.Content.Change.NetWorth.Value, "0")
	t.Logf("household 40->55: leftScope=%+v rightScope=%+v union=%+v", link.LeftScope, link.RightScope, link.Scope)
}

func TestFinancialAttributionRequestedRangeOrderAndRevision(t *testing.T) {
	for _, order := range []string{"late_then_early", "early_then_late"} {
		t.Run(order, func(t *testing.T) {
			s, db, owner, now := overviewFixture(t)
			cash := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
			broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
			instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			for day := 1; day <= 11; day++ {
				if _, err := s.AppendManualInstrumentQuote(t.Context(), instrument.ID, fmt.Sprint(day+4), fmt.Sprintf("2026-08-%02d", day), false); err != nil {
					t.Fatal(err)
				}
			}
			holding, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{holding.ID: "5"}); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			rows := func() int {
				var count int
				if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&count); err != nil {
					t.Fatal(err)
				}
				return count
			}
			dates := func(label string) []string {
				snapshots, err := s.repository.ListDailyValuationSnapshots(t.Context(), cash.Account.HouseholdID, time.Time{}, time.Time{})
				if err != nil {
					t.Fatal(err)
				}
				result := []string{}
				for _, snapshot := range snapshots {
					if !snapshot.Complete {
						t.Fatal("incomplete synthetic snapshot", snapshot.LocalDate)
					}
					result = append(result, snapshot.LocalDate)
				}
				state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(struct {
					Dates                         []string
					Watermark, DirtyFrom, DirtyTo *string
					InputGeneration, PhysicalRows int
				}{result, state.LastCompletedClosedOn, state.DirtyFrom, state.DirtyTo, state.InputGeneration, rows()})
				t.Logf("%s snapshots/state=%s", label, encoded)
				return result
			}
			call := func(left, right string) FinancialComparisonResult {
				r := attributionFor(t, s, left, right)
				wantAttributionStatus(t, r, "compatible", "")
				assertAttributionPrecisionIdentity(t, r)
				t.Logf("%s->%s status=%s", left, right, r.Content.Attribution.Status)
				return r
			}
			if len(dates("clean")) != 0 {
				t.Fatal("fixture was already materialized")
			}
			if order == "late_then_early" {
				call("2026-08-10", "2026-08-11")
				if got := dates("after late"); !reflect.DeepEqual(got, []string{"2026-08-10", "2026-08-11"}) {
					t.Fatal(got)
				}
				call("2026-08-01", "2026-08-02")
			} else {
				call("2026-08-01", "2026-08-02")
				call("2026-08-10", "2026-08-11")
			}
			if got := dates("both requested ranges"); !reflect.DeepEqual(got, []string{"2026-08-01", "2026-08-02", "2026-08-10", "2026-08-11"}) {
				t.Fatal(got)
			}
			old := call("2026-08-01", "2026-08-02")
			before := rows()
			if repeated := call("2026-08-01", "2026-08-02"); repeated.ContentHash != old.ContentHash || rows() != before {
				t.Fatal("unchanged range was republished/rebuilt")
			}
			// A partially populated gap must fill only its absent days.
			if _, err := s.RebuildHistoricalSnapshots(t.Context(), "2026-08-05", "2026-08-05"); err != nil {
				t.Fatal(err)
			}
			before = rows()
			call("2026-08-04", "2026-08-07")
			if rows() != before+3 {
				t.Fatal("existing gap day was unnecessarily revised", rows(), before)
			}
			wantAttributionStatus(t, attributionFor(t, s, "2026-08-01", "2026-08-02", broker.Account.ID), "compatible", "")
			// Corrected source evidence must rebuild the dirty day, not only holes.
			if _, err := s.AppendManualInstrumentQuote(t.Context(), instrument.ID, "7", "2026-08-02", false); err != nil {
				t.Fatal(err)
			}
			state, err := s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" {
				t.Fatal(state, err)
			}
			revised := call("2026-08-01", "2026-08-02")
			wantOverviewAmount(t, revised.Content.Change.NetWorth.Value, "20")
			if revised.ContentHash == old.ContentHash {
				t.Fatal("revision evidence ignored")
			}
			state, err = s.DailySnapshotState(t.Context(), cash.Account.HouseholdID)
			if err != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-11" || (state.DirtyFrom != nil && *state.DirtyFrom <= "2026-08-02") {
				t.Fatal("dirty/watermark semantics lost", state, err)
			}
			call("2026-08-10", "2026-08-11")
		})
	}
}

func TestFinancialAttributionGapBackfillChunks(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	wantAttributionStatus(t, attributionFor(t, s, "2026-09-25", "2026-09-26"), "compatible", "")
	wantAttributionStatus(t, attributionFor(t, s, "2026-08-01", "2026-09-10"), "compatible", "")
	snapshots, err := s.repository.ListDailyValuationSnapshots(t.Context(), a.Account.HouseholdID, time.Time{}, time.Time{})
	if err != nil || len(snapshots) != 43 {
		t.Fatal("31-day chunk backfill", len(snapshots), err)
	}
}

func TestFinancialAttributionPreservesEarlierDirtyRange(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic earlier revision", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	wantAttributionStatus(t, attributionFor(t, s, "2026-08-01", "2026-08-02"), "compatible", "")
	wantAttributionStatus(t, attributionFor(t, s, "2026-08-10", "2026-08-11"), "compatible", "")
	if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	state, err := s.DailySnapshotState(t.Context(), a.Account.HouseholdID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" {
		t.Fatal("earlier revision not dirty", state, err)
	}
	wantAttributionStatus(t, attributionFor(t, s, "2026-08-10", "2026-08-11"), "compatible", "")
	state, err = s.DailySnapshotState(t.Context(), a.Account.HouseholdID)
	encoded, _ := json.Marshal(struct {
		DirtyFrom, DirtyTo, Watermark *string
		Generation                    int
	}{state.DirtyFrom, state.DirtyTo, state.LastCompletedClosedOn, state.InputGeneration})
	t.Logf("after later request state=%s", encoded)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-02" {
		t.Fatal("unbuilt earlier dirty range was cleared", state, err)
	}
	early := attributionFor(t, s, "2026-08-01", "2026-08-02")
	wantAttributionStatus(t, early, "compatible", "")
	wantOverviewAmount(t, early.Content.Change.NetWorth.Value, "1")
	state, err = s.DailySnapshotState(t.Context(), a.Account.HouseholdID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-11" {
		t.Fatal("dirty prefix/watermark not preserved", state, err)
	}
}

func TestFinancialAttributionConsumesOnlyRebuiltBoundedDirtyPrefix(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic bounded revision", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	wantAttributionStatus(t, attributionFor(t, s, "2026-08-01", "2026-08-11"), "compatible", "")
	// Synthetic repository fixture for the bounded dirty marker used by source
	// repairs. Per-day saves and range completion retain their existing meaning.
	if _, err := db.SQL.Exec("UPDATE history_snapshot_state SET dirty_from = ?, dirty_to = ? WHERE household_id = ?", "2026-08-02", "2026-08-05", a.Account.HouseholdID.String()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ left, right, remaining string }{
		{"2026-08-07", "2026-08-08", "2026-08-02"},
		{"2026-08-04", "2026-08-05", "2026-08-02"},
		{"2026-08-01", "2026-08-02", "2026-08-03"},
		{"2026-08-03", "2026-08-06", ""},
	} {
		wantAttributionStatus(t, attributionFor(t, s, tc.left, tc.right), "compatible", "")
		state, err := s.DailySnapshotState(t.Context(), a.Account.HouseholdID)
		if err != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-11" {
			t.Fatal("watermark changed", state, err)
		}
		if tc.remaining == "" {
			if state.DirtyFrom != nil || state.DirtyTo != nil {
				t.Fatal("fully rebuilt bounded dirty range retained", state)
			}
		} else if state.DirtyFrom == nil || *state.DirtyFrom != tc.remaining || state.DirtyTo == nil || *state.DirtyTo != "2026-08-05" {
			t.Fatal("unbuilt bounded dirty range lost", state)
		}
	}
}

func TestFinancialAttributionMidnightPreflightIncludesLeft(t *testing.T) {
	for _, boundary := range [][2]string{{"2026-03-08", "2026-03-09"}, {"2026-03-07", "2026-03-08"}, {"2026-11-01", "2026-11-02"}, {"2026-10-31", "2026-11-01"}} {
		t.Run(boundary[0], func(t *testing.T) {
			s, db, owner, now := overviewFixture(t)
			*now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			overviewAccount(t, s, owner, "Synthetic Havana cash", "bank_account", "asset", "balance", "CNY", "100")
			if _, err := s.StartHistory(t.Context(), "America/Havana"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
			if attributionBoundariesSupported(boundary[0], boundary[1], "America/Havana") {
				t.Fatal("unsupported midnight admitted")
			}
			r := attributionFor(t, s, boundary[0], boundary[1])
			wantAttributionStatus(t, r, "incompatible", "historical_boundary_unsupported")
			var rows int
			if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&rows); err != nil || rows != 0 {
				t.Fatal("preflight wrote snapshots", rows, err)
			}
			t.Logf("%s->%s status=%s reasons=%v snapshots=%d", boundary[0], boundary[1], r.Content.Attribution.Status, r.Content.Attribution.MismatchReasons, rows)
		})
	}
}

func TestFinancialAttributionPrecisionSixDecimalFX(t *testing.T) {
	for _, tc := range []struct{ name, leftFX, rightFX, role, adjustment string }{
		{"static", "7.123456", "7.123456", "asset", "0"},
		{"changed", "7.123456", "7.234567", "asset", "0.000011"},
		{"liability", "7.123456", "7.234567", "liability", "-0.000011"},
		{"four_place_control", "7.1234", "7.1234", "asset", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, owner, now := overviewFixture(t)
			kind := "bank_account"
			if tc.role == "liability" {
				kind = "loan"
			}
			a := overviewAccount(t, s, owner, "Synthetic USD", kind, tc.role, "balance", "USD", "1")
			for _, quote := range []struct{ date, fx string }{{"2026-08-01", tc.leftFX}, {"2026-08-02", tc.rightFX}} {
				if _, err := s.AppendManualFXQuote(t.Context(), "USD", "CNY", quote.fx, quote.date); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
			r := attributionFor(t, s, "2026-08-01", "2026-08-02", a.Account.ID)
			wantAttributionStatus(t, r, "compatible", "")
			wantOverviewAmount(t, r.Content.Attribution.PrecisionAdjustment.Value, tc.adjustment)
			exactLeft, exactRight := tc.leftFX, tc.rightFX
			if tc.role == "liability" {
				exactLeft, exactRight = "-"+exactLeft, "-"+exactRight
			}
			wantOverviewAmount(t, r.Content.Left.Summary.NetWorth.Value, exactLeft)
			wantOverviewAmount(t, r.Content.Right.Summary.NetWorth.Value, exactRight)
			assertAttributionPrecisionIdentity(t, r)
			raw, _ := json.Marshal(struct {
				ComparisonLeft, ComparisonRight, ComparisonChange FinancialContextAmount
				Attribution                                       *FinancialAttributionLink
			}{r.Content.Left.Summary.NetWorth, r.Content.Right.Summary.NetWorth, r.Content.Change.NetWorth, r.Content.Attribution})
			t.Logf("FX=%s->%s response=%s", tc.leftFX, tc.rightFX, raw)
		})
	}
}

func TestFinancialAttributionPrecisionComponentTiesAndMultiDay(t *testing.T) {
	for _, tc := range []struct{ name, role1, role2, amount2, begin, end, exactBegin, exactEnd, boundaryAdjustment, driverAdjustment string }{
		{"positive", "asset", "asset", "1", "2", "2.0004", "2.0001", "2.0003", "-0.0002", "0.0002"},
		{"negative", "liability", "liability", "1", "-2", "-2.0004", "-2.0001", "-2.0003", "0.0002", "-0.0002"},
		{"mixed_liability", "asset", "liability", "3", "-2.0002", "-2.0002", "-2.0001", "-2.0003", "-0.0002", "0.0002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, owner, now := overviewFixture(t)
			for _, account := range []struct{ role, amount string }{{tc.role1, "1"}, {tc.role2, tc.amount2}} {
				kind := "bank_account"
				if account.role == "liability" {
					kind = "loan"
				}
				overviewAccount(t, s, owner, "Synthetic tie", kind, account.role, "balance", "USD", account.amount)
			}
			for index, fx := range []string{"1.00005", "1.00010", "1.00015"} {
				if _, err := s.AppendManualFXQuote(t.Context(), "USD", "CNY", fx, fmt.Sprintf("2026-08-%02d", index+1)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
			r := attributionFor(t, s, "2026-08-01", "2026-08-03")
			wantAttributionStatus(t, r, "compatible", "")
			link := r.Content.Attribution
			wantOverviewAmount(t, link.BeginningValue.Value, tc.begin)
			wantOverviewAmount(t, link.EndingValue.Value, tc.end)
			wantOverviewAmount(t, r.Content.Left.Summary.NetWorth.Value, tc.exactBegin)
			wantOverviewAmount(t, r.Content.Right.Summary.NetWorth.Value, tc.exactEnd)
			wantOverviewAmount(t, link.Precision.BoundaryAdjustment.Value, tc.boundaryAdjustment)
			wantOverviewAmount(t, link.Precision.DriverAdjustment.Value, tc.driverAdjustment)
			assertAttributionPrecisionIdentity(t, r)
		})
	}
}

func TestFinancialAttributionPrecisionDoesNotHideUnknownDailyGap(t *testing.T) {
	householdID := domain.NewHouseholdID()
	a := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: a}}}}
	for index, exact := range []string{"100", "100.000001", "100"} {
		input.Snapshots = append(input.Snapshots, analysisSnapshot(fmt.Sprintf("2026-08-%02d", index+1), domain.DailyValuationSnapshotItem{AccountID: a.ID, NativeAmount: exact, NativeCurrency: "CNY", BaseAmountExact: exact, Complete: true}))
	}
	query := analysisBaseQuery(domain.ValuationBase)
	query.To = "2026-08-03"
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	comparison := FinancialComparisonContent{Left: FinancialComparisonSide{Basis: FinancialContextBasis{BaseCurrency: "CNY"}, Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("100"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Right: FinancialComparisonSide{Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("100"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Change: FinancialComparisonChange{NetWorth: contextAmount(historicalString("0"), "CNY")}}
	link := &FinancialAttributionLink{Drivers: []FinancialAttributionDriver{}}
	status, reasons, err := projectFinancialAttribution(link, &comparison, result, input)
	if err != nil || status != "incompatible" || !reflect.DeepEqual(reasons, []string{"driver_reconciliation_mismatch"}) || link.PrecisionAdjustment.Value != nil || len(link.Drivers) != 0 || link.InvestmentReturn != nil {
		t.Fatal("unknown gaps called precision", status, reasons, link, err)
	}
}

func TestFinancialAttributionPrecisionDoesNotHideCancellingComponentGaps(t *testing.T) {
	householdID := domain.NewHouseholdID()
	a := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	b := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: a}, {Account: b}}}}
	for index, exact := range [][2]string{{"100", "100"}, {"100.000001", "99.999999"}} {
		input.Snapshots = append(input.Snapshots, analysisSnapshot(fmt.Sprintf("2026-08-%02d", index+1), domain.DailyValuationSnapshotItem{AccountID: a.ID, NativeAmount: exact[0], NativeCurrency: "CNY", BaseAmountExact: exact[0], Complete: true}, domain.DailyValuationSnapshotItem{AccountID: b.ID, NativeAmount: exact[1], NativeCurrency: "CNY", BaseAmountExact: exact[1], Complete: true}))
	}
	result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	comparison := FinancialComparisonContent{Left: FinancialComparisonSide{Basis: FinancialContextBasis{BaseCurrency: "CNY"}, Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("200"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Right: FinancialComparisonSide{Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("200"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Change: FinancialComparisonChange{NetWorth: contextAmount(historicalString("0"), "CNY")}}
	link := &FinancialAttributionLink{Drivers: []FinancialAttributionDriver{}}
	status, reasons, err := projectFinancialAttribution(link, &comparison, result, input)
	if err != nil || status != "incompatible" || !reflect.DeepEqual(reasons, []string{"driver_reconciliation_mismatch"}) || link.PrecisionAdjustment.Value != nil || len(link.Drivers) != 0 || link.InvestmentReturn != nil {
		t.Fatal("unknown component gaps called precision", status, reasons, link, err)
	}
}

func TestFinancialAttributionPrecisionRejectsCancellingComponentCorruption(t *testing.T) {
	householdID := domain.NewHouseholdID()
	a := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	b := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: a}, {Account: b}}}}
	for day := 1; day <= 3; day++ {
		input.Snapshots = append(input.Snapshots, analysisSnapshot(fmt.Sprintf("2026-08-%02d", day), analysisItem(t, a.ID, "CNY", "100", "100", nil, nil, "", ""), analysisItem(t, b.ID, "CNY", "100", "100", nil, nil, "", "")))
	}
	query := analysisBaseQuery(domain.ValuationBase)
	query.To = "2026-08-03"
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt two interior projected values while preserving every aggregate
	// boundary and exact driver. Numeric household equality must not hide this.
	result.Days[0].EndingValue, err = domain.ParseSignedMoney("100.0001", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	result.Days[1].EndingValue, err = domain.ParseSignedMoney("99.9999", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	comparison := FinancialComparisonContent{Left: FinancialComparisonSide{Basis: FinancialContextBasis{BaseCurrency: "CNY"}, Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("200"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Right: FinancialComparisonSide{Summary: FinancialContextSummary{NetWorth: contextAmount(historicalString("200"), "CNY")}, Coverage: FinancialContextCoverage{ValuationComplete: true}}, Change: FinancialComparisonChange{NetWorth: contextAmount(historicalString("0"), "CNY")}}
	link := &FinancialAttributionLink{Drivers: []FinancialAttributionDriver{}}
	status, reasons, err := projectFinancialAttribution(link, &comparison, result, input)
	if err != nil || status != "incompatible" || !reflect.DeepEqual(reasons, []string{"analysis_endpoint_mismatch"}) || link.PrecisionAdjustment.Value != nil || len(link.Drivers) != 0 || link.InvestmentReturn != nil {
		t.Fatal("component corruption called precision", status, reasons, link, err)
	}
}
