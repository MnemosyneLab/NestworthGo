package main

import (
	"context"
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

type loanProbeCase struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Expected map[string]any `json:"expected"`
	Got      map[string]any `json:"got"`
	Detail   string         `json:"detail,omitempty"`
}

type loanProbeOut struct {
	When              string          `json:"when"`
	Mode              string          `json:"mode"`
	DBPath            string          `json:"dbPath"`
	DrawLocalDate     string          `json:"drawLocalDate"`
	RepayLocalDate    string          `json:"repayLocalDate"`
	InterestLocalDate string          `json:"interestLocalDate"`
	Results           []loanProbeCase `json:"results"`
	AllPass           bool            `json:"allPass"`
	Summary           string          `json:"summary"`
}

type loanSeedIDs struct {
	DrawLocalDate     string `json:"draw_local_date"`
	RepayLocalDate    string `json:"repay_local_date"`
	InterestLocalDate string `json:"interest_local_date"`
	AcctCash          string `json:"acct_cash"`
	AcctLoan          string `json:"acct_loan"`
	Timezone          string `json:"timezone"`
	BaseCurrency      string `json:"base_currency"`
	WeekStart         string `json:"week_start"`
}

func runLoanFC07Probe() error {
	dbPath := envOr("NESTWORTH_DATABASE_PATH", filepath.Join("/workspace/nestworth-analytics-qa", "data", "nestworth.db"))
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/seed/probe-loan-fc07.json")

	ids := loadLoanSeedIDs(dbPath)
	drawDate, repayDate, interestDate := loanProbeDates(ids)
	if drawDate == "" || repayDate == "" || interestDate == "" {
		return fmt.Errorf("loan-fc07 probe needs draw/repay/interest local dates")
	}

	out := loanProbeOut{
		When:              time.Now().UTC().Format(time.RFC3339),
		Mode:              "NESTWORTH_PROBE_MODE=loan-fc07",
		DBPath:            dbPath,
		DrawLocalDate:     drawDate,
		RepayLocalDate:    repayDate,
		InterestLocalDate: interestDate,
	}

	database, _, _, err := openProbeDB(dbPath)
	if err != nil {
		return writeLoanProbe(outPath, out, []loanProbeCase{{
			ID: "open_db", Name: "open loan DB", Status: "FAIL", Detail: err.Error(),
		}})
	}
	defer database.Close()
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	return writeLoanProbe(outPath, out, collectLoanFC07Cases(ctx, svc, dbPath, ids, drawDate, repayDate, interestDate))
}

func loanProbeDates(ids *loanSeedIDs) (draw, repay, interest string) {
	draw = envOr("NESTWORTH_PROBE_DRAW_DATE", "")
	repay = envOr("NESTWORTH_PROBE_REPAY_DATE", "")
	interest = envOr("NESTWORTH_PROBE_INTEREST_DATE", "")
	tzName := "Asia/Singapore"
	if ids != nil {
		if draw == "" {
			draw = ids.DrawLocalDate
		}
		if repay == "" {
			repay = ids.RepayLocalDate
		}
		if interest == "" {
			interest = ids.InterestLocalDate
		}
		if ids.Timezone != "" {
			tzName = ids.Timezone
		}
	}
	if draw != "" && repay != "" && interest != "" {
		return draw, repay, interest
	}
	loc := mustLoc(tzName)
	anchor, err := time.Parse(time.RFC3339, envOr("NESTWORTH_QA_ANCHOR", "2026-07-26T00:00:00Z"))
	if err != nil {
		return draw, repay, interest
	}
	local := func(offset int) string {
		return anchor.Add(time.Duration(offset) * 24 * time.Hour).In(loc).Format("2006-01-02")
	}
	if draw == "" {
		draw = local(1)
	}
	if repay == "" {
		repay = local(3)
	}
	if interest == "" {
		interest = local(5)
	}
	return draw, repay, interest
}

func collectLoanFC07Cases(ctx context.Context, svc *application.Service, dbPath string, ids *loanSeedIDs, drawDate, repayDate, interestDate string) []loanProbeCase {
	cases := []loanProbeCase{
		probeLoanNeutral(ctx, svc, "fc07_draw_nw0", "draw net-worth neutrality", drawDate, "0", "0"),
		probeLoanNeutral(ctx, svc, "fc07_repay_nw0", "repay principal net-worth neutrality", repayDate, "0", "0"),
		probeLoanInterest(ctx, svc, interestDate),
		probeLoanValuation(ctx, svc, drawDate),
		probeLoanScope(ctx, svc, ids, drawDate),
		probeLoanCashInclude(ctx, svc, drawDate),
		probeLoanSettings(dbPath, ids),
	}
	return cases
}

func loadLoanSeedIDs(dbPath string) *loanSeedIDs {
	candidates := []string{
		envOr("NESTWORTH_PROBE_SEED_RESULTS", ""),
		filepath.Join(filepath.Dir(dbPath), "..", "seed", "seed-results.json"),
	}
	if base := os.Getenv("NESTWORTH_QA_OUTPUT_DIR"); base != "" {
		candidates = append(candidates, filepath.Join(base, "seed", "seed-results.json"))
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var parsed struct {
			IDs loanSeedIDs `json:"ids"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			continue
		}
		if parsed.IDs.DrawLocalDate != "" {
			return &parsed.IDs
		}
	}
	return nil
}

func probeLoanNeutral(ctx context.Context, svc *application.Service, id, name, date, wantDelta, wantSpending string) loanProbeCase {
	got, err := probeAnalyzeLoanDay(ctx, svc, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold}))
	expected := map[string]any{"nwDelta": wantDelta, "spending": wantSpending, "returnAmount": "0", "dividendInterest": "0", "residual": false}
	if err != nil {
		return loanProbeCase{ID: id, Name: name, Status: "FAIL", Expected: expected, Detail: err.Error()}
	}
	actual := map[string]any{
		"nwDelta": got.nwDelta.String(), "spending": got.spending.String(),
		"returnAmount": got.returnAmount.String(), "dividendInterest": got.dividendInterest.String(),
		"residual": got.residual, "begin": got.begin.String(), "end": got.end.String(),
	}
	ok := got.nwDelta.Equal(decimal.RequireFromString(wantDelta)) &&
		got.spending.Equal(decimal.RequireFromString(wantSpending)) &&
		got.returnAmount.IsZero() && got.dividendInterest.IsZero() && !got.residual
	status := "PASS"
	detail := fmt.Sprintf("date=%s nw_delta=%s spending=%s return=%s", date, got.nwDelta, got.spending, got.returnAmount)
	if !ok {
		status = "FAIL"
	}
	return loanProbeCase{ID: id, Name: name, Status: status, Expected: expected, Got: actual, Detail: detail}
}

func probeLoanInterest(ctx context.Context, svc *application.Service, date string) loanProbeCase {
	want := decimal.RequireFromString("-500")
	expected := map[string]any{"nwDelta": "-500", "spending": "-500", "returnAmount": "0", "dividendInterest": "0", "residual": false}
	got, err := probeAnalyzeLoanDay(ctx, svc, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold}))
	if err != nil {
		return loanProbeCase{ID: "fc07_interest_spending", Name: "cash interest is Spending, not return", Status: "FAIL", Expected: expected, Detail: err.Error()}
	}
	cats, catErr := svc.Categories(ctx, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold}), application.CategorySpending)
	catTotal := "<nil>"
	catOK := false
	if catErr == nil && cats.Total != nil {
		catTotal = cats.Total.CanonicalAmount()
		catOK = cats.Total.Amount().Equal(want)
	}
	actual := map[string]any{
		"nwDelta": got.nwDelta.String(), "spending": got.spending.String(),
		"returnAmount": got.returnAmount.String(), "dividendInterest": got.dividendInterest.String(),
		"residual": got.residual, "categoriesSpending": catTotal,
	}
	ok := got.nwDelta.Equal(want) && got.spending.Equal(want) && got.returnAmount.IsZero() &&
		got.dividendInterest.IsZero() && !got.residual && catOK
	status := "PASS"
	detail := fmt.Sprintf("date=%s nw_delta=%s spending=%s categories=%s return=%s", date, got.nwDelta, got.spending, catTotal, got.returnAmount)
	if !ok {
		status = "FAIL"
	}
	return loanProbeCase{ID: "fc07_interest_spending", Name: "cash interest is Spending, not return", Status: status, Expected: expected, Got: actual, Detail: detail}
}

func probeLoanValuation(ctx context.Context, svc *application.Service, date string) loanProbeCase {
	id := "r4_valuation_base_native"
	expected := map[string]any{"baseOK": true, "nativeOK": true, "nativeForced": false, "singleCurrency": true}
	baseQ := probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold})
	nativeQ := probeLoanQuery(date, domain.ValuationNative, true, domain.AnalysisScope{Kind: domain.ScopeHousehold})
	base, baseErr := svc.AssetChange(ctx, baseQ)
	native, nativeErr := svc.AssetChange(ctx, nativeQ)
	_, analyzeNativeErr := svc.Analyze(ctx, nativeQ)
	forced := ""
	if native.ValuationForced != nil {
		forced = *native.ValuationForced
	}
	got := map[string]any{
		"baseErr": errString(baseErr), "nativeErr": errString(nativeErr),
		"analyzeNativeErr": errString(analyzeNativeErr), "nativeForced": forced,
		"baseCurrency": moneyCurrency(base.Summary.Change), "nativeCurrency": moneyCurrency(native.Summary.Change),
	}
	if baseErr != nil || nativeErr != nil || analyzeNativeErr != nil {
		return loanProbeCase{ID: id, Name: "Valuation Base vs Native (single-currency loan)", Status: "FAIL", Expected: expected, Got: got, Detail: "Base or Native failed on single-currency loan"}
	}
	if native.ValuationForced != nil {
		return loanProbeCase{ID: id, Name: "Valuation Base vs Native (single-currency loan)", Status: "FAIL", Expected: expected, Got: got, Detail: "Native unexpectedly forced Base"}
	}
	return loanProbeCase{ID: id, Name: "Valuation Base vs Native (single-currency loan)", Status: "PASS", Expected: expected, Got: got, Detail: "Native not forced; Analyze Native succeeded"}
}

func probeLoanScope(ctx context.Context, svc *application.Service, ids *loanSeedIDs, date string) loanProbeCase {
	id := "r4_scope_portfolio_account"
	expected := map[string]any{"portfolio": "household", "accountCash": true, "accountLoan": true, "instrument": "SKIP loan fixture has no instruments"}
	got := map[string]any{}
	hh, hhErr := svc.Analyze(ctx, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold}))
	got["portfolioErr"] = errString(hhErr)
	got["portfolioDays"] = 0
	if hhErr == nil {
		got["portfolioDays"] = len(hh.Days)
	}
	if ids == nil || ids.AcctCash == "" || ids.AcctLoan == "" {
		return loanProbeCase{ID: id, Name: "Scope Portfolio (household) / Account", Status: "FAIL", Expected: expected, Got: got, Detail: "seed IDs missing acct_cash/acct_loan"}
	}
	cash, cashErr := svc.Analyze(ctx, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeAccount, ID: ids.AcctCash}))
	loan, loanErr := svc.Analyze(ctx, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeAccount, ID: ids.AcctLoan}))
	got["cashErr"] = errString(cashErr)
	got["loanErr"] = errString(loanErr)
	got["cashDays"] = 0
	got["loanDays"] = 0
	if cashErr == nil {
		got["cashDays"] = len(cash.Days)
	}
	if loanErr == nil {
		got["loanDays"] = len(loan.Days)
	}
	if hhErr != nil || cashErr != nil || loanErr != nil {
		return loanProbeCase{ID: id, Name: "Scope Portfolio (household) / Account", Status: "FAIL", Expected: expected, Got: got, Detail: "Analyze failed for household or account scope"}
	}
	return loanProbeCase{ID: id, Name: "Scope Portfolio (household) / Account", Status: "PASS", Expected: expected, Got: got, Detail: fmt.Sprintf("household/account cash=%s loan=%s", ids.AcctCash, ids.AcctLoan)}
}

func probeLoanCashInclude(ctx context.Context, svc *application.Service, date string) loanProbeCase {
	id := "r4_cash_include_exclude_draw"
	expected := map[string]any{"includeCashDietz": ">0", "excludeDiffers": true, "day": "draw capital-flow"}
	incl, inclErr := svc.Analyze(ctx, probeLoanQuery(date, domain.ValuationBase, true, domain.AnalysisScope{Kind: domain.ScopeHousehold}))
	excl, exclErr := svc.Analyze(ctx, probeLoanQuery(date, domain.ValuationBase, false, domain.AnalysisScope{Kind: domain.ScopeHousehold}))
	if inclErr != nil || exclErr != nil {
		return loanProbeCase{ID: id, Name: "Cash Include vs Exclude on draw day", Status: "FAIL", Expected: expected, Detail: fmt.Sprintf("include=%v exclude=%v", inclErr, exclErr)}
	}
	inclFlows := countCashDietzFlows(incl, date)
	exclFlows := countCashDietzFlows(excl, date)
	inclCap := dailyInvestedCapital(incl, date)
	exclCap := dailyInvestedCapital(excl, date)
	got := map[string]any{
		"includeCashDietz": inclFlows, "excludeCashDietz": exclFlows,
		"includeInvestedCapital": inclCap, "excludeInvestedCapital": exclCap,
	}
	differs := inclFlows != exclFlows || inclCap != exclCap
	ok := inclFlows > 0 && differs
	status := "PASS"
	detail := fmt.Sprintf("draw=%s includeFlows=%d excludeFlows=%d cap include=%q exclude=%q", date, inclFlows, exclFlows, inclCap, exclCap)
	if !ok {
		status = "FAIL"
	}
	return loanProbeCase{ID: id, Name: "Cash Include vs Exclude on draw day", Status: status, Expected: expected, Got: got, Detail: detail}
}

func probeLoanSettings(dbPath string, ids *loanSeedIDs) loanProbeCase {
	id := "r4_settings_week_currency"
	path := filepath.Join(filepath.Dir(dbPath), "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return loanProbeCase{ID: id, Name: "settings.json week_start and currency", Status: "FAIL", Detail: err.Error()}
	}
	var parsed struct {
		WeekStart string `json:"week_start"`
		Currency  string `json:"currency"`
		WindowW   int    `json:"window_width"`
		Language  string `json:"language"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return loanProbeCase{ID: id, Name: "settings.json week_start and currency", Status: "FAIL", Detail: err.Error()}
	}
	wantWeek := "monday"
	wantCcy := "AUD"
	if ids != nil {
		if ids.WeekStart != "" {
			wantWeek = ids.WeekStart
		}
		if ids.BaseCurrency != "" {
			wantCcy = ids.BaseCurrency
		}
	}
	expected := map[string]any{"week_start": wantWeek, "currency": wantCcy}
	got := map[string]any{"week_start": parsed.WeekStart, "currency": parsed.Currency, "window_width": parsed.WindowW, "language": parsed.Language, "path": path}
	ok := parsed.WeekStart == wantWeek && parsed.Currency == wantCcy
	status := "PASS"
	if !ok {
		status = "FAIL"
	}
	return loanProbeCase{ID: id, Name: "settings.json week_start and currency", Status: status, Expected: expected, Got: got, Detail: path}
}

func countCashDietzFlows(result domain.PeriodAnalysisResult, date string) int {
	n := 0
	for _, cd := range result.Days {
		if string(cd.Date) != date || !cd.Component.Cash {
			continue
		}
		n += len(cd.DietzCapitalFlows)
	}
	return n
}

func dailyInvestedCapital(result domain.PeriodAnalysisResult, date string) string {
	for _, day := range result.DailyReturns {
		if string(day.Date) == date && day.InvestedCapital != nil {
			return day.InvestedCapital.CanonicalAmount()
		}
	}
	return ""
}

type probeLoanMetrics struct {
	begin, end, nwDelta        decimal.Decimal
	spending, dividendInterest decimal.Decimal
	returnAmount               decimal.Decimal
	residual                   bool
}

func probeLoanQuery(date string, valuation domain.Valuation, includeCash bool, scope domain.AnalysisScope) domain.AnalysisQuery {
	return domain.AnalysisQuery{
		Scope: scope, From: date, To: date, Valuation: valuation,
		IncludeCash: includeCash, Basis: domain.ReturnBasisInvestment,
	}
}

func probeAnalyzeLoanDay(ctx context.Context, svc *application.Service, query domain.AnalysisQuery) (probeLoanMetrics, error) {
	result, err := svc.Analyze(ctx, query)
	if err != nil {
		return probeLoanMetrics{}, err
	}
	out := probeLoanMetrics{}
	date := string(query.From)
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

func writeLoanProbe(outPath string, out loanProbeOut, cases []loanProbeCase) error {
	out.Results = cases
	pass, fail, skip := 0, 0, 0
	all := true
	for _, c := range cases {
		switch c.Status {
		case "PASS":
			pass++
		case "SKIP":
			skip++
		default:
			all = false
			fail++
		}
	}
	out.AllPass = all
	out.Summary = fmt.Sprintf("%d PASS / %d FAIL / %d SKIP", pass, fail, skip)
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
	fmt.Printf("loan-fc07 written %s\n%s\n", outPath, string(raw))
	if !all {
		return fmt.Errorf("loan-fc07 probe failed: %s", out.Summary)
	}
	return nil
}

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
