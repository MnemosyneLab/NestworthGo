package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type reconcileAmount struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type reconcileDay struct {
	Date         string                     `json:"date"`
	ComponentKey string                     `json:"componentKey"`
	Status       string                     `json:"status"`
	Beginning    *reconcileAmount           `json:"beginning,omitempty"`
	Ending       *reconcileAmount           `json:"ending,omitempty"`
	Buckets      map[string]reconcileAmount `json:"buckets,omitempty"`
	Delta        *reconcileAmount           `json:"delta,omitempty"`
}

type reconcileDate struct {
	Date           string           `json:"date"`
	ComponentCount int              `json:"componentCount"`
	Change         *reconcileAmount `json:"change,omitempty"`
	BucketSum      *reconcileAmount `json:"bucketSum,omitempty"`
	Delta          *reconcileAmount `json:"delta,omitempty"`
}

type reconcileOut struct {
	DBPath             string                     `json:"dbPath"`
	From               string                     `json:"from"`
	To                 string                     `json:"to"`
	Status             string                     `json:"status"`
	Beginning          *reconcileAmount           `json:"beginning,omitempty"`
	Ending             *reconcileAmount           `json:"ending,omitempty"`
	Change             *reconcileAmount           `json:"change,omitempty"`
	Waterfall          map[string]reconcileAmount `json:"waterfall"`
	WaterfallSum       *reconcileAmount           `json:"waterfallSum,omitempty"`
	Delta              *reconcileAmount           `json:"delta,omitempty"`
	ResidualIssueCount int                        `json:"residualIssueCount"`
	DailyDeltas        []reconcileDate            `json:"dailyDeltas"`
	NonZeroDayDeltas   []reconcileDay             `json:"nonZeroDayDeltas"`
	Error              string                     `json:"error,omitempty"`
}

func reconcileAmountFromMoney(value *domain.SignedMoney) *reconcileAmount {
	if value == nil || value.Currency() == "" {
		return nil
	}
	return &reconcileAmount{Amount: value.CanonicalAmount(), Currency: value.Currency().String()}
}

func reconcileAmountFromDecimal(value decimal.Decimal, currency domain.CurrencyCode) *reconcileAmount {
	if currency == "" {
		return nil
	}
	return &reconcileAmount{Amount: value.String(), Currency: currency.String()}
}

func reconcileDayBuckets(day domain.ComponentDay) (map[string]reconcileAmount, decimal.Decimal, domain.CurrencyCode) {
	buckets := make(map[string]reconcileAmount, len(day.AssetBuckets))
	total := decimal.Zero
	var currency domain.CurrencyCode
	for bucket, value := range day.AssetBuckets {
		if value.Currency() == "" {
			continue
		}
		buckets[string(bucket)] = reconcileAmount{Amount: value.CanonicalAmount(), Currency: value.Currency().String()}
		total = total.Add(value.Amount())
		if currency == "" {
			currency = value.Currency()
		}
	}
	return buckets, total, currency
}

func runReconcileProbe() error {
	dbPath := envOr("NESTWORTH_DATABASE_PATH", "/workspace/nestworth-analytics-qa/data/nestworth.db")
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/probes/reconcile.json")
	from := envOr("NESTWORTH_PROBE_FROM", "2026-08-01")
	to := envOr("NESTWORTH_PROBE_TO", "2026-08-31")
	out := reconcileOut{DBPath: dbPath, From: from, To: to, Waterfall: map[string]reconcileAmount{}, DailyDeltas: []reconcileDate{}, NonZeroDayDeltas: []reconcileDay{}}

	database, _, lockNote, err := openProbeDB(dbPath)
	if err != nil {
		out.Error = err.Error()
		return writeReconcile(outPath, out)
	}
	defer database.Close()
	if lockNote != "" {
		out.Error = lockNote
	}

	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: from, To: to, Valuation: domain.ValuationBase, IncludeCash: true, Basis: domain.ReturnBasisInvestment}
	service := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	result, err := service.Analyze(ctx, query)
	if err != nil {
		out.Error = "Analyze: " + err.Error()
		return writeReconcile(outPath, out)
	}
	projection, err := service.AssetChange(ctx, query)
	if err != nil {
		out.Error = "AssetChange: " + err.Error()
		return writeReconcile(outPath, out)
	}
	out.Status = string(projection.Status)
	out.Beginning = reconcileAmountFromMoney(projection.Summary.BeginningValue)
	out.Ending = reconcileAmountFromMoney(projection.Summary.EndingValue)
	out.Change = reconcileAmountFromMoney(projection.Summary.Change)
	out.ResidualIssueCount = projection.ResidualIssueCount

	waterfallSum := decimal.Zero
	var currency domain.CurrencyCode
	for _, row := range projection.Waterfall {
		if row.Amount == nil {
			continue
		}
		out.Waterfall[row.Key] = reconcileAmount{Amount: row.Amount.CanonicalAmount(), Currency: row.Amount.Currency().String()}
		waterfallSum = waterfallSum.Add(row.Amount.Amount())
		if currency == "" {
			currency = row.Amount.Currency()
		}
	}
	out.WaterfallSum = reconcileAmountFromDecimal(waterfallSum, currency)
	if projection.Summary.Change != nil && currency != "" {
		out.Delta = reconcileAmountFromDecimal(projection.Summary.Change.Amount().Sub(waterfallSum), currency)
	}

	type dateTotals struct {
		change, buckets decimal.Decimal
		currency        domain.CurrencyCode
		components      int
	}
	daily := map[string]*dateTotals{}
	for _, day := range result.Days {
		buckets, roundedBucketSum, dayCurrency := reconcileDayBuckets(day)
		if day.BeginningValue.Currency() == "" || day.EndingValue.Currency() == "" || dayCurrency == "" {
			continue
		}
		bucketSum := roundedBucketSum
		if len(day.AssetBucketExact) > 0 {
			bucketSum = decimal.Zero
			for _, amount := range day.AssetBucketExact {
				bucketSum = bucketSum.Add(amount)
			}
		}
		totals := daily[string(day.Date)]
		if totals == nil {
			totals = &dateTotals{currency: dayCurrency}
			daily[string(day.Date)] = totals
		}
		totals.change = totals.change.Add(day.EndingValue.Amount().Sub(day.BeginningValue.Amount()))
		totals.buckets = totals.buckets.Add(bucketSum)
		totals.components++
		delta := day.EndingValue.Amount().Sub(day.BeginningValue.Amount()).Sub(bucketSum)
		if delta.IsZero() {
			continue
		}
		out.NonZeroDayDeltas = append(out.NonZeroDayDeltas, reconcileDay{
			Date: string(day.Date), ComponentKey: day.Component.Key(), Status: string(day.Status),
			Beginning: reconcileAmountFromMoney(&day.BeginningValue), Ending: reconcileAmountFromMoney(&day.EndingValue),
			Buckets: buckets, Delta: reconcileAmountFromDecimal(delta, dayCurrency),
		})
	}
	for date, totals := range daily {
		delta := totals.change.Sub(totals.buckets)
		out.DailyDeltas = append(out.DailyDeltas, reconcileDate{Date: date, ComponentCount: totals.components, Change: reconcileAmountFromDecimal(totals.change, totals.currency), BucketSum: reconcileAmountFromDecimal(totals.buckets, totals.currency), Delta: reconcileAmountFromDecimal(delta, totals.currency)})
	}
	sort.Slice(out.DailyDeltas, func(i, j int) bool { return out.DailyDeltas[i].Date < out.DailyDeltas[j].Date })
	sort.Slice(out.NonZeroDayDeltas, func(i, j int) bool {
		if out.NonZeroDayDeltas[i].Date != out.NonZeroDayDeltas[j].Date {
			return out.NonZeroDayDeltas[i].Date < out.NonZeroDayDeltas[j].Date
		}
		return out.NonZeroDayDeltas[i].ComponentKey < out.NonZeroDayDeltas[j].ComponentKey
	})
	return writeReconcile(outPath, out)
}

func writeReconcile(outPath string, out reconcileOut) error {
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
	if out.Error != "" {
		return fmt.Errorf("reconcile probe: %s", out.Error)
	}
	if out.Delta != nil && out.Delta.Amount != "0" {
		return fmt.Errorf("waterfall delta is %s %s", out.Delta.Amount, out.Delta.Currency)
	}
	return nil
}
