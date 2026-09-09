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
}

func runLoanFC07Probe() error {
	dbPath := envOr("NESTWORTH_DATABASE_PATH", filepath.Join("/workspace/nestworth-analytics-qa", "data", "nestworth.db"))
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/seed/probe-loan-fc07.json")

	drawDate := envOr("NESTWORTH_PROBE_DRAW_DATE", "")
	repayDate := envOr("NESTWORTH_PROBE_REPAY_DATE", "")
	interestDate := envOr("NESTWORTH_PROBE_INTEREST_DATE", "")
	if ids := loadLoanSeedIDs(dbPath); ids != nil {
		if drawDate == "" {
			drawDate = ids.DrawLocalDate
		}
		if repayDate == "" {
			repayDate = ids.RepayLocalDate
		}
		if interestDate == "" {
			interestDate = ids.InterestLocalDate
		}
	}
	if drawDate == "" || repayDate == "" || interestDate == "" {
		sgt := mustLoc("Asia/Singapore")
		anchor, err := time.Parse(time.RFC3339, envOr("NESTWORTH_QA_ANCHOR", "2026-07-26T00:00:00Z"))
		if err != nil {
			return err
		}
		local := func(offset int) string {
			return anchor.Add(time.Duration(offset) * 24 * time.Hour).In(sgt).Format("2006-01-02")
		}
		if drawDate == "" {
			drawDate = local(1)
		}
		if repayDate == "" {
			repayDate = local(3)
		}
		if interestDate == "" {
			interestDate = local(5)
		}
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

	cases := []loanProbeCase{
		probeLoanNeutral(ctx, svc, "fc07_draw_nw0", "draw net-worth neutrality", drawDate, "0", "0"),
		probeLoanNeutral(ctx, svc, "fc07_repay_nw0", "repay principal net-worth neutrality", repayDate, "0", "0"),
		probeLoanInterest(ctx, svc, interestDate),
	}
	return writeLoanProbe(outPath, out, cases)
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
	got, err := probeAnalyzeLoanDay(ctx, svc, date)
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
	got, err := probeAnalyzeLoanDay(ctx, svc, date)
	if err != nil {
		return loanProbeCase{ID: "fc07_interest_spending", Name: "cash interest is Spending, not return", Status: "FAIL", Expected: expected, Detail: err.Error()}
	}
	cats, catErr := svc.Categories(ctx, probeLoanQuery(date), application.CategorySpending)
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

type probeLoanMetrics struct {
	begin, end, nwDelta        decimal.Decimal
	spending, dividendInterest decimal.Decimal
	returnAmount               decimal.Decimal
	residual                   bool
}

func probeLoanQuery(date string) domain.AnalysisQuery {
	return domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  date, To: date, Valuation: domain.ValuationBase,
		IncludeCash: true, Basis: domain.ReturnBasisInvestment,
	}
}

func probeAnalyzeLoanDay(ctx context.Context, svc *application.Service, date string) (probeLoanMetrics, error) {
	result, err := svc.Analyze(ctx, probeLoanQuery(date))
	if err != nil {
		return probeLoanMetrics{}, err
	}
	out := probeLoanMetrics{}
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
