package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type tzCase struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Expected map[string]any `json:"expected,omitempty"`
	Got      map[string]any `json:"got,omitempty"`
	Detail   string         `json:"detail,omitempty"`
}

type tzProbeOut struct {
	When    string   `json:"when"`
	Mode    string   `json:"mode"`
	DBRoot  string   `json:"dbRoot"`
	Results []tzCase `json:"results"`
	AllPass bool     `json:"allPass"`
	Summary string   `json:"summary"`
}

// Known UTC instants from TestAnalysisReviewCase40OriginTimezoneBoundaries and
// TestAnalysisReviewCase36 / analysis_fixture_test DST local-date check.
var (
	tzSpringLABoundary  = time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC)  // → America/Los_Angeles 2026-03-08
	tzFallLABoundary    = time.Date(2026, 11, 2, 7, 30, 0, 0, time.UTC) // → America/Los_Angeles 2026-11-01
	tzSpringLAAfternoon = time.Date(2026, 3, 8, 16, 30, 0, 0, time.UTC) // → America/Los_Angeles 2026-03-08
)

func runTimezoneR4Probe() error {
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/seed/probe-timezone-r4.json")
	base := envOr("NESTWORTH_QA_OUTPUT_DIR", "/workspace/nestworth-analytics-qa")
	dbRoot := filepath.Join(base, "timezone-r4")

	out := tzProbeOut{
		When:   time.Now().UTC().Format(time.RFC3339),
		Mode:   "NESTWORTH_PROBE_MODE=timezone-r4",
		DBRoot: dbRoot,
	}
	cases := make([]tzCase, 0, 24)
	cases = append(cases, tzDomainCases()...)
	cases = append(cases, tzInMemoryAnalysisCases()...)
	cases = append(cases, tzBuildOriginDBs(dbRoot)...)
	return writeTZProbe(outPath, out, cases)
}

func tzDomainCases() []tzCase {
	cases := []tzCase{
		tzRejectLocal("dst_gap_ny", "ResolveLocalDateTime rejects 2026-03-08 02:30 America/New_York (spring gap)", "2026-03-08", "02:30", "America/New_York"),
		tzRejectLocal("dst_gap_la", "ResolveLocalDateTime rejects 2026-03-08 02:30 America/Los_Angeles (spring gap)", "2026-03-08", "02:30", "America/Los_Angeles"),
		tzRejectLocal("dst_ambiguity_ny", "ResolveLocalDateTime rejects 2026-11-01 01:30 America/New_York (fall ambiguity)", "2026-11-01", "01:30", "America/New_York"),
		tzRejectLocal("dst_ambiguity_la", "ResolveLocalDateTime rejects 2026-11-01 01:30 America/Los_Angeles (fall ambiguity)", "2026-11-01", "01:30", "America/Los_Angeles"),
	}

	type mapping struct {
		id, tz string
		at     time.Time
		want   string
	}
	for _, m := range []mapping{
		{"local_la_spring_boundary", "America/Los_Angeles", tzSpringLABoundary, "2026-03-08"},
		{"local_la_fall_boundary", "America/Los_Angeles", tzFallLABoundary, "2026-11-01"},
		{"local_la_spring_afternoon", "America/Los_Angeles", tzSpringLAAfternoon, "2026-03-08"},
		{"local_sgt_spring_boundary", "Asia/Singapore", tzSpringLABoundary, "2026-03-09"},
		{"local_sgt_fall_boundary", "Asia/Singapore", tzFallLABoundary, "2026-11-02"},
		{"local_sgt_spring_afternoon", "Asia/Singapore", tzSpringLAAfternoon, "2026-03-09"},
		{"local_utc_spring_boundary", "UTC", tzSpringLABoundary, "2026-03-09"},
		{"local_utc_fall_boundary", "UTC", tzFallLABoundary, "2026-11-02"},
		{"local_utc_spring_afternoon", "UTC", tzSpringLAAfternoon, "2026-03-08"},
	} {
		m := m
		cases = append(cases, tzCheck(m.id, fmt.Sprintf("activityLocalDate %s in %s", m.at.Format(time.RFC3339), m.tz), func() (map[string]any, map[string]any, error) {
			gotDate := probeActivityLocalDate(domain.Activity{EffectiveAt: m.at, EffectiveLocalDate: "1999-01-01"}, m.tz)
			expected := map[string]any{"localDate": m.want, "timezone": m.tz}
			got := map[string]any{"localDate": gotDate, "timezone": m.tz}
			if gotDate != m.want {
				return expected, got, fmt.Errorf("got %s want %s", gotDate, m.want)
			}
			return expected, got, nil
		}))
	}

	for _, tc := range []struct {
		id, date string
		hours    float64
	}{
		{"la_day_length_spring", "2026-03-08", 23},
		{"la_day_length_fall", "2026-11-01", 25},
	} {
		tc := tc
		cases = append(cases, tzCheck(tc.id, fmt.Sprintf("America/Los_Angeles %s local day length", tc.date), func() (map[string]any, map[string]any, error) {
			loc := mustLoc("America/Los_Angeles")
			localDate, err := time.ParseInLocation("2006-01-02", tc.date, loc)
			if err != nil {
				return nil, nil, err
			}
			start := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
			end := start.AddDate(0, 0, 1)
			gotHours := end.Sub(start).Hours()
			expected := map[string]any{"hours": tc.hours}
			got := map[string]any{"hours": gotHours, "start": start.UTC().Format(time.RFC3339), "end": end.UTC().Format(time.RFC3339)}
			if gotHours != tc.hours {
				return expected, got, fmt.Errorf("day length=%v want %v", gotHours, tc.hours)
			}
			return expected, got, nil
		}))
	}

	cases = append(cases, tzCheck("la_origin_bounds_spring_fall", "ResolveLocalDateTime LA 00:00 bounds 23h / 25h", func() (map[string]any, map[string]any, error) {
		loc := mustLoc("America/Los_Angeles")
		springStart, err := domain.ResolveLocalDateTime("2026-03-08", "00:00", loc.String())
		if err != nil {
			return nil, nil, err
		}
		springEnd, err := domain.ResolveLocalDateTime("2026-03-09", "00:00", loc.String())
		if err != nil {
			return nil, nil, err
		}
		fallStart, err := domain.ResolveLocalDateTime("2026-11-01", "00:00", loc.String())
		if err != nil {
			return nil, nil, err
		}
		fallEnd, err := domain.ResolveLocalDateTime("2026-11-02", "00:00", loc.String())
		if err != nil {
			return nil, nil, err
		}
		expected := map[string]any{"springHours": 23, "fallHours": 25}
		got := map[string]any{"springHours": springEnd.Sub(springStart).Hours(), "fallHours": fallEnd.Sub(fallStart).Hours()}
		if springEnd.Sub(springStart) != 23*time.Hour || fallEnd.Sub(fallStart) != 25*time.Hour {
			return expected, got, fmt.Errorf("spring=%s fall=%s", springEnd.Sub(springStart), fallEnd.Sub(fallStart))
		}
		return expected, got, nil
	}))
	return cases
}

func tzInMemoryAnalysisCases() []tzCase {
	return []tzCase{
		tzCheck("analysis_la_spring_assigns_day", "ComputeAnalysis assigns 2026-03-09T06:30Z to LA 2026-03-08", func() (map[string]any, map[string]any, error) {
			return tzAssertAssignedDay("America/Los_Angeles", tzSpringLABoundary, "2026-03-07", "2026-03-08", "2026-03-09")
		}),
		tzCheck("analysis_la_fall_assigns_day", "ComputeAnalysis assigns 2026-11-02T07:30Z to LA 2026-11-01", func() (map[string]any, map[string]any, error) {
			return tzAssertAssignedDay("America/Los_Angeles", tzFallLABoundary, "2026-10-31", "2026-11-01", "2026-11-02")
		}),
		tzCheck("analysis_la_spring_dietz_noon_weight", "LA spring-forward noon Dietz weight uses 23h day", func() (map[string]any, map[string]any, error) {
			return tzAssertNoonDietz("2026-03-08", 23)
		}),
		tzCheck("analysis_la_fall_dietz_noon_weight", "LA fall-back noon Dietz weight uses 25h day", func() (map[string]any, map[string]any, error) {
			return tzAssertNoonDietz("2026-11-01", 25)
		}),
	}
}

func tzAssertAssignedDay(timezone string, at time.Time, prevDate, wantDate, otherDate string) (map[string]any, map[string]any, error) {
	hh := domain.NewHouseholdID()
	account := domain.Account{
		ID: domain.NewAccountID(), HouseholdID: hh, Name: "cash",
		AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingBalance, DefaultCurrency: "AUD", IncludeInNetWorth: true,
	}
	origin, err := domain.NewHistoryOrigin(hh, timezone, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return nil, nil, err
	}
	prev := tzSnapshot(prevDate, "AUD", tzItem(account.ID, "AUD", "100"))
	cur := tzSnapshot(wantDate, "AUD", tzItem(account.ID, "AUD", "200"))
	other := tzSnapshot(otherDate, "AUD", tzItem(account.ID, "AUD", "200"))
	activityID := domain.NewActivityID()
	amount := moneyMust("100", "AUD")
	activity := domain.Activity{
		ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonContribution,
		EffectiveAt: at, EffectiveLocalDate: "1999-01-01",
		Effects: []domain.ActivityEffect{{
			ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1,
			Role: domain.EffectRoleAmount, Direction: domain.EffectAdded,
			Target: domain.EffectTargetAccountValue, Classification: domain.ClassificationExternalInflow,
			AccountID: &account.ID, Money: &amount,
		}},
	}
	input := application.AnalysisInputs{
		Origin: origin,
		Portfolio: domain.PortfolioSnapshot{
			Household: &domain.Household{ID: hh, BaseCurrency: "AUD"},
			Accounts:  []domain.AccountRecord{{Account: account}},
		},
		Snapshots:  []domain.DailyValuationSnapshot{prev, cur, other},
		Activities: []domain.Activity{activity},
	}
	wantResult, err := application.ComputeAnalysis(input, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  wantDate, To: wantDate, Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	})
	if err != nil {
		return nil, nil, err
	}
	otherResult, err := application.ComputeAnalysis(input, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  otherDate, To: otherDate, Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	})
	if err != nil {
		return nil, nil, err
	}
	wantFlow := tzBucketSum(wantResult, wantDate, domain.BucketExternalFlow)
	otherFlow := tzBucketSum(otherResult, otherDate, domain.BucketExternalFlow)
	expected := map[string]any{"assignedDate": wantDate, "externalFlow": "100", "otherDateFlow": "0"}
	got := map[string]any{"assignedDate": wantDate, "externalFlow": wantFlow.String(), "otherDateFlow": otherFlow.String(), "activityLocalDate": probeActivityLocalDate(activity, timezone)}
	if !wantFlow.Equal(decimal.NewFromInt(100)) {
		return expected, got, fmt.Errorf("want-day external flow=%s want 100", wantFlow)
	}
	if !otherFlow.IsZero() {
		return expected, got, fmt.Errorf("other-day external flow=%s want 0", otherFlow)
	}
	return expected, got, nil
}

func tzAssertNoonDietz(date string, hours int64) (map[string]any, map[string]any, error) {
	location := mustLoc("America/Los_Angeles")
	householdID := domain.NewHouseholdID()
	account := domain.Account{
		ID: domain.NewAccountID(), HouseholdID: householdID, Name: "cash",
		AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true,
	}
	previousDate := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)
	if date == "2026-11-01" {
		previousDate = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	}
	prev := domain.DailyValuationSnapshot{
		LocalDate: previousDate.Format("2006-01-02"),
		CutoffAt:  previousDate.Add(23*time.Hour + 59*time.Minute),
		Currency:  "USD", Complete: true,
		Items: []domain.DailyValuationSnapshotItem{tzItem(account.ID, "USD", "100")},
	}
	current := domain.DailyValuationSnapshot{
		LocalDate: date,
		CutoffAt:  previousDate.AddDate(0, 0, 1).Add(23*time.Hour + 59*time.Minute),
		Currency:  "USD", Complete: true,
		Items: []domain.DailyValuationSnapshotItem{tzItem(account.ID, "USD", "200")},
	}
	noon := time.Date(previousDate.Year(), previousDate.Month(), previousDate.Day()+1, 12, 0, 0, 0, location)
	activityID := domain.NewActivityID()
	amount := moneyMust("100", "USD")
	activity := domain.Activity{
		ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonContribution,
		EffectiveAt: noon, EffectiveLocalDate: "1999-01-01",
		Effects: []domain.ActivityEffect{{
			ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1,
			Role: domain.EffectRoleAmount, Direction: domain.EffectAdded,
			Target: domain.EffectTargetAccountValue, Classification: domain.ClassificationExternalInflow,
			AccountID: &account.ID, Money: &amount,
		}},
	}
	origin, err := domain.NewHistoryOrigin(householdID, location.String(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return nil, nil, err
	}
	input := application.AnalysisInputs{
		Origin: origin,
		Portfolio: domain.PortfolioSnapshot{
			Household: &domain.Household{ID: householdID, BaseCurrency: "USD"},
			Accounts:  []domain.AccountRecord{{Account: account}},
		},
		Snapshots:  []domain.DailyValuationSnapshot{prev, current},
		Activities: []domain.Activity{activity},
	}
	result, err := application.ComputeAnalysis(input, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  date, To: date, Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	})
	if err != nil {
		return nil, nil, err
	}
	if len(result.Days) == 0 {
		return nil, nil, fmt.Errorf("no component days")
	}
	want, err := domain.NewSignedMoney(decimal.NewFromInt(100).Mul(decimal.NewFromInt(12).Div(decimal.NewFromInt(hours))), "USD")
	if err != nil {
		return nil, nil, err
	}
	gotAmt := result.Days[0].DietzFlow.Amount()
	expected := map[string]any{"dietzFlow": want.CanonicalAmount(), "hours": hours}
	got := map[string]any{"dietzFlow": gotAmt.String(), "hours": hours}
	if !gotAmt.Equal(want.Amount()) {
		return expected, got, fmt.Errorf("DietzFlow=%s want %s (12/%dh)", gotAmt, want.Amount(), hours)
	}
	return expected, got, nil
}

func tzBuildOriginDBs(dbRoot string) []tzCase {
	cases := make([]tzCase, 0, 6)
	for _, tz := range []string{"Asia/Singapore", "UTC", "America/Los_Angeles"} {
		tz := tz
		dbPath := filepath.Join(dbRoot, tzDirName(tz), "data", "nestworth.db")
		persisted, assigned, err := buildTimezoneSpringDB(dbPath, tz, tzSpringLABoundary)
		wantLocal := probeActivityLocalDate(domain.Activity{EffectiveAt: tzSpringLABoundary}, tz)
		expected := map[string]any{"timezone": tz, "utc": tzSpringLABoundary.Format(time.RFC3339), "localDate": wantLocal}
		got := map[string]any{"timezone": persisted, "assignedLocalDate": assigned, "dbPath": dbPath}
		status := "PASS"
		detail := fmt.Sprintf("origin=%s persisted_local=%s analysis=%s db=%s", tz, assigned, wantLocal, dbPath)
		if err != nil {
			status = "FAIL"
			detail = err.Error()
		} else if persisted != tz || assigned != wantLocal {
			status = "FAIL"
			detail = fmt.Sprintf("persisted tz=%s want=%s assigned=%s wantLocal=%s", persisted, tz, assigned, wantLocal)
		}
		cases = append(cases, tzCase{
			ID: "db_origin_" + tzDirName(tz), Name: "HistoryOrigin timezone " + tz + " spring UTC instant",
			Status: status, Expected: expected, Got: got, Detail: detail,
		})
	}
	return cases
}

func buildTimezoneSpringDB(dbPath, timezone string, at time.Time) (persistedTZ, assignedLocal string, err error) {
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	database, err := sqlite.Open(dbPath)
	if err != nil {
		return "", "", err
	}
	defer database.Close()
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Timezone R4 " + timezone,
		BaseCurrency:  "AUD",
		MemberNames:   []string{"Weichen"},
		Timezone:      "",
	}); err != nil {
		return "", "", err
	}
	bootstrap, err := svc.Bootstrap(ctx)
	if err != nil {
		return "", "", err
	}
	owner := bootstrap.Members[0].ID
	cash, err := svc.CreateAccount(ctx, application.AccountInput{
		Name: "AUD Cash", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "1000",
		IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{owner},
	})
	if err != nil {
		return "", "", err
	}
	origin, err := svc.StartHistory(ctx, timezone)
	if err != nil {
		return "", "", err
	}
	originAt, err := domain.ResolveLocalDateTime("2026-03-01", "00:00", timezone)
	if err != nil {
		return "", "", err
	}
	originISO := originAt.Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(`UPDATE history_origins SET started_at = ?, created_at = ?`, originISO, originISO); err != nil {
		return "", "", err
	}
	if _, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID, AccountID: cash.Account.ID,
		Amount: moneyMust("100", "AUD"), Reason: domain.ReasonContribution, EffectiveAt: at,
	}); err != nil {
		return "", "", err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", "", err
	}
	wantLocal := at.In(loc).Format("2006-01-02")
	start := "2026-03-01"
	end := "2026-03-10"
	if _, err := svc.RebuildHistoricalSnapshots(ctx, start, end); err != nil {
		return "", "", err
	}
	if err := svc.CompleteDailySnapshotRange(ctx, bootstrap.Household.ID, end); err != nil {
		return "", "", err
	}
	var storedTZ, storedLocal string
	if err := database.SQL.QueryRow(`SELECT timezone FROM history_origins WHERE id = ?`, origin.ID.String()).Scan(&storedTZ); err != nil {
		return "", "", err
	}
	_ = database.SQL.QueryRow(`SELECT effective_local_date FROM activities WHERE kind = 'cash_in' LIMIT 1`).Scan(&storedLocal)
	result, err := svc.Analyze(ctx, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  wantLocal, To: wantLocal, Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	})
	if err != nil {
		return storedTZ, storedLocal, err
	}
	flow := tzBucketSum(result, wantLocal, domain.BucketExternalFlow)
	if !flow.Equal(decimal.NewFromInt(100)) {
		return storedTZ, storedLocal, fmt.Errorf("analysis %s external flow=%s want 100 (tz=%s)", wantLocal, flow, timezone)
	}
	if storedLocal != wantLocal {
		return storedTZ, storedLocal, fmt.Errorf("persisted effective_local_date=%s want %s", storedLocal, wantLocal)
	}
	return storedTZ, storedLocal, nil
}

func tzSnapshot(date, currency string, items ...domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshot {
	parsed, _ := time.Parse("2006-01-02", date)
	return domain.DailyValuationSnapshot{
		ID: domain.NewDailyValuationSnapshotID(), LocalDate: date,
		CutoffAt: parsed.Add(23*time.Hour + 59*time.Minute),
		Currency: domain.CurrencyCode(currency), Complete: true, Items: items,
	}
}

func tzItem(accountID domain.AccountID, currency, amount string) domain.DailyValuationSnapshotItem {
	base := moneyMust(amount, currency)
	return domain.DailyValuationSnapshotItem{
		ID: domain.NewDailyValuationSnapshotItemID(), AccountID: accountID,
		NativeAmount: amount, NativeCurrency: domain.CurrencyCode(currency),
		BaseAmount: &base, BaseAmountExact: amount, Complete: true,
	}
}

func tzBucketSum(result domain.PeriodAnalysisResult, date string, bucket domain.AttributionBucket) decimal.Decimal {
	total := decimal.Zero
	for _, day := range result.Days {
		if string(day.Date) != date {
			continue
		}
		if v, ok := day.AssetBuckets[bucket]; ok {
			total = total.Add(v.Amount())
		}
	}
	return total
}

func probeActivityLocalDate(activity domain.Activity, timezone string) string {
	if timezone == "" {
		return activity.EffectiveAt.UTC().Format("2006-01-02")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return activity.EffectiveAt.UTC().Format("2006-01-02")
	}
	return activity.EffectiveAt.In(location).Format("2006-01-02")
}

func tzDirName(timezone string) string {
	return strings.ToLower(strings.ReplaceAll(timezone, "/", "-"))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func tzRejectLocal(id, name, date, clock, timezone string) tzCase {
	return tzCheck(id, name, func() (map[string]any, map[string]any, error) {
		_, err := domain.ResolveLocalDateTime(date, clock, timezone)
		got := map[string]any{"error": errString(err), "timezone": timezone, "local": date + " " + clock}
		if err == nil {
			return map[string]any{"code": domain.ErrInvalidChangeTime}, got, fmt.Errorf("invalid local time was accepted")
		}
		typed, ok := err.(*domain.Error)
		if !ok || typed.Code != domain.ErrInvalidChangeTime {
			return map[string]any{"code": domain.ErrInvalidChangeTime}, got, fmt.Errorf("error = %v", err)
		}
		return map[string]any{"code": domain.ErrInvalidChangeTime}, got, nil
	})
}

func tzCheck(id, name string, fn func() (map[string]any, map[string]any, error)) tzCase {
	expected, got, err := fn()
	c := tzCase{ID: id, Name: name, Expected: expected, Got: got, Status: "PASS"}
	if err != nil {
		c.Status = "FAIL"
		c.Detail = err.Error()
	}
	return c
}

func writeTZProbe(outPath string, out tzProbeOut, cases []tzCase) error {
	out.Results = cases
	pass, fail := 0, 0
	all := true
	for _, c := range cases {
		if c.Status != "PASS" {
			all = false
			fail++
		} else {
			pass++
		}
	}
	out.AllPass = all
	out.Summary = fmt.Sprintf("%d PASS / %d FAIL", pass, fail)
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Printf("timezone-r4 written %s\n%s\n", outPath, string(raw))
	if !all {
		return fmt.Errorf("timezone-r4 probe failed: %s", out.Summary)
	}
	return nil
}
