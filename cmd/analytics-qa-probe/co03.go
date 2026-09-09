package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type co03Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type co03Row struct {
	Key    string     `json:"key"`
	Label  string     `json:"label,omitempty"`
	Amount *co03Money `json:"amount,omitempty"`
}

type co03DayComp struct {
	Date             string               `json:"date"`
	ComponentKey     string               `json:"componentKey"`
	Cash             bool                 `json:"cash"`
	InstrumentID     string               `json:"instrumentId,omitempty"`
	HoldingID        string               `json:"holdingId,omitempty"`
	AccountID        string               `json:"accountId"`
	Status           string               `json:"status"`
	AssetBuckets     map[string]co03Money `json:"assetBuckets,omitempty"`
	ReturnComponents map[string]co03Money `json:"returnComponents,omitempty"`
	Attributed       []co03Attr           `json:"attributedEffects,omitempty"`
}

type co03Attr struct {
	Bucket            string     `json:"bucket,omitempty"`
	ReturnComponent   string     `json:"returnComponent,omitempty"`
	Amount            *co03Money `json:"amount,omitempty"`
	RelatedHolding    string     `json:"relatedHolding,omitempty"`
	RelatedInstrument string     `json:"relatedInstrument,omitempty"`
}

type co03Out struct {
	DBPath              string        `json:"dbPath"`
	From                string        `json:"from"`
	To                  string        `json:"to"`
	DividendLocalDate   string        `json:"dividendLocalDate"`
	CategoriesAvailable bool          `json:"categoriesAvailable"`
	CategoriesStatus    string        `json:"categoriesStatus"`
	CategoriesTotal     *co03Money    `json:"categoriesTotal,omitempty"`
	CategoriesRows      []co03Row     `json:"categoriesRows"`
	CategoriesPath      string        `json:"categoriesPath"` // asset_buckets | return_effects
	ContribAvailable    bool          `json:"contributionAvailable"`
	ContribStatus       string        `json:"contributionStatus"`
	ContribReason       string        `json:"contributionReason,omitempty"`
	ContribRows         []co03Row     `json:"contributionRows"`
	ContribRated        string        `json:"contributionRated"`
	APIHasDividendRows  bool          `json:"apiHasDividendContributionRows"`
	UIWouldShowEmpty    bool          `json:"uiWouldShowEmpty"`
	HasReturnAttrFX     bool          `json:"hasReturnAttributedEffects"`
	HasReturnComponents bool          `json:"hasReturnComponentsMap"`
	DividendDayComps    []co03DayComp `json:"dividendDayComponents"`
	AssetAnalyzeDay     []co03DayComp `json:"assetAnalyzeDividendDay"`
	Conclusion          string        `json:"conclusion"`
	Detail              string        `json:"detail"`
	Error               string        `json:"error,omitempty"`
}

func moneyOutCo03(m *domain.SignedMoney) *co03Money {
	if m == nil {
		return nil
	}
	return &co03Money{Amount: m.CanonicalAmount(), Currency: m.Currency().String()}
}

func moneyMap(m map[domain.AttributionBucket]domain.SignedMoney) map[string]co03Money {
	if len(m) == 0 {
		return nil
	}
	out := map[string]co03Money{}
	for k, v := range m {
		if v.Currency() == "" {
			continue
		}
		out[string(k)] = co03Money{Amount: v.CanonicalAmount(), Currency: v.Currency().String()}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func returnMap(m map[domain.ReturnComponent]domain.SignedMoney) map[string]co03Money {
	if len(m) == 0 {
		return nil
	}
	out := map[string]co03Money{}
	for k, v := range m {
		if v.Currency() == "" {
			continue
		}
		out[string(k)] = co03Money{Amount: v.CanonicalAmount(), Currency: v.Currency().String()}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dayComps(result domain.PeriodAnalysisResult, date string) []co03DayComp {
	var out []co03DayComp
	for _, day := range result.Days {
		if string(day.Date) != date {
			continue
		}
		c := co03DayComp{
			Date:             string(day.Date),
			ComponentKey:     day.Component.Key(),
			Cash:             day.Component.Cash,
			AccountID:        day.Component.AccountID.String(),
			Status:           string(day.Status),
			AssetBuckets:     moneyMap(day.AssetBuckets),
			ReturnComponents: returnMap(day.ReturnComponents),
		}
		if day.Component.HoldingID != nil {
			c.HoldingID = day.Component.HoldingID.String()
		}
		if day.Component.InstrumentID != nil {
			c.InstrumentID = day.Component.InstrumentID.String()
		}
		for _, a := range day.AttributedEffects {
			attr := co03Attr{Amount: moneyOutCo03(&a.Amount)}
			if a.AssetBucket != nil {
				attr.Bucket = string(*a.AssetBucket)
			}
			if a.ReturnComponent != nil {
				attr.ReturnComponent = string(*a.ReturnComponent)
			}
			if a.RelatedHoldingID != nil {
				attr.RelatedHolding = a.RelatedHoldingID.String()
			}
			if a.RelatedInstrumentID != nil {
				attr.RelatedInstrument = a.RelatedInstrumentID.String()
			}
			c.Attributed = append(c.Attributed, attr)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ComponentKey < out[j].ComponentKey })
	return out
}

func runCO03Probe() error {
	dbPath := envOr("NESTWORTH_DATABASE_PATH", "/workspace/nestworth-analytics-qa/data/nestworth.db")
	logPath := envOr("NESTWORTH_PROBE_LOG", "/workspace/nestworth-analytics-qa/logs/11-co03-div.txt")
	from := envOr("NESTWORTH_PROBE_FROM", "2026-08-01")
	to := envOr("NESTWORTH_PROBE_TO", "2026-09-07")
	divDate := envOr("NESTWORTH_PROBE_DIV_DATE", "2026-08-28")

	out := co03Out{DBPath: dbPath, From: from, To: to, DividendLocalDate: divDate}

	database, openMode, lockNote, err := openProbeDB(dbPath)
	_ = openMode
	if err != nil {
		out.Error = err.Error()
		if lockNote != "" {
			out.Error = lockNote + "; " + out.Error
		}
		return writeCO03(logPath, out)
	}
	defer database.Close()
	if lockNote != "" {
		out.Detail = "sqlite_ro_fallback: " + lockNote
	}

	svc := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	query := domain.AnalysisQuery{
		Scope:       domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:        from,
		To:          to,
		Valuation:   domain.ValuationBase,
		IncludeCash: true,
		Basis:       domain.ReturnBasisInvestment,
	}

	cats, err := svc.Categories(ctx, query, application.CategoryDividendInterest)
	if err != nil {
		out.Error = "Categories: " + err.Error()
		return writeCO03(logPath, out)
	}
	out.CategoriesAvailable = cats.Available
	out.CategoriesStatus = string(cats.Status)
	out.CategoriesTotal = moneyOutCo03(cats.Total)
	for _, r := range cats.Rows {
		out.CategoriesRows = append(out.CategoriesRows, co03Row{Key: r.Key, Label: r.Label, Amount: moneyOutCo03(r.Amount)})
	}

	contrib, err := svc.Contribution(ctx, query, application.ContributionDividendInterest, application.ContributionGroupInstrument, application.ContributionSortAmountDesc)
	if err != nil {
		out.Error = "Contribution: " + err.Error()
		return writeCO03(logPath, out)
	}
	out.ContribAvailable = contrib.Available
	out.ContribStatus = string(contrib.Status)
	out.ContribReason = contrib.MissingReason
	out.ContribRated = fmt.Sprintf("%d/%d", contrib.Coverage.RatedDays, contrib.Coverage.TotalDays)
	for _, r := range contrib.Rows {
		out.ContribRows = append(out.ContribRows, co03Row{Key: r.Key, Label: r.Label, Amount: moneyOutCo03(r.Amount)})
	}
	out.APIHasDividendRows = len(contrib.Rows) > 0
	out.UIWouldShowEmpty = !contrib.Available || len(contrib.Rows) == 0

	// Return-path Analyze (same as Contribution)
	retResult, err := svc.Analyze(ctx, query)
	if err != nil {
		out.Error = "Analyze(return): " + err.Error()
		return writeCO03(logPath, out)
	}
	out.DividendDayComps = dayComps(retResult, divDate)
	for _, d := range out.DividendDayComps {
		if d.ReturnComponents != nil {
			if _, ok := d.ReturnComponents[string(domain.ReturnDividendInterest)]; ok {
				out.HasReturnComponents = true
			}
		}
		for _, a := range d.Attributed {
			if a.ReturnComponent == string(domain.ReturnDividendInterest) {
				out.HasReturnAttrFX = true
			}
		}
	}

	// Asset-path uses same Analyze inputs as Categories (ComputeWithValuationFallback).
	// Service.Analyze already uses the primary compute path; dump same day for comparison.
	out.AssetAnalyzeDay = out.DividendDayComps

	// Infer categories path
	if out.HasReturnAttrFX {
		out.CategoriesPath = "return_effects"
	} else {
		out.CategoriesPath = "asset_buckets_fallback"
	}
	// More precise: check asset day for bucket vs return
	for _, d := range out.AssetAnalyzeDay {
		if d.AssetBuckets != nil {
			if _, ok := d.AssetBuckets[string(domain.BucketDividendInterest)]; ok {
				if !out.HasReturnAttrFX {
					out.CategoriesPath = "asset_buckets_fallback"
				}
			}
		}
	}

	switch {
	case out.APIHasDividendRows && out.CategoriesTotal != nil:
		out.Conclusion = "PASS_BOTH"
		out.Detail += "; Categories and Contribution both show dividend rows"
	case !out.APIHasDividendRows && out.CategoriesTotal != nil && !out.HasReturnComponents:
		out.Conclusion = "ROOT_CAUSE_RETURN_COMPONENT_MISSING"
		out.Detail += "; Categories shows Div&Interest via asset buckets (cash leg) but Contribution dividend_interest has 0 rows because day.ReturnComponents[dividend_interest] never populated on holding/instrument component"
	case !out.APIHasDividendRows && out.HasReturnComponents:
		out.Conclusion = "ROOT_CAUSE_PROJECTION_FILTER"
		out.Detail += "; ReturnComponents exist but projectDividendContribution filtered them out (eligibility/group key)"
	default:
		out.Conclusion = "INCONCLUSIVE"
		out.Detail += "; unexpected combination"
	}

	return writeCO03(logPath, out)
}

func writeCO03(logPath string, out co03Out) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("CO-03 Dividend Contribution probe\n")
	b.WriteString("================================\n")
	b.Write(raw)
	b.WriteString("\n\nSUMMARY\n")
	fmt.Fprintf(&b, "conclusion=%s\n", out.Conclusion)
	fmt.Fprintf(&b, "categories_rows=%d total=%v path=%s\n", len(out.CategoriesRows), out.CategoriesTotal, out.CategoriesPath)
	fmt.Fprintf(&b, "contribution_rows=%d available=%v ui_empty=%v\n", len(out.ContribRows), out.ContribAvailable, out.UIWouldShowEmpty)
	fmt.Fprintf(&b, "has_return_components=%v has_return_attributed=%v\n", out.HasReturnComponents, out.HasReturnAttrFX)
	fmt.Fprintf(&b, "detail=%s\n", out.Detail)
	if out.Error != "" {
		fmt.Fprintf(&b, "error=%s\n", out.Error)
	}
	if err := os.WriteFile(logPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Print(b.String())
	return nil
}
