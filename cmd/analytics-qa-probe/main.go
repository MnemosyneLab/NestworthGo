package main

import (
	"context"
	"database/sql"
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

func main() {
	switch os.Getenv("NESTWORTH_PROBE_MODE") {
	case "cash-include":
		if err := runCashIncludeProbe(); err != nil {
			fmt.Fprintf(os.Stderr, "cash-include probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "co03":
		if err := runCO03Probe(); err != nil {
			fmt.Fprintf(os.Stderr, "co03 probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "residual":
		if err := runResidualProbe(); err != nil {
			fmt.Fprintf(os.Stderr, "residual probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "reconcile":
		if err := runReconcileProbe(); err != nil {
			fmt.Fprintf(os.Stderr, "reconcile probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "quantitative":
		if err := runQuantitativeProbe(); err != nil {
			fmt.Fprintf(os.Stderr, "quantitative probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "loan-fc07":
		if err := runLoanFC07Probe(); err != nil {
			fmt.Fprintf(os.Stderr, "loan-fc07 probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "timezone-r4":
		if err := runTimezoneR4Probe(); err != nil {
			fmt.Fprintf(os.Stderr, "timezone-r4 probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "fixtures":
		if err := runFixturesProbe(); err != nil {
			fmt.Fprintf(os.Stderr, "fixtures probe failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	runLegacyProbe()
}

type moneyOut struct {
	Amount   string `json:"amount,omitempty"`
	Currency string `json:"currency,omitempty"`
}

type salaryDayOut struct {
	Date               string    `json:"date"`
	Status             string    `json:"status,omitempty"`
	InvestedCapital    *moneyOut `json:"investedCapital,omitempty"`
	ReturnAmount       *moneyOut `json:"returnAmount,omitempty"`
	ReturnRate         *string   `json:"returnRate,omitempty"`
	CashDietzFlowSum   *moneyOut `json:"cashDietzFlowSum,omitempty"`
	CashDietzFlowCount int       `json:"cashDietzFlowCount"`
}

type modeResult struct {
	IncludeCash      bool          `json:"includeCash"`
	RatedDays        int           `json:"ratedDays"`
	TotalDays        int           `json:"totalDays"`
	Status           string        `json:"status,omitempty"`
	PeriodReturnAmt  *moneyOut     `json:"periodReturnAmount,omitempty"`
	PeriodReturnRate *string       `json:"periodReturnRate,omitempty"`
	SalaryDay        *salaryDayOut `json:"salaryDay,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type cashProbeOut struct {
	From             string     `json:"from"`
	To               string     `json:"to"`
	Valuation        string     `json:"valuation"`
	Basis            string     `json:"basis"`
	Scope            string     `json:"scope"`
	DBPath           string     `json:"dbPath"`
	DBOpenMode       string     `json:"dbOpenMode"`
	SalaryLocalDate  string     `json:"salaryLocalDate"`
	IncludeCashTrue  modeResult `json:"includeCashTrue"`
	IncludeCashFalse modeResult `json:"includeCashFalse"`
	FL1819           fl1819Out  `json:"fl18_19"`
	Error            string     `json:"error,omitempty"`
}

type fl1819Out struct {
	SalaryDietzCapitalDiffers bool   `json:"salaryDietzCapitalDiffers"`
	Detail                    string `json:"detail"`
}

func moneyPtr(m *domain.SignedMoney) *moneyOut {
	if m == nil {
		return nil
	}
	return &moneyOut{Amount: m.CanonicalAmount(), Currency: m.Currency().String()}
}

func openProbeDB(dbPath string) (*sqlite.DB, string, string, error) {
	if db, err := sqlite.OpenReadOnlyForVerify(dbPath); err == nil {
		return db, "OpenReadOnlyForVerify(mode=ro,query_only)", "", nil
	} else {
		roErr := err.Error()
		// Explicit mode=ro URI via database/sql wrapped into sqlite.DB shape is not
		// exported; retry OpenReadOnlyForVerify once more after a short note, else
		// fall back to writable Open (may contend with live Nestworth).
		uri := "file:" + filepath.ToSlash(dbPath) + "?mode=ro&_pragma=query_only%3d1&_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3d1"
		raw, err2 := sql.Open("sqlite", uri)
		if err2 == nil {
			raw.SetMaxOpenConns(1)
			if pingErr := raw.Ping(); pingErr == nil {
				// Can read via raw SQL, but Service needs sqlite.Repository(*DB).
				_ = raw.Close()
			} else {
				_ = raw.Close()
				roErr = fmt.Sprintf("%s; mode=ro URI ping: %v", roErr, pingErr)
			}
		}
		db, err3 := sqlite.Open(dbPath)
		if err3 != nil {
			return nil, "failed", roErr, fmt.Errorf("read-only open failed (%s); writable Open failed: %w", roErr, err3)
		}
		note := fmt.Sprintf("Open(writable-fallback); read-only failed: %s", roErr)
		return db, note, roErr, nil
	}
}

func runCashIncludeProbe() error {
	dbPath := os.Getenv("NESTWORTH_DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("/workspace/nestworth-analytics-qa/data", "nestworth.db")
	}
	outPath := os.Getenv("NESTWORTH_PROBE_OUT")
	if outPath == "" {
		outPath = "/workspace/nestworth-analytics-qa/seed/probe-cash-include.json"
	}
	logPath := os.Getenv("NESTWORTH_PROBE_LOG")
	if logPath == "" {
		logPath = "/workspace/nestworth-analytics-qa/logs/10-probe-cash.log"
	}
	from := envOr("NESTWORTH_PROBE_FROM", "2026-08-01")
	to := envOr("NESTWORTH_PROBE_TO", "2026-09-07")
	// Seed act_day10_income: origin local 2026-07-26 + 10d.
	salaryDate := envOr("NESTWORTH_PROBE_SALARY_DATE", "2026-08-05")

	out := cashProbeOut{
		From:            from,
		To:              to,
		Valuation:       string(domain.ValuationBase),
		Basis:           string(domain.ReturnBasisInvestment),
		Scope:           string(domain.ScopeHousehold),
		DBPath:          dbPath,
		SalaryLocalDate: salaryDate,
	}

	database, openMode, lockNote, err := openProbeDB(dbPath)
	out.DBOpenMode = openMode
	if lockNote != "" && err != nil {
		out.Error = err.Error()
		return writeCashProbe(outPath, logPath, out)
	}
	if lockNote != "" {
		// Non-fatal: opened writable after RO failure (possible lock).
		out.Error = "sqlite_ro_fallback: " + lockNote
	}
	if err != nil {
		out.Error = err.Error()
		return writeCashProbe(outPath, logPath, out)
	}
	defer database.Close()

	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()

	out.IncludeCashTrue = analyzeMode(ctx, svc, from, to, salaryDate, true)
	out.IncludeCashFalse = analyzeMode(ctx, svc, from, to, salaryDate, false)
	out.FL1819 = compareFL1819(salaryDate, out.IncludeCashTrue, out.IncludeCashFalse)

	return writeCashProbe(outPath, logPath, out)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func analyzeMode(ctx context.Context, svc *application.Service, from, to, salaryDate string, includeCash bool) modeResult {
	mr := modeResult{IncludeCash: includeCash}
	query := domain.AnalysisQuery{
		Scope:       domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:        from,
		To:          to,
		Valuation:   domain.ValuationBase,
		IncludeCash: includeCash,
		Basis:       domain.ReturnBasisInvestment,
	}
	result, err := svc.Analyze(ctx, query)
	if err != nil {
		mr.Error = err.Error()
		return mr
	}
	mr.RatedDays = result.Coverage.RatedDays
	mr.TotalDays = result.Coverage.TotalDays
	mr.Status = string(result.Status)
	mr.PeriodReturnAmt = moneyPtr(result.ReturnAmount)
	if result.ReturnRate != nil {
		s := result.ReturnRate.String()
		mr.PeriodReturnRate = &s
	}

	sd := &salaryDayOut{Date: salaryDate}
	for _, day := range result.DailyReturns {
		if string(day.Date) == salaryDate {
			sd.Status = string(day.Status)
			sd.InvestedCapital = moneyPtr(day.InvestedCapital)
			sd.ReturnAmount = moneyPtr(day.Amount)
			if day.Rate != nil {
				s := day.Rate.String()
				sd.ReturnRate = &s
			}
			break
		}
	}

	var sum decimal.Decimal
	var cur domain.CurrencyCode
	flowCount := 0
	have := false
	for _, cd := range result.Days {
		if string(cd.Date) != salaryDate || !cd.Component.Cash {
			continue
		}
		for _, f := range cd.DietzCapitalFlows {
			flowCount++
			if !have {
				sum = f.Amount.Amount()
				cur = f.Amount.Currency()
				have = true
			} else {
				sum = sum.Add(f.Amount.Amount())
			}
		}
	}
	sd.CashDietzFlowCount = flowCount
	if have {
		if m, err := domain.NewSignedMoney(sum, cur); err == nil {
			sd.CashDietzFlowSum = moneyPtr(&m)
		}
	}
	mr.SalaryDay = sd
	return mr
}

func compareFL1819(salaryDate string, incl, excl modeResult) fl1819Out {
	tCap, fCap := "", ""
	tFlows, fFlows := 0, 0
	tFlowAmt, fFlowAmt := "", ""
	if incl.SalaryDay != nil {
		tFlows = incl.SalaryDay.CashDietzFlowCount
		if incl.SalaryDay.InvestedCapital != nil {
			tCap = incl.SalaryDay.InvestedCapital.Amount
		}
		if incl.SalaryDay.CashDietzFlowSum != nil {
			tFlowAmt = incl.SalaryDay.CashDietzFlowSum.Amount
		}
	}
	if excl.SalaryDay != nil {
		fFlows = excl.SalaryDay.CashDietzFlowCount
		if excl.SalaryDay.InvestedCapital != nil {
			fCap = excl.SalaryDay.InvestedCapital.Amount
		}
		if excl.SalaryDay.CashDietzFlowSum != nil {
			fFlowAmt = excl.SalaryDay.CashDietzFlowSum.Amount
		}
	}
	differs := tCap != fCap || tFlows != fFlows || tFlowAmt != fFlowAmt
	return fl1819Out{
		SalaryDietzCapitalDiffers: differs,
		Detail: fmt.Sprintf(
			"salaryDay=%s investedCapital include=%q exclude=%q cashDietzFlows include=%d(%s) exclude=%d(%s)",
			salaryDate, tCap, fCap, tFlows, tFlowAmt, fFlows, fFlowAmt,
		),
	}
}

func writeCashProbe(outPath, logPath string, out cashProbeOut) error {
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
	fmt.Println(string(raw))

	sgt := time.FixedZone("SGT", 8*3600)
	note := fmt.Sprintf("[%s SGT] cash-include probe from=%s to=%s db=%s open=%s rated true=%d/%d false=%d/%d fl18_19_differs=%v err=%q\n",
		time.Now().In(sgt).Format("2006-01-02 15:04:05"),
		out.From, out.To, out.DBPath, out.DBOpenMode,
		out.IncludeCashTrue.RatedDays, out.IncludeCashTrue.TotalDays,
		out.IncludeCashFalse.RatedDays, out.IncludeCashFalse.TotalDays,
		out.FL1819.SalaryDietzCapitalDiffers, out.Error,
	)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(note); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, note)
	return nil
}

func runLegacyProbe() {
	dbPath := os.Getenv("NESTWORTH_DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("/workspace/nestworth-analytics-qa/data", "nestworth.db")
	}
	database, err := sqlite.Open(dbPath)
	if err != nil {
		panic(err)
	}
	defer database.Close()
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()

	from := "2026-07-26"
	var originStarted string
	if err := database.SQL.QueryRow(`SELECT started_at FROM history_origins LIMIT 1`).Scan(&originStarted); err == nil && originStarted != "" {
		if t, err := time.Parse(time.RFC3339Nano, originStarted); err == nil {
			sgt := time.FixedZone("SGT", 8*3600)
			from = t.In(sgt).Format("2006-01-02")
		} else if t, err := time.Parse(time.RFC3339, originStarted); err == nil {
			sgt := time.FixedZone("SGT", 8*3600)
			from = t.In(sgt).Format("2006-01-02")
		}
	}
	to := ""
	_ = database.SQL.QueryRow(`SELECT MAX(local_date) FROM daily_valuation_snapshots`).Scan(&to)
	if to == "" {
		to = time.Now().In(time.FixedZone("SGT", 8*3600)).Add(-24 * time.Hour).Format("2006-01-02")
	}
	if v := os.Getenv("NESTWORTH_PROBE_FROM"); v != "" {
		from = v
	}
	if v := os.Getenv("NESTWORTH_PROBE_TO"); v != "" {
		to = v
	}

	query := domain.AnalysisQuery{
		Scope:       domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:        from,
		To:          to,
		Valuation:   domain.ValuationBase,
		IncludeCash: true,
		Basis:       domain.ReturnBasisInvestment,
	}
	result, err := svc.Analyze(ctx, query)
	if err != nil {
		panic(err)
	}

	ok, partial, unavailable := 0, 0, 0
	nonOK := 0
	var negativeSamples []string
	var zeroishSamples []string
	var partialDates []string
	for _, day := range result.DailyReturns {
		switch day.Status {
		case domain.CompletenessOK:
			ok++
		case domain.CompletenessPartial:
			partial++
			nonOK++
			if len(partialDates) < 8 {
				partialDates = append(partialDates, fmt.Sprintf("%s amount=%v rate=%v", day.Date, day.Amount, day.Rate))
			}
		default:
			unavailable++
			nonOK++
			fmt.Printf("UNAVAIL %s status=%s\n", day.Date, day.Status)
		}
		if day.Rate != nil && day.Rate.IsNegative() && len(negativeSamples) < 8 {
			negativeSamples = append(negativeSamples, fmt.Sprintf("%s rate=%v amount=%v status=%s", day.Date, day.Rate, day.Amount, day.Status))
		}
		if day.Rate != nil && day.Rate.IsZero() && len(zeroishSamples) < 5 {
			zeroishSamples = append(zeroishSamples, fmt.Sprintf("%s rate=%v status=%s", day.Date, day.Rate, day.Status))
		}
	}

	fmt.Printf("coverage rated=%d total=%d status=%s from=%s to=%s\n",
		result.Coverage.RatedDays, result.Coverage.TotalDays, result.Status, from, to)
	fmt.Printf("day_statuses ok=%d partial=%d unavailable=%d non_ok=%d\n", ok, partial, unavailable, nonOK)
	fmt.Printf("has_completeness_not_ok=%v\n", nonOK > 0)
	if len(partialDates) > 0 {
		fmt.Printf("sample_partial_days: %s\n", strings.Join(partialDates, " | "))
	}
	if len(negativeSamples) > 0 {
		fmt.Printf("sample_negative_return_days (%d shown): %s\n", len(negativeSamples), strings.Join(negativeSamples, " | "))
	} else {
		fmt.Printf("sample_negative_return_days: (none)\n")
	}
	if len(zeroishSamples) > 0 {
		fmt.Printf("sample_zero_rate_days: %s\n", strings.Join(zeroishSamples, " | "))
	} else {
		fmt.Printf("sample_zero_rate_days: (none — engine may mark flat days partial)\n")
	}

	types := []application.ContributionReturnType{
		application.ContributionTotalReturn,
		application.ContributionRealized,
		application.ContributionUnrealized,
		application.ContributionDividendInterest,
	}
	fmt.Println("contribution_types:")
	availableTypes := []string{}
	for _, rt := range types {
		contrib, err := svc.Contribution(ctx, query, rt, application.ContributionGroupInstrument, application.ContributionSortAmountDesc)
		if err != nil {
			fmt.Printf("  %s ERROR %v\n", rt, err)
			continue
		}
		avail := contrib.Available
		if avail {
			availableTypes = append(availableTypes, string(rt))
		}
		fmt.Printf("  type=%s available=%v status=%s reason=%q rated=%d/%d rows=%d valuationForced=%v\n",
			rt, avail, contrib.Status, contrib.MissingReason, contrib.Coverage.RatedDays, contrib.Coverage.TotalDays, len(contrib.Rows), contrib.ValuationForced)
		for i, row := range contrib.Rows {
			if i >= 3 {
				break
			}
			fmt.Printf("    row key=%s amount=%v rate=%v rated=%d/%d status=%s\n",
				row.Key, row.Amount, row.Rate, row.Coverage.RatedDays, row.Coverage.TotalDays, row.Status)
		}
	}
	fmt.Printf("contribution_types_available=%s\n", strings.Join(availableTypes, ","))
}
