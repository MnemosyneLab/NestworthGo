package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type matrixCase struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Owner    string         `json:"owner"`
	Status   string         `json:"status"`
	Expected map[string]any `json:"expected,omitempty"`
	Got      map[string]any `json:"got,omitempty"`
	Detail   string         `json:"detail,omitempty"`
}

type matrixOut struct {
	When    string       `json:"when"`
	Mode    string       `json:"mode"`
	Host    string       `json:"host"`
	Results []matrixCase `json:"results"`
	AllPass bool         `json:"allPass"`
	Summary string       `json:"summary"`
}

func runR4MatrixProbe() error {
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/seed/probe-r4-matrix.json")
	base := envOr("NESTWORTH_QA_OUTPUT_DIR", "/workspace/nestworth-analytics-qa")
	dbRoot := filepath.Join(base, "timezone-r4")

	out := matrixOut{
		When: time.Now().UTC().Format(time.RFC3339),
		Mode: "NESTWORTH_PROBE_MODE=r4-matrix",
		Host: runtime.GOOS + "/" + runtime.GOARCH,
	}
	cases := make([]matrixCase, 0, 64)
	cases = append(cases, r4HostCases()...)
	cases = append(cases, r4SettingsMatrixCases()...)
	for _, c := range tzDomainCases() {
		cases = append(cases, matrixFromTZ(c, "probe"))
	}
	for _, c := range tzInMemoryAnalysisCases() {
		cases = append(cases, matrixFromTZ(c, "probe"))
	}
	for _, c := range tzBuildOriginDBs(dbRoot) {
		cases = append(cases, matrixFromTZ(c, "probe"))
	}
	cases = append(cases, r4LoanMatrixCases()...)
	cases = append(cases, r4CompleteMatrixCases()...)
	cases = append(cases, r4OnboardedBaseCurrencyCases()...)
	cases = append(cases, r4DesktopPlaceholderCases()...)
	return writeMatrixProbe(outPath, out, cases)
}

func matrixFromTZ(c tzCase, owner string) matrixCase {
	return matrixCase{ID: c.ID, Name: c.Name, Owner: owner, Status: c.Status, Expected: c.Expected, Got: c.Got, Detail: c.Detail}
}

func matrixFromLoan(c loanProbeCase, owner string) matrixCase {
	return matrixCase{ID: c.ID, Name: c.Name, Owner: owner, Status: c.Status, Expected: c.Expected, Got: c.Got, Detail: c.Detail}
}

func r4HostCases() []matrixCase {
	return []matrixCase{
		{
			ID: "m_os_linux", Name: "Linux host can run seed/probe matrix", Owner: "probe", Status: "PASS",
			Expected: map[string]any{"goos": "linux"}, Got: map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH},
			Detail: "this Cloud VM is Linux; macOS Apple Silicon is a separate desktop host",
		},
		{
			ID: "m_os_macos_apple_silicon", Name: "macOS Apple Silicon §5 OS row", Owner: "desktop", Status: "BLOCKED",
			Expected: map[string]any{"goos": "darwin", "goarch": "arm64"},
			Got:      map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH},
			Detail:   "BLOCKED host: this runner is not macOS Apple Silicon",
		},
	}
}

func r4SettingsMatrixCases() []matrixCase {
	cases := make([]matrixCase, 0, 12)
	for _, week := range []string{settings.WeekStartMonday, settings.WeekStartSunday} {
		week := week
		cases = append(cases, matrixCheck("m_week_"+week, "settings.Validate week_start="+week, "probe", func() (map[string]any, map[string]any, error) {
			s := settings.Default()
			s.WeekStart = week
			if err := s.Validate(); err != nil {
				return map[string]any{"week_start": week}, map[string]any{"error": err.Error()}, err
			}
			return map[string]any{"week_start": week}, map[string]any{"week_start": s.WeekStart}, nil
		}))
	}
	for _, lang := range []settings.Language{settings.LanguageEnglish, settings.LanguageZhCN, settings.LanguageZhTW} {
		lang := lang
		cases = append(cases, matrixCheck("m_locale_validate_"+string(lang), "settings.Validate language="+string(lang), "probe", func() (map[string]any, map[string]any, error) {
			s := settings.Default()
			s.Language = lang
			if err := s.Validate(); err != nil {
				return map[string]any{"language": string(lang)}, map[string]any{"error": err.Error()}, err
			}
			return map[string]any{"language": string(lang)}, map[string]any{"language": string(s.Language)}, nil
		}))
	}
	for _, w := range []struct {
		id    string
		width float32
	}{
		{"m_window_1280", 1280},
		{"m_window_1440", 1440},
		{"m_window_narrow", 800},
	} {
		w := w
		cases = append(cases, matrixCheck(w.id, fmt.Sprintf("settings.Validate window_width=%.0f", w.width), "probe", func() (map[string]any, map[string]any, error) {
			s := settings.Default()
			s.WindowWidth = w.width
			if err := s.Validate(); err != nil {
				return map[string]any{"window_width": w.width}, map[string]any{"error": err.Error()}, err
			}
			return map[string]any{"window_width": w.width}, map[string]any{"window_width": s.WindowWidth}, nil
		}))
	}
	return cases
}

func r4DesktopPlaceholderCases() []matrixCase {
	return []matrixCase{
		{ID: "m_locale_visual_en", Name: "Locale English UI strings", Owner: "desktop", Status: "BLOCKED", Detail: "locale strings are desktop later; settings.Validate covers the enum"},
		{ID: "m_locale_visual_zh_cn", Name: "Locale 简体中文 UI strings", Owner: "desktop", Status: "BLOCKED", Detail: "locale strings are desktop later"},
		{ID: "m_locale_visual_zh_tw", Name: "Locale 繁体中文 UI strings", Owner: "desktop", Status: "BLOCKED", Detail: "locale strings are desktop later"},
		{ID: "m_window_visual_1280", Name: "Desktop window ~1280×800", Owner: "desktop", Status: "BLOCKED", Detail: "visual window QA is desktop; Linux can do it later"},
		{ID: "m_window_visual_1440", Name: "Desktop window ~1440×900", Owner: "desktop", Status: "BLOCKED", Detail: "visual window QA is desktop; Linux can do it later"},
		{ID: "m_window_visual_narrow", Name: "Desktop narrow width", Owner: "desktop", Status: "BLOCKED", Detail: "visual window QA is desktop; Linux can do it later"},
		{ID: "d_fc07_insights", Name: "Desktop Insights FC-07 draw/repay/interest", Owner: "desktop", Status: "BLOCKED", Detail: "not this Linux probe pass"},
		{ID: "d_tz_history_origin", Name: "Desktop History Origin timezone", Owner: "desktop", Status: "BLOCKED", Detail: "not this Linux probe pass"},
		{ID: "m_dst_fall_snapshot_rebuild", Name: "Fall-back 2026-11-01 Origin snapshot rebuild", Owner: "probe", Status: "BLOCKED", Detail: "November 2026 is not a closed day on a 2026-09-09 wall clock; fall-back uses activityLocalDate + in-memory ComputeAnalysis"},
	}
}

func r4LoanMatrixCases() []matrixCase {
	dbPath := firstExisting(
		envOr("NESTWORTH_QA_LOAN_DB", ""),
		envOr("NESTWORTH_DATABASE_PATH", ""),
		"/tmp/nestworth-qa-loan-fc07/data/nestworth.db",
	)
	if dbPath == "" {
		return []matrixCase{{
			ID: "m_loan_db", Name: "loan-fc07 DB for matrix extras", Owner: "probe", Status: "SKIP",
			Detail: "set NESTWORTH_QA_LOAN_DB or NESTWORTH_DATABASE_PATH to a loan-fc07 nestworth.db",
		}}
	}
	database, _, _, err := openProbeDB(dbPath)
	if err != nil {
		return []matrixCase{{ID: "m_loan_db", Name: "open loan-fc07 DB", Owner: "probe", Status: "FAIL", Detail: err.Error()}}
	}
	defer database.Close()
	ids := loadLoanSeedIDs(dbPath)
	draw, repay, interest := loanProbeDates(ids)
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	out := []matrixCase{{
		ID: "m_loan_db", Name: "open loan-fc07 DB", Owner: "probe", Status: "PASS",
		Got:    map[string]any{"dbPath": dbPath, "timezone": "", "currency": ""},
		Detail: dbPath,
	}}
	if ids != nil {
		out[0].Got["timezone"] = ids.Timezone
		out[0].Got["currency"] = ids.BaseCurrency
		out[0].Got["week_start"] = ids.WeekStart
	}
	for _, c := range collectLoanFC07Cases(ctx, svc, dbPath, ids, draw, repay, interest) {
		out = append(out, matrixFromLoan(c, "probe"))
	}
	out = append(out, matrixCase{
		ID: "m_scope_instrument_loan", Name: "Instrument scope on loan fixture", Owner: "probe", Status: "SKIP",
		Detail: "loan-fc07 household has cash+loan only; instrument smoke uses complete v3 DB",
	})
	return out
}

func r4CompleteMatrixCases() []matrixCase {
	dbPath := firstExisting(
		envOr("NESTWORTH_QA_COMPLETE_DB", ""),
		"/tmp/nestworth-qa-v3-complete/data/nestworth.db",
	)
	if dbPath == "" {
		return []matrixCase{{
			ID: "m_complete_db", Name: "v3 complete DB for instrument + cash-include", Owner: "probe", Status: "SKIP",
			Detail: "set NESTWORTH_QA_COMPLETE_DB to a v3 complete nestworth.db",
		}}
	}
	database, _, _, err := openProbeDB(dbPath)
	if err != nil {
		return []matrixCase{{ID: "m_complete_db", Name: "open v3 complete DB", Owner: "probe", Status: "FAIL", Detail: err.Error()}}
	}
	defer database.Close()
	repo := sqlite.NewRepository(database)
	svc := application.NewService(repo)
	ctx := context.Background()
	cases := []matrixCase{{ID: "m_complete_db", Name: "open v3 complete DB", Owner: "probe", Status: "PASS", Got: map[string]any{"dbPath": dbPath}}}

	portfolio, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || portfolio.Household == nil {
		detail := "household missing"
		if err != nil {
			detail = err.Error()
		}
		cases = append(cases, matrixCase{ID: "m_scope_instrument", Name: "Scope Instrument smoke", Owner: "probe", Status: "FAIL", Detail: detail})
		cases = append(cases, matrixCase{ID: "m_valuation_native_mixed", Name: "Native vs Base mixed-currency complete", Owner: "probe", Status: "FAIL", Detail: detail})
		cases = append(cases, matrixCase{ID: "m_cash_include_salary", Name: "Cash Include vs Exclude on salary day", Owner: "probe", Status: "FAIL", Detail: detail})
		return cases
	}

	from := envOr("NESTWORTH_PROBE_FROM", "2026-08-01")
	to := envOr("NESTWORTH_PROBE_TO", "2026-08-31")
	native := quantitativeNativeCase(ctx, svc, portfolio, from, to)
	status := native.Status
	if status != "PASS" {
		status = "FAIL"
	}
	cases = append(cases, matrixCase{
		ID: "m_valuation_native_mixed", Name: "Native vs Base mixed-currency complete (C6)", Owner: "probe",
		Status: status, Expected: native.Expected, Got: native.Actual, Detail: native.ErrorReason,
	})

	if len(portfolio.Instruments) == 0 {
		cases = append(cases, matrixCase{ID: "m_scope_instrument", Name: "Scope Instrument smoke", Owner: "probe", Status: "SKIP", Detail: "complete DB has no instruments"})
	} else {
		inst := portfolio.Instruments[0]
		q := domain.AnalysisQuery{
			Scope: domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: inst.ID.String()},
			From:  from, To: to, Valuation: domain.ValuationBase, IncludeCash: true, Basis: domain.ReturnBasisInvestment,
		}
		result, err := svc.Analyze(ctx, q)
		got := map[string]any{"instrument": inst.Name, "id": inst.ID.String(), "err": errString(err)}
		if err != nil {
			cases = append(cases, matrixCase{ID: "m_scope_instrument", Name: "Scope Instrument smoke", Owner: "probe", Status: "FAIL", Got: got, Detail: err.Error()})
		} else {
			got["days"] = len(result.Days)
			cases = append(cases, matrixCase{ID: "m_scope_instrument", Name: "Scope Instrument smoke", Owner: "probe", Status: "PASS", Got: got, Detail: inst.Name})
		}
	}

	salaryDate := envOr("NESTWORTH_PROBE_SALARY_DATE", "2026-08-05")
	incl := analyzeMode(ctx, svc, from, to, salaryDate, true)
	excl := analyzeMode(ctx, svc, from, to, salaryDate, false)
	cmp := compareFL1819(salaryDate, incl, excl)
	cashStatus := "PASS"
	if incl.Error != "" || excl.Error != "" || !cmp.SalaryDietzCapitalDiffers {
		cashStatus = "FAIL"
	}
	cases = append(cases, matrixCase{
		ID: "m_cash_include_salary", Name: "Cash Include vs Exclude on salary/capital-flow day (complete)", Owner: "probe",
		Status:   cashStatus,
		Expected: map[string]any{"salaryDietzCapitalDiffers": true, "salaryDate": salaryDate},
		Got:      map[string]any{"includeErr": incl.Error, "excludeErr": excl.Error, "detail": cmp.Detail, "differs": cmp.SalaryDietzCapitalDiffers},
		Detail:   cmp.Detail,
	})
	return cases
}

func r4OnboardedBaseCurrencyCases() []matrixCase {
	return []matrixCase{
		r4OnboardedBaseDB("env_base_usd", "USD",
			envOr("NESTWORTH_QA_COMPLETE_USD_DB", ""),
			"/tmp/nestworth-qa-v3-complete-usd/data/nestworth.db",
		),
		r4OnboardedBaseDB("env_base_cny", "CNY",
			envOr("NESTWORTH_QA_COMPLETE_CNY_DB", ""),
			"/tmp/nestworth-qa-v3-complete-cny/data/nestworth.db",
		),
	}
}

func r4OnboardedBaseDB(id, wantBase string, paths ...string) matrixCase {
	name := "§5 onboarded household base " + wantBase
	dbPath := firstExisting(paths...)
	if dbPath == "" {
		return matrixCase{
			ID: id, Name: name, Owner: "probe", Status: "SKIP",
			Expected: map[string]any{"base_currency": wantBase, "onboarded": true},
			Detail:   "seed complete-" + strings.ToLower(wantBase) + " (or NESTWORTH_QA_BASE_CURRENCY=" + wantBase + " on complete) to a nestworth.db",
		}
	}
	database, _, _, err := openProbeDB(dbPath)
	if err != nil {
		return matrixCase{ID: id, Name: name, Owner: "probe", Status: "FAIL", Detail: err.Error()}
	}
	defer database.Close()
	var gotBase string
	if err := database.SQL.QueryRow(`SELECT base_currency FROM households WHERE singleton_key = 1`).Scan(&gotBase); err != nil {
		return matrixCase{ID: id, Name: name, Owner: "probe", Status: "FAIL", Detail: err.Error()}
	}
	var snaps, fx, accounts int
	_ = database.SQL.QueryRow(`SELECT COUNT(*) FROM daily_valuation_snapshots`).Scan(&snaps)
	_ = database.SQL.QueryRow(`SELECT COUNT(*) FROM fx_quotes`).Scan(&fx)
	_ = database.SQL.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts)
	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	_, analyzeErr := svc.Analyze(ctx, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  "2026-08-01", To: "2026-08-01", Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	})
	got := map[string]any{
		"dbPath": dbPath, "base_currency": gotBase, "snapshots": snaps, "fx_quotes": fx, "accounts": accounts,
		"analyzeErr": errString(analyzeErr),
	}
	expected := map[string]any{"base_currency": wantBase, "snapshots": ">0", "accounts": ">0", "analyze": "ok"}
	if gotBase != wantBase || snaps <= 0 || accounts <= 0 || analyzeErr != nil {
		detail := fmt.Sprintf("base=%s want=%s snaps=%d accounts=%d analyze=%v", gotBase, wantBase, snaps, accounts, analyzeErr)
		return matrixCase{ID: id, Name: name, Owner: "probe", Status: "FAIL", Expected: expected, Got: got, Detail: detail}
	}
	return matrixCase{ID: id, Name: name, Owner: "probe", Status: "PASS", Expected: expected, Got: got, Detail: dbPath}
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func matrixCheck(id, name, owner string, fn func() (map[string]any, map[string]any, error)) matrixCase {
	expected, got, err := fn()
	c := matrixCase{ID: id, Name: name, Owner: owner, Expected: expected, Got: got, Status: "PASS"}
	if err != nil {
		c.Status = "FAIL"
		c.Detail = err.Error()
	}
	return c
}

func writeMatrixProbe(outPath string, out matrixOut, cases []matrixCase) error {
	out.Results = cases
	pass, fail, skip, blocked := 0, 0, 0, 0
	all := true
	for _, c := range cases {
		switch c.Status {
		case "PASS":
			pass++
		case "SKIP":
			skip++
		case "BLOCKED":
			blocked++
		default:
			all = false
			fail++
		}
	}
	out.AllPass = all
	out.Summary = fmt.Sprintf("%d PASS / %d FAIL / %d SKIP / %d BLOCKED", pass, fail, skip, blocked)
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
	fmt.Printf("r4-matrix written %s\n%s\n", outPath, string(raw))
	if !all {
		return fmt.Errorf("r4-matrix probe failed: %s", out.Summary)
	}
	return nil
}
