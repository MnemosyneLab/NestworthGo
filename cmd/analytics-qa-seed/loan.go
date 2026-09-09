package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

const (
	loanFC07Scenario      = "loan-fc07"
	loanFixtureVersion    = "analytics-linux-qa-loan-fc07"
	loanDrawDay           = 1
	loanRepayDay          = 3
	loanInterestDay       = 5
	loanQuoteHorizon      = 8
	loanCashInitial       = "20000"
	loanDrawPrincipal     = "100000"
	loanRepayPrincipal    = "10000"
	loanInterestPrincipal = "1000"
	loanInterestFee       = "500"
	loanCurrency          = "AUD"
)

func runLoanFC07(base, dbPath string) {
	anchor := parseAnchor(envOr("NESTWORTH_QA_ANCHOR", defaultQAAnchor))
	database := openResetQADatabase(base, dbPath)
	defer database.Close()
	repo := sqlite.NewRepository(database)
	svc := application.NewService(repo)
	ctx := context.Background()

	commit, dirty := gitIdentity()
	report := &Report{
		FixtureVersion: loanFixtureVersion,
		Scenario:       loanFC07Scenario,
		Anchor:         anchor.Format(time.RFC3339),
		Commit:         commit,
		Dirty:          dirty,
		DBPath:         dbPath,
		OutputDir:      base,
		IDs:            map[string]string{},
	}

	must0 := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	sgt := must(time.LoadLocation("Asia/Singapore"))
	localDate := func(t time.Time) string { return t.In(sgt).Format("2006-01-02") }
	day := func(offset int) time.Time {
		return anchor.Add(time.Duration(offset) * 24 * time.Hour).UTC()
	}

	must0(svc.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Analytics QA Loan FC-07",
		BaseCurrency:  loanCurrency,
		MemberNames:   []string{"Weichen"},
		Timezone:      "",
	}))
	bootstrap := must(svc.Bootstrap(ctx))
	hh := bootstrap.Household.ID
	m1 := bootstrap.Members[0].ID
	report.IDs["household"] = string(hh)
	report.IDs["member"] = string(m1)
	report.IDs["fixture_version"] = loanFixtureVersion
	report.IDs["base_currency"] = loanCurrency
	add(report, "onboarding", "PASS", fmt.Sprintf("household=%s member=%s base=%s tz=(empty, no history yet)", hh, m1, loanCurrency))

	cash := must(svc.CreateAccount(ctx, application.AccountInput{
		Name: "AUD Cash", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: loanCurrency, InitialAmount: loanCashInitial,
		IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{m1},
	}))
	loan := must(svc.CreateAccount(ctx, application.AccountInput{
		Name: "AUD Loan", AccountType: "loan", BalanceSheetRole: "liability",
		TrackingMode: "balance", DefaultCurrency: loanCurrency, InitialAmount: "0",
		IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{m1},
	}))
	report.IDs["acct_cash"] = string(cash.Account.ID)
	report.IDs["acct_loan"] = string(loan.Account.ID)
	add(report, "accounts", "PASS", fmt.Sprintf("cash=%s loan=%s initial_cash=%s %s loan_initial=0", cash.Account.ID, loan.Account.ID, loanCashInitial, loanCurrency))

	origin := must(svc.StartHistory(ctx, "Asia/Singapore"))
	compCount := countQuery(database.SQL, `SELECT COUNT(*) FROM history_origin_components`)
	report.IDs["history_origin_id"] = string(origin.ID)
	if compCount <= 0 {
		add(report, "start_history", "FAIL", fmt.Sprintf("history_origin_components=%d want >0", compCount))
	} else {
		add(report, "start_history", "PASS", fmt.Sprintf("tz=Asia/Singapore components=%d origin=%s", compCount, origin.ID))
	}

	originISO := anchor.Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(`UPDATE history_origins SET started_at = ?, created_at = ?`, originISO, originISO); err != nil {
		panic(err)
	}
	report.IDs["history_origin"] = anchor.Format(time.RFC3339)
	add(report, "backdate_origin", "PASS", fmt.Sprintf("started_at=%s anchor=%s", originISO, report.Anchor))

	drawAt, repayAt, interestAt := day(loanDrawDay), day(loanRepayDay), day(loanInterestDay)
	drawLocal, repayLocal, interestLocal := localDate(drawAt), localDate(repayAt), localDate(interestAt)
	report.IDs["draw_day_offset"] = fmt.Sprintf("%d", loanDrawDay)
	report.IDs["draw_local_date"] = drawLocal
	report.IDs["repay_day_offset"] = fmt.Sprintf("%d", loanRepayDay)
	report.IDs["repay_local_date"] = repayLocal
	report.IDs["interest_day_offset"] = fmt.Sprintf("%d", loanInterestDay)
	report.IDs["interest_local_date"] = interestLocal
	report.IDs["draw_principal"] = loanDrawPrincipal
	report.IDs["repay_principal"] = loanRepayPrincipal
	report.IDs["interest_principal"] = loanInterestPrincipal
	report.IDs["interest_or_fee"] = loanInterestFee

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.DebtDrawInput{
			HouseholdID: hh, DebtAccountID: loan.Account.ID, CashAccountID: cash.Account.ID,
			Principal: money(loanDrawPrincipal, loanCurrency), EffectiveAt: drawAt,
		})
		return err
	}())
	add(report, "act_draw", "PASS", fmt.Sprintf("FC-07 draw %s %s local=%s", loanDrawPrincipal, loanCurrency, drawLocal))

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.DebtPaymentInput{
			HouseholdID: hh, DebtAccountID: loan.Account.ID, CashAccountID: cash.Account.ID,
			Principal: money(loanRepayPrincipal, loanCurrency), EffectiveAt: repayAt,
		})
		return err
	}())
	add(report, "act_repay", "PASS", fmt.Sprintf("FC-07 repay principal %s %s local=%s", loanRepayPrincipal, loanCurrency, repayLocal))

	interest := money(loanInterestFee, loanCurrency)
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.DebtPaymentInput{
			HouseholdID: hh, DebtAccountID: loan.Account.ID, CashAccountID: cash.Account.ID,
			Principal: money(loanInterestPrincipal, loanCurrency), InterestOrFee: &interest, EffectiveAt: interestAt,
		})
		return err
	}())
	add(report, "act_interest", "PASS", fmt.Sprintf("FC-07 payment principal %s + interest/fee %s %s local=%s", loanInterestPrincipal, loanInterestFee, loanCurrency, interestLocal))

	drawN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'debt_draw'`)
	payN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'debt_payment'`)
	feeN := countQuery(database.SQL, `SELECT COUNT(*) FROM activity_effects WHERE role = 'fee'`)
	if drawN != 1 || payN != 2 || feeN != 1 {
		add(report, "verify_loan_activities", "FAIL", fmt.Sprintf("debt_draw=%d want=1 debt_payment=%d want=2 fee_effects=%d want=1", drawN, payN, feeN))
	} else {
		add(report, "verify_loan_activities", "PASS", fmt.Sprintf("debt_draw=%d debt_payment=%d fee_effects=%d", drawN, payN, feeN))
	}

	startDate := localDate(anchor)
	endDate := localDate(day(loanQuoteHorizon))
	cursor, _ := time.Parse("2006-01-02", startDate)
	endT, _ := time.Parse("2006-01-02", endDate)
	totalSnap := 0
	var rebuildErr error
	for !cursor.After(endT) {
		chunkEnd := cursor.Add(30 * 24 * time.Hour)
		if chunkEnd.After(endT) {
			chunkEnd = endT
		}
		n, err := svc.RebuildHistoricalSnapshots(ctx, cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
		if err != nil {
			rebuildErr = err
			break
		}
		totalSnap += n
		cursor = chunkEnd.Add(24 * time.Hour)
	}
	report.IDs["snapshot_rebuild_count"] = fmt.Sprintf("%d", totalSnap)
	if rebuildErr != nil {
		add(report, "rebuild_snapshots", "FAIL", rebuildErr.Error())
	} else {
		add(report, "rebuild_snapshots", "PASS", fmt.Sprintf("rebuilt=%d range=%s..%s", totalSnap, startDate, endDate))
	}

	if rebuildErr == nil {
		if err := svc.CompleteDailySnapshotRange(ctx, hh, endDate); err != nil {
			add(report, "complete_daily_range", "FAIL", err.Error())
		} else {
			add(report, "complete_daily_range", "PASS", fmt.Sprintf("targetDate=%s", endDate))
		}
		assertLoanFC07(report, database, svc, ctx, drawLocal, repayLocal, interestLocal)
	}

	writeAUDSettings(report, base)
	finishSeed(report)
}

func writeAUDSettings(report *Report, base string) {
	settingsPath := filepath.Join(base, "data", "settings.json")
	settings := map[string]any{
		"schema_version":     1,
		"appearance":         "system",
		"accent":             "nestworth",
		"language":           "en",
		"timezone":           "system",
		"week_start":         "monday",
		"date_format":        "iso",
		"time_format":        "24h",
		"currency":           loanCurrency,
		"decimal_separator":  ".",
		"grouping_separator": ",",
		"decimal_places":     2,
		"window_width":       1280,
		"window_height":      720,
		"fx_provider":        "frankfurter",
		"quote_cache_ttl":    "12h",
	}
	sb, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, append(sb, '\n'), 0o644); err != nil {
		add(report, "settings_aud", "FAIL", err.Error())
		return
	}
	add(report, "settings_aud", "PASS", fmt.Sprintf("wrote %s currency=%s", settingsPath, loanCurrency))
}

func assertLoanFC07(report *Report, database *sqlite.DB, svc *application.Service, ctx context.Context, drawLocal, repayLocal, interestLocal string) {
	drawPrev := dateMinusDays(drawLocal, 1)
	repayPrev := dateMinusDays(repayLocal, 1)
	interestPrev := dateMinusDays(interestLocal, 1)

	assertNWNeutral(report, database, svc, ctx, "fc07_draw_nw0", drawPrev, drawLocal, "0", "0")
	assertNWNeutral(report, database, svc, ctx, "fc07_repay_nw0", repayPrev, repayLocal, "0", "0")
	assertInterestSpending(report, database, svc, ctx, interestPrev, interestLocal)
}

func assertNWNeutral(report *Report, database *sqlite.DB, svc *application.Service, ctx context.Context, name, prevDate, date, wantNWDelta, wantSpending string) {
	prevNW, prevOK := snapshotNetWorth(database.SQL, prevDate)
	curNW, curOK := snapshotNetWorth(database.SQL, date)
	if !prevOK || !curOK {
		add(report, name, "FAIL", fmt.Sprintf("missing snapshots prev=%s ok=%v date=%s ok=%v", prevDate, prevOK, date, curOK))
		return
	}
	delta := curNW.Sub(prevNW)
	wantDelta := decimal.RequireFromString(wantNWDelta)
	got, err := analyzeLoanDay(ctx, svc, date)
	if err != nil {
		add(report, name, "FAIL", fmt.Sprintf("analyze %s: %v", date, err))
		return
	}
	if !delta.Equal(wantDelta) {
		add(report, name, "FAIL", fmt.Sprintf("snapshot net_worth %s→%s delta=%s want=%s prev=%s cur=%s", prevDate, date, delta, wantDelta, prevNW, curNW))
		return
	}
	if !got.nwDelta.Equal(wantDelta) {
		add(report, name, "FAIL", fmt.Sprintf("analyze net-worth delta=%s want=%s begin=%s end=%s", got.nwDelta, wantDelta, got.begin, got.end))
		return
	}
	if got.residual {
		add(report, name, "FAIL", fmt.Sprintf("residual present on %s", date))
		return
	}
	if !got.spending.Equal(decimal.RequireFromString(wantSpending)) {
		add(report, name, "FAIL", fmt.Sprintf("spending=%s want=%s", got.spending, wantSpending))
		return
	}
	if !got.dividendInterest.IsZero() {
		add(report, name, "FAIL", fmt.Sprintf("dividend_interest=%s want=0 (must not mis-bucket principal)", got.dividendInterest))
		return
	}
	if !got.returnAmount.IsZero() {
		add(report, name, "FAIL", fmt.Sprintf("investment return=%s want=0 (principal is net-worth neutral, not return)", got.returnAmount))
		return
	}
	add(report, name, "PASS", fmt.Sprintf("date=%s snapshot_nw %s→%s delta=0 analyze_nw_delta=0 spending=0 return=0 residual=0", date, prevNW, curNW))
}

func assertInterestSpending(report *Report, database *sqlite.DB, svc *application.Service, ctx context.Context, prevDate, date string) {
	const name = "fc07_interest_spending"
	wantNW := decimal.RequireFromString("-" + loanInterestFee)
	prevNW, prevOK := snapshotNetWorth(database.SQL, prevDate)
	curNW, curOK := snapshotNetWorth(database.SQL, date)
	if !prevOK || !curOK {
		add(report, name, "FAIL", fmt.Sprintf("missing snapshots prev=%s ok=%v date=%s ok=%v", prevDate, prevOK, date, curOK))
		return
	}
	delta := curNW.Sub(prevNW)
	got, err := analyzeLoanDay(ctx, svc, date)
	if err != nil {
		add(report, name, "FAIL", fmt.Sprintf("analyze %s: %v", date, err))
		return
	}
	if !delta.Equal(wantNW) {
		add(report, name, "FAIL", fmt.Sprintf("snapshot net_worth %s→%s delta=%s want=%s", prevDate, date, delta, wantNW))
		return
	}
	if !got.nwDelta.Equal(wantNW) {
		add(report, name, "FAIL", fmt.Sprintf("analyze net-worth delta=%s want=%s begin=%s end=%s", got.nwDelta, wantNW, got.begin, got.end))
		return
	}
	if got.residual {
		add(report, name, "FAIL", fmt.Sprintf("residual present on %s", date))
		return
	}
	if !got.spending.Equal(wantNW) {
		add(report, name, "FAIL", fmt.Sprintf("spending=%s want=%s", got.spending, wantNW))
		return
	}
	if !got.dividendInterest.IsZero() {
		add(report, name, "FAIL", fmt.Sprintf("dividend_interest=%s want=0 (cash loan interest is Spending, not Dividend & Interest)", got.dividendInterest))
		return
	}
	if !got.returnAmount.IsZero() {
		add(report, name, "FAIL", fmt.Sprintf("investment return=%s want=0 (interest must not inflate return)", got.returnAmount))
		return
	}
	cats, catErr := svc.Categories(ctx, loanDayQuery(date), application.CategorySpending)
	if catErr != nil {
		add(report, name, "FAIL", fmt.Sprintf("categories spending: %v", catErr))
		return
	}
	if cats.Total == nil || !cats.Total.Amount().Equal(wantNW) {
		gotTotal := "<nil>"
		if cats.Total != nil {
			gotTotal = cats.Total.CanonicalAmount()
		}
		add(report, name, "FAIL", fmt.Sprintf("categories spending total=%s want=%s", gotTotal, wantNW))
		return
	}
	add(report, name, "PASS", fmt.Sprintf("date=%s nw_delta=%s spending=%s categories=%s return=0 dividend_interest=0 residual=0", date, delta, got.spending, cats.Total.CanonicalAmount()))
}

type loanDayMetrics struct {
	begin, end, nwDelta        decimal.Decimal
	spending, dividendInterest decimal.Decimal
	returnAmount               decimal.Decimal
	residual                   bool
}

func loanDayQuery(date string) domain.AnalysisQuery {
	return domain.AnalysisQuery{
		Scope:       domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:        date,
		To:          date,
		Valuation:   domain.ValuationBase,
		IncludeCash: true,
		Basis:       domain.ReturnBasisInvestment,
	}
}

func analyzeLoanDay(ctx context.Context, svc *application.Service, date string) (loanDayMetrics, error) {
	result, err := svc.Analyze(ctx, loanDayQuery(date))
	if err != nil {
		return loanDayMetrics{}, err
	}
	out := loanDayMetrics{}
	for _, day := range result.Days {
		if string(day.Date) != date {
			continue
		}
		if day.BeginningValue.Currency() != "" {
			out.begin = out.begin.Add(day.BeginningValue.Amount())
		}
		if day.EndingValue.Currency() != "" {
			out.end = out.end.Add(day.EndingValue.Amount())
		}
		if v, ok := day.AssetBuckets[domain.BucketSpending]; ok {
			out.spending = out.spending.Add(v.Amount())
		}
		if v, ok := day.AssetBuckets[domain.BucketDividendInterest]; ok {
			out.dividendInterest = out.dividendInterest.Add(v.Amount())
		}
		if day.Residual != nil {
			out.residual = true
		}
	}
	out.nwDelta = out.end.Sub(out.begin)
	if result.ReturnAmount != nil {
		out.returnAmount = result.ReturnAmount.Amount()
	}
	return out, nil
}

func snapshotNetWorth(db *sql.DB, localDate string) (decimal.Decimal, bool) {
	var amount string
	err := db.QueryRow(`
		SELECT COALESCE(net_worth_amount, '') FROM daily_valuation_snapshots s
		WHERE local_date = ? AND revision = (
			SELECT MAX(revision) FROM daily_valuation_snapshots s2
			WHERE s2.household_id = s.household_id AND s2.local_date = s.local_date
		)
	`, localDate).Scan(&amount)
	if err != nil || amount == "" {
		return decimal.Zero, false
	}
	value, parseErr := decimal.NewFromString(amount)
	if parseErr != nil {
		return decimal.Zero, false
	}
	return value, true
}

func dateMinusDays(localDate string, days int) string {
	parsed, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		panic(err)
	}
	return parsed.AddDate(0, 0, -days).Format("2006-01-02")
}
